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

async function arrival(barrier, name) {
  let timer;
  try {
    await Promise.race([barrier.arrived, new Promise((_, reject) => {
      timer = setTimeout(() => reject(new Error('The actual ' + name + ' observation did not reach its fixed boundary.')), 10000);
    })]);
  } finally { clearTimeout(timer); }
}

async function focusByTab(page, target) {
  for (let count = 0; count < 200; count++) {
    if (await target.evaluate(node => document.activeElement === node)) {
      assert.equal(await target.evaluate(node => node.matches(':focus-visible')), true);
      return;
    }
    await page.keyboard.press('Tab');
  }
  throw new Error('Real keyboard traversal did not reach the complete-original summary.');
}

function originalDetails(page) {
  return page.locator('#graph-definition > details').filter({ hasText: 'reader · complete original instructions' });
}

async function retainedOriginal(page, text, boundary) {
  const observed = await originalDetails(page).evaluate(node => {
    const summary = node.querySelector(':scope > summary');
    return { open: node.open, original: node.querySelector('pre')?.textContent,
      summaryFocused: document.activeElement === summary, focusVisible: summary.matches(':focus-visible'),
      activeTag: document.activeElement?.tagName };
  });
  assert.equal(observed.original, text, boundary + ': complete original text changed');
  assert.equal(observed.open, true, boundary + ': polling collapsed the inspected original: ' + JSON.stringify(observed));
  assert.equal(observed.summaryFocused, true, boundary + ': polling removed the focused original summary: ' + JSON.stringify(observed));
  assert.equal(observed.focusVisible, true, boundary + ': actual keyboard focus is no longer visible');
}

