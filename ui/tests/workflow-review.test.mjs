import assert from 'node:assert/strict';
import test from 'node:test';
import { binary } from '../../scripts/tools.mjs';
import { fixture, openWorkspace, ownerAPI, pollOwner, responseBarrier } from './helpers.mjs';

const task = 'Make council-workflow.txt contain exactly Council controlled workflow validation: VALID followed by CRLF. Inspect and correct the actual bytes.';
const check = { key: 'controlled-validation', executable: binary, arguments: ['__fixture', 'check', '--case', 'workflow-validation'], relativeWorkingDirectory: '.', scope: { kind: 'whole_candidate', paths: [] }, coveredConditionKeys: ['controlled-validation'], timeoutSeconds: 30, successExitCodes: [0], required: true };

test('a real blocking review leads to one correction and applicable recheck with four retained native stages', { timeout: 90000 }, async t => {
  const f = await fixture(t); const owner = await f.start(); const page = await openWorkspace(f, owner, { workflow: 'auto' });
  await page.locator('#task-deliverable').selectOption('candidate');
  await page.locator('#task-fake-scenario').selectOption('workflow-defect');
  await page.locator('#task-review').selectOption('required');
  await page.locator('#workflow-declarations > summary').click();
  await page.locator('#task-checks').fill(JSON.stringify([check]));
  await page.locator('#task-text').fill(task);
  const admitted = page.waitForResponse(r => r.request().method() === 'POST' && new URL(r.url()).pathname === '/api/v1/tasks');
  await page.locator('#start-task').click(); const admission = (await (await admitted).json()).data;
  await page.locator('#workflow-status').filter({ hasText: /completed/i }).waitFor();
  const steps = await ownerAPI(owner, '/tasks/' + admission.task.id + '/workflow/steps');
  const native = steps.envelope.data.items.filter(s => s.executorKind === 'native');
  assert.deepEqual(native.map(s => s.role), ['author', 'review', 'correction', 'review']);
  assert.equal(new Set(native.map(s => s.attemptId)).size, 4);
  await page.locator('#workflow-panel > details > summary').click();
  await page.locator('#load-workflow-reviews').click();
  await page.locator('#workflow-reviews-list').filter({ hasText: 'changes_required' }).waitFor();
  assert.match(await page.locator('#workflow-reviews-list').textContent(), /approved/);
  await page.locator('#load-workflow-findings').click();
  await page.locator('#workflow-findings-list li').first().waitFor();
  const reviews = await ownerAPI(owner, '/tasks/' + admission.task.id + '/workflow/reviews');
  const records = reviews.envelope.data.items;
  assert.equal(records.length, 2); assert.equal(records[0].outcome, 'changes_required'); assert.equal(records[1].outcome, 'approved');
  assert.notEqual(records[0].candidateBinding.candidateId, records[1].candidateBinding.candidateId);
  assert.ok(records[0].report.inspectedEvidence.length > 0);
  assert.ok(records[0].findings.some(finding => finding.finding.severity === 'blocking'));
  const budget = await ownerAPI(owner, '/tasks/' + admission.task.id + '/budget');
  assert.equal(budget.envelope.data.budget.consumedAdmissions, '4');
  assert.equal(budget.envelope.data.budget.remainingAdmissions, '0');

  // CONTROLLED HTTP SHAPE TEST DATA: retain actual stage IDs/evidence, but display the
  // already settled supporting reviewer as active solely to exercise client Stop targeting.
  // No synthetic model, token, cost, quota or provider observations are supplied.
  const reviewerId = native[3].attemptId; const authorId = native[0].attemptId;
  const reviewer = (await ownerAPI(owner, '/attempts/' + reviewerId)).envelope.data;
  await page.route('**/api/v1/tasks/' + admission.task.id, async route => {
    if (route.request().method() !== 'GET') return route.continue();
    const response = await route.fetch(); const envelope = await response.json();
    envelope.data.latestAttempt = { ...reviewer, phase: 'running', outcome: null, settledAt: null };
    await route.fulfill({ response, json: envelope });
  });
  const refreshed = page.waitForResponse(r => r.request().method() === 'GET' && new URL(r.url()).pathname === '/api/v1/tasks/' + admission.task.id);
  await page.locator('#refresh-attempt').click(); await refreshed;
  await page.locator('#stop-attempt').filter({ hasText: 'Stop current workflow stage' }).waitFor();
  await page.waitForFunction(() => !document.querySelector('#stop-attempt')?.disabled);
  assert.equal(await page.locator('#stop-attempt').isEnabled(), true);
  const stops = []; const gate = responseBarrier(); let firstStop = true;
  await page.route('**/api/v1/attempts/*/stop', async route => {
    stops.push({ path: new URL(route.request().url()).pathname, body: route.request().postDataJSON() });
    const response = await route.fetch();
    if (firstStop) { firstStop = false; await gate.hold(); await route.abort('failed'); }
    else await route.fulfill({ response });
  });
  await page.locator('#stop-attempt').click(); await gate.arrived;
  await page.locator('#load-workflow-steps').click();
  await page.locator('#workflow-steps-list button').filter({ hasText: 'Inspect this stage' }).first().click();
  await page.getByText('Source, candidate and observed native details', { exact: true }).click();
  await page.locator('#attempt-facts').filter({ hasText: authorId }).waitFor();
  await page.locator('#task-delivery').selectOption('apply_verified_local'); gate.release();
  await page.locator('#stop-attempt').filter({ hasText: 'Retry pending Stop' }).waitFor();
  const preservedSelection = page.waitForResponse(r => r.request().method() === 'GET' && new URL(r.url()).pathname === '/api/v1/attempts/' + authorId);
  await page.locator('#stop-attempt').click(); await preservedSelection;
  await page.waitForFunction(() => !document.querySelector('#stop-attempt')?.textContent.includes('Retry pending'));
  assert.equal(stops.length, 2); assert.deepEqual(stops[1], stops[0]);
  assert.equal(stops[0].path, '/api/v1/attempts/' + reviewerId + '/stop');
  assert.deepEqual(Object.keys(stops[0].body), ['requestId']);
  assert.equal(await page.locator('#task-delivery').inputValue(), 'apply_verified_local');
  await page.locator('#attempt-facts').filter({ hasText: authorId }).waitFor();
});

