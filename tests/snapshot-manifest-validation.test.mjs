import test from 'node:test';
import assert from 'node:assert/strict';
import fs, { existsSync, mkdirSync, mkdtempSync, readFileSync, realpathSync, rmSync, writeFileSync } from 'node:fs';
import { syncBuiltinESMExports } from 'node:module';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { DatabaseSync } from 'node:sqlite';
import { CouncilService, createStateSnapshot, loadConfig, STATE_DATABASES, stateSnapshotSchema, verifyStateSnapshot } from '../dist/index.js';

async function fixture(t) {
  const root = mkdtempSync(join(realpathSync.native(tmpdir()), 'council-manifest-validation-'));
  const service = new CouncilService(loadConfig(root, { backend: 'mock' }));
  t.after(async () => { await service.close(); rmSync(root, { recursive: true, force: true }); });
  const snapshot = await service.snapshot(join(root, 'snapshot'));
  const path = join(snapshot.destination, 'manifest.json'), original = readFileSync(path, 'utf8');
  return { snapshot, path, original };
}

test('snapshot verification rejects invalid typed metadata, noncanonical dates and unknown fields', async t => {
  const { snapshot, path, original } = await fixture(t);
  const edits = [
    manifest => { manifest.councilVersion = { incompatible: true }; },
    manifest => { manifest.councilVersion = ' '; },
    manifest => { manifest.databases[0].startedAt = null; },
    manifest => { manifest.databases[0].completedAt = 'not a timestamp'; },
    manifest => { manifest.databases[0].startedAt = '2026-02-30T00:00:00.000Z'; },
    manifest => { manifest.startedAt = '2026-11-04'; },
    manifest => { manifest.completedAt = '2026-11-04T19:00:00+00:00'; },
    manifest => { delete manifest.databases[0].completedAt; },
    manifest => { manifest.databases[0].unrecognized = 'Unspecified metadata.'; },
    manifest => { manifest.unrecognized = { data: 'Not part of this format.' }; },
  ];
  for (const edit of edits) {
    const manifest = JSON.parse(original); edit(manifest); writeFileSync(path, JSON.stringify(manifest));
    await assert.rejects(verifyStateSnapshot(snapshot.destination), /snapshot manifest|Snapshot manifest/);
  }
  writeFileSync(path, original);
  assert.equal((await verifyStateSnapshot(snapshot.destination)).valid, true);
});

test('deep manifest JSON is rejected before full parsing or exposure to protocol serialization', async t => {
  const { snapshot, path, original } = await fixture(t);
  const deep = original.trim().slice(0, -1) + ',"extra":' + '['.repeat(2000) + '0' + ']'.repeat(2000) + '}';
  assert(Buffer.byteLength(deep) < 64 * 1024);
  writeFileSync(path, deep);
  const parse = JSON.parse; let rawParses = 0;
  JSON.parse = (value, ...rest) => { if (value === deep) rawParses++; return parse(value, ...rest); };
  try { await assert.rejects(verifyStateSnapshot(snapshot.destination), /Snapshot manifest JSON.*structural/); }
  finally { JSON.parse = parse; }
  assert.equal(rawParses, 0);
});

test('duplicate manifest properties cannot select a different interpretation in another reader', async t => {
  const { snapshot, path, original } = await fixture(t);
  const duplicate = original.replace('"formatVersion": 1', '"formatVersion": 1, "formatVersion": 1');
  writeFileSync(path, duplicate);
  await assert.rejects(verifyStateSnapshot(snapshot.destination), /duplicate/i);
});

test('public snapshot schema copies and database-name lists cannot weaken path or shape validation', async t => {
  const { snapshot, path, original } = await fixture(t);
  const schema = stateSnapshotSchema();
  schema.additionalProperties = true;
  schema.properties.databases.items.properties.name.enum.push('../outside.sqlite');
  assert.equal(stateSnapshotSchema().additionalProperties, false);
  assert(!stateSnapshotSchema().properties.databases.items.properties.name.enum.includes('../outside.sqlite'));
  assert.throws(() => STATE_DATABASES.push('../outside.sqlite'), TypeError);
  const manifest = JSON.parse(original); manifest.databases[0].name = '../outside.sqlite';
  writeFileSync(path, JSON.stringify(manifest));
  await assert.rejects(verifyStateSnapshot(snapshot.destination), /snapshot manifest/);
});


test('manifest growth after its metadata check cannot bypass the bounded descriptor read', async t => {
  const { snapshot, path } = await fixture(t);
  const original = { openSync: fs.openSync, statSync: fs.statSync, fstatSync: fs.fstatSync, readSync: fs.readSync };
  const descriptors = new Set(); let grew = false, readBytes = 0;
  const grow = () => { if (!grew) { grew = true; fs.appendFileSync(path, ' '.repeat(80 * 1024)); } };
  fs.openSync = (...args) => { const descriptor = original.openSync(...args); if (args[0] === path) descriptors.add(descriptor); return descriptor; };
  fs.statSync = (...args) => { const info = original.statSync(...args); if (args[0] === path) grow(); return info; };
  fs.fstatSync = (...args) => { const info = original.fstatSync(...args); if (descriptors.has(args[0])) grow(); return info; };
  fs.readSync = (...args) => { if (descriptors.has(args[0])) { assert(args[3] <= 64 * 1024 + 1); const count = original.readSync(...args); readBytes += count; return count; } return original.readSync(...args); };
  syncBuiltinESMExports();
  try { await assert.rejects(verifyStateSnapshot(snapshot.destination), /Snapshot manifest exceeds 64 KiB/); }
  finally { Object.assign(fs, original); syncBuiltinESMExports(); }
  assert.equal(grew, true);
  assert.equal(readBytes, 64 * 1024 + 1);
});

test('invalid UTF-8 inside a version label cannot be accepted as replacement text', async t => {
  const { snapshot, path, original } = await fixture(t);
  const bytes = Buffer.from(original), marker = Buffer.from('\"councilVersion\": \"');
  const offset = bytes.indexOf(marker) + marker.length;
  assert(offset >= marker.length);
  bytes[offset] = 0xff;
  writeFileSync(path, bytes);
  await assert.rejects(verifyStateSnapshot(snapshot.destination), /valid UTF-8 JSON/);
});

test('snapshot creation never publishes a completion manifest rejected by its own schema', async t => {
  const root = mkdtempSync(join(realpathSync.native(tmpdir()), 'council-manifest-producer-'));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  const state = join(root, 'state'), destination = join(root, 'snapshot');
  mkdirSync(state);
  const path = join(state, 'state.db'), database = new DatabaseSync(path);
  database.exec('CREATE TABLE retained(value TEXT); INSERT INTO retained VALUES (\'Keep malformed source evidence.\'); PRAGMA user_version=-1;');
  database.close();
  const original = readFileSync(path);
  await assert.rejects(createStateSnapshot(state, destination), /snapshot manifest/);
  assert.equal(existsSync(destination), false);
  assert.deepEqual(readFileSync(path), original);
});
