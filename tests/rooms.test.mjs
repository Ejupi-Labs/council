import test from 'node:test';
import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { existsSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { basename, dirname, join, resolve } from 'node:path';
import { setTimeout as delay } from 'node:timers/promises';
import { CouncilService } from '../dist/service.js';
import { loadConfig } from '../dist/config.js';
import { RoomManager } from '../dist/rooms.js';

const member = (agent, overrides = {}) => ({ agent, role: 'worker', weight: 1, ...overrides });
const task = (title = 'Inspect code', overrides = {}) => ({ title, brief: 'Inspect the assigned project files and report evidence.', permission: 'read', ...overrides });
const input = (overrides = {}) => ({ title: 'Offline project room', brief: 'Coordinate these bounded project-manager tasks.', members: [member('codex'), member('claude')], tasks: [task()], ...overrides });
const complete = (request, output) => ({ agent: request.agent, status: 'completed', output, durationMs: 1 });
function ballotData(request) {
  const match = /ROOM_BALLOT_DATA_BEGIN\n([^\n]+)\nROOM_BALLOT_DATA_END/.exec(request.prompt);
  return match ? JSON.parse(match[1]) : undefined;
}
function approve(data, assignee = data.members[0].agent, overrides = {}) {
  return JSON.stringify({ vote: 'approve', rationale: 'The bounded task has clear acceptance evidence.', assignments: Object.fromEntries(data.tasks.map(task => [task.id, assignee])), ...overrides });
}
function fixture(t, handler, { git = false, ...overrides } = {}) {
  const parent = resolve(tmpdir()), root = mkdtempSync(join(parent, 'council-rooms-'));
  if (git) {
    const invoke = (...args) => execFileSync('git', ['-C', root, ...args], { windowsHide: true, stdio: 'pipe' });
    invoke('init', '--quiet'); invoke('config', 'user.name', 'Offline Room Test'); invoke('config', 'user.email', 'offline@example.invalid');
    invoke('config', 'core.autocrlf', 'false'); invoke('config', 'commit.gpgsign', 'false');
    writeFileSync(join(root, '.gitignore'), '.council/\n'); writeFileSync(join(root, 'base.txt'), 'Original fixture.\n');
    invoke('add', '.'); invoke('commit', '--quiet', '-m', 'offline room fixture');
  }
  const requests = [];
  const runner = { doctor: async () => [], run: async request => {
    requests.push(request);
    if (handler) return await handler(request, requests);
    const data = ballotData(request);
    return complete(request, data ? approve(data) : 'Verified the requested offline task.');
  } };
  const service = new CouncilService(loadConfig(root, { backend: 'mock', maxCalls: 64, maxOutputBytes: 32768, maxConcurrency: 2, ...overrides }), runner);
  const manager = new RoomManager(service), managers = [manager], services = [service];
  t.after(async () => {
    await Promise.all(managers.map(value => value.close())); await Promise.all(services.map(value => value.close()));
    assert.equal(dirname(resolve(root)), parent); assert.ok(basename(root).startsWith('council-rooms-'));
    rmSync(root, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 });
  });
  return { root, service, manager, requests, peer() { const peer = new RoomManager(service); managers.push(peer); return peer; },
    backendPeer(backend, runner) {
      const otherService = new CouncilService({ ...service.config, backend }, runner);
      const other = new RoomManager(otherService); services.push(otherService); managers.push(other); return other;
    } };
}

test('rooms persist tasks and chat, clone returned data and expose explicit history previews', t => {
  const { manager, peer } = fixture(t);
  const room = manager.create(input({ tasks: [] }));
  assert.equal(room.autoExecute, true); assert.equal(room.maxCalls, 64); assert.equal(room.contextLimit, null);
  const first = manager.addTask(room.id, task('First'));
  const second = manager.addTask(room.id, task('Second', { dependsOn: [first.tasks[0].id] }));
  assert.deepEqual(second.tasks[1].dependsOn, ['task-1']);
  room.title = 'Caller mutation'; second.tasks[0].permission = 'write';
  assert.equal(manager.get(room.id).title, 'Offline project room'); assert.equal(manager.get(room.id).tasks[0].permission, 'read');
  for (let index = 0; index < 110; index++) manager.postMessage(room.id, { author: 'PM', body: `Full human message ${index}` });
  const restored = peer().get(room.id);
  assert.equal(restored.messages[0].id, 'history-preview'); assert.match(restored.messages[0].body, /Earlier complete messages/);
  const history = []; let cursor;
  do { const page = manager.messagePage(room.id, 30, cursor); history.push(...page.messages); cursor = page.nextCursor; } while (cursor !== null);
  assert.equal(history.filter(message => message.kind === 'human').length, 110);
  assert.ok(history.some(message => message.body === 'Full human message 0'));
  assert.equal(manager.list()[0].taskCount, 2);
  assert.throws(() => manager.get('unknown'), /404/);
});

test('room input validation rejects invalid members, authority, permissions and cyclic task graphs atomically', t => {
  const { manager } = fixture(t);
  for (const override of [{ members: [member('missing')] }, { members: [member('codex'), member('codex')] },
    { members: [member('codex', { weight: 0 })] }, { members: [member('codex', { weight: Number.POSITIVE_INFINITY })] },
    { members: [member('codex', { advisor: 'claude' }), member('claude')] },
    { members: [member('codex', { role: 'advisor' })] }, { rounds: 0 }, { contextLimit: 0 },
    { tasks: [task('Invalid permission', { permission: 'admin' })] },
    { tasks: [task('Cycle1', { dependsOn: ['task-2'] }), task('Cycle2', { dependsOn: ['task-1'] })] }]) {
    assert.throws(() => manager.create(input(override)));
  }
  const room = manager.create(input());
  assert.throws(() => manager.addTask(room.id, task('Unknown dependency', { dependsOn: ['missing'] })), /dependency/);
  assert.equal(manager.get(room.id).revision, room.revision); assert.equal(manager.get(room.id).tasks.length, 1);
});

