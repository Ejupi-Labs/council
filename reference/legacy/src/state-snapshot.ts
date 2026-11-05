import { createHash } from 'node:crypto';
import { closeSync, createReadStream, fstatSync, fsyncSync, lstatSync, mkdirSync, openSync, readSync, realpathSync, rmdirSync, statSync, unlinkSync, writeFileSync } from 'node:fs';
import { basename, dirname, join, resolve } from 'node:path';
import { DatabaseSync } from 'node:sqlite';
import { backupDatabase } from './sqlite-backup.js';
import { requireSafeSqliteRuntime } from './sqlite-runtime.js';
import { checkAgentJsonStructure } from './agent-json.js';
import { assertUniqueObjectKeys } from './control-json.js';
import { objectSchema, parseSchema, type JsonSchema } from './schema.js';
import { VERSION } from './version.js';

export const STATE_DATABASES = Object.freeze(['state.db', 'rooms.sqlite', 'billing.sqlite', 'private-chats.sqlite', 'delegation.sqlite'] as const);
type StateDatabaseName = typeof STATE_DATABASES[number];

export interface StateFileHealth {
  name: StateDatabaseName;
  present: boolean;
  databaseBytes: number;
  walBytes: number;
  shmBytes: number;
  schemaVersion?: number;
  integrity?: string[];
  error?: string;
}

export interface StateSnapshotEntry {
  name: StateDatabaseName;
  bytes: number;
  sha256: string;
  schemaVersion: number;
  startedAt: string;
  completedAt: string;
}

export interface StateSnapshotManifest {
  format: 'council-state-snapshot';
  formatVersion: 1;
  councilVersion: string;
  startedAt: string;
  completedAt: string;
  consistency: 'per-database';
  artifactsIncluded: false;
  databases: StateSnapshotEntry[];
  absentDatabases: StateDatabaseName[];
}

function missing(error: unknown): boolean { return (error as NodeJS.ErrnoException)?.code === 'ENOENT'; }

function regularFile(path: string): void {
  if (!lstatSync(path).isFile()) throw new Error('Expected a regular database file without a symbolic link.');
}

const timestampDefinition: JsonSchema = { type: 'string', minLength: 1, maxLength: 64 };
const snapshotManifestDefinition = objectSchema({
  format: { const: 'council-state-snapshot' }, formatVersion: { const: 1 },
  councilVersion: { type: 'string', minLength: 1, maxLength: 200 },
  startedAt: timestampDefinition, completedAt: timestampDefinition,
  consistency: { const: 'per-database' }, artifactsIncluded: { const: false },
  databases: { type: 'array', minItems: 1, maxItems: STATE_DATABASES.length, items: objectSchema({
    name: { type: 'string', enum: STATE_DATABASES },
    bytes: { type: 'integer', minimum: 0, maximum: Number.MAX_SAFE_INTEGER },
    sha256: { type: 'string', minLength: 64, maxLength: 64, pattern: '^[a-f0-9]{64}$' },
    schemaVersion: { type: 'integer', minimum: 0, maximum: 2147483647 },
    startedAt: timestampDefinition, completedAt: timestampDefinition,
  }, ['name', 'bytes', 'sha256', 'schemaVersion', 'startedAt', 'completedAt']) },
  absentDatabases: { type: 'array', maxItems: STATE_DATABASES.length - 1, items: { type: 'string', enum: STATE_DATABASES } },
}, ['format', 'formatVersion', 'councilVersion', 'startedAt', 'completedAt', 'consistency', 'artifactsIncluded', 'databases', 'absentDatabases']);

/** Return an independent public schema; callers cannot alter verifier invariants. */
export function stateSnapshotSchema(): JsonSchema { return structuredClone(snapshotManifestDefinition); }

function snapshotTimestamp(value: string): void {
  const parsed = Date.parse(value);
  if (!Number.isFinite(parsed) || new Date(parsed).toISOString() !== value) throw new Error('Snapshot manifest timestamps must use canonical UTC ISO format.');
}

function parseSnapshotManifest(source: string): StateSnapshotManifest {
  try { checkAgentJsonStructure(Buffer.from(source)); }
  catch { throw new Error('Snapshot manifest JSON exceeds the supported structural limit.'); }
  const value = JSON.parse(source);
  assertUniqueObjectKeys(source, 'Snapshot manifest');
  const manifest = parseSchema<StateSnapshotManifest>(snapshotManifestDefinition, value, 'snapshot manifest');
  if (!manifest.councilVersion.trim()) throw new Error('Snapshot manifest must name a Council version.');
  snapshotTimestamp(manifest.startedAt); snapshotTimestamp(manifest.completedAt);
  const names = new Set<StateDatabaseName>();
  for (const entry of manifest.databases) {
    if (names.has(entry.name)) throw new Error('Invalid or duplicated database entry in snapshot manifest.');
    snapshotTimestamp(entry.startedAt); snapshotTimestamp(entry.completedAt);
    names.add(entry.name);
  }
  if (!names.has('state.db')) throw new Error('Snapshot does not contain workflow state.');
  for (const name of manifest.absentDatabases) {
    if (names.has(name)) throw new Error('Invalid or duplicated absent database in snapshot manifest.');
    names.add(name);
  }
  if (names.size !== STATE_DATABASES.length) throw new Error('Snapshot manifest must account for every collaboration database.');
  return manifest;
}

