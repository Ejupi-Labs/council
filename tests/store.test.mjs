import assert from 'node:assert/strict';
import { chmodSync, existsSync, mkdtempSync, readFileSync, readdirSync, realpathSync, rmSync, statSync, writeFileSync } from 'node:fs';
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
  const directory = mkdtempSync(join(realpathSync.native(tmpdir()), 'council-store-'));
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
  first.saveRun({ ...record('latest', '2026-02-01T00:00:00.000Z'), status: 'running' });
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
  assert.throws(() => store.sendMessage({ from: 'codex', to: 'junie', subject: 'too large', body: 'x'.repeat(4 * 1024 * 1024 + 1) }), /4194304/);
});

test('peer handoffs and project notes retain complete evidence beyond former text caps across reopen', t => {
  const { open } = fixture(t), store = open();
  const body = 'Peer evidence. '.repeat(9000) + 'PEER_HANDOFF_TAIL';
  const value = 'Project evidence. '.repeat(9000) + 'PROJECT_NOTE_TAIL';
  store.sendMessage({ from: 'codex', to: 'junie', subject: 'Complete evidence', body });
  store.putContext('full-evidence', value, 'PM');
  const reopened = open();
  assert.equal(reopened.inboxPage('junie')[0].body, body);
  assert.equal(reopened.getContext('full-evidence').value, value);
  assert.throws(() => store.putContext('too-large', 'x'.repeat(4 * 1024 * 1024 + 1), 'PM'), /4194304/);
  assert.equal(store.getContext('too-large'), undefined);
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
  const [renewed] = two.reserve('codex', ['SRC/**'], 120000, 'run');
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
  if (process.platform !== 'win32') chmodSync(join(directory, 'state.db'), 0o640);
  const bytes = readFileSync(join(directory, 'state.db'));
  const mode = statSync(join(directory, 'state.db')).mode;
  const files = readdirSync(directory).sort();
  assert.throws(() => new CouncilStore(directory), /newer than supported/);
  assert.deepEqual(readFileSync(join(directory, 'state.db')), bytes, 'Rejecting a newer schema must not change persistent journal configuration.');
  assert.equal(statSync(join(directory, 'state.db')).mode, mode, 'Rejecting a newer schema must not tighten permissions on unsupported state.');
  assert.deepEqual(readdirSync(directory).sort(), files);
  const check = new DatabaseSync(join(directory, 'state.db'));
  assert.equal(check.prepare('PRAGMA user_version').get().user_version, 99);
  assert.equal(check.prepare('PRAGMA journal_mode').get().journal_mode, 'delete');
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
  assert.equal(check.prepare('PRAGMA user_version').get().user_version, 5);
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
    store.acquireSlot('child-owner','child-run',1,'child-pending-call');
    store.reserve('child-maintenance',['.council-maintenance/child-run'],60000,'child-run','child-owner');
    store.reserve('child-user',['user-reservation.ts'],60000,'child-run');
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
  assert.equal(store.leases().length, 2, 'A live owner keeps both maintenance and ordinary leases.');
  assert.equal(store.acquireSlot('surviving-owner', survivor.id, 1, 'surviving-pending-call'), undefined);
  assert.equal(store.health().counts.slotWaiters, 2);
  const closed = once(child, 'close');
  child.kill('SIGKILL');
  await closed;
  const recovered = store.recoverOrphans();
  assert.deepEqual(recovered.map(run => run.id), ['child-run']);
  assert.equal(store.getRun('child-run').status, 'failed');
  assert.equal(store.getRun('child-run').output, 'partial child output');
  assert.deepEqual(store.leases().map(lease => lease.owner), ['child-user'], 'Only instance-owned maintenance leases are recovered; user leases retain their TTL.');
  assert.match(store.getRun('child-run').error, /no task was retried/);
  assert.equal(store.getRun('unowned-run').status, 'running', 'An unowned run has no proven-dead PID');
  assert.equal(store.health().counts.slotWaiters, 1, 'Dead owner waiters must be removed without losing surviving queue positions.');
  assert.ok(store.acquireSlot('surviving-owner', survivor.id, 1, 'surviving-pending-call'));
  assert.deepEqual(store.recoverOrphans(), [], 'Recovery is idempotent');
  assert.throws(() => store.claimRun('child-run', 'surviving-owner'), /terminal state/);
  assert.ok(store.events('child-run').some(event => event.type === 'run.failed' && event.data.reason === 'owner_exited'));
});

test('schema version two migrates summaries and legacy cancellation without losing full records', t => {
  const { directory, open } = fixture(t);
  const old = new DatabaseSync(join(directory, 'state.db'));
  old.exec(`CREATE TABLE runs(id TEXT PRIMARY KEY, updated_at TEXT NOT NULL, document TEXT NOT NULL, owner_instance TEXT, owner_pid INTEGER, cancel_requested INTEGER NOT NULL DEFAULT 0);
    CREATE TABLE events(sequence INTEGER PRIMARY KEY AUTOINCREMENT, run_id TEXT NOT NULL, type TEXT NOT NULL, data TEXT NOT NULL, created_at TEXT NOT NULL); PRAGMA user_version = 2;`);
  const run = { ...record('migrated-v2'), status: 'running', output: 'x'.repeat(20000) };
  old.prepare('INSERT INTO runs(id,updated_at,document) VALUES(?,?,?)').run(run.id, run.updatedAt, JSON.stringify(run));
  old.prepare('INSERT INTO events(run_id,type,data,created_at) VALUES(?,?,?,?)').run(run.id, 'cancel_requested', '{}', run.updatedAt);
  old.close();
  const store = open();
  const summary = store.getRunSummary(run.id);
  assert.equal(summary.status, 'running');
  assert.equal(summary.output.length, 16000);
  assert.equal(summary.outputTruncated, true);
  assert.equal(summary.revision, 1);
  assert.equal(summary.cancellationRequested, true);
  assert.equal(store.isCancellationRequested(run.id), true);
  assert.equal(store.getRun(run.id).output.length, 20000);
});

test('atomic owned creation includes initial events and rolls everything back on a failed event write', t => {
  const store = fixture(t).open();
  store.registerInstance('atomic-owner', process.pid);
  const run = { ...record('atomic-run'), status: 'queued' };
  store.createRun(run, 'atomic-owner', [{ type: 'run.continued', data: { previousId: 'previous' } }]);
  assert.deepEqual(store.events(run.id).map(event => event.type), ['queued', 'run.continued']);
  assert.equal(store.getRunSummary(run.id).revision, 1);
  assert.throws(() => store.createRun(run, 'atomic-owner'), /already exists/);
  const write = store.writeEvent;
  store.writeEvent = () => { throw new Error('Injected event failure'); };
  try { assert.throws(() => store.createRun({ ...run, id: 'failed-creation' }, 'atomic-owner'), /Injected event failure/); }
  finally { store.writeEvent = write; }
  assert.equal(store.getRun('failed-creation'), undefined);
  assert.deepEqual(store.events('failed-creation'), []);
});

