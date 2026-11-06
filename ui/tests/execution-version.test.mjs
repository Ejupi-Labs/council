import test from 'node:test';
import assert from 'node:assert/strict';
import { fixture, legacyExecution } from './helpers.mjs';

test('guidance creates a current version without running or accepting the historical response', { timeout: 90000 }, async () => {
  const f = await fixture(); let owner;
  try {
    owner = await f.start(); const page = await f.browser.newPage(); await page.goto(owner.url);
    await page.locator('#project-path').fill(f.project); await page.locator('#open-project').click();
    await page.waitForFunction(() => !document.querySelector('#start-task')?.disabled);
    await page.locator('#execution-options > summary').click(); await page.locator('#task-fake').check(); await legacyExecution(page); await page.locator('#task-deliverable').selectOption('response_only');
    let starts = 0;
    page.on('request', r => { if (r.method() === 'POST' && /\/tasks(?:\/[a-f0-9]+\/attempts)?$/.test(new URL(r.url()).pathname)) starts++; });
    await page.locator('#task-text').fill('Keep the original intent verbatim.'); await page.locator('#start-task').click();
    await page.locator('#attempt-status').filter({ hasText: 'Verification pending' }).waitFor();
    await page.locator('#read-response').click(); await page.locator('#response-acceptance').waitFor();
    await page.getByText('Add guidance for a new task version', { exact: true }).click();
    await page.locator('#task-guidance').fill('A new complete requirement for the next execution.'); await page.locator('#save-guidance').click();
    await page.locator('#task-detail-heading').filter({ hasText: 'version 2' }).waitFor();
    assert.match(await page.locator('#attempt-status').textContent(), /Historical task version/);
    assert.equal(await page.locator('#response-acceptance').isVisible(), false); assert.equal(starts, 1);
    assert.equal(await page.locator('#task-original').textContent(), 'Keep the original intent verbatim.');
    await page.locator('#run-attempt').click();
    await page.waitForFunction(() => { const text = document.querySelector('#attempt-status')?.textContent ?? ''; return text.includes('Verification pending') && !text.includes('Historical task version'); });
    assert.equal(starts, 2);
    await page.getByText('Execution history and reported usage', { exact: true }).click();
    assert.equal(await page.locator('#attempt-list button').count(), 2);
    assert.match(await page.locator('#usage-total').textContent(), /Execution history is not a token or cost total.*exact per-scope counters, page coverage and unknown charges/);
    await page.locator('#task-routing-panel > summary').click(); await page.locator('#load-task-usage').click();
    await page.locator('#task-usage-summary').filter({ hasText: 'ChargesUnknown' }).waitFor();
    assert.match(await page.locator('#task-usage-summary').textContent(), /fake unmeasured/);
    assert.equal(await page.locator('#task-usage-list > li').count(), 2);
    assert.match(await page.locator('#task-usage-list').textContent(), /fake_unmeasured/);
  } finally { if (owner) await owner.stop(); await f.close(); }
});
