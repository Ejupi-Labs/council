import test from 'node:test';
import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import cp from 'node:child_process';
import { syncBuiltinESMExports } from 'node:module';
import { mkdtemp, readFile, rm, writeFile, mkdir } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join, delimiter } from 'node:path';
import { fileURLToPath } from 'node:url';
import { ManagedProcess, killProcessTree, resolveLaunch, PROCESS_KILL_TIMEOUT_MS } from '../dist/process.js';

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
    const started = performance.now(); await killProcessTree(target.pid);
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
    if (managed.vendorPid) assert.equal(alive(managed.vendorPid), false);
  } finally { await managed.forceKill(); }
});

test('forceKill is idempotent and bounds vendor/supervisor helpers concurrently', { skip: process.platform !== 'win32', timeout: 8000 }, async () => {
  const original = cp.spawn;
  const managed = new ManagedProcess({ command: process.execPath, args: ['-e', 'setInterval(() => {}, 1000)'], cwd });
  managed.stdout.resume(); managed.stderr.resume(); managed.stdin.end(); managed.on('error', () => {});
  const helpers = [];
  try {
    await new Promise(resolve => managed.once('spawn', resolve));
    cp.spawn = (command, args, options) => {
      if (String(command).toLowerCase().endsWith('taskkill.exe')) { const helper = original(process.execPath, ['-e', 'setInterval(() => {}, 1000)'], { stdio: 'ignore', windowsHide: true }); helpers.push(helper); return helper; }
      return original(command, args, options);
    };
    syncBuiltinESMExports();
    const started = performance.now(); const first = managed.forceKill(); const second = managed.forceKill();
    assert.equal(first, second); await first;
    assert.equal(helpers.length, 2); assert.ok(performance.now() - started < PROCESS_KILL_TIMEOUT_MS + 2000);
  } finally { cp.spawn = original; syncBuiltinESMExports(); await managed.forceKill(); for (const helper of helpers) helper.kill('SIGKILL'); }
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
