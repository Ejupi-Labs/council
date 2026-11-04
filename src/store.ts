import { randomUUID } from 'node:crypto';
import { chmodSync, closeSync, fsyncSync, linkSync, lstatSync, mkdirSync, mkdtempSync, openSync, realpathSync, rmSync, statSync } from 'node:fs';
import { basename, dirname, join, resolve } from 'node:path';
import { backup, DatabaseSync, type StatementSync } from 'node:sqlite';
import type { ContextEntry, EventRecord, Lease, Message, RunRecord, WorkspaceArtifact } from './types.js';
import { MAX_EVENT_PAGE_BYTES, summarizeEvent, summarizeRun, type EventPage, type PruneResult, type RunSummary } from './summary.js';

const SCHEMA_VERSION = 5;
const MAX_JSON_BYTES = 4 * 1024 * 1024;
const MAX_RUN_JSON_BYTES = 16 * 1024 * 1024;

function requireSafeSqliteRuntime(): string {
  const version = process.versions.sqlite;
  const match = typeof version === 'string' ? /^(\d+)\.(\d+)\.(\d+)(?:\.\d+)?$/.exec(version) : null;
  const major = Number(match?.[1]);
  const minor = Number(match?.[2]);
  const patch = Number(match?.[3]);
  // https://sqlite.org/wal.html#walresetbug documents the multi-writer WAL reset fix.
  if (!match || !(major > 3 || (major === 3 && (minor > 51 || (minor === 51 && patch >= 3))))) {
    throw new Error(`Council requires SQLite 3.51.3 or newer for safe multi-process WAL writes; this runtime reports ${version ?? 'unknown'}. Upgrade Node.js to 24.18.1 or newer with a supported SQLite build before opening Council state.`);
  }
  return version!;
}

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

function cutoffTimestamp(value: string): string {
  text(value, 'Prune cutoff', 64);
  const match = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.\d{1,3})?(?:Z|[+-]\d{2}:\d{2})$/.exec(value);
  if (!match) throw new Error('Prune cutoff must be a valid ISO datetime with a timezone.');
  const [, year, month, day, hour, minute, second] = match;
  const date = new Date(value);
  const days = new Date(Date.UTC(Number(year), Number(month), 0)).getUTCDate();
  if (!Number.isFinite(date.getTime()) || Number(month) < 1 || Number(month) > 12 || Number(day) < 1 || Number(day) > days || Number(hour) > 23 || Number(minute) > 59 || Number(second) > 59) throw new Error('Prune cutoff must be a valid ISO datetime with a timezone.');
  return date.toISOString();
}

type OwnedRunRow = { status: RunRecord['status']; revision: number; owner_instance: string | null; owner_pid: number | null; cancel_requested: number };
type InstanceRow = { id: string; pid: number; heartbeat_at: number; slot_limit: number | null };

export class RunEventsUnavailableError extends Error {
  readonly status = 404;
  readonly code = 'COUNCIL_RUN_EVENTS_UNAVAILABLE';
  constructor() {
    super('Workflow not found (404). Its stored record and event history may have been pruned.');
    this.name = 'RunEventsUnavailableError';
  }
}

/** Durable collaboration state shared by all Council processes in one workspace. */
export class CouncilStore {
  private readonly db: DatabaseSync;
  private readonly databasePath: string;
  private readonly sqliteVersion: string;
  private readonly statements = new Map<string, StatementSync>();
  private boundInstance?: string;
  private lastRecoveryAt = Number.NEGATIVE_INFINITY;
  private activeBackups = 0;

  constructor(stateDir: string) {
    this.sqliteVersion = requireSafeSqliteRuntime();
    mkdirSync(stateDir, { recursive: true, mode: 0o700 });
    const databasePath = this.databasePath = join(resolve(stateDir), 'state.db');
    this.db = new DatabaseSync(databasePath);
    try {
      this.db.exec('PRAGMA busy_timeout = 250; PRAGMA foreign_keys = ON;');
      // Journal mode is persistent: reject unsupported state before changing it.
      const initial = this.statement('PRAGMA user_version').get() as { user_version: number };
      if (initial.user_version > SCHEMA_VERSION) throw new Error(`Council state schema ${initial.user_version} is newer than supported schema ${SCHEMA_VERSION}`);
      this.restrictDatabaseFiles();
      const journal = this.statement('PRAGMA journal_mode').get() as { journal_mode: string };
      if (initial.user_version !== SCHEMA_VERSION || journal.journal_mode !== 'wal') {
        // Initialization runs before vendor work; runtime writes retain their short contention bound.
        const deadline = performance.now() + 5000;
        this.retryStartup(() => this.db.exec('PRAGMA journal_mode = WAL;'), deadline);
        this.retryStartup(() => this.transaction(() => {
        // Recheck under the migration lock: another process may have upgraded while opening.
        const version = this.statement('PRAGMA user_version').get() as { user_version: number };
        if (version.user_version > SCHEMA_VERSION) throw new Error(`Council state schema ${version.user_version} is newer than supported schema ${SCHEMA_VERSION}`);
        if (version.user_version === SCHEMA_VERSION) return;
        if (version.user_version < SCHEMA_VERSION && this.statement("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = 'instances'").get()) {
          const owners = this.statement('SELECT pid FROM instances').all() as { pid: number }[];
          if (owners.some(owner => processAlive(owner.pid))) throw new Error('Stop older Council hosts using this workspace before upgrading storage, then retry.');
        }
        this.db.exec(`
          CREATE TABLE IF NOT EXISTS runs (id TEXT PRIMARY KEY, updated_at TEXT NOT NULL, document TEXT NOT NULL);
          CREATE TABLE IF NOT EXISTS events (sequence INTEGER PRIMARY KEY AUTOINCREMENT, run_id TEXT NOT NULL, type TEXT NOT NULL, data TEXT NOT NULL, created_at TEXT NOT NULL);
          CREATE INDEX IF NOT EXISTS events_run_sequence ON events(run_id, sequence);
          CREATE TABLE IF NOT EXISTS messages (id TEXT PRIMARY KEY, sender TEXT NOT NULL, recipient TEXT NOT NULL, created_at TEXT NOT NULL, acknowledged_at TEXT, document TEXT NOT NULL);
          CREATE INDEX IF NOT EXISTS messages_inbox ON messages(recipient, created_at);
          CREATE INDEX IF NOT EXISTS messages_unread ON messages(recipient, created_at) WHERE acknowledged_at IS NULL;
          CREATE INDEX IF NOT EXISTS messages_recipient_page ON messages(recipient, created_at, id);
          CREATE INDEX IF NOT EXISTS messages_unread_page ON messages(recipient, created_at, id) WHERE acknowledged_at IS NULL;
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
          CREATE TABLE IF NOT EXISTS instances (id TEXT PRIMARY KEY, pid INTEGER NOT NULL, registered_at INTEGER NOT NULL, heartbeat_at INTEGER NOT NULL, slot_limit INTEGER);
          CREATE TABLE IF NOT EXISTS slots (token TEXT PRIMARY KEY, instance_id TEXT NOT NULL REFERENCES instances(id) ON DELETE CASCADE, run_id TEXT NOT NULL, acquired_at INTEGER NOT NULL);
          CREATE INDEX IF NOT EXISTS slots_instance ON slots(instance_id);
        `);
        if (version.user_version < 3) this.migrateMetadata();
        if (version.user_version < 5) this.db.exec('ALTER TABLE leases ADD COLUMN instance_id TEXT REFERENCES instances(id) ON DELETE CASCADE;');
        this.db.exec(`
          DROP INDEX IF EXISTS runs_unfinished_owner;
          CREATE INDEX IF NOT EXISTS runs_active_owner ON runs(owner_instance, status) WHERE status IN ('queued', 'running');
          CREATE INDEX IF NOT EXISTS runs_recent ON runs(updated_at DESC, id DESC);
          CREATE INDEX IF NOT EXISTS runs_metadata_lookup ON runs(id, status, revision, owner_instance, owner_pid, cancel_requested, summary);
          CREATE INDEX IF NOT EXISTS runs_recent_summary ON runs(updated_at DESC, id DESC, summary, revision, cancel_requested);
          CREATE INDEX IF NOT EXISTS runs_live_metadata ON runs(owner_instance, status, id, revision, owner_pid, cancel_requested) WHERE status IN ('queued', 'running');
          CREATE INDEX IF NOT EXISTS runs_status ON runs(status);
          CREATE INDEX IF NOT EXISTS runs_prunable ON runs(status, has_artifacts, updated_at, id);
          CREATE INDEX IF NOT EXISTS events_run_type_sequence ON events(run_id, type, sequence);
          CREATE INDEX IF NOT EXISTS events_compact_page ON events(run_id, sequence, type, summary, created_at);
          CREATE INDEX IF NOT EXISTS leases_instance ON leases(instance_id) WHERE instance_id IS NOT NULL;
          CREATE INDEX IF NOT EXISTS leases_expiry ON leases(expires_at);
          CREATE TABLE IF NOT EXISTS slot_waiters (
            sequence INTEGER PRIMARY KEY AUTOINCREMENT,
            instance_id TEXT NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
            run_id TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
            waiter_id TEXT NOT NULL,
            UNIQUE(instance_id, waiter_id)
          );
          CREATE INDEX IF NOT EXISTS slot_waiters_run ON slot_waiters(run_id);
          PRAGMA user_version = ${SCHEMA_VERSION};
        `);
        if (version.user_version < 3) this.db.exec("UPDATE runs SET cancel_requested = 1 WHERE cancel_requested = 0 AND EXISTS (SELECT 1 FROM events WHERE events.run_id = runs.id AND events.type = 'cancel_requested')");
        }), deadline);
        this.db.exec('PRAGMA busy_timeout = 250;');
      }
      this.restrictDatabaseFiles();
    } catch (error) {
      this.db.close();
      throw error;
    }
  }

