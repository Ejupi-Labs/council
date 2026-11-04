import test from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join } from 'node:path';
import { AcpxRunner, NativeRunner } from '../dist/runner.js';
import { AcpProtocol } from '../dist/acp.js';
import { ManagedProcess, ProcessCleanupError, PROCESS_CLEANUP_ERROR } from '../dist/process.js';

const root = fileURLToPath(new URL('..', import.meta.url));
const fixture = fileURLToPath(new URL('./fixtures/fake-acpx.mjs', import.meta.url));
const config = { version: 1, backend: 'acpx', workspaceRoot: root, stateDir: join(root, '.council'), maxConcurrency: 1,
  timeoutMs: 15000, maxOutputBytes: 16384, maxCalls: 1, agents: { codex: { enabled: true, profile: 'codex' } } };

for (const scenario of ['late-text', 'late-text-then-cancel', 'error']) {
  test(`cleanup diagnostics retain the public protocol rejection: ${scenario}`, { timeout: 30000 }, async t => {
    const controller = new AbortController(), original = ManagedProcess.prototype.kill;
    let injected = 0;
    t.mock.method(ManagedProcess.prototype, 'kill', function (...args) {
      if (!injected++) this.emit('error', new ProcessCleanupError());
      const stopped = original.apply(this, args);
      if (scenario === 'late-text-then-cancel') controller.abort();
      return stopped;
    });
    const result = await new AcpxRunner(config, { acpxEntrypoint: fixture }).run({ agent: 'codex', cwd: root,
      prompt: scenario === 'late-text-then-cancel' ? 'late-text' : scenario, permission: 'read', timeoutMs: 15000, signal: controller.signal });
    assert.ok(injected > 0); assert.equal(result.status, 'failed'); assert.ok(result.error.startsWith(PROCESS_CLEANUP_ERROR));
    assert.match(result.error, scenario === 'error' ? /protocol error/ : /after terminal completion/);
    assert.ok(result.error.length <= 4096); assert.equal(JSON.stringify(result).includes('SECRET_ACCESS_TOKEN'), false);
    assert.equal(result.output, scenario === 'error' ? '' : 'first');
    if (scenario === 'late-text-then-cancel') assert.equal(controller.signal.aborted, true);
  });
}

test('combined public cleanup diagnostics stay bounded without changing their failed precedence', { timeout: 30000 }, async t => {
  const original = ManagedProcess.prototype.kill;
  t.mock.method(ManagedProcess.prototype, 'kill', function (...args) { this.emit('error', new ProcessCleanupError()); return original.apply(this, args); });
  t.mock.method(AcpProtocol.prototype, 'event', (_event, control) => { control.fail('Deliberate public protocol rejection: ' + 'x'.repeat(10000)); });
  const adapter = fileURLToPath(new URL('./fixtures/fake-acp-adapter.mjs', import.meta.url));
  const runner = new NativeRunner({ ...config, backend: 'native', agents: { codex: { enabled: true, profile: 'codex', transport: 'acp', command: process.execPath, args: [adapter] } } });
  const result = await runner.run({ agent: 'codex', cwd: root, prompt: 'Offline diagnostic fault injection.', permission: 'read', timeoutMs: 15000 });
  assert.equal(result.status, 'failed'); assert.ok(result.error.startsWith(PROCESS_CLEANUP_ERROR));
  assert.match(result.error, /Deliberate public protocol rejection/); assert.equal(result.error.length, 4096);
});
