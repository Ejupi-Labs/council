import test from 'node:test';
import assert from 'node:assert/strict';
import { fixture } from './helpers.mjs';

test('task-list phases follow running and Stop without replacing selected work or newer typing', { timeout: 90000 }, async () => {
  const f = await fixture(); let owner;
  try {
    owner = await f.start(); const page = await f.browser.newPage(); await page.goto(owner.url);
    await page.locator('#project-path').fill(f.project); await page.locator('#open-project').click();
    await page.waitForFunction(() => !document.querySelector('#start-task')?.disabled);
    await page.locator('.execution-options summary').click(); await page.locator('#task-fake').check();
    await page.locator('#task-fake-scenario').selectOption('slow');
    const original = 'Observe this exact slow fake task and stop it explicitly.';
    await page.locator('#task-text').fill(original); await page.locator('#start-task').click();
    await page.locator('#attempt-status').filter({ hasText: 'running' }).waitFor();
    await page.locator('#task-list button').filter({ hasText: 'running · running' }).waitFor();
    const selectedHeading = await page.locator('#task-detail-heading').textContent();
    const newerDraft = 'Keep this newer unsaved draft while lifecycle observations arrive.';
    const newerGuidance = 'Keep this unsaved guidance for a later version.';
    await page.locator('#task-text').fill(newerDraft);
    await page.getByText('Add guidance for a new task version', { exact: true }).click();
    await page.locator('#task-guidance').fill(newerGuidance);

    await page.locator('#stop-attempt').click();
    await page.locator('#attempt-status').filter({ hasText: 'cancelling' }).waitFor();
    await page.locator('#task-list button').filter({ hasText: 'cancelling · cancelling' }).waitFor();
    assert.equal(await page.locator('#task-detail-heading').textContent(), selectedHeading);
    assert.equal(await page.locator('#task-original').textContent(), original);
    assert.equal(await page.locator('#task-text').inputValue(), newerDraft);
    assert.equal(await page.locator('#task-guidance').inputValue(), newerGuidance);

    await page.locator('#attempt-status').filter({ hasText: 'cancelled' }).waitFor();
    await page.locator('#task-list button').filter({ hasText: 'cancelled · settled' }).waitFor();
    assert.equal(await page.locator('#task-detail-heading').textContent(), selectedHeading);
    assert.equal(await page.locator('#task-text').inputValue(), newerDraft);
    assert.equal(await page.locator('#task-guidance').inputValue(), newerGuidance);
  } finally { if (owner) await owner.stop(); await f.close(); }
});