test('weighted approval preserves dissent and selects the heaviest approved assignment without changing permissions', async t => {
  const { manager, requests } = fixture(t, async request => {
    const data = ballotData(request);
    return complete(request, approve(data, request.agent, request.agent === 'junie' ? { vote: 'revise', rationale: 'Additional acceptance evidence is needed.' } : {}));
  });
  const room = manager.create(input({ autoExecute: false, members: [member('codex', { role: 'lead', weight: 1 }), member('claude', { weight: 4 }), member('junie', { weight: 2 })] }));
  assert.equal(manager.startDiscussion(room.id).status, 'discussing');
  const ready = await manager.wait(room.id);
  assert.equal(ready.status, 'ready'); assert.equal(ready.decision.approveWeight, 5); assert.equal(ready.decision.totalWeight, 7);
  assert.equal(ready.tasks[0].assignee, 'claude'); assert.equal(ready.tasks[0].permission, 'read');
  assert.ok(ready.decision.dissent.some(value => value.includes('junie') && value.includes('Additional acceptance')));
  assert.ok(requests.every(request => request.permission === 'read')); assert.equal(ready.callsUsed, 3);
});

test('a mathematically tied decimal approval weight never authorizes execution', async t => {
  const { manager, requests } = fixture(t, async request => complete(request, approve(ballotData(request), 'codex', request.agent === 'junie' ? { vote: 'revise' } : {})));
  const room = manager.create(input({ members: [member('codex', { weight: 0.1 }), member('claude', { weight: 0.2 }), member('junie', { weight: 0.3 })] }));
  manager.startDiscussion(room.id); const result = await manager.wait(room.id);
  assert.equal(result.status, 'needs_attention'); assert.equal(result.decision.approved, false); assert.equal(requests.length, 3);
  assert.ok(result.tasks.every(task => task.status === 'pending'));
});

test('equal-weight assignment ties prefer the lead and then original member order', async t => {
  const { manager } = fixture(t, async request => complete(request, approve(ballotData(request), request.agent)));
  for (const members of [[member('claude'), member('codex', { role: 'lead' }), member('junie')], [member('claude'), member('codex'), member('junie')]]) {
    const room = manager.create(input({ members, autoExecute: false })); manager.startDiscussion(room.id);
    const ready = await manager.wait(room.id);
    assert.equal(ready.tasks[0].assignee, members.some(member => member.role === 'lead') ? 'codex' : 'claude');
  }
});

test('malformed, partial or unauthorized ballots require attention and never dispatch tasks', async t => {
  const invalid = [() => 'prose {"vote":"approve"}', () => '```json\n{"vote":"approve"}\n```',
    data => approve(data, 'codex', { extra: true }), data => approve(data, 'codex', { assignments: {} }),
    data => approve(data, 'not-a-member'), data => approve(data, 'codex', { assignments: { missing: 'codex' } }),
    () => 'null', () => '{"vote":"approve","rationale":"Missing closing document"'];
  for (const output of invalid) {
    const { manager, requests } = fixture(t, async request => complete(request, output(ballotData(request))));
    const room = manager.create(input({ members: [member('codex')] })); manager.startDiscussion(room.id);
    const result = await manager.wait(room.id);
    assert.equal(result.status, 'needs_attention'); assert.equal(requests.length, 1); assert.equal(result.tasks[0].status, 'pending');
  }
});

test('failed discussion calls preserve partial opinions without approving a partial room result', async t => {
  const { manager, requests } = fixture(t, async request => request.agent === 'claude'
    ? { ...complete(request, 'Partial provider evidence.'), status: 'failed', error: 'Offline failure.' }
    : complete(request, approve(ballotData(request))));
  const room = manager.create(input()); manager.startDiscussion(room.id); const result = await manager.wait(room.id);
  assert.equal(result.status, 'needs_attention'); assert.equal(requests.length, 2); assert.equal(result.decision, undefined);
  assert.ok(result.messages.some(message => message.body.includes('Partial provider evidence.')));
});

test('discussion rounds stop on approval and automatically execute the project-manager task', async t => {
  let taskCalls = 0;
  const { manager, requests } = fixture(t, async request => {
    const data = ballotData(request);
    if (!data) { taskCalls++; return complete(request, 'Verified task completion.'); }
    return complete(request, '```json\n' + approve(data, 'codex', data.round === 1 ? { vote: 'revise' } : {}) + '\n```');
  });
  const room = manager.create(input({ rounds: 3, maxCalls: 7 })); manager.startDiscussion(room.id);
  const result = await manager.wait(room.id);
  assert.equal(result.status, 'completed'); assert.equal(result.tasks[0].status, 'completed'); assert.equal(taskCalls, 1);
  assert.equal(result.callsUsed, 5); assert.equal(requests.length, 5);
  assert.ok(result.messages.some(message => message.kind === 'opinion' && message.body.includes('revise')));
});

test('a conservative room budget includes potential advisor and worker-resume calls before discussion', async t => {
  const { manager, requests } = fixture(t);
  const room = manager.create(input({ members: [member('codex', { advisor: 'junie' }), member('junie', { role: 'advisor' })], maxCalls: 3 }));
  assert.throws(() => manager.startDiscussion(room.id), /call budget/);
  assert.equal(requests.length, 0); assert.equal(manager.get(room.id).status, 'idle');
  const largerQuota = manager.create(input({ members: [member('codex', { advisor: 'junie', advisorMaxRequests: 3 }), member('junie', { role: 'advisor' })], maxCalls: 7 }));
  assert.throws(() => manager.startDiscussion(largerQuota.id), /requires up to 8 further calls/);
  assert.equal(requests.length, 0); assert.equal(manager.get(largerQuota.id).status, 'idle');
});

