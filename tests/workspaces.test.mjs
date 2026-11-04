import test from 'node:test';
import assert from 'node:assert/strict';
import fsPromises from 'node:fs/promises';
import cp from 'node:child_process';
import { EventEmitter } from 'node:events';
import { PassThrough } from 'node:stream';
import { syncBuiltinESMExports } from 'node:module';
import { execFileSync } from 'node:child_process';
import { existsSync, mkdirSync, mkdtempSync, readFileSync, readdirSync, realpathSync, rmSync, statSync, writeFileSync, symlinkSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { basename, dirname, isAbsolute, join, relative } from 'node:path';
import { GitWorkspaces } from '../dist/workspaces.js';
import { gitStatusUntrackedPaths } from '../dist/git-status.js';
import { killProcessTree, ManagedProcess, ProcessCleanupError } from '../dist/process.js';

function fixture(t, commit = true) {
  const dir = mkdtempSync(join(realpathSync.native(tmpdir()), 'council-workspaces-'));
  t.after(() => rmSync(dir, { recursive: true, force: true }));
  const root = join(dir, 'repo');
  mkdirSync(root);
  const git = (...args) => execFileSync('git', ['-C', root, ...args], { encoding: 'utf8', windowsHide: true });
  git('init', '--quiet');
  git('config', 'user.email', 'test@example.invalid');
  git('config', 'user.name', 'Council Test');
  git('config', 'core.autocrlf', 'false');
  writeFileSync(join(root, 'base.txt'), 'committed content\n');
  if (commit) { git('add', '.'); git('commit', '--quiet', '-m', 'base'); }
  return { root, git, dir, state: join(dir, 'state'), manager: new GitWorkspaces(root, join(dir, 'state')) };
}

test('requires an existing HEAD and does not modify an unborn checkout', async t => {
  const { root, manager, git } = fixture(t, false);
  await assert.rejects(manager.create('run', 'codex'), /existing HEAD commit/);
  assert.equal(readFileSync(join(root, 'base.txt'), 'utf8'), 'committed content\n');
  assert.equal(git('worktree', 'list', '--porcelain').split('worktree ').length - 1, 1);
});

test('retains detached worktrees and exports tracked, new and binary files', async t => {
  const { root, manager, git } = fixture(t);
  const originalHead = git('rev-parse', 'HEAD');
  writeFileSync(join(root, 'base.txt'), 'dirty original stays here\n');
  const workspace = await manager.create('run', 'codex');
  assert.equal(readFileSync(join(workspace.cwd, 'base.txt'), 'utf8'), 'committed content\n');
  writeFileSync(join(workspace.cwd, 'base.txt'), 'edited in isolation\n');
  writeFileSync(join(workspace.cwd, 'new file.txt'), 'new file\n');
  writeFileSync(join(workspace.cwd, 'asset.bin'), Buffer.from([0, 1, 255, 42]));
  const artifact = await manager.export(workspace);
  const patch = readFileSync(artifact.patchPath, 'utf8');
  assert.equal(artifact.changed, true);
  assert.match(patch, /edited in isolation/);
  assert.match(patch, /new file.txt/);
  assert.match(patch, /GIT binary patch/);
  assert.ok(existsSync(workspace.path));
  assert.equal(git('rev-parse', 'HEAD'), originalHead);
  assert.equal(readFileSync(join(root, 'base.txt'), 'utf8'), 'dirty original stays here\n');
  assert.match(git('worktree', 'list', '--porcelain'), /detached/);
});

test('unchanged workspace exports an empty patch', async t => {
  const { manager } = fixture(t);
  const workspace = await manager.create('run', 'claude');
  const artifact = await manager.export(workspace);
  assert.equal(artifact.changed, false);
  assert.equal(readFileSync(artifact.patchPath, 'utf8'), '');
});

test('patch includes commits made by an agent against the original base', async t => {
  const { manager } = fixture(t);
  const workspace = await manager.create('committed-run', 'codex');
  writeFileSync(join(workspace.path, 'base.txt'), 'committed by worker\n');
  execFileSync('git', ['-C', workspace.path, 'add', '.'], { windowsHide: true });
  execFileSync('git', ['-C', workspace.path, 'commit', '--quiet', '-m', 'agent change'], { windowsHide: true });
  const artifact = await manager.export(workspace);
  assert.equal(artifact.changed, true);
  assert.match(readFileSync(artifact.patchPath, 'utf8'), /committed by worker/);
});

test('untrusted labels cannot escape the state directory', async t => {
  const { manager } = fixture(t);
  const workspace = await manager.create('../../outside', '../agent', '../../slot');
  assert.ok(!relative(join(workspace.path, '..', '..', '..'), workspace.path).startsWith('..'));
  assert.match(workspace.path, /worktrees/);
  assert.ok(!workspace.path.includes('..'));
});

test('durable manifests let a new manager export against the captured base', async t => {
  const { manager, root, state } = fixture(t);
  const workspace = await manager.create('restart', 'codex');
  writeFileSync(join(workspace.path, 'base.txt'), 'survives manager restart\n');
  const restored = await new GitWorkspaces(root, state).export({ agent: workspace.agent, path: workspace.path, patchPath: workspace.patchPath, changed: false, baseCommit: workspace.baseCommit });
  assert.equal(restored.baseCommit, workspace.baseHead);
  assert.match(readFileSync(restored.patchPath, 'utf8'), /survives manager restart/);
});

test('all workers in a run retain one base even if the original HEAD changes', async t => {
  const { manager, root, git } = fixture(t);
  const first = await manager.create('shared-base', 'codex');
  writeFileSync(join(root, 'base.txt'), 'new original commit\n');
  git('add', '.'); git('commit', '--quiet', '-m', 'advance original');
  const second = await manager.create('shared-base', 'claude');
  assert.equal(first.baseHead, second.baseHead);
  assert.equal(readFileSync(join(second.path, 'base.txt'), 'utf8'), 'committed content\n');
});

test('concurrent workers retain one base when HEAD advances during Git preflight', { timeout: 60_000 }, async t => {
  const { manager, root, git } = fixture(t);
  const originalGit = manager.git.bind(manager);
  let releaseFirst, capturedFirst;
  const gate = new Promise(resolve => { releaseFirst = resolve; });
  const captured = new Promise(resolve => { capturedFirst = resolve; });
  let preflights = 0;
  manager.git = async (...args) => {
    const result = await originalGit(...args);
    if (args[1][0] === 'rev-parse' && args[1].includes('--show-toplevel') && ++preflights === 1) {
      capturedFirst();
      await gate;
    }
    return result;
  };
  const pending = manager.create('concurrent-base', 'codex');
  try {
    await Promise.race([captured, pending]);
    writeFileSync(join(root, 'base.txt'), 'advanced while first worker waits\n');
    git('add', '.'); git('commit', '--quiet', '-m', 'advance concurrent original');
    const advancedHead = git('rev-parse', 'HEAD').trim();
    const second = await manager.create('concurrent-base', 'claude');
    releaseFirst();
    const first = await pending;
    const third = await manager.create('concurrent-base', 'junie');
    for (const workspace of [first, second, third]) {
      assert.equal(workspace.baseHead, advancedHead);
      assert.equal(readFileSync(join(workspace.cwd, 'base.txt'), 'utf8'), 'advanced while first worker waits\n');
    }
    assert.equal(preflights, 2, 'Later workers must reuse the selected base without another preflight.');
  } finally {
    releaseFirst();
    await pending.catch(() => {});
    manager.git = originalGit;
  }
});

test('patch export rejects symlinked managed ancestors', async t => {
  const { manager, dir } = fixture(t);
  const workspace = await manager.create('symlink-patch', 'codex');
  const outside = join(dir, 'outside'); mkdirSync(outside);
  writeFileSync(join(outside, 'protected.txt'), 'untouched');
  mkdirSync(dirname(dirname(workspace.patchPath)), { recursive: true });
  symlinkSync(outside, dirname(workspace.patchPath), 'junction');
  await assert.rejects(manager.export(workspace), /symlinks|junctions/);
  assert.equal(readFileSync(join(outside, 'protected.txt'), 'utf8'), 'untouched');
});

test('export rejects redirected Git metadata before touching an unrelated index', async t => {
  const { manager, root, git } = fixture(t);
  const workspace = await manager.create('git-metadata', 'codex');
  rmSync(join(workspace.path, '.git'));
  writeFileSync(join(workspace.path, '.git'), `gitdir: ${join(root, '.git')}\n`);
  await assert.rejects(manager.export(workspace), /redirected/);
  assert.equal(git('diff', '--cached', '--name-only'), '');
});

test('cleanup requires review and a patch, refuses dirty work unless explicitly discarded', async t => {
  const { manager } = fixture(t);
  const workspace = await manager.create('cleanup', 'codex');
  await assert.rejects(manager.cleanup(workspace, {}), /review/i);
  await assert.rejects(manager.cleanup(workspace, { reviewed: true }), /Export/);
  writeFileSync(join(workspace.path, 'base.txt'), 'unreviewed edit\n');
  const artifact = await manager.export(workspace);
  await assert.rejects(manager.cleanup(artifact, { reviewed: true }), /modified|untracked|changes/i);
  assert.equal(existsSync(workspace.path), true);
  await manager.cleanup(artifact, { reviewed: true, discardChanges: true });
  assert.equal(existsSync(workspace.path), false);
  assert.equal(existsSync(artifact.patchPath), true);
});

test('cleanup rejects arbitrary paths even with explicit review', async t => {
  const { manager, root } = fixture(t);
  await assert.rejects(manager.cleanup({ agent: 'codex', path: root, patchPath: join(root, 'patch'), changed: false }, { reviewed: true, discardChanges: true }), /outside/);
  assert.equal(existsSync(join(root, 'base.txt')), true);
});

test('duplicate creation cannot overwrite the durable base manifest', async t => {
  const { manager, root, state, git } = fixture(t);
  const workspace = await manager.create('duplicate', 'codex');
  writeFileSync(join(root, 'base.txt'), 'different original HEAD\n');
  git('add', '.'); git('commit', '--quiet', '-m', 'advance');
  await assert.rejects(new GitWorkspaces(root, state).create('duplicate', 'codex'), /EEXIST/);
  const artifact = await manager.export(workspace);
  assert.equal(artifact.baseCommit, workspace.baseHead);
});

test('cleanup protects clean commits created after the last reviewed patch', async t => {
  const { manager } = fixture(t);
  const workspace = await manager.create('late-commit', 'codex');
  const artifact = await manager.export(workspace);
  writeFileSync(join(workspace.path, 'base.txt'), 'unexported commit\n');
  execFileSync('git', ['-C', workspace.path, 'add', '.'], { windowsHide: true });
  execFileSync('git', ['-C', workspace.path, 'commit', '--quiet', '-m', 'late change'], { windowsHide: true });
  await assert.rejects(manager.cleanup(artifact, { reviewed: true }), /changed since patch export/);
  assert.equal(existsSync(workspace.path), true);
  const updated = await manager.export(workspace);
  await manager.cleanup(updated, { reviewed: true });
  assert.equal(existsSync(workspace.path), false);
});

test('cleanup reconciles interruption after Git removal and before manifest finalization', async t => {
  const { manager, root, state } = fixture(t);
  const workspace = await manager.create('cleanup-interrupted', 'codex');
  const artifact = await manager.export(workspace);
  const original = manager.atomicWrite.bind(manager);
  manager.atomicWrite = (path, data, category, ...rest) => category === 'workspace-manifests' && JSON.parse(String(data)).removed
    ? Promise.reject(new Error('offline interruption after Git removal')) : original(path, data, category, ...rest);
  await assert.rejects(manager.cleanup(artifact, { reviewed: true }), /offline interruption/);
  assert.equal(existsSync(artifact.path), false); assert.equal(existsSync(artifact.patchPath), true);
  const recovered = new GitWorkspaces(root, state);
  await recovered.cleanup(artifact, { reviewed: true });
  await recovered.cleanup(artifact, { reviewed: true });
  assert.equal(existsSync(artifact.patchPath), true);
});

test('cancellation stops an owned Git checkout filter and leaves its creation manifest', { timeout: 25_000 }, async t => {
  const { manager, root, dir, git, state } = fixture(t);
  const script = join(dir, 'stall-filter.cjs'), pidFile = join(dir, 'filter.pid'), heartbeat = join(dir, 'filter-heartbeat.txt');
  writeFileSync(script, `const fs=require('node:fs');fs.writeFileSync(${JSON.stringify(pidFile)},String(process.pid));fs.writeFileSync(${JSON.stringify(heartbeat)},String(Date.now())+'\\n');process.stdin.resume();setInterval(()=>fs.appendFileSync(${JSON.stringify(heartbeat)},String(Date.now())+'\\n'),50);`);
  const command = `"${process.execPath.replaceAll('\\', '/')}" "${script.replaceAll('\\', '/')}"`;
  git('config', 'filter.council-audit.smudge', command);
  writeFileSync(join(root, '.gitattributes'), 'base.txt filter=council-audit\n');
  git('add', '.gitattributes'); git('commit', '--quiet', '-m', 'add local checkout filter');
  const controller = new AbortController(); let error, ownedPid;
  const creating = manager.create('cancel-git', 'codex', 'codex', controller.signal).catch(value => { error = value; });
  const alive = pid => { try { process.kill(pid, 0); return true; } catch { return false; } };
  try {
    const readyUntil = Date.now() + 10_000;
    while ((!existsSync(pidFile) || !existsSync(heartbeat)) && !error && Date.now() < readyUntil) await new Promise(resolve => setTimeout(resolve, 20));
    assert.equal(error, undefined, error?.message);
    assert.ok(existsSync(pidFile), 'local Git filter must be active before cancellation');
    ownedPid = Number(readFileSync(pidFile, 'utf8')); controller.abort(); await creating;
    const settledAt = Date.now();
    assert.ok(error); assert.match(error.message, /cancelled/);
    const stoppedUntil = Date.now() + 3000;
    while (alive(ownedPid) && Date.now() < stoppedUntil) await new Promise(resolve => setTimeout(resolve, 20));
    const lastHeartbeat = Number(readFileSync(heartbeat, 'utf8').trim().split('\n').at(-1));
    assert.equal(alive(ownedPid), false, `Owned filter ${ownedPid} remained live; last heartbeat ${lastHeartbeat}, cancellation settled ${settledAt}.`);
    assert.ok(existsSync(join(state, 'workspace-manifests')));
  } finally { controller.abort(); await creating; if (ownedPid && alive(ownedPid)) await killProcessTree(ownedPid); }
});

test('preparation callback runs before any managed file or directory is created', async t => {
  const { manager, state } = fixture(t);
  let prepared;
  await assert.rejects(manager.create('intent-first', 'codex', 'codex', undefined, artifact => {
    prepared = artifact;
    assert.equal(existsSync(join(state, 'workspace-manifests')), false);
    assert.equal(existsSync(join(state, 'worktrees')), false);
    throw new Error('database intent could not be saved');
  }), /database intent/);
  assert.ok(prepared?.baseCommit); assert.equal(prepared.exportStatus, 'pending');
  assert.equal(existsSync(prepared.path), false);
  assert.equal(existsSync(join(state, 'workspace-manifests')), false);
});

test('inherited Git index and config overrides cannot mutate the original checkout during export', async t => {
  const { manager, root, git } = fixture(t);
  const workspace = await manager.create('inherited-index', 'codex');
  writeFileSync(join(workspace.path, 'new.txt'), 'only the worker owns this file\n');
  const original = readFileSync(join(root, '.git', 'index'));
  const overrides = {
    GIT_INDEX_FILE: join(root, '.git', 'index'), GIT_CONFIG_COUNT: '1',
    GIT_CONFIG_KEY_0: 'diff.noprefix', GIT_CONFIG_VALUE_0: 'true',
  };
  const previous = Object.fromEntries(Object.keys(overrides).map(key => [key, process.env[key]]));
  let artifact;
  try { Object.assign(process.env, overrides); artifact = await manager.export(workspace); }
  finally { for (const [key, value] of Object.entries(previous)) { if (value === undefined) delete process.env[key]; else process.env[key] = value; } }
  assert.deepEqual(readFileSync(join(root, '.git', 'index')), original);
  assert.equal(git('ls-files', '--', 'new.txt'), '');
  assert.match(readFileSync(artifact.patchPath, 'utf8'), /new file mode/);
  git('apply', '--check', artifact.patchPath);
});

test('patches remain applicable and cleanup stays consistent despite Git presentation settings', async t => {
  const { manager, git } = fixture(t);
  git('config', 'diff.noprefix', 'true'); git('config', 'color.ui', 'always'); git('config', 'diff.mnemonicPrefix', 'true');
  const workspace = await manager.create('configured-diff', 'codex');
  writeFileSync(join(workspace.path, 'base.txt'), 'a committed worker update\n');
  execFileSync('git', ['-C', workspace.path, 'add', 'base.txt'], { windowsHide: true });
  execFileSync('git', ['-C', workspace.path, 'commit', '--quiet', '-m', 'worker update'], { windowsHide: true });
  const artifact = await manager.export(workspace);
  const patch = readFileSync(artifact.patchPath, 'utf8');
  assert.match(patch, /--- a\/base.txt/); assert.match(patch, /\+\+\+ b\/base.txt/); assert.ok(!patch.includes('\u001b['));
  git('apply', '--check', artifact.patchPath);
  git('config', 'diff.noprefix', 'false'); git('config', 'color.ui', 'never'); git('config', 'diff.mnemonicPrefix', 'false');
  await manager.cleanup(artifact, { reviewed: true });
  assert.equal(existsSync(artifact.path), false);
});

test('Git submodule ignore and log settings cannot hide a changed gitlink from its patch', async t => {
  const { manager, root, git } = fixture(t);
  const initial = git('rev-parse', 'HEAD').trim();
  mkdirSync(join(root, 'vendor'));
  git('update-index', '--add', '--cacheinfo', `160000,${initial},vendor`);
  git('commit', '--quiet', '-m', 'record local gitlink fixture');
  const updated = git('rev-parse', 'HEAD').trim();
  git('config', 'diff.ignoreSubmodules', 'all'); git('config', 'diff.submodule', 'log');
  const workspace = await manager.create('configured-submodule', 'codex');
  execFileSync('git', ['-C', workspace.path, 'update-index', '--cacheinfo', `160000,${updated},vendor`], { windowsHide: true });
  const artifact = await manager.export(workspace);
  const patch = readFileSync(artifact.patchPath, 'utf8');
  assert.equal(artifact.changed, true); assert.match(patch, /diff --git a\/vendor b\/vendor/);
  assert.ok(patch.includes(`-Subproject commit ${initial}`)); assert.ok(patch.includes(`+Subproject commit ${updated}`));
  git('apply', '--cached', '--check', artifact.patchPath);
});

test('dirty submodule contents never publish a parent patch or pass legacy cleanup checks', { timeout: 60_000 }, async t => {
  const { manager, root, dir, git } = fixture(t);
  const nested = join(dir, 'nested-source'); mkdirSync(nested);
  const childGit = (...args) => execFileSync('git', ['-C', nested, ...args], { encoding: 'utf8', windowsHide: true });
  childGit('init', '--quiet'); childGit('config', 'user.name', 'Offline Nested Test'); childGit('config', 'user.email', 'offline@example.invalid');
  writeFileSync(join(nested, 'file.txt'), 'nested original\n'); childGit('add', '.'); childGit('commit', '--quiet', '-m', 'nested base');
  git('-c', 'protocol.file.allow=always', 'submodule', 'add', '--quiet', nested, 'vendor'); git('commit', '--quiet', '-m', 'add local submodule');
  git('config', 'submodule.vendor.ignore', 'all'); git('config', 'diff.ignoreSubmodules', 'all');
  const workspace = await manager.create('dirty-submodule', 'codex');
  execFileSync('git', ['-C', workspace.path, '-c', 'protocol.file.allow=always', 'submodule', 'update', '--init', '--quiet'], { windowsHide: true });
  const artifact = await manager.export(workspace);
  assert.equal(readFileSync(artifact.patchPath, 'utf8'), '');
  writeFileSync(join(workspace.path, 'vendor', 'file.txt'), 'uncommitted nested content\n');
  await assert.rejects(manager.export(artifact), /Uncommitted submodule files cannot be preserved/);
  assert.equal(readFileSync(artifact.patchPath, 'utf8'), '', 'failed export leaves the previous atomic patch intact');
  const legacy = execFileSync('git', ['-C', workspace.path, 'diff', '--ignore-submodules=none', '--submodule=short', workspace.baseHead, '--'], { encoding: 'utf8', windowsHide: true });
  assert.match(legacy, /-dirty/); writeFileSync(artifact.patchPath, legacy);
  await assert.rejects(manager.cleanup(artifact, { reviewed: true }), /Uncommitted submodule files cannot be preserved/);
  assert.equal(readFileSync(join(workspace.path, 'vendor', 'file.txt'), 'utf8'), 'uncommitted nested content\n');
  writeFileSync(join(workspace.path, 'vendor', 'file.txt'), 'nested original\n');
  writeFileSync(join(workspace.path, 'vendor', 'untracked.txt'), 'untracked nested content\n');
  await assert.rejects(manager.export(artifact), /Uncommitted submodule files cannot be preserved/);
  await assert.rejects(manager.cleanup(artifact, { reviewed: true }), /Uncommitted submodule files cannot be preserved/);
  assert.ok(existsSync(join(workspace.path, 'vendor', 'untracked.txt'))); assert.ok(existsSync(join(root, 'vendor', 'file.txt')));
});

test('a committed nested project retains its own cwd and an uncommitted directory fails before preparation effects', async t => {
  const { root, git, dir } = fixture(t);
  const nested = join(root, 'packages', 'selected'); mkdirSync(nested, { recursive: true });
  writeFileSync(join(nested, 'project.txt'), 'committed nested project\n'); git('add', '.'); git('commit', '--quiet', '-m', 'nested project');
  const state = join(dir, 'nested-state');
  const workspace = await new GitWorkspaces(nested, state).create('nested-project', 'codex');
  assert.equal(workspace.cwd, join(workspace.path, 'packages', 'selected'));
  assert.equal(readFileSync(join(workspace.cwd, 'project.txt'), 'utf8'), 'committed nested project\n');
  const fresh = join(root, 'uncommitted-project'); mkdirSync(fresh); writeFileSync(join(fresh, 'task.txt'), 'not in captured tree\n');
  const freshState = join(dir, 'fresh-state'); let prepared = false;
  await assert.rejects(new GitWorkspaces(fresh, freshState).create('uncommitted-directory', 'codex', 'codex', undefined, () => { prepared = true; }), /project directory is not present in the captured HEAD/);
  assert.equal(prepared, false); assert.equal(existsSync(freshState), false);
});

test('cancellation after patch fsync preserves the previous publication and removes staging files', { timeout: 60_000 }, async t => {
  const { manager } = fixture(t);
  const workspace = await manager.create('cancel-publication', 'codex');
  const artifact = await manager.export(workspace);
  writeFileSync(join(workspace.path, 'base.txt'), 'new patch must remain unpublished\n');
  const originalWrite = manager.atomicWrite.bind(manager), originalCheck = manager.checked.bind(manager);
  const controller = new AbortController(); let inside = false, checks = 0, entered, release;
  const started = new Promise(resolve => { entered = resolve; });
  const gate = new Promise(resolve => { release = resolve; });
  manager.atomicWrite = async function (...args) { inside = args[2] === 'patches'; try { return await originalWrite(...args); } finally { inside = false; } };
  manager.checked = async function (...args) {
    const result = await originalCheck(...args);
    if (inside && args[0] === artifact.patchPath && ++checks === 3) { entered(); await gate; }
    return result;
  };
  const exporting = manager.export(artifact, controller.signal);
  const cancelled = assert.rejects(exporting, error => error.name === 'AbortError');
  try {
    await started; assert.ok(readdirSync(dirname(artifact.patchPath)).some(name => name.endsWith('.tmp')));
    controller.abort(); release(); await cancelled;
    assert.equal(readFileSync(artifact.patchPath, 'utf8'), '');
    assert.ok(readdirSync(dirname(artifact.patchPath)).every(name => !name.endsWith('.tmp')));
  } finally { controller.abort(); release(); await Promise.allSettled([exporting]); }
});

test('a cancellation observed after an issued patch publication never returns a ready result', { timeout: 60_000 }, async t => {
  const { manager } = fixture(t);
  const workspace = await manager.create('cancel-issued-publication', 'codex');
  const artifact = await manager.export(workspace);
  writeFileSync(join(workspace.path, 'base.txt'), 'published patch remains valid evidence\n');
  const original = manager.atomicWrite.bind(manager), controller = new AbortController();
  manager.atomicWrite = async function (...args) { await original(...args); if (args[2] === 'patches') controller.abort(); };
  await assert.rejects(manager.export(artifact, controller.signal), error => error.name === 'AbortError');
  assert.match(readFileSync(artifact.patchPath, 'utf8'), /published patch remains valid evidence/);
});

test('relative Git worktree pointers recover an incomplete manifest and still refuse redirected reverse metadata', { timeout: 60_000 }, async t => {
  const { manager, root, state, git } = fixture(t);
  git('config', 'worktree.useRelativePaths', 'true');
  const workspace = await manager.create('relative-pointer-recovery', 'codex');
  const pointer = /^gitdir: (.+)\s*$/.exec(readFileSync(join(workspace.path, '.git'), 'utf8').trim());
  assert.ok(pointer);
  if (isAbsolute(pointer[1])) { t.skip('Installed Git does not create relative worktree pointers.'); return; }
  const manifestPath = manager.manifestPath(workspace.path);
  const incomplete = JSON.parse(readFileSync(manifestPath, 'utf8'));
  const gitDir = incomplete.gitDir;
  delete incomplete.gitDir; writeFileSync(manifestPath, JSON.stringify(incomplete));
  writeFileSync(join(workspace.path, 'base.txt'), 'relative-pointer recovery evidence\n');
  const artifact = await new GitWorkspaces(root, state).export(workspace);
  assert.equal(artifact.exportStatus, 'ready');
  assert.match(readFileSync(artifact.patchPath, 'utf8'), /relative-pointer recovery evidence/);
  assert.equal(JSON.parse(readFileSync(manifestPath, 'utf8')).gitDir, gitDir);
  writeFileSync(manifestPath, JSON.stringify(incomplete));
  writeFileSync(join(gitDir, 'gitdir'), relative(gitDir, join(root, '.git')) + '\n');
  await assert.rejects(new GitWorkspaces(root, state).export(workspace), /Incomplete worktree metadata does not belong/);
  assert.equal(git('diff', '--cached', '--name-only'), '');
});

test('Windows transient sharing refusals retain old manifests and patches until atomic replacement succeeds', { skip: process.platform !== 'win32', timeout: 60_000 }, async t => {
  const { manager } = fixture(t);
  const original = fsPromises.rename;
  const attempts = new Map(); let patchPath, previousPatch;
  fsPromises.rename = async (source, destination) => {
    const manifest = destination.endsWith('.json') && destination.includes('workspace-manifests');
    if (manifest || destination === patchPath) {
      const count = (attempts.get(destination) ?? 0) + 1; attempts.set(destination, count);
      if (count <= 2) {
        if (manifest) assert.equal(JSON.parse(readFileSync(destination, 'utf8')).gitDir, undefined);
        else assert.deepEqual(readFileSync(destination), previousPatch);
        throw Object.assign(new Error('Injected temporary Windows sharing refusal.'), { code: 'EPERM' });
      }
    }
    return await original(source, destination);
  };
  syncBuiltinESMExports();
  try {
    const workspace = await manager.create('sharing-retry', 'codex');
    assert.equal(attempts.get(manager.manifestPath(workspace.path)), 3);
    const artifact = await manager.export(workspace);
    patchPath = artifact.patchPath; previousPatch = readFileSync(patchPath);
    writeFileSync(join(workspace.path, 'base.txt'), 'publish only after transient sharing clears\n');
    const refreshed = await manager.export(artifact);
    assert.equal(refreshed.exportStatus, 'ready'); assert.equal(attempts.get(patchPath), 3);
    assert.match(readFileSync(patchPath, 'utf8'), /publish only after transient sharing clears/);
  } finally { fsPromises.rename = original; syncBuiltinESMExports(); }
});

test('aborting a Windows publication retry retains the original patch and removes its owned staging file', { skip: process.platform !== 'win32', timeout: 60_000 }, async t => {
  const { manager } = fixture(t);
  const workspace = await manager.create('abort-sharing-retry', 'codex'), artifact = await manager.export(workspace);
  const previous = readFileSync(artifact.patchPath), original = fsPromises.rename;
  const controller = new AbortController(); let attempts = 0, abortTimer;
  writeFileSync(join(workspace.path, 'base.txt'), 'this retry must not publish\n');
  fsPromises.rename = async (source, destination) => {
    if (destination === artifact.patchPath) {
      attempts++; abortTimer ??= setTimeout(() => controller.abort(), 5);
      throw Object.assign(new Error('Injected busy publication target.'), { code: 'EBUSY' });
    }
    return await original(source, destination);
  };
  syncBuiltinESMExports();
  try {
    await assert.rejects(manager.export(artifact, controller.signal), error => error.name === 'AbortError');
    assert.equal(attempts, 1); assert.deepEqual(readFileSync(artifact.patchPath), previous);
    assert.ok(readdirSync(dirname(artifact.patchPath)).every(name => !name.endsWith('.tmp')));
  } finally { clearTimeout(abortTimer); fsPromises.rename = original; syncBuiltinESMExports(); }
});

async function injectedCleanupFailure(t, { cancel = true, forceOnly = false } = {}) {
  const { manager, root } = fixture(t);
  const originalFork = cp.fork, originalForce = ManagedProcess.prototype.forceKill, originalTimeout = globalThis.setTimeout;
  const controller = new AbortController(), streams = [];
  cp.fork = () => {
    const child = new EventEmitter();
    child.stdin = new PassThrough(); child.stdout = new PassThrough(); child.stderr = new PassThrough();
    streams.push(child.stdin, child.stdout, child.stderr); child.connected = true;
    let closed = false;
    const close = () => {
      if (closed) return; closed = true;
      child.emit('message', { type: 'closed', code: 0, signal: null, cleanupFailed: !forceOnly });
      child.emit('close', 0, null);
    };
    child.send = (message, callback) => {
      callback?.();
      if (message.type === 'start') queueMicrotask(() => {
        child.emit('message', { type: 'started', pid: process.pid });
        if (cancel) controller.abort(); else queueMicrotask(close);
      });
      if (message.type === 'stop') originalTimeout(close, forceOnly ? 20 : 0);
    };
    return child;
  };
  if (forceOnly) {
    // Accelerate only the wrapper's escalation timer; no native helper or process is launched.
    globalThis.setTimeout = (callback, milliseconds, ...args) => originalTimeout(callback, milliseconds === 1500 ? 0 : milliseconds, ...args);
    ManagedProcess.prototype.forceKill = () => Promise.reject(new ProcessCleanupError());
  }
  syncBuiltinESMExports();
  try {
    const work = cancel ? manager.git(root, ['rev-parse', 'HEAD'], undefined, controller.signal) : manager.create('preflight-cleanup-failure', 'codex');
    await assert.rejects(work, error => error instanceof ProcessCleanupError && /descendant may still be running/.test(error.message));
  } finally {
    cp.fork = originalFork; ManagedProcess.prototype.forceKill = originalForce; globalThis.setTimeout = originalTimeout;
    syncBuiltinESMExports(); for (const stream of streams) stream.destroy();
  }
}

test('Git retains an unverified cleanup event after ordinary cancellation', async t => {
  await injectedCleanupFailure(t);
});

test('Git retains a rejected force cleanup instead of swallowing it behind cancellation', async t => {
  await injectedCleanupFailure(t, { forceOnly: true });
});

test('Git preflight preserves unverified cleanup diagnostics instead of claiming HEAD is absent', async t => {
  await injectedCleanupFailure(t, { cancel: false });
});

test('a selected workspace junction keeps the same Git identity and exports an applicable reviewed patch', { timeout: 30000 }, async t => {
  const { root, dir, state, git } = fixture(t), alias = join(dir, 'selected-alias');
  symlinkSync(root, alias, process.platform === 'win32' ? 'junction' : 'dir');
  const manager = new GitWorkspaces(alias, state), workspace = await manager.create('actual-directory-alias', 'codex');
  writeFileSync(join(workspace.cwd, 'base.txt'), 'Changed through the isolated alias workspace.\n');
  const artifact = await manager.export(workspace);
  assert.match(readFileSync(artifact.patchPath, 'utf8'), /\+Changed through the isolated alias workspace/);
  git('apply', '--check', artifact.patchPath);
  assert.equal(readFileSync(join(root, 'base.txt'), 'utf8'), 'committed content\n');
  await new GitWorkspaces(alias, state).cleanup(artifact, { reviewed: true, discardChanges: true });
  assert.equal(existsSync(workspace.path), false);
});

test('a nested committed project selected through a real directory alias retains its canonical suffix', { timeout: 30000 }, async t => {
  const { root, dir, state, git } = fixture(t), nested = join(root, 'nested'), alias = join(dir, 'nested-alias');
  mkdirSync(nested); writeFileSync(join(nested, 'project.txt'), 'Original nested evidence.\n');
  git('add', '.'); git('commit', '--quiet', '-m', 'nested project');
  symlinkSync(nested, alias, process.platform === 'win32' ? 'junction' : 'dir');
  const manager = new GitWorkspaces(alias, state), workspace = await manager.create('nested-directory-alias', 'codex');
  assert.equal(workspace.cwd, join(workspace.path, 'nested'));
  writeFileSync(join(workspace.cwd, 'project.txt'), 'Changed nested evidence.\n');
  const artifact = await manager.export(workspace);
  git('apply', '--check', artifact.patchPath);
  assert.match(readFileSync(artifact.patchPath, 'utf8'), /a\/nested\/project.txt/);
  assert.equal(readFileSync(join(nested, 'project.txt'), 'utf8'), 'Original nested evidence.\n');
  await manager.cleanup(artifact, { reviewed: true, discardChanges: true });
});

test('case-distinct filesystem roots cannot borrow a sibling repository through a case-folded containment check', { timeout: 30000 }, async t => {
  const parent = realpathSync.native(tmpdir()), dir = mkdtempSync(join(parent, 'council-case-workspaces-'));
  t.after(() => {
    assert.equal(dirname(realpathSync.native(dir)), parent); assert.ok(basename(dir).startsWith('council-case-workspaces-'));
    rmSync(dir, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 });
  });
  if (process.platform === 'win32') {
    try { execFileSync(join(process.env.SystemRoot ?? 'C:\\Windows', 'System32', 'fsutil.exe'), ['file', 'setCaseSensitiveInfo', dir, 'enable'], { windowsHide: true, stdio: 'pipe' }); }
    catch { t.skip('Per-directory NTFS case sensitivity is unavailable.'); return; }
  }
  const root = join(dir, 'project'), outside = join(dir, 'PROJECT'), state = join(dir, 'state');
  mkdirSync(root);
  if (existsSync(outside)) {
    const lower = statSync(root, { bigint: true }), upper = statSync(outside, { bigint: true });
    if (lower.dev === upper.dev && lower.ino === upper.ino) { t.skip('The fixture volume does not support case-distinct sibling directories.'); return; }
  }
  mkdirSync(outside);
  const git = (...args) => execFileSync('git', ['-C', root, ...args], { encoding: 'utf8', windowsHide: true });
  git('init', '--quiet'); git('config', 'user.name', 'Offline Case Test'); git('config', 'user.email', 'offline@example.invalid');
  git('config', 'commit.gpgsign', 'false'); git('config', 'core.autocrlf', 'false');
  writeFileSync(join(root, 'base.txt'), 'committed content\n'); git('add', '.'); git('commit', '--quiet', '-m', 'case fixture');
  git('config', 'core.worktree', root);
  writeFileSync(join(outside, '.git'), 'gitdir: ' + join(root, '.git') + '\n');
  writeFileSync(join(outside, 'unrelated.txt'), 'Keep the selected unrelated folder intact.\n');
  const reported = execFileSync('git', ['-C', outside, 'rev-parse', '--show-toplevel'], { encoding: 'utf8', windowsHide: true }).trim();
  assert.equal(realpathSync.native(reported), realpathSync.native(root));
  let prepared = false;
  await assert.rejects(new GitWorkspaces(outside, state).create('case-distinct-root', 'codex', 'codex', undefined, () => { prepared = true; }), /outside its Git repository/);
  assert.equal(prepared, false); assert.equal(existsSync(state), false);
  assert.deepEqual(readdirSync(outside).sort(), ['.git', 'unrelated.txt']);
  assert.equal(readFileSync(join(root, 'base.txt'), 'utf8'), 'committed content\n');
});

