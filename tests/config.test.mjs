import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, rmSync, writeFileSync, mkdirSync, symlinkSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { configSchema, initConfig, loadConfig, parseWorkflow } from '../dist/config.js';

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
  assert.throws(() => configSchema.parse({ maxCalls: 64 }), /2 MiB/);
  assert.throws(() => configSchema.parse({ agents: { x: { profile: 'arbitrary-shell-command' } } }));
});

test('configuration refuses escaping or redirected state paths', t => {
  const root = workspace(t);
  assert.throws(() => loadConfig(root, { stateDir: '../outside' }), /inside the workspace/);
  assert.throws(() => loadConfig(root, { stateDir: '.' }), /inside the workspace/);
  const outside = workspace(t);
  symlinkSync(outside, join(root, 'linked'), process.platform === 'win32' ? 'junction' : 'dir');
  assert.throws(() => loadConfig(root, { stateDir: 'linked/state' }), /outside the workspace/);
});

test('initialization never overwrites a configuration and workflow inputs are bounded', t => {
  const root = workspace(t);
  initConfig(root, 'mock');
  assert.equal(loadConfig(root).backend, 'mock');
  assert.throws(() => initConfig(root), /EEXIST/);
  assert.throws(() => parseWorkflow({ mode: 'delegate', agents: ['codex'], task: 'x'.repeat(32_001) }));
  assert.throws(() => parseWorkflow({ mode: 'delegate', agents: ['codex'], task: 'x', extra: true }));
  assert.equal(parseWorkflow({ mode: 'delegate', agents: ['codex'], task: 'x' }).permission, 'read');
});
