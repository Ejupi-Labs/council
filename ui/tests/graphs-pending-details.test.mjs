import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import path from 'node:path';
import test from 'node:test';
import { fixture, fixtureRequestId, ownerAPI, pollOwner, responseBarrier } from './helpers.mjs';

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

function stableAttempts(page) {
  return { ...page, items: page.items.map(item => { const value = { ...item }; delete value.observedAt; delete value.remainingSeconds; return value; }) };
}

// Measure only the real expanded PRE. Closed-details Range rectangles are not
// painted content and therefore cannot establish either visibility or clipping.
async function expandedRequestGeometry(details) {
  return details.locator('pre').evaluate(pre => {
    const details = pre.parentElement, viewport = { width: innerWidth, height: innerHeight };
    const rect = box => ({ left: box.left, right: box.right, top: box.top, bottom: box.bottom, width: box.width, height: box.height });
    const clips = [];
    for (let node = pre; node; node = node.parentElement) {
      const css = getComputedStyle(node), box = node.getBoundingClientRect();
      if (['hidden', 'clip', 'auto', 'scroll'].includes(css.overflowX) || /(?:^|\s)(?:paint|strict|content)(?:\s|$)/.test(css.contain)) {
        clips.push({ tag: node.tagName, id: node.id, left: box.left + node.clientLeft, right: box.left + node.clientLeft + node.clientWidth });
      }
    }
    const textRanges = [], identifiers = [], walker = document.createTreeWalker(pre, NodeFilter.SHOW_TEXT);
    let node;
    while ((node = walker.nextNode())) {
      const range = document.createRange(); range.selectNodeContents(node);
      textRanges.push(...[...range.getClientRects()].filter(box => box.width > 0 && box.height > 0).map(rect));
      for (const match of node.data.matchAll(/\b[0-9a-f]{32}\b/g)) {
        range.setStart(node, match.index); range.setEnd(node, match.index + match[0].length);
        identifiers.push({ text: match[0], rects: [...range.getClientRects()].map(rect) });
      }
      range.detach();
    }
    return { viewport, documentWidth: document.documentElement.scrollWidth, text: pre.textContent, open: details.open,
      visible: pre.checkVisibility({ checkOpacity: true, checkVisibilityCSS: true, contentVisibilityAuto: true }),
      pre: rect(pre.getBoundingClientRect()), textRanges, identifiers, clips };
  });
}

