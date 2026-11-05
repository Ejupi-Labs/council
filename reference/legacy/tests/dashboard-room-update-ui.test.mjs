import test from 'node:test';
import assert from 'node:assert/strict';
import { createContext, runInContext } from 'node:vm';
import { dashboardHtml } from '../dist/dashboard-ui.js';

function deferred() { let resolve; const promise = new Promise(accept => { resolve = accept; }); return { promise, resolve }; }
const list = revision => [{ id: 'room', title: 'Observed room', revision, status: 'idle' }];
const script = /<script nonce="[^"]+">([\s\S]*?)<\/script>/.exec(dashboardHtml('room-update-ui'))[1];
const source = ['async function refreshList(', 'async function stream()'].map(prefix => {
  const line = script.split(/\r?\n/).find(value => value.startsWith(prefix));
  assert(line, `Missing shipped browser function ${prefix}`); return line;
}).join('\n');

function harness(handler, frame = '') {
  const rendered = [], loaded = [], requests = [];
  let reads = 0, cancelled = 0, released = 0;
  const reader = { async read() { return reads++ === 0 && frame ? { value: new TextEncoder().encode(frame), done: false } : { done: true }; }, async cancel() { cancelled++; }, releaseLock() { released++; } };
  const context = createContext({ Promise, AbortController, TextDecoder,
    api: async (path, body, signal) => { requests.push({ path, signal, wasAborted: signal?.aborted }); return handler(path, requests.length, signal); },
    renderList: value => { rendered.push(JSON.parse(JSON.stringify(value))); if (value[0]?.revision === 5) context.token = ''; },
    loadRoom: async id => { loaded.push(id); context.room = { id, revision: rendered.at(-1)?.[0]?.revision }; },
    connection() {}, error: message => { throw new Error(message); }, renderDelegateOverview() {},
    fetch: async () => ({ ok: true, status: 200, body: { getReader: () => reader } }),
    setTimeout: callback => { callback(); return 0; },
  });
  runInContext("var token='offline',selected='room',room={id:'room',revision:0},roomListGeneration=0,streamController;" + source, context);
  return { context, rendered, loaded, requests, run: code => runInContext(code, context), resources: () => ({ cancelled, released }) };
}

test('out-of-order room-list refreshes preserve the newest accepted list and selected evidence', async () => {
  const first = deferred(), second = deferred();
  const wire = harness((path, request) => request === 1 ? first.promise : second.promise);
  const old = wire.run('refreshList()'), current = wire.run('refreshList()');
  second.resolve(list(3)); await current;
  first.resolve(list(2)); await old;
  assert.deepEqual(wire.rendered.map(value => value[0].revision), [3]);
  assert.deepEqual(wire.loaded, ['room']);
  assert.equal(wire.context.room.revision, 3);
});

test('a complete newer stream update invalidates an older HTTP list response', async () => {
  const pending = deferred();
  const frame = 'event: rooms\ndata: ' + JSON.stringify(list(5)) + '\n\n';
  const wire = harness((path, request) => request === 1 ? pending.promise : list(2), frame);
  const old = wire.run('refreshList()');
  await wire.run('stream()');
  pending.resolve(list(1)); await old;
  assert.deepEqual(wire.rendered.map(value => value[0].revision), [2, 5]);
  assert.equal(wire.context.room.revision, 5);
  assert.deepEqual(wire.resources(), { cancelled: 1, released: 1 });
});

test('a bounded stream refresh notification loads the complete list with its connection signal', async () => {
  const wire = harness((path, request) => list(request === 1 ? 1 : 5), 'event: rooms-refresh\ndata: {}\n\n');
  await wire.run('stream()');
  assert.deepEqual(wire.rendered.map(value => value[0].revision), [1, 5]);
  assert.deepEqual(wire.requests.map(request => request.path), ['/api/rooms', '/api/rooms']);
  assert(wire.requests.every(request => request.signal instanceof AbortSignal));
  assert(wire.requests.every(request => request.wasAborted === false));
  assert.deepEqual(wire.resources(), { cancelled: 1, released: 1 });
});

test('a disconnected stream cannot apply its delayed list refresh to the current view', async () => {
  const pending = deferred(), entered = deferred();
  const wire = harness((path, request) => { if (request === 1) return list(1); entered.resolve(); return pending.promise; }, 'event: rooms-refresh\ndata: {}\n\n');
  const streaming = wire.run('stream()');
  await entered.promise;
  wire.run("streamController.abort();token='';");
  pending.resolve(list(5)); await streaming;
  assert.deepEqual(wire.rendered.map(value => value[0].revision), [1]);
  assert.equal(wire.context.room.revision, 1);
  assert.deepEqual(wire.resources(), { cancelled: 1, released: 1 });
});
