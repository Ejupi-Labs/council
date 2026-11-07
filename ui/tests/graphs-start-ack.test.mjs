import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import path from 'node:path';
import test from 'node:test';
import { fixture, fixtureRequestId, openWorkspace, ownerAPI, pollOwner, reportFixture, responseBarrier } from './helpers.mjs';

async function read(owner, route) {
  const result = await ownerAPI(owner, route);
  assert.equal(result.status, 200, JSON.stringify(result.envelope.error));
  return result.envelope.data;
}

async function arrival(barrier) {
  let timer;
  try { await Promise.race([barrier.arrived, new Promise((_, reject) => { timer = setTimeout(() => reject(new Error('The actual committed graph Start reply did not reach its controlled boundary.')), 10000); })]); }
  finally { clearTimeout(timer); }
}

function stableAttempts(page) {
  return { ...page, items: page.items.map(item => { const value = { ...item }; delete value.observedAt; delete value.remainingSeconds; return value; }) };
}

async function compose(f, owner) {
  // Explicit controlled user reports, not measured provider occupancy. Both
  // captured fake contexts stay unavailable: acknowledgement tests launch no model.
  for (const context of ['codex-native', 'claude-native']) assert.equal((await reportFixture(owner, context, 'occupancy', 'busy')).status, 201);
  const page = await openWorkspace(f, owner, { fake: false, workflow: 'auto' });
  page.setDefaultTimeout(30000);
  const standalone = 'Separate unsent standalone original 東京🙂';
  await page.locator('#task-text').fill(standalone);
  await page.locator('#open-graph-composer').click();
  await page.locator('#graph-title').fill('Exact saved graph admission');
  const nodes = page.locator('#graph-editor-nodes > [data-graph-node]');
  assert.equal(await nodes.count(), 2);
  const originals = ['First complete original π. Preserve the user working bytes.', 'Second complete original 東京🙂. Preserve the captured staged index.'];
  const keys = [];
  for (let i = 0; i < 2; i++) {
    const node = nodes.nth(i); keys.push(await node.getAttribute('data-graph-node'));
    await node.locator('[data-graph-field="originalText"]').fill(originals[i]);
    await node.locator('summary').filter({ hasText: /^Task details \(optional\)$/ }).click();
    await node.locator('[data-graph-field="harness"]').selectOption(i ? 'claude-code' : 'codex');
    await node.locator('[data-graph-field="deliverable"]').selectOption('response_only');
  }
  await page.locator('summary').filter({ hasText: /^Group details \(optional\)$/ }).click();
  await page.locator('#graph-admissions').fill('2'); await page.locator('#graph-window').fill('900');
  return { page, standalone, originals, keys };
}

async function bridge(page, first, change, lost = false) {
  const requests = [], forwarded = [], replies = [];
  await page.route('**/api/v1/graphs', async route => {
    if (route.request().method() !== 'POST') { await route.continue(); return; }
    const raw = route.request().postData(), body = route.request().postDataJSON();
    assert.equal(body.executionMode, undefined);
    assert.ok(body.nodes.every(node => node.fakeScenario === undefined));
    requests.push({ raw, body });
    assert.ok(requests.length <= 2, 'Only one captured Start and its exact explicit replay are authorized.');
    // Same bounded offline bridge as graphs-start.test.mjs. No installed vendor
    // executable can receive the browser's default native declaration.
    const encoded = JSON.stringify({ ...body, executionMode: 'fake', nodes: body.nodes.map(node => ({ ...node, fakeScenario: 'graph-response' })) });
    forwarded.push(encoded);
    const response = await route.fetch({ postData: encoded }), envelope = await response.json();
    assert.equal(response.status(), 201, JSON.stringify(envelope.error));
    assert.equal(envelope.requestId, body.requestId); assert.equal(envelope.receipt.requestId, body.requestId);
    assert.equal(envelope.receipt.operation, 'graphs.start', 'The actual owner receipt fixes the wire operation.');
    replies.push(envelope);
    if (requests.length === 1) {
      await first.hold();
      if (lost) { await route.abort('failed'); return; }
      const altered = structuredClone(envelope); change(altered);
      assert.equal(altered.receipt.requestId, body.requestId, 'The existing generic receipt-ID guard must pass.');
      await route.fulfill({ response, json: altered });
    } else await route.fulfill({ response });
  });
  return { requests, forwarded, replies };
}

