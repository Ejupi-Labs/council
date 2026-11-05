import test from 'node:test';
import assert from 'node:assert/strict';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { NativeRunner } from '../dist/runner.js';
import { ManagedProcess, ProcessCleanupError, PROCESS_CLEANUP_ERROR } from '../dist/process.js';

const deferred = () => { let resolve; const promise = new Promise(done => { resolve = done; }); return { promise, resolve }; };
const root = fileURLToPath(new URL('..', import.meta.url));
const fixture = fileURLToPath(new URL('./fixtures/fake-native.mjs', import.meta.url));

for (const order of ['cancel-first', 'timeout-first', 'cleanup-failure']) {
  test(`runner total deadline preserves the first abort cause during owned cleanup: ${order}`, { timeout: 30000 }, async t => {
    const launched = deferred(), stopped = deferred(), controller = new AbortController();
    const originalTimeout = globalThis.setTimeout, originalKill = ManagedProcess.prototype.kill, originalEmit = ManagedProcess.prototype.emit;
    let expire, managed, running;
    t.mock.method(globalThis, 'setTimeout', (callback, milliseconds, ...args) => {
      if (!expire && milliseconds === 15000) {
        expire = () => callback(...args);
        return originalTimeout(callback, 60000, ...args);
      }
      return originalTimeout(callback, milliseconds, ...args);
    });
    t.mock.method(ManagedProcess.prototype, 'emit', function (event, ...args) {
      if (event === 'spawn') { managed = this; launched.resolve(); }
      return originalEmit.call(this, event, ...args);
    });
    t.mock.method(ManagedProcess.prototype, 'kill', function () {
      managed = this;
      if (order === 'cleanup-failure') this.emit('error', new ProcessCleanupError());
      stopped.resolve(); return true; // Keep owned cleanup pending until after both abort causes.
    });
    try {
      const config = { version: 1, backend: 'native', workspaceRoot: root, stateDir: join(root, '.council'), maxConcurrency: 1,
        maxCalls: 1, timeoutMs: 15000, maxOutputBytes: 4096,
        agents: { fake: { enabled: true, profile: 'codex', transport: 'native', command: process.execPath, args: [fixture] } } };
      running = new NativeRunner(config).run({ agent: 'fake', prompt: 'hang', cwd: root, permission: 'read', timeoutMs: 15000, signal: controller.signal });
      await launched.promise; assert.equal(typeof expire, 'function');
      if (order === 'timeout-first') { expire(); controller.abort(); }
      else { controller.abort(); expire(); }
      await stopped.promise;
      originalKill.call(managed);
      const result = await running;
      assert.equal(result.status, order === 'timeout-first' ? 'timeout' : order === 'cleanup-failure' ? 'failed' : 'cancelled', result.error);
      if (order === 'cleanup-failure') assert.equal(result.error, PROCESS_CLEANUP_ERROR);
      else assert.match(result.error, order === 'timeout-first' ? /total time limit/ : /request cancelled/);
    } finally {
      controller.abort();
      if (managed) { originalKill.call(managed); await managed.forceKill().catch(() => {}); }
      if (running) await running;
    }
  });
}
