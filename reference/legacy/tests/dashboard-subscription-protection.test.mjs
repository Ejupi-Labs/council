import test from 'node:test';
import assert from 'node:assert/strict';
import { billingRaw, billingInput, billingPatch } from '../dist/dashboard-billing-ui.js';
import { dashboardHtml } from '../dist/dashboard-ui.js';

const record = { mode: 'subscription', accountKey: null, plan: 'Recorded plan', observedAt: '2026-11-04T12:00:00Z', validUntil: '2026-11-05T12:00:00Z',
  windows: [{ id: 'weekly', label: 'Included allowance', unit: 'requests', scope: 'subscription', limit: 100, remaining: 0, used: 100, resetAt: null },
    { id: 'credits', label: 'Paid credits', unit: 'credits', scope: 'credits', limit: null, remaining: 500, used: null, resetAt: null }],
  prices: null, routing: null, skills: null, stopAtSubscriptionLimit: false, subscriptionWindowIds: null };

test('billing protection edits preserve the quota observation, credit scope and timestamps', () => {
  const raw = billingRaw(record); raw.stopAtSubscriptionLimit = true; raw.subscriptionWindowIds = 'weekly';
  const next = billingInput(raw, record), patch = billingPatch(record, next);
  assert.deepEqual(patch, { stopAtSubscriptionLimit: true, subscriptionWindowIds: ['weekly'] });
  assert.equal(next.windows[1].scope, 'credits'); assert.equal(next.windows[1].remaining, 500);
  assert.equal(next.observedAt, record.observedAt); assert.equal(next.validUntil, record.validUntil);
});

test('billing protection can be explicitly disabled without changing exhausted allowance data', () => {
  const saved = { ...record, stopAtSubscriptionLimit: true, subscriptionWindowIds: ['weekly'] }, raw = billingRaw(saved);
  raw.stopAtSubscriptionLimit = false; raw.subscriptionWindowIds = '';
  assert.deepEqual(billingPatch(saved, billingInput(raw, saved)), { stopAtSubscriptionLimit: false, subscriptionWindowIds: null });
  assert.equal(saved.windows[0].remaining, 0);
});

test('legacy subscription windows receive explicit scope without inventing a new observation', () => {
  const legacy = { ...record, windows: [{ ...record.windows[0], scope: undefined }] }, raw = billingRaw(legacy);
  assert.equal(raw.windows[0].scope, 'subscription');
  assert.deepEqual(billingPatch(legacy, billingInput(raw, legacy)), {});
});

test('English dashboard exposes explicit subscription protection and opt-in private peer budgets', () => {
  const html = dashboardHtml('protection-test');
  assert.match(html, /Stop at zero included allowance/); assert.match(html, /Paid credits do not override/);
  assert.match(html, /id="billingSubscriptionWindows"/); assert.match(html, /id="createPrivateChatRequests"/);
  assert.match(html, /id="privateChats"/); assert.match(html, /id="privateChatDialog"/);
});
