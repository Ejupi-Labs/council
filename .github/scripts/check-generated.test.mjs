import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { lstatSync, mkdirSync, mkdtempSync, readFileSync, realpathSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { basename, dirname, join } from 'node:path';
import test from 'node:test';
import { sanitizedGitEnvironment } from '../../dist/git-env.js';
import { checkGenerated } from './check-generated.mjs';

function fixture(prefix) {
  const base = realpathSync(tmpdir());
  const directory = realpathSync(mkdtempSync(join(base, prefix)));
  const identity = lstatSync(directory, { bigint: true });
  return { directory, remove() {
    const current = lstatSync(directory, { bigint: true });
    assert.ok(current.isDirectory() && !current.isSymbolicLink());
    assert.equal(current.dev, identity.dev); assert.equal(current.ino, identity.ino);
    assert.equal(realpathSync(directory), directory);
    assert.equal(dirname(directory), base);
    assert.ok(basename(directory).startsWith(prefix));
    assert.equal(realpathSync(base), base);
    rmSync(directory, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 });
  } };
}

function fixtureGit(directory, args, environment = process.env) {
  const result = spawnSync('git', args, {
    cwd: directory, shell: false, windowsHide: true, encoding: 'utf8', timeout: 10_000,
    env: sanitizedGitEnvironment(environment),
  });
  assert.equal(result.status, 0, result.error?.message ?? result.stderr);
  return result.stdout;
}

test('generated-file check covers clean, modified, staged, untracked and ignored output', () => {
  const owned = fixture('council-ci-generated-');
  const directory = owned.directory;
  const git = args => fixtureGit(directory, args);
  try {
    git(['-c', 'init.defaultBranch=main', 'init', '--quiet']);
    mkdirSync(join(directory, 'dist'));
    mkdirSync(join(directory, 'schemas'));
    writeFileSync(join(directory, '.gitignore'), 'dist/ignored-*\n');
    writeFileSync(join(directory, 'dist', 'base.js'), 'export const value = 1;\n');
    writeFileSync(join(directory, 'schemas', 'base.json'), '{}\n');
    git(['add', '--', '.gitignore', 'dist', 'schemas']);
    git(['-c', 'core.hooksPath=.git/no-hooks', '-c', 'commit.gpgsign=false', '-c', 'user.name=Offline CI Fixture', '-c', 'user.email=offline@example.invalid', 'commit', '--quiet', '-m', 'Generated baseline']);
    assert.doesNotThrow(() => checkGenerated(directory));

    writeFileSync(join(directory, 'dist', 'base.js'), 'export const value = 2;\n');
    assert.throws(() => checkGenerated(directory), /Generated files must be committed/);
    git(['add', '--', 'dist/base.js']);
    assert.throws(() => checkGenerated(directory), /Generated files must be committed/);
    git(['restore', '--staged', '--worktree', '--', 'dist/base.js']);

    writeFileSync(join(directory, 'dist', 'new-module.js'), 'export {};\n');
    assert.throws(() => checkGenerated(directory), /\?\? dist\/new-module\.js/);
    rmSync(join(directory, 'dist', 'new-module.js'));
    writeFileSync(join(directory, 'dist', 'ignored-output.js'), 'export {};\n');
    assert.throws(() => checkGenerated(directory), /!! dist\/ignored-output\.js/);
    rmSync(join(directory, 'dist', 'ignored-output.js'));

    writeFileSync(join(directory, 'schemas', 'new-schema.json'), '{}\n');
    assert.throws(() => checkGenerated(directory), /\?\? schemas\/new-schema\.json/);
    rmSync(join(directory, 'schemas', 'new-schema.json'));
    writeFileSync(join(directory, 'unrelated.txt'), 'Unrelated source files are outside this check.\n');
    assert.doesNotThrow(() => checkGenerated(directory));
  } finally { owned.remove(); }
});

test('a directory without Git metadata fails closed', () => {
  const owned = fixture('council-ci-no-git-');
  try { assert.throws(() => checkGenerated(owned.directory), /Unable to verify/); }
  finally { owned.remove(); }
});

test('Git environment redirection cannot hide intended generated changes or touch another index', () => {
  const owned = fixture('council-ci-redirection-');
  const intended = join(owned.directory, 'intended');
  const redirected = join(owned.directory, 'redirected');
  try {
    for (const directory of [intended, redirected]) {
      mkdirSync(directory);
      fixtureGit(directory, ['-c', 'init.defaultBranch=main', 'init', '--quiet']);
      mkdirSync(join(directory, 'dist'));
      writeFileSync(join(directory, 'dist', 'base.js'), 'export {};\n');
      fixtureGit(directory, ['add', '--', 'dist']);
      fixtureGit(directory, ['-c', 'core.hooksPath=.git/no-hooks', '-c', 'commit.gpgsign=false', '-c', 'user.name=Offline CI Fixture', '-c', 'user.email=offline@example.invalid', 'commit', '--quiet', '-m', 'Generated baseline']);
    }
    const externalIndex = join(redirected, '.git', 'index');
    const before = readFileSync(externalIndex);
    const beforeIdentity = lstatSync(externalIndex, { bigint: true });
    const environment = { ...process.env, GIT_DIR: join(redirected, '.git'), GIT_WORK_TREE: redirected, GIT_INDEX_FILE: externalIndex };
    writeFileSync(join(intended, 'dist', 'new-module.js'), 'export {};\n');
    assert.equal(realpathSync(fixtureGit(intended, ['rev-parse', '--show-toplevel'], environment).trim()), intended);
    assert.throws(() => checkGenerated(intended, environment), /\?\? dist\/new-module\.js/);
    assert.deepEqual(readFileSync(externalIndex), before);
    const afterIdentity = lstatSync(externalIndex, { bigint: true });
    for (const key of ['dev', 'ino', 'mtimeNs', 'ctimeNs']) assert.equal(afterIdentity[key], beforeIdentity[key]);
    assert.doesNotThrow(() => checkGenerated(redirected));
  } finally { owned.remove(); }
});
