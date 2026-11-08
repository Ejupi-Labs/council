import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { once } from 'node:events';
import { createInterface } from 'node:readline';
import test from 'node:test';
import { binary, root } from '../../scripts/tools.mjs';
import { fixture, fixtureRequestId, openWorkspace, pollOwner } from './helpers.mjs';

// The actual local stdio process and browser share a disposable fixture owner.
// No vendor process or credentials are used.
test('MCP and the browser share real fake tasks and their original instructions', { timeout: 120000 }, async t => {
  const f = await fixture(t);
  const owner = await f.start();
  const fragment = new URL(owner.url).hash.slice(1);
  const ownerToken = new URLSearchParams(fragment).get('token') ?? decodeURIComponent(fragment);
  assert.ok(ownerToken, 'fixture owner token is unavailable');
  const page = await openWorkspace(f, owner);
  const projectId = await page.locator('#project-select').inputValue();
  const child = spawn(binary, ['mcp'], { cwd: root, env: f.env, shell: false, stdio: ['pipe', 'pipe', 'pipe'] });
  const lines = createInterface({ input: child.stdout });
  const pending = new Map();
  let nextId = 0;
  let stderr = '';
  child.stderr.setEncoding('utf8');
  child.stderr.on('data', chunk => { stderr += chunk; });
  lines.on('line', line => {
    let message;
    try { message = JSON.parse(line); } catch {
      for (const request of pending.values()) request.reject(new Error('MCP stdout contained non-protocol output'));
      pending.clear(); return;
    }
    const request = pending.get(message.id);
    if (!request) return;
    pending.delete(message.id); clearTimeout(request.timer);
    if (message.error) request.reject(new Error('MCP protocol error: ' + message.error.code));
    else request.resolve(message.result);
  });
  // `close` follows stream drain; `exit` alone can precede final stderr bytes.
  const closed = once(child, 'close');
  let closing;
  const closeHost = () => closing ??= (async () => {
    lines.close(); child.stdin.end();
    let timer;
    const timedOut = await Promise.race([
      closed.then(() => false),
      new Promise(resolve => { timer = setTimeout(() => resolve(true), 5000); timer.unref(); }),
    ]);
    clearTimeout(timer);
    if (timedOut) {
      if (child.exitCode === null && child.signalCode === null) child.kill();
      let drainTimer;
      const drained = await Promise.race([
        closed.then(() => true),
        new Promise(resolve => { drainTimer = setTimeout(() => resolve(false), 5000); drainTimer.unref(); }),
      ]);
      clearTimeout(drainTimer);
      if (!drained) throw new Error('MCP fixture streams did not close');
    }
  })();
  t.after(async () => {
    await closeHost();
    assert.equal(stderr.includes(ownerToken), false, 'fixture owner token leaked to diagnostics');
  });
  const rpc = (method, params) => new Promise((resolve, reject) => {
    const id = ++nextId;
    const timer = setTimeout(() => { pending.delete(id); reject(new Error('MCP request timed out')); }, 20000);
    pending.set(id, { resolve, reject, timer });
    child.stdin.write(JSON.stringify({ jsonrpc: '2.0', id, method, params }) + '\n');
  });
  await rpc('initialize', { protocolVersion: '2025-11-25', capabilities: {}, clientInfo: { name: 'browser-fixture', version: '1' } });
  child.stdin.write(JSON.stringify({ jsonrpc: '2.0', method: 'notifications/initialized' }) + '\n');
  const call = async (name, args) => {
    const result = await rpc('tools/call', { name, arguments: args });
    assert.equal(result.isError, undefined, JSON.stringify(result.structuredContent?.envelope?.error));
    return result.structuredContent.envelope.data;
  };

  const hostText = 'MCP → browser: original task café 東京';
  const admission = await call('council_tasks_start', { input: {
    requestId: fixtureRequestId(), projectId, originalText: hostText,
    executionMode: 'fake', fakeScenario: 'response', workflow: { mode: 'author_only' },
    acceptance: { deliverable: 'response_only' },
  } });
  await pollOwner(owner, '/attempts/' + admission.attempt.id, value => value.phase === 'settled');
  const hostAttempts = await call('council_tasks_attempts_list', { taskId: admission.task.id });
  assert.equal(hostAttempts.items.length, 1);
  assert.equal(hostAttempts.items[0].id, admission.attempt.id);
  await page.locator('#refresh-tasks').click();
  await page.locator('#task-list button').filter({ hasText: admission.task.id.slice(0, 8) }).click();
  await page.locator('#task-original').filter({ hasText: hostText }).waitFor({ state: 'attached' });

  const browserText = 'Browser → MCP: unchanged text naïve 🧪';
  let rejectedNativeAdmission = false;
  await page.route('**/api/v1/tasks', async route => {
    if (route.request().method() !== 'POST') return route.continue();
    const input = route.request().postDataJSON();
    if (input.executionMode !== 'fake' || input.fakeScenario !== 'response') {
      rejectedNativeAdmission = true;
      return route.abort('blockedbyclient');
    }
    return route.continue();
  });
  await page.locator('#task-text').fill(browserText);
  const started = page.waitForResponse(response => response.request().method() === 'POST' && new URL(response.url()).pathname === '/api/v1/tasks');
  await page.locator('#start-task').click();
  const browserAdmission = (await (await started).json()).data;
  assert.ok(browserAdmission?.task?.id);
  assert.equal(rejectedNativeAdmission, false);
  assert.equal(browserAdmission.attempt.executionMode, 'fake');
  assert.equal(browserAdmission.attempt.fakeScenario, 'response');
  await pollOwner(owner, '/attempts/' + browserAdmission.attempt.id, value => value.phase === 'settled');
  const detail = await call('council_tasks_get', { taskId: browserAdmission.task.id });
  assert.equal(detail.task.id, browserAdmission.task.id);
  assert.equal(detail.task.originalText, browserText);
  assert.equal(detail.latestAttempt.id, browserAdmission.attempt.id);
  assert.equal(detail.latestAttempt.phase, 'settled');
  const browserAttempts = await call('council_tasks_attempts_list', { taskId: browserAdmission.task.id });
  assert.equal(browserAttempts.items.length, 1);
  assert.equal(browserAttempts.items[0].id, browserAdmission.attempt.id);
  await closeHost();
  assert.equal(stderr.includes(ownerToken), false, 'fixture owner token leaked to diagnostics');
});
