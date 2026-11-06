import test from 'node:test';
import assert from 'node:assert/strict';
import { fixture, legacyExecution } from './helpers.mjs';

test('replaying a Start autosave preserves a newer acknowledged draft revision', { timeout: 90000 }, async () => {
  const f = await fixture(); let owner;
  try {
    owner = await f.start(); const page = await f.browser.newPage(); await page.goto(owner.url);
    await page.locator('#project-path').fill(f.project); await page.locator('#open-project').click();
    await page.waitForFunction(() => !document.querySelector('#start-task')?.disabled);
    await page.locator('#execution-options > summary').click(); await page.locator('#task-fake').check(); await legacyExecution(page);
    await page.locator('#task-deliverable').selectOption('response_only');
    await page.locator('#task-text').fill('The saved original.'); await page.locator('#save-draft').click();
    await page.locator('#draft-status').filter({ hasText: 'Saved revision 1' }).waitFor();

    const clicked = 'The exact clicked execution intent.';
    await page.locator('#task-text').fill(clicked);
    const draftWrites = []; const starts = [];
    page.on('request', request => {
      const pathname = new URL(request.url()).pathname;
      if (request.method() === 'PUT' && /\/api\/v1\/drafts\//.test(pathname)) draftWrites.push(request.postDataJSON());
      if (request.method() === 'POST' && pathname === '/api/v1/tasks') starts.push(request.postDataJSON());
    });
    await page.route('**/api/v1/drafts/*', async route => {
      if (route.request().method() !== 'PUT') return route.continue();
      await route.fetch(); await route.abort('connectionfailed');
    }, { times: 1 });
    await page.locator('#start-task').click();
    await page.locator('#start-task').filter({ hasText: 'Retry pending start' }).waitFor();
    assert.equal(starts.length, 0);

    // The draft editor can reconcile its shared save independently of the pending Start.
    await page.locator('#save-draft').click();
    await page.locator('#draft-status').filter({ hasText: 'Saved revision 2' }).waitFor();
    const newerSaved = 'A newer independently acknowledged draft.';
    await page.locator('#task-text').fill(newerSaved); await page.locator('#save-draft').click();
    await page.locator('#draft-status').filter({ hasText: 'Saved revision 3' }).waitFor();

    await page.locator('#start-task').click();
    await page.locator('#task-original').filter({ hasText: clicked }).waitFor({ state: 'attached' });
    assert.equal(starts.length, 1); assert.equal(starts[0].originalText, clicked);
    assert.equal(starts[0].sourceDraftRef.revision, '2');
    assert.deepEqual(draftWrites[0], draftWrites[1]);
    assert.deepEqual(draftWrites[0], draftWrites[3]);
    assert.notEqual(starts[0].requestId, draftWrites[0].requestId);
    assert.equal(await page.locator('#task-text').inputValue(), newerSaved);
    assert.match(await page.locator('#draft-meta').textContent(), /Saved revision3/);

    const latest = 'The next local edit must save without a false conflict.';
    await page.locator('#task-text').fill(latest);
    const nextSave = page.waitForResponse(response => response.request().method() === 'PUT'
      && /\/api\/v1\/drafts\//.test(new URL(response.url()).pathname));
    await page.locator('#save-draft').click(); const response = await nextSave;
    assert.equal(response.request().postDataJSON().expectedRevision, '3');
    const envelope = await response.json(); assert.equal(envelope.error, undefined);
    assert.equal(envelope.data.revision, '4'); assert.equal(envelope.data.originalText, latest);
    await page.locator('#draft-status').filter({ hasText: 'Saved revision 4' }).waitFor();
  } finally { if (owner) await owner.stop(); await f.close(); }
});

test('accepted responses and new guidance refresh the retained task list immediately', { timeout: 90000 }, async () => {
  const f = await fixture(); let owner;
  try {
    owner = await f.start(); const page = await f.browser.newPage(); await page.goto(owner.url);
    await page.locator('#project-path').fill(f.project); await page.locator('#open-project').click();
    await page.waitForFunction(() => !document.querySelector('#start-task')?.disabled);
    await page.locator('#execution-options > summary').click(); await page.locator('#task-fake').check(); await legacyExecution(page);
    await page.locator('#task-deliverable').selectOption('response_only');
    let starts = 0;
    page.on('request', request => {
      if (request.method() === 'POST' && /\/api\/v1\/tasks(?:\/[a-f0-9]+\/attempts)?$/.test(new URL(request.url()).pathname)) starts++;
    });
    await page.locator('#task-text').fill('Return an exact response for explicit user review.');
    await page.locator('#start-task').click();
    await page.locator('#attempt-status').filter({ hasText: 'Verification pending' }).waitFor();
    await page.locator('#read-response').click(); await page.locator('#response-acceptance').waitFor();
    await page.locator('#reviewed-response').check(); await page.locator('#accept-response').click();
    await page.locator('#task-facts').filter({ hasText: 'Reviewed response accepted' }).waitFor();
    await page.locator('#task-list button').filter({ hasText: 'completed' }).waitFor();
    assert.doesNotMatch(await page.locator('#task-list').textContent(), /response acceptance pending/);

    await page.getByText('Add guidance for a new task version', { exact: true }).click();
    await page.locator('#task-guidance').fill('Use this new complete guidance on the next explicit execution.');
    const refreshedList = page.waitForResponse(async response => {
      if (response.request().method() !== 'GET' || new URL(response.url()).pathname !== '/api/v1/tasks') return false;
      const envelope = await response.json();
      return envelope.data?.items?.some(task => task.currentVersion === '2') ?? false;
    });
    await page.locator('#save-guidance').click();
    const envelope = await (await refreshedList).json();
    assert.equal(envelope.data.items[0].currentVersion, '2');
    await page.locator('#task-detail-heading').filter({ hasText: 'version 2' }).waitFor();
    await page.locator('#task-list button').filter({ hasText: 'awaiting execution' }).waitFor();
    assert.match(await page.locator('#task-list').textContent(), /Start an explicit execution against the current task version/);
    assert.doesNotMatch(await page.locator('#task-list').textContent(), /completed|response acceptance pending/);
    assert.equal(starts, 1);
  } finally { if (owner) await owner.stop(); await f.close(); }
});
