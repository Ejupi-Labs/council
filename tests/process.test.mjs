import test from 'node:test';
import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import cp from 'node:child_process';
import { syncBuiltinESMExports } from 'node:module';
import { mkdtemp, readFile, rm, writeFile, mkdir } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join, delimiter } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { ManagedProcess, killProcessTree, ProcessCleanupError, resolveLaunch, PROCESS_KILL_TIMEOUT_MS } from '../dist/process.js';

const cwd = fileURLToPath(new URL('..', import.meta.url));
const fixture = fileURLToPath(new URL('./fixtures/supervised-host.mjs', import.meta.url));
const pause = ms => new Promise(resolve => setTimeout(resolve, ms));
const alive = pid => { try { process.kill(pid, 0); return true; } catch { return false; } };

test('supervisor kills active vendor and grandchild when Council parent is killed abruptly', { timeout: 20000 }, async () => {
  const temporary = await mkdtemp(join(tmpdir(), 'council-orphan-'));
  const pidFile = join(temporary, 'pids.json');
  const host = spawn(process.execPath, [fixture], { cwd, env: { ...process.env, COUNCIL_TEST_PID_FILE: pidFile, COUNCIL_TEST_CHATTER: '1' }, shell: false, windowsHide: true, stdio: 'ignore' });
  let pids;
  try {
    const readyUntil = Date.now() + 10000;
    while (Date.now() < readyUntil) {
      try { pids = JSON.parse(await readFile(pidFile, 'utf8')); break; } catch { await pause(50); }
    }
    assert.ok(pids?.vendor && pids?.grandchild, 'Owned subprocesses must start before parent-death test.');
    assert.ok(alive(pids.vendor)); assert.ok(alive(pids.grandchild));
    host.kill('SIGKILL');
    const stoppedUntil = Date.now() + 7000;
    while (Date.now() < stoppedUntil && (alive(pids.vendor) || alive(pids.grandchild))) await pause(50);
    assert.equal(alive(pids.vendor), false, 'Vendor process must stop after IPC parent disconnect.');
    assert.equal(alive(pids.grandchild), false, 'Vendor grandchild must stop after IPC parent disconnect.');
  } finally {
    if (host.pid && alive(host.pid)) host.kill('SIGKILL');
    if (pids?.vendor && alive(pids.vendor)) await killProcessTree(pids.vendor);
    if (pids?.grandchild && alive(pids.grandchild)) await killProcessTree(pids.grandchild);
    await rm(temporary, { recursive: true, force: true });
  }
});

test('process resolution preserves argument boundaries for JavaScript entrypoints', async () => {
  const result = await resolveLaunch(fixture, ['literal $() & ; "argument"'], cwd);
  assert.equal(result.command, process.execPath); assert.deepEqual(result.args, [fixture, 'literal $() & ; "argument"']);
});

test('POSIX tree termination distinguishes missing processes from unverified cleanup', async () => {
  const platform = Object.getOwnPropertyDescriptor(process, 'platform');
  const originalKill = process.kill;
  try {
    Object.defineProperty(process, 'platform', { value: 'linux' });
    for (const [groupError, leaderError, failed] of [
      [undefined, undefined, false], ['ESRCH', undefined, false], ['ESRCH', 'ESRCH', false],
      ['EPERM', undefined, true], ['EPERM', 'ESRCH', true], ['ESRCH', 'EPERM', true], ['EPERM', 'EPERM', true],
    ]) {
      const calls = [];
      process.kill = (pid, signal) => {
        calls.push([pid, signal]);
        const code = pid < 0 ? groupError : leaderError;
        if (code) throw Object.assign(new Error(code), { code });
        return true;
      };
      const cleanup = killProcessTree(987654);
      if (failed) await assert.rejects(cleanup, ProcessCleanupError);
      else await cleanup;
      assert.deepEqual(calls, groupError ? [[-987654, 'SIGKILL'], [987654, 'SIGKILL']] : [[-987654, 'SIGKILL']]);
    }
  } finally {
    process.kill = originalKill;
    Object.defineProperty(process, 'platform', platform);
  }
});

