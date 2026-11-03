import { randomUUID } from 'node:crypto';
import { chmodSync, mkdirSync } from 'node:fs';
import { join } from 'node:path';
import { DatabaseSync } from 'node:sqlite';
import type { ContextEntry, EventRecord, Lease, Message, RunRecord } from './types.js';

const SCHEMA_VERSION = 2;
const MAX_JSON_BYTES = 4 * 1024 * 1024;
const MAX_RUN_JSON_BYTES = 16 * 1024 * 1024;

function text(value: string, label: string, maxBytes: number): string {
  if (typeof value !== 'string' || !value.trim() || Buffer.byteLength(value, 'utf8') > maxBytes || value.includes('\0')) {
    throw new Error(`${label} must be non-empty text of at most ${maxBytes} UTF-8 bytes`);
  }
  return value;
}

function encode(value: unknown, maxBytes = MAX_JSON_BYTES): string {
  const json = JSON.stringify(value);
  if (json === undefined || Buffer.byteLength(json, 'utf8') > maxBytes) {
    throw new Error(`Stored JSON must be serializable and no larger than ${maxBytes / (1024 * 1024)} MiB`);
  }
  return json;
}

function normalizePath(value: string): string {
  text(value, 'Reservation path', 4096);
  const normalized = value.replaceAll('\\', '/');
  if (normalized.startsWith('/') || /^[a-z]:/i.test(normalized) || /[\x00-\x1f:]/.test(normalized)) {
    throw new Error('Reservation paths must be workspace-relative paths');
  }
  if (normalized === '**') return normalized;
  const subtree = normalized.endsWith('/**');
  const base = subtree ? normalized.slice(0, -3) : normalized;
  if (!base || base.split('/').some(part => !part || part === '..' || part === '.') || /[*?]/.test(base)) {
    throw new Error('Reservation paths cannot contain traversal, empty segments, or unsupported globs; use directory/**');
  }
  return normalized;
}

function overlaps(left: string, right: string): boolean {
  const a = left.toLowerCase();
  const b = right.toLowerCase();
  if (a === b || a === '**' || b === '**') return true;
  const contains = (pattern: string, candidate: string) => {
    if (!pattern.endsWith('/**')) return false;
    const base = pattern.slice(0, -3);
    const candidateBase = candidate.endsWith('/**') ? candidate.slice(0, -3) : candidate;
    return candidateBase === base || candidateBase.startsWith(`${base}/`);
  };
  return contains(a, b) || contains(b, a);
}

function processAlive(pid: number): boolean {
  if (pid === process.pid) return true;
  try { process.kill(pid, 0); return true; }
  catch (error) { return (error as NodeJS.ErrnoException).code !== 'ESRCH'; }
}

type OwnedRunRow = { document: string; owner_instance: string | null; owner_pid: number | null; cancel_requested: number };
type InstanceRow = { id: string; pid: number; heartbeat_at: number; slot_limit: number | null };

/** Durable collaboration state shared by all Council processes in one workspace. */
export class CouncilStore {
  private readonly db: DatabaseSync;

