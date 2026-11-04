import test from 'node:test';
import { execFileSync } from 'node:child_process';
import assert from 'node:assert/strict';
import { mkdtempSync, rmSync, writeFileSync, mkdirSync, realpathSync, existsSync, statSync, symlinkSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { configSchema, initConfig, loadConfig, parseWorkflow, readJsonFile } from '../dist/config.js';

function workspace(t) {
  const root = mkdtempSync(join(realpathSync.native(tmpdir()), 'council-config-'));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  return root;
}

test('configuration defaults and aliases preserve explicit models and bounds', t => {
  const root = workspace(t);
  const config = loadConfig(root);
  assert.equal(config.backend, 'native');
  assert.equal(config.maxCalls * config.maxOutputBytes, 2_097_152);
  writeFileSync(join(root, 'council.config.json'), JSON.stringify({ agents: { reviewer: { enabled: true, profile: 'claude', model: 'user-selected-model' } } }));
  assert.equal(loadConfig(root).agents.reviewer.model, 'user-selected-model');
  assert.equal(configSchema.parse({ maxCalls: 1000 }).maxCalls, 1000);
  assert.throws(() => configSchema.parse({ agents: { x: { profile: 'arbitrary-shell-command' } } }));
});

test('explicit call budgets, debate rounds and authority are not constrained by fixed workflow caps', () => {
  assert.equal(configSchema.parse({ maxCalls: 10_000, maxOutputBytes: 2_097_152 }).maxCalls, 10_000);
  for (const authority of [Number.MIN_VALUE, 0.001, 1000, 1e308]) {
    assert.equal(configSchema.parse({ agents: { peer: { profile: 'codex', authority } } }).agents.peer.authority, authority);
  }
  for (const authority of [0, -1, Infinity, NaN]) assert.throws(() => configSchema.parse({ agents: { peer: { profile: 'codex', authority } } }));
  const input = { mode: 'debate', agents: ['codex', 'claude'], task: 'Compare the evidence.', rounds: 40, maxCalls: 81 };
  assert.equal(parseWorkflow(input).rounds, 40);
  for (const value of [0, -1, 1.5, Number.MAX_SAFE_INTEGER + 1]) {
    assert.throws(() => configSchema.parse({ maxCalls: value }));
    assert.throws(() => parseWorkflow({ ...input, maxCalls: value }));
    assert.throws(() => parseWorkflow({ ...input, rounds: value }));
  }
});

test('configuration refuses escaping or redirected state paths', t => {
  const root = workspace(t);
  assert.throws(() => loadConfig(root, { stateDir: '../outside' }), /inside the workspace/);
  assert.throws(() => loadConfig(root, { stateDir: '.' }), /inside the workspace/);
  const outside = workspace(t);
  symlinkSync(outside, join(root, 'linked'), process.platform === 'win32' ? 'junction' : 'dir');
  assert.throws(() => loadConfig(root, { stateDir: 'linked/state' }), /outside the workspace/);
});

test('initialization never overwrites a configuration and workflow text remains complete', t => {
  const root = workspace(t);
  initConfig(root, 'mock');
  assert.equal(loadConfig(root).backend, 'mock');
  assert.throws(() => initConfig(root), /EEXIST/);
  assert.equal(parseWorkflow({ mode: 'delegate', agents: ['codex'], task: 'x'.repeat(32_001) }).task.length, 32_001);
  assert.throws(() => parseWorkflow({ mode: 'delegate', agents: ['codex'], task: 'x', extra: true }));
  assert.equal(parseWorkflow({ mode: 'delegate', agents: ['codex'], task: 'x' }).permission, 'read');
});

test('assignment-file readers can explicitly preserve JSON larger than the configuration file guard', t => {
  const root = workspace(t), filename = join(root, 'assignments.json');
  const assignment = 'Full independent task. '.repeat(50000) + 'ASSIGNMENT_END';
  writeFileSync(filename, JSON.stringify({ codex: assignment }));
  assert.throws(() => readJsonFile(filename), /1 MiB/);
  assert.equal(readJsonFile(filename, Infinity).codex, assignment);
  assert.throws(() => readJsonFile(filename, 0), /positive integer/);
});

test('configuration canonicalizes selected directory aliases and existing contained state ancestors without creating state', t => {
  const parent = workspace(t), root = join(parent, 'actual-project'), alias = join(parent, 'selected-alias');
  mkdirSync(root); mkdirSync(join(root, 'actual-state'));
  symlinkSync(root, alias, process.platform === 'win32' ? 'junction' : 'dir');
  symlinkSync(join(root, 'actual-state'), join(root, 'state-alias'), process.platform === 'win32' ? 'junction' : 'dir');
  const config = loadConfig(alias, { stateDir: 'state-alias/new-state' });
  assert.equal(config.workspaceRoot, realpathSync.native(root));
  assert.equal(config.stateDir, join(realpathSync.native(join(root, 'actual-state')), 'new-state'));
  assert.equal(existsSync(config.stateDir), false);
  symlinkSync(root, join(root, 'root-alias'), process.platform === 'win32' ? 'junction' : 'dir');
  assert.throws(() => loadConfig(root, { stateDir: 'root-alias' }), /directory inside the workspace/);
});

test('Windows 8.3 workspace and state aliases resolve to native canonical paths', { skip: process.platform !== 'win32' }, t => {
  const parent = workspace(t), root = join(parent, 'canonical-workspace-directory'), alias = join(parent, 'CANONI~1');
  mkdirSync(root);
  const state = join(root, 'stateparent-directory'), stateAlias = join(root, 'STATEP~1'); mkdirSync(state);
  if (!existsSync(alias) || !existsSync(stateAlias)) { t.skip('This volume does not create the expected Windows 8.3 aliases.'); return; }
  const config = loadConfig(alias, { stateDir: 'STATEP~1/new-state' });
  assert.equal(config.workspaceRoot, realpathSync.native(root));
  assert.equal(config.stateDir, join(realpathSync.native(state), 'new-state'));
  assert.equal(existsSync(config.stateDir), false);
});

test('configuration rejects case-distinct existing state ancestors outside the selected filesystem root', t => {
  const parent = workspace(t);
  if (process.platform === 'win32') {
    try { execFileSync(join(process.env.SystemRoot ?? 'C:\\Windows', 'System32', 'fsutil.exe'), ['file', 'setCaseSensitiveInfo', parent, 'enable'], { windowsHide: true, stdio: 'pipe' }); }
    catch { t.skip('Per-directory NTFS case sensitivity is unavailable.'); return; }
  }
  const root = join(parent, 'project'), sibling = join(parent, 'PROJECT'); mkdirSync(root);
  if (existsSync(sibling)) {
    const lower = statSync(root, { bigint: true }), upper = statSync(sibling, { bigint: true });
    if (lower.dev === upper.dev && lower.ino === upper.ino) { t.skip('The fixture volume does not support case-distinct sibling directories.'); return; }
  }
  mkdirSync(sibling);
  assert.throws(() => loadConfig(root, { stateDir: join(sibling, 'new-state') }), /(?:inside|outside) the workspace/);
  assert.equal(existsSync(join(sibling, 'new-state')), false);
  assert.equal(existsSync(join(root, 'new-state')), false);
});