test('room task advisors inherit their own settings and remain read-only while the worker resumes', async t => {
  let requested = false;
  const { manager, requests } = fixture(t, async request => {
    const data = ballotData(request);
    if (data) return complete(request, approve(data));
    if (request.agent === 'codex' && !requested) { requested = true; return complete(request, JSON.stringify({ council_advisor_request: { question: 'Check the acceptance evidence.', context: 'Offline fixture evidence.' } })); }
    return complete(request, request.agent === 'junie' ? 'Independent advisor evidence.' : 'Worker verified the final result.');
  });
  const room = manager.create(input({ members: [member('codex', { model: 'worker-model', effort: 'high', advisor: 'junie' }), member('junie', { role: 'advisor', model: 'advisor-model', effort: 'medium' })], maxCalls: 4 }));
  manager.startDiscussion(room.id); const result = await manager.wait(room.id);
  assert.equal(result.status, 'completed'); assert.equal(result.callsUsed, 4); assert.equal(requests.length, 4);
  assert.ok(requests.filter(request => request.agent === 'codex').every(request => request.model === 'worker-model' && request.effort === 'high'));
  const advisor = requests.find(request => request.agent === 'junie'); assert.equal(advisor.permission, 'read'); assert.equal(advisor.model, 'advisor-model');
  assert.ok(result.messages.some(message => message.kind === 'advisor' && message.body.includes('Independent advisor')));
});

test('task execution respects dependency order, parallel slots and original read/write permissions', { timeout: 60000 }, async t => {
  const events = []; let running = 0, peak = 0, writerStarted;
  const writerReady = new Promise(resolve => { writerStarted = resolve; });
  const { manager, requests } = fixture(t, async request => {
    const data = ballotData(request); if (data) return complete(request, approve(data));
    const id = /Room task (task-\d+)/.exec(request.prompt)[1]; events.push(`start:${id}`); running++; peak = Math.max(peak, running);
    if (id === 'task-1') writerStarted();
    if (id === 'task-2') await writerReady;
    await delay(15); running--; events.push(`end:${id}`); return complete(request, 'x'.repeat(4000));
  }, { git: true });
  const room = manager.create(input({ members: [member('codex')], tasks: [task('Writer', { permission: 'write' }), task('Independent reader'), task('Dependent reader', { dependsOn: ['task-1', 'task-2'] })] }));
  manager.startDiscussion(room.id); const result = await manager.wait(room.id);
  assert.equal(result.status, 'completed'); assert.equal(peak, 2);
  assert.ok(events.indexOf('start:task-3') > events.indexOf('end:task-1')); assert.ok(events.indexOf('start:task-3') > events.indexOf('end:task-2'));
  const writer = requests.find(request => request.permission === 'write'); assert.ok(writer.cwd.includes('worktrees'));
  assert.ok(result.tasks.every(task => task.resultPreview.length <= 2000));
  assert.ok(result.tasks.every(task => task.resultPreview.includes('Preview truncated')));
});

test('failed tasks leave the room failed and never dispatch their dependent tasks', async t => {
  let taskCalls = 0;
  const { manager } = fixture(t, async request => {
    const data = ballotData(request); if (data) return complete(request, approve(data));
    taskCalls++; return { ...complete(request, 'Partial task evidence.'), status: 'failed', error: 'Offline task failure.' };
  });
  const room = manager.create(input({ members: [member('codex')], tasks: [task(), task('Downstream', { dependsOn: ['task-1'] })] }));
  manager.startDiscussion(room.id); const result = await manager.wait(room.id);
  assert.equal(result.status, 'failed'); assert.equal(taskCalls, 1); assert.equal(result.tasks[0].status, 'failed'); assert.equal(result.tasks[1].status, 'cancelled');
});

test('separate managers cannot start the same room twice and foreign cancellation drains owned calls', async t => {
  let entered;
  const started = new Promise(resolve => { entered = resolve; });
  const { manager, peer } = fixture(t, async request => {
    entered(); await new Promise(resolve => request.signal.addEventListener('abort', resolve, { once: true }));
    return { ...complete(request, 'Cancelled partial evidence.'), status: 'cancelled' };
  });
  const other = peer(), room = manager.create(input({ members: [member('codex')] }));
  manager.startDiscussion(room.id); assert.throws(() => other.startDiscussion(room.id), /current state|active/);
  assert.throws(() => other.addTask(room.id, task('Too late')), /active/);
  await started; other.cancel(room.id); const result = await manager.wait(room.id);
  assert.equal(result.status, 'cancelled'); assert.ok(result.tasks.every(task => task.status === 'cancelled'));
});

test('room close cancels its own work without closing the shared workflow service', async t => {
  let entered;
  const started = new Promise(resolve => { entered = resolve; });
  const { manager, service } = fixture(t, async request => {
    if (!ballotData(request)) return complete(request, 'Shared service is still available.');
    entered(); await new Promise(resolve => request.signal.addEventListener('abort', resolve, { once: true }));
    return { ...complete(request, 'Stopped.'), status: 'cancelled' };
  });
  const room = manager.create(input({ members: [member('codex')] })); manager.startDiscussion(room.id); await started; await manager.close();
  const run = await service.run({ mode: 'delegate', agents: ['codex'], permission: 'read', task: 'Verify the service remains open.' });
  assert.equal(run.status, 'completed');
});

test('room persistence faults remain failed and retain an emergency snapshot without re-reading unavailable state', async t => {
  let finish, entered;
  const started = new Promise(resolve => { entered = resolve; });
  const { manager } = fixture(t, async request => { entered(); await new Promise(resolve => { finish = resolve; }); return complete(request, approve(ballotData(request))); });
  const room = manager.create(input({ members: [member('codex')] })); manager.startDiscussion(room.id); await started;
  const nativeMutate = manager.store.mutate;
  manager.store.mutate = () => { throw new Error('Injected unavailable room persistence.'); };
  finish();
  const result = await manager.wait(room.id);
  assert.equal(result.status, 'failed'); assert.match(result.error, /persistence failed/);
  assert.throws(() => manager.create(input()), /persistence failed/);
  manager.store.mutate = nativeMutate;
});

