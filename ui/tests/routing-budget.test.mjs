import test from 'node:test';
import assert from 'node:assert/strict';
import { fixture, openWorkspace, ownerAPI, seedSchema3 } from './helpers.mjs';

test('actual schema3 adoption and its dependent Start retain separate identities through two lost acknowledgements', { timeout: 180000 }, async t => {
  const f = await fixture(t); const legacy = await seedSchema3(f); const owner = await f.start(); const page = await openWorkspace(f, owner);
  const selectLegacy = async () => { await page.locator('#task-list button').filter({ hasText: legacy.taskId.slice(0, 8) }).click(); await page.locator('#task-routing-panel > summary').click(); };
  await selectLegacy(); await page.locator('#task-route-facts').filter({ hasText: 'explicit adoption required' }).waitFor();
  assert.match(await page.locator('#task-facts').textContent(), /Captured harness choiceAuto · no literal harness pin/);
  assert.match(await page.locator('#attempt-facts').textContent(), /Requested harness choiceAuto · no literal harness pin/);
  assert.doesNotMatch(await page.locator('#attempt-facts').textContent(), /Auto · economical routing/);
  await page.locator('#budget-additional-admissions').fill('2'); await page.locator('#budget-new-window').fill('1800');
  const amendments = []; const starts = [];
  page.on('request', r => { if (r.method() === 'POST' && r.url().endsWith('/budget-amendments')) amendments.push(r.postDataJSON()); if (r.method() === 'POST' && r.url().endsWith('/tasks/' + legacy.taskId + '/attempts')) starts.push(r.postDataJSON()); });
  await page.route('**/api/v1/tasks/*/budget-amendments', async route => { await route.fetch(); await route.abort('connectionfailed'); }, { times: 1 });
  let droppedStart = false;
  await page.route('**/api/v1/tasks/' + legacy.taskId + '/attempts', async route => { if (route.request().method() !== 'POST' || droppedStart) return route.continue(); droppedStart = true; await route.fetch(); await route.abort('connectionfailed'); });
  await page.locator('#adopt-start-task').click(); await page.locator('#task-routing-status').filter({ hasText: 'Retry' }).waitFor();
  assert.equal(starts.length, 0);
  const adopted = (await ownerAPI(owner, '/tasks/' + legacy.taskId + '/budget')).envelope.data.budget;
  assert.equal(adopted.remainingAdmissions, '2'); assert.equal(adopted.historicalAdmissions, '1');
  assert.equal(adopted.currentWindow.firstAdmissionAt, null); assert.equal(adopted.currentWindow.deadlineAt, null);
  await page.locator('#task-text').fill('Newer composer text during legacy reconciliation.'); await page.locator('#task-profile').selectOption('claude-sonnet-general');
  await page.reload(); await page.waitForFunction(() => !document.querySelector('#start-task')?.disabled); await selectLegacy();
  await page.locator('#adopt-start-task').click(); await page.locator('#task-routing-status').filter({ hasText: 'Retry' }).waitFor();
  assert.equal(amendments.length, 2); assert.deepEqual(amendments[1], amendments[0]); assert.equal(starts.length, 1);
  await page.reload(); await page.waitForFunction(() => !document.querySelector('#start-task')?.disabled); await selectLegacy();
  await page.locator('#adopt-start-task').click(); await page.locator('#task-routing-status').filter({ hasText: 'separately captured' }).waitFor();
  assert.equal(amendments.length, 2); assert.equal(starts.length, 2); assert.deepEqual(starts[1], starts[0]);
  assert.notEqual(starts[0].requestId, amendments[0].requestId);
  assert.equal(await page.locator('#task-original').textContent(), legacy.originalText);
  assert.equal(await page.locator('#task-text').inputValue(), 'Newer composer text during legacy reconciliation.');
  assert.equal(await page.locator('#task-profile').inputValue(), 'claude-sonnet-general');
  assert.equal((await ownerAPI(owner, '/tasks/' + legacy.taskId + '/attempts')).envelope.data.items.length, 2);
  await page.waitForFunction(() => !document.querySelector('#adopt-start-task')?.disabled);
  await page.getByText('Execution history and reported usage', { exact: true }).click();
  await page.locator('#attempt-list button').filter({ hasText: legacy.attemptId.slice(0, 8) }).click();
  await page.getByText('Source, candidate and observed native details', { exact: true }).click();
  await page.locator('#attempt-facts').filter({ hasText: legacy.attemptId }).waitFor();
  assert.match(await page.locator('#attempt-facts').textContent(), /Captured connectioncodex-native.*Requested harness choiceAuto · no literal harness pin/);
  assert.doesNotMatch(await page.locator('#attempt-facts').textContent(), /Auto · economical routing/);
});

