import test from 'node:test';
import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { mkdtemp, readFile, rm, writeFile, mkdir } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join, delimiter } from 'node:path';
import { fileURLToPath } from 'node:url';
import { killProcessTree, resolveLaunch } from '../dist/process.js';

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
