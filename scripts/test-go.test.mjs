import assert from 'node:assert/strict';
import test from 'node:test';
import { listedCases, caseBatches } from './test-go.mjs';

test('discovered Go test, example and fuzz parents are executed exactly once without broadening selectors', () => {
  const names = listedCases('TestFirst\r\nExample\r\nFuzzInput\r\nTestUnicode東京\r\nok\tmodule/engine\t0.1s\r\n');
  const batches = caseBatches(names, 2);
  assert.deepEqual(batches.flatMap(batch => batch.cases), names);
  for (const batch of batches) {
    const matcher = new RegExp(batch.pattern, 'u');
    for (const name of names) assert.equal(matcher.test(name), batch.cases.includes(name));
    assert.equal(matcher.test('TestFirstExtra'), false);
    assert.equal(matcher.test('UnlistedCase'), false);
  }
  assert.deepEqual(caseBatches(['TestMetacharacter$'], 1)[0].pattern, '^(TestMetacharacter\\$)$');
});

test('missing, duplicate or unrecognized discovery output cannot silently reduce verification', () => {
  for (const output of ['', 'ok\tmodule/engine\t0.1s', 'TestA\nTestA\n', 'TestA\nunexpected output\n']) assert.throws(() => listedCases(output));
  for (const size of [0, -1, 1.5, NaN]) assert.throws(() => caseBatches(['TestA'], size));
  assert.throws(() => caseBatches([]));
  assert.throws(() => caseBatches(['TestA', 'TestA']));
});