test('full room context retains old history and complete briefs and rationales beyond preview lengths', async t => {
  const rationale = 'Complete evidence. '.repeat(250);
  const { manager, requests } = fixture(t, async request => complete(request, approve(ballotData(request), 'codex', { rationale })));
  const room = manager.create(input({ members: [member('codex')], autoExecute: false, brief: 'B'.repeat(9000) + 'FULL_ROOM_BRIEF_END',
    tasks: [task('Full brief', { brief: 'T'.repeat(10000) + 'FULL_TASK_BRIEF_END' })] }));
  manager.postMessage(room.id, { author: 'PM', body: 'EARLY_HISTORY_SENTINEL' });
  for (let index = 0; index < 110; index++) manager.postMessage(room.id, { author: 'PM', body: `Retained historical message ${index}.` });
  manager.startDiscussion(room.id); const result = await manager.wait(room.id);
  assert.equal(result.status, 'ready');
  assert.ok(requests[0].prompt.includes('EARLY_HISTORY_SENTINEL')); assert.ok(requests[0].prompt.includes('FULL_ROOM_BRIEF_END'));
  assert.ok(requests[0].prompt.includes('FULL_TASK_BRIEF_END')); assert.equal(result.decision.rationale, rationale);
});

test('an explicit insufficient context budget fails before any model call instead of clipping evidence', t => {
  const { manager, requests } = fixture(t);
  const room = manager.create(input({ contextLimit: 1000, brief: 'Full evidence '.repeat(200) }));
  assert.throws(() => manager.startDiscussion(room.id), /increase the context budget/);
  assert.equal(requests.length, 0); assert.equal(manager.get(room.id).status, 'idle');
});

test('closing a foreign room waiter interrupts its timer without cancelling the owning manager', async t => {
  let entered;
  const started = new Promise(resolve => { entered = resolve; });
  const { manager, peer } = fixture(t, async request => {
    entered(); await new Promise(resolve => request.signal.addEventListener('abort', resolve, { once: true }));
    return { ...complete(request, 'Cancelled fixture.'), status: 'cancelled' };
  });
  const other = peer(), room = manager.create(input({ members: [member('codex')] })); manager.startDiscussion(room.id); await started;
  const waiting = other.wait(room.id); const rejected = assert.rejects(waiting, error => error.name === 'AbortError');
  await other.close(); await rejected;
  assert.equal(manager.get(room.id).status, 'discussing');
  manager.cancel(room.id); assert.equal((await manager.wait(room.id)).status, 'cancelled');
});

test('persisted room backend provenance prevents mock rooms from dispatching through a native service', async t => {
  const { manager, backendPeer, requests } = fixture(t);
  let nativeCalls = 0;
  const native = backendPeer('native', { doctor: async () => [], run: async request => { nativeCalls++; return complete(request, 'This runner must never be called.'); } });
  const room = manager.create(input({ members: [member('codex')], autoExecute: false }));
  assert.equal(room.backend, 'mock'); assert.equal(native.get(room.id).backend, 'mock');
  assert.throws(() => native.startDiscussion(room.id), /backend mock differs.*backend native/);
  assert.equal(manager.get(room.id).status, 'idle');
  manager.startDiscussion(room.id); assert.equal((await manager.wait(room.id)).status, 'ready');
  assert.throws(() => native.execute(room.id), /backend mock differs.*backend native/);
  manager.execute(room.id); assert.equal((await manager.wait(room.id)).status, 'completed');
  assert.equal(nativeCalls, 0); assert.equal(requests.length, 2);
  const legacy = manager.create(input({ members: [member('codex')] }));
  manager.store.mutate(legacy.id, stored => { delete stored.backend; });
  assert.throws(() => manager.startDiscussion(legacy.id), /provenance is unknown.*Recreate/);
  assert.equal(requests.length, 2);
});

test('room weights inherit configured agent authority only when the PM omits an explicit weight', t => {
  const { manager } = fixture(t, undefined, { agents: {
    codex: { enabled: true, profile: 'codex', authority: 7 }, claude: { enabled: true, profile: 'claude', authority: 3 },
  } });
  const room = manager.create(input({ members: [{ agent: 'codex', role: 'lead' }, { agent: 'claude', role: 'worker', weight: 0.5 }] }));
  assert.deepEqual(room.members.map(member => member.weight), [7, 0.5]);
  assert.deepEqual(manager.get(room.id).members.map(member => member.weight), [7, 0.5]);
});

test('PM guidance arriving during discussion preserves opinions but prevents stale approval and automatic execution', async t => {
  let entered, finish;
  const started = new Promise(resolve => { entered = resolve; }), gate = new Promise(resolve => { finish = resolve; });
  const { manager, requests, peer } = fixture(t, async request => {
    const data = ballotData(request);
    if (data) { entered(); await gate; return complete(request, approve(data)); }
    return complete(request, 'This stale task must never start.');
  });
  const room = manager.create(input({ members: [member('codex')] })); manager.startDiscussion(room.id); await started;
  peer().postMessage(room.id, { author: 'PM', body: 'The acceptance criteria changed while the vote was in progress.' });
  finish(); const result = await manager.wait(room.id);
  assert.equal(result.status, 'needs_attention'); assert.equal(result.decision, undefined); assert.equal(result.tasks[0].status, 'pending');
  assert.match(result.error, /instructions changed/); assert.ok(result.messages.some(message => message.kind === 'opinion'));
  assert.equal(requests.length, 1); assert.equal(result.instructionRevision, 1);
});

