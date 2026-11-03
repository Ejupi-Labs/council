import assert from 'node:assert/strict';
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { test } from 'node:test';
import { spawn } from 'node:child_process';
import { pathToFileURL } from 'node:url';
import { resolve } from 'node:path';
import { DatabaseSync } from 'node:sqlite';
import { CouncilStore } from '../dist/store.js';

function fixture(t) {
  const directory = mkdtempSync(join(tmpdir(), 'council-store-'));
  const stores = [];
  const open = () => { const store = new CouncilStore(directory); stores.push(store); return store; };
  t.after(() => { for (const store of stores) { try { store.close(); } catch {} } rmSync(directory, { recursive: true, force: true }); });
  return { directory, open };
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
