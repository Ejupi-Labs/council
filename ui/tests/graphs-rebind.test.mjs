import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import path from 'node:path';
import test from 'node:test';
import { fixture, fixtureRequestId, openWorkspace, ownerAPI, pollOwner, responseBarrier } from './helpers.mjs';

async function read(owner, route) {
  const result = await ownerAPI(owner, route);
  assert.equal(result.status, 200, JSON.stringify(result.envelope.error));
  return result.envelope.data;
}

async function arrival(barrier) {
  let timer;
  try { await Promise.race([barrier.arrived, new Promise((_, reject) => { timer = setTimeout(() => reject(new Error('The actual rebind acknowledgement was not reached.')), 10000); })]); }
  finally { clearTimeout(timer); }
}

function budget(detail) {
  const actual = detail.budget.budget;
  return { owner: actual.taskId, granted: actual.grantedAdmissions, consumed: actual.consumedAdmissions, remaining: actual.remainingAdmissions,
    epoch: detail.graph.budgetAuthorityEpoch, window: actual.currentWindow.number, firstAdmission: actual.currentWindow.firstAdmissionAt, deadline: actual.currentWindow.deadlineAt };
}

async function terminal(owner, attemptId) {
  const attempt = await pollOwner(owner, '/attempts/' + attemptId, data => data.phase === 'settled', { timeoutMs: 30000 });
  assert.equal(attempt.executionMode, 'fake');
  assert.equal(attempt.outcome, 'succeeded');
  assert.equal(attempt.cleanup.state, 'verified');
  assert.ok(attempt.startedAt);
  return attempt;
}

