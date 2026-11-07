import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import path from 'node:path';
import test from 'node:test';
import { fixture, openWorkspace, ownerAPI, pollOwner, responseBarrier } from './helpers.mjs';

async function arrival(barrier) {
  let timer;
  try { await Promise.race([barrier.arrived, new Promise((_, reject) => { timer = setTimeout(() => reject(new Error('The real graph Start acknowledgement was not reached.')), 10000); })]); }
  finally { clearTimeout(timer); }
}

async function compose(f, owner) {
  const page = await openWorkspace(f, owner, { fake: false, workflow: 'auto' });
  const standalone = 'Unsent standalone original 東京🙂\nThis is a separate draft.';
  await page.locator('#task-text').fill(standalone);
  await page.locator('#open-graph-composer').click({ timeout: 5000 });
  const title = 'Actual UI declaration · two separate responses';
  const shared = '\ufeffComplete shared original 東京🙂\nPreserve the staged index and return separate reviewable results.';
  const originals = ['First complete instruction π\nReturn a response without changing source files.', 'Second complete instruction 東京\nKeep the shared finite allowance.'];
  await page.locator('#graph-title').fill(title);
  await page.locator('summary').filter({ hasText: /^Shared instructions \(optional\)$/ }).click({ timeout: 5000 });
  await page.locator('#graph-shared').fill(shared);
  const nodes = page.locator('#graph-editor-nodes > [data-graph-node]');
  assert.equal(await nodes.count(), 2);
  const keys = [];
  for (let i = 0; i < 2; i++) {
    const node = nodes.nth(i);
    keys.push(await node.getAttribute('data-graph-node'));
    await node.locator('[data-graph-field="originalText"]').fill(originals[i]);
    await node.locator('summary').filter({ hasText: /^Task details \(optional\)$/ }).click({ timeout: 5000 });
    await node.locator('[data-graph-field="harness"]').selectOption(i === 0 ? 'codex' : 'claude-code');
    await node.locator('[data-graph-field="deliverable"]').selectOption('response_only');
  }
  await page.locator('summary').filter({ hasText: /^Group details \(optional\)$/ }).click({ timeout: 5000 });
  await page.locator('#graph-admissions').fill('2');
  await page.locator('#graph-window').fill('900');
  assert.equal(await page.locator('#graph-final-node').inputValue(), '');
  assert.match(await page.locator('#graph-scope-preview').textContent(), /Separate reviewable results/);
  return { page, title, shared, originals, keys, standalone };
}

// The browser declaration is captured unchanged. Only this test-owned bridge
// selects the fixed offline workers; it never forwards a native-default body.
// Real HTTP admission, private preparation, receipts and worker results remain
// service-owned. This is not qualification of an installed provider executable.
async function bridge(page, { delayed } = {}) {
  const requests = [], forwarded = [], replies = [];
  await page.route('**/api/v1/graphs', async route => {
    const request = route.request();
    if (request.method() !== 'POST') { await route.continue(); return; }
    assert.equal(new URL(request.url()).pathname, '/api/v1/graphs');
    const raw = request.postData(), body = request.postDataJSON();
    assert.equal(body.executionMode, undefined);
    assert.ok(body.nodes.every(node => node.executionMode === undefined && node.fakeScenario === undefined));
    requests.push({ raw, body });
    const fixtureBody = { ...body, executionMode: 'fake', nodes: body.nodes.map(node => ({ ...node, fakeScenario: 'graph-response' })) };
    const encoded = JSON.stringify(fixtureBody);
    forwarded.push(encoded);
    const response = await route.fetch({ postData: encoded });
    const envelope = await response.json();
    assert.equal(response.status(), 201, JSON.stringify(envelope.error));
    assert.equal(envelope.receipt?.requestId, body.requestId);
    replies.push(envelope);
    if (delayed && replies.length === 1) { await delayed.hold(); await route.abort('failed'); }
    else await route.fulfill({ response });
  });
  return { requests, forwarded, replies };
}

function assertDeclaration(captured, draft) {
  assert.equal(captured.title, draft.title);
  assert.equal(captured.sharedInstructions, draft.shared);
  assert.deepEqual(captured.nodes.map(node => node.key), draft.keys);
  assert.deepEqual(captured.nodes.map(node => node.originalText), draft.originals);
  assert.deepEqual(captured.nodes.map(node => node.pins.harnessId), ['codex', 'claude-code']);
  assert.ok(captured.nodes.every(node => node.acceptance.deliverable === 'response_only'));
  assert.deepEqual(captured.dependencies, []);
  assert.equal(captured.maxParallelism, '2');
  assert.equal(captured.routing.budget.admissionLimit, '2');
  assert.equal(captured.routing.budget.windowSeconds, 900);
  assert.equal(captured.finalNodeKey, undefined);
  assert.equal(captured.delivery, undefined);
  assert.match(captured.expectedPolicyRevision, /^[1-9][0-9]*$/);
  assert.match(captured.routing.expectedPolicyRevision, /^[1-9][0-9]*$/);
}

