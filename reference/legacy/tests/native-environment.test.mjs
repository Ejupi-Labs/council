import test from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join } from 'node:path';
import { NativeRunner } from '../dist/runner.js';

const cwd = fileURLToPath(new URL('..', import.meta.url));
const fixture = fileURLToPath(new URL('./fixtures/fake-native.mjs', import.meta.url));
function setup(profile, markerKeys = ['CODEX_WINDOWS_REGISTERED_CORE', 'CODEX_WINDOWS_SANDBOX_PACKAGE_FAMILY'], transport = 'native') {
  const environment = { ...process.env };
  for (const key of Object.keys(environment)) if (key.toUpperCase().startsWith('CODEX_')) delete environment[key];
  Object.assign(environment, { [markerKeys[0]]: '1', [markerKeys[1]]: 'fixture-desktop-package',
    CODEX_HOME: 'fixture-account-home', CODEX_SESSION_ID: 'fixture-session', CODEX_PERMISSION_PROFILE: 'fixture-profile', COUNCIL_DEPTH: '2' });
  const config = { version: 1, workspaceRoot: cwd, stateDir: join(cwd, '.council'), backend: 'native', maxConcurrency: 2,
    timeoutMs: 15000, maxOutputBytes: 16384, maxCalls: 8, agents: { worker: { enabled: true, profile, transport, command: process.execPath,
      args: [transport === 'acp' ? fileURLToPath(new URL('./fixtures/fake-acp-adapter.mjs', import.meta.url)) : fixture] } } };
  return { environment, runner: new NativeRunner(config, { environment }) };
}

for (const permission of ['read', 'write']) test(`independent native Codex drops Desktop dispatch markers and preserves ${permission} sandbox policy`, async () => {
  const { environment, runner } = setup('codex');
  const original = { ...environment };
  const result = await runner.run({ agent: 'worker', prompt: 'codex-routing-env', cwd, permission, timeoutMs: 15000 });
  assert.equal(result.status, 'completed', result.error);
  const child = JSON.parse(result.output);
  assert.equal(child.environment.CODEX_WINDOWS_REGISTERED_CORE, undefined);
  assert.equal(child.environment.CODEX_WINDOWS_SANDBOX_PACKAGE_FAMILY, undefined);
  assert.equal(child.environment.CODEX_HOME, 'fixture-account-home');
  assert.equal(child.environment.CODEX_SESSION_ID, 'fixture-session');
  assert.equal(child.environment.CODEX_PERMISSION_PROFILE, 'fixture-profile');
  assert.equal(child.environment.COUNCIL_DEPTH, '3');
  assert.equal(child.args[child.args.indexOf('--sandbox') + 1], permission === 'read' ? 'read-only' : 'workspace-write');
  assert.equal(child.args[child.args.indexOf('--ask-for-approval') + 1], 'never');
  assert.deepEqual(environment, original, 'Starting a worker must not alter its parent environment.');
});

test('native Codex removes case variants of Desktop dispatch markers', async () => {
  const { runner } = setup('codex', ['Codex_Windows_Registered_Core', 'codex_windows_sandbox_package_family']);
  const result = await runner.run({ agent: 'worker', prompt: 'codex-routing-env', cwd, permission: 'read', timeoutMs: 15000 });
  assert.equal(result.status, 'completed', result.error);
  const keys = Object.keys(JSON.parse(result.output).environment).map(key => key.toUpperCase());
  assert.ok(!keys.includes('CODEX_WINDOWS_REGISTERED_CORE'));
  assert.ok(!keys.includes('CODEX_WINDOWS_SANDBOX_PACKAGE_FAMILY'));
});

test('Codex environment isolation does not modify other native providers', async () => {
  const { runner } = setup('claude');
  const result = await runner.run({ agent: 'worker', prompt: 'codex-routing-env', cwd, permission: 'read', timeoutMs: 15000 });
  assert.equal(result.status, 'completed', result.error);
  const child = JSON.parse(result.output);
  assert.equal(child.environment.CODEX_WINDOWS_REGISTERED_CORE, '1');
  assert.equal(child.environment.CODEX_WINDOWS_SANDBOX_PACKAGE_FAMILY, 'fixture-desktop-package');
  assert.equal(child.args[child.args.indexOf('--tools') + 1], 'Read,Glob,Grep');
});

test('Codex environment isolation does not modify an explicitly configured ACP adapter', async () => {
  const { runner, environment } = setup('codex', undefined, 'acp');
  environment.COUNCIL_TEST_ACP_CASE = 'codex-routing-env';
  const result = await runner.run({ agent: 'worker', prompt: 'hello', cwd, permission: 'read', timeoutMs: 15000 });
  assert.equal(result.status, 'completed', result.error);
  assert.deepEqual(JSON.parse(result.output), { CODEX_WINDOWS_REGISTERED_CORE: '1',
    CODEX_WINDOWS_SANDBOX_PACKAGE_FAMILY: 'fixture-desktop-package', CODEX_HOME: 'fixture-account-home', COUNCIL_DEPTH: '3' });
});
