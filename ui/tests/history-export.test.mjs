import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { lstat, readFile, readdir } from 'node:fs/promises';
import path from 'node:path';
import test from 'node:test';
import { fixture, fixtureRequestId, openWorkspace, ownerAPI, pollOwner } from './helpers.mjs';

const canonical = value => {
  if (Array.isArray(value)) return '[' + value.map(canonical).join(',') + ']';
  if (value && typeof value === 'object') return '{' + Object.keys(value).sort().map(key => JSON.stringify(key) + ':' + canonical(value[key])).join(',') + '}';
  return JSON.stringify(value);
};

async function keyboardActivateAt360(page, button) {
  await page.setViewportSize({ width: 360, height: 800 }); await button.scrollIntoViewIfNeeded();
  await button.focus(); await page.keyboard.press('Tab'); await page.keyboard.press('Shift+Tab');
  const state = await button.evaluate(node => ({ focused: document.activeElement === node, outline: getComputedStyle(node).outlineStyle !== 'none' && parseFloat(getComputedStyle(node).outlineWidth) > 0, width: node.getBoundingClientRect().width, viewport: innerWidth, documentWidth: document.documentElement.scrollWidth }));
  assert.equal(state.focused, true); assert.equal(state.outline, true); assert.ok(state.width <= state.viewport && state.documentWidth <= state.viewport + 1);
  await page.keyboard.press('Enter');
}