test('explicit selected dependency rebind derives one version and retries its immutable intent without refill or launch', { timeout: 90000 }, async t => {
  const delayed = responseBarrier(), f = await fixture();
  t.after(async () => { delayed.release(); await f.close(); });
  const owner = await f.start();
  const opened = await ownerAPI(owner, '/projects/open', 'POST', { requestId: fixtureRequestId(), selectedPath: f.project, basePath: f.project });
  assert.equal(opened.status, 201);
  // All stale-input prerequisites use actual owned workers and acceptance. No
  // database state, accepted tuple or response envelope is fabricated.
  const declaration = { requestId: fixtureRequestId(), projectId: opened.envelope.data.project.id, executionMode: 'fake', title: 'Actual explicit stale input rebind',
    nodes: [
      { key: 'source', originalText: 'Inspect the complete fixed branch A result and preserve the user index.', fakeScenario: 'graph-branch-a', pins: { harnessId: 'codex' }, workflow: { review: 'required' }, acceptance: { deliverable: 'candidate' } },
      { key: 'receiver', originalText: 'Read the exact accepted source context and return this complete independent response 東京🙂.', fakeScenario: 'graph-response', pins: { harnessId: 'claude-code' },
        workflow: { review: 'auto', checks: [], maxCorrectionCycles: 0 }, acceptance: { deliverable: 'response_only' } },
    ], dependencies: [{ from: 'source', to: 'receiver', kind: 'files' }], routing: { budget: { admissionLimit: '5', windowSeconds: 900 } } };
  const created = await ownerAPI(owner, '/graphs', 'POST', declaration);
  assert.equal(created.status, 201, JSON.stringify(created.envelope.error));
  const graphId = created.envelope.data.graph.id;
  const initial = await pollOwner(owner, '/graphs/' + graphId, data => data.nodes.find(node => node.key === 'source')?.acceptedResult !== null
    && data.nodes.find(node => node.key === 'receiver')?.latestAttemptId !== null && data.budget.budget.consumedAdmissions === '3', { timeoutMs: 30000 });
  const source = initial.nodes.find(node => node.key === 'source'), receiver = initial.nodes.find(node => node.key === 'receiver');
  const oldAttempt = await terminal(owner, receiver.latestAttemptId);
  await pollOwner(owner, '/graphs/' + graphId + '/dispatches?nodeKey=receiver', data => data.items.length === 1 && data.items[0].outputObservation?.response?.state === 'complete', { timeoutMs: 10000 });
  const oldVersion = await read(owner, '/tasks/' + receiver.taskId + '/versions/1');
  const oldInput = await read(owner, '/graphs/' + graphId + '/inputs/' + receiver.inputId);
  const oldWorkflow = await read(owner, '/tasks/' + receiver.taskId + '/workflow');
  const oldSteps = await read(owner, '/tasks/' + receiver.taskId + '/workflow/steps?workflowId=' + oldWorkflow.workflow.id);
  const page = await openWorkspace(f, owner, { fake: false, workflow: 'auto' });
  const standalone = 'Unsent standalone draft remains separate from rebinding 東京🙂';
  await page.locator('#task-text').fill(standalone);
  await page.locator('#open-graph-list').click({ timeout: 5000 });
  await page.locator('#graph-list button').filter({ hasText: graphId }).click({ timeout: 5000 });
  await page.waitForFunction(id => document.getElementById('graph-facts')?.textContent.includes(id), graphId, { timeout: 5000 });
  await page.locator('#graph-guidance-node').selectOption(source.id);
  const guidance = 'Complete revised source-node guidance π\nInspect the same complete branch result under this new captured version.';
  await page.locator('#graph-guidance-text').fill(guidance);
  await page.locator('#graph-save-guidance').click({ timeout: 5000 });
  await page.waitForFunction(() => document.getElementById('graph-control-status')?.textContent.includes('Guidance version 2 recorded'), undefined, { timeout: 10000 });
  const guided = await read(owner, '/graphs/' + graphId);
  assert.equal(guided.nodes.find(node => node.id === source.id).taskVersion, '2');
  assert.equal(guided.nodes.find(node => node.id === receiver.id).taskVersion, '1');
  assert.equal(guided.nodes.find(node => node.id === receiver.id).inputId, receiver.inputId);
  assert.equal(guided.budget.budget.consumedAdmissions, '3');
  assert.equal((await read(owner, '/graphs/' + graphId + '/inputs/' + receiver.inputId)).summary.current, false);
  await page.locator('#graph-refresh').click({ timeout: 5000 });
  await page.waitForFunction(() => document.querySelector('#graph-node-list [data-node-key="source"]')?.textContent.includes('Current version 2'), undefined, { timeout: 5000 });
  await page.locator('#graph-advance').click({ timeout: 5000 });
  const changed = await pollOwner(owner, '/graphs/' + graphId, data => data.nodes.find(node => node.id === source.id)?.acceptedResult?.taskVersion === '2'
    && data.budget.budget.consumedAdmissions === '5' && data.activeClaims.length === 0, { timeoutMs: 30000 });
  const stale = changed.nodes.find(node => node.id === receiver.id);
  assert.equal(stale.taskVersion, '1');
  assert.equal(stale.inputId, receiver.inputId);
  assert.equal(stale.nodeAdmissionId, receiver.nodeAdmissionId);
  assert.equal(changed.budget.budget.remainingAdmissions, '0');
  assert.equal(changed.budget.budget.currentWindow.deadlineAt, initial.budget.budget.currentWindow.deadlineAt);
  assert.equal((await read(owner, '/tasks/' + receiver.taskId + '/attempts')).items.length, 1);
  await page.locator('#graph-refresh').click({ timeout: 5000 });
  await page.waitForFunction(() => document.getElementById('graph-facts')?.textContent.includes('5 consumed / 5 granted'), undefined, { timeout: 5000 });

  const advances = [];
  await page.route('**/api/v1/graphs/' + graphId + '/advance', async route => {
    const body = route.request().postDataJSON(), response = await route.fetch(), envelope = await response.json();
    advances.push({ raw: route.request().postData(), body, status: response.status(), envelope });
    if (Object.hasOwn(body, 'rebindNodeIds')) {
      const ordinal = advances.filter(item => Object.hasOwn(item.body, 'rebindNodeIds')).length;
      assert.equal(response.status(), 200, JSON.stringify(envelope.error));
      if (ordinal === 1) {
        assert.ok(envelope.data.dispatchAuthority, 'the actual saved rebind includes its captured execution registration');
        await delayed.hold();
        // Mutate only one field of the real saved acknowledgement. Admitted
        // work cannot be qualified by a reply that omits its registration.
        await route.fulfill({ response, json: { ...envelope, data: { ...envelope.data, dispatchAuthority: null } } }); return;
      }
      if (ordinal === 2) { await route.abort('failed'); return; } // Actual saved reply lost on exact retry.
    }
    await route.fulfill({ response });
  });
  // Omission remains an ordinary observation of eligible unfinished work. It
  // must never edit this receiver's already captured V1 workflow in place.
  // Join this actual response: an older successful Advance message is retained
  // on screen and cannot establish that the new request has completed.
  const omittedReply = page.waitForResponse(response => response.request().method() === 'POST'
    && new URL(response.url()).pathname === '/api/v1/graphs/' + graphId + '/advance', { timeout: 10000 });
  await page.locator('#graph-advance').click({ timeout: 5000 });
  assert.equal((await omittedReply).status(), 200);
  assert.equal(advances.length, 1);
  assert.equal(Object.hasOwn(advances[0].body, 'rebindNodeIds'), false);
  assert.equal(advances[0].status, 200);
  assert.deepEqual(advances[0].envelope.data.reboundNodes, []);
  assert.deepEqual(advances[0].envelope.data.preparationAttemptIds, []);
  assert.deepEqual(advances[0].envelope.data.dispatches, []);
  assert.equal((await read(owner, '/tasks/' + receiver.taskId)).task.currentVersion, '1');
  const checkbox = page.locator('#graph-rebind-nodes [data-graph-rebind-node="' + receiver.id + '"]');
  await checkbox.waitFor({ timeout: 5000 });
  assert.equal(await checkbox.isChecked(), false);
  assert.match(await page.locator('#graph-rebind-nodes').textContent(), /receiver/);
  assert.ok((await page.locator('#graph-rebind-nodes').textContent()).includes(receiver.inputId));
  assert.match(await page.locator('#graph-rebind-nodes').textContent(), /version 1/);
  const useUpdated = page.getByRole('button', { name: 'Use updated predecessor inputs and advance', exact: true });
  assert.equal(await useUpdated.isDisabled(), true, 'rebinding must require an explicit stale-node selection');
  await checkbox.check();
  await useUpdated.click({ timeout: 5000 });
  await arrival(delayed);
  const first = advances[1], captured = first.body;
  assert.deepEqual(captured.rebindNodeIds, [receiver.id]);
  assert.equal(captured.graphVersion, changed.graph.graphVersion);
  assert.equal(captured.expectedGraphControlRevision, changed.graph.controlRevision);
  assert.equal(captured.expectedBudgetAuthorityEpoch, changed.graph.budgetAuthorityEpoch);
  assert.equal(captured.expectedWindowNumber, changed.budget.budget.currentWindow.number);
  assert.deepEqual(captured.nodeAuthorities, changed.nodes.map(node => ({ nodeId: node.id, taskVersion: node.taskVersion, nodeControlRevision: node.controlRevision })));
  const rebound = first.envelope.data.reboundNodes;
  assert.equal(rebound.length, 1);
  assert.equal(rebound[0].nodeId, receiver.id);
  assert.equal(rebound[0].taskId, receiver.taskId);
  assert.equal(rebound[0].fromTaskVersion, '1');
  assert.equal(rebound[0].toTaskVersion, '2');
  assert.equal(rebound[0].provenance.kind, 'dependency_rebind');
  assert.equal(rebound[0].provenance.requestId, captured.requestId);
  assert.equal(rebound[0].provenance.previousInputId, receiver.inputId);
  assert.match(rebound[0].provenance.contributorVectorSha256, /^[a-f0-9]{64}$/);
  assert.equal(first.envelope.data.preparationAttemptIds.length, 1);
  assert.deepEqual(first.envelope.data.dispatches, []);
  // This is a newer local selection while the old real acknowledgement is held.
  // Neither its late error nor an exact retry may silently reselect the node.
  await checkbox.uncheck();
  const newerStatus = await page.locator('#graph-control-status').textContent();
  delayed.release();
  const retry = page.locator('#graph-control-pending').getByRole('button', { name: 'Retry exact Rebind and Advance', exact: true });
  try { await retry.waitFor({ timeout: 5000 }); }
  catch (error) { throw new Error('A real rebind receipt mutated only to null dispatchAuthority incorrectly reconciled admitted work: ' + error.message); }
  assert.equal(await page.locator('#graph-control-status').textContent(), newerStatus);
  await retry.click({ timeout: 5000 });
  await retry.waitFor({ timeout: 5000 });
  assert.equal(advances.length, 3);
  assert.equal(first.raw, advances[2].raw, 'the malformed-ack retry must preserve the captured rebind intent');
  assert.equal(await page.locator('#graph-control-status').textContent(), newerStatus);
  // The busy label changes before the replay reply arrives. Join the exact
  // acknowledged request and its reconciliation within one original deadline.
  const replayDeadline = Date.now() + 10000;
  const [acknowledged] = await Promise.all([
    page.waitForResponse(response => response.request().method() === 'POST'
      && new URL(response.url()).pathname === '/api/v1/graphs/' + graphId + '/advance'
      && response.request().postDataJSON().requestId === captured.requestId, { timeout: 10000 }),
    retry.click({ timeout: 5000 }),
  ]);
  assert.equal(acknowledged.status(), 200);
  assert.deepEqual(await acknowledged.json(), first.envelope);
  const remaining = replayDeadline - Date.now();
  assert.ok(remaining > 0, 'the exact rebind replay exceeded its original ten-second bound');
  await page.waitForFunction(requestId => ![...document.querySelectorAll('#graph-control-pending > li')]
    .some(row => row.dataset.graphControlRequestId === requestId), captured.requestId, { timeout: remaining });
  assert.equal(advances.length, 4);
  assert.equal(first.raw, advances[3].raw, 'lost-reply retry cannot recapture node IDs, versions, input, epoch or the newer empty selection');
  assert.deepEqual(first.envelope.data, advances[2].envelope.data);
  assert.deepEqual(first.envelope.data, advances[3].envelope.data);
  assert.equal(await page.locator('#graph-control-status').textContent(), newerStatus);
  await page.waitForFunction(id => document.getElementById('graph-rebind-receipts')?.textContent.includes(id), captured.requestId, { timeout: 5000 });
  const receipt = await page.locator('#graph-rebind-receipts').textContent();
  assert.match(receipt, /version 1.*version 2/);
  assert.ok(receipt.includes(receiver.inputId));
  assert.match(receipt, /not.*launch|launch.*separately/i);
  const prepared = await pollOwner(owner, '/graphs/' + graphId, data => data.nodes.find(node => node.id === receiver.id)?.taskVersion === '2'
    && data.nodes.find(node => node.id === receiver.id)?.inputId !== null && data.activeClaims.length === 0, { timeoutMs: 30000 });
  const newNode = prepared.nodes.find(node => node.id === receiver.id);
  assert.notEqual(newNode.inputId, receiver.inputId);
  assert.equal(newNode.nodeAdmissionId, null);
  assert.deepEqual(budget(prepared), budget(changed));
  const newInput = await read(owner, '/graphs/' + graphId + '/inputs/' + newNode.inputId);
  assert.equal(newInput.summary.state, 'ready');
  assert.equal(newInput.inputBinding.receivingTaskVersion, '2');
  assert.equal(newInput.inputBinding.contributorVector.length, 1);
  assert.equal(newInput.inputBinding.contributorVector[0].taskVersion, '2');
  const version = await read(owner, '/tasks/' + receiver.taskId + '/versions/2');
  assert.equal(version.version.createdBy, 'dependency_rebind');
  assert.deepEqual({ ...version.version, version: oldVersion.version.version, createdBy: oldVersion.version.createdBy, createdAt: oldVersion.version.createdAt }, oldVersion.version);
  assert.deepEqual(await read(owner, '/tasks/' + receiver.taskId + '/versions/1'), oldVersion);
  const retained = await read(owner, '/tasks/' + receiver.taskId + '/workflow?workflowId=' + oldWorkflow.workflow.id);
  assert.equal(retained.workflow.id, oldWorkflow.workflow.id);
  assert.deepEqual(retained.workflow.authority, oldWorkflow.workflow.authority);
  assert.deepEqual((await read(owner, '/tasks/' + receiver.taskId + '/workflow/steps?workflowId=' + oldWorkflow.workflow.id)).items, oldSteps.items);
  const current = await read(owner, '/tasks/' + receiver.taskId + '/workflow');
  // Rebind derives V2 and retains its input; workflow installation belongs to
  // the later native-admission transaction. With zero remaining allowance,
  // there is no new workflow/step/claim to manufacture here.
  assert.equal(current.taskId, receiver.taskId);
  assert.equal(current.taskVersion, '2');
  assert.equal(current.workflow, null);
  assert.equal(newNode.nodeAdmissionId, null);
  assert.deepEqual((await read(owner, '/graphs/' + graphId + '/inputs/' + receiver.inputId)).inputBinding, oldInput.inputBinding);
  const history = await read(owner, '/tasks/' + receiver.taskId + '/attempts');
  assert.equal(history.items.length, 1);
  assert.equal(history.items[0].id, oldAttempt.id);
  assert.equal(history.items[0].responseArtifactId, oldAttempt.responseArtifactId);
  assert.equal(history.items[0].outcome, 'succeeded');
  assert.equal(await page.locator('#task-text').inputValue(), standalone);
  assert.deepEqual(await readFile(path.join(f.project, '.git', 'index')), f.indexBefore);
  assert.equal(await readFile(path.join(f.project, 'hello.txt'), 'utf8'), 'unstaged\n');
});
