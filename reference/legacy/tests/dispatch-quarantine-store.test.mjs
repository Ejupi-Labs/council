import test from 'node:test';
import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { once } from 'node:events';
import { mkdtempSync, realpathSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { basename, dirname, join } from 'node:path';
import { DatabaseSync } from 'node:sqlite';
import { CouncilStore } from '../dist/store.js';
import { DispatchQuarantinedError, PROCESS_CLEANUP_STAGES, QUARANTINE_INSPECTION_REQUIRED } from '../dist/dispatch-quarantine.js';

function fixture(t) {
  const parent = realpathSync.native(tmpdir()), root = realpathSync.native(mkdtempSync(join(parent, 'council-quarantine-store-'))), stores = [];
  t.after(() => {
    for (const store of stores) { try { store.close(); } catch {} }
    assert.equal(dirname(realpathSync.native(root)), parent); assert.ok(basename(root).startsWith('council-quarantine-store-'));
    rmSync(root, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 });
  });
  return { root, open() { const store = new CouncilStore(root); stores.push(store); return store; } };
}
const record = (id, status = 'queued') => ({ id, status, createdAt: '2026-01-01T00:00:00.000Z', updatedAt: '2026-01-01T00:00:00.000Z',
  input: { mode: 'delegate', task: 'Offline quarantine storage fixture.', agents: ['codex'], permission: 'read' }, steps: [] });
const report = overrides => ({ reportKey: 'owned-attempt', instanceId: 'reporter', stage: 'agent_call', ...overrides });
const reviewed = expectedRevision => ({ expectedRevision, reviewed: true });
const code = expected => error => error.code === expected && error.status === 409;

test('owned cleanup reports are compact, idempotent, conflict-safe and refuse later slot or run admission', t => {
  const setup = fixture(t), store = setup.open();
  store.registerInstance('reporter', process.pid, 1);
  const run = record('protected-run'); store.createRun(run, 'reporter');
  const held = store.acquireSlot('reporter', run.id, 1);
  assert.equal(store.acquireSlot('reporter', run.id, 1, 'existing-waiter'), undefined);
  const input = report({ runId: run.id }), incident = store.reportProcessCleanupIncident(input);
  assert.equal(incident.generation, '1'); assert.equal(incident.reason, 'process_cleanup_unverified'); assert.equal(incident.reportingPid, process.pid);
  assert.match(incident.id, /^[0-9a-f-]{36}$/); assert.equal(new Date(incident.observedAt).toISOString(), incident.observedAt);
  assert.deepEqual(store.reportProcessCleanupIncident(input), incident);
  assert.throws(() => store.reportProcessCleanupIncident({ ...input, stage: 'git_create' }), code('COUNCIL_CLEANUP_REPORT_CONFLICT'));
  assert.throws(() => store.reportProcessCleanupIncident({ ...input, runId: 'PRIVATE_CHANGED_PAYLOAD' }), error => {
    assert.equal(error.message.includes('PRIVATE_CHANGED_PAYLOAD'), false); return code('COUNCIL_CLEANUP_REPORT_CONFLICT')(error);
  });
  const receipt = { blocked: true, currentRevision: '1', openCount: 1 };
  assert.throws(() => store.acquireSlot('reporter', run.id, 1, 'must-not-be-created'), error => {
    assert.ok(error instanceof DispatchQuarantinedError); assert.deepEqual(error.receipt, receipt); return true;
  });
  assert.equal(store.health().counts.slotWaiters, 1);
  assert.throws(() => store.createRun(record('must-not-be-created'), 'reporter'), error => error instanceof DispatchQuarantinedError);
  assert.equal(store.hasRun('must-not-be-created'), false);
  store.releaseSlot(held, 'reporter'); store.cancelSlotWaiter('reporter', 'existing-waiter');
  assert.equal(store.health().counts.slots, 0); assert.equal(store.health().counts.slotWaiters, 0);
  assert.deepEqual(store.health().dispatchQuarantine, receipt);
  assert.equal(store.finalizeRun({ ...run, status: 'cancelled' }).status, 'failed');
  store.unregisterInstance('reporter'); store.close();
  const reopened = setup.open();
  assert.deepEqual(reopened.dispatchQuarantinePage().incidents, [incident]);
  assert.deepEqual(reopened.recoverOrphans(), []); assert.equal(reopened.dispatchQuarantinePage().blocked, true);
  const acknowledged = reopened.acknowledgeDispatchQuarantine(reviewed('1'));
  assert.equal(acknowledged.blocked, false); assert.equal(acknowledged.acknowledgment.incidentCount, 1);
  const retried = reopened.reportProcessCleanupIncident(input);
  assert.equal(retried.id, incident.id); assert.equal(retried.generation, '1'); assert.equal(retried.reportingPid, process.pid);
  assert.equal(retried.acknowledgmentId, acknowledged.acknowledgment.id);
  assert.equal(retried.acknowledgedAt, acknowledged.acknowledgment.acknowledgedAt);
  assert.deepEqual(reopened.acknowledgeDispatchQuarantine(reviewed('1')), acknowledged);
  assert.equal(reopened.db.prepare('SELECT COUNT(*) AS count FROM dispatch_quarantine_acknowledgments').get().count, 1);
});