test('expanded retained rebind request remains fully readable at 360px and exact retry preserves authority', { timeout: 90000 }, async t => {
  const f = await fixture(), held = responseBarrier();
  t.after(async () => { held.release(); await f.close(); });
  const owner = await f.start();
  const opened = await ownerAPI(owner, '/projects/open', 'POST', { requestId: fixtureRequestId(), selectedPath: f.project, basePath: f.project });
  assert.equal(opened.status, 201);
  const page = await f.browser.newPage({ viewport: { width: 1440, height: 1000 }, reducedMotion: 'reduce' });
  page.setDefaultTimeout(30000);
  const consoleErrors = [];
  page.on('pageerror', error => consoleErrors.push(error.message));
  await page.goto(owner.url);
  await page.waitForFunction(() => document.getElementById('app-status')?.textContent.includes('Connected'));
  await page.locator('#project-path').fill(f.project); await page.locator('#open-project').click();
  await page.waitForFunction(() => !document.getElementById('start-task')?.disabled);
  const node = (key, scenario, response, harness) => ({ key,
    originalText: 'Complete controlled original for ' + key + ' 東京🙂. Preserve the user working bytes and staged index; inspect exact supplied graph context.',
    fakeScenario: scenario, pins: { harnessId: harness }, acceptance: { deliverable: response ? 'response_only' : 'candidate' }, workflow: { review: response ? 'auto' : 'required' } });
  const created = await ownerAPI(owner, '/graphs', 'POST', { requestId: fixtureRequestId(), projectId: opened.envelope.data.project.id, executionMode: 'fake',
    nodes: [node('source', 'graph-branch-a', false, 'codex'), node('receiver', 'graph-response', true, 'claude-code')],
    dependencies: [{ from: 'source', to: 'receiver', kind: 'files' }], routing: { budget: { admissionLimit: '5', windowSeconds: 900 } } });
  assert.equal(created.status, 201, JSON.stringify(created.envelope.error));
  const graphId = created.envelope.data.graph.id;
  const first = await pollOwner(owner, '/graphs/' + graphId, graph => graph.nodes.find(node => node.key === 'source')?.acceptedResult != null
    && graph.nodes.find(node => node.key === 'receiver')?.latestAttemptId != null, { timeoutMs: 30000 });
  const source = first.nodes.find(node => node.key === 'source'), receiver = first.nodes.find(node => node.key === 'receiver');
  assert.equal(source.acceptedResult.acceptanceKind, 'workflow');
  assert.ok(source.acceptedResult.candidateId);
  const originalReceiver = await pollOwner(owner, '/attempts/' + receiver.latestAttemptId, attempt => attempt.phase === 'settled', { timeoutMs: 30000 });
  assert.equal(originalReceiver.executionMode, 'fake'); assert.equal(originalReceiver.outcome, 'succeeded'); assert.equal(originalReceiver.cleanup.state, 'verified'); assert.ok(originalReceiver.startedAt);
  const oldAttempts = await read(owner, '/tasks/' + receiver.taskId + '/attempts');
  assert.equal(oldAttempts.items.length, 1);
  const oldVersion = await read(owner, '/tasks/' + receiver.taskId + '/versions/1');
  const oldInput = await read(owner, '/graphs/' + graphId + '/inputs/' + receiver.inputId);
  await page.locator('#open-graph-list').click(); await page.locator('#graph-refresh').click();
  await page.locator('#graph-list button').filter({ hasText: graphId }).click();
  await page.waitForFunction(id => document.getElementById('graph-facts')?.textContent.includes(id), graphId);
  await page.locator('#graph-guidance-node').selectOption(source.id);
  await page.locator('#graph-guidance-text').fill('Complete updated source guidance 東京🙂. Keep the actual complete branch bytes and captured scope.');
  await page.locator('#graph-save-guidance').click();
  await page.waitForFunction(() => document.getElementById('graph-control-status')?.textContent.includes('Guidance version 2 recorded'));
  await page.locator('#graph-refresh').click();
  await page.waitForFunction(() => document.querySelector('#graph-node-list [data-node-key="source"]')?.textContent.includes('Current version 2'));
  await page.locator('#graph-advance').click();
  const stale = await pollOwner(owner, '/graphs/' + graphId, graph => graph.nodes.find(node => node.key === 'source')?.acceptedResult?.taskVersion === '2'
    && graph.budget.budget.remainingAdmissions === '0' && graph.activeClaims.length === 0, { timeoutMs: 30000 });
  assert.equal(stale.budget.budget.consumedAdmissions, '5');
  assert.equal(stale.budget.budget.currentWindow.deadlineAt, first.budget.budget.currentWindow.deadlineAt);
  await page.locator('#graph-refresh').click();
  await page.waitForFunction(() => document.getElementById('graph-facts')?.textContent.includes('5 consumed / 5 granted'));
  await page.setViewportSize({ width: 360, height: 1000 });
  const choice = page.locator('#graph-rebind-nodes [data-graph-rebind-node="' + receiver.id + '"]');
  await choice.waitFor();
  const requests = [], replies = [];
  let captured;
  await page.route('**/api/v1/graphs/' + graphId + '/advance', async route => {
    const body = route.request().postDataJSON(), raw = route.request().postData(), response = await route.fetch(), envelope = await response.json();
    assert.equal(response.status(), 200, JSON.stringify(envelope.error));
    assert.equal(envelope.requestId, body.requestId); assert.equal(envelope.receipt.requestId, body.requestId); assert.equal(envelope.receipt.operation, 'graphs.advance');
    requests.push({ body, raw }); replies.push(envelope);
    if (!captured) { captured = body; await held.hold(); await route.abort('failed'); }
    else await route.fulfill({ response });
  });
  await choice.check(); await page.locator('#graph-rebind-advance').click(); await arrival(held);
  assert.deepEqual(captured.rebindNodeIds, [receiver.id]);
  assert.equal(captured.graphVersion, stale.graph.graphVersion);
  assert.equal(captured.expectedGraphControlRevision, stale.graph.controlRevision);
  assert.equal(captured.expectedBudgetAuthorityEpoch, stale.graph.budgetAuthorityEpoch);
  assert.equal(captured.expectedWindowNumber, stale.budget.budget.currentWindow.number);
  assert.deepEqual(captured.nodeAuthorities, stale.nodes.map(node => ({ nodeId: node.id, taskVersion: node.taskVersion, nodeControlRevision: node.controlRevision })));
  await choice.uncheck(); held.release();
  const retry = page.locator('#graph-control-pending').getByRole('button', { name: 'Retry exact Rebind and Advance', exact: true });
  await retry.waitFor();
  const details = page.locator('#graph-control-pending details').first();
  assert.equal(await details.evaluate(node => node.open), false);
  assert.deepEqual(JSON.parse(await details.locator('pre').textContent()), captured);
  const beforeExpansion = await read(owner, '/graphs/' + graphId);
  await details.locator(':scope > summary').click();
  assert.equal(await details.evaluate(node => node.open), true);
  const geometry = await expandedRequestGeometry(details);
  assert.deepEqual(geometry.viewport, { width: 360, height: 1000 });
  assert.equal(geometry.open, true); assert.equal(geometry.visible, true);
  assert.deepEqual(JSON.parse(geometry.text), captured, 'expanding retained evidence must preserve its exact captured request');
  const expectedIdentifiers = [...JSON.stringify(captured).matchAll(/\b[0-9a-f]{32}\b/g)].map(match => match[0]);
  assert.deepEqual(geometry.identifiers.map(token => token.text), expectedIdentifiers, 'complete request identities must remain present in the real rendered JSON');
  assert.equal(requests.length, 1, 'opening retained details cannot retry or create graph work');
  assert.ok(geometry.textRanges.length > 0);
  assert.ok(geometry.identifiers.every(token => token.rects.length > 0));
  assert.ok(geometry.documentWidth <= geometry.viewport.width + 1, 'expanded retained request must not widen the mobile page: ' + JSON.stringify(geometry));
  assert.ok(geometry.textRanges.every(box => box.left >= -1 && box.right <= geometry.viewport.width + 1), 'all expanded request text must remain inside the actual mobile viewport: ' + JSON.stringify(geometry));
  assert.ok(geometry.identifiers.every(token => token.rects.every(box => box.left >= -1 && box.right <= geometry.viewport.width + 1)), 'every complete request identifier must remain horizontally visible');
  assert.ok(geometry.textRanges.every(box => geometry.clips.every(clip => box.left >= clip.left - 1 && box.right <= clip.right + 1)), 'overflow clipping cannot conceal part of the retained request');
  const afterExpansion = await read(owner, '/graphs/' + graphId);
  assert.equal(afterExpansion.budget.budget.consumedAdmissions, '5'); assert.equal(afterExpansion.budget.budget.remainingAdmissions, '0');
  assert.equal(afterExpansion.budget.budget.currentWindow.deadlineAt, stale.budget.budget.currentWindow.deadlineAt);
  assert.equal(afterExpansion.nodes.find(node => node.id === receiver.id).taskVersion, '2');
  assert.equal(afterExpansion.graph.budgetAuthorityEpoch, beforeExpansion.graph.budgetAuthorityEpoch);
  // Reconciling changes the label before the saved reply arrives. Observe the
  // exact response and removal of this pending identity within one fixed bound.
  const replayDeadline = Date.now() + 30000;
  const replayResponse = page.waitForResponse(response => response.request().method() === 'POST'
    && new URL(response.url()).pathname === '/api/v1/graphs/' + graphId + '/advance'
    && response.request().postDataJSON().requestId === captured.requestId, { timeout: 30000 });
  await retry.click();
  const acknowledged = await replayResponse;
  assert.equal(acknowledged.status(), 200);
  assert.deepEqual(await acknowledged.json(), replies[0]);
  const remaining = replayDeadline - Date.now();
  assert.ok(remaining > 0, 'the actual replay acknowledgement exceeded its original 30-second bound');
  await page.waitForFunction(requestId => ![...document.querySelectorAll('#graph-control-pending > li')].some(node => node.textContent.includes(requestId)), captured.requestId, { timeout: remaining });
  assert.equal(requests.length, 2); assert.equal(requests[0].raw, requests[1].raw); assert.deepEqual(requests[0].body, requests[1].body);
  assert.deepEqual(replies[0], replies[1], 'exact retry must return the original committed owner receipt');
  const prepared = await pollOwner(owner, '/graphs/' + graphId, graph => graph.nodes.find(node => node.id === receiver.id)?.taskVersion === '2'
    && graph.nodes.find(node => node.id === receiver.id)?.inputId != null && graph.activeClaims.length === 0, { timeoutMs: 30000 });
  const currentReceiver = prepared.nodes.find(node => node.id === receiver.id);
  assert.notEqual(currentReceiver.inputId, receiver.inputId);
  await pollOwner(owner, '/graphs/' + graphId + '/inputs/' + currentReceiver.inputId, input => input.summary.state === 'ready' && input.summary.current
    && input.summary.receivingTaskVersion === '2' && input.inputBinding !== null, { timeoutMs: 30000 });
  assert.equal(prepared.budget.budget.consumedAdmissions, '5'); assert.equal(prepared.budget.budget.grantedAdmissions, '5'); assert.equal(prepared.budget.budget.remainingAdmissions, '0');
  assert.deepEqual(prepared.budget.budget.currentWindow, stale.budget.budget.currentWindow);
  assert.equal(currentReceiver.nodeAdmissionId, null);
  assert.deepEqual(stableAttempts(await read(owner, '/tasks/' + receiver.taskId + '/attempts')), stableAttempts(oldAttempts));
  assert.deepEqual(await read(owner, '/tasks/' + receiver.taskId + '/versions/1'), oldVersion);
  const retained = await read(owner, '/graphs/' + graphId + '/inputs/' + receiver.inputId);
  assert.deepEqual(retained.inputBinding, oldInput.inputBinding);
  assert.equal(await readFile(path.join(f.project, 'hello.txt'), 'utf8'), 'unstaged\n');
  assert.deepEqual(await readFile(path.join(f.project, '.git', 'index')), f.indexBefore);
  assert.deepEqual(consoleErrors, []);
});
