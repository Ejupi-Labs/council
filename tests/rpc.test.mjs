import test from 'node:test';
import assert from 'node:assert/strict';
import { PassThrough, Writable } from 'node:stream';
import { setImmediate as nextTurn } from 'node:timers/promises';
import { CouncilMcpServer } from '../dist/rpc.js';
import { objectSchema } from '../dist/schema.js';

const pause = ms => new Promise(resolve => setTimeout(resolve, ms));
const initialize = protocolVersion => ({ protocolVersion, clientInfo: { name: 'raw-test-client', version: '1.0.0' }, capabilities: {} });
const annotations = { readOnlyHint: true, destructiveHint: false, openWorldHint: false };

async function harness(t, setup = () => {}, blocked = false) {
  const input = new PassThrough();
  const frames = [];
  const callbacks = [];
  let pending = '', shutdowns = 0;
  const output = blocked ? new Writable({ highWaterMark: 1, write(_data, _encoding, callback) { callbacks.push(callback); } }) : new PassThrough();
  if (!blocked) output.on('data', chunk => {
    pending += chunk.toString(); let end;
    while ((end = pending.indexOf('\n')) >= 0) { frames.push(JSON.parse(pending.slice(0, end))); pending = pending.slice(end + 1); }
  });
  const server = new CouncilMcpServer(async () => { shutdowns++; });
  setup(server);
  await server.connect(input, output);
  t.after(async () => { await server.close(); for (const callback of callbacks.splice(0)) callback(); input.destroy(); output.destroy(); });
  const send = (id, method, params) => input.write(JSON.stringify({ jsonrpc: '2.0', ...(id === undefined ? {} : { id }), method, ...(params === undefined ? {} : { params }) }) + '\n');
  const response = async id => {
    const until = Date.now() + 2000;
    while (Date.now() < until) { const index = frames.findIndex(frame => frame.id === id); if (index >= 0) return frames.splice(index, 1)[0]; await pause(2); }
    throw new Error(`No raw JSON-RPC response for ${String(id)}`);
  };
  const init = async (version = '2025-11-25') => { send(1, 'initialize', initialize(version)); const result = await response(1); send(undefined, 'notifications/initialized'); return result; };
  return { server, input, output, frames, callbacks, send, response, init, shutdowns: () => shutdowns };
}

test('raw MCP initialization negotiates supported versions and declares only implemented capabilities', async t => {
  for (const version of ['2025-11-25', '2025-06-18', '2025-03-26', '2024-11-05', 'unknown-version']) {
    const wire = await harness(t); const response = await wire.init(version);
    assert.equal(response.result.protocolVersion, version === 'unknown-version' ? '2025-11-25' : version);
    assert.deepEqual(Object.keys(response.result.capabilities).sort(), ['resources', 'tools']);
    assert.equal(response.result.capabilities.tools.listChanged, false); assert.equal(response.result.capabilities.resources.listChanged, false);
  }
});

test('raw MCP permits ping before initialization but blocks operations', async t => {
  const wire = await harness(t);
  wire.send(0, 'ping'); assert.deepEqual((await wire.response(0)).result, {});
  wire.send(2, 'tools/list'); assert.equal((await wire.response(2)).error.code, -32002);
  await wire.init(); wire.send(3, 'tools/list'); assert.deepEqual((await wire.response(3)).result.tools, []);
});

test('invalid client implementation information does not initialize the session', async t => {
  for (const clientInfo of [{}, { name: 1, version: '1.0.0' }, { name: 'client' }, { name: 'client', version: false }]) {
    const wire = await harness(t);
    wire.send(2, 'initialize', { ...initialize('2025-11-25'), clientInfo });
    assert.equal((await wire.response(2)).error?.code, -32602);
    const valid = await wire.init(); assert.equal(valid.result.serverInfo.name, 'council');
  }
});

test('invalid JSON-RPC IDs and unknown methods return bounded errors while valid requests remain usable', async t => {
  const wire = await harness(t); await wire.init();
  for (const id of [null, true, {}, [], 1.5]) {
    wire.input.write(JSON.stringify({ jsonrpc: '2.0', id, method: 'ping' }) + '\n');
    assert.equal((await wire.response(null)).error.code, -32600);
  }
  for (const method of ['unknown-method', 'sampling/createMessage', 'elicitation/create', 'resources/subscribe']) {
    wire.send(method, method); assert.equal((await wire.response(method)).error.code, -32601);
  }
  wire.send('still-usable', 'ping'); assert.deepEqual((await wire.response('still-usable')).result, {});
});

