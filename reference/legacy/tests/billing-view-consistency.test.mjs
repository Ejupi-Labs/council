import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { basename, dirname, join, resolve } from 'node:path';
import { CouncilService } from '../dist/service.js';
import { BillingManager } from '../dist/billing.js';
import { loadConfig } from '../dist/config.js';

function fixture(t) {
  const root = mkdtempSync(join(tmpdir(), 'council-billing-view-')), calls = [];
  const config = loadConfig(root, { backend: 'mock', agents: { worker: { enabled: true, profile: 'codex', model: 'offline-small' } } });
  const service = new CouncilService(config, { doctor: async () => [], run: async request => { calls.push(request); return { agent: request.agent, status: 'completed', output: 'Verified offline.', durationMs: 1 }; } });
  const writer = new BillingManager(config), observedAt = new Date(Date.now() - 1000).toISOString(), validUntil = new Date(Date.now() + 3600000).toISOString();
  const observation = remaining => ({ mode: 'subscription', source: { kind: 'reported', label: 'Offline quota observation ' + remaining }, observedAt, validUntil,
    windows: [{ id: 'allowance', label: 'Shared allowance', unit: 'requests', remaining }], routing: { tier: 'small', capabilities: ['read'], costRank: 1, qualityRank: 1 } });
  service.billing.importSnapshot('worker', observation(5));
  t.after(async () => { writer.close(); await service.close(); assert.equal(dirname(resolve(root)), resolve(tmpdir())); assert.ok(basename(root).startsWith('council-billing-view-')); rmSync(root, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 }); });
  return { service, writer, observation, calls };
}

test('billing views evaluate the returned observation instead of rereading a newer revision', t => {
  const { service, writer, observation, calls } = fixture(t), original = service.billing.list.bind(service.billing);
  service.billing.list = () => { const records = original(); writer.importSnapshot('worker', observation(0)); return records; };
  const displayed = service.billingView().agents[0];
  assert.equal(displayed.windows[0].remaining, 5); assert.equal(displayed.availability.status, 'available');
  assert.equal(displayed.availability.windows[0].effectiveRemaining, 5);
  assert.equal(displayed.source.label, 'Offline quota observation 5');
  service.billing.list = original;
  const latest = service.billingView().agents[0]; assert.equal(latest.windows[0].remaining, 0); assert.equal(latest.availability.status, 'exhausted');
  assert.equal(calls.length, 0);
});

test('routing evaluates quota from the same billing snapshot as the selected routing preferences', t => {
  const { service, writer, observation, calls } = fixture(t), original = service.billing.list.bind(service.billing);
  service.billing.list = () => { const records = original(); writer.importSnapshot('worker', observation(0)); return records; };
  const plan = service.routePlan({ task: 'Inspect the offline evidence.', taskClass: 'routine', requiredCapabilities: ['read'] });
  assert.deepEqual(plan.selectedAliases, ['worker']); assert.equal(plan.selected[0].quota.availability, 'available');
  service.billing.list = original;
  assert.throws(() => service.routePlan({ task: 'Inspect the newer observation.', taskClass: 'routine' }), /suitable|eligible|found/i);
  assert.equal(calls.length, 0);
});

test('admission billing receipts keep quota classification paired with their captured observation', async t => {
  const { service, writer, observation } = fixture(t), original = service.billing.get.bind(service.billing);
  let inside = false, reads = 0;
  service.billing.get = alias => {
    if (inside) return original(alias);
    inside = true; let record; try { record = original(alias); } finally { inside = false; }
    if (++reads === 2) writer.importSnapshot('worker', observation(0));
    return record;
  };
  const queued = service.start({ mode: 'delegate', task: 'Inspect the offline receipt.', agents: ['worker'], permission: 'read' });
  assert.equal((await service.completion(queued.id)).status, 'completed');
  const receipt = service.store.events(queued.id).find(event => event.type === 'run.billing').data.agents[0];
  assert.equal(receipt.source.label, 'Offline quota observation 5'); assert.equal(receipt.availability.status, 'available');
  assert.equal(receipt.availability.windows[0].remaining, 5); assert.equal(receipt.availability.windows[0].effectiveRemaining, 5);
  service.billing.get = original; assert.equal(service.billing.availability('worker').status, 'exhausted');
});
