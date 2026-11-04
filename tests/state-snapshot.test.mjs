import test from 'node:test';
import assert from 'node:assert/strict';
import { execFile } from 'node:child_process';
import { copyFileSync, existsSync, mkdirSync, mkdtempSync, readFileSync, readdirSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { DatabaseSync } from 'node:sqlite';
import { fileURLToPath } from 'node:url';
import { promisify } from 'node:util';
import { CouncilService, loadConfig, STATE_DATABASES, verifyStateSnapshot } from '../dist/index.js';

const execute = promisify(execFile);
const cli = fileURLToPath(new URL('../dist/cli.js', import.meta.url));

function fixture(t) {
  const root = mkdtempSync(join(tmpdir(), 'council-snapshot-'));
  const service = new CouncilService(loadConfig(root, { backend: 'mock' }));
  t.after(async () => { await service.close(); rmSync(root, { recursive: true, force: true }); });
  return { root, service, state: join(root, '.council') };
}

async function populate(service) {
  const run = await service.run({ mode: 'delegate', agents: ['codex'], task: 'Offline snapshot fixture; no provider calls.' });
  const room = service.rooms.create({ title: 'Snapshot project', brief: 'Keep the complete collaboration state.', members: [{ agent: 'codex', role: 'lead' }], tasks: [{ title: 'Inspect', brief: 'Read the project.', permission: 'read' }] });
  service.rooms.postMessage(room.id, { author: 'human', body: 'Preserve this complete room message.' });
  const chat = service.privateChats.create({ participants: ['human', 'reviewer'], roomId: room.id });
  service.privateChats.send({ chatId: chat.id, from: 'human', body: 'Private fixture text 雪', runId: run.id });
  const report = service.delegation.report({ author: 'host', kind: 'decision', body: 'Which task should go first?', roomIds: [room.id] });
  const billing = service.billing.update('codex', { mode: 'local', plan: 'Offline fixture' }, service.billing.get('codex').revision);
  return { run, room, chat, report, billing };
}

test('all-store snapshots verify and restore complete collaboration data without invoking a provider', async t => {
  const { root, service } = fixture(t);
  const saved = await populate(service);
  const snapshot = await service.snapshot(join(root, 'snapshot'));
  assert.deepEqual(snapshot.manifest.databases.map(entry => entry.name), STATE_DATABASES);
  assert.deepEqual(snapshot.manifest.absentDatabases, []);
  assert.equal(snapshot.manifest.consistency, 'per-database');
  assert.equal(snapshot.manifest.artifactsIncluded, false);
  const checked = await verifyStateSnapshot(snapshot.destination);
  assert.equal(checked.valid, true, JSON.stringify(checked.issues));
  assert.deepEqual(checked.manifest, JSON.parse(readFileSync(join(snapshot.destination, 'manifest.json'), 'utf8')));
  const restoredRoot = join(root, 'restored');
  mkdirSync(join(restoredRoot, '.council'), { recursive: true });
  for (const entry of snapshot.manifest.databases) copyFileSync(join(snapshot.destination, entry.name), join(restoredRoot, '.council', entry.name));
  const restored = new CouncilService(loadConfig(restoredRoot, { backend: 'mock' }));
  try {
    assert.deepEqual(restored.getRun(saved.run.id), saved.run);
    assert.equal(restored.rooms.get(saved.room.id).messages.at(-1).body, 'Preserve this complete room message.');
    assert.equal(restored.privateChats.page({ chatId: saved.chat.id, actor: 'reviewer' }).messages[0].body, 'Private fixture text 雪');
    assert.equal(restored.delegation.reportsPage().reports[0].id, saved.report.id);
    assert.equal(restored.billing.get('codex').plan, 'Offline fixture');
    assert.equal(restored.health(true).status, 'ok');
  } finally { await restored.close(); }
});

test('snapshot and health do not initialize absent features, and CLI verification never creates project state', async t => {
  const { root, service, state } = fixture(t);
  const before = readdirSync(state).sort();
  const health = service.health(true);
  assert.equal(health.databases.length, 5);
  assert.equal(health.databases.filter(entry => entry.present).length, 1);
  assert.equal(health.storageBytes > 0, true);
  assert.deepEqual(readdirSync(state).sort(), before);
  const snapshot = await service.snapshot(join(root, 'minimal'));
  assert.deepEqual(snapshot.manifest.databases.map(entry => entry.name), ['state.db']);
  assert.deepEqual(snapshot.manifest.absentDatabases, STATE_DATABASES.slice(1));
  const unused = join(root, 'unused-project'); mkdirSync(unused);
  const result = await execute(process.execPath, [cli, 'verify-snapshot', snapshot.destination, '--workspace', unused], { windowsHide: true, timeout: 10000 });
  assert.equal(JSON.parse(result.stdout).valid, true);
  assert.equal(existsSync(join(unused, '.council')), false);
});

test('snapshot publication is exclusive and failure preserves an existing destination unchanged', async t => {
  const { root, service } = fixture(t);
  const output = join(root, 'existing'); mkdirSync(output);
  writeFileSync(join(output, 'keep.txt'), 'user data');
  await assert.rejects(service.snapshot(output), /EEXIST|already exists/i);
  assert.equal(readFileSync(join(output, 'keep.txt'), 'utf8'), 'user data');
  assert.deepEqual(readdirSync(output), ['keep.txt']);
  const first = await service.snapshot(join(root, 'completed'));
  const manifest = readFileSync(join(first.destination, 'manifest.json'));
  await assert.rejects(service.snapshot(first.destination), /EEXIST|already exists/i);
  assert.deepEqual(readFileSync(join(first.destination, 'manifest.json')), manifest);
});

test('cancelling an active all-store copy keeps the exact reason and publishes no successful snapshot', async t => {
  const { root, service } = fixture(t);
  const controller = new AbortController(), reason = new Error('Stop this requested snapshot.');
  const destination = join(root, 'cancelled');
  const copying = service.snapshot(destination, controller.signal);
  controller.abort(reason);
  await assert.rejects(copying, error => error === reason);
  assert.equal(existsSync(destination), false);
  assert.equal(service.health().maintenanceOperations, 0);
  service.store.putContext('after-cancel', 'The application connection is still writable.', 'test');
  assert.equal(service.store.getContext('after-cancel').value, 'The application connection is still writable.');
  await assert.rejects(service.snapshot(destination, controller.signal), error => error === reason);
  assert.equal(existsSync(destination), false);
});

test('verification detects altered or missing databases and rejects unsafe or incomplete manifests', async t => {
  const { root, service } = fixture(t);
  await populate(service);
  const snapshot = await service.snapshot(join(root, 'tampered'));
  const changed = new DatabaseSync(join(snapshot.destination, 'billing.sqlite'));
  changed.exec('PRAGMA user_version = 99'); changed.close();
  rmSync(join(snapshot.destination, 'rooms.sqlite'));
  const checked = await verifyStateSnapshot(snapshot.destination);
  assert.equal(checked.valid, false);
  assert(checked.issues.some(issue => issue.startsWith('billing.sqlite: SHA-256')));
  assert(checked.issues.some(issue => issue.startsWith('billing.sqlite: schema')));
  assert(checked.issues.some(issue => issue.startsWith('rooms.sqlite:')));
  const path = join(snapshot.destination, 'manifest.json');
  const manifest = JSON.parse(readFileSync(path, 'utf8'));
  manifest.databases[0].name = '../outside.sqlite'; writeFileSync(path, JSON.stringify(manifest));
  await assert.rejects(verifyStateSnapshot(snapshot.destination), /Invalid or duplicated database/);
  delete manifest.databases[0]; writeFileSync(path, JSON.stringify(manifest));
  await assert.rejects(verifyStateSnapshot(snapshot.destination), /Invalid or duplicated database/);
});

test('health includes optional database failures and refuses incomplete all-store backups', async t => {
  const { root, service, state } = fixture(t);
  writeFileSync(join(state, 'private-chats.sqlite'), 'not a database');
  const health = service.health(true);
  assert.equal(health.status, 'degraded');
  assert(health.issues.some(issue => issue.startsWith('private-chats.sqlite:')));
  const destination = join(root, 'invalid-source');
  await assert.rejects(service.snapshot(destination), /not a database|file is not a database/i);
  assert.equal(existsSync(join(destination, 'manifest.json')), false);
});