test('the workspace active-run cap is atomic and terminal runs do not consume admission capacity', t => {
  const store = fixture(t).open();
  store.registerInstance('admission-owner', process.pid);
  for (let index = 0; index < 100; index++) store.createRun({ ...record(`active-${index}`), status: 'queued' }, 'admission-owner');
  assert.throws(() => store.createRun({ ...record('overflow-active'), status: 'queued' }, 'admission-owner'), /100 active Council runs/);
  store.finalizeRun({ ...store.getRun('active-0'), status: 'completed' });
  store.createRun({ ...record('replacement-active'), status: 'queued' }, 'admission-owner');
  assert.equal(store.health().counts.activeRuns, 100);
  assert.equal(store.health().workspaceActiveRunLimit, 100);
});

test('same-PID service connections cannot overwrite each other and terminal results remain immutable', t => {
  const { open } = fixture(t);
  const owner = open();
  const other = open();
  owner.registerInstance('exact-owner', process.pid);
  other.registerInstance('other-owner', process.pid);
  const run = { ...record('fenced-run'), status: 'queued' };
  owner.createRun(run, 'exact-owner');
  assert.throws(() => other.saveRun({ ...run, status: 'running' }), /not bound/);
  assert.throws(() => other.finalizeRun({ ...run, status: 'completed' }), /not bound/);
  owner.saveRun({ ...run, status: 'running' });
  assert.equal(owner.getRunSummary(run.id).revision, 2);
  const completed = owner.finalizeRun({ ...run, status: 'completed', output: 'final evidence' });
  assert.equal(owner.getRunSummary(run.id).revision, 3);
  assert.throws(() => owner.saveRun({ ...run, status: 'running' }), /immutable/);
  assert.deepEqual(owner.finalizeRun({ ...run, status: 'failed', output: 'stale' }), completed);
});

test('run summaries support stable keyset pagination when timestamps are equal', t => {
  const store = fixture(t).open();
  for (const id of ['a', 'b', 'c', 'd', 'e']) store.saveRun(record(id, '2026-01-01T00:00:00.000Z'));
  const first = store.listRunSummaries(2);
  assert.deepEqual(first.map(run => run.id), ['e', 'd']);
  const second = store.listRunSummaries(2, { updatedAt: first.at(-1).updatedAt, id: first.at(-1).id });
  const third = store.listRunSummaries(2, { updatedAt: second.at(-1).updatedAt, id: second.at(-1).id });
  assert.deepEqual([...first, ...second, ...third].map(run => run.id), ['e', 'd', 'c', 'b', 'a']);
  assert.throws(() => store.listRunSummaries(101), /limit/);
});

test('compact event pages omit briefs, bound nested payloads and preserve the raw API', t => {
  const store = fixture(t).open();
  const run = { ...record('events-v3'), status: 'running' };
  store.saveRun(run);
  store.appendEvent(run.id, 'run.started', { input: { ...run.input, task: 'private brief', context: 'x'.repeat(64000), assignments: { codex: 'private task' } } });
  store.appendEvent(run.id, 'step.finished', { result: { agent: 'codex', status: 'completed', durationMs: 1, output: 'o'.repeat(200000), error: 'e'.repeat(20000) } });
  store.appendEvent(run.id, 'custom', { huge: Array.from({ length: 1000 }, () => ({ text: 'z'.repeat(1000) })) });
  const first = store.compactEvents(run.id, 0, 2);
  assert.equal(first.length, 2);
  assert.equal(JSON.stringify(first).includes('private brief'), false);
  assert.equal(JSON.stringify(first).includes('private task'), false);
  assert.equal(first[1].data.result.output, undefined);
  assert.ok(first[1].data.result.error.length <= 1000);
  const next = store.compactEvents(run.id, first.at(-1).sequence, 2);
  assert.equal(next.length, 1);
  assert.ok(Buffer.byteLength(JSON.stringify(next[0].data)) <= 16 * 1024);
  assert.equal(store.events(run.id)[1].data.result.output.length, 200000);
  store.appendEvent('future-run', 'cancel_requested', {});
  assert.equal(store.isCancellationRequested('future-run'), true);
});

test('health and integrity diagnostics expose counts without returning run payloads', t => {
  const store = fixture(t).open();
  store.saveRun(record('health-run'));
  store.appendEvent('health-run', 'completed', {});
  store.sendMessage({ from: 'codex', to: 'claude', subject: 'Review', body: 'A bounded note' });
  store.putContext('api', 'Small contract', 'codex');
  const health = store.health();
  assert.equal(health.schemaVersion, 5);
  assert.equal(health.sqliteVersion, process.versions.sqlite);
  assert.equal(health.journalMode, 'wal');
  assert.equal(health.counts.runs, 1);
  assert.equal(health.counts.events, 1);
  assert.equal(health.counts.unreadMessages, 1);
  assert.equal(health.counts.context, 1);
  assert.equal(health.runStatuses.completed, 1);
  assert.ok(health.databaseBytes > 0 && health.walBytes >= 0);
  assert.ok(health.preparedStatements <= 96);
  assert.deepEqual(store.checkIntegrity(), ['ok']);
});

test('online backups include committed WAL data and never overwrite existing or racing destinations', async t => {
  const { directory, open } = fixture(t);
  const store = open();
  store.saveRun(record('backup-run'));
  const target = join(directory, 'snapshot café.db');
  assert.equal(await store.backupTo(target), target);
  const snapshot = new DatabaseSync(target);
  assert.equal(snapshot.prepare('SELECT status FROM runs WHERE id = ?').get('backup-run').status, 'completed');
  assert.equal(snapshot.prepare('PRAGMA quick_check').get().quick_check, 'ok');
  snapshot.close();
  const protectedPath = join(directory, 'existing.txt');
  writeFileSync(protectedPath, 'preserve this file');
  await assert.rejects(store.backupTo(protectedPath), /already exists/);
  assert.equal(readFileSync(protectedPath, 'utf8'), 'preserve this file');
  const racing = join(directory, 'racing.db');
  const attempts = await Promise.allSettled([store.backupTo(racing), store.backupTo(racing)]);
  assert.equal(attempts.filter(result => result.status === 'fulfilled').length, 1);
  assert.equal(attempts.filter(result => result.status === 'rejected').length, 1);
  assert.ok(readdirSync(directory).every(name => !name.startsWith('.council-backup-')));
  await assert.rejects(store.backupTo(join(directory, 'state.db')), /active Council database/);
  await assert.rejects(store.backupTo(join(directory, 'state.db-wal')), /journal files/);
});

