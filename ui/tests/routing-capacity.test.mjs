import test from 'node:test';
import assert from 'node:assert/strict';
import { fixture, openWorkspace, ownerAPI, reportFixture, responseBarrier } from './helpers.mjs';



test('optional reported busy Codex and available Claude route useful fake work without claiming a login or free allowance', { timeout: 90000 }, async t => {
  const f = await fixture(t); const owner = await f.start();
  assert.equal((await reportFixture(owner, 'codex-native', 'occupancy', 'busy')).status, 201);
  assert.equal((await reportFixture(owner, 'claude-native', 'occupancy', 'available')).status, 201);
  const page = await openWorkspace(f, owner);
  await page.locator('#routing-agents-panel > summary').click();
  await page.locator('#routing-subject').selectOption('context:claude-native');
  await page.locator('#routing-capacity-facts').filter({ hasText: 'available' }).waitFor();
  const facts = await page.locator('#routing-capacity-facts').textContent();
  assert.match(facts, /user_report/); assert.match(facts, /External occupancyavailable/); assert.match(facts, /Allowanceunknown/);
  assert.match(facts, /Actual loginUnknown/); assert.match(facts, /Council managed occupancy/);
  await page.locator('#task-text').fill('CONTROLLED LOCAL TEST: use the reported available context.');
  await page.locator('#start-task').click(); await page.locator('#task-route-facts').filter({ hasText: 'claude-native' }).waitFor();
  assert.match(await page.locator('#attempt-facts').textContent(), /Observed accountUnknown/);
});

test('a delayed report acknowledgement and exact retry preserve newer report fields across reload', { timeout: 90000 }, async t => {
  const f = await fixture(t); const owner = await f.start(); const page = await openWorkspace(f, owner);
  await page.locator('#routing-agents-panel > summary').click();
  await page.locator('#routing-subject').selectOption('context:codex-native'); await page.locator('#routing-report-state').selectOption('busy');
  await page.locator('#routing-report-note').fill('CONTROLLED LOCAL TEST DATA: clicked busy report.');
  const requests = []; page.on('request', r => { if (r.method() === 'POST' && new URL(r.url()).pathname === '/api/v1/routing/reports') requests.push(r.postDataJSON()); });
  const gate = responseBarrier(); t.after(() => gate.release());
  await page.route('**/api/v1/routing/reports', async route => { await route.fetch(); await gate.hold(); await route.abort('connectionfailed'); }, { times: 1 });
  await page.locator('#save-routing-report').click(); await gate.arrived;
  await page.locator('#routing-subject').selectOption('context:claude-native'); await page.locator('#routing-report-state').selectOption('available');
  await page.locator('#routing-report-note').fill('Newer local availability note'); gate.release();
  await page.locator('#routing-report-status').filter({ hasText: 'Retry' }).waitFor();
  await page.reload(); await page.waitForFunction(() => !document.querySelector('#refresh-routing')?.disabled);
  await page.locator('#routing-agents-panel > summary').click(); await page.locator('#save-routing-report').click();
  await page.locator('#routing-report-status').filter({ hasText: 'recorded' }).waitFor();
  assert.equal(requests.length, 2); assert.deepEqual(requests[1], requests[0]);
  assert.equal(requests[0].subject.id, 'codex-native'); assert.equal(requests[0].state, 'busy');
  assert.equal(await page.locator('#routing-subject').inputValue(), 'context:claude-native');
  assert.equal(await page.locator('#routing-report-note').inputValue(), 'Newer local availability note');
});

test('report corrections identify an older retained negative and leave a newer denial visible', { timeout: 90000 }, async t => {
  const f = await fixture(t); const owner = await f.start();
  const older = await reportFixture(owner, 'claude-native', 'allowance', 'exhausted');
  const newer = await reportFixture(owner, 'claude-native', 'allowance', 'exhausted');
  assert.equal(older.status, 201); assert.equal(newer.status, 201);
  const page = await openWorkspace(f, owner); await page.locator('#routing-agents-panel > summary').click();
  await page.locator('#routing-subject').selectOption('context:claude-native');
  await page.locator('#routing-report-dimension').selectOption('allowance'); await page.locator('#routing-report-state').selectOption('available');
  await page.getByText('Observation time, temporary cooldown and exact correction', { exact: true }).click();
  await page.locator('#routing-report-correction').selectOption(older.envelope.data.report.id);
  await page.locator('#routing-correction-action').selectOption('recover'); await page.locator('#save-routing-report').click();
  await page.locator('#routing-report-status').filter({ hasText: 'recorded' }).waitFor();
  const current = (await ownerAPI(owner, '/routing/capacity?contextId=claude-native')).envelope.data;
  assert.equal(current.dimensions.allowance.effectiveState, 'exhausted');
  assert.ok(current.dimensions.allowance.blockingReports.some(r => r.reportId === newer.envelope.data.report.id));
  assert.ok(!current.dimensions.allowance.blockingReports.some(r => r.reportId === older.envelope.data.report.id));
  assert.match(await page.locator('#routing-capacity-facts').textContent(), /exhausted/);
});

