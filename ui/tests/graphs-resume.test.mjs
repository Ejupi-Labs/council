import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import path from 'node:path';
import test from 'node:test';
import { fixture, openWorkspace, ownerAPI, pollOwner, responseBarrier } from './helpers.mjs';

async function read(owner, route) {
  const result = await ownerAPI(owner, route);
  assert.equal(result.status, 200, JSON.stringify(result.envelope.error));
  return result.envelope.data;
}

async function arrival(barrier) {
  let timer;
  try { await Promise.race([barrier.arrived, new Promise((_, reject) => { timer = setTimeout(() => reject(new Error('The real guidance acknowledgement was not reached.')), 10000); })]); }
  finally { clearTimeout(timer); }
}

function authority(detail) {
  return detail.nodes.map(node => ({ nodeId: node.id, taskVersion: node.taskVersion, nodeControlRevision: node.controlRevision }));
}

function stableBudget(detail) {
  const budget = detail.budget.budget;
  return { owner: budget.taskId, granted: budget.grantedAdmissions, consumed: budget.consumedAdmissions, remaining: budget.remainingAdmissions,
    epoch: detail.budget.budgetAuthorityEpoch, window: budget.currentWindow.number, started: budget.currentWindow.firstAdmissionAt, deadline: budget.currentWindow.deadlineAt };
}

async function startOwnedResponseGroup(f, owner) {
  const page = await openWorkspace(f, owner, { fake: false, workflow: 'auto' });
  const standalone = 'A separate standalone draft 東京🙂';
  await page.locator('#task-text').fill(standalone);
  await page.locator('#open-graph-composer').click({ timeout: 5000 });
  await page.locator('#graph-title').fill('Actual guidance and explicit Advance');
  const nodes = page.locator('#graph-editor-nodes > [data-graph-node]');
  assert.equal(await nodes.count(), 2);
  for (let i = 0; i < 2; i++) {
    const node = nodes.nth(i);
    await node.locator('[data-graph-field="originalText"]').fill('Complete original for independent response ' + (i + 1) + ' 東京🙂');
    await node.locator('summary').filter({ hasText: /^Task details \(optional\)$/ }).click({ timeout: 5000 });
    await node.locator('[data-graph-field="harness"]').selectOption(i === 0 ? 'codex' : 'claude-code');
    await node.locator('[data-graph-field="deliverable"]').selectOption('response_only');
  }
  await page.locator('summary').filter({ hasText: /^Group details \(optional\)$/ }).click({ timeout: 5000 });
  await page.locator('#graph-admissions').fill('2');
  await page.locator('#graph-window').fill('900');
  let admission, originalBody;
  // Only this deterministic test bridge changes execution to the fixed offline
  // workers. Guidance and Advance below reach their actual routes unchanged.
  await page.route('**/api/v1/graphs', async route => {
    if (route.request().method() !== 'POST') { await route.continue(); return; }
    assert.equal(new URL(route.request().url()).pathname, '/api/v1/graphs');
    originalBody = route.request().postDataJSON();
    const response = await route.fetch({ postData: JSON.stringify({ ...originalBody, executionMode: 'fake',
      nodes: originalBody.nodes.map(node => ({ ...node, fakeScenario: 'graph-response' })) }) });
    const envelope = await response.json();
    assert.equal(response.status(), 201, JSON.stringify(envelope.error));
    assert.equal(envelope.receipt.requestId, originalBody.requestId);
    admission = envelope.data;
    await route.fulfill({ response });
  });
  await page.locator('#graph-start').click({ timeout: 5000 });
  await page.locator('#graph-start-status').filter({ hasText: /^Task group admitted\./ }).waitFor({ timeout: 10000 });
  const graphId = admission.graph.id;
  const detail = await pollOwner(owner, '/graphs/' + graphId, data => data.nodes.every(node => node.latestAttemptId !== null), { timeoutMs: 30000 });
  const terminal = [];
  for (const node of detail.nodes) {
    const attempt = await pollOwner(owner, '/attempts/' + node.latestAttemptId, data => data.phase === 'settled', { timeoutMs: 30000 });
    assert.equal(attempt.outcome, 'succeeded');
    assert.equal(attempt.executionMode, 'fake');
    assert.equal(attempt.cleanup.state, 'verified');
    assert.ok(attempt.startedAt);
    const dispatches = await pollOwner(owner, '/graphs/' + graphId + '/dispatches?nodeKey=' + encodeURIComponent(node.key),
      data => data.items.length === 1 && data.items[0].outputObservation?.response?.state === 'complete', { timeoutMs: 10000 });
    assert.equal(dispatches.items[0].dispatch.attemptId, attempt.id);
    terminal.push(attempt);
  }
  const settled = await read(owner, '/graphs/' + graphId);
  assert.equal(settled.budget.budget.consumedAdmissions, '2');
  assert.equal(settled.budget.budget.remainingAdmissions, '0');
  assert.equal(settled.activeClaims.length, 0);
  await page.locator('#graph-refresh').click({ timeout: 5000 });
  await page.waitForFunction(() => document.getElementById('graph-facts')?.textContent.includes('2 consumed / 2 granted'), undefined, { timeout: 5000 });
  return { page, admission, detail: settled, terminal, standalone };
}