test('PM guidance invalidates a ready plan and requires a new vote before explicit execution', async t => {
  const { manager, requests } = fixture(t);
  const room = manager.create(input({ members: [member('codex')], autoExecute: false })); manager.startDiscussion(room.id);
  const first = await manager.wait(room.id); assert.equal(first.status, 'ready'); assert.equal(first.decision.instructionRevision, 0);
  const changed = manager.postMessage(room.id, { author: 'PM', body: 'Use the updated verification criteria.' });
  assert.equal(changed.status, 'needs_attention'); assert.equal(changed.decision, undefined); assert.equal(changed.tasks[0].status, 'pending');
  assert.throws(() => manager.execute(room.id), /approved ready room/); assert.equal(requests.length, 1);
  manager.startDiscussion(room.id); const approved = await manager.wait(room.id);
  assert.equal(approved.decision.instructionRevision, 1); manager.execute(room.id);
  assert.equal((await manager.wait(room.id)).status, 'completed'); assert.equal(requests.length, 3);
});

test('PM guidance during execution drains an active task and stops further task dispatch without replaying writes', async t => {
  let entered, finish, activeSignal;
  const started = new Promise(resolve => { entered = resolve; }), gate = new Promise(resolve => { finish = resolve; });
  const { manager, requests, peer } = fixture(t, async request => {
    const data = ballotData(request); if (data) return complete(request, approve(data));
    activeSignal = request.signal; entered(); await gate; return complete(request, 'Completed the task under the original instructions.');
  }, { maxConcurrency: 1 });
  const room = manager.create(input({ members: [member('codex')], tasks: [task('Active task'), task('Not dispatched'), task('Also not dispatched')] }));
  manager.startDiscussion(room.id); await started;
  const updated = peer().postMessage(room.id, { author: 'PM', body: 'Stop after the in-flight task and revise the remaining scope.' });
  assert.equal(updated.status, 'running'); assert.equal(activeSignal.aborted, false);
  finish(); const result = await manager.wait(room.id);
  assert.equal(result.status, 'needs_attention'); assert.equal(result.decision, undefined); assert.equal(activeSignal.aborted, false);
  assert.deepEqual(result.tasks.map(task => task.status), ['completed', 'pending', 'pending']); assert.equal(requests.length, 2);
  assert.ok(result.tasks[0].runId); assert.equal(result.tasks[1].runId, undefined);
});

test('dispatch admission and provider queueing are atomic and a synchronous start failure rolls back the room reservation', async t => {
  const { manager, service, requests } = fixture(t);
  const nativeStart = service.start.bind(service);
  service.start = value => {
    if (value.mode === 'delegate') {
      assert.equal(manager.store.db.isTransaction, true, 'Provider queueing must remain inside the instruction-guarded ownership transaction.');
      throw new Error('Injected synchronous dispatch failure.');
    }
    return nativeStart(value);
  };
  const room = manager.create(input({ members: [member('codex')] })); manager.startDiscussion(room.id);
  const result = await manager.wait(room.id);
  assert.equal(result.status, 'failed'); assert.match(result.error, /Injected synchronous dispatch failure/);
  assert.equal(result.callsUsed, 1); assert.equal(result.tasks[0].runId, undefined); assert.equal(requests.length, 1);
});

test('a foreign PM update immediately before dispatch is rejected by the transactional instruction guard', async t => {
  const { manager, peer, requests } = fixture(t);
  const other = peer(), room = manager.create(input({ members: [member('codex')] }));
  const nativeMutate = manager.store.mutate.bind(manager.store);
  let updated = false;
  manager.store.mutate = (id, change, options) => {
    if (!updated && options?.owner && manager.get(id).status === 'running') {
      updated = true;
      other.postMessage(id, { author: 'PM', body: 'Foreign guidance accepted just before dispatch admission.' });
    }
    return nativeMutate(id, change, options);
  };
  manager.startDiscussion(room.id); const result = await manager.wait(room.id);
  assert.equal(updated, true); assert.equal(result.status, 'needs_attention'); assert.equal(result.callsUsed, 1);
  assert.equal(result.tasks[0].runId, undefined); assert.equal(requests.length, 1);
  assert.ok(result.messages.some(message => message.body.includes('Foreign guidance accepted just before dispatch')));
});

test('failed or cancelled rooms reject new tasks and unavailable prerequisites fail before discussion calls', t => {
  const { manager, requests } = fixture(t);
  for (const status of ['failed', 'cancelled']) {
    const room = manager.create(input()); manager.store.mutate(room.id, stored => { stored.status = status; });
    assert.throws(() => manager.addTask(room.id, task('Cannot revive this room')), /Failed or cancelled rooms/);
    assert.equal(manager.get(room.id).status, status);
  }
  for (const status of ['failed', 'cancelled']) {
    const room = manager.create(input({ tasks: [task('Prerequisite'), task('Blocked task', { dependsOn: ['task-1'] })] }));
    manager.store.mutate(room.id, stored => { stored.status = 'needs_attention'; stored.tasks[0].status = status; });
    assert.throws(() => manager.startDiscussion(room.id), /unavailable prerequisite/);
  }
  assert.equal(requests.length, 0);
});

test('opinion persistence failures are reported as room failures rather than invalid model ballots', async t => {
  const { manager } = fixture(t);
  const nativeMutate = manager.store.mutate.bind(manager.store);
  let injected = false;
  manager.store.mutate = (id, change, options) => nativeMutate(id, room => {
    change(room);
    if (!injected && room.messages.at(-1)?.kind === 'opinion') { injected = true; throw new Error('Injected opinion persistence failure.'); }
  }, options);
  const room = manager.create(input({ members: [member('codex')] })); manager.startDiscussion(room.id);
  const result = await manager.wait(room.id);
  assert.equal(injected, true); assert.equal(result.status, 'failed'); assert.match(result.error, /Injected opinion persistence failure/);
  assert.equal(result.messages.some(message => message.body.includes('Invalid ballot')), false);
});

