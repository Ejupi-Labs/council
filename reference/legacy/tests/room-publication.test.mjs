import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { basename, dirname, join, resolve } from 'node:path';
import { DatabaseSync } from 'node:sqlite';
import { CouncilService } from '../dist/service.js';
import { loadConfig } from '../dist/config.js';
import { RoomManager } from '../dist/rooms.js';

function fixture(t) {
  const parent = resolve(tmpdir()), root = mkdtempSync(join(parent, 'council-room-publication-')), calls = [];
  const config = loadConfig(root, { backend: 'mock', agents: { worker: { enabled: true, profile: 'codex' } } });
  const service = new CouncilService(config, { doctor: async () => [], run: async request => {
    calls.push(request);
    const match = /ROOM_BALLOT_DATA_BEGIN\n([^\n]+)\nROOM_BALLOT_DATA_END/.exec(request.prompt);
    const ballot = match ? JSON.parse(match[1]) : undefined;
    return { agent: request.agent, status: 'completed', durationMs: 1, output: ballot
      ? JSON.stringify({ vote: 'approve', rationale: 'Complete offline evidence.', assignments: Object.fromEntries(ballot.tasks.map(task => [task.id, 'worker'])) })
      : 'Verified offline task.' };
  } });
  const manager = service.rooms, peers = [];
  t.after(async () => {
    for (const peer of peers) await peer.close();
    await manager.close(); await service.close();
    assert.equal(dirname(resolve(root)), parent); assert.ok(basename(root).startsWith('council-room-publication-'));
    rmSync(root, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 });
  });
  const create = extra => manager.create({ title: 'Publication lifecycle', brief: 'Complete only the offline fixture.',
    members: [{ agent: 'worker', role: 'lead' }], tasks: [{ title: 'Inspect fixture', brief: 'Check the complete evidence.', permission: 'read' }],
    autoExecute: false, ...extra });
  const durable = id => {
    const db = new DatabaseSync(join(config.stateDir, 'rooms.sqlite'), { readOnly: true });
    try { return db.prepare('SELECT status,owner,owner_pid,document FROM rooms WHERE id=?').get(id); }
    finally { db.close(); }
  };
  return { service, manager, calls, create, durable, peer() { const peer = new RoomManager(service); peers.push(peer); return peer; } };
}

async function prepare(wire, phase) {
  const { manager } = wire, room = wire.create({ autoExecute: phase === 'resume-execution' });
  if (phase === 'discussion') return { id: room.id, status: 'discussing', begin: () => manager.startDiscussion(room.id) };
  if (phase === 'execution') {
    manager.startDiscussion(room.id); assert.equal((await manager.wait(room.id)).status, 'ready');
    return { id: room.id, status: 'running', begin: () => manager.execute(room.id) };
  }
  if (phase === 'resume-discussion') {
    manager.startDiscussion(room.id); manager.pause(room.id);
    assert.equal((await manager.wait(room.id)).checkpoint.phase, 'discussion');
    return { id: room.id, status: 'discussing', begin: () => manager.resume(room.id) };
  }
  let requested = false;
  const unsubscribe = manager.subscribe(value => {
    if (value.id === room.id && value.status === 'running' && !requested) { requested = true; manager.pause(room.id); }
  });
  manager.startDiscussion(room.id);
  const paused = await manager.wait(room.id); unsubscribe();
  assert.equal(paused.status, 'paused'); assert.equal(paused.checkpoint.phase, 'execution');
  return { id: room.id, status: 'running', begin: () => manager.resume(room.id) };
}

for (const phase of ['discussion', 'execution', 'resume-discussion', 'resume-execution']) {
  for (const closer of ['manager', 'service']) test(`${phase} tracks committed admission before a subscriber closes the ${closer}`, async t => {
    const wire = fixture(t), operation = await prepare(wire, phase), beforeCalls = wire.calls.length;
    let closing, publication;
    wire.manager.subscribe(value => {
      if (value.id !== operation.id || value.status !== operation.status || closing) return;
      // An independent connection must see the claim before the notification.
      // This also proves user callbacks are outside the mutation transaction.
      publication = wire.durable(operation.id);
      closing = closer === 'manager' ? wire.manager.close() : wire.service.close();
    });
    operation.begin();
    assert.ok(closing, 'The admitted state was published.');
    assert.equal(publication.status, operation.status); assert.equal(typeof publication.owner, 'string');
    await closing;
    assert.equal(wire.calls.length, beforeCalls, 'Shutdown cancels the tracked job before any additional runner call.');
    const stored = wire.durable(operation.id), room = wire.peer().get(operation.id);
    assert.equal(stored.status, 'cancelled'); assert.equal(stored.owner, null); assert.equal(stored.owner_pid, null);
    assert.equal(room.status, 'cancelled'); assert.equal(room.checkpoint, undefined); assert.equal(room.pauseRequested, undefined);
    assert.ok(room.tasks.every(task => task.status === 'cancelled'));
    assert.equal(room.tasks.some(task => task.runId), false, 'No task workflow was admitted after publication triggered shutdown.');
  });
}