test('captured two-unit authority stops the actual review cycle without a fabricated correction or completion', { timeout: 90000 }, async t => {
  const f = await fixture(t); const owner = await f.start(); const page = await openWorkspace(f, owner, { workflow: 'auto' });
  await page.locator('#task-deliverable').selectOption('candidate');
  await page.locator('#task-fake-scenario').selectOption('workflow-defect');
  await page.locator('#task-review').selectOption('required');
  await page.locator('#workflow-declarations > summary').click();
  await page.locator('#task-checks').fill(JSON.stringify([check]));
  await page.locator('#task-budget-options > summary').click();
  await page.locator('#task-budget-admissions').fill('2'); await page.locator('#task-text').fill(task);
  const admitted = page.waitForResponse(r => r.request().method() === 'POST' && new URL(r.url()).pathname === '/api/v1/tasks');
  await page.locator('#start-task').click(); const admission = (await (await admitted).json()).data;
  // A just-settled author/reviewer can be transiently blocked while Council owns
  // its next stage. Wait for actionable user exhaustion and the actual verifier.
  const workflowRoute = '/tasks/' + admission.task.id + '/workflow';
  const exhausted = await pollOwner(owner, workflowRoute, data => data.workflow?.state === 'blocked' && data.workflow.nextOwner === 'user' && data.workflow.nextAction === 'budget_exhausted');
  const requiredCheck = admission.workflow.authority.checks.find(definition => definition.key === check.key && definition.required && definition.kind === 'command');
  assert.ok(requiredCheck);
  const receipts = await pollOwner(owner, workflowRoute + '/checks', data => data.items.length === 1 && data.items[0].definitionId === requiredCheck.id && data.items[0].outcome === 'failed');
  const failedCheck = receipts.items[0];
  await pollOwner(owner, '/tasks/' + admission.task.id, data => data.latestAttempt?.id === failedCheck.attemptId && data.latestAttempt.phase === 'settled' && data.latestAttempt.role === 'check');
  assert.equal(failedCheck.rootTerminal, true); assert.equal(failedCheck.exitCode, 1);
  assert.equal(failedCheck.inputComplete, true); assert.equal(failedCheck.outputComplete, true); assert.equal(failedCheck.cleanup.state, 'verified');
  assert.equal(exhausted.acceptance, undefined);
  await page.locator('#workflow-status').filter({ hasText: /budget|exhausted/i }).waitFor();
  const steps = await ownerAPI(owner, '/tasks/' + admission.task.id + '/workflow/steps');
  assert.deepEqual(steps.envelope.data.items.filter(s => s.executorKind === 'native').map(s => s.role), ['author', 'review']);
  assert.doesNotMatch(await page.locator('#workflow-status').textContent(), /completed/);
  assert.match(await page.locator('#workflow-facts').textContent(), /user|grant|advance/i);
  const budget = await ownerAPI(owner, '/tasks/' + admission.task.id + '/budget');
  assert.equal(budget.envelope.data.budget.consumedAdmissions, '2');
  assert.equal(budget.envelope.data.budget.remainingAdmissions, '0');
});