test('report validation never stores unknown diagnostics or accepts unowned new reports', t => {
  const store = fixture(t).open();
  assert.throws(() => store.reportProcessCleanupIncident(report()), /not registered/);
  store.registerInstance('reporter', process.pid);
  for (const property of ['reportKey', 'instanceId', 'runId']) {
    assert.throws(() => store.reportProcessCleanupIncident({ ...report(), [property]: '\ud800' }), /well-formed Unicode/);
  }
  for (const input of [null, [], { ...report(), diagnostics: 'PRIVATE_BODY' }, { ...report(), reportingPid: 1234 }, { ...report(), stage: 'unexpected' },
    { ...report(), reportKey: 'x'.repeat(129) }, { ...report(), instanceId: '' }, { ...report(), runId: '\0' },
    { ...report(), reportKey: '\ud800' }, { ...report(), runId: '🧭'.repeat(65) }]) assert.throws(() => store.reportProcessCleanupIncident(input));
  assert.deepEqual(store.dispatchQuarantinePage(), { blocked: false, currentRevision: '0', openCount: 0, incidents: [], nextCursor: null });
  for (const [index, stage] of PROCESS_CLEANUP_STAGES.entries()) store.reportProcessCleanupIncident(report({ reportKey: 'stage-' + index, stage }));
  assert.equal(store.dispatchQuarantinePage().openCount, PROCESS_CLEANUP_STAGES.length);
  assert.equal(JSON.stringify(store.dispatchQuarantinePage()).includes('PRIVATE_BODY'), false);
});

test('report-time PIDs remain original or explicitly unknown across ownership changes and audited retries', t => {
  const setup = fixture(t), owner = setup.open(); owner.registerInstance('reporter', process.pid);
  const knownInput = report(), unknownInput = report({ reportKey: 'historical-unknown' });
  const known = owner.reportProcessCleanupIncident(knownInput), unknown = owner.reportProcessCleanupIncident(unknownInput);
  owner.db.prepare('UPDATE process_cleanup_incidents SET reporting_pid = NULL WHERE id = ?').run(unknown.id);
  owner.db.prepare('UPDATE instances SET pid = ? WHERE id = ?').run(process.pid + 1, 'reporter');
  assert.equal(owner.reportProcessCleanupIncident(knownInput).reportingPid, process.pid);
  assert.equal(owner.reportProcessCleanupIncident(unknownInput).reportingPid, undefined, 'Historical identity must never be inferred from instance metadata.');
  owner.db.prepare('UPDATE instances SET pid = ? WHERE id = ?').run(process.pid, 'reporter'); owner.unregisterInstance('reporter'); owner.close();
  const offline = setup.open(); offline.acknowledgeDispatchQuarantine(reviewed('2'));
  assert.equal(offline.reportProcessCleanupIncident(knownInput).reportingPid, process.pid);
  const historical = offline.reportProcessCleanupIncident(unknownInput);
  assert.equal(Object.hasOwn(historical, 'reportingPid'), false); assert.ok(historical.acknowledgedAt);
  assert.equal(offline.db.prepare('SELECT reporting_pid FROM process_cleanup_incidents WHERE id = ?').get(unknown.id).reporting_pid, null);
});