async function completedWorkers(owner, admission) {
  const graphId = admission.graph.id;
  const detail = await pollOwner(owner, '/graphs/' + graphId, data => data.nodes.every(node => node.latestAttemptId !== null), { timeoutMs: 30000 });
  const histories = [];
  for (const node of detail.nodes) {
    const history = await pollOwner(owner, '/tasks/' + node.taskId + '/attempts', data => data.items.length === 1 && data.items[0].phase === 'settled', { timeoutMs: 30000 });
    const attempt = history.items[0];
    assert.equal(history.nextCursor, null);
    assert.equal(attempt.id, node.latestAttemptId);
    assert.equal(attempt.executorKind, 'native');
    assert.equal(attempt.executionMode, 'fake');
    assert.equal(attempt.role, 'author');
    assert.equal(attempt.outcome, 'succeeded');
    assert.equal(attempt.cleanup.state, 'verified');
    assert.ok(attempt.startedAt, 'the fixed owned worker must actually pass GO');
    const dispatches = await pollOwner(owner, '/graphs/' + graphId + '/dispatches?nodeKey=' + encodeURIComponent(node.key),
      data => data.items.length === 1 && data.items[0].outputObservation !== null, { timeoutMs: 10000 });
    assert.equal(dispatches.items[0].dispatch.attemptId, attempt.id);
    assert.equal(dispatches.items[0].materialization?.state, 'complete');
    histories.push({ taskId: node.taskId, attemptId: attempt.id, dispatchId: dispatches.items[0].dispatch.id });
  }
  const budget = await ownerAPI(owner, '/graphs/' + graphId + '/budget');
  assert.equal(budget.status, 200);
  assert.equal(budget.envelope.data.budget.consumedAdmissions, '2');
  assert.equal(budget.envelope.data.budget.remainingAdmissions, '0');
  return { detail, histories, budget: budget.envelope.data.budget };
}

async function unchangedUserFiles(f) {
  assert.deepEqual(await readFile(path.join(f.project, '.git', 'index')), f.indexBefore);
  assert.equal(await readFile(path.join(f.project, 'hello.txt'), 'utf8'), 'unstaged\n');
}

test('the graph composer posts its exact declaration and observes real owned workers without inspection admissions', { timeout: 90000 }, async t => {
  const f = await fixture(t), owner = await f.start(), draft = await compose(f, owner);
  const { page } = draft, capture = await bridge(page);
  // The initial RED is the deliberate source gate. Do not force-enable a DOM
  // button or bypass the actual submit handler to manufacture an admission.
  await page.waitForFunction(() => !document.getElementById('graph-start')?.disabled, undefined, { timeout: 5000 });
  await page.locator('#graph-start').click({ timeout: 5000 });
  await page.locator('#graph-start-status').filter({ hasText: /^Task group admitted\./ }).waitFor({ timeout: 10000 });
  assert.equal(capture.requests.length, 1);
  assertDeclaration(capture.requests[0].body, draft);
  const admission = capture.replies[0].data, graphId = admission.graph.id;
  assert.equal(admission.nodes.length, 2);
  assert.equal(admission.graph.projectId, capture.requests[0].body.projectId);
  await page.waitForFunction(id => document.getElementById('graph-facts')?.textContent.includes(id), graphId, { timeout: 5000 });
  const before = await completedWorkers(owner, admission);
  let writes = 0;
  page.on('request', request => { if (!['GET', 'HEAD'].includes(request.method())) writes++; });
  await page.locator('#graph-refresh').click({ timeout: 5000 });
  await page.locator('#graph-overview > details > summary').filter({ hasText: 'Retained definition' }).click({ timeout: 5000 });
  await page.waitForFunction(text => document.getElementById('graph-definition')?.textContent.includes(text), draft.originals[1], { timeout: 5000 });
  const definition = await ownerAPI(owner, '/graphs/' + graphId + '/definition');
  assert.equal(definition.status, 200);
  const expected = { ...capture.requests[0].body, executionMode: 'fake', nodes: capture.requests[0].body.nodes.map(node => ({ ...node, fakeScenario: 'graph-response' })) };
  assert.deepEqual(definition.envelope.data.original, expected, 'retained originals must match the exact test-forwarded declaration');
  assert.equal(definition.envelope.data.delivery.localMode, 'review_only');
  for (let i = 0; i < admission.nodes.length; i++) {
    const node = admission.nodes[i], history = before.histories.find(item => item.taskId === node.taskId);
    await page.locator('#graph-node-list').getByRole('button', { name: 'Open task ' + node.key, exact: true }).click({ timeout: 5000 });
    await page.waitForFunction(id => document.getElementById('task-facts')?.textContent.includes(id), node.taskId, { timeout: 5000 });
    assert.equal(await page.locator('#task-original').textContent(), draft.originals[i]);
    await page.waitForFunction(id => document.getElementById('attempt-list')?.textContent.includes(id.slice(0, 8)), history.attemptId, { timeout: 5000 });
    assert.match(await page.locator('#attempt-list').textContent(), /Local fake/);
    assert.equal(await page.locator('#run-attempt').isDisabled(), true, 'graph children retain graph-scoped admission authority');
  }
  const after = await completedWorkers(owner, admission);
  assert.deepEqual(after.histories, before.histories);
  assert.equal(after.budget.consumedAdmissions, before.budget.consumedAdmissions);
  assert.equal(after.budget.currentWindow.deadlineAt, before.budget.currentWindow.deadlineAt);
  assert.equal(writes, 0, 'opening definition, task and execution history cannot admit another model stage');
  assert.equal(await page.locator('#task-text').inputValue(), draft.standalone);
  await unchangedUserFiles(f);
});