  /** WAL lock upgrades can bypass SQLite's busy handler; retry only startup SQL within one deadline. */
  private retryStartup<T>(operation: () => T, deadline: number): T {
    const wait = new Int32Array(new SharedArrayBuffer(4));
    for (;;) {
      this.db.exec(`PRAGMA busy_timeout = ${Math.max(1, Math.ceil(deadline - performance.now()))};`);
      try { return operation(); }
      catch (error) {
        const remaining = deadline - performance.now();
        const code = error && typeof error === 'object' && 'errcode' in error ? error.errcode : undefined;
        if (typeof code !== 'number' || (code & 0xff) !== 5 || remaining <= 0) throw error;
        Atomics.wait(wait, 0, 0, Math.min(25, remaining));
      }
    }
  }

  /** SQLite inherits the database mode when creating WAL/SHM beside existing state directories. */
  private restrictDatabaseFiles(): void {
    if (process.platform === 'win32') return;
    chmodSync(this.databasePath, 0o600);
    for (const path of [`${this.databasePath}-wal`, `${this.databasePath}-shm`]) {
      try { chmodSync(path, 0o600); }
      catch (error) { if ((error as NodeJS.ErrnoException).code !== 'ENOENT') throw error; }
    }
  }

  /** Fixed SQL only, with a bounded cache rather than re-preparing every polling query. */
  private statement(sql: string): StatementSync {
    const existing = this.statements.get(sql);
    if (existing) return existing;
    if (this.statements.size >= 96) this.statements.delete(this.statements.keys().next().value!);
    const prepared = this.db.prepare(sql);
    this.statements.set(sql, prepared);
    return prepared;
  }

  private migrateMetadata(): void {
    this.db.exec(`
      ALTER TABLE runs ADD COLUMN status TEXT NOT NULL DEFAULT 'queued';
      ALTER TABLE runs ADD COLUMN revision INTEGER NOT NULL DEFAULT 1;
      ALTER TABLE runs ADD COLUMN summary TEXT NOT NULL DEFAULT '{}';
      ALTER TABLE runs ADD COLUMN has_artifacts INTEGER NOT NULL DEFAULT 0;
      ALTER TABLE events ADD COLUMN summary TEXT NOT NULL DEFAULT 'null';
    `);
    let id = '';
    for (;;) {
      const row = this.statement('SELECT id, document FROM runs WHERE id > ? ORDER BY id LIMIT 1').get(id) as { id: string; document: string } | undefined;
      if (!row) break;
      const run = JSON.parse(row.document) as RunRecord;
      this.statement('UPDATE runs SET status = ?, revision = 1, summary = ?, has_artifacts = ? WHERE id = ?')
        .run(run.status, encode(summarizeRun(run)), (run.workspaces?.length ?? 0) > 0 ? 1 : 0, row.id);
      id = row.id;
    }
    let sequence = 0;
    for (;;) {
      const row = this.statement('SELECT sequence, run_id, type, data, created_at FROM events WHERE sequence > ? ORDER BY sequence LIMIT 1').get(sequence) as { sequence: number; run_id: string; type: string; data: string; created_at: string } | undefined;
      if (!row) break;
      const event: EventRecord = { sequence: row.sequence, runId: row.run_id, type: row.type, data: JSON.parse(row.data), createdAt: row.created_at };
      this.statement('UPDATE events SET summary = ? WHERE sequence = ?').run(encode(summarizeEvent(event).data), row.sequence);
      sequence = row.sequence;
    }
  }

