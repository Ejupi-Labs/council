import test from 'node:test';
import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { mkdtempSync, readFileSync, readdirSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { DatabaseSync } from 'node:sqlite';
import { CouncilService, loadConfig, verifyStateSnapshot } from '../dist/index.js';

function fixture(t) {
  const root = mkdtempSync(join(tmpdir(), 'council-snapshot-sidecars-'));
  const service = new CouncilService(loadConfig(root, { backend: 'mock' }));
  t.after(async () => { await service.close(); rmSync(root, { recursive: true, force: true }); });
  return { root, service };
}

test('snapshots seal every copied database so reopening and verifying creates no WAL sidecars', async t => {
  const { root, service } = fixture(t);
  service.rooms.create({ title: 'Sealed state', brief: 'Offline fixture.', members: [{ agent: 'codex', role: 'lead' }] });
  const snapshot = await service.snapshot(join(root, 'snapshot'));
  for (const entry of snapshot.manifest.databases) {
    const db = new DatabaseSync(join(snapshot.destination, entry.name), { readOnly: true });
    try { assert.equal(db.prepare('PRAGMA journal_mode').get().journal_mode, 'delete'); }
    finally { db.close(); }
  }
  assert.equal((await verifyStateSnapshot(snapshot.destination)).valid, true);
  assert.deepEqual(readdirSync(snapshot.destination).sort(), ['manifest.json', ...snapshot.manifest.databases.map(entry => entry.name)].sort());
});

test('verification rejects unhashed WAL changes even when the main file still matches its manifest exactly', async t => {
  const { root, service } = fixture(t);
  const snapshot = await service.snapshot(join(root, 'wal-change'));
  const path = join(snapshot.destination, 'state.db'), manifestPath = join(snapshot.destination, 'manifest.json');
  const writer = new DatabaseSync(path);
  try {
    writer.exec('PRAGMA journal_mode=WAL');
    // Reproduce a legacy WAL-mode copy with a matching main-file manifest, then mutate only its WAL.
    const manifest = JSON.parse(readFileSync(manifestPath, 'utf8'));
    const originalMain = readFileSync(path);
    manifest.databases[0].sha256 = createHash('sha256').update(originalMain).digest('hex');
    manifest.databases[0].bytes = originalMain.length;
    writeFileSync(manifestPath, JSON.stringify(manifest));
    writer.exec('CREATE TABLE unmanifested_evidence(body TEXT); INSERT INTO unmanifested_evidence VALUES(\'Must not verify as sealed data\')');
    assert.deepEqual(readFileSync(path), originalMain);
    const verified = await verifyStateSnapshot(snapshot.destination);
    assert.equal(verified.valid, false);
    assert(verified.issues.some(issue => /state\.db: unmanifested sidecars/.test(issue)));
  } finally { writer.close(); }
});

test('declared-absent databases and stray journal files cannot be smuggled into a verified restore set', async t => {
  const { root, service } = fixture(t);
  const snapshot = await service.snapshot(join(root, 'unexpected-files'));
  writeFileSync(join(snapshot.destination, 'rooms.sqlite'), 'An unmanifested database.');
  writeFileSync(join(snapshot.destination, 'private-chats.sqlite-wal'), '');
  const verified = await verifyStateSnapshot(snapshot.destination);
  assert.equal(verified.valid, false);
  assert(verified.issues.some(issue => issue === 'rooms.sqlite: database is present but declared absent.'));
  assert(verified.issues.some(issue => /private-chats\.sqlite: unmanifested sidecar/.test(issue)));
});
