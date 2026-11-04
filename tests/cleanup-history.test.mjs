import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { basename, dirname, join, resolve } from 'node:path';
import { CouncilService } from '../dist/service.js';
import { loadConfig } from '../dist/config.js';
import { GitWorkspaces } from '../dist/workspaces.js';

function fixture(t) {
  const parent = resolve(tmpdir()), root = mkdtempSync(join(parent, 'council-cleanup-history-')), services = []; let calls = 0;
  const create = () => {
    const service = new CouncilService(loadConfig(root, { backend: 'mock' }), { doctor: async () => [], run: async () => { calls++; throw new Error('Maintenance must not invoke an agent.'); } });
    services.push(service); return service;
  };
  const service = create();
  t.after(async () => {
    await Promise.all(services.map(service => service.close())); assert.equal(calls, 0);
    assert.equal(dirname(resolve(root)), parent); assert.ok(basename(root).startsWith('council-cleanup-history-'));
    rmSync(root, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 });
  });
  const artifact = index => ({ agent: 'codex', path: join(root, `fake-retained-worktree-${index}`), patchPath: join(root, `fake-retained-${index}.patch`), changed: false, exportStatus: 'ready' });
  const save = (artifacts = [artifact(1)]) => {
    const run = { id: 'cleanup-history', status: 'completed', input: { mode: 'delegate', agents: ['codex'], permission: 'write', task: 'Offline cleanup state fixture.' },
      createdAt: '2026-01-01T00:00:00.000Z', updatedAt: '2026-01-01T00:00:00.000Z', steps: [], workspaces: artifacts };
    service.store.saveRun(run); return run;
  };
  return { service, create, artifact, save };
}

test('cleanup after a thousand unrelated events remains an idempotent no-op across services', async t => {
  const { service, create, save } = fixture(t), run = save(); let cleanups = 0;
  t.mock.method(GitWorkspaces.prototype, 'cleanup', async artifact => { assert.deepEqual(artifact, run.workspaces[0]); cleanups++; });
  for (let index = 0; index < 1001; index++) service.store.appendEvent(run.id, 'progress', { index, output: 'NONMATCHING_FULL_PAYLOAD' });
  t.mock.method(service.store, 'events', () => { throw new Error('The capped raw history must not determine cleanup state.'); });
  const parse = JSON.parse;
  t.mock.method(JSON, 'parse', (source, ...args) => {
    assert.ok(!source.includes('NONMATCHING_FULL_PAYLOAD'), 'Cleanup must not deserialize unrelated event payloads.');
    return parse(source, ...args);
  });
  const first = await service.cleanup(run.id, { reviewed: true });
  assert.deepEqual(first, { removed: [run.workspaces[0].path], alreadyRemoved: [] });
  const second = await service.cleanup(run.id, { reviewed: true });
  assert.deepEqual(second, { removed: [], alreadyRemoved: [run.workspaces[0].path] });
  const peer = create();
  assert.deepEqual(await peer.cleanup(run.id, { reviewed: true }), second);
  assert.equal(cleanups, 1); assert.equal(service.eventPage(run.id, 0, 100, 'workspace.cleaned').events.length, 1);
  assert.deepEqual(service.getRun(run.id), run); assert.deepEqual(service.store.leases(), []);
});

test('cleanup traverses all exact-type history pages before touching any retained worktree', async t => {
  const { service, artifact, save } = fixture(t), run = save(Array.from({ length: 151 }, (_, index) => artifact(index)));
  for (let index = 0; index < 1001; index++) service.store.appendEvent(run.id, 'progress', { index });
  for (const artifact of run.workspaces) service.store.appendEvent(run.id, 'workspace.cleaned', { path: artifact.path, discardedChanges: false });
  const eventPage = service.store.eventPage.bind(service.store), cursors = [];
  t.mock.method(service.store, 'eventPage', (id, after, limit, type) => {
    assert.equal(type, 'workspace.cleaned'); assert.equal(limit, 100); cursors.push(after); return eventPage(id, after, limit, type);
  });
  t.mock.method(GitWorkspaces.prototype, 'cleanup', async () => { assert.fail('Every retained path is already cleaned.'); });
  const result = await service.cleanup(run.id, { reviewed: true });
  assert.deepEqual(result.removed, []); assert.deepEqual(result.alreadyRemoved, run.workspaces.map(artifact => artifact.path));
  assert.equal(cursors.length, 2); assert.equal(cursors[0], 0); assert.ok(cursors[1] > 1000);
  assert.deepEqual(service.store.leases(), []); assert.equal(service.health().maintenanceOperations, 0);
});

test('cancellation between cleanup history pages prevents worktree changes and releases maintenance ownership', async t => {
  const { service, save } = fixture(t), run = save(), controller = new AbortController();
  for (let index = 0; index < 101; index++) service.store.appendEvent(run.id, 'workspace.cleaned', { path: `already-cleaned-${index}` });
  const eventPage = service.store.eventPage.bind(service.store); let pages = 0;
  t.mock.method(service.store, 'eventPage', (...args) => { const page = eventPage(...args); pages++; controller.abort(new Error('Expected cleanup history cancellation.')); return page; });
  t.mock.method(GitWorkspaces.prototype, 'cleanup', async () => { assert.fail('Cancelled history inspection must not touch a worktree.'); });
  await assert.rejects(service.cleanup(run.id, { reviewed: true }, controller.signal), /Expected cleanup history cancellation/);
  assert.equal(pages, 1); assert.deepEqual(service.store.leases(), []); assert.equal(service.health().maintenanceOperations, 0);
  assert.deepEqual(service.getRun(run.id), run);
});

test('an unreadable later cleanup history page fails before destructive work and preserves the storage error', async t => {
  const { service, save } = fixture(t), run = save();
  for (let index = 0; index < 101; index++) service.store.appendEvent(run.id, 'workspace.cleaned', { path: `already-cleaned-${index}` });
  const eventPage = service.store.eventPage.bind(service.store), failure = new Error('Injected later cleanup history storage failure.'); let pages = 0;
  t.mock.method(service.store, 'eventPage', (...args) => { if (++pages === 2) throw failure; return eventPage(...args); });
  t.mock.method(GitWorkspaces.prototype, 'cleanup', async () => { assert.fail('Unverified history must not touch a worktree.'); });
  await assert.rejects(service.cleanup(run.id, { reviewed: true }), error => error === failure);
  assert.equal(pages, 2); assert.deepEqual(service.store.leases(), []); assert.equal(service.health().maintenanceOperations, 0);
  assert.deepEqual(service.getRun(run.id), run);
});
