import assert from 'node:assert/strict';
import test from 'node:test';
import { fixture, fixtureRequestId, openWorkspace, ownerAPI, pollOwner, responseBarrier } from './helpers.mjs';

const nextPaint = page => page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))));

async function boundedArrival(barrier, page, boundary) {
  let timer;
  try {
    await Promise.race([barrier.arrived, new Promise((_, reject) => { timer = setTimeout(() => reject(new Error('Controlled boundary was not reached: ' + boundary)), 15000); })]);
  } catch (error) {
    const observed = await page.evaluate(() => Object.fromEntries(['task-detail-heading', 'attempt-status', 'workflow-status', 'guidance-status', 'execution-status'].map(id => [id, document.getElementById(id)?.textContent ?? '']))).catch(() => ({ page: 'unavailable' }));
    throw new Error(error.message + ' · observed UI ' + JSON.stringify(observed));
  } finally { clearTimeout(timer); }
}

test('a delayed settled observer detail cannot replace newer guidance while its workflow read is pending', { timeout: 90000 }, async t => {
  const oldDetail = responseBarrier(); const newWorkflow = responseBarrier();
  const f = await fixture();
  t.after(async () => { oldDetail.release(); newWorkflow.release(); await f.close(); });
  const owner = await f.start(); const page = await openWorkspace(f, owner, { workflow: 'auto' });
  await page.locator('#task-workflow').selectOption('author_only');
  await page.locator('#task-fake-scenario').selectOption('slow');
  await page.locator('#task-text').fill('CONTROLLED OBSERVER RACE: retain the exact original instructions.');
  const observing = page.waitForRequest(request => request.method() === 'GET' && /^\/api\/v1\/attempts\/[a-f0-9]{32}\/wait$/.test(new URL(request.url()).pathname));
  const admitted = page.waitForResponse(response => response.request().method() === 'POST' && new URL(response.url()).pathname === '/api/v1/tasks');
  await page.locator('#start-task').click(); const admission = (await (await admitted).json()).data;
  await page.locator('#task-detail-heading').filter({ hasText: admission.task.id.slice(0, 8) }).waitFor();
  await pollOwner(owner, '/attempts/' + admission.attempt.id, data => data.phase === 'running');
  await observing;
  const taskPath = '/api/v1/tasks/' + admission.task.id;
  let heldOld = false; let heldNew = false;
  await page.route('**' + taskPath, async route => {
    if (route.request().method() !== 'GET') return route.continue();
    const response = await route.fetch(); const envelope = await response.json();
    if (!heldOld && envelope.data.task.currentVersion === '1' && envelope.data.latestAttempt?.phase === 'settled') {
      heldOld = true; await oldDetail.hold();
    }
    await route.fulfill({ response, json: envelope });
  });
  await page.route('**' + taskPath + '/workflow', async route => {
    if (route.request().method() !== 'GET') return route.continue();
    const response = await route.fetch(); const envelope = await response.json();
    if (!heldNew && envelope.data.taskVersion === '2') { heldNew = true; await newWorkflow.hold(); }
    await route.fulfill({ response, json: envelope });
  });
  // A separate actual client stops the fixed slow process. The browser's own
  // terminal observer, rather than a manual refresh, now obtains the held v1 read.
  const stopped = await ownerAPI(owner, '/attempts/' + admission.attempt.id + '/stop', 'POST', { requestId: fixtureRequestId() });
  assert.equal(stopped.status, 202); await boundedArrival(oldDetail, page, 'terminal observer task v1 read');
  const savedGuidance = 'CONTROLLED VERSION 2: require this complete new guidance.';
  const fresh = await ownerAPI(owner, '/tasks/' + admission.task.id);
  const guidance = await ownerAPI(owner, '/tasks/' + admission.task.id + '/guidance', 'PUT', { requestId: fixtureRequestId(), expectedRevision: fresh.envelope.data.task.revision, guidanceText: savedGuidance });
  assert.equal(guidance.status, 200);
  await page.locator('#refresh-attempt').click();
  await boundedArrival(newWorkflow, page, 'guidance v2 workflow read');
  await page.locator('#task-detail-heading').filter({ hasText: 'version 2' }).waitFor();
  await page.getByText('Add guidance for a new task version', { exact: true }).click();
  const newerLocal = 'Newer local guidance entered while the version 2 workflow read is delayed.';
  await page.locator('#task-guidance').fill(newerLocal);
  const releasedOld = page.waitForResponse(response => response.request().method() === 'GET' && new URL(response.url()).pathname === taskPath);
  const refreshedHistory = page.waitForResponse(response => response.request().method() === 'GET' && new URL(response.url()).pathname === taskPath + '/attempts');
  oldDetail.release(); await (await releasedOld).finished(); await nextPaint(page);
  newWorkflow.release();
  await (await refreshedHistory).finished(); await nextPaint(page);
  assert.match(await page.locator('#task-detail-heading').textContent(), /version 2/);
  const currentVersion = page.locator('#task-facts dt').filter({ hasText: /^Current task version$/ });
  assert.equal(await currentVersion.locator('xpath=following-sibling::dd[1]').textContent(), '2');
  assert.match(await page.locator('#workflow-facts').textContent(), /Selected task version2.*Current/s);
  assert.equal(await page.locator('#task-guidance').inputValue(), newerLocal);
  const actual = await ownerAPI(owner, '/tasks/' + admission.task.id);
  assert.equal(actual.envelope.data.task.currentVersion, '2'); assert.equal(actual.envelope.data.version.guidanceText, savedGuidance);
  const history = await ownerAPI(owner, '/tasks/' + admission.task.id + '/attempts');
  assert.equal(history.envelope.data.items.length, 1, 'saving guidance and reconciling a read start no execution');
  assert.equal(heldOld, true); assert.equal(heldNew, true);
});