function readSnapshotManifest(path: string): StateSnapshotManifest {
  const limit = 64 * 1024, descriptor = openSync(path, 'r');
  try {
    const info = fstatSync(descriptor);
    if (!info.isFile()) throw new Error('Expected a regular snapshot manifest.');
    if (info.size > limit) throw new Error('Snapshot manifest exceeds 64 KiB.');
    const buffer = Buffer.alloc(limit + 1);
    let offset = 0;
    while (offset < buffer.length) {
      const count = readSync(descriptor, buffer, offset, buffer.length - offset, null);
      if (count === 0) break;
      offset += count;
    }
    if (offset > limit) throw new Error('Snapshot manifest exceeds 64 KiB.');
    let source: string;
    try { source = new TextDecoder('utf-8', { fatal: true }).decode(buffer.subarray(0, offset)); }
    catch { throw new Error('Snapshot manifest must contain valid UTF-8 JSON.'); }
    return parseSnapshotManifest(source);
  } finally { closeSync(descriptor); }
}

function inspectDatabase(path: string): { schemaVersion: number; integrity: string[]; journalMode: string } {
  regularFile(path);
  const db = new DatabaseSync(path, { readOnly: true });
  try {
    db.exec('PRAGMA busy_timeout = 250;');
    const schemaVersion = Number((db.prepare('PRAGMA user_version').get() as { user_version: number }).user_version);
    const integrity = db.prepare('PRAGMA quick_check').all().map(row => String(row.quick_check));
    const journalMode = String((db.prepare('PRAGMA journal_mode').get() as { journal_mode: string }).journal_mode);
    return { schemaVersion, integrity, journalMode };
  } finally { db.close(); }
}

/** Seal the isolated copy so no unmanifested WAL can alter its visible contents. */
function sealDatabase(path: string): void {
  const db = new DatabaseSync(path);
  try {
    db.exec('PRAGMA busy_timeout = 250; PRAGMA wal_checkpoint(TRUNCATE);');
    const mode = db.prepare('PRAGMA journal_mode = DELETE').get() as { journal_mode: string };
    if (mode.journal_mode !== 'delete') throw new Error('Could not seal snapshot database in DELETE journal mode.');
  } finally { db.close(); }
  const descriptor = openSync(path, 'r+');
  try { fsyncSync(descriptor); } finally { closeSync(descriptor); }
}

function snapshotSidecars(directory: string, name: StateDatabaseName): string[] {
  return ['-wal', '-shm', '-journal'].flatMap(suffix => {
    const filename = `${name}${suffix}`;
    try { lstatSync(join(directory, filename)); return [filename]; }
    catch (error) { if (missing(error)) return []; throw error; }
  });
}

/** Inspect known collaboration stores without creating optional feature databases. */
export function inspectStateFiles(stateDir: string, checkIntegrity = false): StateFileHealth[] {
  if (checkIntegrity) requireSafeSqliteRuntime();
  const size = (path: string): number => {
    try { return lstatSync(path).size; }
    catch (error) { if (missing(error)) return 0; throw error; }
  };
  return STATE_DATABASES.map(name => {
    const path = join(stateDir, name);
    const entry: StateFileHealth = { name, present: false, databaseBytes: 0, walBytes: 0, shmBytes: 0 };
    try {
      try { regularFile(path); } catch (error) { if (missing(error)) return entry; throw error; }
      entry.present = true;
      entry.databaseBytes = size(path); entry.walBytes = size(`${path}-wal`); entry.shmBytes = size(`${path}-shm`);
      if (checkIntegrity) {
        Object.assign(entry, inspectDatabase(path));
        if (entry.integrity?.length !== 1 || entry.integrity[0] !== 'ok') entry.error = 'SQLite integrity check did not return ok.';
      }
    } catch (error) { entry.error = error instanceof Error ? error.message : 'Database inspection failed.'; }
    return entry;
  });
}

async function checksum(path: string, signal?: AbortSignal): Promise<string> {
  signal?.throwIfAborted();
  const hash = createHash('sha256');
  for await (const chunk of createReadStream(path, { signal })) hash.update(chunk);
  signal?.throwIfAborted();
  return hash.digest('hex');
}

