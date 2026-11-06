import test from 'node:test';
import assert from 'node:assert/strict';
import { fixture, openWorkspace, responseBarrier } from './helpers.mjs';

test('actual fake history is unmeasured and page reads cannot claim charges or savings', { timeout: 90000 }, async t => {
  const f = await fixture(t); const owner = await f.start(); const page = await openWorkspace(f, owner);
  await page.locator('#task-text').fill('CONTROLLED LOCAL TEST: retained unmeasured history.');
  await page.locator('#start-task').click(); await page.locator('#attempt-status').filter({ hasText: 'Verification pending' }).waitFor();
  await page.locator('#task-routing-panel > summary').click(); await page.locator('#load-task-usage').click();
  await page.locator('#task-usage-summary').filter({ hasText: 'returned attempts' }).waitFor();
  const facts = await page.locator('#task-usage-summary').textContent();
  assert.match(facts, /ChargesUnknown/); assert.match(facts, /fake.*unmeasured/i); assert.doesNotMatch(facts, /Charges0|saved \d|savings \d/i);
  assert.match(await page.locator('#task-usage-list').textContent(), /fake_unmeasured/);
});

test('a late usage page cannot replace another selected task or its current budget', { timeout: 90000 }, async t => {
  const f = await fixture(t); const owner = await f.start(); const page = await openWorkspace(f, owner); const tasks = [];
  for (const text of ['First usage history task', 'Second usage history task']) {
    await page.locator('#task-text').fill(text); const reply = page.waitForResponse(r => r.request().method() === 'POST' && new URL(r.url()).pathname === '/api/v1/tasks');
    await page.locator('#start-task').click(); tasks.push((await (await reply).json()).data.task);
    await page.locator('#attempt-status').filter({ hasText: 'Verification pending' }).waitFor();
  }
  await page.locator('#task-routing-panel > summary').click(); const gate = responseBarrier(); t.after(() => gate.release());
  await page.route('**/api/v1/tasks/*/usage*', async route => { const response = await route.fetch(); await gate.hold(); await route.fulfill({ response }); }, { times: 1 });
  await page.locator('#load-task-usage').click(); await gate.arrived;
  await page.locator('#task-list button').filter({ hasText: tasks[0].id.slice(0, 8) }).click(); gate.release();
  await page.locator('#task-route-facts').filter({ hasText: tasks[0].id }).waitFor();
  assert.match(await page.locator('#task-detail-heading').textContent(), new RegExp(tasks[0].id.slice(0, 8)));
  assert.doesNotMatch(await page.locator('#task-usage-list').textContent(), new RegExp(tasks[1].id));
});

test('controlled usage wire data preserves huge exact counters and estimates without claiming full-task totals or charges', { timeout: 90000 }, async t => {
  const f = await fixture(t); const owner = await f.start(); const page = await openWorkspace(f, owner);
  await page.locator('#task-text').fill('CONTROLLED UI USAGE SHAPE TEST DATA. No provider was called.');
  const admitted = page.waitForResponse(r => r.request().method() === 'POST' && new URL(r.url()).pathname === '/api/v1/tasks');
  await page.locator('#start-task').click(); const admission = (await (await admitted).json()).data;
  await page.locator('#attempt-status').filter({ hasText: 'Verification pending' }).waitFor();
  const attemptId = admission.attempt.id; const observedAt = '2026-11-06T12:00:00.000Z';
  const counter = knownValue => ({ knownValue, contributingAttemptIds: knownValue === null ? [] : [attemptId], missingAttemptIds: knownValue === null ? [attemptId] : [] });
  const unknownScope = { inputTokens: counter(null), outputTokens: counter(null), cachedInputTokens: counter(null), cacheCreationInputTokens: counter(null) };
  const knownScope = { ...unknownScope, inputTokens: counter('9007199254740993'), outputTokens: counter('9223372036854775807') };
  await page.route('**/api/v1/tasks/' + admission.task.id + '/usage*', async route => {
    const response = await route.fetch(); const envelope = await response.json();
    // This authored HTTP projection qualifies rendering only; it is not measured fake usage.
    envelope.data = { taskId: admission.task.id, items: [{ attemptId, attemptSequence: '1', executionMode: 'native', outcome: 'failed', selectedUsageScope: 'top_level',
      usage: { inputTokens: '9007199254740993', outputTokens: '9223372036854775807', cachedInputTokens: null, cacheCreationInputTokens: null, scope: 'top_level', source: 'CONTROLLED UI TEST DATA', observedAt, completeness: 'partial' }, nativeModels: null, evidenceArtifactIds: [] }], nextCursor: null,
      knownSubtotal: { scope: 'returned_attempts', byUsageScope: { top_level: knownScope, whole_tree: unknownScope } },
      coverage: { scope: 'returned_attempts', status: 'partial', includedAttemptIds: [attemptId], unknownAttemptIds: [attemptId], excludedFakeAttemptIds: [] },
      estimates: [{ attemptId, model: 'CONTROLLED UI TEST MODEL', currency: 'USD', value: '1e999', scope: 'whole_tree_model', source: 'CONTROLLED UI TEST DATA', observedAt, evidenceArtifactIds: [], previewCompleteness: 'partial' }],
      referencePrices: [], charges: { status: 'unknown', items: [] } };
    await route.fulfill({ response, json: envelope });
  });
  await page.locator('#task-routing-panel > summary').click(); await page.locator('#load-task-usage').click();
  await page.locator('#task-usage-summary').filter({ hasText: '9007199254740993' }).waitFor();
  const text = await page.locator('#task-usage-summary').textContent();
  assert.match(text, /9007199254740993/); assert.match(text, /9223372036854775807/);
  assert.match(text, /returned attempts only/); assert.match(text, /ChargesUnknown/);
  assert.doesNotMatch(text, /9007199254740992|Infinity|full.task total/i);
  assert.match(await page.locator('#task-usage-list').textContent(), /1e999 USD/);
  assert.match(await page.locator('#task-usage-list').textContent(), /not a charge/);
});
