import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { BillingManager, BillingRevisionConflict, SubscriptionLimitError } from '../dist/billing.js';
import { CouncilService } from '../dist/service.js';
import { loadConfig } from '../dist/config.js';

const now = Date.now(), iso = offset => new Date(now + offset).toISOString();
const window = (id, remaining, scope = 'subscription') => ({ id, label: id, unit: 'requests', remaining, scope });
const snapshot = (remaining, changes = {}) => ({ mode: 'subscription', stopAtSubscriptionLimit: true, source: { kind: 'reported', label: 'Offline quota report' },
  observedAt: iso(-1000), validUntil: iso(3600000), windows: [window('included', remaining), window('paid', 1000, 'credits')], ...changes });
function fixture(t, handler) {
  const root = mkdtempSync(join(tmpdir(), 'council-subscription-protection-')), requests = [], releases = [];
  const config = loadConfig(root, { backend: 'mock', maxConcurrency: 1, maxCalls: 32 });
  const runner = { doctor: async () => [], run: async request => { requests.push(request); return { agent: request.agent, status: 'completed', durationMs: 1, output: 'Complete offline evidence.', ...await handler?.(request) }; } };
  const service = new CouncilService(config, runner), peers = [];
  t.after(async () => { for (const release of releases) release(); await service.close(); for (const peer of peers) peer.close(); rmSync(root, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 }); });
  return { service, requests, peer() { const peer = new BillingManager(config); peers.push(peer); return peer; },
    gate() { let resolve; const promise = new Promise(done => { resolve = done; }); releases.push(resolve); return { promise, resolve }; } };
}
const workflow = (agent = 'codex', changes = {}) => ({ mode: 'delegate', agents: [agent], permission: 'read', task: 'Verify the bounded offline fixture.', ...changes });

test('subscription protection defaults off and policy changes preserve observations and stay alias-local on shared accounts', t => {
  const { service } = fixture(t), billing = service.billing;
  assert.equal(billing.get('codex').stopAtSubscriptionLimit, false); assert.equal(billing.subscriptionProtection('codex').allowed, true);
  billing.importSnapshot('codex', snapshot(4, { accountKey: 'shared', stopAtSubscriptionLimit: false }));
  billing.update('claude', { accountKey: 'shared' });
  const before = billing.get('codex'), other = billing.get('claude');
  const saved = billing.update('codex', { stopAtSubscriptionLimit: true, subscriptionWindowIds: ['included'] }, before.revision);
  assert.equal(saved.observedAt, before.observedAt); assert.deepEqual(saved.source, before.source); assert.deepEqual(saved.windows, before.windows);
  assert.equal(billing.get('claude').stopAtSubscriptionLimit, false); assert.equal(billing.get('claude').revision, other.revision);
  assert.throws(() => billing.update('codex', { stopAtSubscriptionLimit: false }, before.revision), BillingRevisionConflict);
  assert.equal(billing.remove('codex', saved.revision).stopAtSubscriptionLimit, true, 'Removing observations cannot implicitly turn off protection.');
  assert.equal(billing.subscriptionProtection('codex').allowed, false);
});

test('paid credits cannot mask exhausted included allowance even after validity and reset dates pass', t => {
  const { service } = fixture(t), billing = service.billing;
  billing.importSnapshot('codex', snapshot(0, { validUntil: iso(-100), windows: [{ ...window('included', 0), resetAt: iso(-100) }, window('paid', 999999, 'credits')] }));
  const record = billing.get('codex'); assert.equal(billing.availabilityFor(record).status, 'unknown');
  const protection = billing.subscriptionProtectionFor(record); assert.equal(protection.allowed, false); assert.equal(protection.status, 'exhausted');
  assert.deepEqual(protection.windowIds, ['included']); assert.throws(() => billing.assertSubscriptionAllowed('codex'), error => error instanceof SubscriptionLimitError && error.code === 'COUNCIL_SUBSCRIPTION_LIMIT' && error.receipt.status === 'exhausted');
  billing.importSnapshot('codex', snapshot(5, { observedAt: iso(-2000) })); assert.equal(billing.subscriptionProtection('codex').status, 'exhausted', 'An older positive observation cannot clear exhaustion.');
  billing.importSnapshot('codex', snapshot(5, { observedAt: iso(-500), validUntil: iso(-100) })); assert.equal(billing.subscriptionProtection('codex').status, 'exhausted', 'A stale positive observation cannot clear exhaustion.');
  billing.importSnapshot('codex', snapshot(5, { observedAt: iso(-100) })); assert.equal(billing.subscriptionProtection('codex').allowed, true);
});

