import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { basename, dirname, join, resolve } from 'node:path';
import { setTimeout as delay } from 'node:timers/promises';
import { DatabaseSync } from 'node:sqlite';
import { execFileSync } from 'node:child_process';
import { CouncilService } from '../dist/service.js';
import { loadConfig } from '../dist/config.js';
import { RoomManager } from '../dist/rooms.js';
import { DelegationManager, DelegationRevisionConflict } from '../dist/delegation.js';

function fixture(t) {
  const parent = resolve(tmpdir()), root = mkdtempSync(join(parent, 'council-delegation-'));
  const service = new CouncilService(loadConfig(root, { backend: 'mock' }), {
    doctor: async () => [], run: async () => { assert.fail('Delegation management must never invoke a provider.'); },
  });
  const manager = new DelegationManager(service), managers = [manager], peers = [];
  t.after(async () => {
    for (const value of managers) value.close();
    for (const value of peers) await value.close();
    await service.close();
    assert.equal(dirname(resolve(root)), parent); assert.ok(basename(root).startsWith('council-delegation-'));
    rmSync(root, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 });
  });
  const room = service.rooms.create({ title: 'Executive oversight', brief: 'Preserve original human task permissions.',
    members: [{ agent: 'codex', role: 'lead' }], tasks: [{ title: 'Inspect', brief: 'Inspect without writing.', permission: 'read' }] });
  const configuration = extra => ({ mode: 'agent', delegate: 'Codex host', mandate: 'Set up and monitor read-only work; report decisions to the human.', roomIds: [room.id], expectedRevision: manager.get().revision, ...extra });
  return { root, service, manager, room, configuration,
    peer() { const peer = new DelegationManager(service); managers.push(peer); return peer; },
    roomPeer() { const peer = new RoomManager(service); peers.push(peer); return peer; },
    transition(change, publish = true) {
      const updated = service.rooms.store.mutate(room.id, change);
      if (publish) service.rooms.publish(updated);
      return updated;
    } };
}
const report = (room, extra = {}) => ({ author: 'Host agent', kind: 'decision', body: 'Which of these two approaches should the board pursue?', roomIds: [room.id], ...extra });

test('delegation defaults to human control; valid mandate persists and stale saves cannot overwrite it', t => {
  const { manager, peer, configuration } = fixture(t);
  const initial = manager.get(); assert.equal(initial.mode, 'human'); assert.equal(initial.revision, 0); assert.deepEqual(initial.roomIds, []);
  const other = peer(), saved = manager.configure(configuration());
  assert.equal(saved.revision, 1); assert.deepEqual(other.get(), saved);
  assert.throws(() => other.configure({ ...configuration(), expectedRevision: 0, mandate: 'Stale overwrite' }), DelegationRevisionConflict);
  saved.roomIds.push('caller-mutation'); saved.mandate = 'Caller mutation';
  assert.equal(manager.get().roomIds.length, 1); assert.match(manager.get().mandate, /monitor read-only/);
  assert.throws(() => manager.configure(configuration({ delegate: '  ' })), /Delegate must contain/);
  assert.throws(() => manager.configure(configuration({ mandate: '\t' })), /mandate must contain/);
  assert.throws(() => manager.configure(configuration({ roomIds: ['unknown'] })), /404/);
  assert.throws(() => manager.configure(configuration({ roomIds: ['a', 'a'] })), /unique/);
  assert.throws(() => manager.configure(configuration({ expectedRevision: 1.5 })), /integer/);
  assert.throws(() => manager.configure(configuration({ mandate: 'hidden\0instruction' })), /format/);
  assert.equal(manager.get().revision, 1);
});

test('delegate reports retain room observations without changing guidance, permissions or approvals', async t => {
  const { manager, service, room, transition } = fixture(t);
  const ready = transition(value => {
    value.status = 'ready'; value.tasks[0].status = 'assigned'; value.tasks[0].assignee = 'codex';
    value.decision = { instructionRevision: 0, approved: true, approveWeight: 1, totalWeight: 1, assignments: { 'task-1': 'codex' }, rationale: 'Approved', dissent: [] };
  });
  const saved = manager.report(report(room, { body: 'A full report with no evidence clipping.\n' + 'Long retained evidence. '.repeat(1000) }));
  assert.equal(saved.sequence, 1); assert.equal(saved.revision, 1);
  assert.equal(saved.observedRooms[0].revision, ready.revision); assert.equal(saved.observedRooms[0].instructionRevision, 0);
  assert.deepEqual(service.rooms.get(room.id), ready);
  service.rooms.postMessage(room.id, { author: 'Human', body: 'New scope requires discussion.' });
  assert.equal(manager.reportsPage().reports[0].observedRooms[0].revision, ready.revision, 'Old observations remain old instead of claiming current evidence.');
  saved.observedRooms[0].revision = 999; saved.body = 'Caller overwrite';
  assert.equal(manager.reportsPage().reports[0].observedRooms[0].revision, ready.revision);
  assert.ok(manager.reportsPage().reports[0].body.length > 20000);
  assert.equal((await manager.watch({ timeoutMs: 0 })).openQuestions, 1);
});