  constructor(stateDir: string) {
    mkdirSync(stateDir, { recursive: true, mode: 0o700 });
    const databasePath = join(stateDir, 'state.db');
    this.db = new DatabaseSync(databasePath);
    try {
      this.db.exec('PRAGMA busy_timeout = 5000; PRAGMA journal_mode = WAL; PRAGMA foreign_keys = ON;');
      this.transaction(() => {
        // Recheck under the migration lock: another process may have upgraded while opening.
        const version = this.db.prepare('PRAGMA user_version').get() as { user_version: number };
        if (version.user_version > SCHEMA_VERSION) throw new Error(`Council state schema ${version.user_version} is newer than supported schema ${SCHEMA_VERSION}`);
        this.db.exec(`
          CREATE TABLE IF NOT EXISTS runs (id TEXT PRIMARY KEY, updated_at TEXT NOT NULL, document TEXT NOT NULL);
          CREATE TABLE IF NOT EXISTS events (sequence INTEGER PRIMARY KEY AUTOINCREMENT, run_id TEXT NOT NULL, type TEXT NOT NULL, data TEXT NOT NULL, created_at TEXT NOT NULL);
          CREATE INDEX IF NOT EXISTS events_run_sequence ON events(run_id, sequence);
          CREATE TABLE IF NOT EXISTS messages (id TEXT PRIMARY KEY, sender TEXT NOT NULL, recipient TEXT NOT NULL, created_at TEXT NOT NULL, acknowledged_at TEXT, document TEXT NOT NULL);
          CREATE INDEX IF NOT EXISTS messages_inbox ON messages(recipient, created_at);
          CREATE TABLE IF NOT EXISTS leases (id TEXT PRIMARY KEY, owner TEXT NOT NULL, path TEXT NOT NULL, path_key TEXT NOT NULL UNIQUE, run_id TEXT, expires_at INTEGER NOT NULL);
          CREATE TABLE IF NOT EXISTS context (key TEXT PRIMARY KEY, value TEXT NOT NULL, author TEXT NOT NULL, updated_at TEXT NOT NULL);
        `);
        if (version.user_version < 2) this.db.exec(`
          ALTER TABLE runs ADD COLUMN owner_instance TEXT;
          ALTER TABLE runs ADD COLUMN owner_pid INTEGER;
          ALTER TABLE runs ADD COLUMN cancel_requested INTEGER NOT NULL DEFAULT 0;
        `);
        this.db.exec(`
          CREATE INDEX IF NOT EXISTS runs_owner ON runs(owner_instance);
          CREATE INDEX IF NOT EXISTS runs_unfinished_owner ON runs(owner_instance) WHERE json_extract(document, '$.status') IN ('queued', 'running');
          CREATE TABLE IF NOT EXISTS instances (id TEXT PRIMARY KEY, pid INTEGER NOT NULL, registered_at INTEGER NOT NULL, heartbeat_at INTEGER NOT NULL, slot_limit INTEGER);
          CREATE TABLE IF NOT EXISTS slots (token TEXT PRIMARY KEY, instance_id TEXT NOT NULL REFERENCES instances(id) ON DELETE CASCADE, run_id TEXT NOT NULL, acquired_at INTEGER NOT NULL);
          CREATE INDEX IF NOT EXISTS slots_instance ON slots(instance_id);
          PRAGMA user_version = 2;
        `);
      });
      if (process.platform !== 'win32') chmodSync(databasePath, 0o600);
    } catch (error) {
      this.db.close();
      throw error;
    }
  }

  private transaction<T>(operation: () => T): T {
    this.db.exec('BEGIN IMMEDIATE');
    try {
      const result = operation();
      this.db.exec('COMMIT');
      return result;
    } catch (error) {
      this.db.exec('ROLLBACK');
      throw error;
    }
  }

  saveRun(run: RunRecord): void {
    text(run.id, 'Run id', 256);
    text(run.updatedAt, 'Run timestamp', 64);
    const document = encode(run, MAX_RUN_JSON_BYTES);
    this.transaction(() => {
      const existing = this.ownedRun(run.id);
      if (existing?.owner_instance) this.assertRunOwner(existing);
      // Cancellation and owner columns are deliberately absent from the partial-update clause.
      this.db.prepare('INSERT INTO runs(id, updated_at, document) VALUES (?, ?, ?) ON CONFLICT(id) DO UPDATE SET updated_at = excluded.updated_at, document = excluded.document')
        .run(run.id, run.updatedAt, document);
    });
  }

  getRun(id: string): RunRecord | undefined {
    text(id, 'Run id', 256);
    const row = this.db.prepare('SELECT document FROM runs WHERE id = ?').get(id) as { document: string } | undefined;
    return row ? JSON.parse(row.document) as RunRecord : undefined;
  }

  listRuns(limit = 20): RunRecord[] {
    if (!Number.isInteger(limit) || limit < 1) throw new Error('Run list limit must be a positive integer');
    const rows = this.db.prepare('SELECT document FROM runs ORDER BY updated_at DESC, id LIMIT ?').all(Math.min(limit, 100)) as { document: string }[];
    return rows.map(row => JSON.parse(row.document) as RunRecord);
  }