test('generation cursors and acknowledgement preserve SQLite integers beyond JavaScript precision', t => {
  const setup = fixture(t), store = setup.open(); store.registerInstance('reporter', process.pid);
  store.reportProcessCleanupIncident(report({ reportKey: 'first' }));
  store.db.prepare("UPDATE sqlite_sequence SET seq = ? WHERE name = 'process_cleanup_incidents'").run(9007199254740992n);
  const left = store.reportProcessCleanupIncident(report({ reportKey: 'large-left' }));
  const right = store.reportProcessCleanupIncident(report({ reportKey: 'large-right' }));
  assert.equal(left.generation, '9007199254740993'); assert.equal(right.generation, '9007199254740994');
  const first = store.dispatchQuarantinePage({ cursor: '1', limit: 1 });
  assert.deepEqual(first.incidents, [left]); assert.equal(first.nextCursor, left.generation); assert.equal(first.currentRevision, right.generation);
  assert.deepEqual(store.dispatchQuarantinePage({ cursor: first.nextCursor, limit: 1 }).incidents, [right]);
  store.unregisterInstance('reporter');
  const offline = setup.open();
  const result = offline.acknowledgeDispatchQuarantine(reviewed(right.generation));
  assert.equal(result.acknowledgment.revision, right.generation); assert.equal(result.acknowledgment.incidentCount, 3);
  assert.deepEqual(offline.acknowledgeDispatchQuarantine(reviewed(right.generation)), result);
});

test('incident reads are bounded, query-only and independent of large or malformed workflow documents', t => {
  const store = fixture(t).open(); store.registerInstance('reporter', process.pid);
  store.saveRun({ ...record('large', 'completed'), output: 'DO_NOT_DECODE_COMPLETE_RUN ' + '🧭'.repeat(200000) });
  store.saveRun(record('malformed', 'completed'));
  store.db.prepare('UPDATE runs SET document = ? WHERE id = ?').run('{', 'malformed');
  for (let index = 0; index < 125; index++) store.reportProcessCleanupIncident(report({ reportKey: 'report-' + index, runId: 'workflow-' + index }));
  const parse = JSON.parse;
  JSON.parse = () => { assert.fail('Incident observations must not parse workflow or transcript documents.'); };
  store.db.exec('PRAGMA query_only=ON');
  try {
    const first = store.dispatchQuarantinePage({ limit: 100 });
    assert.equal(first.incidents.length, 100); assert.equal(first.openCount, 125); assert.equal(first.currentRevision, '125');
    assert.equal(first.nextCursor, '100'); assert.ok(Buffer.byteLength(JSON.stringify(first)) < 128 * 1024);
    const last = store.dispatchQuarantinePage({ cursor: first.nextCursor });
    assert.equal(last.incidents.length, 25); assert.equal(last.nextCursor, null);
    assert.deepEqual(store.dispatchQuarantinePage({ cursor: '9223372036854775807' }).incidents, []);
    assert.throws(() => store.assertDispatchUnblocked(), error => error instanceof DispatchQuarantinedError && Buffer.byteLength(JSON.stringify(error.receipt)) < 128);
    assert.equal(store.health().dispatchQuarantine.openCount, 125);
  } finally { JSON.parse = parse; store.db.exec('PRAGMA query_only=OFF'); }
  for (const options of [null, { limit: null }, { limit: 0 }, { limit: 101 }, { limit: 1.5 }, { cursor: 1 }, { cursor: '01' }, { cursor: '-1' },
    { cursor: '9223372036854775808' }, { cursor: '1'.repeat(1000) }, { after: '1' }]) assert.throws(() => store.dispatchQuarantinePage(options));
});

test('maximum escaped incident identities still bound complete pages and never enlarge the refusal receipt', t => {
  const store = fixture(t).open(), instanceId = '\u0001'.repeat(128), runId = '\u0001'.repeat(256);
  store.registerInstance(instanceId, process.pid);
  for (let index = 0; index < 101; index++) store.reportProcessCleanupIncident({
    reportKey: '\u0001'.repeat(125) + String(index).padStart(3, '0'), instanceId, runId, stage: 'maintenance_cleanup',
  });
  const page = store.dispatchQuarantinePage({ limit: 100 }), bytes = Buffer.byteLength(JSON.stringify(page));
  assert.equal(page.incidents.length, 100); assert.equal(page.nextCursor, '100');
  assert.ok(bytes > 256 * 1024, 'Escaping must be included in the materialization bound, beyond raw identity bytes.');
  assert.ok(bytes < 512 * 1024); assert.equal(page.incidents[0].instanceId, instanceId); assert.equal(page.incidents[0].runId, runId);
  assert.throws(() => store.assertDispatchUnblocked(), error => error instanceof DispatchQuarantinedError && Buffer.byteLength(JSON.stringify(error.receipt)) < 128);
});

