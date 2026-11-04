import test from 'node:test';
import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { existsSync, mkdirSync, mkdtempSync, readFileSync, realpathSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { basename, dirname, isAbsolute, join, relative, sep } from 'node:path';
import { setImmediate } from 'node:timers/promises';
import { fileURLToPath } from 'node:url';
import { CouncilService } from '../dist/service.js';
import { CouncilStore } from '../dist/store.js';
import { loadConfig } from '../dist/config.js';
import { NativeRunner } from '../dist/runner.js';
import { sanitizedGitEnvironment } from '../dist/git-env.js';

const adapter = fileURLToPath(new URL('./fixtures/fake-acp-adapter.mjs', import.meta.url));
const changed = 'changed by ACP fixture';
const processResources = () => process.getActiveResourcesInfo().filter(name => /Process/i.test(name)).length;

function fixture(t, filenames, chooseFile) {
  const temporaryBase = realpathSync(tmpdir());
  const temporary = mkdtempSync(join(temporaryBase, 'council-service-acp-worktree-'));
  const root = join(temporary, 'project café 日本語'); mkdirSync(root);
  let service;
  t.after(async () => {
    await service?.close();
    const target = realpathSync(temporary);
    assert.equal(dirname(target), temporaryBase); assert.ok(basename(target).startsWith('council-service-acp-worktree-'));
    rmSync(target, { recursive: true, force: true });
  });
  const environment = { ...sanitizedGitEnvironment(), COUNCIL_DEPTH: '0', GIT_TERMINAL_PROMPT: '0' };
  const hooks = join(temporary, 'disabled-hooks'), attributes = join(temporary, 'empty-attributes'); writeFileSync(attributes, '');
  const git = (...args) => execFileSync('git', ['-c', `core.hooksPath=${hooks}`, '-c', `core.attributesFile=${attributes}`, '-c', 'commit.gpgsign=false', '-C', root, ...args], {
    windowsHide: true, encoding: 'utf8', env: environment, timeout: 15_000,
  });
  git('init', '--quiet');
  assert.equal(realpathSync(git('rev-parse', '--show-toplevel').trim()), realpathSync(root));
  for (const [key, value] of [['user.name', 'Offline ACP Worktree Test'], ['user.email', 'offline@example.invalid'], ['commit.gpgsign', 'false'], ['core.autocrlf', 'false'], ['core.attributesFile', attributes], ['core.fsmonitor', 'false'], ['gc.auto', '0'], ['maintenance.auto', 'false']]) git('config', key, value);
  writeFileSync(join(root, '.gitignore'), '.council/\n');
  const originals = Object.fromEntries(filenames.map(filename => [filename, `Original ${filename}.\n`]));
  for (const [filename, content] of Object.entries(originals)) writeFileSync(join(root, filename), content);
  git('add', '.'); git('commit', '--quiet', '-m', 'offline ACP worktree fixture');
  const originalHead = git('rev-parse', 'HEAD'), originalIndex = readFileSync(join(root, '.git', 'index'));
  const settings = loadConfig(root, {
    backend: 'native', maxConcurrency: 2, timeoutMs: 15_000, maxOutputBytes: 4096, maxCalls: 8,
    agents: Object.fromEntries(['codex', 'claude'].map(profile => [profile, { enabled: true, profile, transport: 'acp', command: process.execPath, args: [adapter] }])),
  });
  const calls = []; let active = 0, peak = 0;
  const runner = {
    doctor: async () => [],
    run: async request => {
      const selected = chooseFile(request);
      const managed = relative(join(root, '.council', 'worktrees'), request.cwd);
      assert.ok(managed && managed !== '..' && !managed.startsWith('..' + sep) && !isAbsolute(managed));
      assert.equal(managed.split(sep).length, 2); assert.equal(request.timeoutMs, 15_000);
      const call = { agent: request.agent, cwd: request.cwd, permission: request.permission, prompt: request.prompt, filename: selected };
      calls.push(call); active++; peak = Math.max(peak, active);
      try {
        const native = new NativeRunner(settings, { environment: {
          ...environment, COUNCIL_TEST_ACP_CASE: request.permission === 'write' ? 'fs-write' : 'fs-read',
          COUNCIL_TEST_FILE: join(request.cwd, selected),
        } });
        call.result = await native.run(request); return call.result;
      } finally { active--; }
    },
  };
  const baselineResources = processResources();
  service = new CouncilService(settings, runner);
  const assertOriginal = () => {
    assert.equal(git('rev-parse', 'HEAD'), originalHead);
    assert.deepEqual(readFileSync(join(root, '.git', 'index')), originalIndex);
    for (const [filename, content] of Object.entries(originals)) assert.equal(readFileSync(join(root, filename), 'utf8'), content);
  };
  const finish = async run => {
    for (const artifact of run.workspaces) {
      assert.equal(artifact.exportStatus, 'ready'); assert.equal(artifact.changed, true);
      assert.match(readFileSync(artifact.patchPath, 'utf8'), /changed by ACP fixture/);
      git('apply', '--check', artifact.patchPath);
    }
    assertOriginal();
    const cleanup = await service.cleanup(run.id, { reviewed: true, discardChanges: true });
    assert.deepEqual(cleanup.removed, run.workspaces.map(artifact => artifact.path));
    assert.deepEqual(cleanup.alreadyRemoved, []);
    assert.ok(run.workspaces.every(artifact => !existsSync(artifact.path) && existsSync(artifact.patchPath)));
    assert.equal(git('worktree', 'list', '--porcelain', '-z').split('\0').filter(row => row.startsWith('worktree ')).length, 1);
    assert.equal(active, 0);
    const counts = service.store.health().counts;
    assert.equal(counts.activeRuns, 0); assert.equal(counts.slots, 0); assert.equal(counts.slotWaiters, 0); assert.equal(counts.leases, 0);
    assert.equal(service.health().maintenanceOperations, 0);
    assertOriginal(); await service.close(); await setImmediate();
    assert.ok(processResources() <= baselineResources, 'Completed ACP and Git subprocess resources must drain.');
    const reopened = new CouncilStore(join(root, '.council'));
    try {
      const final = reopened.health().counts;
      assert.equal(final.instances, 0); assert.equal(final.slots, 0); assert.equal(final.slotWaiters, 0); assert.equal(final.leases, 0);
    } finally { reopened.close(); }
  };
  return { service, root, calls, originals, finish, peak: () => peak };
}

// Each workflow includes multiple individually bounded Git operations and 15-second actor calls.
test('direct ACP review writes and reads through the author managed worktree before revision', { timeout: 180_000 }, async t => {
  const filename = 'review café.txt';
  const { service, calls, finish, peak } = fixture(t, [filename], () => filename);
  const run = await service.run({ mode: 'review', agents: ['codex', 'claude'], permission: 'write', task: 'Write the tracked fixture, read it independently, then revise in the same worktree.' });
  assert.equal(run.status, 'completed', run.error); assert.equal(run.workspaces.length, 1); assert.equal(run.steps.length, 3);
  assert.deepEqual(run.steps.map(step => step.role), ['author', 'reviewer', 'revision']);
  assert.deepEqual(calls.map(call => call.permission), ['write', 'read', 'write']);
  assert.deepEqual(calls.map(call => call.agent), ['codex', 'claude', 'codex']);
  assert.ok(calls.every(call => call.cwd === run.workspaces[0].path));
  assert.equal(JSON.parse(calls[1].result.output).content, changed);
  assert.ok(calls[2].prompt.includes(changed), 'The revision must receive the actual reviewer filesystem evidence.');
  assert.equal(readFileSync(join(run.workspaces[0].path, filename), 'utf8'), changed); assert.equal(peak(), 1);
  await finish(run);
});

test('direct ACP parallel writers modify only their assigned files in independent managed worktrees', { timeout: 180_000 }, async t => {
  const files = { codex: 'codex.txt', claude: 'claude.txt' };
  const { service, calls, originals, finish, peak } = fixture(t, Object.values(files), request => files[request.agent]);
  const run = await service.run({ mode: 'parallel', agents: ['codex', 'claude'], permission: 'write', task: 'Each independent writer updates its own tracked fixture.',
    assignments: { codex: 'Update codex.txt only.', claude: 'Update claude.txt only.' } });
  assert.equal(run.status, 'completed', run.error); assert.equal(run.workspaces.length, 2); assert.equal(run.steps.length, 2);
  assert.equal(calls.length, 2); assert.ok(calls.every(call => call.permission === 'write')); assert.equal(peak(), 2);
  assert.equal(new Set(calls.map(call => call.cwd)).size, 2);
  for (const artifact of run.workspaces) {
    const selected = files[artifact.agent], other = files[artifact.agent === 'codex' ? 'claude' : 'codex'];
    assert.equal(readFileSync(join(artifact.path, selected), 'utf8'), changed);
    assert.equal(readFileSync(join(artifact.path, other), 'utf8'), originals[other]);
    const patch = readFileSync(artifact.patchPath, 'utf8');
    assert.ok(patch.includes(`a/${selected}`)); assert.ok(!patch.includes(`a/${other}`));
    assert.equal(calls.find(call => call.agent === artifact.agent).cwd, artifact.path);
  }
  await finish(run);
});
