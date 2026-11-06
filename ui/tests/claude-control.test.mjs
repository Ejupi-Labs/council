import test from 'node:test';
import assert from 'node:assert/strict';
import { fixture } from './helpers.mjs';

async function open(f, owner) {
  const page = await f.browser.newPage(); await page.goto(owner.url);
  await page.locator('#project-path').fill(f.project); await page.locator('#open-project').click();
  await page.waitForFunction(() => !document.querySelector('#start-task')?.disabled);
  await page.locator('.execution-options summary').click(); await page.locator('#task-fake').check(); await page.locator('#task-deliverable').selectOption('response_only');
  return page;
}

test('lost Claude start reply freezes target and separate autosave identity while preserving newer text and selection', { timeout: 90000 }, async () => {
  const f = await fixture(); let owner;
  try {
    owner = await f.start(); const page = await open(f, owner);
    await page.locator('#task-text').fill('Saved original'); await page.locator('#save-draft').click(); await page.locator('#draft-status').filter({ hasText: 'Saved revision 1' }).waitFor();
    await page.locator('#task-text').fill('Clicked Claude intent'); await page.locator('#task-harness').selectOption('claude-code');
    let arrived; let release; const reached = new Promise(resolve => { arrived = resolve; }); const gate = new Promise(resolve => { release = resolve; }); let saved;
    await page.route('**/api/v1/drafts/*', async route => {
      if (route.request().method() !== 'PUT') return route.continue(); saved = route.request().postDataJSON();
      const response = await route.fetch(); arrived(); await gate; await route.fulfill({ response });
    }, { times: 1 });
    const starts = [];
    await page.route('**/api/v1/tasks', async route => {
      if (route.request().method() !== 'POST') return route.continue();
      starts.push(route.request().postDataJSON()); await route.fetch(); await route.abort('connectionfailed');
    }, { times: 1 });
    await page.locator('#start-task').click(); await reached;
    await page.locator('#task-text').fill('Newer unsaved text'); await page.locator('#task-harness').selectOption('codex'); release();
    await page.locator('#start-task').filter({ hasText: 'Retry pending start' }).waitFor();
    await page.reload(); await page.waitForFunction(() => !document.querySelector('#start-task')?.disabled);
    assert.equal(await page.locator('#task-harness').inputValue(), 'codex'); assert.equal(await page.locator('#task-text').inputValue(), 'Newer unsaved text');
    await page.route('**/api/v1/tasks', async route => { if (route.request().method() === 'POST') starts.push(route.request().postDataJSON()); await route.continue(); });
    await page.locator('#start-task').click(); await page.locator('#task-original').filter({ hasText: 'Clicked Claude intent' }).waitFor({ state: 'attached' });
    assert.equal(starts.length, 2); assert.deepEqual(starts[0], starts[1]); assert.equal(starts[0].pins.harnessId, 'claude-code');
    assert.equal(starts[0].sourceDraftRef.revision, '2'); assert.notEqual(starts[0].requestId, saved.requestId);
    assert.equal(await page.locator('#task-text').inputValue(), 'Newer unsaved text'); assert.equal(await page.locator('#task-harness').inputValue(), 'codex');
    assert.match(await page.locator('#attempt-summary').textContent(), /claude-native/);
    assert.equal(await page.locator('#task-list button').count(), 1);
  } finally { if (owner) await owner.stop(); await f.close(); }
});

