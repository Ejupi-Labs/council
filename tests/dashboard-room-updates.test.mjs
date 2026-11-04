import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, realpathSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { setTimeout as delay } from 'node:timers/promises';
import { CouncilService, loadConfig } from '../dist/index.js';

async function fixture(t) {
  const root = mkdtempSync(join(realpathSync.native(tmpdir()), 'council-room-updates-'));
  const service = new CouncilService(loadConfig(root, { backend: 'mock' }));
  const peers = [];
  t.after(async () => { for (const peer of peers) await peer.close(); await service.close(); rmSync(root, { recursive: true, force: true }); });
  return { root, service, peers };
}

async function updates(t, dashboard) {
  const url = new URL(dashboard.url), token = new URLSearchParams(url.hash.slice(1)).get('token');
  const controller = new AbortController();
  const response = await fetch(new URL('/api/events', url), { headers: { Authorization: `Bearer ${token}` }, signal: AbortSignal.any([controller.signal, AbortSignal.timeout(10000)]) });
  assert.equal(response.status, 200);
  const reader = response.body.getReader(), decoder = new TextDecoder();
  let buffer = '';
  t.after(async () => { controller.abort(); await reader.cancel().catch(() => {}); });
  return {
    get: path => fetch(new URL(path, url), { headers: { Authorization: `Bearer ${token}` } }).then(value => value.json()),
    async nextRooms() {
      for (;;) {
        let end;
        while ((end = buffer.indexOf('\n\n')) !== -1) {
          const frame = buffer.slice(0, end); buffer = buffer.slice(end + 2);
          if (frame.startsWith('event: rooms')) return frame;
        }
        const part = await reader.read();
        assert.equal(part.done, false, 'A room update must not close its event stream.');
        buffer += decoder.decode(part.value, { stream: true });
      }
    },
  };
}

test('large legitimate room metadata uses a bounded refresh event and keeps the stream connected', { timeout: 20000 }, async t => {
  const { service } = await fixture(t);
  let first;
  for (let index = 0; index < 400; index++) {
    const room = service.rooms.create({ title: `Room ${index} ` + '界'.repeat(180), brief: 'Offline transport fixture.', members: [{ agent: 'codex', role: 'lead' }] });
    first ??= room;
  }
  assert(Buffer.byteLength(JSON.stringify(service.rooms.list())) > 256 * 1024);
  const wire = await updates(t, await service.openDashboard());
  service.rooms.postMessage(first.id, { author: 'human', body: 'This change must remain observable.' });
  const frame = await wire.nextRooms();
  assert.match(frame, /^event: rooms-refresh\n/);
  assert(Buffer.byteLength(frame) < 1024);
  const listed = await wire.get('/api/rooms');
  assert.equal(listed.find(room => room.id === first.id).revision, service.rooms.metadata(first.id).revision);
  service.rooms.postMessage(first.id, { author: 'human', body: 'The same stream still receives later changes.' });
  assert.match(await wire.nextRooms(), /^event: rooms-refresh\n/);
  assert.equal(service.health().state.counts.runs, 0);
});

test('idle update clients do not rescan rooms and foreign commits still publish fresh summaries', { timeout: 10000 }, async t => {
  const { root, service, peers } = await fixture(t);
  const room = service.rooms.create({ title: 'Observed room', brief: 'Offline metadata fixture.', members: [{ agent: 'codex', role: 'lead' }] });
  let lists = 0;
  const original = service.rooms.list.bind(service.rooms);
  service.rooms.list = (...args) => { lists++; return original(...args); };
  const wire = await updates(t, await service.openDashboard());
  const before = lists;
  await delay(1700);
  assert.equal(lists, before, 'Unchanged room state should not rebuild summary arrays on every poll.');
  const peer = new CouncilService(loadConfig(root, { backend: 'mock' })); peers.push(peer);
  peer.rooms.postMessage(room.id, { author: 'other-host', body: 'A foreign commit must invalidate the observation token.' });
  const frame = await wire.nextRooms();
  assert.match(frame, /^event: rooms\n/);
  const data = JSON.parse(frame.slice('event: rooms\ndata: '.length));
  assert.equal(data.find(value => value.id === room.id).revision, peer.rooms.metadata(room.id).revision);
  assert.equal(lists, before + 1);
});
