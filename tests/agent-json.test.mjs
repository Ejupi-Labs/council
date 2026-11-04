import test from 'node:test';
import assert from 'node:assert/strict';
import { checkAgentJsonStructure, MAX_AGENT_JSON_DEPTH, MAX_AGENT_JSON_TOKENS } from '../dist/agent-json.js';

test('agent JSON depth permits root zero and rejects very deep graphs before parsing', () => {
  const frame = depth => Buffer.from('['.repeat(depth) + '0' + ']'.repeat(depth));
  assert.doesNotThrow(() => checkAgentJsonStructure(frame(MAX_AGENT_JSON_DEPTH)));
  assert.throws(() => checkAgentJsonStructure(frame(MAX_AGENT_JSON_DEPTH + 1)), /JSON depth 64/);
  assert.throws(() => checkAgentJsonStructure(frame(300_000)), /JSON depth 64/);
  assert.doesNotThrow(() => checkAgentJsonStructure(Buffer.from('['.repeat(MAX_AGENT_JSON_DEPTH + 1) + ']'.repeat(MAX_AGENT_JSON_DEPTH + 1))));
});

test('agent JSON structural-token cap has an exact wide-array boundary', () => {
  const frame = length => Buffer.from('[' + Array(length).fill('0').join(',') + ']');
  assert.doesNotThrow(() => checkAgentJsonStructure(frame(MAX_AGENT_JSON_TOKENS - 1)));
  assert.throws(() => checkAgentJsonStructure(frame(MAX_AGENT_JSON_TOKENS)), /65536 structural tokens/);
  assert.throws(() => checkAgentJsonStructure(Buffer.from(JSON.stringify(Object.fromEntries(Array.from({ length: 32_768 }, (_, index) => [`field${index}`, index]))))), /65536 structural tokens/);
});

test('escaped quotes, backslashes, UTF-8 and bracket-looking tool strings do not increase graph depth', () => {
  const payload = '[{"nested":"\\\"你好 🌍"}]'.repeat(50_000);
  const frame = Buffer.from(JSON.stringify({ type: 'progress', payload }));
  assert.doesNotThrow(() => checkAgentJsonStructure(frame));
  assert.equal(JSON.parse(frame).payload, payload);
});

test('lexical bounds preserve JSON grammar validation as a separate step', () => {
  for (const text of ['{"a":}', '{"value":"unfinished', '[1 2]', '{broken}', ']}']) {
    assert.doesNotThrow(() => checkAgentJsonStructure(Buffer.from(text)));
    assert.throws(() => JSON.parse(text));
  }
});

// Deliberately simple independent byte-state oracle for the large-string search optimization.
function reference(frame) {
  let quoted = false, escaped = false, primitive = false, depth = 0, tokens = 0;
  for (const byte of frame) {
    if (quoted) {
      if (escaped) escaped = false;
      else if (byte === 92) escaped = true;
      else if (byte === 34) quoted = false;
      continue;
    }
    let starts = false;
    if (byte === 34) { starts = true; quoted = true; primitive = false; }
    else if (byte === 123 || byte === 91) { starts = true; primitive = false; }
    else if (byte === 125 || byte === 93) { depth--; primitive = false; }
    else if ([58, 44, 32, 9, 13, 10].includes(byte)) primitive = false;
    else if (!primitive) { starts = true; primitive = true; }
    if (starts) {
      if (depth > MAX_AGENT_JSON_DEPTH) return 'depth';
      if (++tokens > MAX_AGENT_JSON_TOKENS) return 'tokens';
      if (byte === 123 || byte === 91) depth++;
    }
  }
  return 'ok';
}
function result(frame) {
  try { checkAgentJsonStructure(frame); return 'ok'; }
  catch (error) { return /depth/.test(error.message) ? 'depth' : /tokens/.test(error.message) ? 'tokens' : error.message; }
}

test('large-frame fast path agrees with the byte-state oracle for random JSON and malformed escape vectors', () => {
  let seed = 0x8a5cd789;
  const next = size => ((seed = (Math.imul(seed, 1664525) + 1013904223) >>> 0) % size);
  const strings = ['plain', '[]{}', '\\', '"', '\\"', '你好 🌍', '\0', '\ud800', '\n\r\t'];
  const padding = 'p'.repeat(16 * 1024);
  const value = depth => {
    const kind = next(depth > 4 ? 4 : 6);
    if (kind === 0) return null;
    if (kind === 1) return !!next(2);
    if (kind === 2) return next(1000) - 500;
    if (kind === 3) return strings[next(strings.length)].repeat(next(6));
    if (kind === 4) return Array.from({ length: next(5) }, () => value(depth + 1));
    return Object.fromEntries(Array.from({ length: next(5) }, (_, index) => [strings[next(strings.length)] + index, value(depth + 1)]));
  };
  for (let index = 0; index < 1000; index++) {
    const frame = Buffer.from(JSON.stringify({ padding, value: value(0) }));
    assert.equal(result(frame), reference(frame));
    const raw = Array.from({ length: 20 }, () => strings[next(strings.length)]).join('');
    const malformed = Buffer.from('{"padding":' + JSON.stringify(padding) + ',"value":"' + raw + '","tail":' + '['.repeat(next(100)) + '0}');
    assert.equal(result(malformed), reference(malformed));
    assert.throws(() => JSON.parse(malformed));
  }
});

test('large-frame quote skipping preserves backslash parity and exact depth/token boundaries', () => {
  const padding = ' '.repeat(16 * 1024);
  for (const count of [0, 1, 2, 3, 4, 7, 8, 31, 32, 255, 256]) {
    const frame = Buffer.from(padding + '{"value":"prefix' + '\\'.repeat(count) + '"' + '['.repeat(100) + '0' + ']'.repeat(100) + '"}');
    assert.equal(result(frame), reference(frame), `Backslash parity ${count}`);
  }
  for (const depth of [MAX_AGENT_JSON_DEPTH, MAX_AGENT_JSON_DEPTH + 1, 300_000]) {
    const frame = Buffer.from(padding + '['.repeat(depth) + '0' + ']'.repeat(depth));
    assert.equal(result(frame), reference(frame));
  }
  for (const count of [MAX_AGENT_JSON_TOKENS - 1, MAX_AGENT_JSON_TOKENS]) {
    const frame = Buffer.from(padding + '[' + Array(count).fill('0').join(',') + ']');
    assert.equal(result(frame), reference(frame));
  }
});