  private transaction<T>(operation: () => T, write = true): T {
    this.db.exec(write ? 'BEGIN IMMEDIATE' : 'BEGIN');
    try {
      const result = operation();
      this.db.exec('COMMIT');
      return result;
    } catch (error) {
      // SQLite can auto-rollback after I/O/full errors. Preserve that original failure.
      try { if (this.db.isTransaction) this.db.exec('ROLLBACK'); } catch { /* The original error remains the primary diagnostic. */ }
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
      if (existing && !['queued', 'running'].includes(existing.status)) throw new Error('Terminal run records are immutable.');
      // Cancellation and owner columns are deliberately absent from the partial-update clause.
      const revision = (existing?.revision ?? 0) + 1;
      this.statement('INSERT INTO runs(id, updated_at, document, status, revision, summary, has_artifacts) VALUES (?, ?, ?, ?, ?, ?, ?) ON CONFLICT(id) DO UPDATE SET updated_at = excluded.updated_at, document = excluded.document, status = excluded.status, revision = excluded.revision, summary = excluded.summary, has_artifacts = excluded.has_artifacts')
        .run(run.id, run.updatedAt, document, run.status, revision, encode(summarizeRun(run, revision)), (run.workspaces?.length ?? 0) > 0 ? 1 : 0);
      if (!['queued', 'running'].includes(run.status)) this.statement('DELETE FROM slot_waiters WHERE run_id = ?').run(run.id);
    });
  }

  /** Creation, ownership and the queued event commit together, leaving no unowned crash window. */
  createRun(run: RunRecord, instanceId: string, initialEvents: Array<{ type: string; data: unknown }> = []): void {
    text(run.id, 'Run id', 256);
    text(run.updatedAt, 'Run timestamp', 64);
    text(instanceId, 'Instance id', 128);
    if (run.status !== 'queued') throw new Error('A new owned run must begin in the queued state.');
    if (!Array.isArray(initialEvents) || initialEvents.length > 16) throw new Error('A new run can contain at most 16 initial events.');
    for (const event of initialEvents) { text(event.type, 'Event type', 128); encode(event.data); }
    const document = encode(run, MAX_RUN_JSON_BYTES);
    const summary = encode(summarizeRun(run, 1));
    this.transaction(() => {
      const instance = this.assertInstance(instanceId);
      if (this.boundInstance !== instanceId) throw new Error('The store connection is not bound to this Council instance.');
      if (this.ownedRun(run.id)) throw new Error('A Council run with this ID already exists.');
      const active = this.statement("SELECT COUNT(*) AS value FROM runs WHERE status IN ('queued', 'running')").get() as { value: number };
      if (active.value >= 100) throw new Error('This workspace already has 100 active Council runs. Wait for completion or inspect abandoned work before starting more.');
      this.statement('INSERT INTO runs(id, updated_at, document, owner_instance, owner_pid, status, revision, summary, has_artifacts, cancel_requested) VALUES (?, ?, ?, ?, ?, ?, 1, ?, ?, ?)')
        .run(run.id, run.updatedAt, document, instanceId, instance.pid, run.status, summary, (run.workspaces?.length ?? 0) > 0 ? 1 : 0, this.legacyCancellation(run.id) ? 1 : 0);
      this.writeEvent(run.id, 'queued', { mode: run.input.mode });
      for (const event of initialEvents) this.writeEvent(run.id, event.type, event.data);
    });
  }

  getRun(id: string): RunRecord | undefined {
    text(id, 'Run id', 256);
    const row = this.statement('SELECT document FROM runs WHERE id = ?').get(id) as { document: string } | undefined;
    return row ? JSON.parse(row.document) as RunRecord : undefined;
  }

  listRuns(limit = 20): RunRecord[] {
    if (!Number.isInteger(limit) || limit < 1) throw new Error('Run list limit must be a positive integer');
    const rows = this.statement('SELECT document FROM runs ORDER BY updated_at DESC, id LIMIT ?').all(Math.min(limit, 100)) as { document: string }[];
    return rows.map(row => JSON.parse(row.document) as RunRecord);
  }

  getRunSummary(id: string): RunSummary | undefined {
    text(id, 'Run id', 256);
    const row = this.statement('SELECT summary, revision, cancel_requested FROM runs INDEXED BY runs_metadata_lookup WHERE id = ?').get(id) as { summary: string; revision: number; cancel_requested: number } | undefined;
    return row ? { ...JSON.parse(row.summary) as RunSummary, revision: row.revision, cancellationRequested: Boolean(row.cancel_requested) } : undefined;
  }

  listRunSummaries(limit = 20, before?: { updatedAt: string; id: string }): RunSummary[] {
    if (!Number.isInteger(limit) || limit < 1 || limit > 100) throw new Error('Run summary limit must be from 1 to 100.');
    if (before) { text(before.updatedAt, 'Run cursor timestamp', 64); text(before.id, 'Run cursor id', 256); }
    const rows = (before
      ? this.statement('SELECT summary, revision, cancel_requested FROM runs INDEXED BY runs_recent_summary WHERE (updated_at, id) < (?, ?) ORDER BY updated_at DESC, id DESC LIMIT ?').all(before.updatedAt, before.id, limit)
      : this.statement('SELECT summary, revision, cancel_requested FROM runs INDEXED BY runs_recent_summary ORDER BY updated_at DESC, id DESC LIMIT ?').all(limit)) as { summary: string; revision: number; cancel_requested: number }[];
    return rows.map(row => ({ ...JSON.parse(row.summary) as RunSummary, revision: row.revision, cancellationRequested: Boolean(row.cancel_requested) }));
  }

  appendEvent(runId: string, type: string, data: unknown): EventRecord {
    text(runId, 'Run id', 256);
    text(type, 'Event type', 128);
    return type === 'cancel_requested' && !this.db.isTransaction
      ? this.transaction(() => this.writeEvent(runId, type, data))
      : this.writeEvent(runId, type, data);
  }

  private writeEvent(runId: string, type: string, data: unknown): EventRecord {
    const createdAt = new Date().toISOString();
    const event: EventRecord = { sequence: 0, runId, type, data, createdAt };
    const result = this.statement('INSERT INTO events(run_id, type, data, created_at, summary) VALUES (?, ?, ?, ?, ?)')
      .run(runId, type, encode(data), createdAt, encode(summarizeEvent(event).data));
    if (type === 'cancel_requested') {
      this.statement('UPDATE runs SET cancel_requested = 1, revision = revision + 1 WHERE id = ? AND cancel_requested = 0').run(runId);
      this.statement('DELETE FROM slot_waiters WHERE run_id = ?').run(runId);
    }
    return { ...event, sequence: Number(result.lastInsertRowid) };
  }

  events(runId: string, after = 0): EventRecord[] {
    text(runId, 'Run id', 256);
    if (!Number.isSafeInteger(after) || after < 0) throw new Error('Event cursor must be a non-negative integer');
    const rows = this.statement('SELECT sequence, run_id, type, data, created_at FROM events WHERE run_id = ? AND sequence > ? ORDER BY sequence LIMIT 1000').all(runId, after) as {
      sequence: number; run_id: string; type: string; data: string; created_at: string;
    }[];
    return rows.map(row => ({ sequence: row.sequence, runId: row.run_id, type: row.type, data: JSON.parse(row.data), createdAt: row.created_at }));
  }

  /** Complete event payloads, with indexed exact-type filtering and stable ascending cursors. */
  eventPage(runId: string, after = 0, limit = 100, type?: string): EventPage {
    text(runId, 'Run id', 256);
    if (!Number.isSafeInteger(after) || after < 0) throw new Error('Event cursor must be a non-negative safe integer.');
    if (!Number.isInteger(limit) || limit < 1 || limit > 100) throw new Error('Full event page limit must be from 1 to 100.');
    if (type !== undefined) text(type, 'Event type', 128);
    const read = (): EventPage => {
      // One snapshot keeps existence, matching keys and complete payloads consistent with pruning.
      if (!this.statement('SELECT 1 FROM runs WHERE id = ?').get(runId)) throw new RunEventsUnavailableError();
      const keys = (type === undefined
        ? this.statement('SELECT sequence FROM events INDEXED BY events_run_sequence WHERE run_id = ? AND sequence > ? ORDER BY sequence LIMIT ?').all(runId, after, limit + 1)
        : this.statement('SELECT sequence FROM events INDEXED BY events_run_type_sequence WHERE run_id = ? AND type = ? AND sequence > ? ORDER BY sequence LIMIT ?').all(runId, type, after, limit + 1)) as { sequence: number }[];
      const events: EventRecord[] = [];
      let bytes = 2; // The events array brackets.
      for (const key of keys.slice(0, limit)) {
        // Inspect only the candidate size before reading/decoding its JSON. A large
        // first event is returned intact alone; a later one remains at the cursor.
        const row = this.statement('SELECT type, created_at, length(CAST(data AS BLOB)) AS data_bytes FROM events WHERE sequence = ? AND run_id = ?')
          .get(key.sequence, runId) as { type: string; created_at: string; data_bytes: number } | undefined;
        if (!row) throw new Error('Workflow event metadata is unavailable in the current storage snapshot.');
        const metadata = { sequence: key.sequence, runId, type: row.type, data: null, createdAt: row.created_at };
        const eventBytes = Buffer.byteLength(JSON.stringify(metadata), 'utf8') - 4 + row.data_bytes + (events.length ? 1 : 0);
        if (events.length && bytes + eventBytes > MAX_EVENT_PAGE_BYTES) break;
        const payload = this.statement('SELECT data FROM events WHERE sequence = ? AND run_id = ?').get(key.sequence, runId) as { data: string } | undefined;
        if (!payload) throw new Error('Workflow event payload is unavailable in the current storage snapshot.');
        events.push({ ...metadata, data: JSON.parse(payload.data) });
        bytes += eventBytes;
        if (bytes >= MAX_EVENT_PAGE_BYTES) break;
      }
      const nextAfter = events.at(-1)?.sequence ?? after;
      const hasMore = keys.length > events.length;
      return { events, nextAfter, hasMore, nextCursor: hasMore ? nextAfter : null };
    };
    return this.db.isTransaction ? read() : this.transaction(read, false);
  }

  compactEvents(runId: string, after = 0, limit = 100): EventRecord[] {
    text(runId, 'Run id', 256);
    if (!Number.isSafeInteger(after) || after < 0) throw new Error('Event cursor must be a non-negative integer.');
    if (!Number.isInteger(limit) || limit < 1 || limit > 1000) throw new Error('Event page limit must be from 1 to 1000.');
    const rows = this.statement('SELECT sequence, run_id, type, summary, created_at FROM events INDEXED BY events_compact_page WHERE run_id = ? AND sequence > ? ORDER BY sequence LIMIT ?').all(runId, after, limit) as { sequence: number; run_id: string; type: string; summary: string; created_at: string }[];
    return rows.map(row => ({ sequence: row.sequence, runId: row.run_id, type: row.type, data: JSON.parse(row.summary), createdAt: row.created_at }));
  }

  isCancellationRequested(runId: string): boolean {
    text(runId, 'Run id', 256);
    const row = this.statement("SELECT EXISTS(SELECT 1 FROM runs INDEXED BY runs_metadata_lookup WHERE id = ? AND cancel_requested = 1) OR EXISTS(SELECT 1 FROM events WHERE run_id = ? AND type = 'cancel_requested') AS requested").get(runId, runId) as { requested: number };
    return Boolean(row.requested);
  }

  sendMessage(input: { from: string; to: string; subject: string; body: string; runId?: string }): Message {
    text(input.from, 'Sender', 128);
    text(input.to, 'Recipient', 128);
    text(input.subject, 'Message subject', 512);
    text(input.body, 'Message body', MAX_JSON_BYTES);
    if (input.runId !== undefined) text(input.runId, 'Run id', 256);
    const message: Message = { ...input, id: randomUUID(), createdAt: new Date().toISOString() };
    this.statement('INSERT INTO messages(id, sender, recipient, created_at, document) VALUES (?, ?, ?, ?, ?)')
      .run(message.id, message.from, message.to, message.createdAt, encode(message));
    return message;
  }

  inbox(agent: string, unreadOnly = true): Message[] {
    text(agent, 'Agent', 128);
    const rows = (unreadOnly
      ? this.statement('SELECT document, acknowledged_at FROM messages INDEXED BY messages_unread WHERE recipient = ? AND acknowledged_at IS NULL ORDER BY created_at, rowid LIMIT 100').all(agent)
      : this.statement('SELECT document, acknowledged_at FROM messages WHERE recipient = ? ORDER BY created_at, rowid LIMIT 100').all(agent)) as { document: string; acknowledged_at: string | null }[];
    return this.decodeMessages(rows);
  }

  /** Complete message bodies with stable timestamp/ID keyset pagination. */
  inboxPage(agent: string, unreadOnly = true, limit = 20, after?: { createdAt: string; id: string }): Message[] {
    text(agent, 'Agent', 128);
    if (typeof unreadOnly !== 'boolean') throw new Error('Unread-only must be a boolean.');
    if (!Number.isInteger(limit) || limit < 1 || limit > 100) throw new Error('Inbox page limit must be from 1 to 100.');
    if (after) { text(after.createdAt, 'Inbox cursor timestamp', 64); text(after.id, 'Inbox cursor id', 256); }
    const rows = (unreadOnly
      ? after
        ? this.statement('SELECT document, acknowledged_at FROM messages INDEXED BY messages_unread_page WHERE recipient = ? AND acknowledged_at IS NULL AND (created_at, id) > (?, ?) ORDER BY created_at, id LIMIT ?').all(agent, after.createdAt, after.id, limit)
        : this.statement('SELECT document, acknowledged_at FROM messages INDEXED BY messages_unread_page WHERE recipient = ? AND acknowledged_at IS NULL ORDER BY created_at, id LIMIT ?').all(agent, limit)
      : after
        ? this.statement('SELECT document, acknowledged_at FROM messages INDEXED BY messages_recipient_page WHERE recipient = ? AND (created_at, id) > (?, ?) ORDER BY created_at, id LIMIT ?').all(agent, after.createdAt, after.id, limit)
        : this.statement('SELECT document, acknowledged_at FROM messages INDEXED BY messages_recipient_page WHERE recipient = ? ORDER BY created_at, id LIMIT ?').all(agent, limit)) as { document: string; acknowledged_at: string | null }[];
    return this.decodeMessages(rows);
  }

  private decodeMessages(rows: { document: string; acknowledged_at: string | null }[]): Message[] {
    return rows.map(row => {
      const message = JSON.parse(row.document) as Message;
      if (row.acknowledged_at) message.acknowledgedAt = row.acknowledged_at;
      return message;
    });
  }

  acknowledge(id: string, agent: string): boolean {
    text(id, 'Message id', 256);
    text(agent, 'Agent', 128);
    const result = this.statement('UPDATE messages SET acknowledged_at = ? WHERE id = ? AND recipient = ? AND acknowledged_at IS NULL')
      .run(new Date().toISOString(), id, agent);
    return result.changes > 0;
  }

  reserve(owner: string, paths: string[], ttlMs: number, runId?: string, instanceId?: string): Lease[] {
    text(owner, 'Lease owner', 128);
    if (!Array.isArray(paths) || paths.length < 1 || paths.length > 100) throw new Error('Reserve between 1 and 100 paths');
    if (!Number.isSafeInteger(ttlMs) || ttlMs < 1 || ttlMs > 24 * 60 * 60 * 1000) throw new Error('Lease TTL must be between 1 ms and 24 hours');
    if (runId !== undefined) text(runId, 'Run id', 256);
    if (instanceId !== undefined) text(instanceId, 'Lease instance id', 128);
    const normalized = [...new Map(paths.map(path => { const value = normalizePath(path); return [value.toLowerCase(), value]; })).values()];
    return this.transaction(() => {
      if (instanceId !== undefined) {
        this.assertInstance(instanceId);
        if (this.boundInstance !== instanceId) throw new Error('The store connection is not bound to this lease owner instance.');
      }
      const now = Date.now();
      this.statement('DELETE FROM leases WHERE expires_at <= ?').run(now);
      const active = this.readLeases(now);
      for (const requested of normalized) {
        const conflict = active.find(lease => (lease.owner !== owner || lease.runId !== runId || lease.instanceId !== instanceId) && lease.paths.some(path => overlaps(path, requested)));
        if (conflict) throw new Error(`Reservation conflict: ${requested} overlaps ${conflict.paths[0]} owned by ${conflict.owner} until ${conflict.expiresAt}`);
      }
      const expiresAt = new Date(now + ttlMs).toISOString();
      return normalized.map(path => {
        const existing = active.find(lease => lease.owner === owner && lease.paths[0]?.toLowerCase() === path.toLowerCase());
        const lease: Lease = { id: existing?.id ?? randomUUID(), owner, paths: [path], expiresAt, ...(runId === undefined ? {} : { runId }), ...(instanceId === undefined ? {} : { instanceId }) };
        this.statement('INSERT INTO leases(id, owner, path, path_key, run_id, expires_at, instance_id) VALUES (?, ?, ?, ?, ?, ?, ?) ON CONFLICT(path_key) DO UPDATE SET id = excluded.id, owner = excluded.owner, path = excluded.path, run_id = excluded.run_id, expires_at = excluded.expires_at, instance_id = excluded.instance_id')
          .run(lease.id, owner, path, path.toLowerCase(), runId ?? null, now + ttlMs, instanceId ?? null);
        return lease;
      });
    });
  }

  release(owner: string, paths?: string[], runId?: string): number {
    text(owner, 'Lease owner', 128);
    if (runId !== undefined) text(runId, 'Run id', 256);
    if (paths === undefined) return Number(runId === undefined
      ? this.statement('DELETE FROM leases WHERE owner = ?').run(owner).changes
      : this.statement('DELETE FROM leases WHERE owner = ? AND run_id = ?').run(owner, runId).changes);
    if (!Array.isArray(paths) || paths.length > 100) throw new Error('Release at most 100 paths');
    const normalized = paths.map(normalizePath);
    return this.transaction(() => normalized.reduce((count, path) => count + Number(runId === undefined
      ? this.statement('DELETE FROM leases WHERE owner = ? AND path_key = ?').run(owner, path.toLowerCase()).changes
      : this.statement('DELETE FROM leases WHERE owner = ? AND path_key = ? AND run_id = ?').run(owner, path.toLowerCase(), runId).changes), 0));
  }

  private readLeases(now = Date.now()): Lease[] {
    const rows = this.statement('SELECT id, owner, path, run_id, expires_at, instance_id FROM leases WHERE expires_at > ? ORDER BY path_key').all(now) as {
      id: string; owner: string; path: string; run_id: string | null; expires_at: number; instance_id: string | null;
    }[];
    return rows.map(row => ({ id: row.id, owner: row.owner, paths: [row.path], expiresAt: new Date(row.expires_at).toISOString(), ...(row.run_id === null ? {} : { runId: row.run_id }), ...(row.instance_id === null ? {} : { instanceId: row.instance_id }) }));
  }

  leases(): Lease[] {
    return this.readLeases();
  }

  putContext(key: string, value: string, author: string): ContextEntry {
    text(key, 'Context key', 256);
    text(value, 'Context value', MAX_JSON_BYTES);
    text(author, 'Context author', 128);
    const entry = { key, value, author, updatedAt: new Date().toISOString() };
    encode(entry); // Apply the same explicit record-size guard used by peer messages.
    this.statement('INSERT INTO context(key, value, author, updated_at) VALUES (?, ?, ?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value, author = excluded.author, updated_at = excluded.updated_at')
      .run(key, value, author, entry.updatedAt);
    return entry;
  }

  getContext(key: string): ContextEntry | undefined {
    text(key, 'Context key', 256);
    const row = this.statement('SELECT key, value, author, updated_at FROM context WHERE key = ?').get(key) as { key: string; value: string; author: string; updated_at: string } | undefined;
    return row ? { key: row.key, value: row.value, author: row.author, updatedAt: row.updated_at } : undefined;
  }

  listContext(): ContextEntry[] {
    const rows = this.statement('SELECT key, value, author, updated_at FROM context ORDER BY updated_at DESC, key LIMIT 100').all() as { key: string; value: string; author: string; updated_at: string }[];
    return rows.map(row => ({ key: row.key, value: row.value, author: row.author, updatedAt: row.updated_at }));
  }

  registerInstance(instanceId: string, pid: number, slotLimit?: number): void {
    text(instanceId, 'Instance id', 128);
    if (!Number.isSafeInteger(pid) || pid !== process.pid) throw new Error('A Council instance must register its own process PID.');
    if (slotLimit !== undefined && (!Number.isInteger(slotLimit) || slotLimit < 1 || slotLimit > 32)) throw new Error('The shared concurrency limit must be from 1 to 32.');
    this.transaction(() => {
      const existing = this.instance(instanceId);
      if (existing && existing.pid !== pid) throw new Error('This Council instance ID is already registered by another process.');
      const now = Date.now();
      this.statement('INSERT INTO instances(id, pid, registered_at, heartbeat_at, slot_limit) VALUES (?, ?, ?, ?, ?) ON CONFLICT(id) DO UPDATE SET heartbeat_at = excluded.heartbeat_at, slot_limit = COALESCE(excluded.slot_limit, instances.slot_limit)')
        .run(instanceId, pid, now, now, slotLimit ?? null);
    });
    this.boundInstance ??= instanceId;
  }

  bindInstance(instanceId: string): void {
    text(instanceId, 'Instance id', 128);
    this.assertInstance(instanceId);
    this.boundInstance = instanceId;
  }

  heartbeat(instanceId: string): boolean {
    text(instanceId, 'Instance id', 128);
    return this.statement('UPDATE instances SET heartbeat_at = ? WHERE id = ? AND pid = ?').run(Date.now(), instanceId, process.pid).changes > 0;
  }

  unregisterInstance(instanceId: string): void {
    text(instanceId, 'Instance id', 128);
    this.transaction(() => {
      const instance = this.instance(instanceId);
      if (!instance) return;
      if (instance.pid !== process.pid) throw new Error('Only the owning process can unregister a Council instance.');
      const active = this.statement("SELECT 1 FROM runs INDEXED BY runs_live_metadata WHERE owner_instance = ? AND status IN ('queued', 'running') LIMIT 1").get(instanceId);
      if (active) {
        throw new Error('Cannot unregister a Council instance while it owns unfinished runs.');
      }
      this.statement('DELETE FROM instances WHERE id = ?').run(instanceId);
    });
  }

  claimRun(runId: string, instanceId: string): void {
    text(runId, 'Run id', 256);
    text(instanceId, 'Instance id', 128);
    this.transaction(() => {
      const instance = this.assertInstance(instanceId);
      const row = this.ownedRun(runId);
      if (!row) throw new Error('Cannot claim a run that does not exist.');
      if (!['queued', 'running'].includes(row.status)) throw new Error('Cannot claim a run that already reached a terminal state.');
      if (row.owner_instance && row.owner_instance !== instanceId) {
        const owner = this.instance(row.owner_instance);
        if (owner && processAlive(owner.pid)) throw new Error('This run is already owned by a live Council instance.');
        throw new Error('This run has an orphaned owner; recover it before starting a replacement.');
      }
      this.statement('UPDATE runs SET owner_instance = ?, owner_pid = ? WHERE id = ?').run(instanceId, instance.pid, runId);
    });
  }

  requestCancellation(runId: string): boolean {
    text(runId, 'Run id', 256);
    return this.transaction(() => {
      const row = this.ownedRun(runId);
      if (!row || !['queued', 'running'].includes(row.status) || this.isCancellationRequested(runId)) return false;
      this.writeEvent(runId, 'cancel_requested', {});
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
      if (row.owner_instance) this.assertRunOwner(row);
      // Every terminal record is immutable; repeated completion is a read, not a rewrite.
      if (!['queued', 'running'].includes(row.status)) return this.getRun(run.id)!;
      const final = structuredClone(run);
      if (final.status === 'completed' && this.isCancellationRequested(run.id)) {
        final.status = 'cancelled';
        final.error = 'Cancellation was requested before workflow completion.';
      }
      final.updatedAt = new Date().toISOString();
      this.writeRun(final, row.revision + 1);
      return final;
    });
  }

  /** Explicit repair metadata only; the caller has already verified and exported the managed worktree. */
  updateArtifact(runId: string, artifact: WorkspaceArtifact): RunRecord {
    text(runId, 'Run id', 256);
    if (!artifact || typeof artifact !== 'object' || Array.isArray(artifact) || typeof artifact.changed !== 'boolean') throw new Error('A repaired artifact must include its identity and a boolean changed flag.');
    if (artifact.exportStatus !== undefined && !['ready', 'pending', 'failed'].includes(artifact.exportStatus)) throw new Error('Artifact export status must be ready, pending or failed.');
    if (artifact.exportError !== undefined && (typeof artifact.exportError !== 'string' || Buffer.byteLength(artifact.exportError) > 32768)) throw new Error('Artifact export error must contain at most 32768 UTF-8 bytes.');
    return this.transaction(() => {
      const row = this.ownedRun(runId);
      if (!row) throw new Error('Run not found.');
      if (['queued', 'running'].includes(row.status)) throw new Error('Cannot refresh artifacts while a run is active.');
      const run = this.getRun(runId)!;
      if (run.status !== row.status) throw new Error('Run metadata does not match its durable result.');
      const matches = (run.workspaces ?? []).map((existing, index) => ({ existing, index })).filter(({ existing }) =>
        existing.agent === artifact.agent && existing.path === artifact.path && existing.patchPath === artifact.patchPath && existing.baseCommit === artifact.baseCommit);
      if (matches.length !== 1) throw new Error('Artifact identity must match exactly one existing managed worktree; adding or redirecting artifacts is not allowed.');
      const { existing, index } = matches[0]!;
      const refreshed: WorkspaceArtifact = { ...existing, changed: artifact.changed };
      delete refreshed.exportStatus;
      delete refreshed.exportError;
      if (artifact.exportStatus !== undefined) refreshed.exportStatus = artifact.exportStatus;
      if (artifact.exportError !== undefined) refreshed.exportError = artifact.exportError;
      run.workspaces![index] = refreshed;
      run.updatedAt = new Date().toISOString();
      this.writeRun(run, row.revision + 1);
      this.writeEvent(runId, 'workspace.refreshed', refreshed);
      return run;
    });
  }

  /** Caller holds the write transaction and has checked its narrow mutation authority. */
  private writeRun(run: RunRecord, revision: number): void {
    this.statement('UPDATE runs SET document = ?, updated_at = ?, status = ?, revision = ?, summary = ?, has_artifacts = ? WHERE id = ?')
      .run(encode(run, MAX_RUN_JSON_BYTES), run.updatedAt, run.status, revision, encode(summarizeRun(run, revision)), (run.workspaces?.length ?? 0) > 0 ? 1 : 0, run.id);
    if (!['queued', 'running'].includes(run.status)) this.statement('DELETE FROM slot_waiters WHERE run_id = ?').run(run.id);
  }

  /** Stable per-call IDs reserve free capacity for older durable waiters across processes. */
  acquireSlot(instanceId: string, runId: string, limit: number, waiterId?: string): string | undefined {
    text(instanceId, 'Instance id', 128);
    text(runId, 'Run id', 256);
    if (!Number.isInteger(limit) || limit < 1 || limit > 32) throw new Error('The shared concurrency limit must be from 1 to 32.');
    if (waiterId !== undefined) text(waiterId, 'Slot waiter id', 128);
    return this.transaction(() => {
      this.assertInstance(instanceId);
      const now = Date.now();
      if (now < this.lastRecoveryAt || now - this.lastRecoveryAt >= 500) { this.recoverDeadOwners(); this.lastRecoveryAt = now; }
      const row = this.ownedRun(runId);
      if (!row || row.owner_instance !== instanceId) throw new Error('The Council instance does not own this run.');
      if (!['queued', 'running'].includes(row.status)) throw new Error('This run is no longer active.');
      if (this.isCancellationRequested(runId)) throw new Error('Cancellation has been requested for this run.');
      this.statement('UPDATE instances SET slot_limit = ? WHERE id = ? AND (slot_limit IS NULL OR slot_limit <> ?)').run(limit, instanceId, limit);
      // Metadata-only cleanup also handles interrupted callers and old manually edited databases.
      this.statement("DELETE FROM slot_waiters WHERE NOT EXISTS (SELECT 1 FROM runs INDEXED BY runs_metadata_lookup WHERE runs.id = slot_waiters.run_id AND runs.owner_instance = slot_waiters.instance_id AND runs.status IN ('queued', 'running') AND runs.cancel_requested = 0) OR NOT EXISTS (SELECT 1 FROM instances WHERE instances.id = slot_waiters.instance_id)").run();
      if (waiterId !== undefined) {
        const existing = this.statement('SELECT run_id FROM slot_waiters WHERE instance_id = ? AND waiter_id = ?').get(instanceId, waiterId) as { run_id: string } | undefined;
        if (existing && existing.run_id !== runId) throw new Error('A slot waiter ID cannot be reused for a different run.');
        if (!existing) {
          const count = this.statement('SELECT COUNT(*) AS total, SUM(run_id = ?) AS own FROM slot_waiters').get(runId) as { total: number; own: number | null };
          if (count.total >= 800 || (count.own ?? 0) >= 32) throw Object.assign(new Error('The Council slot waiter queue is full; cancel abandoned waits before retrying.'), { code: 'COUNCIL_SLOT_QUEUE_FULL' });
          this.statement('INSERT INTO slot_waiters(instance_id, run_id, waiter_id) VALUES (?, ?, ?)').run(instanceId, runId, waiterId);
        }
      }
      const minimum = this.statement('SELECT MIN(slot_limit) AS value FROM instances WHERE slot_limit IS NOT NULL').get() as { value: number };
      const active = this.statement('SELECT COUNT(*) AS value FROM slots').get() as { value: number };
      const available = minimum.value - active.value;
      if (available <= 0) return undefined;
      if (waiterId === undefined) {
        if (this.statement('SELECT 1 FROM slot_waiters LIMIT 1').get()) return undefined;
      } else {
        const older = this.statement('SELECT COUNT(*) AS value FROM slot_waiters WHERE sequence < (SELECT sequence FROM slot_waiters WHERE instance_id = ? AND waiter_id = ?)').get(instanceId, waiterId) as { value: number };
        // Each older waiter keeps one slot reserved even while it is between polling attempts.
        if (older.value >= available) return undefined;
      }
      const token = randomUUID();
      this.statement('INSERT INTO slots(token, instance_id, run_id, acquired_at) VALUES (?, ?, ?, ?)').run(token, instanceId, runId, Date.now());
      if (waiterId !== undefined) this.statement('DELETE FROM slot_waiters WHERE instance_id = ? AND waiter_id = ?').run(instanceId, waiterId);
      return token;
    });
  }

  releaseSlot(token: string, instanceId: string): void {
    text(token, 'Slot token', 256);
    text(instanceId, 'Instance id', 128);
    this.statement('DELETE FROM slots WHERE token = ? AND instance_id = ?').run(token, instanceId);
  }

  /** Idempotent cleanup for a per-call wait that was aborted before admission. */
  cancelSlotWaiter(instanceId: string, waiterId: string): void {
    text(instanceId, 'Instance id', 128);
    text(waiterId, 'Slot waiter id', 128);
    this.assertInstance(instanceId);
    this.statement('DELETE FROM slot_waiters WHERE instance_id = ? AND waiter_id = ?').run(instanceId, waiterId);
  }

  /** Heartbeat age alone is never proof that a process is dead. PID reuse is intentionally conservative. */
  recoverOrphans(): RunRecord[] {
    return this.transaction(() => { const recovered = this.recoverDeadOwners(); this.lastRecoveryAt = Date.now(); return recovered; });
  }

  private instance(instanceId: string): InstanceRow | undefined {
    return this.statement('SELECT id, pid, heartbeat_at, slot_limit FROM instances WHERE id = ?').get(instanceId) as InstanceRow | undefined;
  }

  private assertInstance(instanceId: string): InstanceRow {
    const instance = this.instance(instanceId);
    if (!instance || instance.pid !== process.pid || !processAlive(instance.pid)) throw new Error('The Council owner instance is not registered to this live process.');
    return instance;
  }

  private ownedRun(runId: string): OwnedRunRow | undefined {
    return this.statement('SELECT status, revision, owner_instance, owner_pid, cancel_requested FROM runs INDEXED BY runs_metadata_lookup WHERE id = ?').get(runId) as OwnedRunRow | undefined;
  }

  private assertRunOwner(row: OwnedRunRow): void {
    if (!row.owner_instance) return;
    const owner = this.instance(row.owner_instance);
    if (!owner || owner.pid !== process.pid || owner.pid !== row.owner_pid || this.boundInstance !== row.owner_instance) throw new Error('This store connection is not bound to the Council run owner.');
  }

  private legacyCancellation(runId: string): boolean {
    return this.statement("SELECT 1 FROM events WHERE run_id = ? AND type = 'cancel_requested' LIMIT 1").get(runId) !== undefined;
  }

  private recoverDeadOwners(): RunRecord[] {
    const instances = this.statement('SELECT id, pid, heartbeat_at, slot_limit FROM instances').all() as InstanceRow[];
    const alive = new Map(instances.map(instance => [instance.id, processAlive(instance.pid)]));
    const dead = new Set(instances.filter(instance => !alive.get(instance.id)).map(instance => instance.id));
    const rows = this.statement("SELECT id, status, revision, owner_instance, owner_pid, cancel_requested FROM runs INDEXED BY runs_live_metadata WHERE owner_instance IS NOT NULL AND status IN ('queued', 'running')").all() as (OwnedRunRow & { id: string })[];
    const recovered: RunRecord[] = [];
    for (const row of rows) {
      if (!row.owner_instance || row.owner_pid === null) continue;
      const ownerAlive = alive.get(row.owner_instance) ?? processAlive(row.owner_pid);
      if (ownerAlive) continue;
      dead.add(row.owner_instance);
      const run = this.getRun(row.id)!;
      run.status = 'failed';
      run.error = 'The Council owner process exited before this workflow finished. Inspect retained worktrees before starting a replacement; no task was retried.';
      run.updatedAt = new Date().toISOString();
      this.writeRun(run, row.revision + 1);
      this.appendEvent(run.id, 'run.failed', { reason: 'owner_exited', status: 'failed' });
      recovered.push(run);
    }
    for (const instanceId of dead) {
      this.statement('DELETE FROM slots WHERE instance_id = ?').run(instanceId);
      this.statement('DELETE FROM instances WHERE id = ?').run(instanceId);
    }
    return recovered;
  }

  health(): {
    schemaVersion: number; sqliteVersion: string; journalMode: string; databaseBytes: number; walBytes: number;
    preparedStatements: number; workspaceActiveRunLimit: number;
    counts: { runs: number; activeRuns: number; events: number; messages: number; unreadMessages: number; leases: number; instances: number; slots: number; slotWaiters: number; context: number };
    runStatuses: Record<string, number>;
  } {
    const inspect = () => {
    const size = (path: string): number => { try { return statSync(path).size; } catch (error) { if ((error as NodeJS.ErrnoException).code === 'ENOENT') return 0; throw error; } };
    const counts = this.statement(`SELECT
      (SELECT COUNT(*) FROM runs) AS runs,
      (SELECT COUNT(*) FROM runs WHERE status IN ('queued', 'running')) AS activeRuns,
      (SELECT COUNT(*) FROM events) AS events,
      (SELECT COUNT(*) FROM messages) AS messages,
      (SELECT COUNT(*) FROM messages WHERE acknowledged_at IS NULL) AS unreadMessages,
      (SELECT COUNT(*) FROM leases WHERE expires_at > ?) AS leases,
      (SELECT COUNT(*) FROM instances) AS instances,
      (SELECT COUNT(*) FROM slots) AS slots,
      (SELECT COUNT(*) FROM slot_waiters) AS slotWaiters,
      (SELECT COUNT(*) FROM context) AS context`).get(Date.now()) as { runs: number; activeRuns: number; events: number; messages: number; unreadMessages: number; leases: number; instances: number; slots: number; slotWaiters: number; context: number };
    const statuses = this.statement('SELECT status, COUNT(*) AS count FROM runs GROUP BY status').all() as { status: string; count: number }[];
    const mode = this.statement('PRAGMA journal_mode').get() as { journal_mode: string };
    return {
      schemaVersion: SCHEMA_VERSION, sqliteVersion: this.sqliteVersion, journalMode: mode.journal_mode,
      databaseBytes: size(this.databasePath), walBytes: size(`${this.databasePath}-wal`),
      preparedStatements: this.statements.size, workspaceActiveRunLimit: 100, counts,
      runStatuses: Object.fromEntries(statuses.map(row => [row.status, row.count])),
    };
    };
    return this.db.isTransaction ? inspect() : this.transaction(inspect, false);
  }

  checkIntegrity(): string[] {
    return (this.statement('PRAGMA quick_check').all() as { quick_check: string }[]).map(row => row.quick_check);
  }

  /** Online SQLite backup, published atomically without ever opening the requested destination for writing. */
  async backupTo(path: string, signal?: AbortSignal): Promise<string> {
    signal?.throwIfAborted();
    text(path, 'Backup destination', 4096);
    const requested = resolve(path);
    const parent = realpathSync(dirname(requested));
    const destination = join(parent, basename(requested));
    const source = realpathSync(this.databasePath);
    const key = (value: string) => process.platform === 'win32' ? value.toLowerCase() : value;
    if ([source, `${source}-wal`, `${source}-shm`, `${source}-journal`].map(key).includes(key(destination))) throw new Error('A backup cannot replace the active Council database or its journal files.');
    try { lstatSync(destination); throw new Error('Backup destination already exists; choose a new filename.'); }
    catch (error) { if ((error as NodeJS.ErrnoException).code !== 'ENOENT') throw error; }
    const temporary = mkdtempSync(join(parent, '.council-backup-'));
    const snapshot = join(temporary, 'snapshot.db');
    this.activeBackups++;
    let reader: DatabaseSync | undefined;
    let failed = false;
    try {
      const deadline = performance.now() + 60_000;
      // Pin one WAL snapshot on an isolated connection: foreign commits otherwise restart each batch.
      // The application connection remains free to write while this read transaction is active.
      reader = new DatabaseSync(source, { readOnly: true });
      reader.exec('PRAGMA busy_timeout = 250; BEGIN;');
      reader.prepare('SELECT name FROM sqlite_schema LIMIT 1').get();
      await backup(reader, snapshot, { rate: 128, progress: () => {
        signal?.throwIfAborted();
        if (performance.now() > deadline) throw new Error('Council backup exceeded its 60-second deadline.');
      } });
      signal?.throwIfAborted();
      if (performance.now() > deadline) throw new Error('Council backup exceeded its 60-second deadline.');
      if (process.platform !== 'win32') chmodSync(snapshot, 0o600);
      const descriptor = openSync(snapshot, 'r+');
      try { fsyncSync(descriptor); } finally { closeSync(descriptor); }
      // A same-directory hard link is exclusive: an intervening creator yields EEXIST, never overwrite.
      linkSync(snapshot, destination);
      return destination;
    } catch (error) {
      failed = true;
      throw error;
    } finally {
      let cleanupError: unknown;
      try { if (reader?.isTransaction) reader.exec('ROLLBACK'); }
      catch (error) { cleanupError = error; }
      try { reader?.close(); }
      catch (error) { cleanupError ??= error; }
      this.activeBackups--;
      // This directory was created exclusively by this invocation under the resolved parent.
      try {
        if (dirname(temporary) !== parent || !basename(temporary).startsWith('.council-backup-')) throw new Error('Refusing to remove an unexpected backup staging directory.');
        rmSync(temporary, { recursive: true, force: true });
      } catch (error) { cleanupError ??= error; }
      if (!failed && cleanupError) throw cleanupError;
    }
  }

  prune(options: { before: string; keepLatest?: number; dryRun?: boolean }): PruneResult {
    const before = cutoffTimestamp(options.before);
    const keep = options.keepLatest ?? 100;
    if (!Number.isInteger(keep) || keep < 0 || keep > 100000) throw new Error('keepLatest must be an integer from 0 to 100000.');
    if (options.dryRun !== undefined && typeof options.dryRun !== 'boolean') throw new Error('dryRun must be a boolean.');
    return this.transaction(() => {
      const skipped = this.statement("SELECT COUNT(*) AS count FROM runs WHERE status IN ('completed', 'failed', 'cancelled') AND updated_at < ? AND has_artifacts = 1 AND id NOT IN (SELECT id FROM runs ORDER BY updated_at DESC, id DESC LIMIT ?)").get(before, keep) as { count: number };
      const candidates = this.statement("SELECT id FROM runs WHERE status IN ('completed', 'failed', 'cancelled') AND updated_at < ? AND has_artifacts = 0 AND id NOT IN (SELECT id FROM runs ORDER BY updated_at DESC, id DESC LIMIT ?) ORDER BY updated_at, id LIMIT 1000").all(before, keep) as { id: string }[];
      const result: PruneResult = { candidateIds: candidates.map(row => row.id), deletedRuns: 0, deletedEvents: 0, skippedArtifacts: skipped.count };
      if (options.dryRun !== false) return result;
      for (const { id } of candidates) {
        result.deletedEvents += Number(this.statement('DELETE FROM events WHERE run_id = ?').run(id).changes);
        this.statement('DELETE FROM slots WHERE run_id = ?').run(id);
        result.deletedRuns += Number(this.statement('DELETE FROM runs WHERE id = ?').run(id).changes);
      }
      return result;
    }, options.dryRun === false);
  }

  close(): void {
    if (this.activeBackups > 0) throw new Error('Wait for active Council backups before closing the store.');
    this.db.close();
    this.statements.clear();
  }
}