  appendEvent(runId: string, type: string, data: unknown): EventRecord {
    text(runId, 'Run id', 256);
    text(type, 'Event type', 128);
    const createdAt = new Date().toISOString();
    const result = this.db.prepare('INSERT INTO events(run_id, type, data, created_at) VALUES (?, ?, ?, ?)').run(runId, type, encode(data), createdAt);
    return { sequence: Number(result.lastInsertRowid), runId, type, data, createdAt };
  }

  events(runId: string, after = 0): EventRecord[] {
    text(runId, 'Run id', 256);
    if (!Number.isSafeInteger(after) || after < 0) throw new Error('Event cursor must be a non-negative integer');
    const rows = this.db.prepare('SELECT sequence, run_id, type, data, created_at FROM events WHERE run_id = ? AND sequence > ? ORDER BY sequence LIMIT 1000').all(runId, after) as {
      sequence: number; run_id: string; type: string; data: string; created_at: string;
    }[];
    return rows.map(row => ({ sequence: row.sequence, runId: row.run_id, type: row.type, data: JSON.parse(row.data), createdAt: row.created_at }));
  }

  sendMessage(input: { from: string; to: string; subject: string; body: string; runId?: string }): Message {
    text(input.from, 'Sender', 128);
    text(input.to, 'Recipient', 128);
    text(input.subject, 'Message subject', 512);
    text(input.body, 'Message body', 32 * 1024);
    if (input.runId !== undefined) text(input.runId, 'Run id', 256);
    const message: Message = { ...input, id: randomUUID(), createdAt: new Date().toISOString() };
    this.db.prepare('INSERT INTO messages(id, sender, recipient, created_at, document) VALUES (?, ?, ?, ?, ?)')
      .run(message.id, message.from, message.to, message.createdAt, encode(message));
    return message;
  }

  inbox(agent: string, unreadOnly = true): Message[] {
    text(agent, 'Agent', 128);
    const rows = this.db.prepare('SELECT document, acknowledged_at FROM messages WHERE recipient = ? AND (? = 0 OR acknowledged_at IS NULL) ORDER BY created_at, rowid LIMIT 100')
      .all(agent, unreadOnly ? 1 : 0) as { document: string; acknowledged_at: string | null }[];
    return rows.map(row => {
      const message = JSON.parse(row.document) as Message;
      if (row.acknowledged_at) message.acknowledgedAt = row.acknowledged_at;
      return message;
    });
  }

  acknowledge(id: string, agent: string): boolean {
    text(id, 'Message id', 256);
    text(agent, 'Agent', 128);
    const result = this.db.prepare('UPDATE messages SET acknowledged_at = ? WHERE id = ? AND recipient = ? AND acknowledged_at IS NULL')
      .run(new Date().toISOString(), id, agent);
    return result.changes > 0;
  }

  reserve(owner: string, paths: string[], ttlMs: number, runId?: string): Lease[] {
    text(owner, 'Lease owner', 128);
    if (!Array.isArray(paths) || paths.length < 1 || paths.length > 100) throw new Error('Reserve between 1 and 100 paths');
    if (!Number.isSafeInteger(ttlMs) || ttlMs < 1 || ttlMs > 24 * 60 * 60 * 1000) throw new Error('Lease TTL must be between 1 ms and 24 hours');
    if (runId !== undefined) text(runId, 'Run id', 256);
    const normalized = [...new Map(paths.map(path => { const value = normalizePath(path); return [value.toLowerCase(), value]; })).values()];
    return this.transaction(() => {
      const now = Date.now();
      this.db.prepare('DELETE FROM leases WHERE expires_at <= ?').run(now);
      const active = this.readLeases(now);
      for (const requested of normalized) {
        const conflict = active.find(lease => lease.owner !== owner && lease.paths.some(path => overlaps(path, requested)));
        if (conflict) throw new Error(`Reservation conflict: ${requested} overlaps ${conflict.paths[0]} owned by ${conflict.owner} until ${conflict.expiresAt}`);
      }
      const expiresAt = new Date(now + ttlMs).toISOString();
      return normalized.map(path => {
        const existing = active.find(lease => lease.owner === owner && lease.paths[0]?.toLowerCase() === path.toLowerCase());
        const lease: Lease = { id: existing?.id ?? randomUUID(), owner, paths: [path], expiresAt, ...(runId === undefined ? {} : { runId }) };
        this.db.prepare('INSERT INTO leases(id, owner, path, path_key, run_id, expires_at) VALUES (?, ?, ?, ?, ?, ?) ON CONFLICT(path_key) DO UPDATE SET id = excluded.id, owner = excluded.owner, path = excluded.path, run_id = excluded.run_id, expires_at = excluded.expires_at')
          .run(lease.id, owner, path, path.toLowerCase(), runId ?? null, now + ttlMs);
        return lease;
      });
    });
  }