test('offline review refuses bound or live hosts, preserves ownerless historical rows and records their count', t => {
  const setup = fixture(t), owner = setup.open(); owner.registerInstance('reporter', process.pid);
  const historical = { ...record('ownerless-history', 'running'), output: 'Unresolved historical evidence.' };
  owner.saveRun(historical); owner.reportProcessCleanupIncident(report());
  assert.throws(() => owner.acknowledgeDispatchQuarantine(reviewed('1')), code('COUNCIL_QUARANTINE_OFFLINE_REQUIRED'));
  const offline = setup.open();
  assert.throws(() => offline.acknowledgeDispatchQuarantine(reviewed('1')), code('COUNCIL_QUARANTINE_OFFLINE_REQUIRED'));
  owner.unregisterInstance('reporter');
  assert.throws(() => owner.acknowledgeDispatchQuarantine(reviewed('1')), code('COUNCIL_QUARANTINE_OFFLINE_REQUIRED'), 'A previously bound host cannot clear its own local fence.');
  for (const input of [null, { expectedRevision: '1', reviewed: false }, { expectedRevision: 1, reviewed: true },
    { expectedRevision: '01', reviewed: true }, { ...reviewed('1'), automatic: true }]) assert.throws(() => offline.acknowledgeDispatchQuarantine(input));
  const result = offline.acknowledgeDispatchQuarantine(reviewed('1'));
  assert.equal(result.unownedUnfinishedRuns, 1); assert.equal(result.acknowledgment.unownedUnfinishedRuns, 1);
  assert.deepEqual(offline.getRun(historical.id), historical);
  assert.equal(offline.health().counts.activeRuns, 1); assert.equal(offline.dispatchQuarantinePage().blocked, false);
});

test('review of an empty workspace is an explicit no-op and new reports invalidate stale acknowledgement', t => {
  const setup = fixture(t), offline = setup.open();
  assert.deepEqual(offline.acknowledgeDispatchQuarantine(reviewed('0')), { blocked: false, currentRevision: '0', openCount: 0,
    incidents: [], nextCursor: null, unownedUnfinishedRuns: 0 });
  const first = setup.open(); first.registerInstance('reporter', process.pid); first.reportProcessCleanupIncident(report()); first.unregisterInstance('reporter');
  const acknowledgment = offline.acknowledgeDispatchQuarantine(reviewed('1')).acknowledgment;
  const later = setup.open(); later.registerInstance('later-reporter', process.pid);
  later.reportProcessCleanupIncident(report({ reportKey: 'later-attempt', instanceId: 'later-reporter' })); later.unregisterInstance('later-reporter');
  assert.throws(() => offline.acknowledgeDispatchQuarantine(reviewed('1')), code('COUNCIL_QUARANTINE_REVISION_CONFLICT'));
  assert.equal(offline.dispatchQuarantinePage().blocked, true); assert.equal(offline.dispatchQuarantinePage().currentRevision, '2');
  assert.equal(offline.db.prepare('SELECT COUNT(*) AS count FROM dispatch_quarantine_acknowledgments').get().count, 1);
  const second = offline.acknowledgeDispatchQuarantine(reviewed('2')).acknowledgment;
  assert.notEqual(second.id, acknowledgment.id); assert.equal(second.incidentCount, 1);
  assert.equal(offline.db.prepare('SELECT COUNT(*) AS count FROM dispatch_quarantine_acknowledgments').get().count, 2);
});

