import assert from 'node:assert/strict';
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { test } from 'node:test';
import { spawn } from 'node:child_process';
import { pathToFileURL } from 'node:url';
import { resolve } from 'node:path';
import { DatabaseSync } from 'node:sqlite';
import { once } from 'node:events';
import { CouncilStore } from '../dist/store.js';

function fixture(t) {
  const directory = mkdtempSync(join(tmpdir(), 'council-store-'));
  const stores = [];
  const cleanups = [];
  const open = () => { const store = new CouncilStore(directory); stores.push(store); return store; };
  t.after(async () => {
    for (const cleanup of cleanups.toReversed()) await cleanup();
    for (const store of stores) { try { store.close(); } catch {} }
    rmSync(directory, { recursive: true, force: true });
  });
  return { directory, open, cleanup: callback => cleanups.push(callback) };
}

function record(id, updatedAt = new Date().toISOString()) {
  return { id, createdAt: updatedAt, updatedAt, status: 'completed', input: { mode: 'delegate', task: 'inspect', agents: ['codex'], permission: 'read' }, steps: [], output: 'done' };
}

test('runs and shared context survive reopening and update without duplication', t => {
  const { open } = fixture(t);
  const first = open();
  first.saveRun(record('old', '2026-01-01T00:00:00.000Z'));
  first.saveRun(record('latest', '2026-02-01T00:00:00.000Z'));
  first.saveRun({ ...record('latest', '2026-03-01T00:00:00.000Z'), output: 'revised' });
  first.putContext('api-contract', 'Version 1', 'claude');
  const second = open();
  assert.equal(second.getRun('latest').output, 'revised');
  assert.deepEqual(second.listRuns(1).map(run => run.id), ['latest']);
  assert.equal(second.listRuns().length, 2);
  assert.equal(second.getRun('missing'), undefined);
  assert.equal(second.getContext('api-contract').value, 'Version 1');
  second.putContext('api-contract', 'Version 2', 'codex');
  assert.equal(first.getContext('api-contract').author, 'codex');
  assert.equal(first.listContext().length, 1);
});

test('events maintain an ordered durable cursor scoped to the requested run', t => {
  const { open } = fixture(t);
  const store = open();
  const a = store.appendEvent('one', 'started', { step: 1 });
  store.appendEvent('two', 'started', {});
  const b = store.appendEvent('one', 'completed', { step: 2 });
  assert.ok(b.sequence > a.sequence);
  assert.deepEqual(store.events('one', a.sequence).map(event => event.type), ['completed']);
  assert.deepEqual(open().events('one').map(event => event.data.step), [1, 2]);
  assert.throws(() => store.events('one', -1), /cursor/);
});

test('mail is recipient scoped and only that recipient can acknowledge delivery', t => {
  const store = fixture(t).open();
  const message = store.sendMessage({ from: 'codex', to: 'junie', subject: 'Review', body: 'Please review the diff', runId: 'one' });
  assert.equal(store.inbox('claude').length, 0);
  assert.equal(store.inbox('junie').length, 1);
  assert.equal(store.acknowledge(message.id, 'codex'), false);
  assert.equal(store.inbox('junie').length, 1);
  assert.equal(store.acknowledge(message.id, 'junie'), true);
  assert.equal(store.acknowledge(message.id, 'junie'), false);
  assert.equal(store.inbox('junie').length, 0);
  assert.ok(store.inbox('junie', false)[0].acknowledgedAt);
  assert.throws(() => store.sendMessage({ from: 'codex', to: 'junie', subject: 'too large', body: 'x'.repeat(32769) }), /32768/);
});

test('two connections serialize lease ownership and batch conflict rolls back every path', t => {
  const { open } = fixture(t);
  const one = open();
  const two = open();
  const [lease] = one.reserve('codex', ['src/**'], 60000, 'run');
  assert.equal(two.leases()[0].id, lease.id);
  assert.throws(() => two.reserve('claude', ['docs/readme.md', 'SRC/INDEX.ts'], 60000), /conflict/);
  assert.equal(one.leases().length, 1);
  assert.throws(() => two.reserve('claude', ['src'], 60000), /conflict/);
  assert.throws(() => two.reserve('claude', ['**'], 60000), /conflict/);
  assert.equal(two.release('claude', ['src/**']), 0);
  const [renewed] = two.reserve('codex', ['SRC/**'], 120000);
  assert.equal(renewed.id, lease.id);
  assert.equal(one.leases().length, 1);
  assert.equal(one.release('codex', ['src/**']), 1);
  assert.equal(two.reserve('claude', ['src/main.ts'], 60000).length, 1);
});