test('a direct SDK state alias with missing descendants publishes canonical artifacts that remain repairable after reopening', { timeout: 30000 }, async t => {
  const { root, dir, git } = fixture(t), actual = join(dir, 'state'), alias = join(dir, 'alias');
  mkdirSync(actual); symlinkSync(actual, alias, process.platform === 'win32' ? 'junction' : 'dir');
  const requested = join(alias, 'p', 's'), canonical = join(realpathSync.native(actual), 'p', 's');
  const manager = new GitWorkspaces(root, requested), workspace = await manager.create('direct-state-alias', 'codex');
  assert.ok(workspace.path.startsWith(join(canonical, 'worktrees')));
  writeFileSync(join(workspace.cwd, 'base.txt'), 'State aliases retain actual Git registration identity.\n');
  const first = await manager.export(workspace);
  const repaired = await new GitWorkspaces(root, canonical).export(first);
  git('apply', '--check', repaired.patchPath);
  assert.equal(readFileSync(join(root, 'base.txt'), 'utf8'), 'committed content\n');
  await new GitWorkspaces(root, requested).cleanup(repaired, { reviewed: true, discardChanges: true });
  assert.equal(existsSync(workspace.path), false);
});

test('combined export discovery removes one supervised call while retaining complete new paths and repeated patch bytes', async t => {
  const { manager, root, git } = fixture(t), workspace = await manager.create('combined-discovery', 'codex');
  writeFileSync(join(workspace.path, '[a].txt'), 'Literal bracket contents.\n');
  writeFileSync(join(workspace.path, 'a.txt'), 'Matching plain contents.\n');
  mkdirSync(join(workspace.path, 'new directory')); writeFileSync(join(workspace.path, 'new directory', '雪-é.txt'), 'Complete Unicode evidence.\n');
  const commands = [], original = manager.git.bind(manager);
  manager.git = async (...args) => { commands.push({ cwd: args[0], args: args[1], combinedLimit: args[4] === true }); return original(...args); };
  const first = await manager.export(workspace), patch = readFileSync(first.patchPath);
  assert.deepEqual(commands.map(call => call.args[0]), ['worktree', 'status', '--literal-pathspecs', 'diff']);
  assert.equal(commands[1].cwd, workspace.path); assert.ok(commands[1].args.includes('--untracked-files=all'));
  assert.equal(commands[1].combinedLimit, true); assert.ok(commands.filter(call => call.args[0] !== 'status').every(call => !call.combinedLimit));
  commands.length = 0; await manager.export(first);
  assert.deepEqual(commands.map(call => call.args[0]), ['worktree', 'status', 'diff']);
  assert.deepEqual(readFileSync(first.patchPath), patch);
  git('apply', '--check', first.patchPath); git('apply', first.patchPath);
  assert.equal(readFileSync(join(root, '[a].txt'), 'utf8'), 'Literal bracket contents.\n');
  assert.equal(readFileSync(join(root, 'a.txt'), 'utf8'), 'Matching plain contents.\n');
  assert.equal(readFileSync(join(root, 'new directory', '雪-é.txt'), 'utf8'), 'Complete Unicode evidence.\n');
});