test('backup cancellation rejects cleanly, removes staging files and keeps the database usable', async t => {
  const { directory, open } = fixture(t);
  const store = open();
  store.saveRun({ ...record('large-backup-run'), output: 'x'.repeat(2 * 1024 * 1024) });
  const controller = new AbortController();
  const target = join(directory, 'cancelled.db');
  const operation = store.backupTo(target, controller.signal);
  controller.abort(new Error('Expected backup cancellation'));
  await assert.rejects(operation, /Expected backup cancellation/);
  assert.ok(readdirSync(directory).every(name => !name.startsWith('.council-backup-')));
  assert.throws(() => readFileSync(target), /ENOENT/);
  assert.deepEqual(store.checkIntegrity(), ['ok']);
  assert.equal(store.getRun('large-backup-run').output.length, 2 * 1024 * 1024);
});

test('manual pruning defaults to preview and protects active runs, retained artifacts, messages and notes', t => {
  const store = fixture(t).open();
  store.saveRun(record('old-a', '2020-01-01T00:00:00.000Z'));
  store.saveRun(record('old-b', '2021-01-01T00:00:00.000Z'));
  store.saveRun({ ...record('retained', '2022-01-01T00:00:00.000Z'), workspaces: [{ agent: 'codex', path: '/retained/worktree', patchPath: '/retained/patch', changed: false }] });
  store.saveRun(record('latest', '2024-01-01T00:00:00.000Z'));
  store.saveRun({ ...record('active', '2019-01-01T00:00:00.000Z'), status: 'running' });
  for (const id of ['old-a', 'old-b', 'retained', 'active']) store.appendEvent(id, 'evidence', { id });
  store.sendMessage({ from: 'codex', to: 'claude', subject: 'Retain', body: 'Unread evidence' });
  store.putContext('decision', 'Retain this note', 'codex');
  const options = { before: '2025-01-01T00:00:00Z', keepLatest: 1 };
  const preview = store.prune(options);
  assert.deepEqual(preview.candidateIds, ['old-a', 'old-b']);
  assert.equal(preview.skippedArtifacts, 1);
  assert.equal(preview.deletedRuns, 0);
  assert.ok(store.getRun('old-a'));
  const executed = store.prune({ ...options, dryRun: false });
  assert.equal(executed.deletedRuns, 2);
  assert.equal(executed.deletedEvents, 2);
  assert.equal(store.getRun('old-a'), undefined);
  for (const id of ['retained', 'active', 'latest']) assert.ok(store.getRun(id));
  assert.equal(store.events('active').length, 1);
  assert.equal(store.inbox('claude').length, 1);
  assert.equal(store.getContext('decision').value, 'Retain this note');
  assert.throws(() => store.prune({ before: '2025-02-30T00:00:00Z' }), /valid ISO/);
  assert.throws(() => store.prune({ before: '2025-01-01' }), /valid ISO/);
  assert.throws(() => store.prune({ ...options, keepLatest: -1 }), /keepLatest/);
});

test('manual pruning batches are bounded even when a large terminal history is eligible', t => {
  const store = fixture(t).open();
  for (let index = 0; index < 1005; index++) store.saveRun(record(`history-${String(index).padStart(4, '0')}`, '2020-01-01T00:00:00.000Z'));
  const result = store.prune({ before: '2025-01-01T00:00:00Z', keepLatest: 0, dryRun: false });
  assert.equal(result.candidateIds.length, 1000);
  assert.equal(result.deletedRuns, 1000);
  assert.equal(store.health().counts.runs, 5);
});

test('a contended write fails promptly instead of blocking the event loop for five seconds', t => {
  const { directory, open } = fixture(t);
  const store = open();
  const blocker = new DatabaseSync(join(directory, 'state.db'));
  blocker.exec('BEGIN IMMEDIATE');
  try {
    const start = performance.now();
    assert.throws(() => store.appendEvent('blocked', 'evidence', {}), /locked|busy/i);
    assert.ok(performance.now() - start < 1000, 'SQLite contention must be bounded below one second.');
  } finally { blocker.exec('ROLLBACK'); blocker.close(); }
  assert.ok(store.appendEvent('blocked', 'recovered', {}));
});

test('an automatically rolled-back transaction preserves its original failure diagnostic', t => {
  const store = fixture(t).open();
  const original = new Error('Injected original persistence failure');
  assert.throws(() => store.transaction(() => {
    // This reproduces the autocommit state SQLite enters after selected I/O/full errors.
    store.db.exec('ROLLBACK');
    throw original;
  }), error => error === original);
  assert.ok(store.appendEvent('post-rollback', 'usable', {}));
});

test('a live version-two owner blocks migration before ALTER and migration succeeds after that owner exits', { timeout: 15000 }, async t => {
  const setup = fixture(t);
  const code = `import {DatabaseSync} from 'node:sqlite';import{join}from'node:path';
    const db=new DatabaseSync(join(process.env.COUNCIL_TEST_STATE,'state.db'));
    db.exec("PRAGMA journal_mode=WAL;CREATE TABLE runs(id TEXT PRIMARY KEY,updated_at TEXT NOT NULL,document TEXT NOT NULL,owner_instance TEXT,owner_pid INTEGER,cancel_requested INTEGER NOT NULL DEFAULT 0);CREATE TABLE events(sequence INTEGER PRIMARY KEY AUTOINCREMENT,run_id TEXT NOT NULL,type TEXT NOT NULL,data TEXT NOT NULL,created_at TEXT NOT NULL);CREATE TABLE instances(id TEXT PRIMARY KEY,pid INTEGER NOT NULL,registered_at INTEGER NOT NULL,heartbeat_at INTEGER NOT NULL,slot_limit INTEGER);PRAGMA user_version=2;");
    const now=new Date().toISOString();const run={id:'old-live-run',status:'queued',createdAt:now,updatedAt:now,input:{mode:'delegate',task:'offline upgrade fixture',agents:['codex'],permission:'read'},steps:[]};
    db.prepare('INSERT INTO instances VALUES(?,?,?,?,?)').run('older-owner',process.pid,Date.now(),Date.now(),1);
    db.prepare('INSERT INTO runs VALUES(?,?,?,?,?,?)').run(run.id,now,JSON.stringify(run),'older-owner',process.pid,0);
    console.log('ready');setInterval(()=>{},1000);`;
  const child = spawn(process.execPath, ['--input-type=module', '-e', code], { env: { ...process.env, COUNCIL_TEST_STATE: setup.directory }, windowsHide: true, stdio: ['ignore', 'pipe', 'pipe'] });
  setup.cleanup(async () => { if (child.exitCode === null && child.signalCode === null) { const closed = once(child, 'close'); child.kill('SIGKILL'); await closed; } });
  let diagnostics = '';
  child.stderr.on('data', chunk => { diagnostics += chunk; });
  await new Promise((ready, reject) => {
    let output = '';
    child.stdout.on('data', chunk => { output += chunk; if (output.includes('ready\n')) ready(); });
    child.once('error', reject);
    child.once('exit', () => reject(new Error(`Legacy owner exited before readiness: ${diagnostics}`)));
  });
  assert.throws(() => new CouncilStore(setup.directory), /Stop older Council hosts/);
  const inspection = new DatabaseSync(join(setup.directory, 'state.db'));
  assert.equal(inspection.prepare('PRAGMA user_version').get().user_version, 2);
  assert.equal(inspection.prepare('PRAGMA table_info(runs)').all().some(column => column.name === 'status'), false);
  inspection.close();
  const closed = once(child, 'close'); child.kill('SIGKILL'); await closed;
  const migrated = setup.open();
  assert.equal(migrated.health().schemaVersion, 5);
  assert.equal(migrated.getRunSummary('old-live-run').status, 'queued');
  assert.deepEqual(migrated.recoverOrphans().map(run => run.id), ['old-live-run']);
});