test('explicit guidance retargets a queued version without rerouting its historical attempt or starting work', { timeout: 90000 }, async () => {
  const f = await fixture(); let owner;
  try {
    owner = await f.start(); const page = await open(f, owner);
    await page.locator('#task-fake-scenario').selectOption('slow'); await page.locator('#task-text').fill('Occupy the shared managed slot with a Codex fixture.');
    const firstReply = page.waitForResponse(response => response.request().method() === 'POST' && new URL(response.url()).pathname === '/api/v1/tasks');
    await page.locator('#start-task').click(); const first = (await (await firstReply).json()).data;
    await page.locator('#attempt-status').filter({ hasText: 'running' }).waitFor();
    await page.locator('#task-fake-scenario').selectOption('response'); await page.locator('#task-harness').selectOption('claude-code'); await page.locator('#task-text').fill('Retain the queued Claude version until explicit new guidance.');
    const secondReply = page.waitForResponse(response => response.request().method() === 'POST' && new URL(response.url()).pathname === '/api/v1/tasks');
    await page.locator('#start-task').click(); const second = (await (await secondReply).json()).data;
    await page.locator('#attempt-status').filter({ hasText: 'queued' }).waitFor();
    await page.getByText('Add guidance for a new task version', { exact: true }).click();
    await page.locator('#task-guidance').fill('Use Codex on the next explicit execution.'); await page.locator('#guidance-harness').selectOption('codex'); await page.locator('#save-guidance').click();
    await page.locator('#task-detail-heading').filter({ hasText: 'version 2' }).waitFor();
    assert.match(await page.locator('#task-facts').textContent(), /Captured harness choiceCodex/);
    assert.match(await page.locator('#attempt-summary').textContent(), /claude-native/);
    await page.locator('#task-list button').filter({ hasText: first.task.id.slice(0, 8) }).click(); await page.locator('#stop-attempt').click();
    await page.locator('#attempt-status').filter({ hasText: 'cancelled' }).waitFor();
    await page.locator('#task-list button').filter({ hasText: second.task.id.slice(0, 8) }).click();
    await page.locator('#attempt-status').filter({ hasText: 'blocked' }).waitFor();
    assert.match(await page.locator('#attempt-status').textContent(), /Historical task version/);
    const runReply = page.waitForResponse(response => response.request().method() === 'POST' && new URL(response.url()).pathname === '/api/v1/tasks/' + second.task.id + '/attempts');
    await page.locator('#run-attempt').click(); const next = (await (await runReply).json()).data;
    assert.equal(next.attempt.taskVersion, '2'); assert.equal(next.attempt.connectionId, 'codex-native');
    await page.locator('#attempt-status').filter({ hasText: 'Verification pending' }).waitFor();
    assert.match(await page.locator('#attempt-summary').textContent(), /codex-native/);
  } finally { if (owner) await owner.stop(); await f.close(); }
});

test('Claude Stop keeps the exact captured attempt while newer composer choices remain local', { timeout: 90000 }, async () => {
  const f = await fixture(); let owner;
  try {
    owner = await f.start(); const page = await open(f, owner);
    await page.locator('#task-harness').selectOption('claude-code'); await page.locator('#task-fake-scenario').selectOption('slow'); await page.locator('#task-text').fill('Run the Claude fixture slowly for explicit Stop.');
    const admitted = page.waitForResponse(response => response.request().method() === 'POST' && new URL(response.url()).pathname === '/api/v1/tasks');
    await page.locator('#start-task').click(); const attempt = (await (await admitted).json()).data.attempt;
    await page.locator('#attempt-status').filter({ hasText: 'running' }).waitFor();
    await page.locator('#task-harness').selectOption('codex'); await page.locator('#task-text').fill('New local task text');
    const stopped = page.waitForRequest(request => request.method() === 'POST' && new URL(request.url()).pathname.endsWith('/stop'));
    await page.locator('#stop-attempt').click(); const request = await stopped;
    assert.equal(new URL(request.url()).pathname, '/api/v1/attempts/' + attempt.id + '/stop');
    assert.deepEqual(Object.keys(request.postDataJSON()), ['requestId']);
    await page.locator('#attempt-status').filter({ hasText: 'cancelled' }).waitFor();
    assert.match(await page.locator('#attempt-summary').textContent(), /claude-native/);
    assert.equal(await page.locator('#task-text').inputValue(), 'New local task text'); assert.equal(await page.locator('#task-harness').inputValue(), 'codex');
  } finally { if (owner) await owner.stop(); await f.close(); }
});

