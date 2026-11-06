import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import path from 'node:path';
import { fixture } from './helpers.mjs';

async function open(f, owner) {
  const page = await f.browser.newPage();
  await page.goto(owner.url);
  await page.locator('#project-path').fill(f.project); await page.locator('#open-project').click();
  await page.waitForFunction(() => !document.querySelector('#task-text')?.disabled && !document.querySelector('#start-task')?.disabled);
  return page;
}
async function fakeOptions(page, scenario = 'response', deliverable = 'response_only') {
  await page.locator('.execution-options summary').click();
  await page.locator('#task-fake').check(); await page.locator('#task-fake-scenario').selectOption(scenario);
  await page.locator('#task-deliverable').selectOption(deliverable);
}

test('browser starts one labelled fake, retains exact dirty source, reads response and accepts only reviewed bytes', { timeout: 90000 }, async () => {
  const f = await fixture(); let owner;
  try {
    owner = await f.start(); const page = await open(f, owner); await fakeOptions(page);
    const original = 'Return a useful exact response. Grüße 🙂\nDo not modify source.';
    await page.locator('#task-text').fill(original);
    let admissions = 0;
    page.on('request', request => { if (request.method() === 'POST' && new URL(request.url()).pathname === '/api/v1/tasks') admissions++; });
    await page.locator('#start-task').click();
    await page.locator('#attempt-status').filter({ hasText: 'Verification pending' }).waitFor();
    assert.match(await page.locator('#attempt-status').textContent(), /LOCAL FAKE/);
    assert.equal(await page.locator('#task-original').textContent(), original); assert.equal(admissions, 1);
    assert.match(await page.locator('#attempt-facts').textContent(), /Unknown/);
    await page.locator('#read-response').click(); await page.locator('#artifact-status').filter({ hasText: 'stream closed' }).waitFor();
    assert.ok((await page.locator('#artifact-output').textContent()).length > 0);
    assert.equal(await page.locator('#accept-response').isEnabled(), false);
    await page.locator('#reviewed-response').check(); await page.locator('#accept-response').click();
    await page.locator('#task-facts').filter({ hasText: 'Reviewed response accepted' }).waitFor();
    assert.equal(await readFile(path.join(f.project, 'hello.txt'), 'utf8'), 'unstaged\n');
    assert.deepEqual(await readFile(path.join(f.project, '.git/index')), f.indexBefore);
    assert.equal(admissions, 1);
  } finally { if (owner) await owner.stop(); await f.close(); }
});

test('dirty saved draft Start freezes its autosave and start IDs across a lost reply while retaining newer typing', { timeout: 90000 }, async () => {
  const f = await fixture(); let owner;
  try {
    owner = await f.start(); const page = await open(f, owner); await fakeOptions(page);
    await page.locator('#task-text').fill('Saved original'); await page.locator('#save-draft').click();
    await page.locator('#draft-status').filter({ hasText: 'Saved revision 1' }).waitFor();
    await page.locator('#task-text').fill('Clicked exact intent 🙂');
    let release; let arrived;
    const gate = new Promise(resolve => { release = resolve; }); const reached = new Promise(resolve => { arrived = resolve; });
    let autosaveBody;
    await page.route('**/api/v1/drafts/*', async route => {
      if (route.request().method() !== 'PUT') return route.continue();
      autosaveBody = route.request().postDataJSON(); const response = await route.fetch(); arrived(); await gate; await route.fulfill({ response });
    }, { times: 1 });
    const starts = [];
    await page.route('**/api/v1/tasks', async route => {
      if (route.request().method() !== 'POST') return route.continue();
      starts.push(route.request().postDataJSON()); await route.fetch(); await route.abort('connectionfailed');
    }, { times: 1 });
    await page.locator('#start-task').click(); await reached;
    await page.locator('#task-text').fill('Newer unsaved typing'); release();
    await page.locator('#start-task').filter({ hasText: 'Retry pending start' }).waitFor();
    assert.equal(await page.locator('#task-text').inputValue(), 'Newer unsaved typing');
    await page.reload(); await page.waitForFunction(() => !document.querySelector('#start-task')?.disabled && document.querySelector('#start-task')?.textContent.includes('Retry pending start'));
    assert.equal(await page.locator('#task-text').inputValue(), 'Newer unsaved typing');
    await page.route('**/api/v1/tasks', async route => {
      if (route.request().method() === 'POST') starts.push(route.request().postDataJSON()); await route.continue();
    });
    await page.locator('#start-task').click(); await page.locator('#task-original').filter({ hasText: 'Clicked exact intent' }).waitFor({ state: 'attached' });
    assert.equal(await page.locator('#task-text').inputValue(), 'Newer unsaved typing');
    assert.equal(starts.length, 2); assert.deepEqual(starts[0], starts[1]);
    assert.notEqual(autosaveBody.requestId, starts[0].requestId);
    assert.equal(starts[0].sourceDraftRef.revision, '2'); assert.equal(starts[0].originalText, autosaveBody.originalText);
    const items = await page.evaluate(async () => {
      const token = sessionStorage.getItem('council.accessToken');
      const data = await (await fetch('/api/v1/tasks', { headers: { Authorization: 'Bearer ' + token } })).json(); return data.data.items;
    });
    assert.equal(items.length, 1);
  } finally { if (owner) await owner.stop(); await f.close(); }
});

test('Stop observation retains cancelled output and a candidate remains verification pending', { timeout: 90000 }, async () => {
  const f = await fixture(); let owner;
  try {
    owner = await f.start(); const page = await open(f, owner); await fakeOptions(page, 'slow');
    await page.locator('#task-text').fill('Run slowly for an explicit Stop.'); await page.locator('#start-task').click();
    await page.waitForFunction(() => document.querySelector('#attempt-facts')?.textContent.includes('running'));
    await page.locator('#stop-attempt').click();
    await page.locator('#attempt-status').filter({ hasText: 'cancelled' }).waitFor();
    assert.match(await page.locator('#attempt-facts').textContent(), /verified/);
    await page.locator('#task-text').fill('Create a useful candidate.');
    await page.locator('#task-fake-scenario').selectOption('candidate'); await page.locator('#task-deliverable').selectOption('candidate');
    await page.locator('#start-task').click(); await page.locator('#attempt-status').filter({ hasText: 'Candidate retained' }).waitFor();
    assert.match(await page.locator('#attempt-status').textContent(), /Verification pending/);
    assert.equal(await page.locator('#response-acceptance').isVisible(), false);
    assert.deepEqual(await readFile(path.join(f.project, '.git/index')), f.indexBefore);
  } finally { if (owner) await owner.stop(); await f.close(); }
});