test('declared account association carries an inherited exhaustion through unlink and exact origin correction', { timeout: 90000 }, async t => {
  const f = await fixture(t); const owner = await f.start(); const page = await openWorkspace(f, owner);
  await page.locator('#routing-agents-panel > summary').click();
  await page.locator('#routing-account-details > summary').click();
  await page.locator('#routing-account-label').fill('CONTROLLED LOCAL DECLARED ACCOUNT');
  await page.locator('#routing-account-provider').fill('CONTROLLED PROVIDER DECLARATION');
  const created = page.waitForResponse(r => r.request().method() === 'POST' && new URL(r.url()).pathname === '/api/v1/routing/accounts');
  await page.locator('#save-routing-account').click(); const account = (await (await created).json()).data;
  await page.locator('#routing-subject option[value="account:' + account.id + '"]').waitFor({ state: 'attached' });
  await page.locator('#routing-account-edit').selectOption(account.id); await page.locator('#routing-account-provider').fill('');
  const edited = page.waitForResponse(r => r.request().method() === 'PUT' && new URL(r.url()).pathname === '/api/v1/routing/accounts/' + account.id);
  await page.locator('#save-routing-account').click(); const editResponse = await edited;
  assert.equal(editResponse.request().postDataJSON().providerLabel, ''); assert.equal((await editResponse.json()).data.providerLabel, null);
  await page.locator('#routing-subject').selectOption('account:' + account.id);
  await page.locator('#routing-report-dimension').selectOption('allowance'); await page.locator('#routing-report-state').selectOption('exhausted');
  const reported = page.waitForResponse(r => r.request().method() === 'POST' && new URL(r.url()).pathname === '/api/v1/routing/reports');
  await page.locator('#save-routing-report').click(); const report = (await (await reported).json()).data.report;
  await page.locator('#routing-link-context').selectOption('claude-native'); await page.locator('#routing-link-account').selectOption(account.id);
  const linked = page.waitForResponse(r => r.request().method() === 'PUT' && r.url().endsWith('/account-link'));
  await page.locator('#save-routing-link').click(); await linked;
  await page.locator('#routing-subject').selectOption('context:claude-native');
  await page.locator('#routing-capacity-facts').filter({ hasText: 'inherited from ' + account.id }).waitFor();
  await page.locator('#routing-link-account').selectOption('');
  const unlinked = page.waitForResponse(r => r.request().method() === 'PUT' && r.url().endsWith('/account-link'));
  await page.locator('#save-routing-link').click(); await unlinked;
  await page.locator('#routing-capacity-facts').filter({ hasText: report.id }).waitFor();
  await page.locator('#routing-report-dimension').selectOption('allowance'); await page.locator('#routing-report-state').selectOption('available');
  await page.getByText('Observation time, temporary cooldown and exact correction', { exact: true }).click();
  await page.locator('#routing-report-correction').selectOption(report.id); await page.locator('#routing-correction-action').selectOption('recover');
  const corrected = page.waitForResponse(r => r.request().method() === 'POST' && new URL(r.url()).pathname === '/api/v1/routing/reports');
  await page.locator('#save-routing-report').click(); const response = await corrected;
  assert.deepEqual(response.request().postDataJSON().subject, { kind: 'account', id: account.id });
  assert.equal(response.request().postDataJSON().correctionOf, report.id);
  const capacity = (await ownerAPI(owner, '/routing/capacity?contextId=claude-native')).envelope.data;
  assert.ok(!capacity.dimensions.allowance.blockingReports.some(item => item.reportId === report.id));
  assert.equal(capacity.reportEligibility.positiveCoverage.allowance, false);
  assert.match(await page.locator('#routing-capacity-facts').textContent(), /Actual loginUnknown/);
});