test('actual unchanged and changed graph observation preserves the opened keyboard-focused original without crossing graph identity', { timeout: 90000 }, async t => {
  const f = await fixture(), unchanged = responseBarrier(), changed = responseBarrier();
  let closing = false;
  t.after(async () => { closing = true; unchanged.release(); changed.release(); await f.close(); });
  const owner = await f.start();
  const opened = await ownerAPI(owner, '/projects/open', 'POST', {
    requestId: fixtureRequestId(), selectedPath: f.project, basePath: f.project,
  });
  assert.equal(opened.status, 201);
  const projectId = opened.envelope.data.project.id;
  const page = await openWorkspace(f, owner, { fake: false, workflow: 'auto' });
  page.setDefaultTimeout(30000);
  const consoleErrors = [], browserWrites = [];
  page.on('pageerror', error => consoleErrors.push(error.message));
  page.on('request', request => { if (!['GET', 'HEAD'].includes(request.method())) browserWrites.push(request.method()); });
  const firstText = '\ufeffComplete first retained original 東京🙂\r\nKeep this original open while the actual controlled worker waits.';
  const create = originalText => ({ requestId: fixtureRequestId(), projectId, executionMode: 'fake',
    nodes: [{ key: 'reader', originalText, fakeScenario: 'graph-wait', pins: { harnessId: 'codex' },
      acceptance: { deliverable: 'candidate' }, workflow: { review: 'auto' } }],
    routing: { budget: { admissionLimit: '1', windowSeconds: 900 } } });
  const first = await ownerAPI(owner, '/graphs', 'POST', create(firstText));
  assert.equal(first.status, 201, JSON.stringify(first.envelope.error));
  assert.equal(first.envelope.receipt.operation, 'graphs.start');
  const graphId = first.envelope.data.graph.id;
  const running = await pollOwner(owner, '/graphs/' + graphId,
    graph => graph.graph.state === 'running' && graph.nodes[0].state === 'running' && graph.nodes[0].latestAttemptId !== null,
    { timeoutMs: 30000 });
  const actual = await read(owner, '/attempts/' + running.nodes[0].latestAttemptId);
  assert.equal(actual.executionMode, 'fake'); assert.equal(actual.phase, 'running'); assert.ok(actual.startedAt);
  assert.equal(running.budget.budget.consumedAdmissions, '1'); assert.equal(running.activeClaims.length, 1);
  const waits = [];
  await page.route(url => url.pathname === '/api/v1/graphs/' + graphId + '/wait', async route => {
    const request = new URL(route.request().url()), index = waits.length;
    // This is a real owner read with a supported shorter observation timeout,
    // not a fabricated wait body or an extension of the fixed 8s worker bound.
    // Its immutable response is held until the real summary is open/focused.
    if (index === 0) request.searchParams.set('timeoutMs', '1');
    if (index === 1) await changed.hold();
    if (closing) { await route.abort('aborted'); return; }
    const response = await route.fetch({ url: request.href });
    const envelope = await response.json();
    assert.equal(response.status(), 200, JSON.stringify(envelope.error));
    assert.equal(envelope.data.graph.graph.id, graphId);
    assert.equal(envelope.data.graph.graph.projectId, projectId);
    assert.equal(envelope.data.graph.graph.graphVersion, running.graph.graphVersion);
    if (index === 0) {
      assert.equal(envelope.data.changed, false, 'The unchanged boundary must be an actual owner timeout at the same revision.');
      assert.equal(envelope.data.graph.graph.stateRevision, request.searchParams.get('afterRevision'));
    } else if (index === 1) {
      assert.equal(envelope.data.changed, true, 'The changed boundary must contain the actual accepted Stop transition.');
      assert.ok(BigInt(envelope.data.graph.graph.stateRevision) > BigInt(request.searchParams.get('afterRevision')));
    }
    waits.push(envelope.data);
    if (index === 0) await unchanged.hold();
    if (closing) { await route.abort('aborted'); return; }
    await route.fulfill({ response });
  });
  await page.locator('#open-graph-list').click(); await page.locator('#graph-refresh').click();
  await page.locator('#graph-list button').filter({ hasText: graphId }).click();
  await arrival(unchanged, 'unchanged wait');
  await page.locator('#graph-overview > details > summary').filter({ hasText: 'Retained definition' }).click();
  await page.waitForFunction(() => document.getElementById('graph-definition-status')?.textContent.includes('Retained definition'));
  const summary = originalDetails(page).locator(':scope > summary');
  await focusByTab(page, summary); await page.keyboard.press('Enter');
  await retainedOriginal(page, firstText, 'before observation');
  unchanged.release();
  // Arrival of the next loop request is a repaint barrier: the first real
  // response has passed through renderDetail before this request can be sent.
  await arrival(changed, 'next wait after unchanged repaint');
  await retainedOriginal(page, firstText, 'after actual changed:false repaint');
  const beforeStop = await read(owner, '/graphs/' + graphId);
  assert.deepEqual(beforeStop.budget, running.budget);
  const stopBody = { requestId: fixtureRequestId(), expectedGraphControlRevision: beforeStop.graph.controlRevision };
  const stopped = await ownerAPI(owner, '/graphs/' + graphId + '/stop', 'POST', stopBody);
  assert.equal(stopped.status, 202, JSON.stringify(stopped.envelope.error));
  assert.equal(stopped.envelope.receipt.operation, 'graphs.stop'); assert.equal(stopped.envelope.receipt.requestId, stopBody.requestId);
  assert.equal(stopped.envelope.data.stopAccepted, true); assert.equal(stopped.envelope.data.reason, 'accepted');
  changed.release();
  await page.waitForFunction(() => /Task group · (cancelling|cancelled)/.test(document.getElementById('graph-detail-heading')?.textContent ?? ''));
  assert.ok(waits.length >= 2); assert.equal(waits[1].changed, true);
  await retainedOriginal(page, firstText, 'after actual changed:true repaint');
  const settled = await pollOwner(owner, '/attempts/' + actual.id, attempt => attempt.phase === 'settled', { timeoutMs: 30000 });
  assert.equal(settled.outcome, 'cancelled'); assert.equal(settled.cleanup.state, 'verified');
  const afterStop = await read(owner, '/graphs/' + graphId);
  assert.deepEqual(afterStop.budget, running.budget); assert.equal(afterStop.activeClaims.length, 0);
  assert.equal((await read(owner, '/tasks/' + running.nodes[0].taskId + '/attempts')).items.length, 1);

  // Graph definitions are immutable in 006. Use a second actual graph with the
  // SAME node key/version but different original; no fake graph-version update
  // or private state write is needed to catch a version-only/wrong-graph cache.
  const secondText = 'Second graph complete original π\r\nThis distinct graph must never display the first original.';
  const secondBody = create(secondText); secondBody.nodes[0].fakeScenario = 'graph-response';
  secondBody.nodes[0].acceptance.deliverable = 'response_only'; secondBody.nodes[0].workflow.review = 'auto';
  const second = await ownerAPI(owner, '/graphs', 'POST', secondBody);
  assert.equal(second.status, 201, JSON.stringify(second.envelope.error));
  const secondId = second.envelope.data.graph.id;
  assert.notEqual(secondId, graphId); assert.equal(second.envelope.data.graph.graphVersion, running.graph.graphVersion);
  await page.locator('#graph-refresh').click();
  await page.locator('#graph-list button').filter({ hasText: secondId }).click();
  await page.waitForFunction(id => document.getElementById('graph-facts')?.textContent.includes(id), secondId);
  const definitionToggle = page.locator('#graph-overview > details > summary').filter({ hasText: 'Retained definition' });
  if (!await definitionToggle.evaluate(node => node.parentElement.open)) await definitionToggle.click();
  await page.waitForFunction(text => document.getElementById('graph-definition')?.textContent.includes(text), secondText);
  assert.equal(await originalDetails(page).locator('pre').textContent(), secondText);
  assert.equal(await originalDetails(page).evaluate(node => node.open), false, 'A new graph must not inherit another original\'s open state.');
  assert.ok(!(await page.locator('#graph-definition').textContent()).includes(firstText));
  assert.deepEqual(browserWrites, [], 'definition observation/navigation must not issue any browser mutation');
  assert.equal(await readFile(path.join(f.project, 'hello.txt'), 'utf8'), 'unstaged\n');
  assert.deepEqual(await readFile(path.join(f.project, '.git', 'index')), f.indexBefore);
  assert.deepEqual(consoleErrors, []);
});
