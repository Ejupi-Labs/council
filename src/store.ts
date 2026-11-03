import { randomUUID } from 'node:crypto';
import { chmodSync, mkdirSync } from 'node:fs';
import { join } from 'node:path';
import { DatabaseSync } from 'node:sqlite';
import type { ContextEntry, EventRecord, Lease, Message, RunRecord } from './types.js';

const SCHEMA_VERSION = 1;
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

/** Durable collaboration state shared by all Council processes in one workspace. */
export class CouncilStore {
  private readonly db: DatabaseSync;

  constructor(stateDir: string) {
    mkdirSync(stateDir, { recursive: true, mode: 0o700 });
    const databasePath = join(stateDir, 'state.db');
    this.db = new DatabaseSync(databasePath);
    try {
      this.db.exec('PRAGMA busy_timeout = 5000; PRAGMA journal_mode = WAL; PRAGMA foreign_keys = ON;');
      const version = this.db.prepare('PRAGMA user_version').get() as { user_version: number };
      if (version.user_version > SCHEMA_VERSION) throw new Error(`Council state schema ${version.user_version} is newer than supported schema ${SCHEMA_VERSION}`);
      this.transaction(() => {
        this.db.exec(`
          CREATE TABLE IF NOT EXISTS runs (id TEXT PRIMARY KEY, updated_at TEXT NOT NULL, document TEXT NOT NULL);
          CREATE TABLE IF NOT EXISTS events (sequence INTEGER PRIMARY KEY AUTOINCREMENT, run_id TEXT NOT NULL, type TEXT NOT NULL, data TEXT NOT NULL, created_at TEXT NOT NULL);
          CREATE INDEX IF NOT EXISTS events_run_sequence ON events(run_id, sequence);
          CREATE TABLE IF NOT EXISTS messages (id TEXT PRIMARY KEY, sender TEXT NOT NULL, recipient TEXT NOT NULL, created_at TEXT NOT NULL, acknowledged_at TEXT, document TEXT NOT NULL);
          CREATE INDEX IF NOT EXISTS messages_inbox ON messages(recipient, created_at);
          CREATE TABLE IF NOT EXISTS leases (id TEXT PRIMARY KEY, owner TEXT NOT NULL, path TEXT NOT NULL, path_key TEXT NOT NULL UNIQUE, run_id TEXT, expires_at INTEGER NOT NULL);
          CREATE TABLE IF NOT EXISTS context (key TEXT PRIMARY KEY, value TEXT NOT NULL, author TEXT NOT NULL, updated_at TEXT NOT NULL);
          PRAGMA user_version = 1;
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
    this.db.prepare('INSERT INTO runs(id, updated_at, document) VALUES (?, ?, ?) ON CONFLICT(id) DO UPDATE SET updated_at = excluded.updated_at, document = excluded.document')
      .run(run.id, run.updatedAt, encode(run, MAX_RUN_JSON_BYTES));
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
      const active = this.readLeases();
      for (const requested of normalized) {
        const conflict = active.find(lease => lease.owner !== owner && lease.paths.some(path => overlaps(path, requested)));
        if (conflict) throw new Error(`Reservation conflict: ${requested} overlaps ${conflict.paths[0]} owned by ${conflict.owner} until ${conflict.expiresAt}`);
      }
      const expiresAt = new Date(now + ttlMs).toISOString();
      return normalized.map(path => {
        const existing = active.find(lease => lease.owner === owner && lease.paths[0]?.toLowerCase() === path.toLowerCase());
        const lease: Lease = { id: existing?.id ?? randomUUID(), owner, paths: [path], expiresAt, ...(runId === undefined ? {} : { runId }) };
        this.db.prepare('INSERT INTO leases(id, owner, path, path_key, run_id, expires_at) VALUES (?, ?, ?, ?, ?, ?) ON CONFLICT(path_key) DO UPDATE SET path = excluded.path, run_id = excluded.run_id, expires_at = excluded.expires_at')
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

  private readLeases(): Lease[] {
    const rows = this.db.prepare('SELECT id, owner, path, run_id, expires_at FROM leases WHERE expires_at > ? ORDER BY path_key').all(Date.now()) as {
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

  close(): void { this.db.close(); }
}
