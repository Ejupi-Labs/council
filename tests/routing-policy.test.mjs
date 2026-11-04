import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, realpathSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { basename, dirname, join } from 'node:path';
import { planRouting } from '../dist/routing.js';
import { CouncilService, loadConfig } from '../dist/index.js';

const input = { task: 'Inspect the bounded acceptance evidence.', taskClass: 'routine' };
const profile = costRank => ({ enabled: true, profile: 'codex', model: 'offline-model', routing: { tier: 'small', capabilities: ['analysis'], costRank, qualityRank: 1 } });
const context = availability => ({ agents: { alpha: profile(1), beta: profile(2) }, availability, plan: () => ({ estimatedCalls: 1 }) });

test('subscription dispatch protection excludes unknown allowance without inventing fresh exhaustion evidence', () => {
  const reason = 'No included allowance has been observed.';
  const plan = planRouting(input, context({ alpha: { availability: 'unknown', fresh: false, dispatchBlocked: true, reason } }));
  assert.deepEqual(plan.selectedAliases, ['beta']);
  const alternative = plan.alternatives.find(item => item.alias === 'alpha');
  assert.equal(alternative.eligible, false); assert.equal(alternative.quota.availability, 'unknown'); assert.equal(alternative.quota.fresh, false);
  assert.equal(alternative.quota.dispatchBlocked, true); assert.match(alternative.reason, /subscription protection blocks dispatch/); assert.ok(alternative.reason.includes(reason));
  assert.ok(plan.warnings.some(warning => /beta.*quota is unknown/.test(warning)), 'Ordinary unprotected unknown allowance remains explicit and selectable.');
  assert.throws(() => planRouting({ ...input, agents: ['alpha'] }, context({ alpha: alternative.quota })), error => error.code === 'ROUTING_UNAVAILABLE'
    && error.alternatives.find(item => item.alias === 'alpha').quota.availability === 'unknown');
});

test('a retained policy block survives an expired quota observation while invalid policy metadata fails validation', () => {
  const plan = planRouting(input, context({ alpha: { availability: 'exhausted', fresh: false, dispatchBlocked: true, reason: 'An earlier zero remains blocked until positively observed.' } }));
  const alternative = plan.alternatives.find(item => item.alias === 'alpha');
  assert.equal(alternative.quota.availability, 'unknown'); assert.equal(alternative.quota.fresh, false); assert.equal(alternative.quota.dispatchBlocked, true);
  assert.throws(() => planRouting(input, context({ alpha: { availability: 'unknown', fresh: false, dispatchBlocked: 'true' } })), error => error.code === 'ROUTING_INVALID_INPUT');
});

test('service routing and admission keep unknown policy blocks distinct from current measured exhaustion', async t => {
  const parent = realpathSync(tmpdir()), root = realpathSync(mkdtempSync(join(parent, 'council-routing-policy-')));
  const service = new CouncilService(loadConfig(root, { backend: 'mock', agents: { alpha: { enabled: true, profile: 'codex', model: 'offline-alpha' }, beta: { enabled: true, profile: 'codex', model: 'offline-beta' } } }),
    { doctor: async () => [], run: async () => { assert.fail('Pure routing and protected dispatch must make zero agent calls.'); } });
  t.after(async () => { await service.close(); assert.equal(dirname(root), parent); assert.ok(basename(root).startsWith('council-routing-policy-')); rmSync(root, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 }); });
  service.billing.update('alpha', { mode: 'subscription', stopAtSubscriptionLimit: true, routing: profile(1).routing });
  service.billing.update('beta', { routing: profile(2).routing });
  let alternative = service.routePlan(input).alternatives.find(item => item.alias === 'alpha');
  assert.equal(alternative.quota.availability, 'unknown'); assert.equal(alternative.quota.fresh, false); assert.equal(alternative.quota.dispatchBlocked, true);
  assert.match(alternative.reason, /No included subscription allowance windows/);
  const run = await service.run({ mode: 'delegate', agents: ['alpha'], permission: 'read', task: input.task });
  assert.equal(run.status, 'failed'); assert.equal(run.steps.length, 0);
  const receipt = service.store.events(run.id).find(event => event.type === 'run.billing').data.agents.find(agent => agent.alias === 'alpha');
  assert.equal(receipt.availability.status, 'unknown'); assert.equal(receipt.subscriptionProtection.status, 'unknown');
  const snapshot = { source: { kind: 'manual', label: 'Offline account observation' }, observedAt: new Date().toISOString(), validUntil: new Date(Date.now() + 60000).toISOString(),
    windows: [{ id: 'included', label: 'Included requests', unit: 'requests', scope: 'subscription', remaining: 0 }] };
  service.billing.update('alpha', snapshot);
  alternative = service.routePlan(input).alternatives.find(item => item.alias === 'alpha');
  assert.equal(alternative.quota.availability, 'exhausted'); assert.equal(alternative.quota.fresh, true); assert.equal(alternative.quota.dispatchBlocked, true);
  service.billing.update('alpha', { validUntil: new Date(Date.now() - 1000).toISOString() });
  alternative = service.routePlan(input).alternatives.find(item => item.alias === 'alpha');
  assert.equal(alternative.quota.availability, 'unknown'); assert.equal(alternative.quota.fresh, false); assert.equal(alternative.quota.dispatchBlocked, true);
});