test('an acknowledgement write failure rolls back both the audit and every incident before a reviewed retry', t => {
  const setup = fixture(t), owner = setup.open(); owner.registerInstance('reporter', process.pid);
  owner.reportProcessCleanupIncident(report()); owner.reportProcessCleanupIncident(report({ reportKey: 'second-attempt' })); owner.unregisterInstance('reporter');
  const offline = setup.open(), before = offline.dispatchQuarantinePage();
  offline.db.exec("CREATE TRIGGER reject_review BEFORE UPDATE OF acknowledgment_id ON process_cleanup_incidents BEGIN SELECT RAISE(ABORT, 'Injected acknowledgement write failure'); END;");
  assert.throws(() => offline.acknowledgeDispatchQuarantine(reviewed('2')), /Injected acknowledgement write failure/);
  assert.equal(offline.db.isTransaction, false); assert.deepEqual(offline.dispatchQuarantinePage(), before);
  assert.equal(offline.db.prepare('SELECT COUNT(*) AS count FROM dispatch_quarantine_acknowledgments').get().count, 0);
  offline.db.exec('DROP TRIGGER reject_review');
  const result = offline.acknowledgeDispatchQuarantine(reviewed('2'));
  assert.equal(result.blocked, false); assert.equal(result.acknowledgment.incidentCount, 2);
  assert.equal(offline.db.prepare('SELECT COUNT(*) AS count FROM dispatch_quarantine_acknowledgments').get().count, 1);
});

for (const status of ['completed', 'cancelled', 'failed']) test(`visible quarantine finalization keeps ${status} evidence failed and retained artifacts inspection-required`, t => {
  const store = fixture(t).open(); store.registerInstance('reporter', process.pid);
  const old = record('historical-terminal', 'completed'); store.saveRun(old);
  const run = record('active-' + status); store.createRun(run, 'reporter'); store.requestCancellation(run.id);
  const artifacts = [
    { agent: 'codex', path: 'C:\\retained\\ready', baseCommit: 'exact-base', exportStatus: 'ready', changed: true },
    { agent: 'codex', path: 'C:\\retained\\failed', baseCommit: 'exact-base', exportStatus: 'failed', exportError: 'Original export failure.', changed: true },
  ];
  const candidate = { ...run, status, error: 'Original workflow failure.', output: 'Complete partial evidence.',
    steps: [{ id: 'step', agent: 'codex', role: 'worker', result: { agent: 'codex', status: 'failed', output: 'Full step evidence.', durationMs: 1 } }], workspaces: artifacts };
  store.reportProcessCleanupIncident(report({ runId: run.id }));
  const final = store.finalizeRun(candidate);
  assert.equal(final.status, 'failed'); assert.equal(final.output, candidate.output); assert.deepEqual(final.steps, candidate.steps);
  assert.equal(final.error, 'Original workflow failure.\n' + QUARANTINE_INSPECTION_REQUIRED);
  assert.equal(final.workspaces[0].exportStatus, 'pending'); assert.equal(final.workspaces[1].exportStatus, 'failed');
  assert.equal(final.workspaces[1].exportError, 'Original export failure.\n' + QUARANTINE_INSPECTION_REQUIRED);
  assert.equal(candidate.workspaces[0].exportStatus, 'ready', 'Caller evidence must not be mutated.');
  assert.deepEqual(store.finalizeRun({ ...final, status: 'completed' }), final, 'Terminal evidence remains immutable.');
  assert.deepEqual(store.finalizeRun({ ...old, status: 'failed' }), old, 'A later incident cannot rewrite historical results.');
});

test('ready artifact publication is transactionally refused while failed inspection metadata remains writable', t => {
  const store = fixture(t).open(); store.registerInstance('reporter', process.pid);
  const artifact = { agent: 'codex', path: 'C:\\retained\\artifact', baseCommit: 'exact-base', exportStatus: 'pending', changed: true };
  const run = { ...record('maintenance-run', 'failed'), workspaces: [artifact], error: 'Retained incomplete work.' }; store.saveRun(run);
  store.reportProcessCleanupIncident(report({ runId: run.id, stage: 'maintenance_export' }));
  assert.throws(() => store.updateArtifact(run.id, { ...artifact, exportStatus: 'ready' }), error => error instanceof DispatchQuarantinedError);
  assert.deepEqual(store.getRun(run.id), run);
  const updated = store.updateArtifact(run.id, { ...artifact, exportStatus: 'failed', exportError: 'Inspection found incomplete export.' });
  assert.equal(updated.workspaces[0].exportStatus, 'failed'); assert.equal(updated.workspaces[0].exportError, 'Inspection found incomplete export.');
});

