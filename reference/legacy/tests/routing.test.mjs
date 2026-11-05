import test from 'node:test';
import assert from 'node:assert/strict';
import { join } from 'node:path';
import { planRouting } from '../dist/routing.js';
import { RoutingError } from '../dist/routing-types.js';
import { parseRoutingInput } from '../dist/routing-schema.js';
import { WorkflowEngine } from '../dist/engine.js';
import { BUILT_IN_SKILL_IDS } from '../dist/skills.js';

const profile = (tier, costRank, qualityRank, capabilities = ['read', 'code']) => ({ tier, costRank, qualityRank, capabilities });
const agent = (model, routing, extra = {}) => ({ enabled: true, profile: 'codex', model, routing, ...extra });
const defaultAgents = () => ({
  tiny: agent('user-small-a', profile('small', 1, 1), { authority: 2 }),
  middle: agent('user-small-b', profile('small', 2, 2)),
  refined: agent('user-small-c', profile('small', 3, 3)),
  ordinary: agent('user-standard', profile('standard', 4, 5)),
  deep: agent('user-large', profile('large', 7, 9, ['read', 'code', 'proof'])),
});
const input = overrides => ({ task: 'Make the requested small edit and verify it.', taskClass: 'routine', ...overrides });
function fixture(overrides = {}) {
  const agents = overrides.agents ?? defaultAgents(), calls = [], plans = [];
  const settings = { version: 1, backend: 'mock', workspaceRoot: process.cwd(), stateDir: join(process.cwd(), '.council'),
    maxConcurrency: 2, timeoutMs: 15000, maxCalls: 100, maxOutputBytes: 16384, agents };
  const engine = new WorkflowEngine(settings, { doctor: async () => [], run: async request => {
    calls.push(request); return { agent: request.agent, status: 'completed', output: `Verified result from ${request.agent}.`, durationMs: 1 };
  } }, { saveRun() {}, appendEvent() {} });
  const availability = overrides.availability ?? Object.fromEntries(Object.keys(agents).map(alias => [alias, { availability: 'available', fresh: true }]));
  const context = { agents, availability, plan: workflow => { plans.push(structuredClone(workflow)); return engine.plan(workflow); } };
  return { agents, calls, plans, engine, context };
}

test('routine economy chooses configured small capacity without inspecting task length or calling a model', () => {
  const wire = fixture(), task = 'Long but routine task data. '.repeat(6000) + 'TASK_END';
  const result = planRouting(input({ task }), wire.context);
  assert.deepEqual(result.selectedAliases, ['tiny']); assert.equal(result.workflow.mode, 'delegate');
  assert.equal(result.workflow.task, task); assert.equal(result.workflow.agentSettings.tiny.model, 'user-small-a');
  assert.equal(result.estimatedCalls, 1); assert.equal(wire.plans.length, 1); assert.equal(wire.calls.length, 0);
  assert.ok(result.reasons.some(reason => reason.includes('user-configured ranks')));
  assert.ok(result.alternatives.some(alternative => alternative.alias === 'deep' && !alternative.selected));
});

test('explicit task class chooses the configured capable tier even for a one-character complex task', () => {
  const wire = fixture();
  const result = planRouting(input({ task: '?', taskClass: 'complex', requiredCapabilities: ['proof'] }), wire.context);
  assert.deepEqual(result.selectedAliases, ['deep']); assert.equal(result.workflow.agentSettings.deep.model, 'user-large');
  assert.equal(wire.calls.length, 0);
});

test('economy, balanced and quality preferences compare user ordinal ranks without inferred prices', () => {
  const wire = fixture();
  for (const [strategy, expected] of [['economy', 'tiny'], ['balanced', 'middle'], ['quality', 'refined']]) {
    assert.deepEqual(planRouting(input({ strategy }), wire.context).selectedAliases, [expected]);
  }
  const reordered = { ...wire.context, agents: Object.fromEntries(Object.entries(wire.agents).toReversed()) };
  assert.deepEqual(planRouting(input({ strategy: 'balanced' }), reordered).selectedAliases, ['middle']);
});

test('higher-tier escalation requires explicit acceptance and never silently changes the collaboration request', () => {
  const wire = fixture();
  assert.throws(() => planRouting(input({ requiredCapabilities: ['proof'] }), wire.context), error => error instanceof RoutingError
    && error.code === 'ROUTING_ESCALATION_REQUIRED' && error.escalation.allowed === false);
  const result = planRouting(input({ requiredCapabilities: ['proof'], allowEscalation: true }), wire.context);
  assert.deepEqual(result.selectedAliases, ['deep']); assert.equal(result.escalation.allowed, true);
  assert.deepEqual(result.escalation.targetTiers, ['large']);
  assert.equal(result.workflow.mode, 'delegate'); assert.equal(wire.calls.length, 0);
});

