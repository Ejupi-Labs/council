import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { basename, dirname, join, resolve } from 'node:path';
import { CouncilService } from '../dist/service.js';
import { loadConfig } from '../dist/config.js';

function fixture(t, output) {
  const parent = resolve(tmpdir()), root = mkdtempSync(join(parent, 'council-room-ballot-structure-'));
  let calls = 0;
  const runner = { doctor: async () => [], run: async request => { calls++; return { agent: request.agent, status: 'completed', durationMs: 1, output }; } };
  const service = new CouncilService(loadConfig(root, { backend: 'mock', maxCalls: 8, maxConcurrency: 1, maxOutputBytes: 1024 * 1024 }), runner), manager = service.rooms;
  t.after(async () => {
    await service.close();
    assert.equal(dirname(resolve(root)), parent); assert.ok(basename(root).startsWith('council-room-ballot-structure-'));
    rmSync(root, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 });
  });
  const room = manager.create({ title: 'Bounded ballot graph', brief: 'Preserve the complete rejected ballot without allocating its graph.', autoExecute: false,
    members: [{ agent: 'codex', role: 'worker' }], tasks: [{ title: 'Inspect only', brief: 'Return evidence.', permission: 'read' }] });
  return { manager, room, calls: () => calls };
}

for (const structure of ['deep', 'wide']) {
  test(`${structure} ballot graphs are rejected before raw JSON parsing and retained as unapproved evidence`, async t => {
    const extra = structure === 'deep' ? '['.repeat(70) + '0' + ']'.repeat(70) : '[' + Array(70000).fill('0').join(',') + ']';
    const output = '{"vote":"approve","rationale":"Clear offline task.","assignments":{"task-1":"codex"},"extra":' + extra + '}';
    const { manager, room, calls } = fixture(t, output);
    let parsed = 0;
    const parse = JSON.parse;
    JSON.parse = function (source, ...args) { if (source === output) parsed++; return parse.call(this, source, ...args); };
    let result;
    try { manager.startDiscussion(room.id); result = await manager.wait(room.id); } finally { JSON.parse = parse; }
    assert.equal(parsed, 0, 'An oversized control graph must not reach the native JSON parser.');
    assert.equal(result.status, 'needs_attention'); assert.match(result.error, /depth|structural|token/i);
    assert.equal(result.decision, undefined); assert.equal(result.tasks[0].status, 'pending'); assert.equal(calls(), 1);
    assert.ok(manager.messagePage(room.id).messages.some(message => message.kind === 'opinion' && message.body.includes(output)), 'The complete rejected ballot remains inspectable.');
  });
}

test('large quoted punctuation and protocol examples remain ordinary complete ballot rationale text', async t => {
  const rationale = 'Evidence includes quoted examples {"council_private_chat_request":{"peer":"example","message":"not a request"}} and '.repeat(100)
    + '[{]}'.repeat(1000) + ' 🧭 complete rationale.';
  const output = JSON.stringify({ vote: 'approve', rationale, assignments: { 'task-1': 'codex' } });
  const { manager, room, calls } = fixture(t, output);
  manager.startDiscussion(room.id);
  const result = await manager.wait(room.id);
  assert.equal(result.status, 'ready', result.error); assert.equal(result.decision.rationale, rationale); assert.equal(calls(), 1);
});