test('open cleanup evidence survives pruning and acknowledgment does not couple incidents to run deletion', t => {
  const setup = fixture(t), owner = setup.open(); owner.registerInstance('reporter', process.pid);
  owner.saveRun(record('protected-old-run', 'completed')); owner.saveRun(record('unrelated-old-run', 'completed'));
  const input = report({ runId: 'protected-old-run' }), incident = owner.reportProcessCleanupIncident(input);
  owner.unregisterInstance('reporter');
  const options = { before: '2026-11-05T00:00:00.000Z', keepLatest: 0, dryRun: false };
  assert.deepEqual(owner.prune(options).candidateIds, ['unrelated-old-run']); assert.equal(owner.hasRun('protected-old-run'), true);
  const offline = setup.open(); offline.acknowledgeDispatchQuarantine(reviewed('1'));
  assert.deepEqual(offline.prune(options).candidateIds, ['protected-old-run']);
  assert.equal(offline.hasRun('protected-old-run'), false);
  const retried = offline.reportProcessCleanupIncident(input); assert.equal(retried.id, incident.id); assert.ok(retried.acknowledgedAt);
});

test('proven dead owner recovery never clears the incident and cannot stand in for reviewed inspection', { timeout: 15000 }, async t => {
  const setup = fixture(t), store = setup.open(); store.registerInstance('reporter', process.pid);
  const run = record('dead-owner-run'); store.createRun(run, 'reporter'); store.acquireSlot('reporter', run.id, 1);
  store.reportProcessCleanupIncident(report({ runId: run.id }));
  const child = spawn(process.execPath, ['-e', ''], { windowsHide: true, stdio: 'ignore' }); await once(child, 'close');
  store.db.prepare('UPDATE instances SET pid = ? WHERE id = ?').run(child.pid, 'reporter');
  store.db.prepare('UPDATE runs SET owner_pid = ? WHERE id = ?').run(child.pid, run.id);
  const observer = setup.open(); assert.equal(observer.recoverOrphans()[0].status, 'failed');
  assert.equal(observer.health().counts.slots, 0); assert.equal(observer.health().counts.instances, 0);
  assert.equal(observer.dispatchQuarantinePage().blocked, true);
  assert.throws(() => observer.acknowledgeDispatchQuarantine({ expectedRevision: '1', reviewed: false }), /reviewed/);
  assert.equal(observer.acknowledgeDispatchQuarantine(reviewed('1')).blocked, false);
});

test('SQLite contention and read-only report failure stay explicit without creating a false durable incident', t => {
  const setup = fixture(t), store = setup.open(); store.registerInstance('reporter', process.pid);
  const writer = new DatabaseSync(join(setup.root, 'state.db'));
  try {
    writer.exec('BEGIN IMMEDIATE');
    assert.throws(() => store.reportProcessCleanupIncident(report()), error => error.errcode === 5);
    assert.equal(store.dispatchQuarantinePage().blocked, false); assert.equal(store.db.isTransaction, false);
    assert.equal(store.db.prepare('PRAGMA busy_timeout').get().timeout, 250);
  } finally { writer.exec('ROLLBACK'); writer.close(); }
  store.db.exec('PRAGMA query_only=ON');
  try {
    store.assertDispatchUnblocked();
    assert.throws(() => store.reportProcessCleanupIncident(report()), /readonly|read-only/i);
    assert.equal(store.dispatchQuarantinePage().currentRevision, '0');
  } finally { store.db.exec('PRAGMA query_only=OFF'); }
  assert.equal(store.reportProcessCleanupIncident(report()).generation, '1');
});

test('expected quarantine errors retain a tiny immutable receipt without echoing caller metadata', () => {
  const receipt = { blocked: false, currentRevision: '1', openCount: 1, privateBody: 'DO_NOT_COPY' };
  const error = new DispatchQuarantinedError(receipt); receipt.currentRevision = '2';
  assert.equal(error.code, 'COUNCIL_DISPATCH_QUARANTINED'); assert.equal(error.status, 409);
  assert.deepEqual(error.receipt, { blocked: true, currentRevision: '1', openCount: 1 });
  assert.throws(() => { error.receipt.currentRevision = '3'; }, TypeError);
  assert.equal(JSON.stringify(error).includes('DO_NOT_COPY'), false);
});