test('fresh exhausted quota excludes an alias while stale and unknown observations remain explicitly unknown', () => {
  const wire = fixture();
  wire.context.availability.tiny = { availability: 'exhausted', fresh: true, reason: 'User-reported account window is exhausted.' };
  assert.deepEqual(planRouting(input(), wire.context).selectedAliases, ['middle']);
  assert.throws(() => planRouting(input({ agents: ['tiny'] }), wire.context), error => error.code === 'ROUTING_UNAVAILABLE' && /No alias or model was substituted/.test(error.message));
  wire.context.availability.tiny = { availability: 'exhausted', fresh: false, reason: 'The observation has expired.' };
  const stale = planRouting(input(), wire.context);
  assert.deepEqual(stale.selectedAliases, ['tiny']); assert.equal(stale.selected[0].quota.availability, 'unknown');
  assert.ok(stale.warnings.some(warning => /tiny.*quota is unknown/.test(warning)));
  wire.context.availability = () => undefined;
  const unknown = planRouting(input(), wire.context);
  assert.equal(unknown.selected[0].quota.availability, 'unknown'); assert.equal(unknown.selected[0].quota.fresh, false);
});

test('missing routing configuration never guesses model size, price or capabilities from its name', () => {
  const wire = fixture({ agents: { opaque: agent('name-that-does-not-imply-a-tier', undefined) } });
  assert.throws(() => planRouting(input(), wire.context), error => error.code === 'ROUTING_CONFIG_REQUIRED');
  const explicit = planRouting(input({ agents: ['opaque'] }), wire.context);
  assert.equal(explicit.selected[0].tier, null); assert.ok(explicit.warnings.some(warning => /unconfigured/.test(warning)));
  assert.equal(explicit.workflow.agentSettings.opaque.model, 'name-that-does-not-imply-a-tier');
  assert.throws(() => planRouting(input({ agents: ['opaque'], requiredCapabilities: ['read'] }), wire.context), /Required capabilities are not configured/);
});

test('explicit alias order and model overrides are preserved without fallback', () => {
  const wire = fixture();
  const result = planRouting(input({ collaboration: 'review', agents: ['deep', 'tiny'], modelSettings: {
    deep: { model: 'caller-exact-model', effort: 'max' }, tiny: { effort: 'low' },
  } }), wire.context);
  assert.deepEqual(result.workflow.agents, ['deep', 'tiny']); assert.equal(result.workflow.agentSettings.deep.model, 'caller-exact-model');
  assert.equal(result.workflow.agentSettings.deep.effort, 'max'); assert.equal(result.workflow.agentSettings.tiny.effort, 'low');
  assert.ok(result.warnings.some(warning => /explicit model override is preserved/.test(warning)));
  assert.equal(result.escalation.allowed, true); assert.equal(result.estimatedCalls, 3);
  assert.throws(() => planRouting(input({ agents: ['tiny'], modelSettings: { deep: { model: 'never-discard-this' } } }), wire.context), /explicitly selected alias/);
  assert.throws(() => planRouting(input({ agents: ['absent'] }), wire.context), /not configured.*No alias or model was substituted/);
});

test('modelSettings alone constrain the exact alias rather than being silently discarded', () => {
  const wire = fixture();
  const result = planRouting(input({ modelSettings: { refined: { model: 'caller-override', effort: 'high' } } }), wire.context);
  assert.deepEqual(result.selectedAliases, ['refined']); assert.equal(result.workflow.agentSettings.refined.model, 'caller-override');
});

test('presets return runnable workflows with the requested team size and honest call costs', async () => {
  const wire = fixture();
  for (const [collaboration, count, mode, calls] of [['direct', 1, 'delegate', 1], ['review', 2, 'review', 3], ['parallel', 3, 'parallel', 3], ['council', 2, 'council', 5]]) {
    const result = planRouting(input({ collaboration, participants: count }), wire.context);
    assert.equal(result.workflow.mode, mode); assert.equal(result.workflow.agents.length, count); assert.equal(result.estimatedCalls, calls);
    assert.equal(result.suggestedRoomMembers.length, count); assert.equal(result.suggestedRoomMembers[0].role, 'lead');
    if (collaboration === 'council') assert.equal(result.workflow.judge, 'middle');
  }
  const review = planRouting(input({ collaboration: 'review' }), wire.context);
  const result = await wire.engine.run(review.workflow);
  assert.equal(result.status, 'completed', result.error); assert.equal(wire.calls.length, 3);
  assert.deepEqual(wire.calls.map(call => call.model), ['user-small-a', 'user-small-b', 'user-small-a']);
  assert.throws(() => planRouting(input({ collaboration: 'review', maxCalls: 2 }), wire.context), /requires 3 calls.*No alias, model, or collaboration preset was substituted/);
});

