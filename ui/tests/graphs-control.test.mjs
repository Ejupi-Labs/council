import assert from 'node:assert/strict';
import test from 'node:test';
import { fixture, fixtureRequestId, openWorkspace, ownerAPI, pollOwner, reportFixture, responseBarrier } from './helpers.mjs';

async function arrival(barrier) {
  let timer;
  try { await Promise.race([barrier.arrived, new Promise((_, reject) => { timer = setTimeout(() => reject(new Error('The controlled graph acknowledgement was not reached')), 10000); })]); }
  finally { clearTimeout(timer); }
}

test('graph controls retain exact Stop and grant intent across lost replies, new selection and failed ledger paging', { timeout: 90000 }, async t => {
  const stopReply = responseBarrier(), grantReply = responseBarrier();
  const f = await fixture();
  t.after(async () => { stopReply.release(); grantReply.release(); await f.close(); });
  const owner = await f.start();
  const opened = await ownerAPI(owner, '/projects/open', 'POST', { requestId: fixtureRequestId(), selectedPath: f.project, basePath: f.project });
  assert.equal(opened.status, 201);
  const projectId = opened.envelope.data.project.id;
  for (const context of ['codex-native', 'claude-native']) assert.equal((await reportFixture(owner, context, 'occupancy', 'busy')).status, 201);
  const groups = [];
  for (const title of ['Controlled group A', 'Controlled group B']) {
    const created = await ownerAPI(owner, '/graphs', 'POST', { requestId: fixtureRequestId(), projectId, title, executionMode: 'fake',
      nodes: [{ key: 'one', originalText: 'First complete controlled original.', fakeScenario: 'graph-response' }, { key: 'two', originalText: 'Independent complete controlled original.', fakeScenario: 'graph-response' }] });
    assert.equal(created.status, 201, JSON.stringify(created.envelope.error));
    const graph = created.envelope.data.graph;
    await pollOwner(owner, '/graphs/' + graph.id + '/inputs', data => data.items.length === 3 && data.items.every(input => input.state === 'ready'), { timeoutMs: 30000 });
    groups.push((await ownerAPI(owner, '/graphs/' + graph.id)).envelope.data);
  }
  const [a, b] = groups, page = await openWorkspace(f, owner, { fake: false, workflow: 'auto' });
  await page.locator('#open-graph-list').click({ timeout: 5000 });
  const select = async graph => {
    await page.locator('#graph-list button').filter({ hasText: graph.graph.id }).click({ timeout: 5000 });
    await page.waitForFunction(id => document.getElementById('graph-facts')?.textContent.includes(id), graph.graph.id, { timeout: 5000 });
  };
  await select(a);
  assert.equal(await page.locator('#graph-start').isDisabled(), true, 'the general activation gate must remain off');
  const node = a.nodes.find(node => node.key === 'one');
  const nodePath = '/api/v1/graphs/' + a.graph.id + '/nodes/one/stop';
  const nodeRequests = []; let changedVersion = false, savedNodeReply;
  await page.route('**' + nodePath, async route => {
    nodeRequests.push(route.request().postDataJSON());
    const response = await route.fetch(), envelope = await response.json();
    assert.ok(envelope.data.node, 'the real node Stop receipt must identify its actual target');
    if (!changedVersion) { savedNodeReply = structuredClone(envelope); changedVersion = true; envelope.data.node.taskVersion = (BigInt(envelope.data.node.taskVersion) + 1n).toString(); }
    await route.fulfill({ response, json: envelope });
  });
  const nodeSent = page.waitForRequest(request => request.method() === 'POST' && new URL(request.url()).pathname === '/api/v1/graphs/' + a.graph.id + '/nodes/one/stop');
  await page.getByRole('button', { name: 'Stop node one', exact: true }).click({ timeout: 5000 });
  const nodeBody = (await nodeSent).postDataJSON();
  assert.deepEqual(Object.keys(nodeBody).sort(), ['expectedGraphControlRevision', 'expectedNodeControlRevision', 'nodeId', 'requestId']);
  assert.equal(nodeBody.nodeId, node.id); assert.equal(nodeBody.expectedNodeControlRevision, node.controlRevision);
  const retryNode = page.locator('#graph-control-pending').getByRole('button', { name: 'Retry exact Stop node one', exact: true });
  await retryNode.waitFor({ timeout: 5000 });
  assert.match(await page.locator('#graph-control-pending').textContent(), new RegExp(nodeBody.requestId));
  // A busy label is not an acknowledgement. Join the exact replay and row
  // removal within the existing five-second observation bound.
  const nodeDeadline = Date.now() + 5000;
  const [nodeAck] = await Promise.all([
    page.waitForResponse(response => response.request().method() === 'POST' && new URL(response.url()).pathname === nodePath
      && response.request().postDataJSON().requestId === nodeBody.requestId, { timeout: 5000 }),
    retryNode.click({ timeout: 5000 }),
  ]);
  assert.equal(nodeAck.status(), 202);
  assert.deepEqual(await nodeAck.json(), savedNodeReply);
  const nodeRemaining = nodeDeadline - Date.now();
  assert.ok(nodeRemaining > 0, 'the exact node Stop replay exceeded its original bound');
  await page.waitForFunction(requestId => ![...document.querySelectorAll('#graph-control-pending > li')]
    .some(row => row.dataset.graphControlRequestId === requestId), nodeBody.requestId, { timeout: nodeRemaining });
  assert.equal(nodeRequests.length, 2); assert.deepEqual(nodeRequests[0], nodeRequests[1], 'a changed-version acknowledgement must retain the exact original Stop request');
  const stoppedNode = await pollOwner(owner, '/graphs/' + a.graph.id, data => data.nodes.find(item => item.id === node.id)?.stop !== null);
  assert.notEqual(stoppedNode.nodes.find(item => item.key === 'two').state, 'cancelled');

  const stopPath = '/api/v1/graphs/' + a.graph.id + '/stop';
  const stops = []; let lostStop = false, savedStopReply;
  await page.route('**' + stopPath, async route => {
    stops.push(route.request().postDataJSON());
    const response = await route.fetch(), envelope = await response.json();
    if (!lostStop) { savedStopReply = envelope; lostStop = true; await stopReply.hold(); await route.abort('failed'); }
    else await route.fulfill({ response });
  });
  await page.locator('#graph-stop-group').click({ timeout: 5000 });
  await arrival(stopReply);
  await select(b);
  const bStatus = await page.locator('#graph-control-status').textContent();
  stopReply.release();
  const retryStop = page.locator('#graph-control-pending').getByRole('button', { name: /Retry exact Stop group/ });
  await retryStop.waitFor({ timeout: 5000 });
  assert.equal(await page.locator('#graph-control-status').textContent(), bStatus, 'late A feedback must not overwrite B');
  const stopDeadline = Date.now() + 5000;
  const [stopAck] = await Promise.all([
    page.waitForResponse(response => response.request().method() === 'POST' && new URL(response.url()).pathname === stopPath
      && response.request().postDataJSON().requestId === stops[0].requestId, { timeout: 5000 }),
    retryStop.click({ timeout: 5000 }),
  ]);
  assert.equal(stopAck.status(), 202);
  assert.deepEqual(await stopAck.json(), savedStopReply);
  const stopRemaining = stopDeadline - Date.now();
  assert.ok(stopRemaining > 0, 'the exact group Stop replay exceeded its original bound');
  await page.waitForFunction(requestId => ![...document.querySelectorAll('#graph-control-pending > li')]
    .some(row => row.dataset.graphControlRequestId === requestId), stops[0].requestId, { timeout: stopRemaining });
  assert.equal(stops.length, 2); assert.deepEqual(stops[0], stops[1]);
  assert.deepEqual(Object.keys(stops[0]).sort(), ['expectedGraphControlRevision', 'requestId']);
  assert.ok((await page.locator('#graph-facts').textContent()).includes(b.graph.id));

  const grantPath = '/api/v1/graphs/' + b.graph.id + '/budget-amendments';
  let grantBody;
  await page.route('**' + grantPath, async route => {
    grantBody = route.request().postDataJSON();
    const response = await route.fetch(); await grantReply.hold(); await route.fulfill({ response });
  });
  await page.locator('#graph-grant-admissions').fill('1');
  await page.locator('#graph-grant-budget').click({ timeout: 5000 });
  await arrival(grantReply);
  await page.locator('#graph-grant-admissions').fill('2');
  const newerStatus = await page.locator('#graph-control-status').textContent();
  const grantDeadline = Date.now() + 5000;
  const grantResponse = page.waitForResponse(response => response.request().method() === 'POST' && new URL(response.url()).pathname === grantPath
    && response.request().postDataJSON().requestId === grantBody.requestId, { timeout: 5000 });
  grantReply.release();
  const grantAck = await grantResponse;
  assert.equal(grantAck.status(), 200);
  assert.equal((await grantAck.json()).receipt.requestId, grantBody.requestId);
  const grantRemaining = grantDeadline - Date.now();
  assert.ok(grantRemaining > 0, 'the exact Grant acknowledgement exceeded its original bound');
  await page.waitForFunction(requestId => ![...document.querySelectorAll('#graph-control-pending > li')]
    .some(row => row.dataset.graphControlRequestId === requestId), grantBody.requestId, { timeout: grantRemaining });
  assert.equal(await page.locator('#graph-grant-admissions').inputValue(), '2');
  assert.equal(await page.locator('#graph-control-status').textContent(), newerStatus, 'old grant success cannot claim newer draft was saved');
  assert.equal(grantBody.additionalAdmissions, '1');
  assert.equal(grantBody.expectedGraphControlRevision, b.graph.controlRevision);
  assert.equal(grantBody.expectedBudgetAuthorityEpoch, b.graph.budgetAuthorityEpoch);
  assert.equal(grantBody.expectedWindowNumber, b.budget.budget.currentWindow.number);
  assert.deepEqual(Object.keys(grantBody).sort(), ['additionalAdmissions', 'expectedBudgetAuthorityEpoch', 'expectedGraphControlRevision', 'expectedWindowNumber', 'requestId']);
  const amended = await ownerAPI(owner, '/graphs/' + b.graph.id);
  assert.equal(amended.envelope.data.budget.budget.consumedAdmissions, '0');
  assert.equal(amended.envelope.data.budget.budget.currentWindow.deadlineAt, null);
  assert.equal(amended.envelope.data.dispatchAuthority.state, 'suspended', 'a grant cannot silently resume execution');

  const ledgerPath = '/api/v1/graphs/' + b.graph.id + '/budget-ledger';
  let firstPage = true, failedMore = false; const moreQueries = [];
  await page.route('**' + ledgerPath + '?*', async route => {
    const url = new URL(route.request().url());
    if (firstPage) { firstPage = false; url.searchParams.set('limit', '1'); await route.fulfill({ response: await route.fetch({ url: url.href }) }); return; }
    moreQueries.push(url.search);
    if (!failedMore) { failedMore = true; await route.abort('failed'); return; }
    await route.fulfill({ response: await route.fetch() });
  });
  await page.locator('#graph-ledger-load').click({ timeout: 5000 });
  await page.waitForFunction(() => document.querySelectorAll('#graph-ledger-list > li').length === 1, undefined, { timeout: 5000 });
  const firstRow = await page.locator('#graph-ledger-list > li').textContent();
  await page.locator('#graph-ledger-more').click({ timeout: 5000 });
  await page.locator('#graph-ledger-status').filter({ hasText: /retained|preserved/i }).waitFor({ timeout: 5000 });
  assert.equal(await page.locator('#graph-ledger-list > li').textContent(), firstRow);
  await page.locator('#graph-ledger-more').click({ timeout: 5000 });
  await page.waitForFunction(() => document.querySelectorAll('#graph-ledger-list > li').length >= 2, undefined, { timeout: 5000 });
  assert.equal(moreQueries.length, 2); assert.equal(moreQueries[0], moreQueries[1], 'failed page retry must keep the exact cursor');
});
