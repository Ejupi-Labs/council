import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import { syncBuiltinESMExports } from 'node:module';
import { setImmediate as nextTurn } from 'node:timers/promises';
import { join } from 'node:path';
import { AcpFiles } from '../dist/acp-files.js';
import { metadataLookup, metadataPressure } from '../dist/metadata.js';
import { resolveLaunch } from '../dist/process.js';

test('cancelled resolver lookups retain their global OS budget until real settlement and then recover', async () => {
  const originalStat = fs.stat, originalRealpath = fs.realpath;
  const delayed = [];
  let statCalls = 0, realpathCalls = 0;
  try {
    fs.stat = () => { statCalls++; return new Promise((_resolve, reject) => { delayed.push(reject); }); };
    fs.realpath = async (...args) => { realpathCalls++; return originalRealpath(...args); };
    syncBuiltinESMExports();
    const { limit } = metadataPressure();
    for (let index = 0; index < limit; index++) {
      const controller = new AbortController();
      const work = resolveLaunch(process.execPath, [], process.cwd(), process.env, controller.signal);
      const cancelled = assert.rejects(work, error => error.name === 'AbortError');
      controller.abort();
      await cancelled;
    }
    assert.deepEqual(metadataPressure(), { pending: limit, limit });
    assert.equal(statCalls, limit);
    await assert.rejects(resolveLaunch(process.execPath, [], process.cwd()), /metadata lookup limit/);
    assert.equal(statCalls, limit, 'A rejected capacity check must not issue another stat.');
    const files = new AcpFiles(process.cwd(), 'read');
    await assert.rejects(files.read({ path: join(process.cwd(), 'package.json') }), /metadata lookup limit/);
    assert.equal(realpathCalls, 0, 'ACP roots share the same process-wide lookup budget.');
    delayed.shift()(new Error('Late OS lookup rejection after cancellation.'));
    await nextTurn();
    assert.equal(metadataPressure().pending, limit - 1);
    fs.stat = originalStat; syncBuiltinESMExports();
    const recovered = await resolveLaunch(process.execPath, ['--version'], process.cwd());
    assert.equal(recovered.command, process.execPath);
    assert.equal(metadataPressure().pending, limit - 1);
  } finally {
    for (const reject of delayed) reject(new Error('Controlled delayed lookup cleanup.'));
    fs.stat = originalStat; fs.realpath = originalRealpath; syncBuiltinESMExports();
    await nextTurn();
  }
  assert.equal(metadataPressure().pending, 0);
});

test('pre-cancelled and synchronously failing metadata operations do not consume capacity', async () => {
  const controller = new AbortController(); controller.abort();
  let calls = 0;
  await assert.rejects(metadataLookup(() => { calls++; return Promise.resolve(); }, controller.signal), error => error.name === 'AbortError');
  assert.equal(calls, 0);
  await assert.rejects(metadataLookup(() => { throw new Error('Injected synchronous lookup failure.'); }), /synchronous lookup/);
  assert.equal(metadataPressure().pending, 0);
});

test('cancellation inside a lookup callback handles its later rejection and releases the OS slot', async () => {
  const controller = new AbortController();
  const work = metadataLookup(() => {
    controller.abort();
    return Promise.reject(new Error('Late callback rejection.'));
  }, controller.signal);
  await assert.rejects(work, error => error.name === 'AbortError');
  await nextTurn();
  assert.equal(metadataPressure().pending, 0);
});
