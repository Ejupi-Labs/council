import test from 'node:test';
import assert from 'node:assert/strict';
import { execFile } from 'node:child_process';
import { mkdtempSync, readdirSync, readFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { basename, dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { promisify } from 'node:util';
import { CouncilService, createStateSnapshot, loadConfig } from '../dist/index.js';

const execute = promisify(execFile);
const repository = fileURLToPath(new URL('../', import.meta.url));

function files(root) {
  const result = {};
  const visit = (directory, prefix = '') => {
    for (const item of readdirSync(directory, { withFileTypes: true })) {
      const name = prefix + item.name, path = join(directory, item.name);
      if (item.isDirectory()) { result[name + '/'] = true; visit(path, name + '/'); }
      else result[name] = readFileSync(path);
    }
  };
  visit(root); return result;
}

test('all direct database APIs reject reported unsafe SQLite before touching files while imports and metadata reads remain usable', { timeout: 15000 }, async t => {
  const parent = resolve(tmpdir()), root = mkdtempSync(join(parent, 'council-sqlite-runtime-'));
  const service = new CouncilService(loadConfig(root, { backend: 'mock' }), {
    doctor: async () => [], run: async () => { assert.fail('Runtime guard checks must not invoke providers.'); },
  });
  t.after(async () => {
    await service.close();
    assert.equal(dirname(resolve(root)), parent); assert.ok(basename(root).startsWith('council-sqlite-runtime-'));
    rmSync(root, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 });
  });
  service.rooms.create({ title: 'Retained board', brief: 'Keep original collaboration evidence unchanged.', members: [{ agent: 'codex', role: 'lead' }], tasks: [] });
  service.billing.get('codex');
  service.privateChats.create({ participants: ['first-task', 'second-task'] });
  service.delegation.report({ author: 'Host', kind: 'update', body: 'Retained executive evidence.' });
  await service.close();
  const snapshot = await createStateSnapshot(service.config.stateDir, join(root, 'snapshot'));
  const original = files(root);
  const script = String.raw`
import assert from 'node:assert/strict';
import { existsSync } from 'node:fs';
import { join } from 'node:path';
const root = process.argv[1], snapshot = process.argv[2];
const descriptor = Object.getOwnPropertyDescriptor(process.versions, 'sqlite');
assert.equal(descriptor?.configurable, true);
const versions = ['3.51.2', '3.51.3-invalid', 'unknown', undefined];
let constructorChecks = 0, directChecks = 0, cancellationChecks = 0;
try {
  // Importing the API must not open SQLite or prevent metadata-only inspection.
  Object.defineProperty(process.versions, 'sqlite', { ...descriptor, value: versions[0] });
  const api = await import('./dist/index.js');
  const { backupDatabase } = await import('./dist/sqlite-backup.js');
  const config = api.loadConfig(root, { backend: 'mock' });
  const unsafe = error => error instanceof Error && error.message.includes('requires SQLite 3.51.3');
  for (const version of versions) {
    Object.defineProperty(process.versions, 'sqlite', { ...descriptor, value: version });
    const fresh = join(root, 'must-not-create');
    const constructors = [
      stateDir => new api.CouncilStore(stateDir),
      stateDir => new api.RoomStore(stateDir),
      stateDir => new api.BillingManager({ ...config, stateDir }),
      stateDir => new api.PrivateChatManager({ config: { ...config, stateDir } }),
      stateDir => new api.DelegationManager({ config: { ...config, stateDir } }),
    ];
    for (const create of constructors) {
      assert.throws(() => create(fresh), unsafe); constructorChecks++;
      assert.throws(() => create(config.stateDir), unsafe); constructorChecks++;
    }
    await assert.rejects(api.createStateSnapshot(config.stateDir, join(root, 'rejected-snapshot')), unsafe); directChecks++;
    await assert.rejects(api.verifyStateSnapshot(snapshot), unsafe); directChecks++;
    await assert.rejects(backupDatabase(join(config.stateDir, 'state.db'), join(root, 'rejected-backup.db')), unsafe); directChecks++;
    assert.throws(() => api.inspectStateFiles(config.stateDir, true), unsafe); directChecks++;
    assert.equal(api.inspectStateFiles(config.stateDir, false).filter(entry => entry.present).length, 5);
    assert.equal(api.inspectStateFiles(fresh, false).every(entry => !entry.present), true);
    assert.equal(existsSync(fresh), false);
    assert.equal(existsSync(join(root, 'rejected-snapshot')), false);
    assert.equal(existsSync(join(root, 'rejected-backup.db')), false);
    const controller = new AbortController(), reason = new Error('Preserve the requested cancellation.');
    controller.abort(reason);
    await assert.rejects(api.createStateSnapshot(config.stateDir, join(root, 'cancelled-snapshot'), controller.signal), error => error === reason); cancellationChecks++;
    await assert.rejects(api.verifyStateSnapshot(snapshot, controller.signal), error => error === reason); cancellationChecks++;
    await assert.rejects(backupDatabase(join(config.stateDir, 'state.db'), join(root, 'cancelled-backup.db'), controller.signal), error => error === reason); cancellationChecks++;
  }
  const { requireSafeSqliteRuntime } = await import('./dist/sqlite-runtime.js');
  for (const version of ['3.51.3', '3.51.3.1', '3.52.0', '4.0.0']) {
    Object.defineProperty(process.versions, 'sqlite', { ...descriptor, value: version });
    assert.equal(requireSafeSqliteRuntime(), version);
  }
  console.log(JSON.stringify({ versionsChecked: versions.length, constructorChecks, directChecks, cancellationChecks, importsUsable: true }));
} finally { Object.defineProperty(process.versions, 'sqlite', descriptor); }
`;
  const result = await execute(process.execPath, ['--input-type=module', '-e', script, root, snapshot.destination], { cwd: repository, windowsHide: true, timeout: 10000 });
  assert.deepEqual(JSON.parse(result.stdout), { versionsChecked: 4, constructorChecks: 40, directChecks: 16, cancellationChecks: 12, importsUsable: true });
  assert.deepEqual(files(root), original, 'Rejected runtimes must not create, remove or mutate database, journal, snapshot or manifest files.');
});

