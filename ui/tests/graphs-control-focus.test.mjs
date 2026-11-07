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
  try { await Promise.race([barrier.arrived, new Promise((_, reject) => { timer = setTimeout(() => reject(new Error('The actual saved grant reply did not reach the controlled boundary.')), 10000); })]); }
  finally { clearTimeout(timer); }
}

async function focusByTab(page, target) {
  for (let count = 0; count < 500; count++) {
    if (await target.evaluate(node => document.activeElement === node)) {
      assert.equal(await target.evaluate(node => node.matches(':focus-visible')), true);
      return;
    }
    await page.keyboard.press('Tab');
  }
  throw new Error('Actual keyboard traversal did not reach its named control.');
}

function stableAttempts(page) {
  return { ...page, items: page.items.map(item => { const value = { ...item }; delete value.observedAt; delete value.remainingSeconds; return value; }) };
}

async function focusFixture(t) {
  const f = await fixture(), initial = responseBarrier(), retryReply = responseBarrier();
  t.after(async () => { initial.release(); retryReply.release(); await f.close(); });
  const owner = await f.start();
  const opened = await ownerAPI(owner, '/projects/open', 'POST', { requestId: fixtureRequestId(), selectedPath: f.project, basePath: f.project });
  assert.equal(opened.status, 201);
  // This is an explicit controlled user report, not measured vendor occupancy.
  // Keep the pinned offline author unadmitted while testing local UI focus.
  assert.equal((await reportFixture(owner, 'codex-native', 'occupancy', 'busy')).status, 201);
  const created = await ownerAPI(owner, '/graphs', 'POST', { requestId: fixtureRequestId(), projectId: opened.envelope.data.project.id, executionMode: 'fake',
    nodes: [{ key: 'waiting', originalText: 'Keep this complete original and the user index while the captured offline context is unavailable.',
      fakeScenario: 'graph-response', pins: { harnessId: 'codex' }, workflow: { review: 'auto' }, acceptance: { deliverable: 'response_only' } }],
    routing: { budget: { admissionLimit: '1', windowSeconds: 900 } } });
  assert.equal(created.status, 201, JSON.stringify(created.envelope.error));
  const graphId = created.envelope.data.graph.id;
  await pollOwner(owner, '/graphs/' + graphId + '/inputs', data => data.items.length === 2 && data.items.every(input => input.state === 'ready'), { timeoutMs: 30000 });
  const before = await read(owner, '/graphs/' + graphId);
  assert.equal(before.budget.budget.consumedAdmissions, '0'); assert.equal(before.activeClaims.length, 0);
  assert.equal(before.nodes[0].nodeAdmissionId, null); assert.equal(before.nodes[0].latestAttemptId, null);
  const histories = new Map();
  for (const taskId of [before.graph.rootTaskId, before.nodes[0].taskId]) histories.set(taskId, stableAttempts(await read(owner, '/tasks/' + taskId + '/attempts')));
  const page = await openWorkspace(f, owner, { fake: false, workflow: 'auto' }); page.setDefaultTimeout(30000);
  await page.locator('#open-graph-list').click(); await page.locator('#graph-refresh').click();
  await page.locator('#graph-list button').filter({ hasText: graphId }).click();
  await page.waitForFunction(id => document.getElementById('graph-facts')?.textContent.includes(id), graphId);
  const requests = [], replies = [];
  await page.route('**/api/v1/graphs/' + graphId + '/budget-amendments', async route => {
    const raw = route.request().postData(), body = route.request().postDataJSON(), response = await route.fetch(), envelope = await response.json();
    assert.equal(response.status(), 200, JSON.stringify(envelope.error));
    assert.equal(envelope.requestId, body.requestId); assert.equal(envelope.receipt.requestId, body.requestId); assert.equal(envelope.receipt.operation, 'graphs.budget.amend');
    requests.push({ raw, body }); replies.push(envelope);
    assert.ok(requests.length <= 2, 'No automatic or replacement grant request is authorized by the focus fixture.');
    if (requests.length === 1) await initial.hold();
    else {
      assert.equal(raw, requests[0].raw); assert.deepEqual(body, requests[0].body); assert.deepEqual(envelope, replies[0]);
      await retryReply.hold();
    }
    // Both replies are from actual committed/replayed owner operations. Only
    // their transport acknowledgement is lost; no response data is invented.
    await route.abort('failed');
  });
  await page.locator('#graph-grant-admissions').fill('1');
  await focusByTab(page, page.locator('#graph-grant-budget')); await page.keyboard.press('Enter'); await arrival(initial);
  const captured = requests[0].body;
  assert.equal(captured.additionalAdmissions, '1'); assert.equal(captured.expectedGraphControlRevision, before.graph.controlRevision);
  assert.equal(captured.expectedBudgetAuthorityEpoch, before.graph.budgetAuthorityEpoch); assert.equal(captured.expectedWindowNumber, before.budget.budget.currentWindow.number);
  initial.release();
  const row = page.locator('#graph-control-pending > li').filter({ hasText: captured.requestId });
  const retry = row.getByRole('button', { name: 'Retry exact Grant', exact: true }); await retry.waitFor(); assert.equal(await retry.isEnabled(), true);
  const granted = await read(owner, '/graphs/' + graphId);
  assert.equal(granted.budget.budget.grantedAdmissions, '2'); assert.equal(granted.budget.budget.consumedAdmissions, '0');
  assert.equal(granted.budget.budget.currentWindow.deadlineAt, before.budget.budget.currentWindow.deadlineAt);
  assert.equal(granted.dispatchAuthority.state, 'suspended');
  return { f, owner, page, graphId, before, granted, histories, requests, replies, captured, retry, retryReply };
}

