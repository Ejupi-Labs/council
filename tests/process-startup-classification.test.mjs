import test from 'node:test';
import assert from 'node:assert/strict';
import childProcess from 'node:child_process';
import { EventEmitter } from 'node:events';
import { PassThrough } from 'node:stream';
import { syncBuiltinESMExports } from 'node:module';
import { ManagedProcess, ProcessCleanupError, PROCESS_CLEANUP_ERROR } from '../dist/process.js';

const deferred = () => { let resolve, reject; const promise = new Promise((done, fail) => { resolve = done; reject = fail; }); return { promise, resolve, reject }; };

async function simulate(t, { pid = 123456789, messages = [], supervisorError = false, cleanupError = false, exitCode = 1, expectedCleanup }) {
  const originalFork = childProcess.fork, cleanup = deferred(), fake = new EventEmitter(), errors = [];
  Object.assign(fake, { stdin: new PassThrough(), stdout: new PassThrough(), stderr: new PassThrough(), pid: pid === null ? undefined : pid, connected: false,
    send(_value, callback) { callback?.(null); } });
  const stopped = t.mock.method(ManagedProcess.prototype, 'forceKill', () => cleanup.promise);
  let managed;
  try {
    // No child or OS cleanup helper is ever started. This models IPC facts only.
    childProcess.fork = () => fake; syncBuiltinESMExports();
    managed = new ManagedProcess({ command: 'offline-never-launched', args: [], cwd: process.cwd() });
    managed.on('error', error => errors.push(error));
    for (const message of messages) fake.emit('message', message);
    if (supervisorError) fake.emit('error', new Error('PRIVATE_OS_DIAGNOSTIC'));
    const before = errors.length, closed = new Promise(done => managed.once('close', (code, signal) => done({ code, signal })));
    fake.emit('close', exitCode, null);
    assert.equal(stopped.mock.callCount(), 1); assert.equal(errors.length, before, 'Classification must await the owned cleanup attempt.');
    if (cleanupError) cleanup.reject(new ProcessCleanupError()); else cleanup.resolve();
    const outcome = await closed;
    assert.notEqual(outcome.code, 0); assert.equal(outcome.signal, null);
    assert.equal(errors.some(error => error instanceof ProcessCleanupError), expectedCleanup);
    assert.equal(errors.at(-1).message, expectedCleanup ? PROCESS_CLEANUP_ERROR : 'Agent supervisor exited without a verified vendor outcome.');
    assert.equal(JSON.stringify(errors.map(error => error.message)).includes('PRIVATE_OS_DIAGNOSTIC'), false);
  } finally {
    cleanup.resolve(); childProcess.fork = originalFork; syncBuiltinESMExports();
    fake.stdin.destroy(); fake.stdout.destroy(); fake.stderr.destroy();
  }
}

for (const exitCode of [0, 1]) {
  test(`lost startup and outcome IPC remain unverified after supervisor exit ${exitCode}`, async t => {
    await simulate(t, { exitCode, expectedCleanup: true });
  });
}

test('a trusted launch_error establishes no vendor start even when final outcome IPC is lost', async t => {
  await simulate(t, { messages: [{ type: 'launch_error' }], expectedCleanup: false });
});

test('failure to fork a supervisor does not invent an unknown vendor process', async t => {
  await simulate(t, { pid: null, supervisorError: true, exitCode: -2, expectedCleanup: false });
});

test('a known vendor PID retains the existing successful owned-drain classification', async t => {
  await simulate(t, { messages: [{ type: 'started', pid: 123456790 }], expectedCleanup: false });
});

test('a verified cleanup outcome remains distinct from an abnormal supervisor exit', async t => {
  await simulate(t, { messages: [{ type: 'closed', code: 1, signal: null, cleanupFailed: false }], expectedCleanup: false });
});

test('an unverified owned cleanup attempt keeps precedence over a known launch failure', async t => {
  await simulate(t, { messages: [{ type: 'launch_error' }], cleanupError: true, expectedCleanup: true });
});
