import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, realpathSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { basename, dirname, join } from 'node:path';
import { CouncilService } from '../dist/service.js';
import { loadConfig } from '../dist/config.js';

function fixture(t, handler) {
  const parent = realpathSync.native(tmpdir()), root = realpathSync.native(mkdtempSync(join(parent, 'council-room-diagnostics-'))), calls = [];
  const runner = { doctor: async () => [], run: async request => { calls.push(request); return handler(request); } };
  const service = new CouncilService(loadConfig(root, { backend: 'mock', maxCalls: 16, maxConcurrency: 2 }), runner);
  t.after(async () => {
    await service.close();
    assert.equal(dirname(realpathSync.native(root)), parent); assert.ok(basename(root).startsWith('council-room-diagnostics-'));
    rmSync(root, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 });
  });
  return { service, manager: service.rooms, calls };
}

test('failed discussion identifies the voter, public error and retained workflow without approving partial opinions', async t => {
  const diagnostic = 'The configured reviewer model is unavailable for this account. Choose an available explicit profile.';
  const { manager, service, calls } = fixture(t, request => {
    const data = JSON.parse(/ROOM_BALLOT_DATA_BEGIN\n([^\n]+)\nROOM_BALLOT_DATA_END/.exec(request.prompt)[1]);
    return { agent: request.agent, durationMs: 1, status: request.agent === 'claude' ? 'failed' : 'completed',
      output: request.agent === 'claude' ? '' : JSON.stringify({ vote: 'approve', rationale: 'An independent successful opinion.', assignments: Object.fromEntries(data.tasks.map(task => [task.id, 'codex'])) }),
      ...(request.agent === 'claude' ? { error: diagnostic } : {}) };
  });
  const room = manager.create({ title: 'Actionable provider failure', brief: 'Do not execute partial approvals.', members: [{ agent: 'codex', role: 'worker' }, { agent: 'claude', role: 'worker' }],
    tasks: [{ title: 'Implementation', brief: 'Inspect only after complete approval.', permission: 'read' }] });
  manager.startDiscussion(room.id);
  const result = await manager.wait(room.id), history = manager.messagePage(room.id).messages;
  const failed = history.find(message => message.kind === 'opinion' && message.author === 'claude');
  assert.equal(result.status, 'needs_attention'); assert.equal(result.decision, undefined); assert.equal(result.tasks[0].status, 'pending');
  assert.equal(calls.length, 2); assert.ok(failed.body.includes(diagnostic)); assert.ok(result.error.includes('claude failed')); assert.ok(result.error.includes(diagnostic));
  assert.ok(result.error.includes(failed.runId)); assert.equal(service.getRun(failed.runId).status, 'failed');
  assert.ok(history.some(message => message.author === 'codex' && message.body.includes('independent successful opinion')));
});

test('public discussion diagnostics do not reintroduce private request bodies or private runner error text', async t => {
  const secret = 'PRIVATE_PAIR_PAYLOAD_7c8bc3e1', errorSecret = 'PRIVATE_RUNNER_ERROR_3bd47d11';
  const output = JSON.stringify({ council_private_chat_request: { peer: 'claude', message: secret, context: secret } });
  const { manager, service } = fixture(t, request => ({ agent: request.agent, durationMs: 1, status: 'failed', output, error: errorSecret }));
  const room = manager.create({ title: 'Private diagnostic boundary', brief: 'Keep pair payloads out of the shared room.', members: [{ agent: 'codex', role: 'worker' }],
    tasks: [{ title: 'Read only', brief: 'No unapproved work.', permission: 'read' }] });
  manager.startDiscussion(room.id);
  const result = await manager.wait(room.id), history = manager.messagePage(room.id).messages;
  assert.equal(result.status, 'needs_attention'); assert.match(result.error, /Private coordination request did not complete/);
  const publicEvidence = JSON.stringify({ result, history, run: service.getRun(history.find(message => message.runId)?.runId) });
  assert.ok(!publicEvidence.includes(secret)); assert.ok(!publicEvidence.includes(errorSecret));
  assert.ok(history.some(message => message.kind === 'opinion' && message.body.includes('Error: Private coordination request')));
});
