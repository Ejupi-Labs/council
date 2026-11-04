import assert from 'node:assert/strict';
import { EventEmitter } from 'node:events';
import { basename } from 'node:path';
import { statSync } from 'node:fs';
import test from 'node:test';
import { endurancePlan, runEndurance } from './endurance.mjs';

const defaults = Object.freeze({ COUNCIL_ENDURANCE_OS: 'ubuntu', COUNCIL_ENDURANCE_WORKLOAD: 'storage', COUNCIL_ENDURANCE_DURATION: '60s' });

test('every workflow choice maps to a fixed shell-free Node argument list', () => {
  for (const [os, platform] of [['ubuntu', 'linux'], ['windows', 'win32'], ['macos', 'darwin']]) {
    for (const [workload, script] of [['service', 'soak.mjs'], ['storage', 'storage-soak.mjs'], ['artifacts', 'artifact-soak.mjs'], ['coordination', 'coordination-soak.mjs'], ['rooms', 'room-soak.mjs']]) {
      for (const [duration, milliseconds] of [['60s', 60000], ['5m', 300000], ['10m', 600000], ['30m', 1800000]]) {
        const plan = endurancePlan({ COUNCIL_ENDURANCE_OS: os, COUNCIL_ENDURANCE_WORKLOAD: workload, COUNCIL_ENDURANCE_DURATION: duration }, platform);
        assert.equal(plan.command, process.execPath);
        assert.equal(plan.args[0], '--expose-gc');
        assert.equal(basename(plan.args[1]), script);
        assert.ok(statSync(plan.args[1]).isFile(), `Declared workload must exist: ${workload}`);
        assert.deepEqual(plan.args.slice(2), ['--duration-ms', String(milliseconds), ...(workload === 'service' ? ['--label', 'github-manual'] : [])]);
      }
    }
  }
});

test('missing, inherited-key and shell-like input values launch no child', async () => {
  let launches = 0;
  const spawnChild = () => { launches++; throw new Error('Must not launch.'); };
  for (const name of Object.keys(defaults)) {
    for (const value of [undefined, '', 'constructor', '__proto__', '60s; echo unsafe', '$(echo unsafe)', '300000', '60s\n']) {
      await assert.rejects(runEndurance({ ...defaults, [name]: value }, spawnChild, 'linux'), /Choose/);
    }
  }
  await assert.rejects(runEndurance(defaults, spawnChild, 'win32'), /does not match/);
  assert.equal(launches, 0);
});

test('dispatcher launches exactly one offline child with explicit argv and no shell', async () => {
  let launches = 0;
  const environment = { ...defaults, COUNCIL_BACKEND: 'native', COUNCIL_DEPTH: '9' };
  const child = new EventEmitter();
  const code = await runEndurance(environment, (command, args, options) => {
    launches++;
    assert.equal(command, process.execPath);
    assert.deepEqual(args, endurancePlan(environment, 'linux').args);
    assert.equal(options.shell, false);
    assert.equal(options.stdio, 'inherit');
    assert.equal(options.windowsHide, true);
    assert.equal(options.env.COUNCIL_BACKEND, 'mock');
    assert.equal(options.env.COUNCIL_DEPTH, '0');
    assert.equal(options.cwd, endurancePlan(environment, 'linux').cwd);
    queueMicrotask(() => child.emit('close', 0, null));
    return child;
  }, 'linux');
  assert.equal(code, 0);
  assert.equal(launches, 1);
  assert.equal(environment.COUNCIL_BACKEND, 'native');
  assert.equal(child.listenerCount('error'), 0);
});

test('nonzero exit, signal and spawn failure remain failures', async () => {
  for (const [code, signal, expected] of [[7, null, 7], [null, 'SIGTERM', 1], [null, null, 1]]) {
    const child = new EventEmitter();
    assert.equal(await runEndurance(defaults, () => {
      queueMicrotask(() => child.emit('close', code, signal));
      return child;
    }, 'linux'), expected);
    assert.equal(child.listenerCount('error'), 0);
  }
  const child = new EventEmitter();
  await assert.rejects(runEndurance(defaults, () => {
    queueMicrotask(() => child.emit('error', new Error('Synthetic startup failure.')));
    return child;
  }, 'linux'), /could not start/);
  assert.equal(child.listenerCount('close'), 0);
});
