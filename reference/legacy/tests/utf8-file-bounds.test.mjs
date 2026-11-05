import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import { syncBuiltinESMExports } from 'node:module';
import { tmpdir } from 'node:os';
import { basename, dirname, join } from 'node:path';
import { readJsonFile } from '../dist/config.js';
import { readUtf8File } from '../dist/utf8-file.js';
function fixture(t) {
  const parent = fs.realpathSync.native(tmpdir()), root = fs.mkdtempSync(join(parent, 'council-file-bounds-'));
  t.after(() => { assert.equal(dirname(fs.realpathSync.native(root)), parent); assert.ok(basename(root).startsWith('council-file-bounds-')); fs.rmSync(root, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 }); });
  return join(root, 'input.json');
}
function observed(filename, contents, operation, { afterRead, afterStat, ioFailure, nativeReadLimit } = {}) {
  fs.writeFileSync(filename, contents);
  const original = Object.fromEntries(['openSync', 'closeSync', 'readSync', 'statSync', 'fstatSync'].map(key => [key, fs[key]])), parse = JSON.parse;
  const live = new Set(), closed = [], requests = []; let opens = 0, closes = 0, bytesRead = 0, parseCalls = 0, reads = 0, stats = 0;
  fs.openSync = function (path, flags, ...args) { const fd = original.openSync.call(this, path, flags, ...args); if (String(path) === filename && flags === 'r') { opens++; live.add(fd); } return fd; };
  fs.closeSync = function (fd) { const value = original.closeSync.call(this, fd); if (live.delete(fd)) { closes++; closed.push(fd); } return value; };
  fs.readSync = function (fd, buffer, offset, length, position) {
    if (!live.has(fd)) return original.readSync.call(this, fd, buffer, offset, length, position);
    requests.push(length); if (ioFailure) throw ioFailure;
    const count = original.readSync.call(this, fd, buffer, offset, nativeReadLimit === undefined ? length : Math.min(length, nativeReadLimit), position); bytesRead += count; afterRead?.({ reads: ++reads, count, fd }); return count;
  };
  fs.statSync = function (path, ...args) { const result = original.statSync.call(this, path, ...args); if (String(path) === filename) afterStat?.({ count: ++stats, result }); return result; };
  JSON.parse = function (...args) { parseCalls++; return parse.apply(this, args); };
  syncBuiltinESMExports(); let value, error;
  try { value = operation(); } catch (failure) { error = failure; }
  finally { Object.assign(fs, original); JSON.parse = parse; syncBuiltinESMExports(); }
  assert.equal(live.size, 0); assert.equal(opens, closes);
  for (const fd of closed) assert.throws(() => original.fstatSync(fd), failure => failure.code === 'EBADF');
  return { value, error, opens, closes, bytesRead, parseCalls, maximumRequest: Math.max(0, ...requests) };
}
test('finite file limits count exact BOM and Unicode bytes without rereading or truncation', t => {
  const filename = fixture(t), text = 'café 東京 🧪 with interior \uFEFF and intentional �.', document = JSON.stringify({ text }), bytes = Buffer.from('\uFEFF' + document);
  const exact = observed(filename, bytes, () => readJsonFile(filename, bytes.length));
  assert.equal(exact.error, undefined); assert.deepEqual(exact.value, { text }); assert.equal(exact.bytesRead, bytes.length); assert.equal(exact.opens, 1);
  const overflow = observed(filename, bytes, () => readUtf8File(filename, bytes.length - 1)); assert.match(overflow.error.message, /byte limit/); assert.equal(overflow.bytesRead, 0); assert.equal(overflow.opens, 1);
});
test('a real file change between stat and read cannot bypass the configured JSON byte limit', t => {
  const filename = fixture(t), limit = 1048576;
  const result = observed(filename, '{}', () => readJsonFile(filename, limit), { afterStat: ({ count, result }) => { if (count === 1) { assert.equal(result.size, 2); fs.writeFileSync(filename, JSON.stringify({ payload: 'x'.repeat(1100000) })); } } });
  assert.match(result.error.message, /byte limit/); assert.equal(result.parseCalls, 0); assert.equal(result.bytesRead, 0); assert.equal(result.opens, 1);
});
test('growth of the same open file stops at limit plus one before UTF-8 or JSON interpretation', t => {
  const filename = fixture(t), limit = 65540;
  const result = observed(filename, '{}', () => readJsonFile(filename, limit), { afterRead: ({ reads }) => { if (reads === 1) fs.appendFileSync(filename, ' '.repeat(limit - 1)); } });
  assert.match(result.error.message, /byte limit/); assert.equal(result.bytesRead, limit + 1); assert.equal(result.parseCalls, 0); assert.ok(result.maximumRequest <= 65536); assert.equal(result.opens, 1);
});
test('UTF-8, JSON and repeated I/O failures close the actual native descriptor and preserve the cause', t => {
  const filename = fixture(t), utf8 = observed(filename, Buffer.from([0xc3, 0x28]), () => readJsonFile(filename, 32));
  assert.match(utf8.error.message, /valid UTF-8/); assert.equal(utf8.parseCalls, 0); assert.equal(utf8.opens, 1);
  const syntax = observed(filename, '{', () => readJsonFile(filename, 32)); assert.match(syntax.error.message, /Invalid JSON/); assert.equal(syntax.opens, 1);
  const fault = new Error('Owned synthetic read failure.');
  for (let index = 0; index < 50; index++) { const result = observed(filename, '{}', () => readJsonFile(filename, 32), { ioFailure: fault }); assert.equal(result.error, fault); assert.equal(result.opens, 1); assert.equal(result.closes, 1); }
});
test('unsafe limits fail before open while huge finite limits allocate bounded chunks', t => {
  const filename = fixture(t);
  for (const limit of [0, -1, 1.5, NaN, Number.MAX_SAFE_INTEGER + 1]) { const result = observed(filename, '{}', () => readUtf8File(filename, limit)); assert.match(result.error.message, /positive integer or Infinity/); assert.equal(result.opens, 0); }
  const huge = observed(filename, '{}', () => readUtf8File(filename, Number.MAX_SAFE_INTEGER)); assert.equal(huge.value, '{}'); assert.equal(huge.maximumRequest, 65536);
});
test('explicit Infinity preserves complete supplied text above the configuration limit', t => {
  const filename = fixture(t), text = 'Complete café 東京 🧪 evidence.\n'.repeat(70000);
  fs.writeFileSync(filename, text); assert.ok(Buffer.byteLength(text) > 1048576); assert.equal(readUtf8File(filename, Infinity), text);
});
test('legal one-byte native reads retain at most the accepted bytes plus one bounded chunk', t => {
  const filename = fixture(t), text = 'Short native reads retain exact café 東京 evidence.', bytes = Buffer.from(JSON.stringify({ text }));
  const concat = Buffer.concat; let retainedCapacity = 0, concatenations = 0;
  Buffer.concat = function (chunks, ...args) {
    concatenations++; retainedCapacity = [...new Set(chunks.map(chunk => chunk.buffer))].reduce((total, backing) => total + backing.byteLength, 0);
    return concat.call(this, chunks, ...args);
  };
  let result;
  try { result = observed(filename, bytes, () => readJsonFile(filename, 65664), { nativeReadLimit: 1 }); }
  finally { Buffer.concat = concat; }
  assert.equal(result.error, undefined); assert.deepEqual(result.value, { text }); assert.equal(result.bytesRead, bytes.length); assert.equal(result.opens, 1);
  assert.equal(concatenations, 1); assert.ok(retainedCapacity <= bytes.length + 65536, `Retained ${retainedCapacity} backing bytes for ${bytes.length} accepted bytes.`);
});