test('an empty paid-credit balance does not block positive included subscription capacity or ordinary routing availability', t => {
  const { service } = fixture(t), billing = service.billing;
  billing.importSnapshot('codex', snapshot(5, { windows: [window('included', 5), window('paid', 0, 'credits')] }));
  assert.equal(billing.availability('codex').status, 'available'); assert.equal(billing.subscriptionProtection('codex').allowed, true);
  assert.equal(billing.availability('codex').windows.find(window => window.id === 'paid').state, 'exhausted', 'Every reported counter remains visible without treating paid credits as included capacity.');
});

test('protected unknown, missing, future, stale, and credit-only data fail closed without changing counters', t => {
  const { service } = fixture(t), billing = service.billing;
  for (const changes of [{ windows: [] }, { windows: [window('paid', 100, 'credits')] }, { observedAt: iso(100000) },
    { validUntil: iso(-100) }, { windows: [window('included', null)] }, { subscriptionWindowIds: ['missing'] }, { subscriptionWindowIds: ['paid'] }, { mode: 'api' }]) {
    billing.importSnapshot('codex', snapshot(5, changes)); assert.equal(billing.subscriptionProtection('codex').allowed, false, JSON.stringify(changes));
  }
  billing.importSnapshot('codex', snapshot(5, { subscriptionWindowIds: [] })); const record = billing.get('codex');
  assert.equal(billing.subscriptionProtectionFor(record).allowed, true); assert.equal(record.windows[0].remaining, 5);
  billing.update('codex', { stopAtSubscriptionLimit: false }); assert.equal(billing.subscriptionProtection('codex').status, 'disabled');
});

test('window IDs resembling object prototype keys remain ordinary quota windows', t => {
  const { service } = fixture(t), billing = service.billing;
  billing.importSnapshot('codex', snapshot(1, { windows: [window('__proto__', 0), window('constructor', 3)] }));
  assert.equal(billing.subscriptionProtection('codex').status, 'exhausted');
  billing.importSnapshot('codex', snapshot(1, { observedAt: iso(-100), windows: [window('__proto__', 3), window('constructor', 3)] }));
  assert.equal(billing.subscriptionProtection('codex').allowed, true);
});

test('a future-dated zero blocks calls but a corrected fresh positive observation can replace its invalid clock', t => {
  const { service } = fixture(t), billing = service.billing;
  billing.importSnapshot('codex', snapshot(0, { observedAt: iso(100000) })); assert.equal(billing.subscriptionProtection('codex').allowed, false);
  billing.importSnapshot('codex', snapshot(5, { observedAt: iso(-100) }));
  assert.equal(billing.subscriptionProtection('codex').allowed, true, 'An invalid future timestamp cannot keep valid corrected observations blocked until that imaginary time.');
});

test('direct workflows retain a structured failed receipt and make zero calls when protected allowance is exhausted', async t => {
  const { service, requests } = fixture(t); service.billing.importSnapshot('codex', snapshot(0));
  const run = service.start(workflow()), result = await service.completion(run.id);
  assert.equal(result.status, 'failed'); assert.equal(requests.length, 0); assert.match(result.error, /COUNCIL_SUBSCRIPTION_LIMIT/);
  const blocked = service.store.events(run.id).find(event => event.type === 'step.dispatch_blocked');
  assert.equal(blocked.data.code, 'COUNCIL_SUBSCRIPTION_LIMIT'); assert.equal(blocked.data.receipt.allowed, false);
});

test('queued workflows recheck current quota after acquiring their provider slot', { timeout: 10000 }, async t => {
  let finish, started;
  const { service, requests, gate, peer } = fixture(t, async request => { if (request.agent === 'claude') { started.resolve(); await finish.promise; } return {}; });
  finish = gate(); started = gate(); service.billing.importSnapshot('codex', snapshot(4));
  const blocker = service.start(workflow('claude')); await started.promise;
  const queued = service.start(workflow()); peer().importSnapshot('codex', snapshot(0, { observedAt: iso(-100) })); finish.resolve();
  assert.equal((await service.completion(blocker.id)).status, 'completed');
  assert.equal((await service.completion(queued.id)).status, 'failed'); assert.deepEqual(requests.map(request => request.agent), ['claude']);
});