/** Back up all present stores; a durable manifest is published only after every copy verifies. */
export async function createStateSnapshot(stateDir: string, output: string, signal?: AbortSignal): Promise<{ destination: string; manifest: StateSnapshotManifest }> {
  signal?.throwIfAborted();
  requireSafeSqliteRuntime();
  if (typeof output !== 'string' || !output.trim() || output.includes('\0') || Buffer.byteLength(output) > 4096) throw new Error('Snapshot destination must be non-empty text of at most 4096 UTF-8 bytes.');
  const requested = resolve(output), parent = realpathSync.native(dirname(requested));
  const destination = join(parent, basename(requested));
  const source = realpathSync.native(stateDir);
  const inventory = inspectStateFiles(source);
  const problem = inventory.find(entry => entry.error);
  if (problem) throw new Error(`Cannot snapshot ${problem.name}: ${problem.error}`);
  if (!inventory.some(entry => entry.name === 'state.db' && entry.present)) throw new Error('The workflow state database is missing.');
  // This reservation is exclusive; a partial directory never has a completion manifest.
  mkdirSync(destination, { mode: 0o700 });
  const published: string[] = [];
  const manifest: StateSnapshotManifest = {
    format: 'council-state-snapshot', formatVersion: 1, councilVersion: VERSION,
    startedAt: new Date().toISOString(), completedAt: '', consistency: 'per-database', artifactsIncluded: false,
    databases: [], absentDatabases: inventory.filter(entry => !entry.present).map(entry => entry.name),
  };
  try {
    for (const entry of inventory.filter(entry => entry.present)) {
      signal?.throwIfAborted();
      const startedAt = new Date().toISOString(), target = join(destination, entry.name);
      regularFile(join(source, entry.name));
      await backupDatabase(join(source, entry.name), target, signal);
      published.push(target);
      sealDatabase(target);
      if (snapshotSidecars(destination, entry.name).length) throw new Error(`Snapshot contains unexpected journal files for ${entry.name}.`);
      const details = inspectDatabase(target);
      if (details.integrity.length !== 1 || details.integrity[0] !== 'ok') throw new Error(`Snapshot integrity check failed for ${entry.name}.`);
      manifest.databases.push({ name: entry.name, bytes: statSync(target).size, sha256: await checksum(target, signal), schemaVersion: details.schemaVersion, startedAt, completedAt: new Date().toISOString() });
    }
    signal?.throwIfAborted();
    manifest.completedAt = new Date().toISOString();
    // Never publish a completion marker that the strict verifier cannot consume.
    const validated = parseSnapshotManifest(JSON.stringify(manifest));
    const completion = join(destination, 'manifest.json');
    writeFileSync(completion, JSON.stringify(validated, null, 2) + '\n', { flag: 'wx', mode: 0o600 });
    published.push(completion);
    const descriptor = openSync(completion, 'r+');
    try { fsyncSync(descriptor); } finally { closeSync(descriptor); }
    signal?.throwIfAborted();
    return { destination, manifest: validated };
  } catch (error) {
    // Remove only files this operation published. Preserve any unexpected concurrent additions.
    for (const file of published.reverse()) { try { unlinkSync(file); } catch {} }
    try { rmdirSync(destination); } catch {}
    throw error;
  }
}

/** Check a completed snapshot before a manual, stopped-host restore; never opens live state. */
export async function verifyStateSnapshot(directory: string, signal?: AbortSignal): Promise<{ valid: boolean; directory: string; manifest: StateSnapshotManifest; issues: string[] }> {
  signal?.throwIfAborted();
  requireSafeSqliteRuntime();
  const root = realpathSync.native(directory), manifestPath = join(root, 'manifest.json');
  regularFile(manifestPath);
  const manifest = readSnapshotManifest(manifestPath);
  const issues: string[] = [];
  for (const name of manifest.absentDatabases) {
    try { lstatSync(join(root, name)); issues.push(`${name}: database is present but declared absent.`); }
    catch (error) { if (!missing(error)) issues.push(`${name}: cannot verify its declared absence.`); }
    for (const sidecar of snapshotSidecars(root, name)) issues.push(`${name}: unmanifested sidecar ${sidecar}.`);
  }
  for (const entry of manifest.databases) {
    signal?.throwIfAborted();
    try {
      const path = join(root, entry.name);
      regularFile(path);
      const sidecars = snapshotSidecars(root, entry.name);
      if (sidecars.length) { issues.push(`${entry.name}: unmanifested sidecars ${sidecars.join(', ')}.`); continue; }
      if (statSync(path).size !== entry.bytes) issues.push(`${entry.name}: file size differs from the manifest.`);
      if (await checksum(path, signal) !== entry.sha256) issues.push(`${entry.name}: SHA-256 differs from the manifest.`);
      const details = inspectDatabase(path);
      if (details.journalMode !== 'delete') issues.push(`${entry.name}: snapshot database is not sealed in DELETE journal mode.`);
      if (snapshotSidecars(root, entry.name).length) issues.push(`${entry.name}: journal files appeared during verification.`);
      if (await checksum(path, signal) !== entry.sha256) issues.push(`${entry.name}: database changed during verification.`);
      if (details.schemaVersion !== entry.schemaVersion) issues.push(`${entry.name}: schema version differs from the manifest.`);
      if (details.integrity.length !== 1 || details.integrity[0] !== 'ok') issues.push(`${entry.name}: integrity check did not return ok.`);
    } catch (error) { signal?.throwIfAborted(); issues.push(`${entry.name}: ${error instanceof Error ? error.message : 'Verification failed.'}`); }
  }
  return { valid: issues.length === 0, directory: root, manifest, issues };
}