test('accepted cancellation remains cancellation while slow owned work drains across multiple polling ticks', async t => {
  let entered;
  const started = new Promise(resolve => { entered = resolve; });
  const { manager } = fixture(t, async request => {
    entered(); await new Promise(resolve => request.signal.addEventListener('abort', resolve, { once: true }));
    await delay(250); return { ...complete(request, 'Retained cancelled evidence.'), status: 'cancelled' };
  });
  const room = manager.create(input({ members: [member('codex')] })); manager.startDiscussion(room.id); await started;
  manager.cancel(room.id); const result = await manager.wait(room.id);
  assert.equal(result.status, 'cancelled');
  assert.ok(result.messages.some(message => message.body.includes('Room cancelled; partial results and isolated worktrees are retained.')));
});

test('positive finite hierarchy weights have no artificial range and scientific decimal ties never approve', async t => {
  const { manager, requests } = fixture(t, async request => complete(request, approve(ballotData(request), 'codex', request.agent === 'junie' ? { vote: 'revise' } : {})));
  for (const weights of [[1e-20, 2e-20, 3e-20], [5e-324, 1e-323, 1.5e-323]]) {
    const room = manager.create(input({ members: [member('codex', { weight: weights[0] }), member('claude', { weight: weights[1] }), member('junie', { weight: weights[2] })] }));
    manager.startDiscussion(room.id); const result = await manager.wait(room.id);
    assert.equal(result.status, 'needs_attention'); assert.equal(result.decision.approved, false);
    assert.ok(result.tasks.every(task => task.status === 'pending'));
  }
  const large = manager.create(input({ members: [member('codex', { weight: 1000000 })] }));
  assert.equal(large.members[0].weight, 1000000);
  assert.equal(requests.length, 6);
});

test('overflowing hierarchy totals retain exact scientific strings and a correct weighted majority', async t => {
  const { manager } = fixture(t, async request => {
    const data = ballotData(request), revise = request.agent === (data.members.length === 2 ? 'claude' : 'junie');
    return complete(request, approve(data, 'codex', revise ? { vote: 'revise' } : {}));
  });
  const room = manager.create(input({ autoExecute: false, members: [member('codex', { weight: 1e308 }), member('claude', { weight: 1e308 }), member('junie', { weight: 1e308 })] }));
  manager.startDiscussion(room.id); const result = await manager.wait(room.id);
  assert.equal(result.status, 'ready'); assert.equal(result.decision.approved, true);
  assert.equal(result.decision.approveWeight, '2e308'); assert.equal(result.decision.totalWeight, '3e308');
  assert.deepEqual(manager.get(room.id).decision, result.decision);
  assert.equal(JSON.stringify(result.decision).includes('null'), false);
  const tied = manager.create(input({ members: [member('codex', { weight: 1e308 }), member('claude', { weight: 1e308 })] }));
  manager.startDiscussion(tied.id); const tie = await manager.wait(tied.id);
  assert.equal(tie.status, 'needs_attention'); assert.equal(tie.decision.approved, false);
  assert.equal(tie.decision.approveWeight, 1e308); assert.equal(tie.decision.totalWeight, '2e308');
});

test('room call refunds count failed advisor calls exactly near the largest safe integer budget', async t => {
  const { manager, requests } = fixture(t, async request => {
    const data = ballotData(request); if (data) return complete(request, approve(data));
    if (request.agent === 'junie') return { ...complete(request, 'Partial advisor evidence.'), status: 'failed', error: 'Offline advisor failure.' };
    return complete(request, JSON.stringify({ council_advisor_request: { question: 'Verify the arithmetic evidence.', context: 'Offline fixture.' } }));
  });
  const maximum = Number.MAX_SAFE_INTEGER;
  const room = manager.create(input({ maxCalls: maximum, members: [member('codex', { advisor: 'junie' }), member('junie', { role: 'advisor' })] }));
  manager.store.mutate(room.id, stored => { stored.callsUsed = maximum - 4; });
  manager.startDiscussion(room.id); const result = await manager.wait(room.id);
  assert.equal(result.status, 'failed'); assert.equal(requests.length, 3);
  assert.equal(result.callsUsed, maximum - 1);
});

test('cancellation still succeeds when a foreign discussion becomes ready immediately before the atomic request', t => {
  const { manager, peer, requests } = fixture(t);
  const other = peer(), room = manager.create(input({ members: [member('codex')], autoExecute: false }));
  manager.store.mutate(room.id, current => { current.status = 'discussing'; }, { claim: { id: 'foreign-owner', pid: process.pid } });
  const requestCancel = manager.store.requestCancel.bind(manager.store);
  manager.store.requestCancel = (id, options) => {
    other.store.mutate(id, current => { current.status = 'ready'; current.tasks[0].status = 'assigned'; }, { owner: 'foreign-owner', release: true });
    return requestCancel(id, options);
  };
  const result = manager.cancel(room.id);
  assert.equal(result.status, 'cancelled'); assert.equal(other.get(room.id).status, 'cancelled');
  assert.equal(result.tasks[0].status, 'cancelled'); assert.equal(requests.length, 0);
});