  release(owner: string, paths?: string[]): number {
    text(owner, 'Lease owner', 128);
    if (paths === undefined) return Number(this.db.prepare('DELETE FROM leases WHERE owner = ?').run(owner).changes);
    if (!Array.isArray(paths) || paths.length > 100) throw new Error('Release at most 100 paths');
    const normalized = paths.map(normalizePath);
    return this.transaction(() => normalized.reduce((count, path) => count + Number(this.db.prepare('DELETE FROM leases WHERE owner = ? AND path_key = ?').run(owner, path.toLowerCase()).changes), 0));
  }

  private readLeases(now = Date.now()): Lease[] {
    const rows = this.db.prepare('SELECT id, owner, path, run_id, expires_at FROM leases WHERE expires_at > ? ORDER BY path_key').all(now) as {
      id: string; owner: string; path: string; run_id: string | null; expires_at: number;
    }[];
    return rows.map(row => ({ id: row.id, owner: row.owner, paths: [row.path], expiresAt: new Date(row.expires_at).toISOString(), ...(row.run_id === null ? {} : { runId: row.run_id }) }));
  }

  leases(): Lease[] {
    return this.readLeases();
  }

  putContext(key: string, value: string, author: string): ContextEntry {
    text(key, 'Context key', 256);
    text(value, 'Context value', 64 * 1024);
    text(author, 'Context author', 128);
    const entry = { key, value, author, updatedAt: new Date().toISOString() };
    this.db.prepare('INSERT INTO context(key, value, author, updated_at) VALUES (?, ?, ?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value, author = excluded.author, updated_at = excluded.updated_at')
      .run(key, value, author, entry.updatedAt);
    return entry;
  }

  getContext(key: string): ContextEntry | undefined {
    text(key, 'Context key', 256);
    const row = this.db.prepare('SELECT key, value, author, updated_at FROM context WHERE key = ?').get(key) as { key: string; value: string; author: string; updated_at: string } | undefined;
    return row ? { key: row.key, value: row.value, author: row.author, updatedAt: row.updated_at } : undefined;
  }

  listContext(): ContextEntry[] {
    const rows = this.db.prepare('SELECT key, value, author, updated_at FROM context ORDER BY updated_at DESC, key LIMIT 100').all() as { key: string; value: string; author: string; updated_at: string }[];
    return rows.map(row => ({ key: row.key, value: row.value, author: row.author, updatedAt: row.updated_at }));
  }

  registerInstance(instanceId: string, pid: number): void {
    text(instanceId, 'Instance id', 128);
    if (!Number.isSafeInteger(pid) || pid !== process.pid) throw new Error('A Council instance must register its own process PID.');
    this.transaction(() => {
      const existing = this.instance(instanceId);
      if (existing && existing.pid !== pid) throw new Error('This Council instance ID is already registered by another process.');
      const now = Date.now();
      this.db.prepare('INSERT INTO instances(id, pid, registered_at, heartbeat_at) VALUES (?, ?, ?, ?) ON CONFLICT(id) DO UPDATE SET heartbeat_at = excluded.heartbeat_at')
        .run(instanceId, pid, now, now);
    });
  }

  heartbeat(instanceId: string): boolean {
    text(instanceId, 'Instance id', 128);
    return this.db.prepare('UPDATE instances SET heartbeat_at = ? WHERE id = ? AND pid = ?').run(Date.now(), instanceId, process.pid).changes > 0;
  }