test('Windows npm wrapper resolution uses known package metadata rather than a shell', { skip: process.platform !== 'win32' }, async () => {
  const temporary = await mkdtemp(join(tmpdir(), 'council-shim-'));
  try {
    const root = join(temporary, 'node_modules', '@openai', 'codex');
    await mkdir(join(root, 'bin'), { recursive: true });
    await writeFile(join(temporary, 'codex'), '#!/bin/sh\necho never execute this POSIX shell wrapper');
    await writeFile(join(temporary, 'codex.cmd'), '@echo never execute this shell wrapper');
    await writeFile(join(root, 'package.json'), JSON.stringify({ name: '@openai/codex', bin: { codex: 'bin/codex.js' } }));
    await writeFile(join(root, 'bin', 'codex.js'), '// fixture only');
    const result = await resolveLaunch('codex', ['--version'], cwd, { PATH: temporary });
    assert.equal(result.command, process.execPath); assert.deepEqual(result.args, [join(root, 'bin', 'codex.js'), '--version']);
    await writeFile(join(temporary, 'unrecognized.cmd'), '@echo never execute this shell wrapper');
    await writeFile(join(temporary, 'unrecognized'), '#!/bin/sh\necho never execute this POSIX shell wrapper');
    await assert.rejects(resolveLaunch('unrecognized', [], cwd, { PATH: temporary + delimiter }), /shell wrappers are not executed/);
    await assert.rejects(resolveLaunch(join(temporary, 'unrecognized'), [], cwd), /shell entrypoints are not executed/);
  } finally { await rm(temporary, { recursive: true, force: true }); }
});

test('Windows Junie installer launchers resolve their bounded version pointer to a native executable', { skip: process.platform !== 'win32' }, async () => {
  const temporary = await mkdtemp(join(tmpdir(), 'council-junie-shim-'));
  try {
    const bin = join(temporary, '.local', 'bin'), root = join(temporary, '.local', 'share', 'junie');
    const target = join(root, 'versions', '2383.10', 'junie', 'junie.exe');
    await mkdir(bin, { recursive: true }); await mkdir(join(root, 'versions', '2383.10', 'junie'), { recursive: true });
    await writeFile(join(bin, 'junie.bat'), '@echo This fixture batch must never run.');
    await writeFile(join(root, 'current'), '2383.10\r\n');
    await writeFile(target, 'MZ native executable fixture');
    for (const command of ['junie', join(bin, 'junie.bat')]) {
      const args = ['--acp=true', 'literal $() & ; "argument"'];
      const result = await resolveLaunch(command, args, cwd, { PATH: bin });
      assert.equal(result.command, target); assert.deepEqual(result.args, args); assert.notEqual(result.args, args);
    }
  } finally { await rm(temporary, { recursive: true, force: true }); }
});

test('Windows Junie resolution refuses invalid pointers, missing native targets and arbitrary batch wrappers', { skip: process.platform !== 'win32' }, async () => {
  const temporary = await mkdtemp(join(tmpdir(), 'council-junie-shim-invalid-'));
  try {
    const bin = join(temporary, '.local', 'bin'), root = join(temporary, '.local', 'share', 'junie');
    const target = join(root, 'versions', '2383.10', 'junie', 'junie.exe');
    await mkdir(bin, { recursive: true }); await mkdir(join(root, 'versions', '2383.10', 'junie'), { recursive: true });
    await writeFile(join(bin, 'junie.bat'), '@echo This fixture batch must never run.');
    await writeFile(target, 'MZ native executable fixture');
    for (const pointer of ['', '../outside', '..\\outside', 'C:\\outside', '2383.10\0', '2383.10\n2383.11', '2383.10' + ' '.repeat(129), '2383.11']) {
      await writeFile(join(root, 'current'), pointer);
      await assert.rejects(resolveLaunch('junie', [], cwd, { PATH: bin }), /shell wrappers are not executed/);
    }
    await writeFile(join(root, 'current'), '2383.10');
    await writeFile(target, '#!/bin/sh\necho not a native executable');
    await assert.rejects(resolveLaunch('junie', [], cwd, { PATH: bin }), /shell wrappers are not executed/);
    await writeFile(join(temporary, 'junie.bat'), '@echo unrecognized installation layout');
    await writeFile(join(bin, 'arbitrary.bat'), '@echo never execute an arbitrary batch wrapper');
    await assert.rejects(resolveLaunch(join(temporary, 'junie.bat'), [], cwd), /shell wrappers are not executed/);
    await assert.rejects(resolveLaunch('arbitrary', [], cwd, { PATH: bin }), /shell wrappers are not executed/);
  } finally { await rm(temporary, { recursive: true, force: true }); }
});

