import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { mkdir, writeFile } from 'node:fs/promises';
import path from 'node:path';
import test from 'node:test';
import { fixture, fixtureRequestId, openWorkspace, ownerAPI, pollOwner } from './helpers.mjs';

async function keyboardActivateAt360(page, button) {
  await page.setViewportSize({ width: 360, height: 800 });
  await button.scrollIntoViewIfNeeded();
  await button.focus(); await page.keyboard.press('Tab'); await page.keyboard.press('Shift+Tab');
  const state = await button.evaluate(node => {
    const style = getComputedStyle(node), rect = node.getBoundingClientRect();
    return { focused: document.activeElement === node, outline: style.outlineStyle !== 'none' && parseFloat(style.outlineWidth) > 0, width: rect.width, viewport: innerWidth, documentWidth: document.documentElement.scrollWidth };
  });
  assert.equal(state.focused, true); assert.equal(state.outline, true);
  assert.ok(state.width <= state.viewport && state.documentWidth <= state.viewport + 1);
  await page.keyboard.press('Enter');
}

test('real project capture is private, scoped, keyboard accessible and inspectable by CLI', { timeout: 120000 }, async t => {
  const f = await fixture(t), owner = await f.start();
  const page = await openWorkspace(f, owner, { fake: false });
  const firstProject = await page.locator('#project-select').inputValue();
  assert.match(firstProject, /^[a-f0-9]{32}$/);

  const otherPath = path.join(f.home, 'Other project'); await mkdir(otherPath);
  const git = args => { const result = spawnSync('git', args, { cwd: otherPath, env: f.env, encoding: 'utf8', shell: false }); assert.equal(result.status, 0, result.stderr); };
  git(['init', '-q']); await writeFile(path.join(otherPath, 'fixture.txt'), 'second fixture\n'); git(['add', 'fixture.txt']);
  git(['-c', 'user.name=Council Test', '-c', 'user.email=council@example.invalid', 'commit', '-qm', 'fixture']);
  const opened = await ownerAPI(owner, '/projects/open', 'POST', { requestId: fixtureRequestId(), selectedPath: otherPath, basePath: otherPath });
  assert.equal(opened.status, 201, JSON.stringify(opened.envelope.error));
  const otherProject = opened.envelope.data.project.id;
  const sentinel = 'HISTORY_EXCLUDED_USER_CONTENT_6b304';
  async function createTask(projectId, text) {
    const response = await ownerAPI(owner, '/tasks', 'POST', { requestId: fixtureRequestId(), projectId, originalText: text, executionMode: 'fake', fakeScenario: 'response', workflow: { mode: 'author_only' }, acceptance: { deliverable: 'response_only' } });
    assert.equal(response.status, 201, JSON.stringify(response.envelope.error));
    return response.envelope.data;
  }
  const first = await createTask(firstProject, sentinel), other = await createTask(otherProject, 'FOREIGN_PROJECT_TASK_7e29');
  for (const attempt of [first.attempt.id, other.attempt.id]) await pollOwner(owner, '/attempts/' + attempt, value => value.phase === 'settled', { timeoutMs: 30000 });

  let admitted = 0;
  page.on('request', request => { if (request.method() === 'POST' && new URL(request.url()).pathname === '/api/v1/tasks') admitted++; });
  await page.locator('#task-text').fill('Keep this newer independent draft.');
  await page.locator('#history-panel > summary').click();
  const response = page.waitForResponse(r => r.request().method() === 'POST' && new URL(r.url()).pathname === '/api/v1/history/snapshots');
  await keyboardActivateAt360(page, page.getByRole('button', { name: 'Capture selected project history', exact: true }));
  const captured = await response, envelope = await captured.json();
  assert.equal(captured.status(), 201, JSON.stringify(envelope.error));
  const snapshot = envelope.data.snapshot;
  assert.equal(snapshot.projectId, firstProject); assert.equal(snapshot.completeness, 'incomplete_history');
  const notice = 'Incomplete observational history. Not a backup, evidence export, restore, or proof of acceptance.';
  assert.equal(snapshot.notice, notice);
  await page.locator('#history-facts').filter({ hasText: snapshot.snapshotId }).waitFor();
  assert.match(await page.locator('#history-facts').textContent(), /Not a backup/);
  assert.match(await page.locator('#history-omissions').textContent(), /task_text/);
  await page.getByRole('button', { name: 'Read tasks', exact: true }).click();
  await page.locator('#history-records').filter({ hasText: first.task.id }).waitFor();
  assert.doesNotMatch(await page.locator('#history-records').textContent(), new RegExp(other.task.id));
  await page.getByRole('button', { name: 'Read attempts', exact: true }).click();
  await page.locator('#history-records').filter({ hasText: first.attempt.id }).waitFor();
  assert.equal(await page.locator('#task-text').inputValue(), 'Keep this newer independent draft.');

  const shown = f.cli(['history', 'show', snapshot.snapshotId]);
  assert.equal(shown.status, 0, JSON.stringify(shown.envelope.error));
  assert.deepEqual(shown.envelope.data.snapshot, snapshot);
  const tasks = f.cli(['history', 'records', snapshot.snapshotId, '--kind', 'tasks']);
  const attempts = f.cli(['history', 'records', snapshot.snapshotId, '--kind', 'attempts']);
  assert.equal(tasks.status, 0, JSON.stringify(tasks.envelope.error)); assert.equal(attempts.status, 0, JSON.stringify(attempts.envelope.error));
  for (const result of [tasks, attempts]) { assert.equal(result.envelope.data.completeness, 'incomplete_history'); assert.equal(result.envelope.data.notice, notice); }
  assert.deepEqual(tasks.envelope.data.items.map(row => row.taskId), [first.task.id]);
  assert.deepEqual(attempts.envelope.data.items.map(row => row.attemptId), [first.attempt.id]);
  const serialized = JSON.stringify([snapshot, tasks.envelope.data, attempts.envelope.data]);
  for (const excluded of [sentinel, other.task.id, other.attempt.id, f.project, otherPath]) assert.equal(serialized.includes(excluded), false, 'history must omit excluded value');

  // Switch projects using the actual workspace. The old metadata must be
  // detached, while the typed snapshot identity remains available to inspect.
  await page.locator('#project-path').fill(otherPath);
  await page.locator('#open-project').click();
  await page.waitForFunction(id => document.querySelector('#project-select')?.value === id, otherProject);
  assert.equal(await page.locator('#history-snapshot').isVisible(), false);
  assert.equal(await page.locator('#history-records').textContent(), '');
  assert.equal(await page.locator('#history-load-tasks').isDisabled(), true);
  assert.equal(await page.locator('#history-snapshot-id').inputValue(), snapshot.snapshotId);
  await page.getByRole('button', { name: 'Inspect snapshot', exact: true }).click();
  await page.locator('#history-status').filter({ hasText: 'belongs to a different project' }).waitFor();
  assert.equal(await page.locator('#history-snapshot-id').inputValue(), snapshot.snapshotId);
  assert.equal(await page.locator('#history-snapshot').isVisible(), false);
  await page.locator('#project-select').selectOption(firstProject);
  await page.getByRole('button', { name: 'Inspect snapshot', exact: true }).click();
  await page.locator('#history-facts').filter({ hasText: snapshot.snapshotId }).waitFor();
  await page.getByRole('button', { name: 'Read tasks', exact: true }).click();
  await page.locator('#history-records').filter({ hasText: first.task.id }).waitFor();
  assert.equal(admitted, 0, 'history inspection must not start a new task or model');
});
