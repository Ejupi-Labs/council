import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { basename, dirname, join, resolve } from 'node:path';
import { CouncilService } from '../dist/service.js';
import { loadConfig } from '../dist/config.js';

for (const boundary of ['task-result', 'budget-refund']) {
  test(`room list counters match retained failed evidence after a lost ${boundary} commit acknowledgment`, { timeout: 15000 }, async t => {
    const parent = resolve(tmpdir()), root = mkdtempSync(join(parent, 'council-room-emergency-summary-')), requests = [];
    const runner = { doctor: async () => [], run: async request => {
      requests.push(request);
      const marker = /ROOM_BALLOT_DATA_BEGIN\n([^\n]+)\nROOM_BALLOT_DATA_END/.exec(request.prompt);
      const data = marker ? JSON.parse(marker[1]) : undefined;
      return { agent: request.agent, status: 'completed', durationMs: 1,
        output: data ? JSON.stringify({ vote: 'approve', rationale: 'The focused offline tasks are clear.', assignments: Object.fromEntries(data.tasks.map(task => [task.id, 'codex'])) })
          : 'Complete task evidence retained in the workflow record.' };
    } };
    const service = new CouncilService(loadConfig(root, { backend: 'mock', maxCalls: 32, maxConcurrency: 1 }), runner), manager = service.rooms;
    const mutate = manager.store.mutate;
    t.after(async () => {
      manager.store.mutate = mutate;
      await service.close();
      assert.equal(dirname(resolve(root)), parent); assert.ok(basename(root).startsWith('council-room-emergency-summary-'));
      rmSync(root, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 });
    });
    const room = manager.create({ title: 'Uncertain persistence acknowledgment', brief: 'Preserve uncertain evidence and stop new admission.',
      members: [{ agent: 'codex', role: 'worker', advisor: 'claude', advisorMaxRequests: 2 }, { agent: 'claude', role: 'advisor' }],
      tasks: [1, 2].map(index => ({ title: 'Task ' + index, brief: 'Return complete offline evidence without requesting advice.', permission: 'read' })) });
    let failed = false, tokenAtFault;
    manager.store.mutate = function (id, change, options) {
      if (failed) throw new Error('Injected room writer outage after acknowledgment loss.');
      const before = this.metadata(id), result = mutate.call(this, id, change, options);
      const atBoundary = boundary === 'task-result' ? result.tasks[0].status === 'completed'
        : before.tasks[0].status === 'running' && result.callsUsed < before.callsUsed;
      if (atBoundary) {
        tokenAtFault = { store: this.changeToken(), manager: manager.changeToken() };
        failed = true; throw new Error('Injected lost room commit acknowledgment.');
      }
      return result;
    };
    manager.startDiscussion(room.id);
    const retained = await manager.wait(room.id), durable = manager.store.list(1)[0], summary = manager.list(1)[0];
    assert.equal(failed, true); assert.equal(retained.status, 'failed'); assert.match(retained.error, /persistence failed/);
    assert.equal(manager.store.changeToken(), tokenAtFault.store, 'The failed writer cannot commit another durable room change.');
    assert.notEqual(manager.changeToken(), tokenAtFault.manager, 'The emergency publication must still invalidate cached room lists.');
    if (boundary === 'task-result') assert.notEqual(durable.completedTasks, retained.tasks.filter(task => task.status === 'completed').length);
    else assert.notEqual(durable.callsUsed, retained.callsUsed);
    assert.deepEqual(summary, { id: retained.id, backend: retained.backend, title: retained.title, status: retained.status,
      taskCount: retained.tasks.length, completedTasks: retained.tasks.filter(task => task.status === 'completed').length,
      callsUsed: retained.callsUsed, maxCalls: retained.maxCalls, revision: retained.revision, updatedAt: retained.updatedAt });
    summary.completedTasks = 99; summary.callsUsed = 99;
    assert.equal(manager.list(1)[0].completedTasks, retained.tasks.filter(task => task.status === 'completed').length);
    assert.equal(manager.list(1)[0].callsUsed, retained.callsUsed);
    assert.equal(requests.length, 2, 'The writer fault must fence the second task; advisor reservations must not create unused calls.');
    assert.equal(service.getRun(retained.tasks[0].runId).status, 'completed', 'Full workflow evidence remains inspectable despite room publication uncertainty.');
  });
}