test('unsafe bundled SQLite versions fail before creating directories or touching existing state', t => {
  const { directory, open } = fixture(t);
  const healthy = open();
  healthy.saveRun(record('runtime-guard-evidence'));
  const snapshot = () => Object.fromEntries(readdirSync(directory).map(name => [name, readFileSync(join(directory, name))]));
  const originalFiles = snapshot();
  const descriptor = Object.getOwnPropertyDescriptor(process.versions, 'sqlite');
  assert.equal(descriptor?.configurable, true, 'This Node test runtime must permit restoring the SQLite version descriptor.');
  try {
    for (const version of ['3.51.2', '3.50.7', '3.44.6', '3.9.99', 'unknown']) {
      Object.defineProperty(process.versions, 'sqlite', { ...descriptor, value: version });
      const unopened = join(directory, 'must-not-be-created');
      assert.throws(() => new CouncilStore(unopened), /requires SQLite 3\.51\.3.*Upgrade Node\.js/);
      assert.equal(existsSync(unopened), false);
      assert.throws(() => new CouncilStore(directory), /requires SQLite 3\.51\.3/);
      assert.deepEqual(snapshot(), originalFiles, 'Rejected runtimes must not mutate any existing SQLite/WAL/SHM file.');
    }
  } finally { Object.defineProperty(process.versions, 'sqlite', descriptor); }
  assert.equal(healthy.health().sqliteVersion, descriptor.value);
  assert.equal(healthy.getRun('runtime-guard-evidence').status, 'completed');
});

function pendingArtifact() {
  return { agent: 'codex', path: '/managed/worktrees/repair-run/codex', patchPath: '/managed/patches/repair-run/codex.patch', baseCommit: 'a'.repeat(40), changed: false, exportStatus: 'pending', exportError: 'Export was interrupted.' };
}

test('explicit artifact refresh from a new host preserves the terminal outcome and updates summary and event atomically', t => {
  const { open } = fixture(t);
  const original = open();
  original.registerInstance('original-export-owner', process.pid);
  const artifact = pendingArtifact();
  const run = { ...record('repair-run', '2020-01-01T00:00:00.000Z'), status: 'queued', workspaces: [artifact] };
  original.createRun(run, 'original-export-owner');
  const failed = original.finalizeRun({ ...run, status: 'failed', output: 'Partial implementation evidence.', error: 'The provider failed before completion.', steps: [{ id: 'failed-step', agent: 'codex', role: 'worker', result: { agent: 'codex', status: 'failed', output: 'Partial code evidence.', durationMs: 25 } }] });
  original.unregisterInstance('original-export-owner');
  const repair = open();
  repair.registerInstance('repair-maintenance-owner', process.pid);
  const before = repair.getRunSummary(run.id);
  const refreshed = repair.updateArtifact(run.id, { ...artifact, changed: true, exportStatus: 'ready', exportError: undefined, status: 'completed', output: 'Must not overwrite the outcome.' });
  for (const field of ['status', 'input', 'steps', 'output', 'error', 'createdAt']) assert.deepEqual(refreshed[field], failed[field]);
  for (const field of ['agent', 'path', 'patchPath', 'baseCommit']) assert.equal(refreshed.workspaces[0][field], artifact[field]);
  assert.equal(refreshed.workspaces[0].changed, true);
  assert.equal(refreshed.workspaces[0].exportStatus, 'ready');
  assert.equal(Object.hasOwn(refreshed.workspaces[0], 'exportError'), false);
  const summary = repair.getRunSummary(run.id);
  assert.equal(summary.revision, before.revision + 1);
  assert.equal(summary.status, 'failed');
  assert.equal(summary.workspaces[0].exportStatus, 'ready');
  assert.equal(summary.updatedAt, refreshed.updatedAt);
  const events = repair.events(run.id).filter(event => event.type === 'workspace.refreshed');
  assert.equal(events.length, 1);
  assert.equal(events[0].data.path, artifact.path);
  assert.equal(events[0].data.exportStatus, 'ready');
  assert.throws(() => repair.saveRun({ ...refreshed, status: 'running' }), /not bound|immutable/);
  assert.throws(() => repair.finalizeRun({ ...refreshed, status: 'completed' }), /not bound/);
});

test('artifact refresh rejects active runs and every identity redirect without changing metadata', t => {
  const store = fixture(t).open();
  const artifact = pendingArtifact();
  store.saveRun({ ...record('active-repair'), status: 'running', workspaces: [artifact] });
  assert.throws(() => store.updateArtifact('active-repair', { ...artifact, exportStatus: 'ready' }), /run is active/);
  store.saveRun({ ...record('terminal-repair'), status: 'cancelled', workspaces: [artifact] });
  const before = store.getRunSummary('terminal-repair');
  for (const changed of [{ agent: 'claude' }, { path: '/redirected/worktree' }, { patchPath: '/redirected/patch' }, { baseCommit: 'b'.repeat(40) }]) {
    assert.throws(() => store.updateArtifact('terminal-repair', { ...artifact, ...changed, exportStatus: 'ready' }), /identity must match/);
  }
  assert.deepEqual(store.getRunSummary('terminal-repair'), before);
  assert.deepEqual(store.events('terminal-repair'), []);
});

