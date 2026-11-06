import test from 'node:test';
import assert from 'node:assert/strict';
import { fixture, legacyExecution } from './helpers.mjs';

test('partial fake failure retains useful evidence and unknown cleanup exposes an incident with admission closed', { timeout: 90000 }, async () => {
  const f = await fixture(); let owner;
  try {
    owner = await f.start(); const page = await f.browser.newPage(); await page.goto(owner.url);
    await page.locator('#project-path').fill(f.project); await page.locator('#open-project').click();
    await page.waitForFunction(() => !document.querySelector('#start-task')?.disabled);
    await page.locator('#execution-options > summary').click(); await page.locator('#task-fake').check(); await legacyExecution(page); await page.locator('#task-fake-scenario').selectOption('partial_failure');
    await page.locator('#task-text').fill('Retain the partial failure evidence.'); await page.locator('#start-task').click();
    await page.waitForFunction(() => document.querySelector('#attempt-status')?.textContent.includes('LOCAL FAKE · failed'));
    assert.match(await page.locator('#attempt-status').textContent(), /Candidate retained/);
    assert.match(await page.locator('#attempt-facts').textContent(), /Retained managed workspace/);
    assert.equal(await page.locator('#response-acceptance').isVisible(), false);
    await page.locator('#read-response').click(); await page.locator('#artifact-status').filter({ hasText: 'stream closed' }).waitFor();
    assert.ok((await page.locator('#artifact-output').textContent()).length > 0);
    await page.locator('#task-fake-scenario').selectOption('cleanup_unknown'); await page.locator('#task-text').fill('Expose the explicitly simulated cleanup uncertainty.'); await page.locator('#start-task').click();
    await page.locator('#execution-status').filter({ hasText: 'Dispatch is fenced' }).waitFor();
    assert.equal(await page.locator('#start-task').isEnabled(), false);
    assert.match(await page.locator('#attempt-facts').textContent(), /Retained incident/);
    assert.match(await page.locator('#execution-status').textContent(), /Preserve the workspace and evidence/);
  } finally { if (owner) await owner.stop(); await f.close(); }
});