test('combined discovery retains repository-root scope for a selected nested project', async t => {
  const { root, dir, git } = fixture(t), selected = join(root, 'project'); mkdirSync(selected);
  writeFileSync(join(selected, 'tracked.txt'), 'Committed nested project.\n'); git('add', '.'); git('commit', '--quiet', '-m', 'nested scope');
  const manager = new GitWorkspaces(selected, join(dir, 'nested-export')), workspace = await manager.create('nested-discovery', 'codex');
  writeFileSync(join(workspace.cwd, 'new-inside.txt'), 'Inside selected project.\n');
  writeFileSync(join(workspace.path, 'new-outside.txt'), 'Sibling repository evidence.\n');
  const original = manager.git.bind(manager); let statusCwd;
  manager.git = (...args) => { if (args[1][0] === 'status') statusCwd = args[0]; return original(...args); };
  const exported = await manager.export(workspace);
  assert.equal(statusCwd, workspace.path); assert.notEqual(statusCwd, workspace.cwd);
  git('apply', '--check', exported.patchPath); git('apply', exported.patchPath);
  assert.equal(readFileSync(join(root, 'project', 'new-inside.txt'), 'utf8'), 'Inside selected project.\n');
  assert.equal(readFileSync(join(root, 'new-outside.txt'), 'utf8'), 'Sibling repository evidence.\n');
});