test('a refresh event failure rolls back artifact metadata instead of leaving an unlogged repair', t => {
  const store = fixture(t).open();
  const artifact = pendingArtifact();
  const run = { ...record('repair-rollback'), status: 'failed', workspaces: [artifact], error: 'Original failure.' };
  store.saveRun(run);
  const before = store.getRunSummary(run.id);
  const writeEvent = store.writeEvent;
  store.writeEvent = () => { throw new Error('Injected refresh event failure'); };
  try { assert.throws(() => store.updateArtifact(run.id, { ...artifact, changed: true, exportStatus: 'ready' }), /Injected refresh event failure/); }
  finally { store.writeEvent = writeEvent; }
  assert.deepEqual(store.getRun(run.id), run);
  assert.deepEqual(store.getRunSummary(run.id), before);
  assert.deepEqual(store.events(run.id), []);
});

test('concurrent runs using one agent label cannot renew or release each other scoped leases', t => {
  const { open } = fixture(t);
  const first = open();
  const second = open();
  const [lease] = first.reserve('codex', ['src/**'], 60000, 'run-a');
  assert.throws(() => second.reserve('codex', ['src/main.ts'], 60000, 'run-b'), /conflict/);
  assert.equal(first.leases()[0].runId, 'run-a');
  assert.equal(first.reserve('codex', ['src/**'], 120000, 'run-a')[0].id, lease.id);
  second.reserve('codex', ['docs/readme.md'], 60000, 'run-b');
  assert.equal(second.release('codex', ['src/**', 'docs/readme.md'], 'run-b'), 1);
  assert.deepEqual(first.leases().map(value => value.runId), ['run-a']);
  second.reserve('codex', ['docs/readme.md'], 60000, 'run-b');
  assert.equal(second.release('codex', undefined, 'run-b'), 1);
  assert.equal(first.leases()[0].id, lease.id);
  assert.equal(first.release('codex', undefined, 'run-a'), 1);
});

test('scoped and unscoped renewals conflict while legacy unscoped ownership remains compatible', t => {
  const store = fixture(t).open();
  const [legacy] = store.reserve('claude', ['shared/file.ts'], 60000);
  assert.throws(() => store.reserve('claude', ['shared/file.ts'], 60000, 'scoped-run'), /conflict/);
  assert.equal(store.release('claude', undefined, 'scoped-run'), 0);
  assert.equal(store.reserve('claude', ['shared/file.ts'], 120000)[0].id, legacy.id);
  assert.equal(store.release('claude'), 1);
  store.reserve('claude', ['shared/file.ts'], 60000, 'scoped-run');
  assert.throws(() => store.reserve('claude', ['shared/**'], 60000), /conflict/);
  assert.equal(store.release('claude'), 1, 'Omitting runId deliberately preserves legacy release behavior.');
});

test('an idle low-limit host constrains new dispatch while existing slots drain and unregistering relaxes the limit', t => {
  const { open } = fixture(t);
  const high = open();
  const idle = open();
  high.registerInstance('high-limit', process.pid, 3);
  const run = { ...record('limit-drain'), status: 'queued' };
  high.createRun(run, 'high-limit');
  const first = high.acquireSlot('high-limit', run.id, 3);
  const second = high.acquireSlot('high-limit', run.id, 3);
  assert.ok(first && second);
  idle.registerInstance('idle-low-limit', process.pid, 1);
  idle.registerInstance('idle-low-limit', process.pid);
  assert.equal(high.acquireSlot('high-limit', run.id, 3), undefined);
  high.releaseSlot(first, 'high-limit');
  assert.equal(high.acquireSlot('high-limit', run.id, 3), undefined, 'One existing slot still fills the reduced limit.');
  high.releaseSlot(second, 'high-limit');
  const only = high.acquireSlot('high-limit', run.id, 3);
  assert.ok(only);
  assert.equal(high.acquireSlot('high-limit', run.id, 3), undefined);
  high.releaseSlot(only, 'high-limit');
  idle.unregisterInstance('idle-low-limit');
  const relaxed = Array.from({ length: 3 }, () => high.acquireSlot('high-limit', run.id, 3));
  assert.ok(relaxed.every(Boolean));
  assert.equal(high.acquireSlot('high-limit', run.id, 3), undefined);
  for (const token of relaxed) high.releaseSlot(token, 'high-limit');
  for (const invalid of [0, 33, NaN]) assert.throws(() => idle.registerInstance('invalid-limit', process.pid, invalid), /limit/);
  assert.equal(idle.heartbeat('invalid-limit'), false);
});

test('retention preview reads a consistent WAL snapshot without taking a reserved writer lock', t => {
  const { directory, open } = fixture(t);
  const store = open();
  store.saveRun(record('preview-old', '2020-01-01T00:00:00.000Z'));
  store.appendEvent('preview-old', 'evidence', {});
  const writer = new DatabaseSync(join(directory, 'state.db'));
  writer.exec('BEGIN IMMEDIATE');
  try {
    const preview = store.prune({ before: '2025-01-01T00:00:00Z', keepLatest: 0 });
    assert.deepEqual(preview.candidateIds, ['preview-old']);
    assert.equal(preview.deletedRuns, 0);
    assert.equal(store.events('preview-old').length, 1);
    assert.throws(() => store.prune({ before: '2025-01-01T00:00:00Z', keepLatest: 0, dryRun: false }), /locked|busy/i);
  } finally { writer.exec('ROLLBACK'); writer.close(); }
  assert.ok(store.getRun('preview-old'));
});

test('health counts remain coherent when another writer commits between metadata queries', t => {
  const { open } = fixture(t);
  const reader = open();
  const writer = open();
  reader.saveRun(record('before-health'));
  const statement = reader.statement.bind(reader);
  let injected = false;
  reader.statement = sql => {
    if (!injected && sql === 'SELECT status, COUNT(*) AS count FROM runs GROUP BY status') {
      writer.saveRun(record('during-health'));
      injected = true;
    }
    return statement(sql);
  };
  const snapshot = reader.health();
  assert.equal(injected, true);
  assert.ok(writer.getRun('during-health'), 'The read snapshot must not block the concurrent WAL writer.');
  assert.equal(snapshot.counts.runs, 1);
  assert.equal(Object.values(snapshot.runStatuses).reduce((sum, count) => sum + count, 0), snapshot.counts.runs);
  assert.equal(reader.health().counts.runs, 2);
});

