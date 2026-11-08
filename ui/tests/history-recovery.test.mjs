import assert from 'node:assert/strict';
import { readFile, writeFile } from 'node:fs/promises';
import test from 'node:test';
import { fixture, openWorkspace } from './helpers.mjs';

test('real pre-permit collision is reconciled through the UI without changing the existing file', { timeout: 120000 }, async t => {
  const f = await fixture(t), owner = await f.start();
  const page = await openWorkspace(f, owner, { fake: false });
  const project = await page.locator('#project-select').inputValue();
  const capture = f.cli(['history', 'capture', '--project', project]);
  assert.equal(capture.status, 0, JSON.stringify(capture.envelope.error));
  const snapshot = capture.envelope.data.snapshot;

  await page.locator('#task-text').fill('Preserve this unrelated draft.');
  await page.locator('#history-panel > summary').click();
  await page.locator('#history-snapshot-id').fill(snapshot.snapshotId);
  await page.getByRole('button', { name: 'Inspect snapshot', exact: true }).click();
  await page.locator('#history-facts').filter({ hasText: snapshot.snapshotId }).waitFor();
  await page.locator('#history-filename').fill('history-recovery-test.json');
  const previewResponse = page.waitForResponse(r => r.request().method() === 'POST' && new URL(r.url()).pathname === '/api/v1/history/exports/preview');
  await page.getByRole('button', { name: 'Preview export', exact: true }).click();
  const previewHTTP = await previewResponse, previewEnvelope = await previewHTTP.json();
  assert.equal(previewHTTP.status(), 201, JSON.stringify(previewEnvelope.error));
  const preview = previewEnvelope.data.preview;
  await page.locator('#history-preview-facts').filter({ hasText: preview.destinationPath }).waitFor();

  const unrelated = 'Unrelated existing file: must remain untouched.\n';
  await writeFile(preview.destinationPath, unrelated, { flag: 'wx' });
  const exportResponse = page.waitForResponse(r => r.request().method() === 'POST' && new URL(r.url()).pathname === '/api/v1/history/exports');
  await page.getByRole('button', { name: 'Export this snapshot', exact: true }).click();
  const exportHTTP = await exportResponse, exportEnvelope = await exportHTTP.json();
  assert.ok([200, 201, 202].includes(exportHTTP.status()), JSON.stringify(exportEnvelope.error));
  const operation = exportEnvelope.data.operation;
  assert.equal(operation.state, 'prepared');
  assert.equal(operation.nextAction, 'reconcile');
  // Windows may conservatively report root_unavailable when the existing file's
  // ACL cannot be verified; Unix reports target_conflict for an existing name.
  assert.ok(operation.blockers.some(code => ['target_conflict', 'root_unavailable'].includes(code)), JSON.stringify(exportEnvelope));
  await page.locator('#history-operation-facts').filter({ hasText: operation.operationId }).waitFor();
  assert.equal(await readFile(preview.destinationPath, 'utf8'), unrelated);

  const reconcilePath = '/api/v1/history/operations/' + operation.operationId + '/reconciliations';
  const reconcileResponse = page.waitForResponse(r => r.request().method() === 'POST' && new URL(r.url()).pathname === reconcilePath);
  await page.getByRole('button', { name: 'Verify no publication', exact: true }).click();
  const reconcileHTTP = await reconcileResponse, result = await reconcileHTTP.json();
  assert.equal(reconcileHTTP.status(), 200, JSON.stringify(result.error));
  assert.equal(result.data.decision, 'no_publication');
  assert.deepEqual(result.data.observationCodes, ['no_permit_proven']);
  assert.equal(result.data.operation.operationId, operation.operationId);
  assert.equal(result.data.operation.state, 'no_publication');
  await page.locator('#history-operation-facts').filter({ hasText: 'no_publication' }).waitFor();
  assert.equal(await readFile(preview.destinationPath, 'utf8'), unrelated);
  assert.equal(await page.locator('#task-text').inputValue(), 'Preserve this unrelated draft.');

  const inspected = f.cli(['history', 'operation', operation.operationId]);
  assert.equal(inspected.status, 0, JSON.stringify(inspected.envelope.error));
  assert.deepEqual(inspected.envelope.data.operation, result.data.operation);
  const retried = f.cli(['history', 'reconcile', operation.operationId, '--revision', operation.revision, '--request-id', result.requestId]);
  assert.equal(retried.status, 0, JSON.stringify(retried.envelope.error));
  assert.deepEqual(retried.envelope.receipt, result.receipt);
  assert.deepEqual(retried.envelope.data, result.data);
  assert.equal(await readFile(preview.destinationPath, 'utf8'), unrelated);
});
