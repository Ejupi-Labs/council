import test from 'node:test';
import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { fixture } from './helpers.mjs';

test('browser fixes a byte snapshot, decodes split Unicode and drops late evidence after task selection changes', { timeout: 90000 }, async () => {
  const f = await fixture(); let owner;
  try {
    owner = await f.start(); const page = await f.browser.newPage(); await page.goto(owner.url);
    await page.locator('#project-path').fill(f.project); await page.locator('#open-project').click();
    await page.waitForFunction(() => !document.querySelector('#start-task')?.disabled);
    await page.locator('#execution-options > summary').click(); await page.locator('#task-fake').check(); await page.locator('#task-deliverable').selectOption('response_only');
    await page.locator('#task-text').fill('First retained response'); await page.locator('#start-task').click();
    await page.locator('#attempt-status').filter({ hasText: 'Verification pending' }).waitFor();
    let artifactId;
    await page.route('**/api/v1/artifacts/*', async route => {
      if (new URL(route.request().url()).pathname.endsWith('/bytes')) return route.continue();
      const response = await route.fetch(); const envelope = await response.json(); artifactId = envelope.data.id;
      envelope.data.byteLength = String(raw.length); envelope.data.sha256 = hash;
      await route.fulfill({ response, json: envelope });
    });
    const text = 'x'.repeat(65535) + '🙂 Grüße\n'; const raw = Buffer.from(text); const hash = createHash('sha256').update(raw).digest('hex');
    const throughs = [];
    await page.route('**/api/v1/artifacts/*/bytes*', async route => {
      const url = new URL(route.request().url()); const offset = Number(url.searchParams.get('offset') ?? '0'); throughs.push(url.searchParams.get('through'));
      const end = Math.min(raw.length, offset + 65536);
      await route.fulfill({ status: 200, contentType: 'application/json', json: { apiVersion: 1, requestId: '11111111111111111111111111111111', data: {
        artifactId, offset: String(offset), nextOffset: String(end), through: String(raw.length), bytesBase64: raw.subarray(offset, end).toString('base64'), eof: end === raw.length,
        streamClosed: true, completeness: 'complete', sha256: hash,
      } } });
    });
    await page.locator('#read-response').click(); await page.locator('#artifact-status').filter({ hasText: 'stream closed' }).waitFor();
    assert.equal(await page.locator('#artifact-output').textContent(), text); assert.deepEqual(throughs, [null, String(raw.length)]);
    await page.unroute('**/api/v1/artifacts/*'); await page.unroute('**/api/v1/artifacts/*/bytes*');
    await page.locator('#task-text').fill('Second retained response'); await page.locator('#start-task').click();
    await page.locator('#task-original').filter({ hasText: 'Second retained response' }).waitFor({ state: 'attached' });
    await page.locator('#attempt-status').filter({ hasText: 'Verification pending' }).waitFor();
    let release; let arrived; const gate = new Promise(resolve => { release = resolve; }); const pending = new Promise(resolve => { arrived = resolve; });
    await page.route('**/api/v1/artifacts/*/bytes*', async route => { const response = await route.fetch(); arrived(); await gate; await route.fulfill({ response }); }, { times: 1 });
    await page.locator('#read-response').click(); await pending;
    await page.locator('#task-list button').first().click();
    await page.locator('#task-original').filter({ hasText: 'First retained response' }).waitFor({ state: 'attached' }); release();
    await page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))));
    assert.equal(await page.locator('#artifact-output').textContent(), '');
    assert.equal(await page.locator('#reviewed-response').isChecked(), false);
  } finally { if (owner) await owner.stop(); await f.close(); }
});