test('closing repairs a failed emergency room after storage recovers without losing newer foreign PM guidance', async t => {
  let entered, finish;
  const started = new Promise(resolve => { entered = resolve; }), gate = new Promise(resolve => { finish = resolve; });
  const { manager, peer, service, requests } = fixture(t, async request => {
    entered(); await gate; return complete(request, approve(ballotData(request)));
  });
  const room = manager.create(input({ members: [member('codex')] })); manager.startDiscussion(room.id); await started;
  const nativeMutate = manager.store.mutate.bind(manager.store);
  manager.store.mutate = () => { throw new Error('Injected temporary room storage outage.'); };
  finish(); const emergency = await manager.wait(room.id); assert.equal(emergency.status, 'failed');
  manager.store.mutate = nativeMutate;
  const foreign = peer(); foreign.postMessage(room.id, { author: 'PM', body: 'FOREIGN_GUIDANCE_AFTER_EMERGENCY' });
  let retries = 0;
  manager.store.mutate = (id, change, options) => {
    if (options?.preserveFailure && ++retries <= 2) throw Object.assign(new Error('Injected transient repair contention.'), { errcode: 5 });
    return nativeMutate(id, change, options);
  };
  await manager.close();
  const reopened = peer(), recovered = await reopened.wait(room.id, 20);
  assert.equal(retries, 3); assert.equal(recovered.status, 'failed'); assert.equal(recovered.instructionRevision, 1);
  assert.ok(recovered.messages.some(message => message.body === 'FOREIGN_GUIDANCE_AFTER_EMERGENCY'));
  const runId = recovered.messages.find(message => message.runId)?.runId;
  assert.ok(runId); assert.equal(service.getRun(runId).status, 'completed'); assert.equal(requests.length, 1);
  assert.match(recovered.error, /persistence failed/);
});

test('persistent emergency storage failure rejects close explicitly while still closing the room database', async t => {
  let entered, finish;
  const started = new Promise(resolve => { entered = resolve; }), gate = new Promise(resolve => { finish = resolve; });
  const { manager } = fixture(t, async request => { entered(); await gate; return complete(request, approve(ballotData(request))); });
  const room = manager.create(input({ members: [member('codex')] })); manager.startDiscussion(room.id); await started;
  const nativeMutate = manager.store.mutate;
  manager.store.mutate = () => { throw new Error('Injected persistent room storage outage.'); };
  finish(); assert.equal((await manager.wait(room.id)).status, 'failed');
  try { await assert.rejects(manager.close(), /could not be durably finalized during shutdown/); }
  finally {
    // The expected rejected close is already observed; the fixture still closes the shared service.
    manager.close = async () => {}; manager.store.mutate = nativeMutate;
  }
  assert.throws(() => manager.store.db.prepare('SELECT 1'), /not open|closed/i);
});

test('contradictory duplicate ballot keys never authorize execution and the full raw ballot remains available', async t => {
  const duplicate = '{"vote":"revise","rationale":"Constraints remain unmet.","assignments":{"task-1":"codex"},"vote":"approve"}';
  const escaped = String.raw`{"vote":"revise","rationale":"Constraints remain unmet.","assignments":{"task-1":"codex"},"\u0076ote":"approve"}`;
  const assignment = String.raw`{"vote":"approve","rationale":"An ambiguous assignment.","assignments":{"task-1":"missing","\u0074ask-1":"codex"}}`;
  for (const output of [duplicate, '```json\n' + duplicate + '\n```', escaped, assignment]) {
    const { manager, service, requests } = fixture(t, async request => complete(request, output));
    const room = manager.create(input({ members: [member('codex')] })); manager.startDiscussion(room.id);
    const result = await manager.wait(room.id);
    assert.equal(result.status, 'needs_attention'); assert.equal(requests.length, 1); assert.equal(result.tasks[0].status, 'pending');
    assert.match(result.error, /duplicate JSON object keys/);
    const opinion = result.messages.find(message => message.kind === 'opinion');
    assert.ok(opinion); assert.equal(service.getRun(opinion.runId).steps[0].result.output, output);
  }
});

test('quoted JSON-looking rationale text is ordinary ballot evidence and does not create duplicate keys', async t => {
  const rationale = 'The quoted data was {"vote":"revise","vote":"approve"}; escaped backslashes \\ are retained.';
  const { manager } = fixture(t, async request => complete(request, approve(ballotData(request), 'codex', { rationale })));
  const room = manager.create(input({ members: [member('codex')], autoExecute: false })); manager.startDiscussion(room.id);
  const result = await manager.wait(room.id);
  assert.equal(result.status, 'ready'); assert.equal(result.decision.rationale, rationale);
});

test('dependent tasks receive exact retained artifacts and the complete prerequisite report once without assuming a merge', { timeout: 60000 }, async t => {
  const output = 'PREREQUISITE_FULL_BEGIN ' + 'verified evidence '.repeat(600) + ' PREREQUISITE_FULL_END';
  let producerCwd, handoff;
  const { manager, service, requests, root, peer } = fixture(t, async request => {
    const data = ballotData(request); if (data) return complete(request, approve(data));
    if (/Room task task-1:/.test(request.prompt)) {
      producerCwd = request.cwd; writeFileSync(join(request.cwd, 'prerequisite.txt'), 'Retained implementation evidence.\n');
      return complete(request, output);
    }
    handoff = JSON.parse(request.prompt.split('\n').find(line => line.startsWith('{"task":"task-1"')));
    assert.equal(handoff.workflowRecordAvailable, true);
    assert.equal(handoff.fullResultResource, `council://runs/${handoff.runId}`);
    assert.deepEqual(handoff.workspaces, service.getRun(handoff.runId).workspaces);
    assert.equal(handoff.workspaces[0].path, producerCwd); assert.equal(handoff.workspaces[0].exportStatus, 'ready');
    assert.match(readFileSync(handoff.workspaces[0].patchPath, 'utf8'), /Retained implementation evidence/);
    const history = request.prompt.split('\n').filter(line => line.startsWith('{"author":')).map(line => JSON.parse(line));
    const reports = history.filter(message => message.kind === 'task' && message.body.includes('PREREQUISITE_FULL_BEGIN'));
    assert.equal(reports.length, 1); assert.equal(reports[0].body, 'task-1: completed\n' + output); assert.equal(reports[0].runId, handoff.runId);
    assert.equal(request.prompt.split('PREREQUISITE_FULL_END').length - 1, 1);
    assert.equal(request.permission, 'read'); assert.equal(request.cwd, root); assert.equal(existsSync(join(request.cwd, 'prerequisite.txt')), false);
    assert.match(request.prompt, /never merged into your current workspace automatically/);
    assert.match(request.prompt, /If a sandbox or file policy blocks access/);
    return complete(request, 'Reviewed the retained patch; the original checkout remains separate.');
  }, { git: true });
  const room = manager.create(input({ members: [member('codex')], tasks: [task('Produce retained files', { permission: 'write' }), task('Review predecessor evidence', { dependsOn: ['task-1'] })] }));
  manager.startDiscussion(room.id); const result = await manager.wait(room.id);
  assert.equal(result.status, 'completed'); assert.equal(requests.length, 3); assert.ok(handoff);
  assert.deepEqual(result.tasks[0].workspaces, service.getRun(result.tasks[0].runId).workspaces);
  assert.deepEqual(peer().get(room.id).tasks[0].workspaces, result.tasks[0].workspaces);
  assert.equal(existsSync(join(root, 'prerequisite.txt')), false); assert.ok(result.tasks[0].resultPreview.includes('Preview truncated'));
});