test('schema version three upgrades queue metadata without rebuilding existing run summaries', t => {
  const { directory, open } = fixture(t);
  const original = open();
  const run = record('v3-preserved', '2020-01-01T00:00:00.000Z');
  original.saveRun(run);
  const [lease] = original.reserve('legacy-user', ['legacy/file.ts'], 60000, run.id);
  const summary = original.getRunSummary(run.id);
  original.close();
  const old = new DatabaseSync(join(directory, 'state.db'));
  old.exec('DROP TABLE slot_waiters; DROP INDEX leases_instance; ALTER TABLE leases DROP COLUMN instance_id; PRAGMA user_version = 3;');
  old.close();
  const migrate = CouncilStore.prototype.migrateMetadata;
  CouncilStore.prototype.migrateMetadata = () => { throw new Error('Version three metadata must not be rebuilt.'); };
  let upgraded;
  try { upgraded = open(); } finally { CouncilStore.prototype.migrateMetadata = migrate; }
  assert.deepEqual(upgraded.getRun(run.id), run);
  assert.deepEqual(upgraded.getRunSummary(run.id), summary);
  assert.deepEqual(upgraded.leases(), [lease]);
  assert.equal(upgraded.health().schemaVersion, 5);
  assert.equal(upgraded.health().counts.slotWaiters, 0);
});

test('a live version three host prevents queue migration before any schema mutation', t => {
  const { directory, open } = fixture(t);
  const old = open();
  old.registerInstance('v3-live-host', process.pid, 1);
  old.close();
  const raw = new DatabaseSync(join(directory, 'state.db'));
  raw.exec('DROP TABLE slot_waiters; DROP INDEX leases_instance; ALTER TABLE leases DROP COLUMN instance_id; PRAGMA user_version = 3;');
  assert.throws(() => new CouncilStore(directory), /Stop older Council hosts/);
  assert.equal(raw.prepare('PRAGMA user_version').get().user_version, 3);
  assert.equal(raw.prepare("SELECT 1 FROM sqlite_master WHERE name = 'slot_waiters'").get(), undefined);
  raw.prepare('DELETE FROM instances WHERE id = ?').run('v3-live-host');
  raw.close();
  assert.equal(open().health().schemaVersion, 5);
});

test('independent process waiters receive the next slot before an incumbent can start another call', { timeout: 15000 }, async t => {
  const setup = fixture(t);
  const incumbent = setup.open();
  incumbent.registerInstance('fifo-incumbent', process.pid, 1);
  const run = { ...record('fifo-incumbent-run'), status: 'queued' };
  incumbent.createRun(run, 'fifo-incumbent');
  const held = incumbent.acquireSlot('fifo-incumbent', run.id, 1);
  const moduleUrl = pathToFileURL(resolve('dist/store.js')).href;
  const code = `import { CouncilStore } from ${JSON.stringify(moduleUrl)};
    const store = new CouncilStore(process.env.COUNCIL_TEST_STATE);
    store.registerInstance('fifo-foreign', process.pid, 1);
    const now = new Date().toISOString();
    const run = {id:'fifo-foreign-run',status:'queued',createdAt:now,updatedAt:now,input:{mode:'delegate',task:'offline FIFO test',agents:['codex'],permission:'read'},steps:[]};
    store.createRun(run,'fifo-foreign');
    if (store.acquireSlot('fifo-foreign',run.id,1,'foreign-call') !== undefined) throw new Error('Incumbent slot must still be held.');
    process.send({type:'queued'});
    let token;
    process.on('message', message => {
      if (message.type === 'acquire') {
        token = store.acquireSlot('fifo-foreign',run.id,1,'foreign-call');
        if (!token) throw new Error('Foreign waiter lost its FIFO position.');
        process.send({type:'acquired'});
      } else if (message.type === 'finish') {
        store.releaseSlot(token,'fifo-foreign');
        store.finalizeRun({...run,status:'completed'});
        store.unregisterInstance('fifo-foreign');
        store.close();process.disconnect();
      }
    });`;
  const child = spawn(process.execPath, ['--input-type=module', '-e', code], {
    env: { ...process.env, COUNCIL_TEST_STATE: setup.directory }, windowsHide: true, stdio: ['ignore', 'ignore', 'pipe', 'ipc'],
  });
  setup.cleanup(async () => {
    if (child.exitCode === null && child.signalCode === null) { const closed = once(child, 'close'); child.kill('SIGKILL'); await closed; }
  });
  let diagnostics = '';
  child.stderr.on('data', chunk => { diagnostics += chunk; });
  const message = type => new Promise((resolveMessage, reject) => {
    const receive = value => { if (value.type === type) { cleanup(); resolveMessage(value); } };
    const exit = () => { cleanup(); reject(new Error(`FIFO child exited unexpectedly: ${diagnostics}`)); };
    const cleanup = () => { child.off('message', receive); child.off('exit', exit); child.off('error', reject); };
    child.on('message', receive); child.once('exit', exit); child.once('error', reject);
  });
  await message('queued');
  assert.equal(incumbent.health().counts.slotWaiters, 1);
  incumbent.releaseSlot(held, 'fifo-incumbent');
  assert.equal(incumbent.acquireSlot('fifo-incumbent', run.id, 1), undefined, 'Legacy try-acquire cannot jump a durable waiter.');
  for (let index = 0; index < 5; index++) assert.equal(incumbent.acquireSlot('fifo-incumbent', run.id, 1, 'next-incumbent-call'), undefined);
  assert.equal(incumbent.health().counts.slotWaiters, 2, 'Retries reuse their original queue position.');
  const acquired = message('acquired');
  child.send({ type: 'acquire' });
  await acquired;
  assert.equal(incumbent.acquireSlot('fifo-incumbent', run.id, 1, 'next-incumbent-call'), undefined);
  const closed = once(child, 'close');
  child.send({ type: 'finish' });
  await closed;
  assert.equal(child.exitCode, 0, diagnostics);
  const next = incumbent.acquireSlot('fifo-incumbent', run.id, 1, 'next-incumbent-call');
  assert.ok(next);
  assert.equal(incumbent.health().counts.slotWaiters, 0);
  incumbent.releaseSlot(next, 'fifo-incumbent');
});

test('aborted, cancelled and terminal waits leave FIFO capacity available to following calls', t => {
  const { open } = fixture(t);
  const first = open(), second = open(), third = open();
  const stores = [first, second, third];
  const runs = stores.map((store, index) => {
    store.registerInstance(`fifo-owner-${index}`, process.pid, 1);
    const run = { ...record(`fifo-run-${index}`), status: 'queued' };
    store.createRun(run, `fifo-owner-${index}`);
    return run;
  });
  const held = first.acquireSlot('fifo-owner-0', runs[0].id, 1);
  assert.equal(second.acquireSlot('fifo-owner-1', runs[1].id, 1, 'abort-call'), undefined);
  assert.equal(second.acquireSlot('fifo-owner-1', runs[1].id, 1, 'cancel-call'), undefined);
  assert.equal(third.acquireSlot('fifo-owner-2', runs[2].id, 1, 'last-call'), undefined);
  third.cancelSlotWaiter('fifo-owner-2', 'abort-call');
  assert.equal(first.health().counts.slotWaiters, 3, 'A different instance cannot cancel another instance waiter ID.');
  second.cancelSlotWaiter('fifo-owner-1', 'abort-call');
  second.cancelSlotWaiter('fifo-owner-1', 'abort-call');
  assert.equal(first.health().counts.slotWaiters, 2);
  second.requestCancellation(runs[1].id);
  assert.equal(first.health().counts.slotWaiters, 1);
  first.releaseSlot(held, 'fifo-owner-0');
  const last = third.acquireSlot('fifo-owner-2', runs[2].id, 1, 'last-call');
  assert.ok(last);
  assert.equal(first.acquireSlot('fifo-owner-0', runs[0].id, 1, 'terminal-call'), undefined);
  first.finalizeRun({ ...runs[0], status: 'failed', error: 'Offline fixture failure.' });
  assert.equal(first.health().counts.slotWaiters, 0);
  third.releaseSlot(last, 'fifo-owner-2');
});

