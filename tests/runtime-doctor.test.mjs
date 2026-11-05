import test from 'node:test';
import assert from 'node:assert/strict';
import { execFile } from 'node:child_process';
import { existsSync, mkdirSync, mkdtempSync, readFileSync, readdirSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { promisify } from 'node:util';
import { runtimeReadiness } from '../dist/sqlite-runtime.js';

const execute = promisify(execFile);
const cli = fileURLToPath(new URL('../dist/cli.js', import.meta.url));
function workspace(t) {
  const root = mkdtempSync(join(tmpdir(), 'council-runtime-doctor-'));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  return root;
}
function doctor(root, prelude) {
  const args = prelude ? ['--import', `data:text/javascript,${encodeURIComponent(prelude)}`] : [];
  return execute(process.execPath, [...args, cli, 'doctor', '--workspace', root, '--backend', 'mock'], {
    windowsHide: true, timeout: 15000, env: { ...process.env, COUNCIL_DEPTH: '0' },
  });
}

test('doctor identifies the actual runtime without creating collaboration state', async t => {
  const root = workspace(t);
  const { stdout } = await doctor(root);
  const result = JSON.parse(stdout);
  assert.equal(result.runtime.supported, true);
  assert.equal(result.runtime.node.executable, process.execPath);
  assert.equal(result.runtime.node.version, process.versions.node);
  assert.equal(result.runtime.sqlite.version, process.versions.sqlite);
  assert.deepEqual(result.recoveredRuns, []);
  assert.ok(result.agents.length > 0);
  assert.equal(existsSync(join(root, '.council')), false);
});

test('doctor diagnoses unsafe SQLite with a nonzero exit and leaves existing state untouched', async t => {
  const root = workspace(t), state = join(root, '.council');
  mkdirSync(state);
  for (const name of ['state.db', 'state.db-wal', 'state.db-shm']) writeFileSync(join(state, name), `retained-${name}`);
  const snapshot = () => Object.fromEntries(readdirSync(state).map(name => [name, readFileSync(join(state, name))]));
  const before = snapshot();
  await assert.rejects(doctor(root, "Object.defineProperty(process.versions, 'sqlite', {value:'3.51.2'});"), error => {
    assert.equal(error.code, 1);
    const result = JSON.parse(error.stdout);
    assert.equal(result.runtime.supported, false);
    assert.equal(result.runtime.sqlite.supported, false);
    assert.equal(result.runtime.sqlite.version, '3.51.2');
    assert.equal(result.runtime.sqlite.minimum, '3.51.3');
    return true;
  });
  assert.deepEqual(snapshot(), before);
});

test('doctor reports an unsupported Node runtime before initializing state', async t => {
  const root = workspace(t);
  await assert.rejects(doctor(root, "Object.defineProperty(process.versions, 'node', {value:'24.14.1'});"), error => {
    assert.equal(error.code, 1);
    const result = JSON.parse(error.stdout);
    assert.equal(result.runtime.node.supported, false);
    assert.equal(result.runtime.sqlite.supported, true);
    assert.equal(result.runtime.node.minimum, '24.18.1');
    return true;
  });
  assert.equal(existsSync(join(root, '.council')), false);
});

test('runtime readiness uses the embedded SQLite guard and recognizes supported Node patch releases', () => {
  const descriptors = Object.fromEntries(['node', 'sqlite'].map(key => [key, Object.getOwnPropertyDescriptor(process.versions, key)]));
  try {
    for (const [node, sqlite, supported] of [['24.18.0', '3.51.3', false], ['24.18.1', '3.51.3', true],
      ['24.22.1', '3.51.2', false], ['26.0.0', '3.53.4', true], ['24.18.1', 'unknown', false]]) {
      Object.defineProperty(process.versions, 'node', { ...descriptors.node, value: node });
      Object.defineProperty(process.versions, 'sqlite', { ...descriptors.sqlite, value: sqlite });
      assert.equal(runtimeReadiness().supported, supported, `${node}/${sqlite}`);
    }
  } finally { for (const [key, descriptor] of Object.entries(descriptors)) Object.defineProperty(process.versions, key, descriptor); }
});
