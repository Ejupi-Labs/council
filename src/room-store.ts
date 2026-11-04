import { chmodSync, mkdirSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { DatabaseSync } from 'node:sqlite';
import { randomUUID } from 'node:crypto';
import { requireSafeSqliteRuntime } from './sqlite-runtime.js';
import type { RoomMessage, RoomObservation, RoomRecord, RoomSummary } from './rooms-types.js';

const ACTIVE = new Set(['discussing', 'running']);
const HISTORY_MARKER = 'history-preview';
const MESSAGE_PAGE_BYTES = 1024 * 1024;
function json(value: unknown): string {
  const text = JSON.stringify(value);
  if (text === undefined) throw new Error('Room data must be JSON serializable.');
  return text;
}
function alive(pid: number): boolean {
  if (!Number.isSafeInteger(pid) || pid < 1) return false;
  try { process.kill(pid, 0); return true; }
  catch (error) { return (error as NodeJS.ErrnoException).code !== 'ESRCH'; }
}
interface Row { document: string; owner: string | null; owner_pid: number | null; cancel_requested: number; revision: number }

/** Room state has its own database so optional UI features never migrate workflow storage. */
export class RoomStore {
  private readonly db: DatabaseSync;
  constructor(stateDir: string) {
    requireSafeSqliteRuntime();
    mkdirSync(stateDir, { recursive: true, mode: 0o700 });
    const filename = join(resolve(stateDir), 'rooms.sqlite');
    this.db = new DatabaseSync(filename);
    try {
      this.db.exec('PRAGMA busy_timeout=250; PRAGMA foreign_keys=ON;');
      const version = Number(this.db.prepare('PRAGMA user_version').get()!.user_version);
      if (version > 1) throw new Error('Room state was created by a newer Council version.');
      if (process.platform !== 'win32') chmodSync(filename, 0o600);
      if (version === 0) {
        const deadline = performance.now() + 5000;
        const wait = new Int32Array(new SharedArrayBuffer(4));
        for (;;) {
          try {
            this.db.exec(`PRAGMA busy_timeout=${Math.max(1, Math.ceil(deadline - performance.now()))}; PRAGMA journal_mode=WAL;`);
            this.transaction(() => {
              const current = Number(this.db.prepare('PRAGMA user_version').get()!.user_version);
              if (current > 1) throw new Error('Room state was created by a newer Council version.');
              if (current === 1) return;
              this.db.exec(`CREATE TABLE rooms(id TEXT PRIMARY KEY, document TEXT NOT NULL, revision INTEGER NOT NULL,
                updated_at TEXT NOT NULL, status TEXT NOT NULL, owner TEXT, owner_pid INTEGER, cancel_requested INTEGER NOT NULL DEFAULT 0);
                CREATE INDEX rooms_recent ON rooms(updated_at DESC,id DESC);
                CREATE TABLE room_messages(sequence INTEGER PRIMARY KEY AUTOINCREMENT, room_id TEXT NOT NULL REFERENCES rooms(id),
                  id TEXT NOT NULL, document TEXT NOT NULL, UNIQUE(room_id,id));
                CREATE INDEX room_message_page ON room_messages(room_id,sequence);
                PRAGMA user_version=1;`);
            });
            break;
          } catch (error) {
            const code = (error as { errcode?: number }).errcode;
            const remaining = deadline - performance.now();
            if (typeof code !== 'number' || (code & 0xff) !== 5 || remaining <= 0) throw error;
            Atomics.wait(wait, 0, 0, Math.min(25, remaining));
          }
        }
        this.db.exec('PRAGMA busy_timeout=250;');
      }
      if (process.platform !== 'win32') {
        for (const path of [`${filename}-wal`, `${filename}-shm`]) {
          try { chmodSync(path, 0o600); } catch (error) { if ((error as NodeJS.ErrnoException).code !== 'ENOENT') throw error; }
        }
      }
      this.recover();
    } catch (error) { this.db.close(); throw error; }
  }

  private transaction<T>(action: () => T, write = true): T {
    this.db.exec(write ? 'BEGIN IMMEDIATE' : 'BEGIN');
    try { const value = action(); this.db.exec('COMMIT'); return value; }
    catch (error) { try { if (this.db.isTransaction) this.db.exec('ROLLBACK'); } catch {} throw error; }
  }

  private row(id: string): Row | undefined {
    return this.db.prepare('SELECT document,owner,owner_pid,cancel_requested,revision FROM rooms WHERE id=?').get(id) as Row | undefined;
  }

  private append(id: string, messages: RoomMessage[]): void {
    const insert = this.db.prepare('INSERT OR IGNORE INTO room_messages(room_id,id,document) VALUES(?,?,?)');
    for (const message of messages) {
      if (message.id === HISTORY_MARKER) continue;
      if (typeof message.id !== 'string' || !message.id || message.id.length > 256) throw new Error('Invalid room message ID.');
      insert.run(id, message.id, json(message));
    }
  }

  insert(room: RoomRecord): void {
    this.transaction(() => {
      const document = { ...room, messages: [] };
      this.db.prepare('INSERT INTO rooms(id,document,revision,updated_at,status) VALUES(?,?,?,?,?)')
        .run(room.id, json(document), room.revision, room.updatedAt, room.status);
      this.append(room.id, room.messages);
    });
  }

  get(id: string): RoomRecord | undefined {
    if (!this.db.isTransaction) return this.transaction(() => this.get(id), false);
    const row = this.row(id);
    if (!row) return undefined;
    const room = JSON.parse(row.document) as RoomRecord;
    room.messages = [];
    let bytes = 2, omitted = false;
    const rows = this.db.prepare('SELECT document FROM room_messages WHERE room_id=? ORDER BY sequence DESC LIMIT 101').iterate(id);
    for (const row of rows) {
      const document = row.document as string;
      const size = Buffer.byteLength(document) + (room.messages.length ? 1 : 0);
      if (room.messages.length >= 100 || room.messages.length > 0 && bytes + size > MESSAGE_PAGE_BYTES) { omitted = true; break; }
      // Preserve at least one complete message, even when it exceeds the page
      // budget. Never clip evidence or materialize every large row eagerly.
      room.messages.push(JSON.parse(document) as RoomMessage); bytes += size;
    }
    room.messages.reverse();
    if (omitted) room.messages.unshift({ id: HISTORY_MARKER, author: 'system', kind: 'system',
      body: `Showing the latest ${room.messages.length} complete messages. Earlier complete messages remain available through messagePage.`, createdAt: room.createdAt });
    return room;
  }

  /** Read coordination state without fetching or decoding any chat history. */
  metadata(id: string): Omit<RoomRecord, 'messages'> | undefined {
    const row = this.row(id);
    if (!row) return undefined;
    const { messages: _messages, ...metadata } = JSON.parse(row.document) as RoomRecord;
    return metadata;
  }

  has(id: string): boolean { return Boolean(this.db.prepare('SELECT 1 FROM rooms WHERE id=?').get(id)); }

  /** A cache invalidator for this connection, not a durable cursor or shared revision. */
  changeToken(): string {
    const row = this.db.prepare('SELECT CAST(total_changes() AS TEXT) AS writes, CAST(data_version AS TEXT) AS version FROM pragma_data_version').get() as { writes: string; version: string };
    return `${row.writes}:${row.version}`;
  }

  observation(id: string): RoomObservation | undefined {
    const row = this.db.prepare(`SELECT id, status, revision,
      json_extract(document, '$.title') AS title, COALESCE(json_extract(document, '$.instructionRevision'), 0) AS instruction_revision,
      json_array_length(document, '$.tasks') AS task_count,
      (SELECT COUNT(*) FROM json_each(rooms.document, '$.tasks') task WHERE json_extract(task.value, '$.status')='completed') AS completed_tasks,
      (SELECT COUNT(*) FROM json_each(rooms.document, '$.tasks') task WHERE json_extract(task.value, '$.status') IN ('failed','cancelled')) AS failed_tasks,
      json_extract(document, '$.callsUsed') AS calls_used, json_extract(document, '$.maxCalls') AS max_calls,
      NULLIF(json_extract(document, '$.error'), '') AS error, json_extract(document, '$.pauseRequested') AS pause_requested
      FROM rooms WHERE id=?`).get(id) as {
        id: string; title: string; status: RoomObservation['status']; revision: number; instruction_revision: number;
        task_count: number; completed_tasks: number; failed_tasks: number; calls_used: number; max_calls: number;
        error: string | null; pause_requested: number | null;
      } | undefined;
    return row ? { id: row.id, title: row.title, status: row.status, revision: row.revision, instructionRevision: row.instruction_revision,
      taskCount: row.task_count, completedTasks: row.completed_tasks, failedTasks: row.failed_tasks, callsUsed: row.calls_used, maxCalls: row.max_calls,
      ...(row.error ? { error: row.error } : {}), ...(row.pause_requested === 1 ? { pauseRequested: true } : {}) } : undefined;
  }

  list(limit = 1000): RoomSummary[] {
    if (!Number.isInteger(limit) || limit < 1 || limit > 1000) throw new Error('Room summary limit must be from 1 to 1000.');
    // Project only list metadata inside SQLite. Large briefs, task bodies and
    // retained artifact records stay outside JavaScript during routine updates.
    const rows = this.db.prepare(`SELECT rooms.id, rooms.status, rooms.revision, rooms.updated_at,
      json_extract(document, '$.backend') AS backend, json_extract(document, '$.title') AS title,
      json_array_length(document, '$.tasks') AS task_count,
      (SELECT COUNT(*) FROM json_each(rooms.document, '$.tasks') task
        WHERE json_extract(task.value, '$.status')='completed') AS completed_tasks,
      json_extract(document, '$.callsUsed') AS calls_used, json_extract(document, '$.maxCalls') AS max_calls
      FROM rooms ORDER BY updated_at DESC,id DESC LIMIT ?`).all(limit) as {
        id: string; backend: RoomSummary['backend'] | null; title: string; status: RoomSummary['status']; revision: number;
        updated_at: string; task_count: number; completed_tasks: number; calls_used: number; max_calls: number;
      }[];
    return rows.map(row => ({ id: row.id, backend: row.backend ?? undefined, title: row.title, status: row.status,
      taskCount: row.task_count, completedTasks: row.completed_tasks, callsUsed: row.calls_used, maxCalls: row.max_calls,
      revision: row.revision, updatedAt: row.updated_at }));
  }

  mutate(id: string, change: (room: RoomRecord) => void, options: { owner?: string; claim?: { id: string; pid: number }; release?: boolean; requireIdle?: boolean; preserveFailure?: boolean } = {}): RoomRecord {
    return this.transaction(() => {
      const row = this.row(id);
      const room = this.get(id);
      if (!row || !room) throw new Error('Room not found.');
      if (options.owner && row.owner !== options.owner) throw new Error('Room ownership was lost.');
      if (options.claim && (row.owner || ACTIVE.has(room.status))) throw new Error('Room is already active in another operation.');
      if (options.requireIdle && (row.owner || ACTIVE.has(room.status))) throw new Error('Room cannot be changed while active.');
      change(room);
      if (room.id !== id) throw new Error('Room ID cannot be changed.');
      if (options.release && row.cancel_requested && !(options.preserveFailure && room.status === 'failed')) {
        room.status = 'cancelled'; room.error = 'Cancellation was accepted before room completion.';
        for (const task of room.tasks) if (['pending', 'assigned', 'running'].includes(task.status)) task.status = 'cancelled';
      }
      if (options.release) {
        if (room.pauseRequested && ['discussing', 'running', 'ready'].includes(room.status)) {
          if (!room.checkpoint) throw new Error('Room pause has no durable continuation checkpoint.');
          room.status = 'paused';
        }
        delete room.pauseRequested;
        if (room.status !== 'paused') delete room.checkpoint;
      }
      room.revision = row.revision + 1;
      room.updatedAt = new Date().toISOString();
      this.append(id, room.messages);
      const owner = options.release ? null : options.claim?.id ?? row.owner;
      const pid = options.release ? null : options.claim?.pid ?? row.owner_pid;
      const cancelled = options.release || options.claim ? 0 : row.cancel_requested;
      this.db.prepare('UPDATE rooms SET document=?,revision=?,updated_at=?,status=?,owner=?,owner_pid=?,cancel_requested=? WHERE id=?')
        .run(json({ ...room, messages: [] }), room.revision, room.updatedAt, room.status, owner, pid, cancelled, id);
      return structuredClone(room);
    });
  }

  requestCancel(id: string, options: { activeOnly?: boolean } = {}): RoomRecord {
    return this.transaction(() => {
      const row = this.row(id);
      const room = this.get(id);
      if (!row || !room) throw new Error('Room not found.');
      if (['completed', 'failed', 'cancelled'].includes(room.status)) return room;
      if (!row.owner) {
        if (options.activeOnly) return room;
        if (ACTIVE.has(room.status)) throw new Error('Active room ownership is missing; cancellation could not be verified.');
        room.status = 'cancelled'; room.error = 'Room cancelled by the project manager.';
        delete room.pauseRequested; delete room.checkpoint;
        for (const task of room.tasks) if (['pending', 'assigned'].includes(task.status)) task.status = 'cancelled';
        room.messages.push({ id: randomUUID(), author: 'system', kind: 'system', body: room.error, createdAt: new Date().toISOString() });
        room.revision = row.revision + 1; room.updatedAt = new Date().toISOString();
        this.append(id, room.messages);
        this.db.prepare('UPDATE rooms SET status=?,revision=?,updated_at=?,document=? WHERE id=?')
          .run(room.status, room.revision, room.updatedAt, json({ ...room, messages: [] }), id);
        return room;
      }
      if (row.cancel_requested) return room;
      room.revision = row.revision + 1;
      room.updatedAt = new Date().toISOString();
      this.db.prepare('UPDATE rooms SET cancel_requested=1,revision=?,updated_at=?,document=? WHERE id=?')
        .run(room.revision, room.updatedAt, json({ ...room, messages: [] }), id);
      return room;
    });
  }

  requestPause(id: string): RoomRecord {
    return this.transaction(() => {
      const row = this.row(id), room = this.get(id);
      if (!row || !room) throw new Error('Room not found.');
      if (room.status === 'paused' || ['completed', 'failed', 'cancelled'].includes(room.status) || row.cancel_requested) return room;
      if (!['discussing', 'running', 'ready'].includes(room.status)) throw new Error('Only an active or approved ready room can pause.');
      if (room.pauseRequested) return room;
      if (room.status === 'ready' && !row.owner) {
        room.status = 'paused'; room.checkpoint = { phase: 'ready' };
      } else {
        if (!row.owner) throw new Error('Active room ownership is missing; pause could not be verified.');
        if (!room.checkpoint) throw new Error('The active room owner does not support durable pause checkpoints. Let it finish or stop it; do not resume its unfinished work.');
        room.pauseRequested = true;
      }
      room.messages.push({ id: randomUUID(), author: 'system', kind: 'system', body: room.status === 'paused'
        ? 'Room paused. Resuming restores the approved ready plan without executing it.'
        : 'Pause requested. Already admitted workflows will drain; further workflow admission is blocked.', createdAt: new Date().toISOString() });
      room.revision = row.revision + 1; room.updatedAt = new Date().toISOString();
      this.append(id, room.messages);
      this.db.prepare('UPDATE rooms SET status=?,revision=?,updated_at=?,document=? WHERE id=?')
        .run(room.status, room.revision, room.updatedAt, json({ ...room, messages: [] }), id);
      return structuredClone(room);
    });
  }

  cancelled(id: string, owner: string): boolean {
    const row = this.row(id);
    if (!row || row.owner !== owner) throw new Error('Room ownership was lost.');
    return row.cancel_requested === 1;
  }

  messagePage(id: string, limit = 100, cursor?: number): { messages: RoomMessage[]; nextCursor: number | null; nextAfter: number } {
    if (!Number.isInteger(limit) || limit < 1 || limit > 100 || (cursor !== undefined && (!Number.isSafeInteger(cursor) || cursor < 0))) throw new Error('Invalid message page bounds.');
    if (!this.has(id)) throw new Error('Room not found.');
    const messages: RoomMessage[] = [];
    let bytes = 2, lastSequence: number | null = null, more = false;
    const rows = this.db.prepare('SELECT sequence,document FROM room_messages WHERE room_id=? AND sequence>? ORDER BY sequence LIMIT ?')
      .iterate(id, cursor ?? 0, limit + 1);
    for (const row of rows) {
      const document = row.document as string;
      const size = Buffer.byteLength(document) + (messages.length ? 1 : 0);
      if (messages.length >= limit || messages.length > 0 && bytes + size > MESSAGE_PAGE_BYTES) { more = true; break; }
      messages.push(JSON.parse(document) as RoomMessage); bytes += size; lastSequence = row.sequence as number;
    }
    return { messages, nextCursor: more ? lastSequence : null, nextAfter: lastSequence ?? cursor ?? 0 };
  }

  recover(id?: string): void {
    const query = this.db.prepare(`SELECT id,owner,owner_pid FROM rooms WHERE owner IS NOT NULL${id === undefined ? '' : ' AND id=?'}`);
    const rows = (id === undefined ? query.all() : query.all(id)) as { id: string; owner: string; owner_pid: number }[];
    for (const row of rows) {
      if (alive(row.owner_pid)) continue;
      this.transaction(() => {
        const current = this.row(row.id);
        if (!current || current.owner !== row.owner || alive(current.owner_pid!)) return;
        const room = JSON.parse(current.document) as RoomRecord;
        room.status = current.cancel_requested ? 'cancelled' : 'failed';
        delete room.pauseRequested; delete room.checkpoint;
        room.error = 'The owning process exited. Partial evidence was retained; unfinished work was not replayed.';
        for (const task of room.tasks) if (['running', 'assigned'].includes(task.status)) {
          task.status = current.cancel_requested ? 'cancelled' : 'failed'; task.error = room.error;
        }
        room.revision = current.revision + 1;
        room.updatedAt = new Date().toISOString();
        this.append(room.id, [{ id: randomUUID(), author: 'system', kind: 'system', body: room.error, createdAt: room.updatedAt }]);
        this.db.prepare('UPDATE rooms SET document=?,revision=?,updated_at=?,status=?,owner=NULL,owner_pid=NULL,cancel_requested=0 WHERE id=?')
          .run(json({ ...room, messages: [] }), room.revision, room.updatedAt, room.status, room.id);
      });
    }
  }

  close(): void { this.db.close(); }
}