test('POSIX combined status preserves invalid UTF-8 untracked paths through export', { skip: process.platform === 'win32' }, async t => {
  const { manager, root, git } = fixture(t), workspace = await manager.create('raw-posix-discovery', 'codex');
  const rawName = Buffer.concat([Buffer.from('raw-'), Buffer.from([0xff, 0x80])]);
  const rawPath = Buffer.concat([Buffer.from(workspace.path + '/'), rawName]);
  try { writeFileSync(rawPath, 'Complete raw pathname evidence.\n'); }
  catch (error) {
    if (['EILSEQ', 'EINVAL', 'ENOTSUP', 'EOPNOTSUPP'].includes(error.code)) { t.skip('Filesystem rejects invalid UTF-8 filenames: ' + error.code); return; }
    throw error;
  }
  const exported = await manager.export(workspace); git('apply', '--check', exported.patchPath); git('apply', exported.patchPath);
  assert.equal(readFileSync(Buffer.concat([Buffer.from(root + '/'), rawName]), 'utf8'), 'Complete raw pathname evidence.\n');
});

test('POSIX combined discovery preserves a literal colon filename independently of pathspec interpretation', { skip: process.platform === 'win32' }, async t => {
  const { manager } = fixture(t), workspace = await manager.create('colon-posix-discovery', 'codex');
  const colon = ':(exclude)victim'; writeFileSync(join(workspace.path, colon), 'Literal colon filename.\n');
  const bytes = execFileSync('git', ['-C', workspace.path, 'status', '--porcelain=v2', '-z', '--untracked-files=all', '--ignore-submodules=none'], { windowsHide: true });
  const discovered = gitStatusUntrackedPaths(bytes);
  assert.deepEqual(discovered, Buffer.from(colon + '\0'), 'Discovery preserves literal bytes; pathspec interpretation is a separate add concern.');
});

