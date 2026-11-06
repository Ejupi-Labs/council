import test from 'node:test';
import assert from 'node:assert/strict';
import { fixture } from './helpers.mjs';

async function open(f, owner) {
  const page = await f.browser.newPage(); await page.goto(owner.url);
  await page.locator('#project-path').fill(f.project); await page.locator('#open-project').click();
  await page.waitForFunction(() => !document.querySelector('#start-task')?.disabled);
  return page;
}

test('optional Claude Agents action starts one captured fake without a connection wizard or promoted provider facts', { timeout: 90000 }, async () => {
  const f = await fixture(); let owner;
  try {
    owner = await f.start(); const page = await open(f, owner);
    await page.getByTestId('agent-claude-code').getByRole('button', { name: 'Use Claude Code for next task' }).click();
    assert.equal(await page.locator('#task-harness').inputValue(), 'claude-code');
    await page.locator('#task-fake').check(); await page.locator('#task-deliverable').selectOption('response_only');
    await page.locator('#task-model').fill('haiku');
    const original = 'Use the explicitly selected Claude fixture. Grüße 🙂\nPreserve the complete original intent.';
    await page.locator('#task-text').fill(original);
    const requests = []; page.on('request', request => { if (request.method() === 'POST' && new URL(request.url()).pathname === '/api/v1/tasks') requests.push(request.postDataJSON()); });
    const admitted = page.waitForResponse(response => response.request().method() === 'POST' && new URL(response.url()).pathname === '/api/v1/tasks');
    await page.locator('#start-task').click(); const admission = (await (await admitted).json()).data;
    await page.locator('#attempt-status').filter({ hasText: 'Verification pending' }).waitFor();
    assert.equal(requests.length, 1); assert.deepEqual(requests[0].pins, { harnessId: 'claude-code', model: 'haiku' });
    assert.equal(admission.attempt.connectionId, 'claude-native');
    assert.equal(await page.locator('#task-original').textContent(), original);
    assert.match(await page.locator('#attempt-summary').textContent(), /claude-native.*Local fake/);
    assert.match(await page.locator('#attempt-status').textContent(), /LOCAL FAKE/);
    assert.match(await page.locator('#attempt-facts').textContent(), /Native conversation continuityUnsupported.*Council tasks and history do not resume/);
    assert.match(await page.locator('#attempt-facts').textContent(), /Declared version assumptionMinimum 2\.1\.259.*2\.1\.318/);
    assert.match(await page.locator('#attempt-facts').textContent(), /Live provider qualificationUnqualified/);
    await page.locator('#read-response').click(); await page.locator('#artifact-status').filter({ hasText: 'stream closed' }).waitFor();
    assert.ok((await page.locator('#artifact-output').textContent()).length > 0);
    const current = await page.evaluate(async id => {
      const token = sessionStorage.getItem('council.accessToken');
      return (await (await fetch('/api/v1/attempts/' + id, { headers: { Authorization: 'Bearer ' + token } })).json()).data;
    }, admission.attempt.id);
    for (const key of ['provider', 'account', 'model', 'nativeSession', 'configuredModel', 'nativeVersion']) {
      if (current.targetObservations[key]) assert.equal(current.targetObservations[key].status, 'unknown', key);
    }
    assert.equal(current.targetObservations.nativeModels, undefined);
    assert.equal(current.targetObservations.usage.inputTokens, null);
    assert.equal(current.targetObservations.usage.outputTokens, null);
    assert.equal(requests.length, 1);
  } finally { if (owner) await owner.stop(); await f.close(); }
});

test('omitted economical Auto resolves unknown work conservatively without inserting a harness pin', { timeout: 90000 }, async () => {
  const f = await fixture(); let owner;
  try {
    owner = await f.start(); const page = await open(f, owner);
    assert.equal(await page.locator('#task-harness').inputValue(), '');
    assert.match(await page.locator('#composer-target').textContent(), /Auto.*economical routing/);
    await page.locator('#execution-options > summary').click(); await page.locator('#task-fake').check();
    await page.locator('#task-deliverable').selectOption('response_only');
    await page.locator('#task-text').fill('Keep the existing Auto meaning for this saved draft.');
    await page.locator('#save-draft').click(); await page.locator('#draft-status').filter({ hasText: 'Saved revision 1' }).waitFor();
    const returned = page.waitForResponse(response => response.request().method() === 'POST' && new URL(response.url()).pathname === '/api/v1/tasks');
    await page.locator('#start-task').click(); const response = await returned; const body = response.request().postDataJSON(); const envelope = await response.json();
    assert.equal(body.pins, undefined); assert.equal(body.sourceDraftRef.revision, '1');
    assert.equal(envelope.data.attempt.connectionId, 'codex-native');
    assert.equal(envelope.data.version.pins.harnessId, undefined);
  } finally { if (owner) await owner.stop(); await f.close(); }
});

test('a supervised Claude request blocks without another target or automatic fallback', { timeout: 90000 }, async () => {
  const f = await fixture(); let owner;
  try {
    owner = await f.start(); const page = await open(f, owner);
    await page.locator('#policy-mode').selectOption('supervised'); await page.locator('#save-policy').click();
    await page.locator('#policy-status').filter({ hasText: 'Saved requested preference: Supervised' }).waitFor();
    await page.locator('#execution-options > summary').click(); await page.locator('#task-harness').selectOption('claude-code'); await page.locator('#task-fake').check();
    await page.locator('#task-text').fill('Keep this unsupported explicit request and report the limitation.');
    let starts = 0; page.on('request', request => { if (request.method() === 'POST' && new URL(request.url()).pathname === '/api/v1/tasks') starts++; });
    const returned = page.waitForResponse(response => response.request().method() === 'POST' && new URL(response.url()).pathname === '/api/v1/tasks');
    await page.locator('#start-task').click(); const response = await returned; const envelope = await response.json();
    assert.equal(envelope.error.code, 'permission_unavailable'); assert.equal(envelope.receipt, undefined);
    assert.match(await page.locator('#execution-status').textContent(), /supervised|approvals/i); assert.equal(starts, 1);
    assert.equal(response.request().postDataJSON().pins.harnessId, 'claude-code');
    assert.equal(await page.locator('#task-list button').count(), 0);
  } finally { if (owner) await owner.stop(); await f.close(); }
});