async function failedRetryVisible(f) {
  await f.page.waitForFunction(requestId => [...document.querySelectorAll('#graph-control-pending > li')].some(row => row.textContent.includes(requestId)
    && [...row.querySelectorAll('button')].some(button => button.textContent === 'Retry exact Grant' && !button.disabled && button.getAttribute('aria-disabled') !== 'true')), f.captured.requestId, { timeout: 30000 });
}

async function unchangedAuthority(f) {
  assert.equal(f.requests.length, 2); assert.equal(f.requests[0].raw, f.requests[1].raw); assert.deepEqual(f.requests[0].body, f.requests[1].body); assert.deepEqual(f.replies[0], f.replies[1]);
  const graph = await read(f.owner, '/graphs/' + f.graphId);
  assert.deepEqual(graph.budget, f.granted.budget); assert.equal(graph.graph.controlRevision, f.granted.graph.controlRevision);
  assert.equal(graph.graph.budgetAuthorityEpoch, f.granted.graph.budgetAuthorityEpoch); assert.equal(graph.dispatchAuthority.state, 'suspended');
  assert.equal(graph.activeClaims.length, 0); assert.equal(graph.nodes[0].nodeAdmissionId, null); assert.equal(graph.nodes[0].latestAttemptId, null);
  assert.equal(graph.nodes[0].taskVersion, f.before.nodes[0].taskVersion); assert.equal(graph.nodes[0].controlRevision, f.before.nodes[0].controlRevision);
  for (const [taskId, history] of f.histories) assert.deepEqual(stableAttempts(await read(f.owner, '/tasks/' + taskId + '/attempts')), history);
  const row = f.page.locator('#graph-control-pending > li').filter({ hasText: f.captured.requestId });
  assert.equal(await row.count(), 1); assert.deepEqual(JSON.parse(await row.locator('pre').textContent()), f.captured);
  assert.equal(await readFile(path.join(f.f.project, 'hello.txt'), 'utf8'), 'unstaged\n');
  assert.deepEqual(await readFile(path.join(f.f.project, '.git', 'index')), f.f.indexBefore);
}

test('keyboard retry preserves meaningful pending focus and restores the exact request after another lost reply', { timeout: 90000 }, async t => {
  const f = await focusFixture(t);
  await focusByTab(f.page, f.retry); await f.page.keyboard.press('Enter'); await arrival(f.retryReply);
  const focus = await f.page.evaluate(requestId => {
    const row = [...document.querySelectorAll('#graph-control-pending > li')].find(row => row.textContent.includes(requestId)), active = document.activeElement;
    return { tag: active?.tagName, id: active?.id, text: active?.textContent?.slice(0, 160),
      meaningful: !!row && active !== document.body && row.contains(active) && active.checkVisibility()
        && active.matches('button,a,input,select,textarea,[tabindex],[role="status"]') };
  }, f.captured.requestId);
  assert.equal(focus.meaningful, true, 'keyboard focus left its actual pending request while the exact reply was held: ' + JSON.stringify(focus));
  f.retryReply.release(); await failedRetryVisible(f);
  assert.equal(await f.retry.evaluate(node => document.activeElement === node), true, 'another lost acknowledgement must restore the same request retry focus');
  assert.equal(await f.retry.evaluate(node => node.matches(':focus-visible')), true);
  await unchangedAuthority(f);
});

test('failed pending control retry never steals keyboard focus after the user moves elsewhere', { timeout: 90000 }, async t => {
  const f = await focusFixture(t);
  await focusByTab(f.page, f.retry); await f.page.keyboard.press('Enter'); await arrival(f.retryReply);
  const elsewhere = f.page.locator('#graph-refresh'); await focusByTab(f.page, elsewhere);
  assert.equal(await elsewhere.evaluate(node => document.activeElement === node), true);
  f.retryReply.release(); await failedRetryVisible(f);
  assert.equal(await elsewhere.evaluate(node => document.activeElement === node), true, 'failed replay moved focus away from the user-selected graph refresh control');
  assert.equal(await elsewhere.evaluate(node => node.matches(':focus-visible')), true);
  await unchangedAuthority(f);
});
