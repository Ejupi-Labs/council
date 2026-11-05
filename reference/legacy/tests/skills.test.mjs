import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { BUILT_IN_SKILL_IDS, collaborationSkills, skillInstructions, validateSkills } from '../dist/skills.js';
import { configSchema } from '../dist/config.js';
import { CouncilService } from '../dist/service.js';
import { loadConfig } from '../dist/config.js';

const economy = 'dont-kill-an-ant-with-a-bazooka', review = 'council-implementation-review';
const researcher = 'council-evidence-researcher', skeptic = 'council-constructive-skeptic', clarity = 'council-clear-explanations';
const lazyGenius = 'council-lazy-genius';
function fixture(t, agents, output = () => 'Verified offline result.') {
  const root = mkdtempSync(join(tmpdir(), 'council-skills-')), calls = [];
  const service = new CouncilService(loadConfig(root, { backend: 'mock', agents }), { doctor: async () => [], run: async request => {
    calls.push(request); return { agent: request.agent, status: 'completed', output: await output(request), durationMs: 1 };
  } });
  t.after(async () => { await service.close(); rmSync(root, { recursive: true, force: true }); });
  return { root, service, calls };
}

test('collaboration skills load only their worker guidance and reject arbitrary paths or duplicate selection', () => {
  const catalog = collaborationSkills(); assert.deepEqual(catalog.map(skill => skill.id), [...BUILT_IN_SKILL_IDS]);
  assert.equal(new Set(catalog.map(skill => skill.id)).size, catalog.length);
  assert.deepEqual(catalog.slice(0, 4).map(skill => skill.id), [economy, review, 'council-independent-perspectives', 'council-advisor']);
  assert.deepEqual(catalog.slice(4).map(skill => skill.id), [researcher, 'council-proposal-advocate', skeptic, 'council-constraint-reviewer', 'council-user-advocate', clarity, lazyGenius]);
  for (const skill of catalog) {
    const instructions = skillInstructions([skill.id]);
    assert.ok(instructions.length > 0); assert.ok(!instructions.includes('council_route_run'));
    assert.ok(!instructions.includes('<!--')); assert.ok(!instructions.includes('description:'));
  }
  for (const invalid of [['../auth'], [economy, economy], ['unknown'], [null], Array(1), 'all']) assert.throws(() => validateSkills(invalid));
  assert.equal(skillInstructions([]), '');
  catalog[0].name = 'mutated'; assert.notEqual(collaborationSkills()[0].name, 'mutated');
});

test('debate lenses remain per-alias approach guidance while explicit models, effort, phases and the judge call stay intact', async t => {
  const { service, calls } = fixture(t, {
    investigator: { profile: 'codex', model: 'profile-research-model', effort: 'low', skills: [researcher, clarity] },
    critic: { profile: 'claude', model: 'profile-critic-model', effort: 'low', skills: [skeptic, clarity] },
  }, request => request.prompt.startsWith('You are judge ') ? 'The evidence supports a conditional decision; one objection remains unresolved.' : `Independent evidence from ${request.agent}.`);
  const input = { mode: 'debate', agents: ['investigator', 'critic'], judge: 'critic', rounds: 2, maxCalls: 5,
    task: 'Compare two approaches using evidence; preserve unresolved objections.', permission: 'read',
    agentSettings: { investigator: { model: 'explicit-research-model', effort: 'medium' }, critic: { model: 'explicit-critic-model', effort: 'high' } } };
  const preview = service.engine.plan(input); assert.equal(preview.estimatedCalls, 5); assert.equal(calls.length, 0);
  const result = await service.run(input);
  assert.equal(result.status, 'completed'); assert.equal(calls.length, preview.estimatedCalls);
  assert.deepEqual(result.steps.map(step => step.role), ['debater-round-1', 'debater-round-1', 'debater-round-2', 'debater-round-2', 'judge']);
  assert.equal(result.steps.at(-1).agent, 'critic'); assert.match(calls.at(-1).prompt, /^You are judge in a Council debate workflow\./);
  assert.equal(result.output, 'The evidence supports a conditional decision; one objection remains unresolved.');
  for (const call of calls) {
    const role = call.agent === 'investigator' ? researcher : skeptic, other = call.agent === 'investigator' ? skeptic : researcher;
    assert.ok(call.prompt.includes(skillInstructions([role]))); assert.ok(call.prompt.includes(skillInstructions([clarity])));
    // Judge/cross-round context legitimately includes peers' answers, but never their private approach instructions.
    assert.equal(call.prompt.includes(skillInstructions([other])), false);
    assert.equal(call.model, input.agentSettings[call.agent].model); assert.equal(call.effort, input.agentSettings[call.agent].effort);
    assert.equal(call.permission, 'read');
  }
  assert.deepEqual(result.input.agentSettings.investigator.skills, [researcher, clarity]);
  assert.deepEqual(result.input.agentSettings.critic.skills, [skeptic, clarity]);
});