test('a live peer-process exhaustion observation aborts an active call and preserves partial evidence as failed', { timeout: 10000 }, async t => {
  let started;
  const { service, requests, gate, peer } = fixture(t, async request => {
    started.resolve(); await new Promise(resolve => { if (request.signal.aborted) resolve(); else request.signal.addEventListener('abort', resolve, { once: true }); });
    return { status: 'cancelled', output: 'Retained partial active-call evidence.' };
  });
  started = gate(); service.billing.importSnapshot('codex', snapshot(4));
  const run = service.start(workflow()); await started.promise;
  peer().importSnapshot('codex', snapshot(0, { observedAt: iso(-100) }));
  const result = await service.completion(run.id);
  assert.equal(requests[0].signal.aborted, true); assert.equal(result.status, 'failed'); assert.equal(result.steps[0].result.status, 'failed');
  assert.match(result.steps[0].result.output, /partial active-call evidence/); assert.match(result.error, /COUNCIL_SUBSCRIPTION_LIMIT/);
  assert.ok(service.store.events(run.id).some(event => event.type === 'step.dispatch_blocked' && event.data.active === true));
});

test('advisors, judges and continuations cannot bypass the per-dispatch subscription guard', async t => {
  const { service, requests } = fixture(t, async request => request.agent === 'codex' && request.prompt.includes('Council advisor protocol')
    ? { output: JSON.stringify({ council_advisor_request: { question: 'Review the bounded evidence.' } }) } : {});
  service.billing.importSnapshot('junie', snapshot(0));
  const advice = service.start(workflow('codex', { advisors: { codex: { agent: 'junie' } } }));
  assert.equal((await service.completion(advice.id)).status, 'failed'); assert.ok(!requests.some(request => request.agent === 'junie'));
  service.billing.importSnapshot('claude', snapshot(0));
  const judged = service.start({ mode: 'parallel', agents: ['codex'], judge: 'claude', permission: 'read', task: 'Analyze evidence then judge.' });
  assert.equal((await service.completion(judged.id)).status, 'failed'); assert.ok(!requests.some(request => request.agent === 'claude'));
  const original = service.start(workflow()); assert.equal((await service.completion(original.id)).status, 'completed');
  service.billing.importSnapshot('codex', snapshot(0));
  const continued = service.continue(original.id, { mode: 'delegate', agents: ['codex'], task: 'Continue with the retained evidence.' });
  assert.equal((await service.completion(continued.id)).status, 'failed');
});

test('enabling protection during an active call with unknown quota aborts even a claimed completed result', { timeout: 10000 }, async t => {
  let started;
  const { service, gate, requests } = fixture(t, async request => {
    started.resolve(); await new Promise(resolve => request.signal.addEventListener('abort', resolve, { once: true }));
    return { status: 'completed', output: 'A completion claim after protection blocked the call.' };
  });
  started = gate(); const run = service.start(workflow()); await started.promise;
  service.billing.update('codex', { stopAtSubscriptionLimit: true });
  const result = await service.completion(run.id);
  assert.equal(requests[0].signal.aborted, true); assert.equal(result.status, 'failed'); assert.equal(result.steps[0].result.status, 'failed');
  assert.match(result.steps[0].result.output, /completion claim/);
});

test('protected room discussion cannot authorize or dispatch tasks when subscription capacity is exhausted', async t => {
  const { service, requests } = fixture(t); service.billing.importSnapshot('codex', snapshot(0));
  const room = service.rooms.create({ title: 'Protected board', brief: 'Preserve allowance boundaries.', members: [{ agent: 'codex', role: 'worker' }],
    tasks: [{ title: 'Offline task', brief: 'Retain complete evidence.', permission: 'read' }] });
  service.rooms.startDiscussion(room.id); const result = await service.rooms.wait(room.id);
  assert.equal(result.status, 'needs_attention'); assert.equal(result.decision, undefined); assert.equal(result.tasks[0].status, 'pending');
  assert.equal(result.tasks[0].runId, undefined); assert.equal(requests.length, 0);
});
