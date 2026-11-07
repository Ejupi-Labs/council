import assert from 'node:assert/strict';
import test from 'node:test';
import { readFile } from 'node:fs/promises';
import path from 'node:path';
import { fixture, openWorkspace, ownerAPI, fixtureRequestId, pollOwner, responseBarrier } from './helpers.mjs';

const documentTask = 'Replace "old" with "new" in docs/note.md only';
const documentFiles = { 'docs/note.md': 'CONTROLLED LOCAL WORKFLOW TEST DATA\r\nold\r\n東京\r\n' };

test('the shipped client requires exact verified-delivery metadata and limits known old contracts to owner Stop', { timeout: 60000 }, async t => {
  const f = await fixture(t); const owner = await f.start(); const page = await f.browser.newPage(); await page.goto(owner.url);
  const meta = (await ownerAPI(owner, '/meta')).envelope.data;
  const result = await page.evaluate(async actual => {
    const { supportsManagedExecution, supportsOwnerStop } = await import('/api.js');
    const known5 = { ...actual, appVersion: '2.0.0-dev.5', executionContract: '005-v1', workflowContract: '005-v1', capacity: { resourceId: 'managed-native', limit: 1, activeAttemptId: null } }; delete known5.graphContract;
    const known4 = { ...known5, appVersion: '2.0.0-dev.4', executionContract: '004-v1' }; delete known4.workflowContract;
    const known3 = { ...known4, appVersion: '2.0.0-dev.3', executionContract: '003-v1' };
    const known2 = { ...known4, appVersion: '2.0.0-dev.2', executionContract: '002-v1', managedExecution: 'native_codex', capacity: { limit: 1, connectionId: 'codex-native', activeAttemptId: null } }; delete known2.maintenanceContract;
    return {
      current: supportsManagedExecution(actual), missingWorkflow: supportsManagedExecution({ ...actual, workflowContract: undefined }),
      missingCapability: supportsManagedExecution({ ...actual, capabilities: actual.capabilities.filter(c => c !== 'verified_delivery') }),
      oldExecute: [known2, known3, known4, known5].map(supportsManagedExecution), oldStop: [known2, known3, known4, known5].map(supportsOwnerStop),
      inventedStop: supportsOwnerStop({ ...known4, appVersion: '2.0.0-dev.6' }),
    };
  }, meta);
  assert.equal(result.current, true); assert.equal(result.missingWorkflow, false); assert.equal(result.missingCapability, false);
  assert.deepEqual(result.oldExecute, [false, false, false, false]); assert.deepEqual(result.oldStop, [true, true, true, true]); assert.equal(result.inventedStop, false);
});

test('minimal Auto completes one authored document with observed builtin proof and honest review-only delivery', { timeout: 60000 }, async t => {
  const f = await fixture(t, { files: documentFiles }); const owner = await f.start();
  const page = await openWorkspace(f, owner, { workflow: 'auto' });
  await page.locator('#task-deliverable').selectOption('candidate');
  await page.locator('#task-fake-scenario').selectOption('workflow-document');
  await page.locator('#task-text').fill(documentTask);
  const admitted = page.waitForResponse(r => r.request().method() === 'POST' && new URL(r.url()).pathname === '/api/v1/tasks');
  await page.locator('#start-task').click(); const response = await admitted;
  assert.equal(response.status(), 201); const admission = (await response.json()).data;
  assert.equal(response.request().postDataJSON().workflow, undefined, 'normal Auto needs no workflow wizard or declaration');
  await page.locator('#workflow-facts').filter({ hasText: /Review only/i }).waitFor();
  await page.locator('#workflow-status').filter({ hasText: /completed/i }).waitFor();
  const steps = await ownerAPI(owner, '/tasks/' + admission.task.id + '/workflow/steps');
  assert.equal(steps.status, 200);
  assert.equal(steps.envelope.data.items.filter(s => s.executorKind === 'native').length, 1);
  assert.equal(steps.envelope.data.items.filter(s => s.role === 'review' || s.role === 'correction').length, 0);
  const checks = await ownerAPI(owner, '/tasks/' + admission.task.id + '/workflow/checks');
  assert.equal(checks.status, 200);
  const workflow = await ownerAPI(owner, '/tasks/' + admission.task.id + '/workflow');
  const builtin = workflow.envelope.data.workflow.authority.checks.find(c => c.builtinId === 'literal_document_replace/1');
  assert.ok(builtin); assert.ok(checks.envelope.data.items.some(c => c.definitionId === builtin.id && c.builtinCompleted && c.exitCode === null));
  const budget = await ownerAPI(owner, '/tasks/' + admission.task.id + '/budget');
  assert.equal(budget.envelope.data.budget.grantedAdmissions, '4');
  assert.equal(budget.envelope.data.budget.consumedAdmissions, '1');
  assert.equal(await readFile(path.join(f.project, 'docs/note.md'), 'utf8'), documentFiles['docs/note.md']);
  assert.deepEqual(await readFile(path.join(f.project, '.git/index')), f.indexBefore);
  assert.match(await page.locator('#workflow-status').textContent(), /review|apply/i);
  assert.doesNotMatch(await page.locator('#workflow-facts').textContent(), /0 tokens|free|saved money/i);
});

