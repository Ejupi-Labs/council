import test from 'node:test';
import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, relative } from 'node:path';
import { GitWorkspaces } from '../dist/workspaces.js';

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
  return { root, git, manager: new GitWorkspaces(root, join(dir, 'state')) };
}

test('requires an existing HEAD and does not modify an unborn checkout', t => {
  const { root, manager, git } = fixture(t, false);
  assert.throws(() => manager.create('run', 'codex'), /existing HEAD commit/);
  assert.equal(readFileSync(join(root, 'base.txt'), 'utf8'), 'committed content\n');
  assert.equal(git('worktree', 'list', '--porcelain').split('worktree ').length - 1, 1);
});

test('retains detached worktrees and exports tracked, new and binary files', t => {
  const { root, manager, git } = fixture(t);
  const originalHead = git('rev-parse', 'HEAD');
  writeFileSync(join(root, 'base.txt'), 'dirty original stays here\n');
  const workspace = manager.create('run', 'codex');
  assert.equal(readFileSync(join(workspace.cwd, 'base.txt'), 'utf8'), 'committed content\n');
  writeFileSync(join(workspace.cwd, 'base.txt'), 'edited in isolation\n');
  writeFileSync(join(workspace.cwd, 'new file.txt'), 'new file\n');
  writeFileSync(join(workspace.cwd, 'asset.bin'), Buffer.from([0, 1, 255, 42]));
  const artifact = manager.export(workspace);
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

test('unchanged workspace exports an empty patch', t => {
  const { manager } = fixture(t);
  const workspace = manager.create('run', 'claude');
  const artifact = manager.export(workspace);
  assert.equal(artifact.changed, false);
  assert.equal(readFileSync(artifact.patchPath, 'utf8'), '');
});

test('patch includes commits made by an agent against the original base', t => {
  const { manager } = fixture(t);
  const workspace = manager.create('committed-run', 'codex');
  writeFileSync(join(workspace.path, 'base.txt'), 'committed by worker\n');
  execFileSync('git', ['-C', workspace.path, 'add', '.'], { windowsHide: true });
  execFileSync('git', ['-C', workspace.path, 'commit', '--quiet', '-m', 'agent change'], { windowsHide: true });
  const artifact = manager.export(workspace);
  assert.equal(artifact.changed, true);
  assert.match(readFileSync(artifact.patchPath, 'utf8'), /committed by worker/);
});

test('untrusted labels cannot escape the state directory', t => {
  const { manager } = fixture(t);
  const workspace = manager.create('../../outside', '../agent', '../../slot');
  assert.ok(!relative(join(workspace.path, '..', '..', '..'), workspace.path).startsWith('..'));
  assert.match(workspace.path, /worktrees/);
  assert.ok(!workspace.path.includes('..'));
});
