import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdirSync, mkdtempSync, realpathSync, rmSync, symlinkSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { CouncilService, loadConfig, verifyStateSnapshot } from '../dist/index.js';

test('database backups and snapshot verification report the physical destination through a directory alias', async t => {
  const root = mkdtempSync(join(realpathSync.native(tmpdir()), 'council-snapshot-alias-'));
  const service = new CouncilService(loadConfig(root, { backend: 'mock' }));
  t.after(async () => { await service.close(); rmSync(root, { recursive: true, force: true }); });
  const physical = join(root, 'physical-backups'), alias = join(root, 'backups-alias');
  mkdirSync(physical);
  try { symlinkSync(physical, alias, process.platform === 'win32' ? 'junction' : 'dir'); }
  catch (error) { if (['EPERM', 'EACCES', 'ENOSYS'].includes(error.code)) { t.skip('The operating system does not permit creating this directory alias.'); return; } throw error; }
  const destination = await service.backup(join(alias, 'workflow.db'));
  assert.equal(destination, join(physical, 'workflow.db'));
  const snapshot = await service.snapshot(join(alias, 'complete'));
  assert.equal(snapshot.destination, join(physical, 'complete'));
  const verified = await verifyStateSnapshot(join(alias, 'complete'));
  assert.equal(verified.valid, true);
  assert.equal(verified.directory, join(physical, 'complete'));
  assert.equal(service.health(true).status, 'ok');
});
