import assert from 'node:assert/strict';
import { readFile, readdir } from 'node:fs/promises';
import path from 'node:path';
import test from 'node:test';
import { fixture, fixtureRequestId, openWorkspace, ownerAPI, pollOwner } from './helpers.mjs';

test('an inspected CLI export replays its exact receipt in UI, including a lost UI acknowledgement', { timeout: 120000 }, async t => {
  const f = await fixture(t), owner = await f.start();
  const page = await openWorkspace(f, owner, { fake: false });
  const projectId = await page.locator('#project-select').inputValue();
  const task = await ownerAPI(owner, '/tasks', 'POST', { requestId: fixtureRequestId(), projectId, originalText: 'Controlled fake history task.', executionMode: 'fake', fakeScenario: 'response', workflow: { mode: 'author_only' }, acceptance: { deliverable: 'response_only' } });
  assert.equal(task.status, 201, JSON.stringify(task.envelope.error));
  const taskId = task.envelope.data.task.id, attemptId = task.envelope.data.attempt.id;
  await pollOwner(owner, '/attempts/' + attemptId, value => value.phase === 'settled', { timeoutMs: 30000 });
  const attemptsBefore = await ownerAPI(owner, '/tasks/' + taskId + '/attempts');
  assert.equal(attemptsBefore.status, 200); assert.equal(attemptsBefore.envelope.data.items.length, 1);
  const run = args => {
    const result = f.cli(args);
    assert.equal(result.status, 0, JSON.stringify(result.envelope.error));
    return result.envelope;
  };
  const snapshot = run(['history', 'capture', '--project', projectId]).data.snapshot;
  const preview = run(['history', 'export-preview', '--snapshot', snapshot.snapshotId, '--revision', '1', '--filename', 'history-cli-replay.json']).data.preview;
  const originalRequestId = fixtureRequestId();
  const cliResult = run(['history', 'export', '--preview', preview.previewId, '--revision', '1', '--request-id', originalRequestId]);
  const operation = cliResult.data.operation;
  assert.equal(operation.state, 'published');
  assert.equal(operation.requestId, originalRequestId);
  const bytes = await readFile(preview.destinationPath, 'utf8');

  // The CLI response is retained by this test solely as independent evidence.
  // The user-facing journey starts with the operation ID, as when the original
  // command's acknowledgement is unavailable; the CLI transport itself was not dropped.
  await page.locator('#task-text').fill('Preserve my newer draft during CLI replay.');
  await page.locator('#history-panel > summary').click();
  await page.locator('#history-operation-id').fill(operation.operationId);
  await page.getByRole('button', { name: 'Inspect operation', exact: true }).click();
  await page.locator('#history-operation-facts').filter({ hasText: operation.operationId }).waitFor();
  assert.match(await page.locator('#history-operation-facts').textContent(), new RegExp(originalRequestId));

  const replayBodies = []; let savedReplay, received;
  const arrived = new Promise(resolve => { received = resolve; });
  await page.route('**/api/v1/history/exports', async route => {
    if (route.request().method() !== 'POST') return route.continue();
    replayBodies.push(route.request().postDataJSON());
    if (replayBodies.length === 1) {
      const response = await route.fetch(); savedReplay = await response.json(); await route.abort('failed'); received();
    } else await route.continue();
  });
  let taskStarts = 0;
  page.on('request', request => { if (request.method() === 'POST' && new URL(request.url()).pathname === '/api/v1/tasks') taskStarts++; });
  await page.getByRole('button', { name: 'Retry this exact export', exact: true }).click();
  let timer;
  try { await Promise.race([arrived, new Promise((_, reject) => { timer = setTimeout(() => reject(new Error('Actual replay acknowledgement did not arrive')), 30000); })]); }
  finally { clearTimeout(timer); }
  assert.deepEqual(replayBodies[0], { requestId: originalRequestId, previewId: preview.previewId, previewRevision: '1' });
  assert.deepEqual(savedReplay.receipt, cliResult.receipt);
  assert.equal(savedReplay.data.operation.operationId, operation.operationId);
  await page.getByRole('button', { name: 'Retry same request', exact: true }).waitFor();

  await page.reload();
  await page.locator('#history-panel > summary').click();
  await page.waitForFunction(() => { const button = document.querySelector('#history-retry'); return button && !button.hidden && !button.disabled; });
  const reply = page.waitForResponse(r => r.request().method() === 'POST' && new URL(r.url()).pathname === '/api/v1/history/exports');
  await page.getByRole('button', { name: 'Retry same request', exact: true }).click();
  const replay = await (await reply).json();
  assert.deepEqual(replayBodies[1], replayBodies[0]);
  assert.deepEqual(replay.receipt, cliResult.receipt); assert.equal(replay.data.operation.operationId, operation.operationId);
  await page.locator('#history-pending').filter({ hasText: 'Pending' }).waitFor({ state: 'hidden' });
  assert.equal(await page.locator('#task-text').inputValue(), 'Preserve my newer draft during CLI replay.');
  assert.equal(await readFile(preview.destinationPath, 'utf8'), bytes);
  assert.deepEqual((await readdir(path.dirname(preview.destinationPath))).filter(name => name === preview.filename), [preview.filename]);
  const attemptsAfter = await ownerAPI(owner, '/tasks/' + taskId + '/attempts');
  // These two read-time observations advance even after settlement; preserve
  // equality for every other returned attempt field and the exact count/order.
  const stableAttempts = items => items.map(({ observedAt, remainingSeconds, ...attempt }) => attempt);
  assert.equal(attemptsAfter.status, 200);
  assert.deepEqual(stableAttempts(attemptsAfter.envelope.data.items), stableAttempts(attemptsBefore.envelope.data.items));
  assert.equal(taskStarts, 0, 'history replay must not start another task or model');
});