test('failed prerequisite exports retain their exact artifact diagnostics and never dispatch dependent tasks', { timeout: 60000 }, async t => {
  let implementationCalls = 0;
  const { manager, service, peer } = fixture(t, async request => {
    const data = ballotData(request); if (data) return complete(request, approve(data));
    implementationCalls++; writeFileSync(join(request.cwd, 'unexported.txt'), 'Retained partial write.\n'); return complete(request, 'The fake writer finished its edit.');
  }, { git: true });
  service.engine.workspaces.export = async () => { throw new Error('Injected prerequisite patch export failure.'); };
  const room = manager.create(input({ members: [member('codex')], tasks: [task('Writer', { permission: 'write' }), task('Blocked follower', { dependsOn: ['task-1'] })] }));
  manager.startDiscussion(room.id); const result = await manager.wait(room.id);
  assert.equal(result.status, 'failed'); assert.equal(implementationCalls, 1); assert.equal(result.tasks[0].status, 'failed');
  assert.equal(result.tasks[1].runId, undefined); assert.equal(result.tasks[1].status, 'cancelled');
  const artifacts = service.getRun(result.tasks[0].runId).workspaces;
  assert.deepEqual(result.tasks[0].workspaces, artifacts); assert.equal(artifacts[0].exportStatus, 'failed');
  assert.match(artifacts[0].exportError, /Injected prerequisite patch export failure/);
  assert.deepEqual(peer().get(room.id).tasks[0].workspaces, artifacts);
});

test('pruned read-only workflow records retain full room dependency reports and explicitly report record availability', async t => {
  const output = 'PRUNED_FULL_BEGIN ' + 'historical report '.repeat(300) + ' PRUNED_FULL_END';
  let followerEvidence;
  const { manager, service } = fixture(t, async request => {
    const data = ballotData(request); if (data) return complete(request, approve(data));
    if (/Room task task-1:/.test(request.prompt)) return complete(request, output);
    followerEvidence = JSON.parse(request.prompt.split('\n').find(line => line.startsWith('{"task":"task-1"')));
    const report = request.prompt.split('\n').filter(line => line.startsWith('{"author":')).map(line => JSON.parse(line)).find(message => message.kind === 'task' && message.runId === followerEvidence.runId && message.body.includes('PRUNED_FULL_BEGIN'));
    assert.equal(report.body, 'task-1: completed\n' + output);
    return complete(request, 'Used the durable room report without assuming the pruned workflow still exists.');
  });
  const room = manager.create(input({ members: [member('codex')] })); manager.startDiscussion(room.id);
  const original = await manager.wait(room.id); assert.deepEqual(original.tasks[0].workspaces, []);
  service.prune({ before: '9999-12-31T23:59:59.000Z', keepLatest: 0, dryRun: false });
  assert.equal(service.getRun(original.tasks[0].runId), undefined);
  manager.addTask(room.id, task('Use retained report', { dependsOn: ['task-1'] })); manager.startDiscussion(room.id);
  const result = await manager.wait(room.id);
  assert.equal(result.status, 'completed'); assert.equal(followerEvidence.workflowRecordAvailable, false);
  assert.equal(followerEvidence.fullResultResource, undefined); assert.deepEqual(followerEvidence.workspaces, []);
});

test('unverifiable completed prerequisites fail before any new discussion or task call', t => {
  for (const state of ['missing-identity', 'missing-artifacts', 'failed-workflow', 'pending-export']) {
    const { manager, service, requests } = fixture(t);
    const room = manager.create(input({ members: [member('codex')], tasks: [task('Prerequisite'), task('Dependent', { dependsOn: ['task-1'] })] }));
    manager.store.mutate(room.id, current => {
      current.tasks[0].status = 'completed';
      if (state !== 'missing-identity') current.tasks[0].runId = 'retained-prerequisite';
      if (state === 'failed-workflow' || state === 'pending-export') current.tasks[0].workspaces = [];
    });
    const getRun = service.getRun.bind(service);
    service.getRun = id => id !== 'retained-prerequisite' ? getRun(id) : state === 'failed-workflow' || state === 'pending-export'
      ? { id, input: { mode: 'delegate', task: 'Stored fixture', agents: ['codex'], permission: 'write' },
        status: state === 'failed-workflow' ? 'failed' : 'completed', createdAt: '2026-01-01T00:00:00Z', updatedAt: '2026-01-01T00:00:00Z', steps: [],
        workspaces: state === 'pending-export' ? [{ agent: 'codex', path: '/retained/worktree', patchPath: '/retained/patch', changed: true, exportStatus: 'pending' }] : [] }
      : undefined;
    assert.throws(() => manager.startDiscussion(room.id), /Prerequisite/);
    assert.equal(requests.length, 0); assert.equal(manager.get(room.id).status, 'idle');
  }
});