test('Lazy Genius review reaches the author revision without changing models, permissions, call bounds or exact result formats', async t => {
  const finding = { verdict: 'SIMPLIFY', findings: [{ component: 'Exporter registry', reason: 'Only the CSV format is requested.', simplerAlternative: 'Use the existing CSV exporter directly.' }] };
  const final = { status: 'revised', implementation: 'existing CSV exporter', preserved: ['validation', 'CSV acceptance checks'] };
  const { service, calls } = fixture(t, {
    author: { profile: 'codex', model: 'profile-author', effort: 'low' },
    reviewer: { profile: 'claude', model: 'profile-reviewer', effort: 'low', skills: [lazyGenius, clarity] },
  }, request => {
    if (request.prompt.startsWith('You are author ')) return 'Propose a registry with configurable exporters for one required CSV output.';
    if (request.prompt.startsWith('You are reviewer ')) return JSON.stringify(finding);
    assert.match(request.prompt, /^You are revision in a Council review workflow\./);
    assert.ok(request.prompt.includes(JSON.stringify(finding)), 'The author receives the exact simplicity finding, not an automatic approval or rewritten summary.');
    return JSON.stringify(final);
  });
  const input = { mode: 'review', agents: ['author', 'reviewer'], task: 'Review a CSV-export proposal. Preserve validation and required CSV behavior. Return the final result as JSON only.',
    permission: 'read', maxCalls: 3, agentSettings: { author: { model: 'explicit-author', effort: 'medium' }, reviewer: { model: 'explicit-reviewer', effort: 'high' } } };
  const plan = service.engine.plan(input); assert.equal(plan.estimatedCalls, 3); assert.equal(calls.length, 0);
  const result = await service.run(input);
  assert.equal(result.status, 'completed'); assert.equal(calls.length, plan.estimatedCalls);
  assert.deepEqual(result.steps.map(step => [step.agent, step.role]), [['author', 'author'], ['reviewer', 'reviewer'], ['author', 'revision']]);
  assert.deepEqual(JSON.parse(result.steps[1].result.output), finding); assert.deepEqual(JSON.parse(result.output), final);
  assert.ok(calls[1].prompt.includes(skillInstructions([lazyGenius]))); assert.ok(calls[1].prompt.includes(skillInstructions([clarity])));
  for (const call of [calls[0], calls[2]]) assert.equal(call.prompt.includes(skillInstructions([lazyGenius])), false, 'Reviewer guidance does not become a new author policy.');
  assert.ok(calls.every(call => call.permission === 'read' && call.model === input.agentSettings[call.agent].model && call.effort === input.agentSettings[call.agent].effort));
  assert.deepEqual(result.input.agentSettings.reviewer.skills, [lazyGenius, clarity]);
  assert.deepEqual(result.input.agentSettings.author.skills, []);
});

test('room lenses and plain explanations preserve exact ballot JSON and then execute only the approved task', async t => {
  const rationale = 'The task is clear. The worker can inspect the fixture and report what was checked.';
  const { service, calls } = fixture(t, { worker: { profile: 'codex', model: 'room-fixture-model', effort: 'low' } }, request => {
    const match = /ROOM_BALLOT_DATA_BEGIN\n([^]*?)\nROOM_BALLOT_DATA_END/.exec(request.prompt);
    if (!match) return 'The fixture was inspected. No changes were required.';
    const data = JSON.parse(match[1]);
    return JSON.stringify({ vote: 'approve', rationale, assignments: Object.fromEntries(data.tasks.map(task => [task.id, request.agent])) });
  });
  const room = service.rooms.create({ title: 'Evidence and clear reporting', brief: 'Inspect a read-only fixture.',
    members: [{ agent: 'worker', role: 'lead', model: 'explicit-room-model', effort: 'medium', skills: [skeptic, clarity, lazyGenius] }],
    tasks: [{ title: 'Inspect the fixture', brief: 'Report the observed result in plain language.', permission: 'read' }], autoExecute: true });
  service.rooms.startDiscussion(room.id);
  const completed = await service.rooms.wait(room.id);
  assert.equal(completed.status, 'completed'); assert.equal(completed.tasks.length, 1); assert.equal(completed.tasks[0].status, 'completed');
  assert.equal(completed.decision.rationale, rationale); assert.deepEqual(completed.decision.assignments, { [room.tasks[0].id]: 'worker' });
  assert.equal(calls.length, 2, 'A discussion and one approved task do not add a style-rewrite or debate call.');
  assert.equal(calls.filter(call => call.prompt.includes('ROOM_BALLOT_DATA_BEGIN')).length, 1);
  assert.ok(calls[1].prompt.startsWith('You are delegate in a Council delegate workflow.'));
  assert.ok(calls.every(call => call.model === 'explicit-room-model' && call.effort === 'medium' && call.permission === 'read'));
  assert.ok(calls.every(call => call.prompt.includes(skillInstructions([skeptic])) && call.prompt.includes(skillInstructions([clarity])) && call.prompt.includes(skillInstructions([lazyGenius]))));
  const taskRun = service.getRun(completed.tasks[0].runId);
  assert.equal(taskRun.output, 'The fixture was inspected. No changes were required.');
  assert.deepEqual(taskRun.input.agentSettings.worker.skills, [skeptic, clarity, lazyGenius]);
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
