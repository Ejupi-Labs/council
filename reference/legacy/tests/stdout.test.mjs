import test from 'node:test';
import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { fileURLToPath, pathToFileURL } from 'node:url';

const rpc = pathToFileURL(fileURLToPath(new URL('../dist/rpc.js', import.meta.url))).href;
const pause = ms => new Promise(resolve => setTimeout(resolve, ms));

function blockedClient() {
  const source = `
import { CouncilMcpServer } from ${JSON.stringify(rpc)};
let calls = 0;
const server = new CouncilMcpServer(async () => { process.send?.({ shutdown: true, calls }); });
server.registerTool({ name: 'large', description: 'Offline output-pressure fixture',
  inputSchema: { type: 'object', properties: {}, additionalProperties: false },
  annotations: { readOnlyHint: true, destructiveHint: false, openWorldHint: false } },
  () => { calls++; return { payload: 'x'.repeat(1024 * 1024) }; });
await server.connect();
process.send?.({ ready: true });
`;
  const child = spawn(process.execPath, ['--input-type=module', '-e', source], {
    shell: false, windowsHide: true, stdio: ['pipe', 'pipe', 'pipe', 'ipc'],
  });
  child.stderr.resume();
  // Deliberately never consume stdout. A real Windows stdio write can block here.
  let ready = false, closed = false, shutdown;
  child.on('message', value => { if (value.ready) ready = true; if (value.shutdown) shutdown = value; });
  child.once('close', () => { closed = true; });
  const send = value => child.stdin.write(JSON.stringify({ jsonrpc: '2.0', ...value }) + '\n');
  return { child, ready: () => ready, closed: () => closed, shutdown: () => shutdown, send };
}

async function until(predicate, timeoutMs) {
  const deadline = Date.now() + timeoutMs;
  while (!predicate() && Date.now() < deadline) await pause(10);
  return !!predicate();
}

async function startPressure(client) {
  assert.equal(await until(client.ready, 3000), true, 'The local MCP fixture must become ready.');
  client.send({ id: 1, method: 'initialize', params: { protocolVersion: '2025-11-25', clientInfo: { name: 'blocked-client', version: '1' }, capabilities: {} } });
  await pause(50);
  client.send({ id: 2, method: 'tools/call', params: { name: 'large', arguments: {} } });
  await pause(200);
}

function cleanup(client) {
  if (!client.closed()) client.child.kill('SIGKILL');
  client.child.stdin.destroy(); client.child.stdout.destroy();
  if (client.child.connected) client.child.disconnect();
}

test('EOF shuts down a real stdio server while its client refuses to read large stdout', { timeout: 10000 }, async () => {
  const client = blockedClient();
  try {
    await startPressure(client); client.child.stdin.end();
    assert.equal(await until(() => client.shutdown(), 3000), true, 'Blocked stdout must not prevent service shutdown on EOF.');
    assert.equal(client.shutdown().calls, 1, 'Only the offline fixture handler may run.');
    assert.equal(await until(client.closed, 3000), true, 'Owned output resources must close after service shutdown.');
  } finally { cleanup(client); }
});

test('abrupt parent death releases all inherited stdout writers, including a blocked output pump', { timeout: 10000 }, async () => {
  const client = blockedClient();
  try {
    await startPressure(client); client.child.kill('SIGKILL');
    assert.equal(await until(client.closed, 4000), true, 'No output helper may keep the dead server stdout pipe open.');
  } finally { cleanup(client); }
});