test('a lost real graph Start acknowledgement retries the captured body while preserving newer drafts', { timeout: 90000 }, async t => {
  const delayed = responseBarrier(), f = await fixture();
  t.after(async () => { delayed.release(); await f.close(); });
  const owner = await f.start(), draft = await compose(f, owner), { page } = draft;
  const capture = await bridge(page, { delayed });
  await page.waitForFunction(() => !document.getElementById('graph-start')?.disabled, undefined, { timeout: 5000 });
  await page.locator('#graph-start').click({ timeout: 5000 });
  await arrival(delayed);
  assertDeclaration(capture.requests[0].body, draft);
  const admission = capture.replies[0].data;
  const newerTitle = 'Newer unsent group draft 東京', newerOriginal = 'Newer original🙂\nThis must not be replaced by the older receipt.';
  await page.locator('#graph-title').fill(newerTitle);
  await page.locator('#graph-editor-nodes > [data-graph-node]').first().locator('[data-graph-field="originalText"]').fill(newerOriginal);
  const newerStatus = await page.locator('#graph-start-status').textContent();
  delayed.release();
  const retry = page.locator('#graph-pending').getByRole('button', { name: 'Retry exact group start', exact: true });
  await retry.waitFor({ timeout: 5000 });
  assert.match(await page.locator('#graph-pending').textContent(), new RegExp(capture.requests[0].body.requestId));
  assert.equal(await page.locator('#graph-title').inputValue(), newerTitle);
  assert.equal(await page.locator('#graph-start-status').textContent(), newerStatus, 'late request failure cannot paint over newer editor feedback');
  // A busy label appears before the replay is acknowledged. Join the exact
  // response and removal of its pending row within the original deadline.
  const replayDeadline = Date.now() + 10000;
  const [acknowledged] = await Promise.all([
    page.waitForResponse(response => response.request().method() === 'POST'
      && new URL(response.url()).pathname === '/api/v1/graphs'
      && response.request().postDataJSON().requestId === capture.requests[0].body.requestId, { timeout: 10000 }),
    retry.click({ timeout: 5000 }),
  ]);
  assert.equal(acknowledged.status(), 201);
  assert.deepEqual(await acknowledged.json(), capture.replies[0]);
  const remaining = replayDeadline - Date.now();
  assert.ok(remaining > 0, 'the exact group start replay exceeded its original ten-second bound');
  await page.waitForFunction(requestId => ![...document.querySelectorAll('#graph-pending > li')]
    .some(row => row.textContent.includes(requestId)), capture.requests[0].body.requestId, { timeout: remaining });
  assert.equal(capture.requests.length, 2);
  assert.equal(capture.requests[0].raw, capture.requests[1].raw, 'retry must preserve the exact original request serialization and request ID');
  assert.equal(capture.forwarded[0], capture.forwarded[1], 'the offline fixture transform must remain identical on replay');
  assert.equal(capture.replies[1].data.graph.id, admission.graph.id);
  assert.deepEqual(capture.replies[1].data, capture.replies[0].data, 'the actual service must return its immutable saved admission');
  assert.equal(await page.locator('#graph-title').inputValue(), newerTitle);
  assert.equal(await page.locator('#graph-editor-nodes > [data-graph-node]').first().locator('[data-graph-field="originalText"]').inputValue(), newerOriginal);
  assert.equal(await page.locator('#graph-start-status').textContent(), newerStatus, 'old success cannot claim the newer draft was admitted');
  assert.equal(await page.locator('#task-text').inputValue(), draft.standalone);
  const groups = await ownerAPI(owner, '/graphs?projectId=' + capture.requests[0].body.projectId);
  assert.equal(groups.status, 200);
  assert.deepEqual(groups.envelope.data.items.map(graph => graph.id), [admission.graph.id]);
  await completedWorkers(owner, admission);
  await unchangedUserFiles(f);
});
