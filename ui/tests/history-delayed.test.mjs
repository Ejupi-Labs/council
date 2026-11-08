import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { mkdir, readFile, writeFile } from 'node:fs/promises';
import path from 'node:path';
import test from 'node:test';
import { fixture, fixtureRequestId, openWorkspace, responseBarrier } from './helpers.mjs';

test('a delayed real export acknowledgement preserves the newer project, operation input and draft', { timeout: 120000 }, async t => {
  const f = await fixture(t), owner = await f.start(), barrier = responseBarrier();
  t.after(() => barrier.release());
  const page = await openWorkspace(f, owner, { fake: false });
  const projectA = await page.locator('#project-select').inputValue();
  const otherPath = path.join(f.home, 'Delayed response project'); await mkdir(otherPath);
  const git = args => { const result = spawnSync('git', args, { cwd: otherPath, env: f.env, encoding: 'utf8', shell: false }); assert.equal(result.status, 0, result.stderr); };
  git(['init', '-q']); await writeFile(path.join(otherPath, 'fixture.txt'), 'second project\n'); git(['add', 'fixture.txt']);
  git(['-c', 'user.name=Council Test', '-c', 'user.email=council@example.invalid', 'commit', '-qm', 'fixture']);
  await page.locator('#project-path').fill(otherPath); await page.locator('#open-project').click();
  await page.waitForFunction(id => { const value = document.querySelector('#project-select')?.value; return value && value !== id; }, projectA);
  const projectB = await page.locator('#project-select').inputValue();
  await page.locator('#project-select').selectOption(projectA);
  await page.waitForFunction(id => document.querySelector('#project-select')?.value === id, projectA);

  const capture = f.cli(['history', 'capture', '--project', projectA]);
  assert.equal(capture.status, 0, JSON.stringify(capture.envelope.error));
  const snapshot = capture.envelope.data.snapshot;
  await page.locator('#history-panel > summary').click();
  await page.locator('#history-snapshot-id').fill(snapshot.snapshotId);
  await page.getByRole('button', { name: 'Inspect snapshot', exact: true }).click();
  await page.locator('#history-facts').filter({ hasText: snapshot.snapshotId }).waitFor();
  await page.locator('#history-filename').fill('history-delayed-response.json');
  const previewResponse = page.waitForResponse(r => r.request().method() === 'POST' && new URL(r.url()).pathname === '/api/v1/history/exports/preview');
  await page.getByRole('button', { name: 'Preview export', exact: true }).click();
  const previewHTTP = await previewResponse, previewEnvelope = await previewHTTP.json();
  assert.equal(previewHTTP.status(), 201, JSON.stringify(previewEnvelope.error));
  const preview = previewEnvelope.data.preview;
  await page.locator('#history-preview-facts').filter({ hasText: preview.destinationPath }).waitFor();

  let saved, sent, taskStarts = 0, exportPosts = 0;
  page.on('request', request => {
    if (request.method() !== 'POST') return;
    const pathname = new URL(request.url()).pathname;
    if (pathname === '/api/v1/tasks') taskStarts++;
    if (pathname === '/api/v1/history/exports') exportPosts++;
  });
  await page.route('**/api/v1/history/exports', async route => {
    sent = route.request().postDataJSON();
    const response = await route.fetch(); saved = await response.json();
    await barrier.hold(); await route.fulfill({ response });
  });
  await page.getByRole('button', { name: 'Export this snapshot', exact: true }).click();
  let timer;
  try { await Promise.race([barrier.arrived, new Promise((_, reject) => { timer = setTimeout(() => reject(new Error('The real export response was not reached')), 30000); })]); }
  finally { clearTimeout(timer); }
  assert.ok(saved?.receipt, JSON.stringify(saved?.error));
  assert.equal(saved.data.operation.state, 'published');
  assert.equal(saved.data.operation.snapshotId, snapshot.snapshotId);
  assert.equal(saved.receipt.requestId, sent.requestId);

  await page.locator('#project-select').selectOption(projectB);
  const newerOperationInput = fixtureRequestId();
  await page.locator('#history-operation-id').fill(newerOperationInput);
  await page.locator('#task-text').fill('Newer project B draft survives delayed export.');
  assert.equal(await page.locator('#history-snapshot').isVisible(), false);
  barrier.release();
  await page.locator('#history-status').filter({ hasText: 'Your newer selection is retained' }).waitFor();
  assert.equal(await page.locator('#project-select').inputValue(), projectB);
  assert.equal(await page.locator('#history-operation-id').inputValue(), newerOperationInput);
  assert.equal(await page.locator('#task-text').inputValue(), 'Newer project B draft survives delayed export.');
  assert.equal(await page.locator('#history-snapshot').isVisible(), false);
  assert.equal(await page.locator('#history-operation').isVisible(), false);
  assert.equal(await page.locator('#history-pending').textContent(), '');
  assert.equal(exportPosts, 1); assert.equal(taskStarts, 0);

  const exact = f.cli(['history', 'export', '--preview', preview.previewId, '--revision', '1', '--request-id', sent.requestId]);
  assert.equal(exact.status, 0, JSON.stringify(exact.envelope.error));
  assert.deepEqual(exact.envelope.receipt, saved.receipt);
  assert.equal(exact.envelope.data.operation.operationId, saved.data.operation.operationId);
  const changed = f.cli(['history', 'export', '--preview', fixtureRequestId(), '--revision', '1', '--request-id', sent.requestId]);
  assert.notEqual(changed.status, 0);
  assert.equal(changed.envelope.error?.code, 'request_conflict');
  const output = JSON.parse(await readFile(preview.destinationPath, 'utf8'));
  assert.equal(output.snapshot.snapshotId, snapshot.snapshotId);
  assert.equal(output.snapshot.projectId, projectA);
});
