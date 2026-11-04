import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, rmSync, writeFileSync, mkdirSync, symlinkSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { configSchema, initConfig, loadConfig, parseWorkflow, readJsonFile } from '../dist/config.js';

function workspace(t) {
  const root = mkdtempSync(join(tmpdir(), 'council-config-'));
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
