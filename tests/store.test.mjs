import assert from 'node:assert/strict';
import { existsSync, mkdtempSync, readFileSync, readdirSync, rmSync, writeFileSync } from 'node:fs';
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
  assert.equal(check.prepare('PRAGMA user_version').get().user_version, 3);
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
  assert.equal(health.schemaVersion, 3);
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
  assert.equal(migrated.health().schemaVersion, 3);
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