test('expired leases do not block other agents and exact files do not claim siblings', async t => {
  const store = fixture(t).open();
  store.reserve('codex', ['src/a.ts'], 1);
  await new Promise(resolve => setTimeout(resolve, 15));
  assert.equal(store.leases().length, 0);
  store.reserve('claude', ['src/a.ts'], 60000);
  store.reserve('junie', ['src/b.ts'], 60000);
  assert.equal(store.leases().length, 2);
  assert.equal(store.release('claude'), 1);
  assert.equal(store.leases()[0].owner, 'junie');
});

test('reservation paths reject traversal, absolute paths and ambiguous globs', t => {
  const store = fixture(t).open();
  for (const path of ['../secrets', 'src/../secrets', 'C:\\private', '/etc/passwd', '\\\\server\\share', 'src/*.ts', 'src//a.ts', './a', 'a:stream', 'src/']) {
    assert.throws(() => store.reserve('codex', [path], 60000), /path|traversal|glob/);
  }
  const [lease] = store.reserve('codex', ['src\\main.ts'], 60000);
  assert.deepEqual(lease.paths, ['src/main.ts']);
  assert.throws(() => store.reserve('claude', ['SRC/main.ts'], 60000), /conflict/);
  assert.throws(() => store.reserve('codex', ['safe'], 0), /TTL/);
});

test('independent processes cannot both reserve the same path', async t => {
  const { directory, open } = fixture(t);
  open();
  const moduleUrl = pathToFileURL(resolve('dist/store.js')).href;
  const code = `import { CouncilStore } from ${JSON.stringify(moduleUrl)};
    const store = new CouncilStore(process.env.COUNCIL_TEST_STATE);
    try { store.reserve(process.env.COUNCIL_TEST_OWNER, ['shared/**'], 60000); console.log('reserved'); }
    catch (error) { if (!String(error).includes('conflict')) throw error; console.log('conflict'); }
    finally { store.close(); }`;
  const attempt = owner => new Promise((resolveResult, reject) => {
    const child = spawn(process.execPath, ['--input-type=module', '-e', code], {
      env: { ...process.env, COUNCIL_TEST_STATE: directory, COUNCIL_TEST_OWNER: owner },
      windowsHide: true,
      stdio: ['ignore', 'pipe', 'pipe'],
    });
    let output = '';
    let error = '';
    child.stdout.on('data', chunk => { output += chunk; });
    child.stderr.on('data', chunk => { error += chunk; });
    child.once('error', reject);
    child.once('close', status => status === 0 ? resolveResult(output.trim()) : reject(new Error(error)));
  });
  assert.deepEqual((await Promise.all([attempt('codex-process'), attempt('claude-process')])).sort(), ['conflict', 'reserved']);
});

test('newer schema versions fail closed instead of mutating existing state', t => {
  const { directory } = fixture(t);
  const db = new DatabaseSync(join(directory, 'state.db'));
  db.exec('PRAGMA user_version = 99');
  db.close();
  assert.throws(() => new CouncilStore(directory), /newer than supported/);
  const check = new DatabaseSync(join(directory, 'state.db'));
  assert.equal(check.prepare('PRAGMA user_version').get().user_version, 99);
  check.close();
});

test('large aggregate run records fit the 16 MiB record budget and overflow is rejected', t => {
  const store = fixture(t).open();
  store.saveRun({ ...record('large'), output: 'x'.repeat(5 * 1024 * 1024) });
  assert.equal(store.getRun('large').output.length, 5 * 1024 * 1024);
  assert.throws(() => store.saveRun({ ...record('oversized'), output: 'x'.repeat(16 * 1024 * 1024) }), /16 MiB/);
  assert.equal(store.getRun('oversized'), undefined);
});