async function embeddedRepository(t, stage) {
  const value = fixture(t);
  if (stage === 'regular' || stage === 'renamed' || stage === 'symlink') {
    const name = stage === 'renamed' ? 'original' : 'nested';
    writeFileSync(join(value.root, name), 'Original tracked candidate contents.\n');
    if (stage === 'symlink') {
      value.git('config', 'core.symlinks', 'false');
      const hash = value.git('hash-object', '-w', '--', name).trim();
      value.git('update-index', '--add', '--cacheinfo', '120000,' + hash + ',' + name);
    } else value.git('add', '--', name);
    value.git('commit', '--quiet', '-m', 'tracked candidate');
  }
  const workspace = await value.manager.create('embedded-' + stage, 'codex');
  writeFileSync(join(workspace.path, 'base.txt'), 'Prior complete patch.\n');
  const previous = await value.manager.export(workspace), patch = readFileSync(previous.patchPath);
  const nested = join(workspace.path, 'nested');
  if (stage === 'renamed') execFileSync('git', ['-C', workspace.path, 'mv', '--', 'original', 'nested'], { windowsHide: true });
  if (stage === 'regular' || stage === 'renamed' || stage === 'symlink') await fsPromises.unlink(nested);
  mkdirSync(nested);
  const childGit = (...args) => execFileSync('git', ['-C', nested, ...args], { windowsHide: true, encoding: 'utf8' });
  childGit('init', '--quiet'); childGit('config', 'user.name', 'Offline Child'); childGit('config', 'user.email', 'child@example.invalid');
  childGit('config', 'core.autocrlf', 'false'); childGit('config', 'core.fsmonitor', 'false'); childGit('config', 'core.untrackedCache', 'false');
  writeFileSync(join(nested, 'tracked.txt'), 'Committed child base.\n'); childGit('add', '.'); childGit('commit', '--quiet', '-m', 'child base');
  if (stage === 'intent') execFileSync('git', ['-C', workspace.path, '-c', 'advice.addEmbeddedRepo=false', 'add', '--intent-to-add', '--', 'nested'], { windowsHide: true });
  return { ...value, workspace, previous, patch, nested, childGit };
}

