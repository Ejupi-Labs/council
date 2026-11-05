import test from 'node:test';
import assert from 'node:assert/strict';
import { fixture } from './helpers.mjs';

test('lost save acknowledgement retries the same intent while preserving newer typing', { timeout: 60000 }, async () => {
  const f = await fixture(); let owner;
  try {
    const projectID = f.cli(['projects', 'open', f.project]).envelope.data.project.id;
    owner = await f.start(); const page = await f.browser.newPage();
    await page.goto(owner.url); await page.waitForFunction(() => document.querySelector('#project-select')?.value);
    const sent = []; let attempt = 0;
    await page.route('**/api/v1/drafts', async route => {
      if (route.request().method() !== 'POST') return route.continue();
      sent.push(route.request().postDataJSON());
      attempt++;
      if (attempt === 1) { await route.fetch(); await route.abort('failed'); }
      else if (attempt === 2) await route.fulfill({ status: 401, contentType: 'application/json', body: JSON.stringify({ apiVersion: 1, requestId: sent.at(-1).requestId, error: { code: 'unauthenticated', message: 'Access is temporarily unavailable.' } }) });
      else await route.continue();
    });
    await page.locator('#task-text').fill('First exact saved intent');
    await page.locator('#save-draft').click();
    await page.waitForFunction(() => /unresolved/i.test(document.querySelector('#draft-status')?.textContent ?? ''));
    await page.locator('#task-text').fill('Newer typing must survive');
    await page.locator('#save-draft').click();
    await page.waitForFunction(() => /temporarily unavailable/i.test(document.querySelector('#draft-status')?.textContent ?? ''));
    await page.locator('#save-draft').click();
    await page.waitForFunction(() => /Reconnect/i.test(document.querySelector('#draft-status')?.textContent ?? ''));
    await page.locator('#reconnect').click();
    await page.waitForFunction(() => /Connected to the local workspace/i.test(document.querySelector('#app-status')?.textContent ?? ''));
    await page.locator('#save-draft').click();
    await page.waitForFunction(() => /Saved revision 1/i.test(document.querySelector('#draft-status')?.textContent ?? ''));
    assert.equal(sent.length, 3); assert.deepEqual(sent[0], sent[1]); assert.deepEqual(sent[0], sent[2]);
    assert.equal(await page.locator('#task-text').inputValue(), 'Newer typing must survive');
    const listed = f.cli(['drafts', 'list', '--project', projectID]).envelope.data.items;
    assert.equal(listed.length, 1);
    assert.equal(f.cli(['drafts', 'get', listed[0].id]).envelope.data.originalText, 'First exact saved intent');
    await page.locator('#save-draft').click();
    await page.waitForFunction(() => /Saved revision 2/i.test(document.querySelector('#draft-status')?.textContent ?? ''));
    assert.equal(f.cli(['drafts', 'get', listed[0].id]).envelope.data.originalText, 'Newer typing must survive');
    // U+FEFF is preserved content, not Unicode White_Space; UI and CLI use the same rule.
    await page.locator('#task-text').fill('\uFEFF'); await page.locator('#save-draft').click();
    await page.waitForFunction(() => /Saved revision 3/i.test(document.querySelector('#draft-status')?.textContent ?? ''));
    assert.equal(f.cli(['drafts', 'get', listed[0].id]).envelope.data.originalText, '\uFEFF');
  } finally { if (owner) await owner.stop(); await f.close(); }
});

test('late draft reads and stale writes preserve local edits with explicit reconciliation', { timeout: 60000 }, async () => {
  const f = await fixture(); let owner;
  try {
    const projectID = f.cli(['projects', 'open', f.project]).envelope.data.project.id;
    const original = f.cli(['drafts', 'create', '--project', projectID, '--task', 'Saved original']).envelope.data;
    owner = await f.start(); const page = await f.browser.newPage();
    await page.goto(owner.url); await page.locator(`[data-draft-id="${original.id}"]`).waitFor();
    let release; let arrived;
    const pending = new Promise(resolve => { arrived = resolve; });
    const gate = new Promise(resolve => { release = resolve; });
    await page.route(`**/api/v1/drafts/${original.id}`, async route => {
      const response = await route.fetch(); arrived(); await gate; await route.fulfill({ response });
    }, { times: 1 });
    await page.locator(`[data-draft-id="${original.id}"]`).click(); await pending;
    await page.locator('#task-text').fill('New unsaved text during read'); release();
    await page.waitForFunction(() => /newer local edits/i.test(document.querySelector('#draft-status')?.textContent ?? ''));
    assert.equal(await page.locator('#task-text').inputValue(), 'New unsaved text during read');
    await page.locator(`[data-draft-id="${original.id}"]`).click();
    await page.waitForFunction(() => document.querySelector('#task-text')?.value === 'Saved original');
    const concurrent = f.cli(['drafts', 'update', original.id, '--expected-revision', '1', '--task', 'Concurrent writer']);
    assert.equal(concurrent.status, 0);
    await page.locator('#task-text').fill('My retained local correction'); await page.locator('#save-draft').click();
    await page.locator('#apply-latest-revision').waitFor({ state: 'visible' });
    assert.equal(await page.locator('#task-text').inputValue(), 'My retained local correction');
    assert.equal(f.cli(['drafts', 'get', original.id]).envelope.data.originalText, 'Concurrent writer');
    await page.locator('#apply-latest-revision').click();
    await page.waitForFunction(() => /latest|reconciled|revision 2/i.test(document.querySelector('#draft-status')?.textContent ?? ''));
    assert.equal(await page.locator('#task-text').inputValue(), 'My retained local correction');
    await page.locator('#save-draft').click();
    await page.waitForFunction(() => /Saved revision 3/i.test(document.querySelector('#draft-status')?.textContent ?? ''));
    assert.equal(f.cli(['drafts', 'get', original.id]).envelope.data.originalText, 'My retained local correction');
  } finally { if (owner) await owner.stop(); await f.close(); }
});