test('Windows process termination remains bounded when its taskkill helper stalls', { skip: process.platform !== 'win32', timeout: 8000 }, async () => {
  const original = cp.spawn;
  const target = original(process.execPath, ['-e', 'setInterval(() => {}, 1000)'], { stdio: 'ignore', windowsHide: true });
  const helpers = [];
  try {
    cp.spawn = (command, args, options) => {
      if (String(command).toLowerCase().endsWith('taskkill.exe')) { const helper = original(process.execPath, ['-e', 'setInterval(() => {}, 1000)'], { stdio: 'ignore', windowsHide: true }); helpers.push(helper); return helper; }
      return original(command, args, options);
    };
    syncBuiltinESMExports();
    const started = performance.now(); const first = killProcessTree(target.pid), duplicate = killProcessTree(target.pid);
    assert.equal(first, duplicate, 'Concurrent cleanup of one owned tree must share its native helper.');
    await assert.rejects(first, ProcessCleanupError);
    assert.equal(helpers.length, 1);
    assert.ok(performance.now() - started < PROCESS_KILL_TIMEOUT_MS + 2000, 'A stalled native helper must not block cleanup indefinitely.');
    await pause(100); assert.equal(alive(target.pid), false); assert.ok(helpers.every(helper => !alive(helper.pid)));
  } finally { cp.spawn = original; syncBuiltinESMExports(); target.kill('SIGKILL'); for (const helper of helpers) helper.kill('SIGKILL'); }
});

test('an unexpected supervisor exit cannot masquerade as a successful vendor outcome', { timeout: 8000 }, async () => {
  const managed = new ManagedProcess({ command: process.execPath, args: ['-e', 'setInterval(() => {}, 1000)'], cwd });
  managed.stdout.resume(); managed.stderr.resume(); managed.stdin.end();
  const errors = []; managed.on('error', error => errors.push(error.message));
  const closed = new Promise(resolve => managed.once('close', (code, signal) => resolve({ code, signal })));
  try {
    await new Promise(resolve => managed.once('spawn', resolve));
    process.kill(managed.pid, 'SIGKILL');
    const outcome = await closed;
    assert.notEqual(outcome.code, 0); assert.ok(errors.some(error => /verified vendor outcome/.test(error)));
    if (managed.vendorPid) {
      // POSIX signal delivery can finish just after the failed supervisor close event.
      const deadline = Date.now() + 2000;
      while (alive(managed.vendorPid) && Date.now() < deadline) await pause(10);
      assert.equal(alive(managed.vendorPid), false, 'The owned vendor must actually stop within the cleanup bound.');
    }
  } finally { await managed.forceKill(); }
});

test('forceKill is idempotent and drains the vendor attempt before stopping its supervisor', { skip: process.platform !== 'win32', timeout: 2 * PROCESS_KILL_TIMEOUT_MS + 5000 }, async () => {
  const original = cp.spawn;
  const managed = new ManagedProcess({ command: process.execPath, args: ['-e', 'setInterval(() => {}, 1000)'], cwd });
  managed.stdout.resume(); managed.stderr.resume(); managed.stdin.end(); managed.on('error', () => {});
  const helpers = [];
  try {
    await new Promise(resolve => managed.once('spawn', resolve));
    cp.spawn = (command, args, options) => {
      if (String(command).toLowerCase().endsWith('taskkill.exe')) {
        if (helpers.length > 0) assert.equal(helpers[0].terminationRequested, true, 'The vendor-tree attempt must settle before killing its supervisor.');
        const helper = original(process.execPath, ['-e', 'setInterval(() => {}, 1000)'], { stdio: 'ignore', windowsHide: true });
        const kill = helper.kill.bind(helper);
        helper.kill = (...values) => { helper.terminationRequested = true; return kill(...values); };
        helpers.push(helper); return helper;
      }
      return original(command, args, options);
    };
    syncBuiltinESMExports();
    const started = performance.now(); const first = managed.forceKill(); const second = managed.forceKill();
    assert.equal(first, second); await assert.rejects(first, ProcessCleanupError);
    assert.equal(helpers.length, 2); assert.ok(performance.now() - started < 2 * PROCESS_KILL_TIMEOUT_MS + 2000);
  } finally { cp.spawn = original; syncBuiltinESMExports(); await managed.forceKill().catch(() => {}); for (const helper of helpers) helper.kill('SIGKILL'); }
});