test('graph guidance keeps exact pending intent and Advance prepares only the new version under shared allowance', { timeout: 90000 }, async t => {
  const delayed = responseBarrier(), f = await fixture();
  t.after(async () => { delayed.release(); await f.close(); });
  const owner = await f.start(), started = await startOwnedResponseGroup(f, owner), { page } = started;
  const graphId = started.detail.graph.id, [a, b] = started.detail.nodes;
  const originalAttempt = started.terminal.find(attempt => attempt.taskId === a.taskId);
  const originalVersion = await read(owner, '/tasks/' + a.taskId + '/versions/1');
  const oldInput = await read(owner, '/graphs/' + graphId + '/inputs/' + a.inputId);
  const guidance = 'Complete new guidance café🙂\nReturn a newly scoped response; keep all prior originals.';
  const newerB = 'Unsent guidance for the other node 東京\nThis newer text is separate.';
  const guidancePath = '/api/v1/graphs/' + graphId + '/nodes/' + a.key + '/guidance';
  const guidanceRequests = [], guidanceReplies = [];
  await page.route('**' + guidancePath, async route => {
    guidanceRequests.push({ raw: route.request().postData(), body: route.request().postDataJSON() });
    const response = await route.fetch(), envelope = await response.json();
    assert.equal(response.status(), 200, JSON.stringify(envelope.error));
    assert.equal(envelope.receipt.requestId, guidanceRequests.at(-1).body.requestId);
    guidanceReplies.push(envelope);
    if (guidanceReplies.length === 1) { await delayed.hold(); await route.abort('failed'); }
    else await route.fulfill({ response });
  });
  // This selector is absent in the initial source. No standalone guidance
  // route, DOM enable override or fabricated response substitutes for it.
  await page.locator('#graph-guidance-node').selectOption(a.id, { timeout: 5000 });
  await page.locator('#graph-guidance-text').fill(guidance);
  await page.locator('#graph-save-guidance').click({ timeout: 5000 });
  await arrival(delayed);
  const first = guidanceRequests[0].body;
  assert.deepEqual(Object.keys(first).sort(), ['expectedGraphControlRevision', 'expectedNodeControlRevision', 'expectedRevision', 'guidanceText', 'nodeId', 'requestId', 'taskVersion']);
  assert.equal(first.nodeId, a.id);
  assert.equal(first.taskVersion, '1');
  assert.equal(first.expectedRevision, a.taskRevision);
  assert.equal(first.expectedNodeControlRevision, a.controlRevision);
  assert.equal(first.expectedGraphControlRevision, started.detail.graph.controlRevision);
  assert.equal(first.guidanceText, guidance);
  await page.locator('#graph-guidance-node').selectOption(b.id);
  await page.locator('#graph-guidance-text').fill(newerB);
  const newerStatus = await page.locator('#graph-control-status').textContent();
  delayed.release();
  const retry = page.locator('#graph-control-pending').getByRole('button', { name: 'Retry exact Guidance node ' + a.key, exact: true });
  await retry.waitFor({ timeout: 5000 });
  assert.equal(await page.locator('#graph-guidance-node').inputValue(), b.id);
  assert.equal(await page.locator('#graph-guidance-text').inputValue(), newerB);
  assert.equal(await page.locator('#graph-control-status').textContent(), newerStatus);
  // The busy label changes before a reply arrives. Observe this exact replay
  // and pending identity under one immutable original ten-second bound.
  const replayDeadline = Date.now() + 10000;
  const [acknowledged] = await Promise.all([
    page.waitForResponse(response => response.request().method() === 'POST'
      && new URL(response.url()).pathname === guidancePath
      && response.request().postDataJSON().requestId === first.requestId, { timeout: 10000 }),
    retry.click({ timeout: 5000 }),
  ]);
  assert.equal(acknowledged.status(), 200);
  assert.deepEqual(await acknowledged.json(), guidanceReplies[0]);
  const remaining = replayDeadline - Date.now();
  assert.ok(remaining > 0, 'the exact guidance replay exceeded its original ten-second bound');
  await page.waitForFunction(requestId => ![...document.querySelectorAll('#graph-control-pending > li')]
    .some(row => row.dataset.graphControlRequestId === requestId), first.requestId, { timeout: remaining });
  assert.equal(guidanceRequests.length, 2);
  assert.equal(guidanceRequests[0].raw, guidanceRequests[1].raw, 'pending retry cannot read a newer proof or overwrite its old captured guidance');
  assert.deepEqual(guidanceReplies[0].data, guidanceReplies[1].data);
  assert.equal(await page.locator('#graph-guidance-node').inputValue(), b.id);
  assert.equal(await page.locator('#graph-guidance-text').inputValue(), newerB);
  assert.equal(await page.locator('#graph-control-status').textContent(), newerStatus, 'late A success must not claim B typing was saved');
  const guided = await pollOwner(owner, '/graphs/' + graphId, data => data.nodes.find(node => node.id === a.id)?.taskVersion === '2');
  const currentA = guided.nodes.find(node => node.id === a.id), currentB = guided.nodes.find(node => node.id === b.id);
  assert.equal(currentA.inputId, null);
  assert.equal(currentA.nodeAdmissionId, null);
  assert.equal(currentB.taskVersion, '1');
  assert.deepEqual(stableBudget(guided), stableBudget(started.detail), 'guidance cannot grant units, renew a window or debit another model stage');
  assert.equal(guided.dispatchAuthority.generation, started.detail.dispatchAuthority.generation);
  assert.deepEqual(await read(owner, '/tasks/' + a.taskId + '/versions/1'), originalVersion);
  const task = await read(owner, '/tasks/' + a.taskId);
  assert.equal(task.version.version, '2');
  assert.equal(task.version.guidanceText, guidance);
  assert.equal(task.version.pins.harnessId, originalVersion.version.pins.harnessId);
  assert.deepEqual(task.version.acceptance, originalVersion.version.acceptance);
  const retainedOldInput = await read(owner, '/graphs/' + graphId + '/inputs/' + a.inputId);
  assert.deepEqual(retainedOldInput.inputBinding, oldInput.inputBinding);
  assert.equal((await read(owner, '/attempts/' + originalAttempt.id)).responseArtifactId, originalAttempt.responseArtifactId);
  assert.equal((await read(owner, '/tasks/' + a.taskId + '/attempts')).items.length, 1);
  assert.equal((await read(owner, '/tasks/' + b.taskId + '/attempts')).items.length, 1);

  const advances = [];
  await page.route('**/api/v1/graphs/' + graphId + '/advance', async route => {
    const body = route.request().postDataJSON(), response = await route.fetch(), envelope = await response.json();
    assert.equal(response.status(), 200, JSON.stringify(envelope.error));
    assert.equal(envelope.receipt.requestId, body.requestId);
    advances.push({ body, data: envelope.data });
    await route.fulfill({ response });
  });
  const refresh = async version => {
    await page.locator('#graph-refresh').click({ timeout: 5000 });
    await page.waitForFunction(({ key, version }) => document.querySelector('#graph-node-list [data-node-key="' + key + '"]')?.textContent.includes('Current version ' + version), { key: a.key, version }, { timeout: 5000 });
  };
  await refresh('2');
  await page.locator('#graph-advance').click({ timeout: 5000 });
  await page.waitForFunction(() => document.getElementById('graph-control-status')?.textContent.includes('Advance acknowledged'), undefined, { timeout: 10000 });
  assert.equal(advances.length, 1);
  const firstAdvance = advances[0];
  assert.deepEqual(Object.keys(firstAdvance.body).sort(), ['expectedBudgetAuthorityEpoch', 'expectedGraphControlRevision', 'expectedWindowNumber', 'graphVersion', 'nodeAuthorities', 'requestId']);
  assert.equal(firstAdvance.body.graphVersion, guided.graph.graphVersion);
  assert.equal(firstAdvance.body.expectedGraphControlRevision, guided.graph.controlRevision);
  assert.equal(firstAdvance.body.expectedBudgetAuthorityEpoch, guided.graph.budgetAuthorityEpoch);
  assert.equal(firstAdvance.body.expectedWindowNumber, guided.budget.budget.currentWindow.number);
  assert.deepEqual(firstAdvance.body.nodeAuthorities, authority(guided));
  assert.equal(Object.hasOwn(firstAdvance.body, 'rebindNodeIds'), false, 'ordinary Advance cannot silently derive another node version');
  assert.equal(firstAdvance.data.preparationAttemptIds.length, 1);
  assert.deepEqual(firstAdvance.data.dispatches, []);
  assert.deepEqual(firstAdvance.data.reboundNodes, []);
  // Establish the actual sealed-input budget block before a grant can change
  // the observed state; a non-null input ID alone can still be preparing.
  const prepared = await pollOwner(owner, '/graphs/' + graphId, data => {
    const node = data.nodes.find(node => node.id === a.id);
    return node?.taskVersion === '2' && node.taskId === a.taskId && node.inputId !== null && node.nodeAdmissionId === null
      && node.state === 'blocked' && data.activeClaims.length === 0
      && data.blockers.some(blocker => blocker.code === 'budget_exhausted' && blocker.nodeId === node.id && blocker.inputId === node.inputId);
  }, { timeoutMs: 30000 });
  const preparedA = prepared.nodes.find(node => node.id === a.id);
  assert.notEqual(preparedA.inputId, a.inputId);
  assert.equal(preparedA.taskVersion, '2');
  const input = await pollOwner(owner, '/graphs/' + graphId + '/inputs/' + preparedA.inputId, data =>
    data.summary.id === preparedA.inputId && data.summary.state === 'ready' && data.summary.current === true
    && data.summary.receivingNodeId === a.id && data.summary.receivingTaskVersion === '2'
    && data.inputBinding !== null && data.inputBinding.id === preparedA.inputId
    && data.inputBinding.receivingNodeId === a.id && data.inputBinding.receivingTaskId === a.taskId
    && data.inputBinding.receivingTaskVersion === '2', { timeoutMs: 30000 });
  assert.equal(input.summary.state, 'ready');
  assert.equal(input.inputBinding.receivingTaskVersion, '2');
  assert.deepEqual(stableBudget(prepared), stableBudget(guided));
  assert.equal((await read(owner, '/tasks/' + a.taskId + '/attempts')).items.length, 1, 'zero balance permits local input preparation, not a fresh model admission');

  await page.locator('#graph-grant-admissions').fill('1');
  await page.locator('#graph-grant-budget').click({ timeout: 5000 });
  await page.waitForFunction(() => document.getElementById('graph-control-status')?.textContent.includes('Shared allowance grant recorded'), undefined, { timeout: 10000 });
  const granted = await read(owner, '/graphs/' + graphId);
  assert.equal(granted.budget.budget.grantedAdmissions, '3');
  assert.equal(granted.budget.budget.consumedAdmissions, '2');
  assert.equal(granted.budget.budget.currentWindow.deadlineAt, started.detail.budget.budget.currentWindow.deadlineAt);
  assert.equal(granted.nodes.find(node => node.id === a.id).taskVersion, '2');
  assert.equal((await read(owner, '/tasks/' + a.taskId + '/attempts')).items.length, 1, 'a grant alone cannot resume the suspended registration');
  await refresh('2');
  await page.locator('#graph-advance').click({ timeout: 5000 });
  await pollOwner(owner, '/tasks/' + a.taskId + '/attempts', data => data.items.length === 2 && data.items.every(attempt => attempt.phase === 'settled'), { timeoutMs: 30000 });
  assert.equal(advances.length, 2);
  assert.notEqual(advances[1].body.requestId, firstAdvance.body.requestId);
  assert.equal(advances[1].body.expectedBudgetAuthorityEpoch, granted.graph.budgetAuthorityEpoch);
  assert.equal(Object.hasOwn(advances[1].body, 'rebindNodeIds'), false);
  const history = await read(owner, '/tasks/' + a.taskId + '/attempts');
  const fresh = history.items.find(attempt => attempt.id !== originalAttempt.id);
  assert.equal(fresh.taskVersion, '2');
  assert.equal(fresh.executionMode, 'fake');
  assert.equal(fresh.role, 'author');
  assert.equal(fresh.outcome, 'succeeded');
  assert.equal(fresh.cleanup.state, 'verified');
  assert.ok(fresh.startedAt);
  const final = await read(owner, '/graphs/' + graphId);
  assert.equal(final.budget.budget.consumedAdmissions, '3');
  assert.equal(final.budget.budget.currentWindow.deadlineAt, started.detail.budget.budget.currentWindow.deadlineAt);
  assert.equal(final.nodes.find(node => node.id === b.id).taskVersion, '1');
  assert.equal((await read(owner, '/tasks/' + b.taskId + '/attempts')).items.length, 1);
  assert.equal((await read(owner, '/attempts/' + originalAttempt.id)).responseArtifactId, originalAttempt.responseArtifactId);
  assert.equal(await page.locator('#graph-guidance-node').inputValue(), b.id);
  assert.equal(await page.locator('#graph-guidance-text').inputValue(), newerB);
  assert.equal(await page.locator('#task-text').inputValue(), started.standalone);
  assert.deepEqual(await readFile(path.join(f.project, '.git', 'index')), f.indexBefore);
  assert.equal(await readFile(path.join(f.project, 'hello.txt'), 'utf8'), 'unstaged\n');
});
