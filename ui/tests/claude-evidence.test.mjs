import test from 'node:test';
import assert from 'node:assert/strict';
import { fixture, legacyExecution } from './helpers.mjs';

test('controlled UI test data keeps configured, primary, whole-tree and top-level facts separate with exact estimate lexemes', { timeout: 90000 }, async () => {
  const f = await fixture(); let owner;
  try {
    owner = await f.start(); const page = await f.browser.newPage(); await page.goto(owner.url);
    await page.locator('#project-path').fill(f.project); await page.locator('#open-project').click();
    await page.waitForFunction(() => !document.querySelector('#start-task')?.disabled);
    await page.locator('#execution-options > summary').click(); await page.locator('#task-harness').selectOption('claude-code');
    await page.locator('#task-fake').check(); await legacyExecution(page); await page.locator('#task-deliverable').selectOption('response_only');
    await page.locator('#task-text').fill('CONTROLLED UI OBSERVATION TEST DATA. No live provider was called.');
    let starts = 0; page.on('request', request => { if (request.method() === 'POST' && new URL(request.url()).pathname === '/api/v1/tasks') starts++; });
    const admitted = page.waitForResponse(response => response.request().method() === 'POST' && new URL(response.url()).pathname === '/api/v1/tasks');
    await page.locator('#start-task').click(); const admission = (await (await admitted).json()).data;
    await page.locator('#attempt-status').filter({ hasText: 'Verification pending' }).waitFor();
    assert.match(await page.locator('#attempt-facts').textContent(), /Observed actual primary modelUnknown/);
    const stdoutId = await page.evaluate(async id => {
      const token = sessionStorage.getItem('council.accessToken');
      const envelope = await (await fetch('/api/v1/attempts/' + id + '/artifacts', { headers: { Authorization: 'Bearer ' + token } })).json();
      return envelope.data.items.find(artifact => artifact.kind === 'stdout').id;
    }, admission.attempt.id);

    // Authored HTTP shapes exercise rendering only. They are visibly labelled test data,
    // never persisted as actual observations and never used to qualify a live provider.
    const observedAt = '2026-11-06T12:00:00.000Z';
    const observed = (value, field) => ({ status: 'observed', value, source: 'CONTROLLED UI TEST DATA: ' + field, observedAt });
    const unknown = { status: 'unknown', value: null, source: null, observedAt: null };
    const estimate = '1.234567890123456789e-17';
    const decorate = attempt => ({ ...attempt, executionMode: 'native', targetObservations: {
      ...attempt.targetObservations, provider: observed('Controlled provider', 'provider'), account: unknown,
      model: observed('claude-actual-main', 'claude.assistant.message.model'),
      configuredModel: observed('haiku-configured-alias', 'claude.system.init.model'),
      nativeVersion: observed('2.1.318', 'claude.system.init.claude_code_version'),
      nativeSession: observed('controlled-session-id', 'claude.result.session_id'),
      usage: { inputTokens: '9007199254740993', outputTokens: '5', cachedInputTokens: null, cacheCreationInputTokens: '1', scope: 'top_level', source: 'CONTROLLED UI TEST DATA: claude.result.usage', observedAt, completeness: 'partial' },
      nativeModels: { scope: 'whole_tree', source: 'CONTROLLED UI TEST DATA: claude.result.modelUsage', observedAt, completeness: 'partial', evidenceArtifactIds: [stdoutId], omittedModelCount: '2',
        models: Array.from({ length: 16 }, (_, index) => ({ model: 'background-model-' + String(index).padStart(2, '0'), inputTokens: index === 0 ? '9223372036854775807' : null, outputTokens: '2', cachedInputTokens: null, cacheCreationInputTokens: null, costEstimateUsd: index === 0 ? estimate : null })) },
    } });
    await page.route('**/api/v1/tasks/' + admission.task.id, async route => {
      const response = await route.fetch(); const envelope = await response.json(); envelope.data.latestAttempt = decorate(envelope.data.latestAttempt); await route.fulfill({ response, json: envelope });
    });
    await page.locator('#refresh-attempt').click();
    await page.waitForFunction(() => document.querySelector('#attempt-facts')?.textContent.includes('CONTROLLED UI TEST DATA'));
    const details = await page.locator('#attempt-facts').textContent();
    assert.match(details, /Observed actual primary modelclaude-actual-main/);
    assert.match(details, /Native configured model settinghaiku-configured-alias/);
    assert.match(details, /Observed invocation version2\.1\.318.*CONTROLLED UI TEST DATA/);
    assert.match(details, /Observed native session IDcontrolled-session-id/);
    assert.match(details, /Native conversation continuityUnsupported.*Council tasks and history do not resume/);
    assert.match(details, /Declared version assumptionMinimum 2\.1\.259.*2\.1\.318.*official_documentation_2026-11-06/);
    assert.match(details, /Live provider qualificationUnqualified/);
    assert.match(await page.locator('#attempt-summary').textContent(), /Top-level native usage.*9007199254740993/);
    await page.locator('#native-models-panel > summary').click();
    assert.equal(await page.locator('#native-models-list > li').count(), 16);
    assert.match(await page.locator('#native-models-facts').textContent(), /partial.*omitted models 2/);
    assert.match(await page.locator('#native-models-facts').textContent(), /Whole-tree.*Separate from the actual primary model and top-level counters/);
    const models = await page.locator('#native-models-list').textContent();
    assert.ok(models.includes(estimate + ' USD')); assert.ok(models.includes('9223372036854775807'));
    assert.match(models, /estimate, not a billed charge or subscription allowance/);
    await page.locator('#native-models-evidence button').click();
    await page.locator('#artifact-status').filter({ hasText: 'stream closed' }).waitFor();
    assert.ok((await page.locator('#artifact-output').textContent()).length > 0);
    assert.equal(await page.locator('#download-artifact').isEnabled(), true); assert.equal(starts, 1);
    assert.equal(await page.locator('#response-acceptance').isVisible(), false);
  } finally { if (owner) await owner.stop(); await f.close(); }
});

test('a controlled earlier-owner metadata shape cannot authorize new Claude execution', { timeout: 90000 }, async () => {
  const f = await fixture(); let owner;
  try {
    owner = await f.start(); const page = await f.browser.newPage(); let starts = 0;
    page.on('request', request => { if (request.method() === 'POST' && new URL(request.url()).pathname === '/api/v1/tasks') starts++; });
    await page.route('**/api/v1/meta', async route => {
      const response = await route.fetch(); const envelope = await response.json();
      envelope.data.appVersion = '2.0.0-dev.2'; envelope.data.managedExecution = 'native_codex'; envelope.data.executionContract = '002-v1';
      envelope.data.capacity = { connectionId: 'codex-native', limit: 1, activeAttemptId: null }; delete envelope.data.maintenanceContract; delete envelope.data.workflowContract; delete envelope.data.graphContract;
      envelope.data.capabilities = ['workspace_foundation', 'managed_execution'];
      await route.fulfill({ response, json: envelope });
    });
    await page.goto(owner.url); await page.locator('#app-status').filter({ hasText: 'Older owners require their exact maintenance Stop command' }).waitFor();
    assert.equal(await page.locator('#start-task').isDisabled(), true); assert.equal(starts, 0);
  } finally { if (owner) await owner.stop(); await f.close(); }
});