test('explicit reviewed revisions reject stale or incomplete reports atomically', t => {
  const { manager, service, room } = fixture(t);
  service.rooms.postMessage(room.id, { author: 'Human', body: 'Changed after delegate reviewed the room.' });
  assert.throws(() => manager.report(report(room, { expectedRoomRevisions: { [room.id]: room.revision } })), /changed after review/);
  assert.throws(() => manager.report(report(room, { expectedRoomRevisions: {} })), /every report room/);
  assert.throws(() => manager.report(report(room, { expectedRoomRevisions: { unknown: 0 } })), /report's room IDs/);
  assert.equal(manager.reportsPage().reports.length, 0);
  const current = service.rooms.get(room.id);
  const saved = manager.report(report(room, { expectedRoomRevisions: { [room.id]: current.revision } }));
  assert.equal(saved.observedRooms[0].revision, current.revision);
  assert.equal(saved.expectedRoomRevisions, undefined, 'Only verified captured observations are stored.');
});

test('reports page every retained entry in ascending order and filter unresolved questions completely', t => {
  const { manager, room } = fixture(t);
  const inserted = [];
  for (let index = 0; index < 107; index++) inserted.push(manager.report(report(room, { kind: index % 3 === 0 ? 'decision' : index % 3 === 1 ? 'blocked' : 'update', body: `Retained report ${index}` })));
  manager.resolve(inserted[0].id, { author: 'Human', body: 'Use approach A.', expectedRevision: 1 });
  let cursor = 0; const all = [];
  do { const page = manager.reportsPage({ cursor, limit: 17 }); all.push(...page.reports); cursor = page.nextCursor; } while (cursor !== null);
  assert.deepEqual(all.map(item => item.sequence), inserted.map(item => item.sequence));
  assert.equal(all.at(-1).body, 'Retained report 106');
  cursor = 0; const open = [];
  do { const page = manager.reportsPage({ cursor, limit: 11, openOnly: true }); open.push(...page.reports); cursor = page.nextCursor; } while (cursor !== null);
  assert.deepEqual(open.map(item => item.id), inserted.filter((item, index) => index !== 0 && item.kind !== 'update').map(item => item.id));
  for (const options of [{ limit: 0 }, { limit: 101 }, { cursor: -1 }, { openOnly: 'yes' }]) assert.throws(() => manager.reportsPage(options), /bounds/);
});

test('human responses use optimistic report revisions and never change the project instructions', t => {
  const { manager, peer, service, room } = fixture(t), other = peer();
  const before = service.rooms.get(room.id), saved = manager.report(report(room));
  const resolved = other.resolve(saved.id, { author: 'Human CEO', body: 'Choose A. Ask again if scope changes.', expectedRevision: 1 });
  assert.equal(resolved.revision, 2); assert.equal(resolved.resolution.author, 'Human CEO'); assert.equal(resolved.body, saved.body);
  assert.throws(() => manager.resolve(saved.id, { author: 'Other browser', body: 'Stale response', expectedRevision: 1 }), DelegationRevisionConflict);
  assert.throws(() => manager.resolve(saved.id, { author: 'Human', body: 'Repeated response', expectedRevision: 2 }), /already resolved/);
  assert.throws(() => manager.resolve(saved.id, { author: 'Human', body: 'No revision' }), /missing required/);
  const update = manager.report(report(room, { kind: 'update' }));
  assert.throws(() => manager.resolve(update.id, { author: 'Human', body: 'Not a question', expectedRevision: 1 }), /Only a decision/);
  assert.equal(manager.reportsPage({ openOnly: true }).reports.length, 0); assert.deepEqual(service.rooms.get(room.id), before);
});

test('delegation overview is a compact consistent badge without report bodies or the private mandate', t => {
  const { manager, configuration, room } = fixture(t);
  manager.configure(configuration()); const issue = manager.report(report(room));
  assert.deepEqual(manager.overview(), { mode: 'agent', delegate: 'Codex host', configurationRevision: 1, reportRevision: 1, openQuestions: 1 });
  manager.resolve(issue.id, { author: 'Human', body: 'Proceed.', expectedRevision: 1 });
  assert.deepEqual(manager.overview(), { mode: 'agent', delegate: 'Codex host', configurationRevision: 1, reportRevision: 2, openQuestions: 0 });
});

test('delegate watch waits without returning ordinary progress, then notices quiescence and human guidance', async t => {
  const { manager, room, configuration, transition, service } = fixture(t);
  manager.configure(configuration()); transition(value => { value.status = 'running'; value.tasks[0].status = 'running'; });
  const initial = await manager.watch({ timeoutMs: 0 }); assert.equal(initial.reason, 'changed');
  const quiet = manager.watch({ cursor: initial.cursor, timeoutMs: 70 });
  transition(value => { value.callsUsed++; });
  const timed = await quiet; assert.equal(timed.reason, 'timeout'); assert.equal(timed.rooms[0].status, 'running'); assert.equal(timed.rooms[0].callsUsed, 1);
  let settled = false;
  const next = manager.watch({ cursor: timed.cursor, timeoutMs: 2000 }).then(value => { settled = true; return value; });
  await delay(20); assert.equal(settled, false);
  service.rooms.postMessage(room.id, { author: 'Human', body: 'Please change the acceptance criterion.' });
  const guided = await next; assert.equal(guided.reason, 'changed'); assert.ok(guided.cursor.rooms[room.id].instructionRevision > timed.cursor.rooms[room.id].instructionRevision);
  const waiting = manager.watch({ cursor: guided.cursor, timeoutMs: 2000 });
  transition(value => { value.status = 'needs_attention'; });
  const actionable = await waiting; assert.equal(actionable.reason, 'changed'); assert.equal(actionable.rooms[0].actionable, true);
  assert.equal(manager.listeners.size, 0); assert.equal(service.rooms.listeners.size, 0);
});

test('delegate watch sees configuration revocation, reports and responses from another manager', async t => {
  const { manager, peer, configuration, room } = fixture(t), other = peer();
  manager.configure(configuration());
  let snapshot = await manager.watch({ timeoutMs: 0 });
  const reportWait = manager.watch({ cursor: snapshot.cursor, timeoutMs: 2000 });
  const issue = other.report(report(room)); snapshot = await reportWait;
  assert.equal(snapshot.reason, 'changed'); assert.equal(snapshot.openQuestions, 1);
  const responseWait = manager.watch({ cursor: snapshot.cursor, timeoutMs: 2000 });
  other.resolve(issue.id, { author: 'Human', body: 'Proceed with A.', expectedRevision: 1 }); snapshot = await responseWait;
  assert.equal(snapshot.openQuestions, 0);
  const revokeWait = manager.watch({ cursor: snapshot.cursor, timeoutMs: 2000 });
  other.configure(configuration({ mode: 'human', delegate: '', mandate: '' })); snapshot = await revokeWait;
  assert.equal(snapshot.configuration.mode, 'human'); assert.equal(manager.reportsPage().reports.length, 1);
});

test('change cursors discover human replies to earlier reports after newer reports were already read', async t => {
  const { manager, peer, room } = fixture(t), other = peer();
  const first = manager.report(report(room));
  manager.report(report(room, { body: 'A newer update.', kind: 'update' }));
  const observed = await manager.watch({ timeoutMs: 0 });
  assert.equal(observed.cursor.reportRevision, 2);
  other.resolve(first.id, { author: 'Human', body: 'Reply to the older question.', expectedRevision: 1 });
  const changes = manager.reportChangesPage({ after: observed.cursor.reportRevision });
  assert.equal(changes.changes.length, 1); assert.equal(changes.changes[0].revision, 3);
  assert.equal(changes.changes[0].report.id, first.id); assert.equal(changes.changes[0].report.resolution.body, 'Reply to the older question.');
  assert.equal(changes.nextCursor, null);
  assert.deepEqual(manager.reportChangesPage({ after: 3 }), { changes: [], nextCursor: null });
});

test('report changes page completely by monotonic change revision and expose current retained state', t => {
  const { manager, room } = fixture(t), inserted = [];
  for (let index = 0; index < 107; index++) inserted.push(manager.report(report(room, { body: `Question ${index}` })));
  for (let index = 0; index < 10; index++) manager.resolve(inserted[index].id, { author: 'Human', body: `Answer ${index}`, expectedRevision: 1 });
  const changes = []; let cursor = 0;
  do { const page = manager.reportChangesPage({ after: cursor, limit: 13 }); changes.push(...page.changes); cursor = page.nextCursor; } while (cursor !== null);
  assert.equal(changes.length, 107); assert.equal(new Set(changes.map(change => change.report.id)).size, 107);
  assert.deepEqual(changes.map(change => change.revision), Array.from({ length: 107 }, (_, index) => index + 11));
  assert.equal(changes.at(-1).report.id, inserted[9].id); assert.equal(changes.at(-1).report.resolution.body, 'Answer 9');
  for (const options of [{ after: -1 }, { after: 1.5 }, { limit: 0 }, { limit: 101 }]) assert.throws(() => manager.reportChangesPage(options), /bounds/);
});

test('legacy delegation stores migrate retained replies to unique change revisions above earlier cursors', async t => {
  const { manager, peer, service, room, configuration } = fixture(t);
  const saved = manager.configure(configuration()), reports = [manager.report(report(room)), manager.report(report(room, { kind: 'update' }))];
  manager.resolve(reports[0].id, { author: 'Human', body: 'Retain this legacy reply.', expectedRevision: 1 });
  const earlier = await manager.watch({ timeoutMs: 0 }); assert.equal(earlier.cursor.reportRevision, 3);
  manager.close();
  const legacy = new DatabaseSync(join(service.config.stateDir, 'delegation.sqlite'));
  try { legacy.exec('DROP INDEX delegation_report_changes; ALTER TABLE delegation_reports DROP COLUMN change_revision; PRAGMA user_version=1;'); }
  finally { legacy.close(); }
  const migrated = peer();
  assert.deepEqual(migrated.get(), saved);
  const changes = migrated.reportChangesPage({ after: earlier.cursor.reportRevision, limit: 1 });
  assert.equal(changes.changes.length, 1); assert.equal(changes.changes[0].revision, 4); assert.equal(changes.nextCursor, 4);
  assert.equal(changes.changes[0].report.resolution.body, 'Retain this legacy reply.');
  const rest = migrated.reportChangesPage({ after: changes.nextCursor }); assert.equal(rest.changes[0].revision, 5);
  const snapshot = await migrated.watch({ cursor: earlier.cursor, timeoutMs: 0 });
  assert.equal(snapshot.reason, 'changed'); assert.equal(snapshot.cursor.workspaceId, earlier.cursor.workspaceId); assert.equal(snapshot.cursor.reportRevision, 5);
});

test('delegate watch sees cross-host room changes without a local publication', async t => {
  const { manager, configuration, roomPeer, room } = fixture(t);
  manager.configure(configuration()); const initial = await manager.watch({ timeoutMs: 0 });
  const wait = manager.watch({ cursor: initial.cursor, timeoutMs: 2000 });
  roomPeer().postMessage(room.id, { author: 'Human', body: 'Cross-host guidance.' });
  const changed = await wait;
  assert.equal(changed.reason, 'changed'); assert.equal(changed.cursor.rooms[room.id].instructionRevision, 1);
});

test('delegate watch rejects foreign cursors and cleans up cancelled or closed waits', async t => {
  const { manager, configuration, service } = fixture(t); manager.configure(configuration());
  const initial = await manager.watch({ timeoutMs: 0 });
  await assert.rejects(manager.watch({ cursor: { ...initial.cursor, workspaceId: 'another-workspace' } }), /another workspace/);
  const controller = new AbortController(), cancelled = manager.watch({ cursor: initial.cursor, timeoutMs: Number.MAX_SAFE_INTEGER }, controller.signal);
  controller.abort(new DOMException('Human left the host.', 'AbortError'));
  await assert.rejects(cancelled, /Human left/); assert.equal(manager.listeners.size, 0); assert.equal(service.rooms.listeners.size, 0);
  const closing = manager.watch({ cursor: initial.cursor, timeoutMs: 100000000000 }); manager.close();
  await assert.rejects(closing, /shutting down/); assert.equal(manager.listeners.size, 0); assert.equal(service.rooms.listeners.size, 0);
  assert.throws(() => manager.get(), /closed/); manager.close();
});

test('delegate watch supports finite safe timeouts without provider calls or a model wait loop', async t => {
  const { manager } = fixture(t);
  for (const timeoutMs of [-1, 1.5, Number.MAX_SAFE_INTEGER + 1, Infinity]) {
    assert.throws(() => manager.watch({ timeoutMs }), /delegate watch|JSON-compatible/);
  }
  const initial = await manager.watch({ timeoutMs: 0 });
  const timed = await manager.watch({ cursor: initial.cursor, timeoutMs: 5 });
  assert.equal(timed.reason, 'timeout'); assert.deepEqual(timed.cursor, initial.cursor);
});

test('executive settings, reports and watches use metadata without loading conversation history', async t => {
  const { manager, service, room, configuration } = fixture(t);
  for (let index = 0; index < 103; index++) service.rooms.postMessage(room.id, { author: 'Human', body: `Complete retained message ${index}` });
  const fullRead = service.rooms.get;
  service.rooms.get = () => { assert.fail('Executive coordination must not decode a conversation preview.'); };
  try {
    manager.configure(configuration()); const saved = manager.report(report(room));
    assert.equal(saved.observedRooms[0].instructionRevision, 103);
    const snapshot = await manager.watch({ timeoutMs: 0 }); assert.equal(snapshot.rooms[0].instructionRevision, 103);
    assert.equal((await manager.watch({ cursor: snapshot.cursor, timeoutMs: 5 })).reason, 'timeout');
  } finally { service.rooms.get = fullRead; }
});

test('watch setup and snapshot errors release every local subscription', async t => {
  const { manager, service, configuration } = fixture(t); manager.configure(configuration());
  const rooms = service.rooms, subscribe = rooms.subscribe, metadata = rooms.metadata;
  rooms.subscribe = () => { throw new Error('Room subscriptions unavailable.'); };
  try { await assert.rejects(manager.watch({}), /subscriptions unavailable/); }
  finally { rooms.subscribe = subscribe; }
  assert.equal(manager.listeners.size, 0); assert.equal(rooms.listeners.size, 0);
  rooms.metadata = () => { throw new Error('Room storage unavailable.'); };
  try { await assert.rejects(manager.watch({}), /storage unavailable/); }
  finally { rooms.metadata = metadata; }
  assert.equal(manager.listeners.size, 0); assert.equal(rooms.listeners.size, 0);
});

test('watch recovers a tracked foreign owner that exits after its baseline without replaying any work', async t => {
  const { manager, service, room, configuration } = fixture(t);
  service.rooms.addTask(room.id, { title: 'Unfinished task', brief: 'Keep partial evidence if its owner exits.', permission: 'read' });
  const partialArtifact = { agent: 'codex', path: 'retained-worktree', baseCommit: 'retained-base', exportStatus: 'failed', error: 'Partial export before host exit.' };
  service.rooms.store.mutate(room.id, value => {
    value.status = 'running';
    value.tasks[0].status = 'completed'; value.tasks[0].runId = 'retained-completed-run'; value.tasks[0].resultPreview = 'Completed once.'; value.tasks[0].workspaces = [];
    value.tasks[1].status = 'running'; value.tasks[1].runId = 'retained-partial-run'; value.tasks[1].resultPreview = 'Retained partial evidence.'; value.tasks[1].workspaces = [partialArtifact];
  }, { claim: { id: 'foreign-owner', pid: process.pid } });
  manager.configure(configuration()); const baseline = await manager.watch({ timeoutMs: 0 });
  assert.equal(baseline.rooms[0].status, 'running');
  const deadPid = Number(execFileSync(process.execPath, ['-e', 'process.stdout.write(String(process.pid))'], { encoding: 'utf8', windowsHide: true }));
  assert.ok(Number.isSafeInteger(deadPid) && deadPid > 0);
  service.rooms.store.db.prepare('UPDATE rooms SET owner_pid=? WHERE id=?').run(deadPid, room.id);
  const untracked = service.rooms.create({ title: 'Untracked room', brief: 'Do not recover unrelated owners during an executive watch.', members: [{ agent: 'codex', role: 'lead' }], tasks: [] });
  service.rooms.store.mutate(untracked.id, value => { value.status = 'running'; }, { claim: { id: 'untracked-owner', pid: deadPid } });
  const watched = await manager.watch({ cursor: baseline.cursor, timeoutMs: 3000 });
  assert.equal(watched.reason, 'changed'); assert.equal(watched.rooms[0].status, 'failed'); assert.equal(watched.rooms[0].completedTasks, 1);
  assert.match(watched.rooms[0].error, /owning process exited/);
  const recovered = service.rooms.get(room.id);
  assert.deepEqual(recovered.tasks.map(task => task.status), ['completed', 'failed']);
  assert.equal(recovered.tasks[0].runId, 'retained-completed-run'); assert.equal(recovered.tasks[0].resultPreview, 'Completed once.');
  assert.equal(recovered.tasks[1].runId, 'retained-partial-run'); assert.equal(recovered.tasks[1].resultPreview, 'Retained partial evidence.');
  assert.deepEqual(recovered.tasks[1].workspaces, [partialArtifact]);
  assert.equal(service.rooms.metadata(untracked.id).status, 'running', 'The watch only recovers its explicitly tracked rooms.');
  assert.equal(manager.listeners.size, 0); assert.equal(service.rooms.listeners.size, 0);
});