test('a lost prospective delivery preference acknowledgement preserves its scope while newer selection survives retry', { timeout: 60000 }, async t => {
  const f = await fixture(t); const owner = await f.start(); const page = await openWorkspace(f, owner, { workflow: 'auto' });
  const requests = []; const barrier = responseBarrier(); let first = true;
  await page.route('**/api/v1/workflow/preferences', async route => {
    if (route.request().method() !== 'PATCH') return route.continue();
    requests.push(route.request().postDataJSON());
    const response = await route.fetch();
    if (first) { first = false; await barrier.hold(); await route.abort('failed'); }
    else await route.fulfill({ response });
  });
  await page.locator('#delivery-preference').selectOption('apply_verified_local');
  await page.locator('#save-delivery-preference').click(); await barrier.arrived;
  await page.locator('#delivery-preference').selectOption('review_only'); barrier.release();
  await page.locator('#delivery-preference-status').filter({ hasText: /retry/i }).waitFor();
  await page.locator('#save-delivery-preference').click();
  await page.locator('#delivery-preference-status').filter({ hasText: /saved/i }).waitFor();
  assert.equal(requests.length, 2); assert.deepEqual(requests[1], requests[0]);
  assert.equal(requests[0].localMode, 'apply_verified_local');
  assert.equal(await page.locator('#delivery-preference').inputValue(), 'review_only');
  const actual = await ownerAPI(owner, '/workflow/preferences');
  assert.equal(actual.envelope.data.localMode, 'apply_verified_local');
  assert.match(await page.locator('#delivery-preference-status').textContent(), /future|prospective|next/i);
});

test('plain Auto response keeps exact user acceptance and selected deliverable producer without a fake local integration', { timeout: 60000 }, async t => {
  const f = await fixture(t); const owner = await f.start(); const page = await openWorkspace(f, owner, { workflow: 'auto' });
  const preference = await ownerAPI(owner, '/workflow/preferences');
  const saved = await ownerAPI(owner, '/workflow/preferences', 'PATCH', { requestId: fixtureRequestId(), expectedRevision: preference.envelope.data.revision, localMode: 'apply_verified_local' });
  assert.equal(saved.status, 200);
  await page.locator('#task-text').fill('CONTROLLED LOCAL TEST DATA: explain the project without changing files.');
  const admitted = page.waitForResponse(r => r.request().method() === 'POST' && new URL(r.url()).pathname === '/api/v1/tasks');
  await page.locator('#start-task').click(); const admission = (await (await admitted).json()).data;
  const settled = await pollOwner(owner, '/attempts/' + admission.attempt.id, d => d.phase === 'settled');
  assert.equal(settled.outcome, 'succeeded');
  await page.locator('#workflow-status').filter({ hasText: /accept|review/i }).waitFor();
  const steps = await ownerAPI(owner, '/tasks/' + admission.task.id + '/workflow/steps');
  assert.deepEqual(steps.envelope.data.items.map(s => s.role), ['author']);
  const deliveries = await ownerAPI(owner, '/tasks/' + admission.task.id + '/workflow/deliveries');
  assert.equal(deliveries.envelope.data.items.length, 0);
  assert.doesNotMatch(await page.locator('#workflow-status').textContent(), /applied|integrated/i);
});

