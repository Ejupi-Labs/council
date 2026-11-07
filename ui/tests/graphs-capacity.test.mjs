import assert from 'node:assert/strict';
import test from 'node:test';
import { fixture, ownerAPI } from './helpers.mjs';

test('capacity copy uses all current claims when the sole-attempt marker is null', { timeout: 30000 }, async t => {
  const f = await fixture(t); const owner = await f.start(); const page = await f.browser.newPage();
  const base = (await ownerAPI(owner, '/routing/capacity?contextId=codex-native')).envelope;
  const claims = [1, 2].map(n => ({ slot: String(n), attemptId: String(n).repeat(32), graphId: null, nodeId: null,
    executorKind: 'native', contextId: n === 1 ? 'codex-native' : 'claude-native', capturedDeclaredAccountId: null,
    capturedLinkRevision: null, legacyBarrier: false }));
  let occupied = 2, writes = 0;
  page.on('request', request => { if (!['GET', 'HEAD'].includes(request.method())) writes++; });
  // Only this observation is controlled. No worker is launched or represented as
  // actual provider capacity; all other data comes from the disposable real owner.
  await page.route('**/api/v1/routing/capacity?**', async route => {
    const envelope = structuredClone(base);
    envelope.data.managedOccupancy = { resourceId: 'managed-native', limit: 2, occupiedSlots: String(occupied),
      activeClaims: structuredClone(claims.slice(0, occupied)), activeAttemptId: occupied === 1 ? claims[0].attemptId : null,
      activeContextId: occupied === 1 ? 'codex-native' : null, claimsSubject: occupied > 0 };
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(envelope) });
  });
  await page.goto(owner.url);
  await page.locator('#routing-agents-panel > summary').click({ timeout: 5000 });
  const occupancy = page.locator('#routing-capacity-facts > div').filter({ has: page.locator('dt', { hasText: 'Council managed occupancy' }) }).locator('dd');
  await page.waitForFunction(() => document.getElementById('routing-capacity-facts')?.textContent.includes('occupied slots: 2'), undefined, { timeout: 5000 });
  assert.match(await occupancy.textContent(), /occupied slots: 2/);
  assert.match(await occupancy.textContent(), new RegExp(claims[0].attemptId));
  assert.match(await occupancy.textContent(), new RegExp(claims[1].attemptId));
  assert.doesNotMatch(await occupancy.textContent(), /No Council worker claim/);
  for (occupied of [1, 0]) {
    await page.locator('#refresh-routing').click({ timeout: 5000 });
    await page.waitForFunction(expected => document.getElementById('routing-capacity-facts')?.textContent.includes('occupied slots: ' + expected), occupied, { timeout: 5000 });
    assert.match(await occupancy.textContent(), new RegExp('occupied slots: ' + occupied));
  }
  assert.equal(writes, 0, 'observing capacity must not create work or alter a report');
});