test('lease expiry uses one transaction timestamp and never returns a different stored owner', t => {
  const store = fixture(t).open();
  const realNow = Date.now;
  try {
    Date.now = () => 1000;
    store.reserve('codex', ['src/main.ts'], 1);
    let samples = 0;
    Date.now = () => ++samples === 1 ? 1000 : 1002;
    assert.throws(() => store.reserve('claude', ['src/main.ts'], 10000), /conflict/);
    Date.now = () => 1002;
    const replacement = store.reserve('claude', ['src/main.ts'], 10000)[0];
    const persisted = store.leases()[0];
    assert.equal(persisted.owner, 'claude');
    assert.equal(persisted.id, replacement.id);
  } finally { Date.now = realNow; }
});

function ownedRun(store, id, owner) {
  const run = { ...record(id), status: 'queued' };
  store.saveRun(run);
  store.claimRun(id, owner);
  return run;
}

test('schema version one migrates existing results without losing their data', t => {
  const { directory, open } = fixture(t);
  const legacy = new DatabaseSync(join(directory, 'state.db'));
  legacy.exec('CREATE TABLE runs (id TEXT PRIMARY KEY, updated_at TEXT NOT NULL, document TEXT NOT NULL); PRAGMA user_version = 1;');
  legacy.prepare('INSERT INTO runs(id, updated_at, document) VALUES (?, ?, ?)').run('legacy', '2026-01-01T00:00:00.000Z', JSON.stringify(record('legacy')));
  legacy.close();
  const store = open();
  assert.equal(store.getRun('legacy').output, 'done');
  store.registerInstance('migrated-owner', process.pid);
  ownedRun(store, 'new-run', 'migrated-owner');
  assert.ok(store.acquireSlot('migrated-owner', 'new-run', 1));
  const check = new DatabaseSync(join(directory, 'state.db'));
  assert.equal(check.prepare('PRAGMA user_version').get().user_version, 2);
  check.close();
});

test('registered instances fence run claims and share the most restrictive advertised slot limit', t => {
  const { open } = fixture(t);
  const one = open();
  const two = open();
  one.registerInstance('instance-a', process.pid);
  two.registerInstance('instance-b', process.pid);
  assert.equal(one.heartbeat('instance-a'), true);
  assert.equal(one.heartbeat('missing-instance'), false);
  assert.throws(() => one.registerInstance('wrong-pid', process.pid + 1), /own process PID/);
  const a = ownedRun(one, 'run-a', 'instance-a');
  const b = ownedRun(two, 'run-b', 'instance-b');
  assert.throws(() => two.claimRun(a.id, 'instance-b'), /live Council instance/);
  assert.throws(() => two.acquireSlot('instance-b', a.id, 2), /does not own/);
  assert.throws(() => one.unregisterInstance('instance-a'), /unfinished runs/);
  const first = one.acquireSlot('instance-a', a.id, 2);
  const second = two.acquireSlot('instance-b', b.id, 2);
  assert.ok(first && second);
  assert.equal(one.acquireSlot('instance-a', a.id, 2), undefined);
  one.releaseSlot(second, 'instance-a');
  assert.equal(one.acquireSlot('instance-a', a.id, 2), undefined, 'A different owner cannot release the slot');
  two.releaseSlot(second, 'instance-b');
  // B lowers its advertised limit. A cannot exceed it even when asking for more.
  assert.equal(two.acquireSlot('instance-b', b.id, 1), undefined);
  assert.equal(one.acquireSlot('instance-a', a.id, 3), undefined);
  one.releaseSlot(first, 'instance-a');
  const only = one.acquireSlot('instance-a', a.id, 3);
  assert.ok(only);
  assert.equal(two.acquireSlot('instance-b', b.id, 1), undefined);
  one.releaseSlot(only, 'instance-a');
  one.finalizeRun({ ...a, status: 'completed' });
  two.finalizeRun({ ...b, status: 'completed' });
  two.unregisterInstance('instance-b');
  assert.equal(two.heartbeat('instance-b'), false);
  assert.throws(() => two.acquireSlot('instance-b', b.id, 1), /not registered/);
  one.unregisterInstance('instance-a');
});

