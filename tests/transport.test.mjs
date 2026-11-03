import assert from 'node:assert/strict';
import { finished } from 'node:stream/promises';
import { test } from 'node:test';
import { BoundedMcpInput, MAX_MCP_FRAME_BYTES, MAX_MCP_NODES, MAX_MCP_DEPTH } from '../dist/transport.js';

async function capture(chunks) {
  const stream = new BoundedMcpInput();
  const frames = [];
  stream.on('data', chunk => frames.push(Buffer.from(chunk)));
  const complete = finished(stream);
  for (const chunk of chunks) stream.write(chunk);
  stream.end();
  await complete;
  return frames;
}

test('input frames preserve UTF-8 split at every byte and escaped structural characters', async () => {
  const payload = JSON.stringify({ jsonrpc: '2.0', id: 1, method: 'test', params: { text: '你好 🦉 é [ ] { } " \\ \\"' } }) + '\n';
  const frames = await capture([...Buffer.from(payload)].map(byte => Buffer.from([byte])));
  assert.equal(frames.length, 1);
  assert.equal(frames[0].toString('utf8'), payload);
});

test('multiple frames are independently validated and retain their original bytes', async () => {
  const first = '{ "jsonrpc": "2.0", "id": 1, "method": "one" }\r\n';
  const second = '{"jsonrpc":"2.0","id":2,"method":"two"}\n';
  const frames = await capture([Buffer.from(first + '\n \t\r\n' + second)]);
  assert.deepEqual(frames.map(frame => frame.toString()), [first, second]);
  // The first frame must not alias the fixed buffer reused for the second.
  assert.equal(frames[0].toString(), first);
});

test('no data is forwarded before the complete frame has been validated', async () => {
  const stream = new BoundedMcpInput();
  const frames = [];
  stream.on('data', chunk => frames.push(Buffer.from(chunk)));
  const complete = finished(stream);
  stream.write(Buffer.from('{"value":"private'));
  assert.equal(frames.length, 0);
  stream.write(Buffer.from('"}'));
  assert.equal(frames.length, 0);
  stream.end(Buffer.from('\n'));
  await complete;
  assert.equal(frames.length, 1);
  assert.equal(frames[0].toString(), '{"value":"private"}\n');
});

test('frame byte cap permits the exact limit and rejects complete or unfinished overflow', async () => {
  const skeleton = '{"value":""}';
  const exact = '{"value":"' + 'x'.repeat(MAX_MCP_FRAME_BYTES - Buffer.byteLength(skeleton)) + '"}';
  assert.equal(Buffer.byteLength(exact), MAX_MCP_FRAME_BYTES);
  assert.equal((await capture([Buffer.from(exact + '\n')]))[0].length, MAX_MCP_FRAME_BYTES + 1);
  await assert.rejects(capture([Buffer.from(exact + ' \n')]), /frame exceeds 1048576 bytes/);
  await assert.rejects(capture([Buffer.alloc(MAX_MCP_FRAME_BYTES, 0x20), Buffer.from(' ')]), /frame exceeds 1048576 bytes/);
});

test('JSON graph node cap covers arrays and object properties before forwarding', async () => {
  const exact = JSON.stringify(Array(MAX_MCP_NODES - 1).fill(null)) + '\n';
  assert.equal((await capture([Buffer.from(exact)])).length, 1);
  const tooMany = JSON.stringify(Array(MAX_MCP_NODES).fill(null)) + '\n';
  await assert.rejects(capture([Buffer.from(tooMany)]), /maximum JSON node count/);
  const properties = Object.fromEntries(Array.from({ length: MAX_MCP_NODES }, (_, index) => [`item${index}`, index]));
  await assert.rejects(capture([Buffer.from(JSON.stringify(properties) + '\n')]), /maximum JSON node count/);
});

test('depth cap permits root depth zero through depth 64 and rejects deeper values', async () => {
  const nested = depth => '['.repeat(depth) + '0' + ']'.repeat(depth) + '\n';
  assert.equal((await capture([Buffer.from(nested(MAX_MCP_DEPTH))])).length, 1);
  await assert.rejects(capture([Buffer.from(nested(MAX_MCP_DEPTH + 1))]), /maximum JSON depth/);
  await assert.rejects(capture([Buffer.from(nested(10000))]), /maximum JSON depth/);
});

test('invalid JSON, invalid UTF-8 and unfinished frames fail without payload disclosure', async () => {
  const secret = 'private-provider-secret';
  for (const frame of [`{"${secret}":]\n`, `{ "${secret}": 1 }`, `${secret}\n`]) {
    await assert.rejects(capture([Buffer.from(frame)]), error => {
      assert.match(error.message, /invalid JSON|incomplete frame/);
      assert.ok(!error.message.includes(secret));
      return true;
    });
  }
  await assert.rejects(capture([Buffer.from([0x22, 0xc3, 0x28, 0x22, 0x0a])]), /valid UTF-8/);
  await assert.rejects(capture([Buffer.from('\v\n')]), /invalid JSON/);
  assert.deepEqual(await capture([Buffer.from(' \t\r\n\n \t')]), []);
});

test('a malformed frame is never forwarded and terminates the guard with one controlled error', async () => {
  const stream = new BoundedMcpInput();
  const frames = [];
  let errors = 0;
  stream.on('data', chunk => frames.push(Buffer.from(chunk)));
  stream.on('error', () => { errors++; });
  const complete = finished(stream);
  stream.end(Buffer.from('{"valid":true}\n{"broken":}\n{"later":true}\n'));
  await assert.rejects(complete, /invalid JSON/);
  assert.deepEqual(frames.map(frame => frame.toString()), ['{"valid":true}\n']);
  assert.equal(errors, 1);
  assert.equal(stream.destroyed, true);
});
