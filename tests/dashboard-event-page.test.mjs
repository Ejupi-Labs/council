import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { basename, dirname, join, resolve } from 'node:path';
import { CouncilService } from '../dist/service.js';
import { loadConfig } from '../dist/config.js';
import { serveDashboard } from '../dist/dashboard.js';

async function fixture(t) {
  const root = mkdtempSync(join(tmpdir(), 'council-event-http-')); let calls = 0;
  const service = new CouncilService(loadConfig(root, { backend: 'mock', agents: { worker: { enabled: true, profile: 'codex', model: 'offline-fixture' } } }), {
    doctor: async () => [], run: async request => { calls++; return { agent: request.agent, status: 'completed', output: 'Verified offline.', durationMs: 1 }; },
  });
  const run = await service.run({ mode: 'delegate', task: 'Retain complete offline evidence.', agents: ['worker'], permission: 'read' });
  const dashboard = await serveDashboard(service, service.rooms);
  t.after(async () => {
    await dashboard.close(); await service.close();
    assert.equal(dirname(resolve(root)), resolve(tmpdir())); assert.ok(basename(root).startsWith('council-event-http-'));
    rmSync(root, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 });
  });
  const url = new URL(dashboard.url), token = new URLSearchParams(url.hash.slice(1)).get('token');
  return { service, run, calls: () => calls, get: (query = '', options = {}, id = run.id) => fetch(url.origin + '/api/runs/' + id + '/events' + query, {
    ...options, headers: { Authorization: 'Bearer ' + token, ...options.headers },
  }) };
}

test('event HTTP pages authenticate before reading and make no extra model calls', async t => {
  const wire = await fixture(t); let reads = 0;
  const page = wire.service.eventPage.bind(wire.service);
  wire.service.eventPage = (...args) => { reads++; return page(...args); };
  assert.equal((await wire.get('', { headers: { Authorization: '' } })).status, 401);
  assert.equal((await wire.get('', { headers: { Origin: 'https://example.com' } })).status, 403);
  assert.equal((await wire.get('', { method: 'POST' })).status, 405); assert.equal(reads, 0);
  const response = await wire.get('?limit=2'); assert.equal(response.status, 200); assert.equal(response.headers.get('cache-control'), 'no-store');
  const body = await response.json(); assert.equal(body.events.length, 2); assert.equal(body.hasMore, true);
  assert.equal(body.nextAfter, body.events.at(-1).sequence); assert.equal(body.nextCursor, body.nextAfter);
  assert.equal(reads, 1); assert.equal(wire.calls(), 1);
});

test('event HTTP pages preserve complete large payloads and exact event types', async t => {
  const wire = await fixture(t), type = ' fixture.event ', data = { exact: '</script>' + 'Unabridged evidence. '.repeat(90000), settings: { model: 'offline-fixture', effort: 'low' } };
  const stored = wire.service.store.appendEvent(wire.run.id, type, data);
  const response = await wire.get('?type=' + encodeURIComponent(type)); assert.equal(response.status, 200);
  const page = await response.json(); assert.deepEqual(page.events, [stored]); assert.equal(page.hasMore, false);
  assert.equal(page.nextCursor, null); assert.equal(page.nextAfter, stored.sequence); assert.equal(wire.calls(), 1);
});

test('filtered event HTTP pages retain their cursor across empty reads and later appends', async t => {
  const wire = await fixture(t), type = 'fixture.selected';
  const first = wire.service.store.appendEvent(wire.run.id, type, { ordinal: 1 });
  wire.service.store.appendEvent(wire.run.id, 'fixture.other', { ordinal: 2 });
  const second = wire.service.store.appendEvent(wire.run.id, type, { ordinal: 3 });
  const page1 = await (await wire.get('?type=' + type + '&limit=1')).json(); assert.deepEqual(page1.events, [first]); assert.equal(page1.hasMore, true);
  const page2 = await (await wire.get('?type=' + type + '&limit=1&after=' + page1.nextAfter)).json(); assert.deepEqual(page2.events, [second]); assert.equal(page2.hasMore, false);
  const empty = await (await wire.get('?type=' + type + '&after=' + page2.nextAfter)).json(); assert.deepEqual(empty.events, []); assert.equal(empty.nextAfter, page2.nextAfter);
  const appended = wire.service.store.appendEvent(wire.run.id, type, { ordinal: 4 });
  assert.deepEqual((await (await wire.get('?type=' + type + '&after=' + empty.nextAfter)).json()).events, [appended]);
});

test('event HTTP bounds reject ambiguity and malformed filters without invoking page reads', async t => {
  const wire = await fixture(t); let reads = 0;
  wire.service.eventPage = () => { reads++; assert.fail('Invalid page input must not invoke the service.'); };
  for (const query of ['?after=', '?after=-1', '?after=1e2', '?after=1.5', '?after=01', '?after=9007199254740992', '?limit=', '?limit=1&limit=2', '?after=0&after=1', '?type=a&type=b', '?unused=true']) {
    assert.equal((await wire.get(query)).status, 400, query);
  }
  assert.equal(reads, 0); assert.equal(wire.calls(), 1);
});

test('event HTTP validation rejects invalid page ranges and unsupported type encodings', async t => {
  const wire = await fixture(t);
  for (const query of ['?limit=0', '?limit=101', '?type=', '?type=%00', '?type=%20%20', '?type=' + encodeURIComponent('é'.repeat(65))]) {
    assert.equal((await wire.get(query)).status, 400, query);
  }
});

test('missing or pruned workflows are explicit HTTP failures rather than empty successful histories', async t => {
  const wire = await fixture(t);
  const response = await wire.get('', {}, 'missing'); assert.equal(response.status, 404);
  const failure = await response.json(); assert.equal(failure.code, 'COUNCIL_RUN_EVENTS_UNAVAILABLE'); assert.match(failure.error, /not.*available|not.*found|prun/i);
  assert.equal(wire.calls(), 1);
});
