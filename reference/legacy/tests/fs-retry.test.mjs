import test from 'node:test';
import assert from 'node:assert/strict';
import { performance } from 'node:perf_hooks';
import { retryWindowsSharing } from '../dist/fs-retry.js';

const windows = { skip: process.platform !== 'win32' };

test('Windows sharing violations retry the issued operation and preserve its final result', windows, async () => {
  let attempts = 0;
  const result = await retryWindowsSharing(async () => {
    if (++attempts <= 2) throw Object.assign(new Error('Temporary sharing violation.'), { code: 'EPERM' });
    return 'committed';
  });
  assert.equal(result, 'committed'); assert.equal(attempts, 3);
});

test('persistent Windows permissions fail within the retry window without destructive fallback', { ...windows, timeout: 10_000 }, async () => {
  const failure = Object.assign(new Error('Persistent ACL refusal.'), { code: 'EACCES' });
  let attempts = 0; const started = performance.now();
  await assert.rejects(retryWindowsSharing(async () => { attempts++; throw failure; }), error => error === failure);
  assert.ok(attempts >= 2 && attempts <= 14);
  assert.ok(performance.now() - started < 5000, 'A persistent refusal must settle instead of retrying indefinitely.');
});

test('aborting Windows sharing backoff prevents another attempt', windows, async () => {
  const controller = new AbortController(); let attempts = 0;
  const abort = setTimeout(() => controller.abort(), 5);
  try {
    await assert.rejects(retryWindowsSharing(async () => {
      attempts++; throw Object.assign(new Error('Busy destination.'), { code: 'EBUSY' });
    }, controller.signal), error => error.name === 'AbortError');
  } finally { clearTimeout(abort); }
  assert.equal(attempts, 1);
});

test('an abort waits for an already issued filesystem operation to settle', async () => {
  const controller = new AbortController(); let release, entered, settled = false;
  const started = new Promise(resolve => { entered = resolve; });
  const gate = new Promise(resolve => { release = resolve; });
  const running = retryWindowsSharing(async () => { entered(); await gate; return 'issued'; }, controller.signal);
  const rejected = assert.rejects(running, error => error.name === 'AbortError');
  running.finally(() => { settled = true; }).catch(() => {});
  await started; controller.abort(); await Promise.resolve(); assert.equal(settled, false);
  release(); await rejected;
});

test('unrelated failures and non-Windows permission errors are never retried', async () => {
  let attempts = 0;
  const code = process.platform === 'win32' ? 'EEXIST' : 'EPERM';
  const failure = Object.assign(new Error('Preserve the existing destination.'), { code });
  await assert.rejects(retryWindowsSharing(async () => { attempts++; throw failure; }), error => error === failure);
  assert.equal(attempts, 1);
});
