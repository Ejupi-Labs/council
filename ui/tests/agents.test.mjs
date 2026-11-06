import test from 'node:test';
import assert from 'node:assert/strict';
import { fixture } from './helpers.mjs';

test('discovery remains truthful and refresh failure preserves composition and keyboard access', { timeout: 60000 }, async () => {
  const f = await fixture(); let owner;
  try {
    f.cli(['projects', 'open', f.project]);
    owner = await f.start(); const page = await f.browser.newPage({ reducedMotion: 'reduce' });
    await page.goto(owner.url); await page.waitForFunction(() => document.querySelector('#project-select')?.value);
    await page.waitForFunction(() => document.querySelectorAll('#agents-list > *').length === 5);
    const project = await page.locator('#project-select').inputValue();
    assert.match(await page.locator('#agents-list').innerText(), /Unknown/);
    assert.equal(await page.locator('#agents-list').getByText('Managed execution: not implemented', { exact: true }).count(), 3);
    assert.match(await page.getByTestId('agent-codex').textContent(), /native Codex supported.*live provider unqualified/);
    await page.locator('#task-text').fill('Keep this original draft');
    await page.route('**/api/v1/agents', route => route.fulfill({ status: 503, contentType: 'application/json', body: JSON.stringify({ apiVersion: 1, requestId: '11111111111111111111111111111111', error: { code: 'discovery_unavailable', message: 'Installation inspection is unavailable. Retry later.' } }) }));
    await page.locator('#refresh-agents').click();
    await page.waitForFunction(() => /unavailable/i.test(document.querySelector('#agents-status')?.textContent ?? ''));
    assert.equal(await page.locator('#task-text').inputValue(), 'Keep this original draft');
    assert.equal(await page.locator('#project-select').inputValue(), project);
    await page.goto('about:blank'); await page.goto(owner.url); await page.waitForFunction(() => document.querySelector('#project-select')?.value); await page.keyboard.press('Tab');
    const first = await page.evaluate(() => ({ tag: document.activeElement?.tagName, text: document.activeElement?.textContent }));
    assert.equal(first.tag, 'A'); assert.match(first.text, /Skip to task/);
    await page.keyboard.press('Enter');
    assert.equal(await page.evaluate(() => document.activeElement?.id), 'task-text');
    await page.locator('#task-text').fill('Keyboard prepared draft');
    await page.locator('#save-draft').focus(); await page.keyboard.press('Enter');
    await page.waitForFunction(() => /Saved revision/i.test(document.querySelector('#draft-status')?.textContent ?? ''));
  } finally { if (owner) await owner.stop(); await f.close(); }
});