test('slot waiter identity and capacity limits fail without disturbing existing queue positions', t => {
  const { open } = fixture(t);
  const owner = open();
  owner.registerInstance('bounded-fifo-owner', process.pid, 1);
  const run = { ...record('bounded-fifo-run'), status: 'queued' };
  const other = { ...run, id: 'other-bounded-fifo-run' };
  owner.createRun(run, 'bounded-fifo-owner');
  owner.createRun(other, 'bounded-fifo-owner');
  const held = owner.acquireSlot('bounded-fifo-owner', run.id, 1);
  for (let index = 0; index < 32; index++) assert.equal(owner.acquireSlot('bounded-fifo-owner', run.id, 1, `call-${index}`), undefined);
  assert.throws(() => owner.acquireSlot('bounded-fifo-owner', other.id, 1, 'call-0'), /cannot be reused/);
  assert.throws(() => owner.acquireSlot('bounded-fifo-owner', run.id, 1, 'call-32'), error => error.code === 'COUNCIL_SLOT_QUEUE_FULL');
  assert.equal(owner.health().counts.slotWaiters, 32);
  owner.releaseSlot(held, 'bounded-fifo-owner');
  assert.equal(owner.acquireSlot('bounded-fifo-owner', run.id, 1, 'call-31'), undefined);
  const first = owner.acquireSlot('bounded-fifo-owner', run.id, 1, 'call-0');
  assert.ok(first);
  owner.releaseSlot(first, 'bounded-fifo-owner');
  owner.finalizeRun({ ...run, status: 'completed' });
  assert.equal(owner.health().counts.slotWaiters, 0);
});

test('free slots admit the reserved waiter prefix while retaining capacity for an older sleeping caller', t => {
  const { open } = fixture(t);
  const jobs = Array.from({ length: 5 }, (_, index) => {
    const store = open();
    const owner = `prefix-owner-${index}`;
    store.registerInstance(owner, process.pid, 3);
    const run = { ...record(`prefix-run-${index}`), status: 'queued' };
    store.createRun(run, owner);
    return { store, owner, run };
  });
  const [holder, a, b, c, later] = jobs;
  const acquire = (job, id) => job.store.acquireSlot(job.owner, job.run.id, 3, id);
  const release = (job, token) => job.store.releaseSlot(token, job.owner);
  const held = Array.from({ length: 3 }, () => acquire(holder));
  for (const job of [a, b, c]) assert.equal(acquire(job, job.owner), undefined);
  for (const token of held) release(holder, token);
  const cToken = acquire(c, c.owner);
  const bToken = acquire(b, b.owner);
  assert.ok(cToken && bToken, 'B and C can use their reserved slots without waiting for A to poll.');
  assert.equal(holder.store.health().counts.slots, 2);
  assert.equal(acquire(later, 'later-d'), undefined);
  assert.equal(acquire(later, 'later-e'), undefined);
  for (let index = 0; index < 3; index++) assert.equal(acquire(later, 'later-d'), undefined);
  assert.equal(acquire(holder), undefined, 'An anonymous call cannot consume the remaining slot reserved for A.');
  assert.equal(holder.store.health().counts.slotWaiters, 3);
  const aToken = acquire(a, a.owner);
  assert.ok(aToken, 'A retains its reserved capacity even though B and C started first.');
  release(c, cToken);
  assert.equal(acquire(later, 'later-e'), undefined, 'E cannot take the one free slot reserved for older D.');
  const dToken = acquire(later, 'later-d');
  assert.ok(dToken);
  release(b, bToken);
  const eToken = acquire(later, 'later-e');
  assert.ok(eToken);
  assert.equal(holder.store.health().counts.slotWaiters, 0);
  for (const [job, token] of [[a, aToken], [later, dToken], [later, eToken]]) release(job, token);
});

test('instance-owned maintenance leases fence renewals and clear on safe unregister without deleting user reservations', t => {
  const { open } = fixture(t);
  const first = open(), second = open();
  first.registerInstance('lease-instance-a', process.pid, 1);
  second.registerInstance('lease-instance-b', process.pid, 1);
  first.reserve('user-owner', ['src/user.ts'], 60000, 'terminal-run');
  const [lease] = first.reserve('maintenance-owner', ['.council-maintenance/terminal-run'], 60000, 'terminal-run', 'lease-instance-a');
  assert.equal(lease.instanceId, 'lease-instance-a');
  assert.equal(first.reserve('maintenance-owner', lease.paths, 120000, 'terminal-run', 'lease-instance-a')[0].id, lease.id);
  assert.throws(() => second.reserve('maintenance-owner', lease.paths, 60000, 'terminal-run', 'lease-instance-b'), /conflict/);
  assert.throws(() => second.reserve('maintenance-owner', lease.paths, 60000, 'terminal-run'), /conflict/);
  assert.throws(() => second.reserve('other', ['other.ts'], 60000, undefined, 'lease-instance-a'), /not bound/);
  assert.throws(() => first.reserve('other', ['other.ts'], 60000, undefined, 'missing-instance'), /not registered/);
  first.unregisterInstance('lease-instance-a');
  assert.deepEqual(second.leases().map(value => value.owner), ['user-owner']);
  assert.equal(second.leases()[0].instanceId, undefined);
  assert.equal(second.reserve('maintenance-owner', lease.paths, 60000, 'terminal-run', 'lease-instance-b')[0].instanceId, 'lease-instance-b');
  second.unregisterInstance('lease-instance-b');
  assert.deepEqual(second.leases().map(value => value.owner), ['user-owner']);
});

