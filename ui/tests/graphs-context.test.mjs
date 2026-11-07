import assert from 'node:assert/strict';
import test from 'node:test';
import { fixture, fixtureRequestId, openWorkspace, ownerAPI, pollOwner, reportFixture, responseBarrier } from './helpers.mjs';

async function createdGroup(f, owner, { review = false, busy = false } = {}) {
  const opened = await ownerAPI(owner, '/projects/open', 'POST', { requestId: fixtureRequestId(), selectedPath: f.project, basePath: f.project });
  assert.equal(opened.status, 201);
  if (busy) for (const context of ['codex-native', 'claude-native']) assert.equal((await reportFixture(owner, context, 'occupancy', 'busy')).status, 201);
  const original = '\ufeffComplete receiver original 東京🙂\r\nKeep working bytes and staged intent separate.';
  const shared = 'Complete graph-shared original π\r\nNo external publication.';
  const result = await ownerAPI(owner, '/graphs', 'POST', { requestId: fixtureRequestId(), projectId: opened.envelope.data.project.id,
    sharedInstructions: shared, executionMode: 'fake', nodes: [{ key: 'receiver', originalText: original, fakeScenario: 'graph-response',
      acceptance: { deliverable: 'response_only' }, ...(review ? { workflow: { review: 'required' } } : {}) }] });
  assert.equal(result.status, 201, JSON.stringify(result.envelope.error));
  const graphId = result.envelope.data.graph.id;
  const view = await pollOwner(owner, '/graphs/' + graphId, data => !!data.nodes[0].inputId, { timeoutMs: 30000 });
  const input = await pollOwner(owner, '/graphs/' + graphId + '/inputs/' + view.nodes[0].inputId, data => data.summary.state === 'ready', { timeoutMs: 30000 });
  return { graphId, input, node: view.nodes[0], original, shared };
}

async function openInputs(f, owner, graph) {
  const page = await openWorkspace(f, owner, { fake: false, workflow: 'auto' });
  await page.locator('#open-graph-list').click({ timeout: 5000 });
  await page.locator('#graph-list button').filter({ hasText: graph.graphId }).click({ timeout: 5000 });
  await page.getByRole('button', { name: 'Inputs and context receiver', exact: true }).click({ timeout: 5000 });
  await page.waitForFunction(id => document.getElementById('graph-input-facts')?.textContent.includes(id), graph.input.summary.id, { timeout: 5000 });
  return page;
}

test('retained input inspection preserves complete originals and exact failed member pages without implying materialization', { timeout: 90000 }, async t => {
  const f = await fixture(t), owner = await f.start();
  const graph = await createdGroup(f, owner, { busy: true });
  const page = await openInputs(f, owner, graph);
  assert.match(await page.locator('#graph-input-facts').textContent(), /Composed input ready/);
  assert.match(await page.locator('#graph-dispatch-facts').textContent(), /not observed|select an execution/i);
  assert.doesNotMatch(await page.locator('#graph-input-facts').textContent(), /Files materialized for receiving attempt/);
  assert.ok((await page.locator('#graph-context-access').textContent()).includes('receiver'));
  assert.match(await page.locator('#graph-context-access').textContent(), /inline|file/);
  let writes = 0;
  page.on('request', request => { if (!['GET', 'HEAD'].includes(request.method())) writes++; });
  await page.getByRole('button', { name: 'Read complete receiver original', exact: true }).click({ timeout: 5000 });
  await page.waitForFunction(text => document.getElementById('graph-input-artifact')?.textContent === text, graph.original, { timeout: 5000 });
  assert.match(await page.locator('#graph-input-artifact-status').textContent(), /complete|exact/i);
  await page.getByRole('button', { name: 'Read complete shared instructions', exact: true }).click({ timeout: 5000 });
  await page.waitForFunction(text => document.getElementById('graph-input-artifact')?.textContent === text, graph.shared, { timeout: 5000 });
  const membersPath = '/api/v1/graphs/' + graph.graphId + '/inputs/' + graph.input.summary.id + '/members';
  let first = true, failed = false; const queries = [];
  await page.route('**' + membersPath + '?*', async route => {
    const url = new URL(route.request().url());
    if (first) { first = false; url.searchParams.set('limit', '1'); await route.fulfill({ response: await route.fetch({ url: url.href }) }); return; }
    queries.push(url.search);
    if (!failed) { failed = true; await route.abort('failed'); return; }
    await route.fulfill({ response: await route.fetch() });
  });
  await page.locator('#graph-members-load').click({ timeout: 5000 });
  await page.waitForFunction(() => document.querySelectorAll('#graph-members-list > li').length === 1, undefined, { timeout: 5000 });
  const retained = await page.locator('#graph-members-list > li').textContent();
  assert.match(retained, /hello\.txt/);
  await page.locator('#graph-members-more').click({ timeout: 5000 });
  await page.locator('#graph-members-status').filter({ hasText: /retained|preserved/i }).waitFor({ timeout: 5000 });
  assert.equal(await page.locator('#graph-members-list > li').textContent(), retained);
  await page.locator('#graph-members-more').click({ timeout: 5000 });
  await page.waitForFunction(() => document.querySelectorAll('#graph-members-list > li').length >= 2, undefined, { timeout: 5000 });
  assert.equal(queries.length, 2); assert.equal(queries[0], queries[1]);
  assert.match(await page.locator('#graph-members-list').textContent(), /working/);
  assert.match(await page.locator('#graph-members-list').textContent(), /index/);
  assert.equal(writes, 0, 'inspecting input/context/member/raw sources must remain read-only');
});