test('normal vendor-leader exit also cleans its same-tree background child', { timeout: 8000 }, async () => {
  const temporary = await mkdtemp(join(tmpdir(), 'council-leader-exit-'));
  const pidFile = join(temporary, 'child.pid'); let pid;
  try {
    const source = `const {spawn}=require('node:child_process');const{writeFileSync}=require('node:fs');const c=spawn(process.execPath,['-e','setInterval(()=>{},1000)'],{stdio:'ignore',windowsHide:true});writeFileSync(process.argv[1],String(c.pid));setTimeout(()=>process.exit(0),20);`;
    const managed = new ManagedProcess({ command: process.execPath, args: ['-e', source, pidFile], cwd: temporary });
    managed.stdout.resume(); managed.stderr.resume(); managed.stdin.end(); managed.on('error', () => {});
    const outcome = await new Promise(resolve => managed.once('close', code => resolve(code)));
    pid = Number(await readFile(pidFile, 'utf8')); assert.equal(outcome, 0);
    await pause(100); assert.equal(alive(pid), false);
  } finally { if (pid && alive(pid)) await killProcessTree(pid); await rm(temporary, { recursive: true, force: true }); }
});

test('a slow Windows native tree walk finishes before escalation can orphan its descendant', { skip: process.platform !== 'win32', timeout: 25000 }, async () => {
  const temporary = await mkdtemp(join(tmpdir(), 'council-slow-tree-walk-'));
  const pidFile = join(temporary, 'pids.json'), killLog = join(temporary, 'kill.jsonl');
  const preload = pathToFileURL(fileURLToPath(new URL('./fixtures/delayed-taskkill-preload.mjs', import.meta.url))).href;
  const vendor = fileURLToPath(new URL('./fixtures/fake-native.mjs', import.meta.url));
  const managed = new ManagedProcess({ command: process.execPath, args: [vendor], cwd: temporary,
    environment: { ...process.env, NODE_OPTIONS: `--import=${preload}`, COUNCIL_TEST_PID_FILE: pidFile, COUNCIL_TEST_KILL_LOG: killLog } });
  managed.stdout.resume(); managed.stderr.resume(); managed.stdin.end('tree-hang');
  const errors = []; managed.on('error', error => errors.push(error));
  const closed = new Promise(resolve => managed.once('close', resolve));
  let pids, forceTimer, force;
  try {
    const readyUntil = Date.now() + 10000;
    while (Date.now() < readyUntil) {
      try { pids = JSON.parse(await readFile(pidFile, 'utf8')); break; } catch { await pause(20); }
    }
    assert.ok(pids?.vendor && pids?.grandchild, 'The owned tree must be alive before cancellation.');
    managed.kill();
    forceTimer = setTimeout(() => { force = managed.forceKill(); force.catch(() => {}); }, 1500);
    await closed;
    if (force) await force;
    const stopUntil = Date.now() + 3000;
    while (Date.now() < stopUntil && (alive(pids.vendor) || alive(pids.grandchild))) await pause(20);
    assert.equal(alive(pids.vendor), false);
    assert.equal(alive(pids.grandchild), false, 'The descendant must actually stop, not outlive a cancelled tree walk.');
    assert.equal(errors.some(error => error instanceof ProcessCleanupError), false);
    assert.equal((await readFile(killLog, 'utf8')).trim().split('\n').length, 1, 'Stop/close/escalation must not create competing native tree walks.');
  } finally {
    clearTimeout(forceTimer);
    await managed.forceKill().catch(() => {});
    if (pids?.grandchild && alive(pids.grandchild)) await killProcessTree(pids.grandchild);
    await rm(temporary, { recursive: true, force: true });
  }
});