test('the global waiter bound rejects excess admission and cancelled waits free capacity without reordering survivors', t => {
  const { open } = fixture(t);
  const store = open();
  const owner = 'global-queue-owner';
  store.registerInstance(owner, process.pid, 1);
  for (let index = 0; index < 26; index++) store.createRun({ ...record(`global-run-${index}`), status: 'queued' }, owner);
  const held = store.acquireSlot(owner, 'global-run-0', 1);
  const insert = store.db.prepare('INSERT INTO slot_waiters(instance_id, run_id, waiter_id) VALUES (?, ?, ?)');
  store.db.exec('BEGIN IMMEDIATE');
  try {
    for (let run = 0; run < 25; run++) for (let call = 0; call < 32; call++) insert.run(owner, `global-run-${run}`, `global-${run}-${call}`);
    store.db.exec('COMMIT');
  } catch (error) { store.db.exec('ROLLBACK'); throw error; }
  assert.equal(store.health().counts.slotWaiters, 800);
  assert.throws(() => store.acquireSlot(owner, 'global-run-25', 1, 'excess-call'), error => error.code === 'COUNCIL_SLOT_QUEUE_FULL');
  assert.equal(store.health().counts.slotWaiters, 800);
  store.cancelSlotWaiter(owner, 'global-0-0');
  assert.equal(store.acquireSlot(owner, 'global-run-25', 1, 'excess-call'), undefined);
  assert.equal(store.health().counts.slotWaiters, 800);
  store.releaseSlot(held, owner);
  assert.equal(store.acquireSlot(owner, 'global-run-25', 1, 'excess-call'), undefined);
  const oldest = store.acquireSlot(owner, 'global-run-0', 1, 'global-0-1');
  assert.ok(oldest);
  store.releaseSlot(oldest, owner);
});

test('message pages preserve complete large bodies, timestamp ties and recipient scope through acknowledgements', t => {
  const { open } = fixture(t);
  const store = open();
  const timestamp = '2026-01-01T00:00:00.000Z';
  const body = '\u0001'.repeat(32000);
  const insert = store.db.prepare('INSERT INTO messages(id,sender,recipient,created_at,document) VALUES (?,?,?,?,?)');
  store.db.exec('BEGIN IMMEDIATE');
  try {
    for (let index = 99; index >= 0; index--) {
      const message = { id: `message-${String(index).padStart(3, '0')}`, from: 'codex', to: 'claude', subject: 'Complete evidence', body, createdAt: timestamp };
      insert.run(message.id, message.from, message.to, message.createdAt, JSON.stringify(message));
      if (index % 10 === 0) insert.run(`other-${index}`, 'codex', 'junie', timestamp, JSON.stringify({ ...message, id: `other-${index}`, to: 'junie' }));
    }
    store.db.exec('COMMIT');
  } catch (error) { store.db.exec('ROLLBACK'); throw error; }
  for (let index = 0; index < 100; index += 3) store.acknowledge(`message-${String(index).padStart(3, '0')}`, 'claude');
  const readAll = (unreadOnly, acknowledgePage = false) => {
    const result = [];
    let cursor;
    for (;;) {
      const page = store.inboxPage('claude', unreadOnly, 7, cursor);
      if (!page.length) break;
      for (const message of page) {
        assert.equal(message.to, 'claude');
        assert.equal(message.body, body, 'Pagination must never shorten an individual handoff.');
        if (acknowledgePage) assert.equal(store.acknowledge(message.id, 'claude'), true);
      }
      result.push(...page);
      const last = page.at(-1);
      cursor = { createdAt: last.createdAt, id: last.id };
    }
    return result;
  };
  const unread = readAll(true, true);
  assert.deepEqual(unread.map(value => value.id), Array.from({ length: 100 }, (_, index) => index).filter(index => index % 3 !== 0).map(index => `message-${String(index).padStart(3, '0')}`));
  assert.deepEqual(store.inboxPage('claude'), []);
  const history = readAll(false);
  assert.equal(history.length, 100);
  assert.ok(history.every(message => message.acknowledgedAt));
  assert.equal(new Set(history.map(message => message.id)).size, 100);
  assert.equal(store.inboxPage('junie', true, 100).length, 10);
  assert.ok(store.inboxPage('junie', true, 100, { createdAt: timestamp, id: 'message-000' }).every(message => message.to === 'junie'));
  assert.equal(store.inbox('claude', false).length, 100, 'Legacy inbox retains its original full-batch behavior.');
  for (const limit of [0, 101, 1.5, NaN]) assert.throws(() => store.inboxPage('claude', true, limit), /limit/);
  assert.throws(() => store.inboxPage('claude', 'yes'), /boolean/);
  assert.throws(() => store.inboxPage('claude', true, 20, { createdAt: '', id: 'id' }), /timestamp/);
  assert.throws(() => store.inboxPage('claude', true, 20, { createdAt: timestamp, id: '' }), /cursor id/);
});

test('supported POSIX databases and existing WAL sidecars remain private inside precreated state directories', { skip: process.platform === 'win32' }, t => {
  const { directory, open } = fixture(t);
  chmodSync(directory, 0o755);
  const first = open();
  first.putContext('private-note', 'Local task evidence.', 'codex');
  const files = ['state.db', 'state.db-wal', 'state.db-shm'].map(name => join(directory, name));
  for (const path of files) assert.equal(statSync(path).mode & 0o777, 0o600);
  for (const path of files) chmodSync(path, 0o644);
  const reopened = open();
  assert.equal(reopened.getContext('private-note').value, 'Local task evidence.');
  reopened.putContext('private-note', 'Updated local evidence.', 'claude');
  for (const path of files) assert.equal(statSync(path).mode & 0o777, 0o600, `Expected private permissions on ${path}.`);
  assert.equal(statSync(directory).mode & 0o777, 0o755, 'Existing parent directory permissions are not changed.');
});

test('future POSIX WAL databases preserve their file modes and committed bytes when rejected', { skip: process.platform === 'win32' }, t => {
  const { directory } = fixture(t);
  const raw = new DatabaseSync(join(directory, 'state.db'));
  try {
    raw.exec('PRAGMA journal_mode = WAL; PRAGMA user_version = 99; CREATE TABLE future_state(value TEXT); INSERT INTO future_state VALUES (\'preserved\');');
    assert.equal(raw.prepare('PRAGMA user_version').get().user_version, 99);
    const files = ['state.db', 'state.db-wal', 'state.db-shm'].map(name => join(directory, name));
    for (const path of files) chmodSync(path, 0o640);
    const before = files.map(path => ({ path, mode: statSync(path).mode, bytes: readFileSync(path) }));
    assert.throws(() => new CouncilStore(directory), /newer than supported/);
    for (const file of before) {
      assert.equal(statSync(file.path).mode, file.mode, 'Unsupported databases and journals must retain their original permissions.');
      assert.deepEqual(readFileSync(file.path), file.bytes, 'Read-only version preflight must preserve future committed state.');
    }
  } finally { raw.close(); }
});
