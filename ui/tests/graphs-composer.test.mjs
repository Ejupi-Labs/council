import assert from 'node:assert/strict';
import test from 'node:test';
import path from 'node:path';
import { fixture, openWorkspace } from './helpers.mjs';

test('the optional task-group editor preserves standalone text and stable node/dependency choices', { timeout: 60000 }, async t => {
  const f = await fixture(t); const owner = await f.start();
  const page = await openWorkspace(f, owner, { fake: false, workflow: 'auto' });
  const original = 'Standalone original 東京\nKeep this draft and its choices.';
  await page.locator('#task-text').fill(original);
  assert.equal(await page.locator('#open-graph-composer').count(), 1, 'the optional group entry must exist');
  await page.locator('#open-graph-composer').click();
  const nodes = page.locator('#graph-editor-nodes > [data-graph-node]');
  assert.equal(await nodes.count(), 2);
  const firstKey = await nodes.nth(0).getAttribute('data-graph-node');
  const secondKey = await nodes.nth(1).getAttribute('data-graph-node');
  await nodes.nth(0).locator('[data-graph-field="originalText"]').fill('First complete instruction🙂');
  await nodes.nth(1).locator('[data-graph-field="originalText"]').fill('Second complete instruction 東京');
  await nodes.nth(1).locator('summary').filter({ hasText: 'Depends on' }).click();
  await nodes.nth(1).locator('[data-graph-dependency-from]').selectOption(firstKey);
  await nodes.nth(1).locator('[data-graph-dependency-kind]').selectOption('context');
  await nodes.nth(1).getByRole('button', { name: 'Add dependency' }).click();
  await nodes.nth(0).getByRole('button', { name: 'Remove task' }).click();
  assert.match(await page.locator('#graph-draft-issues').textContent(), /missing|removed/i, 'removal must retain an invalid edge for explicit correction');
  assert.equal(await page.locator('#graph-editor-nodes > [data-graph-node]').first().getAttribute('data-graph-node'), secondKey);
  await page.locator('#graph-add-node').click();
  const addedKey = await page.locator('#graph-editor-nodes > [data-graph-node]').last().getAttribute('data-graph-node');
  assert.notEqual(addedKey, firstKey, 'a removed stable key must never be silently reused');
  assert.equal(await page.locator('#task-text').inputValue(), original);
  assert.match(await page.locator('#graph-scope-preview').textContent(), /Separate reviewable results/i);
  assert.match(await page.locator('#graph-scope-preview').textContent(), /no local application|without local application/i);
  await page.locator('#close-graph-composer').click();
  assert.equal(await page.locator('#task-text').inputValue(), original);
  await page.locator('#open-graph-composer').click();
  assert.equal(await page.locator('#graph-editor-nodes > [data-graph-node]').first().locator('[data-graph-field="originalText"]').inputValue(), 'Second complete instruction 東京');
});

test('task-group drafts stay separate across projects when browser caching is denied', { timeout: 30000 }, async t => {
  const f = await fixture(t, { files: { 'nested/note.txt': 'Existing selected subfolder\r\n' } });
  const owner = await f.start(); const page = await f.browser.newPage();
  await page.addInitScript(() => {
    const original = Storage.prototype.setItem;
    Storage.prototype.setItem = function(key, value) {
      if (key.startsWith('council.foundation.')) throw new DOMException('Controlled denied draft cache', 'QuotaExceededError');
      return original.call(this, key, value);
    };
  });
  await page.goto(owner.url);
  await page.locator('#project-path').fill(f.project); await page.locator('#open-project').click();
  await page.waitForFunction(() => !document.querySelector('#open-graph-composer')?.disabled, undefined, { timeout: 5000 });
  const first = await page.locator('#project-select').inputValue();
  await page.locator('#open-graph-composer').click();
  const original = 'Unsaved graph original 東京\r\nPreserve project A🙂';
  await page.locator('#graph-editor-nodes textarea[data-graph-field="originalText"]').first().fill(original);
  const exactOriginal = await page.locator('#graph-editor-nodes textarea[data-graph-field="originalText"]').first().inputValue();
  await page.locator('#project-path').fill(path.join(f.project, 'nested')); await page.locator('#open-project').click();
  await page.waitForFunction(previous => document.querySelector('#project-select')?.value !== previous, first, { timeout: 5000 });
  assert.equal(await page.locator('#graph-editor-nodes textarea[data-graph-field="originalText"]').first().inputValue(), '');
  await page.locator('#graph-editor-nodes textarea[data-graph-field="originalText"]').first().fill('Different project B instruction');
  await page.locator('#project-select').selectOption(first);
  assert.equal(await page.locator('#graph-editor-nodes textarea[data-graph-field="originalText"]').first().inputValue(), exactOriginal,
    'denied best-effort session cache must not discard this page\'s project-scoped draft');
});