  unregisterInstance(instanceId: string): void {
    text(instanceId, 'Instance id', 128);
    this.transaction(() => {
      const instance = this.instance(instanceId);
      if (!instance) return;
      if (instance.pid !== process.pid) throw new Error('Only the owning process can unregister a Council instance.');
      const active = this.db.prepare("SELECT 1 FROM runs WHERE owner_instance = ? AND json_extract(document, '$.status') IN ('queued', 'running') LIMIT 1").get(instanceId);
      if (active) {
        throw new Error('Cannot unregister a Council instance while it owns unfinished runs.');
      }
      this.db.prepare('DELETE FROM instances WHERE id = ?').run(instanceId);
    });
  }

  claimRun(runId: string, instanceId: string): void {
    text(runId, 'Run id', 256);
    text(instanceId, 'Instance id', 128);
    this.transaction(() => {
      const instance = this.assertInstance(instanceId);
      const row = this.ownedRun(runId);
      if (!row) throw new Error('Cannot claim a run that does not exist.');
      if (!['queued', 'running'].includes((JSON.parse(row.document) as RunRecord).status)) throw new Error('Cannot claim a run that already reached a terminal state.');
      if (row.owner_instance && row.owner_instance !== instanceId) {
        const owner = this.instance(row.owner_instance);
        if (owner && processAlive(owner.pid)) throw new Error('This run is already owned by a live Council instance.');
        throw new Error('This run has an orphaned owner; recover it before starting a replacement.');
      }
      this.db.prepare('UPDATE runs SET owner_instance = ?, owner_pid = ? WHERE id = ?').run(instanceId, instance.pid, runId);
    });
  }

  requestCancellation(runId: string): boolean {
    text(runId, 'Run id', 256);
    return this.transaction(() => {
      const row = this.ownedRun(runId);
      if (!row || !['queued', 'running'].includes((JSON.parse(row.document) as RunRecord).status) || row.cancel_requested) return false;
      this.db.prepare('UPDATE runs SET cancel_requested = 1 WHERE id = ?').run(runId);
      if (!this.legacyCancellation(runId)) this.appendEvent(runId, 'cancel_requested', {});
      return true;
    });
  }

  /** Final completion and a concurrent cancellation request are ordered by one SQLite transaction. */
  finalizeRun(run: RunRecord): RunRecord {
    text(run.id, 'Run id', 256);
    if (['queued', 'running'].includes(run.status)) throw new Error('Only a terminal run can be finalized.');
    return this.transaction(() => {
      const row = this.ownedRun(run.id);
      if (!row) throw new Error('Cannot finalize a run that does not exist.');
      const existing = JSON.parse(row.document) as RunRecord;
      if (row.owner_instance) this.assertRunOwner(row);
      // A previously settled failure/cancellation cannot be rewritten by a stale completion.
      if (['failed', 'cancelled'].includes(existing.status) && existing.status !== run.status) return existing;
      const final = structuredClone(run);
      if (final.status === 'completed' && (row.cancel_requested || this.legacyCancellation(run.id))) {
        final.status = 'cancelled';
        final.error = 'Cancellation was requested before workflow completion.';
      }
      final.updatedAt = new Date().toISOString();
      this.db.prepare('UPDATE runs SET document = ?, updated_at = ? WHERE id = ?').run(encode(final, MAX_RUN_JSON_BYTES), final.updatedAt, run.id);
      return final;
    });
  }

  acquireSlot(instanceId: string, runId: string, limit: number): string | undefined {
    text(instanceId, 'Instance id', 128);
    text(runId, 'Run id', 256);
    if (!Number.isInteger(limit) || limit < 1 || limit > 32) throw new Error('The shared concurrency limit must be from 1 to 32.');
    return this.transaction(() => {
      this.assertInstance(instanceId);
      this.recoverDeadOwners();
      const row = this.ownedRun(runId);
      if (!row || row.owner_instance !== instanceId) throw new Error('The Council instance does not own this run.');
      if (!['queued', 'running'].includes((JSON.parse(row.document) as RunRecord).status)) throw new Error('This run is no longer active.');
      if (row.cancel_requested || this.legacyCancellation(runId)) throw new Error('Cancellation has been requested for this run.');
      this.db.prepare('UPDATE instances SET slot_limit = ? WHERE id = ?').run(limit, instanceId);
      const minimum = this.db.prepare('SELECT MIN(slot_limit) AS value FROM instances WHERE slot_limit IS NOT NULL').get() as { value: number };
      const active = this.db.prepare('SELECT COUNT(*) AS value FROM slots').get() as { value: number };
      if (active.value >= minimum.value) return undefined;
      const token = randomUUID();
      this.db.prepare('INSERT INTO slots(token, instance_id, run_id, acquired_at) VALUES (?, ?, ?, ?)').run(token, instanceId, runId, Date.now());
      return token;
    });
  }