test('an exact receiving dispatch rejects late and foreign materialization even when the input is reused', { timeout: 90000 }, async t => {
  const delayed = responseBarrier(), f = await fixture();
  t.after(async () => { delayed.release(); await f.close(); });
  const owner = await f.start(), graph = await createdGroup(f, owner, { review: true });
  const pageData = await pollOwner(owner, '/graphs/' + graph.graphId + '/dispatches?inputId=' + graph.input.summary.id,
    data => data.items.length === 2 && data.items.every(item => item.materialization?.state === 'complete'), { timeoutMs: 40000 });
  for (const item of pageData.items) await pollOwner(owner, '/attempts/' + item.dispatch.attemptId, data => data.phase === 'settled' && data.outcome === 'succeeded', { timeoutMs: 25000 });
  const settled = await ownerAPI(owner, '/graphs/' + graph.graphId + '/dispatches?inputId=' + graph.input.summary.id);
  assert.equal(settled.status, 200);
  assert.ok(settled.envelope.data.items.every(item => item.materialization?.state === 'complete' && item.outputObservation !== null), 'capture the actual settled append-once output facts before opening the inspector');
  const [author, reviewer] = settled.envelope.data.items;
  assert.notEqual(author.dispatch.attemptId, reviewer.dispatch.attemptId);
  assert.equal(author.dispatch.sourceBasis.basisKind, 'graph_input');
  assert.equal(reviewer.dispatch.sourceBasis.basisKind, 'node_subject');
  const page = await openInputs(f, owner, graph);
  const authorPath = '/api/v1/graphs/' + graph.graphId + '/dispatches/' + author.dispatch.id;
  let held = false;
  await page.route('**' + authorPath, async route => {
    const response = await route.fetch();
    if (!held) { held = true; await delayed.hold(); }
    await route.fulfill({ response });
  });
  await page.locator('#graph-dispatch-selector').selectOption(author.dispatch.id);
  let timer;
  try { await Promise.race([delayed.arrived, new Promise((_, reject) => { timer = setTimeout(() => reject(new Error('Actual author dispatch read was not held')), 10000); })]); }
  finally { clearTimeout(timer); }
  await page.locator('#graph-dispatch-selector').selectOption(reviewer.dispatch.id);
  await page.waitForFunction(id => document.getElementById('graph-dispatch-facts')?.textContent.includes(id), reviewer.dispatch.attemptId, { timeout: 5000 });
  delayed.release();
  await page.waitForFunction(id => document.getElementById('graph-dispatch-selector')?.value === id, reviewer.dispatch.id, { timeout: 5000 });
  assert.ok((await page.locator('#graph-dispatch-facts').textContent()).includes(reviewer.dispatch.attemptId));
  assert.match(await page.locator('#graph-dispatch-facts').textContent(), /node_subject/);
  const reviewerPath = '/api/v1/graphs/' + graph.graphId + '/dispatches/' + reviewer.dispatch.id;
  let fault = '';
  await page.route('**' + reviewerPath, async route => {
    const response = await route.fetch(), envelope = await response.json();
    const receipt = envelope.data.materialization;
    if (fault === 'receiving_attempt') receipt.receivingAttemptId = author.dispatch.attemptId;
    else if (fault === 'manifest_producer') receipt.observedManifest.producerAttemptId = author.dispatch.attemptId;
    else if (fault === 'missing_staged_proof') receipt.stagedBlobsSha256 = null;
    else if (fault === 'complete_with_failure') receipt.failure = { code: 'controlled_receipt_failure', message: 'This actual-response fixture deliberately supplies a nonqualifying failure.' };
    else if (fault === 'impossible_calendar') receipt.completedAt = '2026-02-31T12:00:00Z';
    else if (fault === 'non_utc_offset') receipt.completedAt = '2026-02-28T12:00:00+01:00';
    else if (fault === 'final_basis') receipt.sourceBasis.finalVerificationRef = author.materialization.observedManifest;
    await route.fulfill({ response, json: envelope });
  });
  const refresh = async () => {
    const response = page.waitForResponse(response => response.request().method() === 'GET' && new URL(response.url()).pathname === reviewerPath);
    await page.locator('#graph-dispatch-refresh').click({ timeout: 5000 });
    await (await response).finished();
    await page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))));
  };
  for (const changed of ['receiving_attempt', 'manifest_producer', 'missing_staged_proof', 'complete_with_failure', 'impossible_calendar', 'non_utc_offset', 'final_basis']) {
    fault = ''; await refresh();
    await page.locator('#graph-input-status').filter({ hasText: 'Exact receiving execution observed.' }).waitFor({ timeout: 5000 });
    const before = await page.locator('#graph-dispatch-facts').textContent();
    fault = changed; await refresh();
    try { await page.locator('#graph-input-status').filter({ hasText: 'The materialization receipt belongs to another execution or does not match its captured basis.' }).waitFor({ timeout: 5000 }); }
    catch (error) { throw new Error('Actual receipt mutation ' + changed + ' did not retain an explicit mismatch: ' + await page.locator('#graph-input-status').textContent(), { cause: error }); }
    assert.equal(await page.locator('#graph-dispatch-facts').textContent(), before, changed + ': a nonqualifying receipt cannot replace loaded proof');
  }
});
