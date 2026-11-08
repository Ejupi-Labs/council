import assert from 'node:assert/strict';
import test from 'node:test';
import { fixture, openWorkspace } from './helpers.mjs';

const cacheKey = 'council.foundation.historyPending';

test('history never posts when session storage denies the exact pending intent', { timeout: 90000 }, async t => {
  const f = await fixture(t), owner = await f.start();
  const page = await openWorkspace(f, owner, { fake: false });
  await page.locator('#task-text').fill('Keep this independent draft.');
  await page.locator('#history-panel > summary').click();
  await page.evaluate(key => {
    const original = Storage.prototype.setItem;
    window.__historyOriginalSetItem = original;
    Storage.prototype.setItem = function (name, value) {
      if (name === key) throw new DOMException('Controlled storage denial', 'QuotaExceededError');
      return original.call(this, name, value);
    };
  }, cacheKey);

  const posts = [];
  page.on('request', request => { if (request.method() === 'POST' && new URL(request.url()).pathname.startsWith('/api/v1/history/')) posts.push(request.postDataJSON()); });
  await page.getByRole('button', { name: 'Capture selected project history', exact: true }).click();
  await page.locator('#history-status').filter({ hasText: 'No request was sent' }).waitFor();
  assert.equal(posts.length, 0);
  assert.equal(await page.locator('#task-text').inputValue(), 'Keep this independent draft.');
  const pendingText = await page.locator('#history-pending').textContent();
  const pendingId = pendingText.match(/[a-f0-9]{32}/)?.[0];
  assert.ok(pendingId, 'the unsent intent must retain its request identity');

  await page.evaluate(() => { Storage.prototype.setItem = window.__historyOriginalSetItem; delete window.__historyOriginalSetItem; });
  const response = page.waitForResponse(r => r.request().method() === 'POST' && new URL(r.url()).pathname === '/api/v1/history/snapshots');
  await page.getByRole('button', { name: 'Retry same request', exact: true }).click();
  const captured = await response, envelope = await captured.json();
  assert.equal(captured.status(), 201, JSON.stringify(envelope.error));
  assert.equal(posts.length, 1); assert.equal(posts[0].requestId, pendingId);
  await page.locator('#history-facts').filter({ hasText: envelope.data.snapshot.snapshotId }).waitFor();
  assert.equal(await page.locator('#task-text').inputValue(), 'Keep this independent draft.');
});

test('history preserves a different saved pending value rather than overwriting or posting', { timeout: 90000 }, async t => {
  const f = await fixture(t), owner = await f.start();
  const page = await openWorkspace(f, owner, { fake: false });
  await page.locator('#history-panel > summary').click();
  const original = '{"retained":"unreadable earlier intent"}';
  await page.evaluate(({ key, value }) => sessionStorage.setItem(key, value), { key: cacheKey, value: original });
  let posts = 0;
  page.on('request', request => { if (request.method() === 'POST' && new URL(request.url()).pathname.startsWith('/api/v1/history/')) posts++; });
  await page.getByRole('button', { name: 'Capture selected project history', exact: true }).click();
  await page.locator('#history-status').filter({ hasText: 'No request was sent' }).waitFor();
  assert.equal(posts, 0);
  assert.equal(await page.evaluate(key => sessionStorage.getItem(key), cacheKey), original);
});

test('history requires a readback even when the storage write succeeds', { timeout: 90000 }, async t => {
  const f = await fixture(t), owner = await f.start();
  const page = await openWorkspace(f, owner, { fake: false });
  await page.locator('#history-panel > summary').click();
  await page.evaluate(key => {
    const get = Storage.prototype.getItem, set = Storage.prototype.setItem;
    let written = false;
    window.__historyStorageMethods = { get, set };
    Storage.prototype.setItem = function (name, value) { const result = set.call(this, name, value); if (name === key) written = true; return result; };
    Storage.prototype.getItem = function (name) { if (name === key && written) throw new DOMException('Controlled readback denial', 'SecurityError'); return get.call(this, name); };
  }, cacheKey);
  const posts = [];
  page.on('request', request => { if (request.method() === 'POST' && new URL(request.url()).pathname.startsWith('/api/v1/history/')) posts.push(request.postDataJSON()); });
  await page.getByRole('button', { name: 'Capture selected project history', exact: true }).click();
  await page.locator('#history-status').filter({ hasText: 'No request was sent' }).waitFor();
  assert.equal(posts.length, 0);
  await page.evaluate(() => { Storage.prototype.getItem = window.__historyStorageMethods.get; Storage.prototype.setItem = window.__historyStorageMethods.set; delete window.__historyStorageMethods; });
  const saved = JSON.parse(await page.evaluate(key => sessionStorage.getItem(key), cacheKey));
  assert.match(saved.body.requestId, /^[a-f0-9]{32}$/);
  const response = page.waitForResponse(r => r.request().method() === 'POST' && new URL(r.url()).pathname === '/api/v1/history/snapshots');
  await page.getByRole('button', { name: 'Retry same request', exact: true }).click();
  assert.equal((await response).status(), 201);
  assert.equal(posts.length, 1); assert.deepEqual(posts[0], saved.body);
});