test('durable cancellation survives partial saves and wins atomic final completion', t => {
  const { open } = fixture(t);
  const writer = open();
  const requester = open();
  writer.registerInstance('cancel-owner', process.pid);
  const run = ownedRun(writer, 'cancel-run', 'cancel-owner');
  assert.equal(requester.requestCancellation(run.id), true);
  assert.equal(requester.requestCancellation(run.id), false);
  writer.saveRun({ ...run, status: 'running', output: 'Partial result' });
  assert.throws(() => writer.acquireSlot('cancel-owner', run.id, 1), /Cancellation/);
  const final = writer.finalizeRun({ ...run, status: 'completed', output: 'Retained result' });
  assert.equal(final.status, 'cancelled');
  assert.equal(final.output, 'Retained result');
  assert.equal(requester.getRun(run.id).status, 'cancelled');
  assert.equal(requester.events(run.id).filter(event => event.type === 'cancel_requested').length, 1);
  assert.equal(requester.requestCancellation(run.id), false);
  assert.equal(requester.requestCancellation('missing-run'), false);
  assert.equal(writer.finalizeRun({ ...run, status: 'completed' }).status, 'cancelled');
  writer.unregisterInstance('cancel-owner');
});

test('legacy cancellation events also prevent a stale success result', t => {
  const store = fixture(t).open();
  store.registerInstance('legacy-cancel-owner', process.pid);
  const run = ownedRun(store, 'legacy-cancel', 'legacy-cancel-owner');
  store.appendEvent(run.id, 'cancel_requested', {});
  assert.equal(store.finalizeRun({ ...run, status: 'completed' }).status, 'cancelled');
});

test('orphan recovery keeps live stale-heartbeat owners but fails hard-killed owners and releases slots', { timeout: 15000 }, async t => {
  const setup = fixture(t);
  const store = setup.open();
  const moduleUrl = pathToFileURL(resolve('dist/store.js')).href;
  const code = `import { CouncilStore } from ${JSON.stringify(moduleUrl)};
    const store = new CouncilStore(process.env.COUNCIL_TEST_STATE);
    const now = new Date().toISOString();
    store.registerInstance('child-owner', process.pid);
    store.saveRun({id:'child-run',status:'running',createdAt:now,updatedAt:now,input:{mode:'delegate',task:'offline test',agents:['codex'],permission:'read'},steps:[],output:'partial child output'});
    store.claimRun('child-run','child-owner');
    store.acquireSlot('child-owner','child-run',1);
    console.log('ready');
    setInterval(() => {},1000);`;
  const child = spawn(process.execPath, ['--input-type=module', '-e', code], {
    env: { ...process.env, COUNCIL_TEST_STATE: setup.directory },
    windowsHide: true, stdio: ['ignore', 'pipe', 'pipe'],
  });
  setup.cleanup(async () => {
    if (child.exitCode === null && child.signalCode === null) { const closed = once(child, 'close'); child.kill('SIGKILL'); await closed; }
  });
  let diagnostics = '';
  child.stderr.on('data', chunk => { diagnostics += chunk; });
  await new Promise((resolveReady, reject) => {
    let output = '';
    child.stdout.on('data', chunk => { output += chunk; if (output.includes('ready\n')) resolveReady(); });
    child.once('error', reject);
    child.once('exit', () => reject(new Error(`Owner child exited before readiness: ${diagnostics}`)));
  });
  const raw = new DatabaseSync(join(setup.directory, 'state.db'));
  raw.prepare('UPDATE instances SET heartbeat_at = 0 WHERE id = ?').run('child-owner');
  raw.close();
  store.saveRun({ ...record('unowned-run'), status: 'running' });
  store.registerInstance('surviving-owner', process.pid);
  const survivor = ownedRun(store, 'surviving-run', 'surviving-owner');
  assert.deepEqual(store.recoverOrphans(), []);
  assert.equal(store.getRun('child-run').status, 'running');
  assert.equal(store.acquireSlot('surviving-owner', survivor.id, 1), undefined);
  const closed = once(child, 'close');
  child.kill('SIGKILL');
  await closed;
  const recovered = store.recoverOrphans();
  assert.deepEqual(recovered.map(run => run.id), ['child-run']);
  assert.equal(store.getRun('child-run').status, 'failed');
  assert.equal(store.getRun('child-run').output, 'partial child output');
  assert.match(store.getRun('child-run').error, /no task was retried/);
  assert.equal(store.getRun('unowned-run').status, 'running', 'An unowned run has no proven-dead PID');
  assert.ok(store.acquireSlot('surviving-owner', survivor.id, 1));
  assert.deepEqual(store.recoverOrphans(), [], 'Recovery is idempotent');
  assert.throws(() => store.claimRun('child-run', 'surviving-owner'), /terminal state/);
  assert.ok(store.events('child-run').some(event => event.type === 'run.failed' && event.data.reason === 'owner_exited'));
});