test('CLI snapshot is exported from UI once across a lost acknowledgement', { timeout: 120000 }, async t => {
  const f = await fixture(t), owner = await f.start();
  const page = await openWorkspace(f, owner, { fake: false });
  const projectId = await page.locator('#project-select').inputValue();
  const sentinel = 'PRIVATE_FAKE_TASK_CONTENT_029fa';
  const task = await ownerAPI(owner, '/tasks', 'POST', { requestId: fixtureRequestId(), projectId, originalText: sentinel, executionMode: 'fake', fakeScenario: 'response', workflow: { mode: 'author_only' }, acceptance: { deliverable: 'response_only' } });
  assert.equal(task.status, 201, JSON.stringify(task.envelope.error));
  await pollOwner(owner, '/attempts/' + task.envelope.data.attempt.id, value => value.phase === 'settled', { timeoutMs: 30000 });
  const captured = f.cli(['history', 'capture', '--project', projectId]);
  assert.equal(captured.status, 0, JSON.stringify(captured.envelope.error));
  const snapshot = captured.envelope.data.snapshot;

  let admitted = 0;
  page.on('request', request => { if (request.method() === 'POST' && new URL(request.url()).pathname === '/api/v1/tasks') admitted++; });
  await page.locator('#task-text').fill('Keep this draft while exporting history.');
  await page.locator('#history-panel > summary').click();
  await page.locator('#history-snapshot-id').fill(snapshot.snapshotId);
  await page.getByRole('button', { name: 'Inspect snapshot', exact: true }).click();
  await page.locator('#history-facts').filter({ hasText: snapshot.snapshotId }).waitFor();
  const outputName = 'history-browser-export.json';
  await page.locator('#history-filename').fill(outputName);
  const previewResponse = page.waitForResponse(r => r.request().method() === 'POST' && new URL(r.url()).pathname === '/api/v1/history/exports/preview');
  await keyboardActivateAt360(page, page.getByRole('button', { name: 'Preview export', exact: true }));
  const previewHTTP = await previewResponse, previewEnvelope = await previewHTTP.json();
  assert.equal(previewHTTP.status(), 201, JSON.stringify(previewEnvelope.error));
  const preview = previewEnvelope.data.preview;
  assert.equal(preview.snapshotId, snapshot.snapshotId); assert.equal(preview.filename, outputName);
  assert.equal(path.resolve(preview.destinationPath), path.resolve(f.home, '.council', 'v2', outputName));
  await page.locator('#history-preview-facts').filter({ hasText: preview.destinationPath }).waitFor();
  await assert.rejects(lstat(preview.destinationPath), error => error.code === 'ENOENT');

  let saved, savedStatus; const sent = [];
  let arrive, fail;
  const committedResponse = new Promise((resolve, reject) => { arrive = resolve; fail = reject; });
  await page.route('**/api/v1/history/exports', async route => {
    if (route.request().method() !== 'POST') return route.continue();
    sent.push(route.request().postDataJSON());
    if (sent.length === 1) {
      try {
        const response = await route.fetch(); savedStatus = response.status(); saved = await response.json();
        await route.abort('failed'); arrive();
      } catch (error) { fail(error); throw error; }
    }
    else await route.continue();
  });
  const exportButton = page.getByRole('button', { name: 'Export this snapshot', exact: true });
  await keyboardActivateAt360(page, exportButton);
  let timer;
  try { await Promise.race([committedResponse, new Promise((_, reject) => { timer = setTimeout(() => reject(new Error('The actual export response was not observed in 30 seconds')), 30000); })]); }
  finally { clearTimeout(timer); }
  const retry = page.getByRole('button', { name: 'Retry same request', exact: true });
  await retry.waitFor();
  assert.ok(saved?.receipt, `the owner must actually commit before the response is lost (status ${savedStatus}, response ${JSON.stringify(saved)})`);
  const operationId = saved.data.operation.operationId;
  assert.equal(saved.data.operation.snapshotId, snapshot.snapshotId);
  const replayResponse = page.waitForResponse(r => r.request().method() === 'POST' && new URL(r.url()).pathname === '/api/v1/history/exports' && r.request().postDataJSON().requestId === sent[0].requestId);
  await retry.click();
  const replay = await (await replayResponse).json();
  assert.deepEqual(sent[1], sent[0], 'retry must send the complete original intent');
  assert.deepEqual(replay.receipt, saved.receipt); assert.equal(replay.data.operation.operationId, operationId);
  await page.locator('#history-operation-facts').filter({ hasText: operationId }).waitFor();
  assert.equal(await page.locator('#task-text').inputValue(), 'Keep this draft while exporting history.');

  const current = await pollOwner(owner, '/history/operations/' + operationId, data => data.operation.state === 'published', { timeoutMs: 30000 });
  assert.equal(current.operation.destinationPath, preview.destinationPath);
  assert.equal(current.operation.durability.fileFlush, 'succeeded');
  assert.ok(['succeeded', 'unavailable'].includes(current.operation.durability.directorySync));
  assert.equal(current.operation.durability.powerLossNamespaceGuarantee, false);
  const bytes = await readFile(preview.destinationPath, 'utf8'); const output = JSON.parse(bytes);
  assert.equal(output.format, 'council.metadata-history/1'); assert.deepEqual(output.snapshot, snapshot);
  assert.equal(output.snapshot.completeness, 'incomplete_history');
  assert.equal(output.snapshot.notice, 'Incomplete observational history. Not a backup, evidence export, restore, or proof of acceptance.');
  assert.deepEqual(output.tasks.map(row => row.taskId), [task.envelope.data.task.id]);
  assert.deepEqual(output.attempts.map(row => row.attemptId), [task.envelope.data.attempt.id]);
  for (const excluded of [sentinel, f.project, preview.destinationPath, outputName]) assert.equal(bytes.includes(excluded), false);
  const expectedDigest = createHash('sha256').update(canonical({ snapshot: output.snapshot, tasks: output.tasks, attempts: output.attempts })).digest('hex');
  assert.equal(output.integrity.snapshotSha256, expectedDigest);
  assert.equal(bytes, canonical(output), 'the actual file must be the canonical encoded document');
  assert.deepEqual((await readdir(path.dirname(preview.destinationPath))).filter(name => name === outputName), [outputName]);
  const cli = f.cli(['history', 'export', '--preview', preview.previewId, '--revision', preview.revision, '--request-id', sent[0].requestId]);
  assert.equal(cli.status, 0, JSON.stringify(cli.envelope.error));
  assert.deepEqual(cli.envelope.receipt, saved.receipt); assert.equal(cli.envelope.data.operation.operationId, operationId);
  assert.equal(await readFile(preview.destinationPath, 'utf8'), bytes, 'cross-interface replay cannot publish another file');
  assert.equal(admitted, 0, 'history observation and export must not start a task or model');

  // Inject only the owner GET projection to verify that a real documented
  // pre-permit blocker remains inspectable; this does not claim a physical
  // conflict occurred or exercise physical reconciliation.
  await page.route('**/api/v1/history/operations/' + operationId, async route => {
    const actual = await route.fetch(); const projection = await actual.json();
    projection.data.operation.state = 'prepared'; projection.data.operation.outcome = null;
    projection.data.operation.durability = null; projection.data.operation.blockers = ['target_conflict'];
    projection.data.operation.nextAction = 'reconcile';
    await route.fulfill({ response: actual, json: projection });
  });
  await page.getByRole('button', { name: 'Inspect operation', exact: true }).click();
  await page.locator('#history-operation-facts').filter({ hasText: 'target_conflict' }).waitFor();
  assert.equal(await page.getByRole('button', { name: 'Verify no publication', exact: true }).isEnabled(), true);
});