async function unchangedOwner(f, owner, capture) {
  const admission = capture.replies[0].data, graphId = admission.graph.id;
  await pollOwner(owner, '/graphs/' + graphId + '/inputs', data => data.items.length === 3 && data.items.every(input => input.state === 'ready'), { timeoutMs: 30000 });
  const graph = await pollOwner(owner, '/graphs/' + graphId, data => data.activeClaims.length === 0 && data.nodes.every(node => node.nodeAdmissionId === null && node.latestAttemptId === null), { timeoutMs: 30000 });
  assert.equal(graph.budget.budget.grantedAdmissions, '2'); assert.equal(graph.budget.budget.consumedAdmissions, '0');
  assert.equal(graph.budget.budget.currentWindow.firstAdmissionAt, null); assert.equal(graph.budget.budget.currentWindow.deadlineAt, null);
  const histories = new Map();
  for (const taskId of [graph.graph.rootTaskId, ...graph.nodes.map(node => node.taskId)]) histories.set(taskId, stableAttempts(await read(owner, '/tasks/' + taskId + '/attempts')));
  const groups = await read(owner, '/graphs?projectId=' + capture.requests[0].body.projectId);
  assert.deepEqual(groups.items.map(item => item.id), [graphId]); assert.equal(groups.nextCursor, null);
  assert.equal(await readFile(path.join(f.project, 'hello.txt'), 'utf8'), 'unstaged\n');
  assert.deepEqual(await readFile(path.join(f.project, '.git', 'index')), f.indexBefore);
  return { graph, histories };
}

async function exactReplay(page, capture) {
  const captured = capture.requests[0].body;
  const retry = page.locator('#graph-pending > li').filter({ hasText: captured.requestId }).getByRole('button', { name: 'Retry exact group start', exact: true });
  // A busy label is not acknowledgement. Observe the actual second response and
  // exact pending-row removal under one immutable original 30-second boundary.
  const deadline = Date.now() + 30000;
  const response = page.waitForResponse(value => value.request().method() === 'POST' && new URL(value.url()).pathname === '/api/v1/graphs'
    && value.request().postDataJSON().requestId === captured.requestId, { timeout: 30000 });
  await retry.click(); const acknowledged = await response;
  assert.equal(acknowledged.status(), 201); assert.deepEqual(await acknowledged.json(), capture.replies[0]);
  const remaining = deadline - Date.now(); assert.ok(remaining > 0);
  await page.waitForFunction(requestId => ![...document.querySelectorAll('#graph-pending > li')].some(row => row.textContent.includes(requestId)), captured.requestId, { timeout: remaining });
  assert.equal(capture.requests.length, 2); assert.equal(capture.requests[1].raw, capture.requests[0].raw);
  assert.deepEqual(capture.requests[1].body, captured); assert.equal(capture.forwarded[1], capture.forwarded[0]);
  assert.deepEqual(capture.replies[1], capture.replies[0], 'The owner must replay its unchanged actual canonical admission and historical authority.');
}

const changedReplies = [
  ['foreign envelope request', reply => { reply.requestId = fixtureRequestId(); }],
  ['foreign receipt operation', reply => { reply.receipt.operation = 'graphs.stop'; }],
  ['foreign graph project', reply => { reply.data.graph.projectId = fixtureRequestId(); }],
  ['foreign definition graph', reply => { reply.data.definition.graphId = fixtureRequestId(); }],
  ['foreign node membership', reply => { reply.data.nodes[0].graphId = fixtureRequestId(); }],
  ['foreign shared budget owner', reply => { reply.data.budget.budgetOwnerTaskId = reply.data.nodes[0].taskId; }],
  ['different registration node version', reply => { reply.data.dispatchAuthority.nodeAuthorities[0].taskVersion = '2'; }],
];