  releaseSlot(token: string, instanceId: string): void {
    text(token, 'Slot token', 256);
    text(instanceId, 'Instance id', 128);
    this.db.prepare('DELETE FROM slots WHERE token = ? AND instance_id = ?').run(token, instanceId);
  }

  /** Heartbeat age alone is never proof that a process is dead. PID reuse is intentionally conservative. */
  recoverOrphans(): RunRecord[] { return this.transaction(() => this.recoverDeadOwners()); }

  private instance(instanceId: string): InstanceRow | undefined {
    return this.db.prepare('SELECT id, pid, heartbeat_at, slot_limit FROM instances WHERE id = ?').get(instanceId) as InstanceRow | undefined;
  }

  private assertInstance(instanceId: string): InstanceRow {
    const instance = this.instance(instanceId);
    if (!instance || instance.pid !== process.pid || !processAlive(instance.pid)) throw new Error('The Council owner instance is not registered to this live process.');
    return instance;
  }

  private ownedRun(runId: string): OwnedRunRow | undefined {
    return this.db.prepare('SELECT document, owner_instance, owner_pid, cancel_requested FROM runs WHERE id = ?').get(runId) as OwnedRunRow | undefined;
  }

  private assertRunOwner(row: OwnedRunRow): void {
    if (!row.owner_instance) return;
    const owner = this.instance(row.owner_instance);
    if (!owner || owner.pid !== process.pid || owner.pid !== row.owner_pid) throw new Error('This process no longer owns the Council run.');
  }

  private legacyCancellation(runId: string): boolean {
    return this.db.prepare("SELECT 1 FROM events WHERE run_id = ? AND type = 'cancel_requested' LIMIT 1").get(runId) !== undefined;
  }

  private recoverDeadOwners(): RunRecord[] {
    const instances = this.db.prepare('SELECT id, pid, heartbeat_at, slot_limit FROM instances').all() as InstanceRow[];
    const alive = new Map(instances.map(instance => [instance.id, processAlive(instance.pid)]));
    const dead = new Set(instances.filter(instance => !alive.get(instance.id)).map(instance => instance.id));
    const rows = this.db.prepare("SELECT document, owner_instance, owner_pid, cancel_requested FROM runs WHERE owner_instance IS NOT NULL AND json_extract(document, '$.status') IN ('queued', 'running')").all() as OwnedRunRow[];
    const recovered: RunRecord[] = [];
    for (const row of rows) {
      const run = JSON.parse(row.document) as RunRecord;
      if (!['queued', 'running'].includes(run.status) || !row.owner_instance || row.owner_pid === null) continue;
      const ownerAlive = alive.get(row.owner_instance) ?? processAlive(row.owner_pid);
      if (ownerAlive) continue;
      dead.add(row.owner_instance);
      run.status = 'failed';
      run.error = 'The Council owner process exited before this workflow finished. Inspect retained worktrees before starting a replacement; no task was retried.';
      run.updatedAt = new Date().toISOString();
      this.db.prepare('UPDATE runs SET document = ?, updated_at = ? WHERE id = ?').run(encode(run, MAX_RUN_JSON_BYTES), run.updatedAt, run.id);
      this.appendEvent(run.id, 'run.failed', { reason: 'owner_exited', status: 'failed' });
      recovered.push(run);
    }
    for (const instanceId of dead) {
      this.db.prepare('DELETE FROM slots WHERE instance_id = ?').run(instanceId);
      this.db.prepare('DELETE FROM instances WHERE id = ?').run(instanceId);
    }
    return recovered;
  }

  close(): void { this.db.close(); }
}
