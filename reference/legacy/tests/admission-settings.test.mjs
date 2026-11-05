import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { basename, dirname, join, resolve } from 'node:path';
import { CouncilService } from '../dist/service.js';
import { loadConfig } from '../dist/config.js';

function fixture(t, agents) {
  const root = mkdtempSync(join(tmpdir(), 'council-admission-')), calls = [];
  const service = new CouncilService(loadConfig(root, { backend: 'mock', maxCalls: 12, agents }), {
    doctor: async () => [], run: async request => { calls.push(request); return { agent: request.agent, status: 'completed', output: 'Verified offline.', durationMs: 1 }; },
  });
  t.after(async () => { await service.close(); assert.equal(dirname(resolve(root)), resolve(tmpdir())); assert.ok(basename(root).startsWith('council-admission-')); rmSync(root, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 }); });
  return { service, calls };
}

test('admission snapshots inherited model and effort before profile settings can change', async t => {
  const { service, calls } = fixture(t, { worker: { enabled: true, profile: 'codex', model: 'admitted-model', effort: 'low', skills: ['council-advisor'] } });
  const queued = service.start({ mode: 'delegate', task: 'Inspect the offline fixture.', agents: ['worker'], permission: 'read' });
  assert.deepEqual(queued.input.agentSettings.worker, { model: 'admitted-model', effort: 'low', skills: ['council-advisor'] });
  service.config.agents.worker.model = 'later-model'; service.config.agents.worker.effort = 'high'; service.config.agents.worker.skills = [];
  const result = await service.completion(queued.id);
  assert.equal(result.status, 'completed'); assert.equal(calls[0].model, 'admitted-model'); assert.equal(calls[0].effort, 'low');
  assert.deepEqual(service.getRun(queued.id).input.agentSettings.worker, queued.input.agentSettings.worker);
});

test('explicit settings prevail while admission records actual judge and advisor defaults', async t => {
  const { service, calls } = fixture(t, {
    author: { enabled: true, profile: 'codex', model: 'profile-author', effort: 'high', skills: ['council-advisor'] },
    peer: { enabled: true, profile: 'claude', model: 'profile-peer', effort: 'medium' },
    judge: { enabled: true, profile: 'codex', model: 'profile-judge', effort: 'high' },
    advisor: { enabled: true, profile: 'codex', model: 'profile-advisor', effort: 'high' },
    unused: { enabled: true, profile: 'codex', model: 'unused-model' },
  });
  const queued = service.start({ mode: 'council', task: 'Compare the offline evidence.', agents: ['author', 'peer'], judge: 'judge', permission: 'read',
    agentSettings: { author: { model: 'caller-author', effort: 'low', skills: [] } }, advisors: { author: { agent: 'advisor', maxRequests: 1 } } });
  assert.deepEqual(queued.input.agentSettings, {
    author: { model: 'caller-author', effort: 'low', skills: [] }, peer: { model: 'profile-peer', effort: 'medium', skills: [] },
    judge: { model: 'profile-judge', effort: 'high', skills: [] }, advisor: { model: 'profile-advisor', effort: 'high', skills: [] },
  });
  const result = await service.completion(queued.id); assert.equal(result.status, 'completed');
  assert.ok(calls.every(call => call.agent !== 'unused' && call.agent !== 'advisor'));
  assert.equal(calls.find(call => call.agent === 'author').model, 'caller-author');
});

test('admission retains native unknowns and does not promote an unused judge to a participant', async t => {
  const { service } = fixture(t, { worker: { enabled: true, profile: 'codex' }, unused: { enabled: true, profile: 'codex', model: 'not-selected', effort: 'high' } });
  const queued = service.start({ mode: 'delegate', task: 'Inspect the offline fixture.', agents: ['worker'], judge: 'unused', permission: 'read', agentSettings: { worker: { skills: [] } } });
  assert.deepEqual(queued.input.agentSettings, { worker: { skills: [] } });
  assert.equal((await service.completion(queued.id)).status, 'completed');
});