for (const [name, change] of changedReplies) test('graph Start retains exact intent after an actual saved reply has ' + name, { timeout: 90000 }, async t => {
  const first = responseBarrier(), f = await fixture(); t.after(async () => { first.release(); await f.close(); });
  const owner = await f.start(), draft = await compose(f, owner), { page } = draft, capture = await bridge(page, first, change);
  const response = page.waitForResponse(value => value.request().method() === 'POST' && new URL(value.url()).pathname === '/api/v1/graphs', { timeout: 30000 });
  await page.locator('#graph-start').click(); await arrival(first);
  assert.deepEqual(capture.requests[0].body.nodes.map(node => node.key), draft.keys);
  assert.deepEqual(capture.requests[0].body.nodes.map(node => node.originalText), draft.originals);
  first.release(); await response;
  // Finish either the retained-error or old unsafe-success handler before the
  // decisive assertion; this is not a sleep or an enlarged completion timeout.
  await page.waitForFunction(requestId => [...document.querySelectorAll('#graph-pending > li')].some(row => row.textContent.includes(requestId)
    && [...row.querySelectorAll('button')].some(button => button.textContent === 'Retry exact group start' && !button.disabled))
    || document.getElementById('graph-start')?.disabled === false || document.getElementById('graph-start-status')?.textContent.startsWith('Task group admitted.'), capture.requests[0].body.requestId, { timeout: 30000 });
  const row = page.locator('#graph-pending > li').filter({ hasText: capture.requests[0].body.requestId });
  assert.equal(await row.count(), 1, 'A mismatched acknowledgement must retain this exact saved Start request.');
  assert.doesNotMatch(await page.locator('#graph-start-status').textContent(), /^Task group admitted\./);
  assert.equal(await page.locator('#graph-overview').isVisible(), false, 'A mismatched admission cannot select a graph.');
  assert.ok((await row.textContent()).includes(draft.originals[0]));
  const before = await unchangedOwner(f, owner, capture);
  const newerTitle = 'Newer unsent graph draft 東京', newerOriginal = 'Newer unsent original🙂. The old receipt cannot acknowledge this text.';
  await page.locator('#graph-title').fill(newerTitle);
  await page.locator('#graph-editor-nodes > [data-graph-node]').first().locator('[data-graph-field="originalText"]').fill(newerOriginal);
  const newerStatus = await page.locator('#graph-start-status').textContent();
  await exactReplay(page, capture);
  assert.equal(await page.locator('#graph-title').inputValue(), newerTitle);
  assert.equal(await page.locator('#graph-editor-nodes > [data-graph-node]').first().locator('[data-graph-field="originalText"]').inputValue(), newerOriginal);
  assert.equal(await page.locator('#graph-start-status').textContent(), newerStatus, 'A historical receipt cannot paint success over a newer editor.');
  assert.equal(await page.locator('#task-text').inputValue(), draft.standalone);
  const after = await unchangedOwner(f, owner, capture);
  assert.deepEqual(after.histories, before.histories); assert.deepEqual(after.graph.budget, before.graph.budget);
});

test('graph Start exact acknowledgement accepts its immutable historical owner after actual owner recovery', { timeout: 90000 }, async t => {
  const first = responseBarrier(), f = await fixture(); t.after(async () => { first.release(); await f.close(); });
  const owner = await f.start(), draft = await compose(f, owner), { page } = draft, capture = await bridge(page, first, undefined, true);
  await page.locator('#graph-start').click(); await arrival(first); first.release();
  const requestId = capture.requests[0].body.requestId, projectId = capture.requests[0].body.projectId;
  await page.locator('#graph-pending > li').filter({ hasText: requestId }).getByRole('button', { name: 'Retry exact group start', exact: true }).waitFor();
  const before = await unchangedOwner(f, owner, capture);
  const newerTitle = 'Retained unsent graph draft after owner recovery 東京', newerOriginal = 'Retained newer original🙂. Do not replay this draft.';
  await page.locator('#graph-title').fill(newerTitle);
  await page.locator('#graph-editor-nodes > [data-graph-node]').first().locator('[data-graph-field="originalText"]').fill(newerOriginal);
  const keys = ['council.foundation.graphPendingStarts', 'council.foundation.graphDraft:' + projectId];
  const retained = await page.evaluate(names => names.map(key => [key, sessionStorage.getItem(key)]), keys);
  assert.ok(retained.every(([, value]) => value !== null));
  const pending = JSON.parse(retained[0][1]); assert.equal(pending.length, 1); assert.deepEqual(pending[0].body, capture.requests[0].body);
  const oldOwnerId = capture.replies[0].data.dispatchAuthority.ownerInstanceId;
  assert.equal((await read(owner, '/meta')).ownerInstanceId, oldOwnerId);
  await owner.stop(); const next = await f.start();
  const nextMeta = await read(next, '/meta'); assert.notEqual(nextMeta.ownerInstanceId, oldOwnerId);
  // New owners may choose another loopback port. Carry only the actual complete
  // unmodified browser recovery entries to that origin, never a token or invented
  // request/authority. Normal owner bootstrap consumes the new local URL itself.
  await page.addInitScript(entries => { for (const [key, value] of entries) sessionStorage.setItem(key, value); }, retained);
  await page.goto(next.url); await page.locator('#project-path').fill(f.project); await page.locator('#open-project').click();
  await page.waitForFunction(() => !document.getElementById('start-task')?.disabled, undefined, { timeout: 30000 });
  await page.locator('#open-graph-composer').click();
  assert.equal(await page.locator('#graph-title').inputValue(), newerTitle);
  assert.equal(await page.locator('#graph-editor-nodes > [data-graph-node]').first().locator('[data-graph-field="originalText"]').inputValue(), newerOriginal);
  const priorStatus = await page.locator('#graph-start-status').textContent();
  await exactReplay(page, capture);
  assert.equal(capture.replies[1].data.dispatchAuthority.ownerInstanceId, oldOwnerId);
  assert.equal(await page.locator('#graph-title').inputValue(), newerTitle);
  assert.equal(await page.locator('#graph-start-status').textContent(), priorStatus);
  const after = await unchangedOwner(f, next, capture);
  assert.deepEqual(after.histories, before.histories); assert.deepEqual(after.graph.budget, before.graph.budget);
});