test('a delayed adoption acknowledgement preserves another selected task status and its exact captured receipt', { timeout: 90000 }, async t => {
  const barrier = responseBarrier(); const f = await fixture();
  t.after(async () => { barrier.release(); await f.close(); });
  const owner = await f.start(); const page = await openWorkspace(f, owner, { workflow: 'auto' });
  const projectId = await page.locator('#project-select').inputValue();
  const createRetained = async name => {
    const response = await ownerAPI(owner, '/tasks', 'POST', { requestId: fixtureRequestId(), projectId, originalText: 'CONTROLLED SELECTION RACE: retained ' + name + '.', executionMode: 'fake', fakeScenario: 'response', workflow: { mode: 'author_only' }, acceptance: { deliverable: 'response_only' } });
    assert.equal(response.status, 201);
    await pollOwner(owner, '/attempts/' + response.envelope.data.attempt.id, data => data.phase === 'settled' && data.outcome === 'succeeded');
    return response.envelope.data;
  };
  const first = await createRetained('task A'); const second = await createRetained('task B');
  await page.locator('#refresh-tasks').click();
  await page.locator('#task-list button').filter({ hasText: first.task.id.slice(0, 8) }).click();
  await page.locator('#workflow-status').filter({ hasText: /Standalone task/ }).waitFor();
  await page.waitForFunction(() => !document.querySelector('#adopt-task-workflow')?.disabled);
  const path = '/api/v1/tasks/' + first.task.id + '/workflow/adopt';
  let captured; let acknowledged;
  await page.route('**' + path, async route => {
    captured = route.request().postDataJSON();
    const response = await route.fetch(); acknowledged = await response.json();
    await barrier.hold(); await route.fulfill({ response, json: acknowledged });
  });
  await page.locator('#adopt-task-workflow').click(); await boundedArrival(barrier, page, 'task A adoption acknowledgement');
  assert.equal(acknowledged.error, undefined);
  const pending = await page.evaluate(async taskId => {
    const { readSession } = await import('/api.js'); return readSession('workflowPending')['adopt:' + taskId];
  }, first.task.id);
  assert.deepEqual(pending.body, captured); assert.equal(pending.path, '/tasks/' + first.task.id + '/workflow/adopt');
  assert.equal(captured.taskVersion, first.task.currentVersion); assert.equal(captured.expectedRevision, first.task.revision);
  await page.locator('#task-list button').filter({ hasText: second.task.id.slice(0, 8) }).click();
  await page.locator('#task-detail-heading').filter({ hasText: second.task.id.slice(0, 8) }).waitFor();
  await page.locator('#workflow-status').filter({ hasText: /Standalone task/ }).waitFor();
  const selectedStatus = await page.locator('#workflow-status').textContent();
  await page.locator('#task-text').fill('Newer composer text while task A adoption acknowledgement is delayed.');
  await page.locator('#task-delivery').selectOption('apply_verified_local');
  const released = page.waitForResponse(response => response.request().method() === 'POST' && new URL(response.url()).pathname === path);
  barrier.release(); await (await released).finished();
  await page.waitForFunction(async taskId => {
    const { readSession } = await import('/api.js'); return !readSession('workflowPending')?.['adopt:' + taskId];
  }, first.task.id);
  await nextPaint(page);
  assert.match(await page.locator('#task-detail-heading').textContent(), new RegExp(second.task.id.slice(0, 8)));
  assert.equal(await page.locator('#workflow-status').textContent(), selectedStatus);
  assert.doesNotMatch(await page.locator('#workflow-status').textContent(), /Workflow adopted|Run task/);
  assert.equal(await page.locator('#task-text').inputValue(), 'Newer composer text while task A adoption acknowledgement is delayed.');
  assert.equal(await page.locator('#task-delivery').inputValue(), 'apply_verified_local');
  const replay = await ownerAPI(owner, '/tasks/' + first.task.id + '/workflow/adopt', 'POST', captured);
  assert.equal(replay.status, 200); assert.deepEqual(replay.envelope, acknowledged);
  const firstWorkflow = await ownerAPI(owner, '/tasks/' + first.task.id + '/workflow');
  assert.equal(firstWorkflow.envelope.data.taskVersion, acknowledged.data.taskVersion);
  assert.equal(firstWorkflow.envelope.data.workflow.currentStepId, null);
  const secondWorkflow = await ownerAPI(owner, '/tasks/' + second.task.id + '/workflow');
  assert.equal(secondWorkflow.envelope.data.workflow, null);
});