test('lost Auto Start keeps clicked delivery scope across dependent autosave while newer text and review choices survive', { timeout: 90000 }, async t => {
  const f = await fixture(t, { files: documentFiles }); const owner = await f.start(); const page = await openWorkspace(f, owner, { workflow: 'auto' });
  await page.locator('#task-text').fill('Saved original before clicked workflow intent.');
  await page.locator('#save-draft').click(); await page.locator('#draft-status').filter({ hasText: /saved/i }).waitFor();
  await page.locator('#task-deliverable').selectOption('candidate'); await page.locator('#task-fake-scenario').selectOption('workflow-document');
  await page.locator('#task-delivery').selectOption('review_only'); await page.locator('#task-text').fill(documentTask);
  const autosave = responseBarrier(); const starts = []; let first = true;
  await page.route('**/api/v1/drafts/*', async route => {
    if (route.request().method() !== 'PUT') return route.continue();
    const response = await route.fetch(); await autosave.hold(); await route.fulfill({ response });
  });
  await page.route('**/api/v1/tasks', async route => {
    if (route.request().method() !== 'POST') return route.continue();
    starts.push(route.request().postDataJSON()); const response = await route.fetch();
    if (first) { first = false; await route.abort('failed'); } else await route.fulfill({ response });
  });
  await page.locator('#start-task').click(); await autosave.arrived;
  await page.locator('#task-text').fill('Newer local text after the clicked workflow.');
  await page.locator('#task-delivery').selectOption('apply_verified_local'); await page.locator('#task-review').selectOption('required'); autosave.release();
  await page.locator('#start-task').filter({ hasText: 'Retry pending start' }).waitFor();
  await page.locator('#start-task').click(); await page.locator('#task-detail').waitFor();
  await page.getByText('Full original task and current conditions', { exact: true }).click();
  await page.locator('#task-original').filter({ hasText: documentTask }).waitFor();
  assert.equal(starts.length, 2); assert.deepEqual(starts[1], starts[0]);
  assert.equal(starts[0].workflow.delivery.localMode, 'review_only'); assert.equal(starts[0].originalText, documentTask);
  assert.equal(starts[0].workflow.review, undefined);
  assert.equal(await page.locator('#task-text').inputValue(), 'Newer local text after the clicked workflow.');
  assert.equal(await page.locator('#task-delivery').inputValue(), 'apply_verified_local'); assert.equal(await page.locator('#task-review').inputValue(), 'required');
});

test('a Council-owned settled transition keeps observing until the actual document workflow completes', { timeout: 60000 }, async t => {
  const f = await fixture(t, { files: documentFiles }); const owner = await f.start();
  const page = await openWorkspace(f, owner, { workflow: 'auto' });
  // CONTROLLED HTTP SHAPE TEST DATA: actual fixed-agent work completes first.
  // One read then presents its real retained subject/evidence at the valid
  // blocked/Council-owned transition; later reads return the actual completion.
  // No provider, usage, billing or model observation is invented.
  await page.route('**/api/v1/tasks/*', async route => {
    const match = /^\/api\/v1\/tasks\/([a-f0-9]{32})$/.exec(new URL(route.request().url()).pathname);
    if (route.request().method() !== 'GET' || !match) return route.continue();
    await pollOwner(owner, '/tasks/' + match[1] + '/workflow', data => data.workflow?.state === 'completed');
    await route.continue();
  });
  let injected = false; let workflowReads = 0;
  await page.route('**/api/v1/tasks/*/workflow', async route => {
    if (route.request().method() !== 'GET') return route.continue();
    workflowReads++;
    const response = await route.fetch(); const envelope = await response.json();
    if (!injected && envelope.data.workflow?.state === 'completed') {
      injected = true;
      envelope.data.workflow = { ...envelope.data.workflow, state: 'blocked', nextAction: 'accept', nextOwner: 'council', latestAcceptanceId: null };
      delete envelope.data.acceptance;
      return route.fulfill({ response, json: envelope });
    }
    await route.fulfill({ response });
  });
  await page.locator('#task-deliverable').selectOption('candidate');
  await page.locator('#task-fake-scenario').selectOption('workflow-document');
  await page.locator('#task-text').fill(documentTask);
  const admitted = page.waitForResponse(r => r.request().method() === 'POST' && new URL(r.url()).pathname === '/api/v1/tasks');
  await page.locator('#start-task').click(); const admission = (await (await admitted).json()).data;
  await page.locator('#workflow-status').filter({ hasText: /completed/i }).waitFor();
  assert.equal(injected, true); assert.ok(workflowReads >= 2, 'the client must observe beyond the Council-owned transition');
  const actual = await ownerAPI(owner, '/tasks/' + admission.task.id + '/workflow');
  assert.equal(actual.envelope.data.workflow.state, 'completed'); assert.equal(actual.envelope.data.acceptance.deliveryResult, 'review_only');
});