for (const stage of ['untracked', 'intent', 'regular', 'renamed', 'symlink']) for (const dirty of ['tracked', 'untracked', 'both']) {
  test(`a ${stage} nested repository with ${dirty} dirty evidence never replaces its reviewed patch`, async t => {
    const { manager, workspace, previous, patch, nested } = await embeddedRepository(t, stage);
    if (dirty !== 'untracked') writeFileSync(join(nested, 'tracked.txt'), 'Retain uncommitted child evidence.\n');
    if (dirty !== 'tracked') writeFileSync(join(nested, 'new.txt'), 'Retain untracked child evidence.\n');
    const original = manager.git.bind(manager), calls = [];
    manager.git = (...args) => { calls.push(args[1]); return original(...args); };
    await assert.rejects(manager.export(previous), /Uncommitted submodule files cannot be preserved/);
    assert.deepEqual(readFileSync(previous.patchPath), patch, 'A previously published patch remains byte-for-byte intact.');
    assert.equal(calls.some(args => args.includes('add')), false, 'Dirty detection occurs before intent-to-add modifies the parent index.');
    assert.equal(calls.some(args => args[0] === 'diff'), false, 'No incomplete gitlink patch is generated.');
    assert.ok(calls.some(args => args.some(arg => arg.startsWith('--git-dir=')) && args.includes('--untracked-files=all') && args.includes('--ignore-submodules=none')));
    assert.ok(existsSync(workspace.path));
    if (dirty !== 'untracked') assert.equal(readFileSync(join(nested, 'tracked.txt'), 'utf8'), 'Retain uncommitted child evidence.\n');
    if (dirty !== 'tracked') assert.equal(readFileSync(join(nested, 'new.txt'), 'utf8'), 'Retain untracked child evidence.\n');
  });
}

