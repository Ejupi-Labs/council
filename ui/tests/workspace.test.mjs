import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile, mkdir } from 'node:fs/promises';
import path from 'node:path';
import { fixture } from './helpers.mjs';

test('branded project/policy/draft flow preserves Git state across interfaces and owner restart', { timeout: 90000 }, async () => {
  const f = await fixture(); let owner;
  try {
    owner = await f.start();
    const page = await f.browser.newPage({ viewport: { width: 1440, height: 1000 } });
    const pageErrors = []; page.on('pageerror', error => pageErrors.push(error.message));
    await page.goto(owner.url);
    await page.waitForFunction(() => !location.hash);
    assert.equal(await page.locator('#policy-mode').inputValue(), 'autonomous');
    await page.locator('#project-path').fill(f.project);
    await page.locator('#open-project').click();
    await page.waitForFunction(() => document.querySelector('#project-select')?.value);
    const projectID = await page.locator('#project-select').inputValue();
    const text = '\uFEFFImplement café\r\nKeep original instructions. \uFFFD\n  ';
    await page.locator('#task-text').fill(text);
    const browserText = await page.locator('#task-text').inputValue();
    await page.locator('#save-draft').click();
    await page.waitForFunction(() => /Saved|saved/i.test(document.querySelector('#draft-status')?.textContent ?? ''));
    const drafts = f.cli(['drafts', 'list', '--project', projectID]);
    assert.equal(drafts.status, 0);
    assert.equal(drafts.envelope.data.items.length, 1);
    const draftID = drafts.envelope.data.items[0].id;
    const retrieved = f.cli(['drafts', 'get', draftID]);
    assert.equal(retrieved.envelope.data.originalText, browserText);
    assert.equal(retrieved.envelope.data.status, 'draft');
    assert.equal(retrieved.envelope.data.requestedPolicy.mode, 'autonomous');
    // Browsers normalize textarea CRLF by platform rules; the direct CLI preserves raw source.
    const raw = f.cli(['drafts', 'create', '--project', projectID, '--task', text]);
    assert.equal(raw.envelope.data.originalText, text);
    assert.deepEqual(await readFile(path.join(f.project, '.git/index')), f.indexBefore);
    assert.equal(await readFile(path.join(f.project, 'hello.txt'), 'utf8'), 'unstaged\n');
    await mkdir('.council/refactor/screenshots', { recursive: true });
    await page.evaluate(() => scrollTo(0, 0));
    await page.screenshot({ path: '.council/refactor/screenshots/foundation-desktop-viewport.png' });
    await page.screenshot({ path: '.council/refactor/screenshots/foundation-desktop.png', fullPage: true });
    await page.setViewportSize({ width: 360, height: 900 });
    await page.evaluate(() => scrollTo(0, 0));
    await page.screenshot({ path: '.council/refactor/screenshots/foundation-mobile-viewport.png' });
    await page.screenshot({ path: '.council/refactor/screenshots/foundation-mobile.png', fullPage: true });
    assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true);
    await page.close();
    await owner.stop(); owner = undefined;
    const after = f.cli(['drafts', 'get', draftID]);
    assert.equal(after.envelope.data.originalText, browserText);
    assert.equal(after.envelope.data.projectId, projectID);
    assert.equal(f.cli(['policy', 'get']).envelope.data.mode, 'autonomous');
    owner = await f.start();
    const reopened = await f.browser.newPage();
    await reopened.goto(owner.url);
    await reopened.waitForFunction(() => document.querySelector('#project-select')?.options.length > 1);
    assert.equal(pageErrors.length, 0, pageErrors.join('\n'));
  } finally { if (owner) await owner.stop(); await f.close(); }
});

test('an explicit policy reload cannot overwrite a newer dropdown edit', { timeout: 60000 }, async () => {
  const f = await fixture(); let owner;
  try {
    owner = await f.start(); const page = await f.browser.newPage();
    await page.goto(owner.url); await page.waitForFunction(() => !document.querySelector('#policy-mode')?.disabled);
    assert.equal(f.cli(['policy', 'set', 'supervised', '--expected-revision', '1']).status, 0);
    await page.locator('#save-policy').click();
    await page.locator('#reload-policy').waitFor({ state: 'visible' });
    let release; let arrived;
    const pending = new Promise(resolve => { arrived = resolve; });
    const gate = new Promise(resolve => { release = resolve; });
    await page.route('**/api/v1/policy', async route => { const response = await route.fetch(); arrived(); await gate; await route.fulfill({ response }); }, { times: 1 });
    await page.locator('#reload-policy').click(); await pending;
    await page.locator('#policy-mode').selectOption('supervised');
    await page.locator('#policy-mode').selectOption('autonomous');
    const delivered = page.waitForResponse(response => response.url().endsWith('/api/v1/policy') && response.request().method() === 'GET');
    release(); await (await delivered).finished();
    await page.evaluate(() => new Promise(resolve => requestAnimationFrame(resolve)));
    assert.equal(await page.locator('#policy-mode').inputValue(), 'autonomous');
  } finally { if (owner) await owner.stop(); await f.close(); }
});

test('in-page navigation fragments never replace the stored access token', { timeout: 60000 }, async () => {
  const f = await fixture(); let owner;
  try {
    owner = await f.start();
    const token = new URLSearchParams(new URL(owner.url).hash.slice(1)).get('token');
    assert.ok(token, 'fixture owner token is unavailable');
    const page = await f.browser.newPage({ viewport: { width: 1440, height: 1000 } });
    const pageErrors = []; page.on('pageerror', error => pageErrors.push(error.message));
    await page.goto(owner.url);
    await page.waitForFunction(() => !location.hash);
    assert.equal(await page.evaluate(() => sessionStorage.getItem('council.accessToken')), token);
    // A navigation link adds "#agents"; reloading must not mistake it for a credential.
    await page.locator('a[href="#agents"]').first().click();
    await page.waitForFunction(() => location.hash === '#agents');
    await page.reload();
    assert.equal(await page.evaluate(() => sessionStorage.getItem('council.accessToken')), token);
    assert.equal(await page.evaluate(() => location.hash), '#agents', 'navigation fragment must be left in place');
    assert.equal(pageErrors.length, 0, pageErrors.join('\n'));
  } finally { if (owner) await owner.stop(); await f.close(); }
});