test('late Claude admission and task-list refresh preserve a newer retained-task selection', { timeout: 90000 }, async () => {
  const f = await fixture(); let owner;
  try {
    owner = await f.start(); const page = await open(f, owner);
    await page.locator('#task-harness').selectOption('claude-code');
    const start = async text => {
      await page.locator('#task-text').fill(text);
      const returned = page.waitForResponse(response => response.request().method() === 'POST' && new URL(response.url()).pathname === '/api/v1/tasks');
      await page.locator('#start-task').click(); const admission = (await (await returned).json()).data;
      await page.locator('#task-original').filter({ hasText: text }).waitFor({ state: 'attached' });
      await page.locator('#attempt-status').filter({ hasText: 'Verification pending' }).waitFor();
      await page.waitForFunction(() => !document.querySelector('#start-task')?.disabled);
      return admission;
    };
    const first = await start('First retained Claude response.');
    const second = await start('Second retained Claude response.');

    let releaseAdmission; let admissionArrived;
    const admissionGate = new Promise(resolve => { releaseAdmission = resolve; });
    const admissionPending = new Promise(resolve => { admissionArrived = resolve; });
    await page.route('**/api/v1/tasks', async route => {
      if (route.request().method() !== 'POST') return route.continue();
      const response = await route.fetch(); admissionArrived(); await admissionGate; await route.fulfill({ response });
    }, { times: 1 });
    await page.locator('#task-text').fill('A third explicitly admitted Claude task.'); await page.locator('#start-task').click(); await admissionPending;
    await page.locator('#task-text').fill('Newer composer text during admission'); await page.locator('#task-harness').selectOption('codex');
    await page.locator('#task-list button').filter({ hasText: first.task.id.slice(0, 8) }).click();
    await page.locator('#task-original').filter({ hasText: 'First retained Claude response.' }).waitFor({ state: 'attached' });
    releaseAdmission(); await page.waitForFunction(() => !document.querySelector('#start-task')?.disabled);
    assert.match(await page.locator('#task-detail-heading').textContent(), new RegExp(first.task.id.slice(0, 8)));
    assert.equal(await page.locator('#task-original').textContent(), 'First retained Claude response.');
    assert.equal(await page.locator('#task-text').inputValue(), 'Newer composer text during admission');
    assert.equal(await page.locator('#task-harness').inputValue(), 'codex');

    let releaseRefresh; let refreshArrived;
    const refreshGate = new Promise(resolve => { releaseRefresh = resolve; });
    const refreshPending = new Promise(resolve => { refreshArrived = resolve; });
    await page.route('**/api/v1/tasks?*', async route => {
      const response = await route.fetch(); refreshArrived(); await refreshGate; await route.fulfill({ response });
    }, { times: 1 });
    await page.locator('#task-harness').selectOption('claude-code'); await page.locator('#task-text').fill('A fourth explicit Claude task.');
    await page.locator('#start-task').click(); await refreshPending;
    await page.locator('#task-list button').filter({ hasText: second.task.id.slice(0, 8) }).click();
    await page.locator('#task-original').filter({ hasText: 'Second retained Claude response.' }).waitFor({ state: 'attached' });
    await page.locator('#task-text').fill('Newer composer text during refresh'); releaseRefresh();
    await page.waitForFunction(() => !document.querySelector('#start-task')?.disabled);
    assert.match(await page.locator('#task-detail-heading').textContent(), new RegExp(second.task.id.slice(0, 8)));
    assert.equal(await page.locator('#task-original').textContent(), 'Second retained Claude response.');
    assert.equal(await page.locator('#task-text').inputValue(), 'Newer composer text during refresh');
  } finally { if (owner) await owner.stop(); await f.close(); }
});