for (const stage of ['untracked', 'intent', 'regular', 'renamed', 'symlink']) {
  test(`a clean ${stage} nested Git reference remains exportable after its HEAD advances`, async t => {
    const { manager, git, previous, nested, childGit } = await embeddedRepository(t, stage);
    writeFileSync(join(nested, 'tracked.txt'), 'New committed child evidence.\n'); childGit('add', '.'); childGit('commit', '--quiet', '-m', 'advanced clean child');
    const head = childGit('rev-parse', 'HEAD').trim(), exported = await manager.export(previous), patch = readFileSync(exported.patchPath, 'utf8');
    assert.equal(exported.exportStatus, 'ready'); assert.ok(patch.includes('+Subproject commit ' + head));
    assert.equal(patch.includes(head + '-dirty'), false); git('apply', '--check', exported.patchPath);
  });
}

test('embedded .git files, symlinks and commondir redirects are refused before child Git inspection', async t => {
  for (const redirect of ['file', 'symlink', 'commondir']) {
    await t.test(redirect, async t => {
      const { manager, dir, previous, patch, nested } = await embeddedRepository(t, 'untracked');
      const metadata = join(nested, '.git'), originalMetadata = join(dir, 'child-metadata-' + redirect);
      await fsPromises.rename(metadata, originalMetadata);
      if (redirect === 'file') writeFileSync(metadata, 'gitdir: ' + originalMetadata + '\n');
      else if (redirect === 'symlink') symlinkSync(originalMetadata, metadata, process.platform === 'win32' ? 'junction' : 'dir');
      else {
        mkdirSync(metadata);
        writeFileSync(join(metadata, 'HEAD'), readFileSync(join(originalMetadata, 'HEAD')));
        writeFileSync(join(metadata, 'index'), readFileSync(join(originalMetadata, 'index')));
        writeFileSync(join(metadata, 'commondir'), originalMetadata + '\n');
      }
      const original = manager.git.bind(manager), calls = [];
      manager.git = (...args) => { calls.push(args[1]); return original(...args); };
      await assert.rejects(manager.export(previous), /Nested Git repository metadata cannot be verified/);
      assert.equal(calls.some(args => args.some(arg => arg.startsWith('--git-dir='))), false);
      assert.deepEqual(readFileSync(previous.patchPath), patch);
    });
  }
});

