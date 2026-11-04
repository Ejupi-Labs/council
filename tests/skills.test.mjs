import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { collaborationSkills, skillInstructions, validateSkills } from '../dist/skills.js';
import { configSchema } from '../dist/config.js';
import { CouncilService } from '../dist/service.js';
import { loadConfig } from '../dist/config.js';

const economy = 'dont-kill-an-ant-with-a-bazooka', review = 'council-implementation-review';
function fixture(t, agents) {
  const root = mkdtempSync(join(tmpdir(), 'council-skills-')), calls = [];
  const service = new CouncilService(loadConfig(root, { backend: 'mock', agents }), { doctor: async () => [], run: async request => {
    calls.push(request); return { agent: request.agent, status: 'completed', output: 'Verified offline result.', durationMs: 1 };
  } });
  t.after(async () => { await service.close(); rmSync(root, { recursive: true, force: true }); });
  return { service, calls };
}

test('collaboration skills load only their worker guidance and reject arbitrary paths or duplicate selection', () => {
  const catalog = collaborationSkills(); assert.equal(catalog.length, 4);
  assert.equal(new Set(catalog.map(skill => skill.id)).size, 4);
  for (const skill of catalog) {
    const instructions = skillInstructions([skill.id]);
    assert.ok(instructions.length > 0); assert.ok(!instructions.includes('council_route_run'));
    assert.ok(!instructions.includes('<!--')); assert.ok(!instructions.includes('description:'));
  }
  for (const invalid of [['../auth'], [economy, economy], ['unknown'], [null], Array(1), 'all']) assert.throws(() => validateSkills(invalid));
  assert.equal(skillInstructions([]), '');
  catalog[0].name = 'mutated'; assert.notEqual(collaborationSkills()[0].name, 'mutated');
});

test('agent skill defaults are snapshotted in a run and per-run overrides can disable them', async t => {
  const { service, calls } = fixture(t, { worker: { profile: 'codex', skills: [economy] } });
  const queued = service.start({ mode: 'delegate', agents: ['worker'], task: 'Make the small requested correction.' });
  service.config.agents.worker.skills = [review];
  const result = await service.completion(queued.id);
  assert.equal(result.status, 'completed'); assert.deepEqual(result.input.agentSettings.worker.skills, [economy]);
  assert.match(calls[0].prompt, /Keep the work proportional/); assert.ok(!calls[0].prompt.includes('Follow your assigned author'));
  await service.run({ mode: 'delegate', agents: ['worker'], task: 'No skill override.', permission: 'read', agentSettings: { worker: { skills: [] } } });
  assert.ok(!calls[1].prompt.includes('Selected collaboration skills'));
});

test('local per-agent skill assignments apply only to their selected worker and remain workflow evidence', async t => {
  const { service, calls } = fixture(t, { worker: { profile: 'codex' }, peer: { profile: 'claude' } });
  service.billing.update('worker', { skills: [economy] });
  const result = await service.run({ mode: 'parallel', agents: ['worker', 'peer'], task: 'Inspect independent offline evidence.', permission: 'read' });
  assert.equal(result.status, 'completed'); assert.deepEqual(result.input.agentSettings.worker.skills, [economy]);
  assert.match(calls.find(call => call.agent === 'worker').prompt, /Keep the work proportional/);
  assert.ok(!calls.find(call => call.agent === 'peer').prompt.includes('Selected collaboration skills'));
});

test('unsupported configured or per-run skill names fail before provider calls', async t => {
  assert.throws(() => configSchema.parse({ agents: { worker: { profile: 'codex', skills: ['arbitrary-file'] } } }));
  const { service, calls } = fixture(t, { worker: { profile: 'codex' } });
  assert.throws(() => service.start({ mode: 'delegate', agents: ['worker'], task: 'Inspect.', agentSettings: { worker: { skills: ['../../private'] } } }));
  assert.equal(calls.length, 0);
});

test('ignored judge metadata never assigns skills or billing participants to a nonparticipating agent', async t => {
  const { service, calls } = fixture(t, { worker: { profile: 'codex' }, peer: { profile: 'claude', skills: [review] } });
  const result = await service.run({ mode: 'delegate', agents: ['worker'], judge: 'peer', task: 'One worker only.', permission: 'read' });
  assert.equal(result.status, 'completed'); assert.equal(calls.length, 1); assert.equal(calls[0].agent, 'worker');
  assert.equal(result.input.agentSettings?.peer, undefined);
});

test('a suggested room team preserves explicit skills through voting and automatic task execution', async t => {
  const root = mkdtempSync(join(tmpdir(), 'council-room-skills-'));
  const service = new CouncilService(loadConfig(root, { backend: 'mock', agents: { worker: { profile: 'codex' } } }));
  t.after(async () => { await service.close(); rmSync(root, { recursive: true, force: true }); });
  const selection = service.routePlan({ task: 'Inspect the offline fixture.', taskClass: 'routine', agents: ['worker'], skills: [economy] });
  const room = service.rooms.create({ title: 'Skill selection', brief: 'Offline fixture.', members: selection.suggestedRoomMembers,
    tasks: [{ title: 'Inspect', brief: 'Inspect the fixture and report verification.', permission: 'read' }] });
  assert.deepEqual(room.members[0].skills, [economy]);
  service.rooms.startDiscussion(room.id);
  assert.equal((await service.rooms.wait(room.id)).status, 'completed');
  const runs = service.store.listRuns(10);
  assert.equal(runs.length, 2);
  assert.ok(runs.every(run => run.status === 'completed' && run.input.agentSettings.worker.skills[0] === economy));
});