test('capability failures, disabled aliases and insufficient team capacity fail clearly without paid fallback', () => {
  const wire = fixture(); wire.agents.tiny.enabled = false;
  assert.throws(() => planRouting(input({ agents: ['tiny'] }), wire.context), /disabled/);
  assert.throws(() => planRouting(input({ requiredCapabilities: ['not-configured-anywhere'] }), wire.context), error => error.code === 'ROUTING_UNAVAILABLE');
  assert.throws(() => planRouting(input({ collaboration: 'review', participants: 4 }), wire.context), error => error.code === 'ROUTING_ESCALATION_REQUIRED');
  assert.throws(() => planRouting(input({ collaboration: 'direct', participants: 2 }), wire.context), /exactly one/);
  assert.throws(() => planRouting(input({ agents: ['middle'], participants: 2 }), wire.context), /same team size/);
  assert.equal(wire.calls.length, 0);
});

test('unknown comparative ranks are visible and never presented as measured cost or quality', () => {
  const wire = fixture({ agents: { uncertain: agent('user-model', profile('small', null, null)) } });
  const result = planRouting(input(), wire.context);
  assert.equal(result.selected[0].costRank, null); assert.equal(result.selected[0].qualityRank, null);
  assert.ok(result.warnings.some(warning => /cost rank is unknown; no cheaper-than claim/.test(warning)));
  assert.ok(result.warnings.some(warning => /quality rank is unknown/.test(warning)));
});

test('an unpinned native default warns about routing drift without changing the selected alias or inventing a model', () => {
  const wire = fixture({ agents: {
    'native-default': agent(undefined, profile('small', 1, 1)),
    pinned: agent('other-user-configured-model', profile('small', 2, 2)),
  } });
  const result = planRouting(input(), wire.context);
  assert.deepEqual(result.selectedAliases, ['native-default']);
  assert.equal(Object.hasOwn(result.workflow.agentSettings['native-default'], 'model'), false);
  assert.equal(result.selected[0].model, undefined);
  assert.ok(result.warnings.some(warning => /no explicit model is pinned.*default may change.*No model was substituted/.test(warning)));
  assert.equal(wire.calls.length, 0);
});

test('full task context is preserved, planner mutation is isolated and no config object is changed', () => {
  const wire = fixture(), before = structuredClone(wire.agents), context = 'Full project evidence. '.repeat(5000) + 'CONTEXT_END';
  const original = wire.context.plan; wire.context.plan = workflow => { const plan = original(workflow); workflow.agents[0] = 'mutated-validation-copy'; return plan; };
  const result = planRouting(input({ context, contextLimit: null }), wire.context);
  assert.equal(result.workflow.context, context); assert.equal(result.workflow.contextLimit, null);
  assert.deepEqual(result.workflow.agents, ['tiny']); assert.deepEqual(wire.agents, before); assert.equal(wire.calls.length, 0);
});

test('skills are assigned only when requested and per-alias skill overrides take priority', () => {
  const wire = fixture(), economical = BUILT_IN_SKILL_IDS[0], review = BUILT_IN_SKILL_IDS[1];
  wire.agents.tiny.skills = [review];
  assert.equal(planRouting(input(), wire.context).workflow.agentSettings.tiny.skills, undefined);
  const result = planRouting(input({ collaboration: 'review', agents: ['tiny', 'middle'], skills: [economical],
    modelSettings: { middle: { skills: [review] } } }), wire.context);
  assert.deepEqual(result.workflow.agentSettings.tiny.skills, [economical]); assert.deepEqual(result.workflow.agentSettings.middle.skills, [review]);
  assert.deepEqual(result.suggestedRoomMembers[0].skills, [economical]);
  result.suggestedRoomMembers[0].skills.push(review);
  assert.deepEqual(result.workflow.agentSettings.tiny.skills, [economical], 'Room preview mutation cannot change the runnable workflow.');
});

test('routing schemas reject malformed choices while allowing user-defined positive safe budgets', () => {
  assert.equal(parseRoutingInput(input({ participants: 1000 })).participants, 1000);
  for (const override of [{ taskClass: 'infer-from-text' }, { task: ' ' }, { participants: Number.MAX_SAFE_INTEGER + 1 },
    { agents: ['tiny', 'tiny'] }, { requiredCapabilities: [' '] }, { skills: ['unregistered-skill'] }]) {
    assert.throws(() => planRouting(input(override), fixture().context), RoutingError);
  }
  const wire = fixture(); wire.agents.tiny.routing.costRank = 0;
  assert.throws(() => planRouting(input(), wire.context), error => error.code === 'ROUTING_CONFIG_REQUIRED');
});