test('tool input failures and null arguments do not call handlers or destroy the MCP session', async t => {
  let calls = 0;
  const wire = await harness(t, server => {
    server.registerTool({ name: 'strict', description: 'fixture', annotations, inputSchema: objectSchema({ value: { type: 'integer' } }, ['value']) }, input => { calls++; return input; });
    server.registerTool({ name: 'empty', description: 'fixture', annotations, inputSchema: objectSchema({}) }, () => { calls++; return {}; });
  });
  await wire.init();
  wire.send(2, 'tools/call', { name: 'strict', arguments: { value: 'bad' } }); assert.equal((await wire.response(2)).result.isError, true);
  wire.send(3, 'tools/call', { name: 'empty', arguments: null });
  const nullResult = await wire.response(3); assert.ok(nullResult.error || nullResult.result?.isError);
  assert.equal(calls, 0);
  wire.send(4, 'tools/call', { name: 'strict', arguments: { value: 3 } }); assert.equal((await wire.response(4)).result.structuredContent.value, 3);
  assert.equal(calls, 1); wire.send(5, 'ping'); assert.deepEqual((await wire.response(5)).result, {});
});

test('cancellation aborts a bounded wait, emits no cancelled response, and preserves later requests', async t => {
  let waiting = false, aborted = false;
  const wire = await harness(t, server => server.registerTool({ name: 'wait', description: 'fixture', annotations, inputSchema: objectSchema({}) }, (_input, signal) => new Promise(resolve => {
    waiting = true; signal.addEventListener('abort', () => { aborted = true; resolve({ cancelled: true }); }, { once: true });
  })));
  await wire.init(); wire.send(2, 'tools/call', { name: 'wait' }); await nextTurn(); assert.equal(waiting, true);
  wire.send(undefined, 'notifications/cancelled', { requestId: 999 });
  wire.send(undefined, 'notifications/cancelled', { requestId: 2 });
  wire.send(3, 'ping'); assert.deepEqual((await wire.response(3)).result, {});
  assert.equal(aborted, true); await nextTurn(); assert.equal(wire.frames.some(frame => frame.id === 2), false);
});

test('duplicate in-flight IDs cannot execute a second tool handler', async t => {
  let calls = 0, release;
  const wire = await harness(t, server => server.registerTool({ name: 'wait', description: 'fixture', annotations, inputSchema: objectSchema({}) }, () => { calls++; return new Promise(resolve => { release = resolve; }); }));
  await wire.init(); wire.send(2, 'tools/call', { name: 'wait' }); await nextTurn(); wire.send(2, 'tools/call', { name: 'wait' });
  assert.equal((await wire.response(2)).error.code, -32600); assert.equal(calls, 1); release({ finished: true });
  assert.equal((await wire.response(2)).result.structuredContent.finished, true);
});

test('EOF cancels pending work without starting unrelated handlers', async t => {
  let aborted = false, calls = 0;
  const wire = await harness(t, server => server.registerTool({ name: 'wait', description: 'fixture', annotations, inputSchema: objectSchema({}) }, (_input, signal) => new Promise(resolve => {
    calls++; signal.addEventListener('abort', () => { aborted = true; resolve({ cancelled: true }); }, { once: true });
  })));
  await wire.init(); wire.send(2, 'tools/call', { name: 'wait' }); await nextTurn(); wire.input.end();
  const until = Date.now() + 1000; while (!wire.shutdowns() && Date.now() < until) await pause(2);
  assert.equal(wire.shutdowns(), 1); assert.equal(aborted, true); assert.equal(calls, 1);
});

test('malformed JSON and incomplete EOF fail closed without executing tools', async t => {
  const previousExitCode = process.exitCode;
  try {
    for (const input of ['{broken}\n', '{"jsonrpc":"2.0"']) {
      let calls = 0;
      const wire = await harness(t, server => server.registerTool({ name: 'never', description: 'fixture', annotations, inputSchema: objectSchema({}) }, () => { calls++; return {}; }));
      wire.input.end(input);
      const until = Date.now() + 1000; while (!wire.shutdowns() && Date.now() < until) await pause(2);
      assert.equal(wire.shutdowns(), 1); assert.equal(calls, 0); await wire.server.close();
    }
  } finally { process.exitCode = previousExitCode; }
});

test('a stalled output client is closed at the 128-frame queue bound', async t => {
  const wire = await harness(t, () => {}, true);
  wire.send(1, 'initialize', initialize('2025-11-25'));
  for (let id = 2; id < 300; id++) wire.send(id, 'ping');
  const until = Date.now() + 1000; while (!wire.shutdowns() && Date.now() < until) await pause(2);
  assert.equal(wire.shutdowns(), 1);
  assert.ok(wire.server.queuedFrames <= 128); assert.ok(wire.server.queuedBytes <= 32 * 1024 * 1024);
});

test('a stalled output client is closed at the 32-MiB queue byte bound', async t => {
  const payload = 'x'.repeat(1024 * 1024);
  const wire = await harness(t, server => server.registerTool({ name: 'large', description: 'fixture', annotations, inputSchema: objectSchema({}) }, () => ({ payload })), true);
  wire.send(1, 'initialize', initialize('2025-11-25'));
  await nextTurn();
  for (let id = 2; id < 26; id++) wire.send(id, 'tools/call', { name: 'large' });
  const until = Date.now() + 2000; while (!wire.shutdowns() && Date.now() < until) await pause(2);
  assert.equal(wire.shutdowns(), 1); assert.ok(wire.server.queuedBytes <= 32 * 1024 * 1024);
});
