import test from 'node:test';
import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync, symlinkSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join, relative } from 'node:path';
import { GitWorkspaces } from '../dist/workspaces.js';
import { killProcessTree } from '../dist/process.js';

function fixture(t, commit = true) {
  const dir = mkdtempSync(join(tmpdir(), 'council-workspaces-'));
  t.after(() => rmSync(dir, { recursive: true, force: true }));
  const root = join(dir, 'repo');
  mkdirSync(root);
  const git = (...args) => execFileSync('git', ['-C', root, ...args], { encoding: 'utf8', windowsHide: true });
  git('init', '--quiet');
  git('config', 'user.email', 'test@example.invalid');
  git('config', 'user.name', 'Council Test');
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
  const script = join(dir, 'stall-filter.cjs'), pidFile = join(dir, 'filter.pid');
  writeFileSync(script, `require('node:fs').writeFileSync(${JSON.stringify(pidFile)}, String(process.pid));process.stdin.resume();setInterval(()=>{},1000);`);
  const command = `"${process.execPath.replaceAll('\\', '/')}" "${script.replaceAll('\\', '/')}"`;
  git('config', 'filter.council-audit.smudge', command);
  writeFileSync(join(root, '.gitattributes'), 'base.txt filter=council-audit\n');
  git('add', '.gitattributes'); git('commit', '--quiet', '-m', 'add local checkout filter');
  const controller = new AbortController(); let error, ownedPid;
  const creating = manager.create('cancel-git', 'codex', 'codex', controller.signal).catch(value => { error = value; });
  const alive = pid => { try { process.kill(pid, 0); return true; } catch { return false; } };
  try {
    const readyUntil = Date.now() + 10_000;
    while (!existsSync(pidFile) && !error && Date.now() < readyUntil) await new Promise(resolve => setTimeout(resolve, 20));
    assert.equal(error, undefined, error?.message);
    assert.ok(existsSync(pidFile), 'local Git filter must be active before cancellation');
    ownedPid = Number(readFileSync(pidFile, 'utf8')); controller.abort(); await creating;
    assert.ok(error); assert.match(error.message, /cancelled/);
    const stoppedUntil = Date.now() + 3000;
    while (alive(ownedPid) && Date.now() < stoppedUntil) await new Promise(resolve => setTimeout(resolve, 20));
    assert.equal(alive(ownedPid), false);
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