async function start(page) {
  await page.locator('#task-text').fill('CONTROLLED LOCAL TEST: budget and immutable history.');
  const returned = page.waitForResponse(r => r.request().method() === 'POST' && new URL(r.url()).pathname === '/api/v1/tasks');
  await page.locator('#start-task').click(); const data = (await (await returned).json()).data;
  await page.locator('#attempt-status').filter({ hasText: 'Verification pending' }).waitFor(); return data;
}

test('a lost grant reply spends no worker and retries its exact amendment without overwriting newer grant input', { timeout: 90000 }, async t => {
  const f = await fixture(t); const owner = await f.start(); const page = await openWorkspace(f, owner); const admission = await start(page);
  await page.locator('#task-routing-panel > summary').click();
  await page.locator('#budget-additional-admissions').fill('1');
  const grants = []; page.on('request', r => { if (r.method() === 'POST' && r.url().endsWith('/budget-amendments')) grants.push(r.postDataJSON()); });
  await page.route('**/api/v1/tasks/*/budget-amendments', async route => { await route.fetch(); await route.abort('connectionfailed'); }, { times: 1 });
  await page.locator('#grant-task-budget').click(); await page.locator('#task-routing-status').filter({ hasText: 'Retry' }).waitFor();
  await page.locator('#budget-additional-admissions').fill('3'); await page.reload();
  await page.locator('#task-list button').filter({ hasText: admission.task.id.slice(0, 8) }).click();
  await page.locator('#task-routing-panel > summary').click(); await page.locator('#grant-task-budget').click();
  await page.locator('#task-budget-facts').filter({ hasText: 'Granted admissions3' }).waitFor();
  assert.equal(grants.length, 2); assert.deepEqual(grants[1], grants[0]); assert.equal(grants[0].additionalAdmissions, '1');
  assert.equal(await page.locator('#budget-additional-admissions').inputValue(), '3');
  const attempts = (await ownerAPI(owner, '/tasks/' + admission.task.id + '/attempts')).envelope.data.items;
  assert.equal(attempts.length, 1);
});

test('the third explicit attempt blocks at two units and guidance cannot replenish the shared window', { timeout: 90000 }, async t => {
  const f = await fixture(t); const owner = await f.start(); const page = await openWorkspace(f, owner); const admission = await start(page);
  await page.locator('#run-attempt').click(); await page.locator('#task-budget-facts').filter({ hasText: 'Remaining admissions0' }).waitFor();
  await page.locator('#attempt-status').filter({ hasText: 'Verification pending' }).waitFor();
  const before = (await ownerAPI(owner, '/tasks/' + admission.task.id + '/budget')).envelope.data.budget;
  const rejected = page.waitForResponse(r => r.request().method() === 'POST' && new URL(r.url()).pathname === '/api/v1/tasks/' + admission.task.id + '/attempts');
  await page.locator('#run-attempt').click(); assert.equal((await (await rejected).json()).error.code, 'budget_exhausted');
  await page.locator('#attempt-status').filter({ hasText: 'Grant additional authority explicitly' }).waitFor();
  const after = (await ownerAPI(owner, '/tasks/' + admission.task.id + '/budget')).envelope.data.budget;
  assert.deepEqual(after, before); assert.equal(after.consumedAdmissions, '2');
  await page.getByText('Add guidance for a new task version', { exact: true }).click();
  await page.locator('#task-guidance').fill('Explicit new version, without any additional work authority.');
  await page.locator('#guidance-profile').selectOption('claude-sonnet-general'); await page.locator('#guidance-suitability').selectOption('general');
  const guided = page.waitForResponse(r => r.request().method() === 'PUT' && r.url().endsWith('/guidance'));
  await page.locator('#save-guidance').click(); const guidanceReply = await guided;
  assert.deepEqual(guidanceReply.request().postDataJSON().routing, { targetProfileId: 'claude-sonnet-general', suitability: 'general' });
  await page.locator('#task-detail-heading').filter({ hasText: 'version 2' }).waitFor();
  const afterGuidance = (await ownerAPI(owner, '/tasks/' + admission.task.id + '/budget')).envelope.data.budget;
  assert.equal(afterGuidance.consumedAdmissions, '2'); assert.equal(afterGuidance.remainingAdmissions, '0');
  assert.deepEqual(afterGuidance.currentWindow, before.currentWindow);
  assert.equal((await ownerAPI(owner, '/tasks/' + admission.task.id + '/attempts')).envelope.data.items.length, 2);
});