test('POSIX nested repository names with invalid UTF-8 fail explicitly while preserving the prior patch', { skip: process.platform === 'win32' }, async t => {
  const { manager, workspace, previous, patch, nested } = await embeddedRepository(t, 'untracked');
  const raw = Buffer.concat([Buffer.from(workspace.path + '/raw-nested-'), Buffer.from([0xff, 0x80])]);
  try { await fsPromises.rename(nested, raw); }
  catch (error) {
    if (['EILSEQ', 'EINVAL', 'ENOTSUP', 'EOPNOTSUPP'].includes(error.code)) { t.skip('Filesystem rejects invalid UTF-8 filenames: ' + error.code); return; }
    throw error;
  }
  await assert.rejects(manager.export(previous), /nested Git repository path cannot be represented/);
  assert.deepEqual(readFileSync(previous.patchPath), patch);
});

test('literal staging ignores conflicting inherited Git pathspec selectors without changing caller settings', async t => {
  const { manager, root, git } = fixture(t), workspace = await manager.create('literal-environment', 'codex');
  writeFileSync(join(workspace.path, '[a].txt'), 'Bracket literal.\n'); writeFileSync(join(workspace.path, 'a.txt'), 'Plain literal.\n');
  const keys = ['GIT_GLOB_PATHSPECS', 'GIT_NOGLOB_PATHSPECS', 'GIT_ICASE_PATHSPECS', 'GIT_LITERAL_PATHSPECS'];
  const saved = keys.map(key => process.env[key]); let exported;
  try {
    for (const key of keys) process.env[key] = '1';
    exported = await manager.export(workspace);
    for (const key of keys) assert.equal(process.env[key], '1', 'Sanitizing helper environments never edits the caller.');
  } finally {
    for (let index = 0; index < keys.length; index++) { if (saved[index] === undefined) delete process.env[keys[index]]; else process.env[keys[index]] = saved[index]; }
  }
  git('apply', '--check', exported.patchPath); git('apply', exported.patchPath);
  assert.equal(readFileSync(join(root, '[a].txt'), 'utf8'), 'Bracket literal.\n'); assert.equal(readFileSync(join(root, 'a.txt'), 'utf8'), 'Plain literal.\n');
});

test('POSIX literal pathspec magic filenames survive complete export and patch application', { skip: process.platform === 'win32' }, async t => {
  const { manager, root, git } = fixture(t), workspace = await manager.create('literal-magic-posix', 'codex');
  const names = [':(exclude)victim', ':(top)victim', 'victim', 'marker'];
  for (const name of names) writeFileSync(join(workspace.path, name), 'Complete literal contents for ' + name + '\n');
  const exported = await manager.export(workspace); git('apply', '--check', exported.patchPath); git('apply', exported.patchPath);
  for (const name of names) assert.equal(readFileSync(join(root, name), 'utf8'), 'Complete literal contents for ' + name + '\n');
});
