import assert from 'node:assert/strict';
import test from 'node:test';
import { readFile } from 'node:fs/promises';
import path from 'node:path';
import { fixture, fixtureRequestId, openWorkspace, ownerAPI, pollOwner, reportFixture } from './helpers.mjs';

test('actual registered groups expose root preparation, originals, shared budget and scoped pending tasks', { timeout: 60000 }, async t => {
  const f = await fixture(t); const owner = await f.start();
  const opened = await ownerAPI(owner, '/projects/open', 'POST', { requestId: fixtureRequestId(), selectedPath: f.project, basePath: f.project });
  assert.equal(opened.status, 201);
  const projectId = opened.envelope.data.project.id;
  // These are explicit controlled reports, not vendor-capacity observations.
  // They keep native node admissions blocked even after the dispatcher is enabled;
  // local root preparation still runs under its own real owned operation.
  for (const context of ['codex-native', 'claude-native']) {
    const report = await reportFixture(owner, context, 'occupancy', 'busy');
    assert.equal(report.status, 201);
  }
  const shared = '\ufeffComplete shared original 東京\r\nEvery task stays required; no external publication.';
  const originals = ['First full original🙂\r\nPreserve the shared finite allowance.', 'Second full original π\r\nReturn a separate reviewable result.'];
  const body = { requestId: fixtureRequestId(), projectId, title: 'Actual read-only group observation', sharedInstructions: shared,
    executionMode: 'fake', maxParallelism: '2', routing: { budget: { admissionLimit: '3', windowSeconds: 900 } },
    nodes: originals.map((originalText, i) => ({ key: 'read_' + (i + 1), originalText, pins: { harnessId: 'codex' },
      fakeScenario: i === 0 ? 'graph-branch-a' : 'graph-branch-b' })) };
  const created = await ownerAPI(owner, '/graphs', 'POST', body);
  assert.equal(created.status, 201, JSON.stringify(created.envelope.error));
  assert.equal(created.envelope.receipt.requestId, body.requestId);
  const admission = created.envelope.data, graphId = admission.graph.id;
  assert.equal(admission.nodes.length, 2); assert.equal(admission.budget.budget.grantedAdmissions, '3');
  assert.equal(admission.budget.budget.consumedAdmissions, '0');
  const root = await pollOwner(owner, '/graphs/' + graphId + '/inputs/' + admission.graph.rootInputId,
    data => data.summary.state === 'ready', { timeoutMs: 25000 });
  assert.equal(root.summary.operationAttemptId, admission.preparationAttemptId);
  assert.ok(root.rootBinding?.manifest?.sha256); assert.ok(root.operationEffects?.artifactId);

  const page = await openWorkspace(f, owner, { fake: false, workflow: 'auto' });
  let writes = 0;
  page.on('request', request => { if (!['GET', 'HEAD'].includes(request.method())) writes++; });
  await page.locator('#open-graph-list').click({ timeout: 5000 });
  await page.locator('#graph-list button').filter({ hasText: graphId }).click({ timeout: 5000 });
  await page.waitForFunction(id => document.getElementById('graph-facts')?.textContent.includes(id), graphId, { timeout: 5000 });
  assert.match(await page.locator('#graph-facts').textContent(), /0 consumed \/ 3 granted/);
  assert.match(await page.locator('#graph-facts').textContent(), /Up to 2/);
  await page.locator('#graph-overview > details > summary').filter({ hasText: 'Retained definition' }).click({ timeout: 5000 });
  await page.waitForFunction(() => document.getElementById('graph-definition-status')?.textContent.includes('Retained definition'), undefined, { timeout: 5000 });
  const definitionText = await page.locator('#graph-definition').textContent();
  assert.ok(definitionText.includes(shared));
  for (const original of originals) assert.ok(definitionText.includes(original), 'complete original bytes must remain available');
  assert.match(definitionText, /no local application|No local application/i);
  await page.locator('#graph-node-list').getByRole('button', { name: 'Open task read_1', exact: true }).click({ timeout: 5000 });
  await page.waitForFunction(id => document.getElementById('task-facts')?.textContent.includes(id), admission.nodes[0].taskId, { timeout: 5000 });
  assert.equal(await page.locator('#task-original').textContent(), originals[0]);
  assert.match(await page.locator('#task-graph-membership').textContent(), /shared group authority/);
  for (const selector of ['#run-attempt', '#stop-attempt', '#save-guidance', '#adopt-task-workflow', '#advance-task-workflow'])
    assert.equal(await page.locator(selector).isDisabled(), true, selector + ' must not expose standalone authority');
  await page.waitForFunction(ownerId => document.getElementById('task-budget-facts')?.textContent.includes(ownerId), admission.graph.rootTaskId, { timeout: 5000 });
  assert.match(await page.locator('#task-budget-facts').textContent(), /Shared task-group budget/);
  assert.doesNotMatch(await page.locator('#task-budget-facts').textContent(), /Not adopted/);
  assert.equal(await page.locator('#task-grant-controls').isVisible(), false);
  assert.equal(await page.locator('#task-adoption-controls').isVisible(), false);
  assert.equal(writes, 0, 'list, overview, originals and pending task inspection must remain observations');
  const ownershipCopy = {
    workflow: await page.locator('#workflow-status').textContent(),
    routing: await page.locator('#task-route-facts').textContent(),
    guidance: await page.locator('#workflow-panel > p.help').first().textContent(),
  };
  assert.ok(Object.values(ownershipCopy).every(message => /Task-group member/i.test(message)
    && !/Standalone task|Legacy task|explicit adoption/i.test(message)),
  'a pending graph member must not be presented as an unadopted standalone task: ' + JSON.stringify(ownershipCopy));

  const current = await ownerAPI(owner, '/graphs/' + graphId);
  const stopBody = { requestId: fixtureRequestId(), expectedGraphControlRevision: current.envelope.data.graph.controlRevision };
  const stopped = await ownerAPI(owner, '/graphs/' + graphId + '/stop', 'POST', stopBody);
  assert.equal(stopped.status, 202); assert.equal(stopped.envelope.receipt.requestId, stopBody.requestId);
  assert.equal(stopped.envelope.data.graph.id, graphId);
  assert.equal(stopped.envelope.data.stopAccepted, true);
  assert.equal(stopped.envelope.data.reason, 'accepted');
  // An exact Stop receipt does not by itself prove cleanup. Read retained state
  // separately and never relabel an admitted/root operation as a native launch.
  await page.locator('#graph-refresh').click({ timeout: 5000 });
  await page.waitForFunction(() => /cancell/.test(document.getElementById('graph-detail-heading')?.textContent ?? ''), undefined, { timeout: 10000 });
  const budget = await ownerAPI(owner, '/graphs/' + graphId + '/budget');
  assert.equal(budget.envelope.data.budget.consumedAdmissions, '0');
  assert.equal(budget.envelope.data.budget.currentWindow.deadlineAt, null);
  assert.deepEqual(await readFile(path.join(f.project, '.git', 'index')), f.indexBefore);
  assert.equal(await readFile(path.join(f.project, 'hello.txt'), 'utf8'), 'unstaged\n');
});