test('continuation defaults OFF and the unsupported terminal category is disclosed rather than enabled', { timeout: 90000 }, async t => {
  const f = await fixture(t); const owner = await f.start(); const page = await openWorkspace(f, owner);
  assert.equal(await page.locator('#task-automatic-admissions').inputValue(), '');
  assert.match(await page.locator('#task-continuation-help').textContent(), /OFF.*pre_launch_ineligible.*terminal_transient.*unsupported/i);
  await page.locator('#task-fake-scenario').selectOption('partial_failure'); await page.locator('#task-text').fill('CONTROLLED LOCAL TEST: failure supplies no automatic authority.');
  const returned = page.waitForResponse(r => r.request().method() === 'POST' && new URL(r.url()).pathname === '/api/v1/tasks');
  await page.locator('#start-task').click(); const data = (await (await returned).json()).data;
  await page.locator('#attempt-status').filter({ hasText: 'failed' }).waitFor();
  assert.equal((await ownerAPI(owner, '/tasks/' + data.task.id + '/attempts')).envelope.data.items.length, 1);
  assert.match(await page.locator('#task-budget-facts').textContent(), /Automatic admissions0/);
});

test('an acknowledged-but-lost adoption-only receipt stays retryable after the task is already adopted and starts no worker', { timeout: 180000 }, async t => {
  const f = await fixture(t); const legacy = await seedSchema3(f); const owner = await f.start(); const page = await openWorkspace(f, owner);
  const selectLegacy = async () => { await page.locator('#task-list button').filter({ hasText: legacy.taskId.slice(0, 8) }).click(); await page.locator('#task-routing-panel > summary').click(); };
  await selectLegacy(); await page.locator('#task-route-facts').filter({ hasText: 'explicit adoption required' }).waitFor();
  assert.match(await page.locator('#task-facts').textContent(), /Captured harness choiceAuto · no literal harness pin/);
  assert.match(await page.locator('#attempt-facts').textContent(), /Requested harness choiceAuto · no literal harness pin/);
  assert.doesNotMatch(await page.locator('#attempt-facts').textContent(), /Auto · economical routing/);
  await page.locator('#budget-additional-admissions').fill('2'); await page.locator('#budget-new-window').fill('1800');
  const requests = []; page.on('request', r => { if (r.method() === 'POST' && r.url().endsWith('/budget-amendments')) requests.push(r.postDataJSON()); });
  await page.route('**/api/v1/tasks/*/budget-amendments', async route => { await route.fetch(); await route.abort('connectionfailed'); }, { times: 1 });
  await page.locator('#adopt-task-budget').click(); await page.locator('#task-routing-status').filter({ hasText: 'Retry' }).waitFor();
  await page.locator('#budget-additional-admissions').fill('3'); await page.reload();
  await page.waitForFunction(() => !document.querySelector('#start-task')?.disabled); await selectLegacy();
  await page.locator('#adopt-task-budget').filter({ hasText: 'Retry pending adopt' }).click();
  await page.locator('#task-routing-status').filter({ hasText: 'Explicit adopt recorded' }).waitFor();
  assert.equal(requests.length, 2); assert.deepEqual(requests[1], requests[0]);
  assert.equal(await page.locator('#budget-additional-admissions').inputValue(), '3');
  const budget = (await ownerAPI(owner, '/tasks/' + legacy.taskId + '/budget')).envelope.data.budget;
  assert.equal(budget.remainingAdmissions, '2'); assert.equal(budget.currentWindow.firstAdmissionAt, null);
  assert.equal((await ownerAPI(owner, '/tasks/' + legacy.taskId + '/attempts')).envelope.data.items.length, 1);
});
