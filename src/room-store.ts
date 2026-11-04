import { chmodSync, mkdirSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { DatabaseSync } from 'node:sqlite';
import { randomUUID } from 'node:crypto';
import type { RoomMessage, RoomRecord, RoomSummary } from './rooms-types.js';

const ACTIVE = new Set(['discussing', 'running']);
const HISTORY_MARKER = 'history-preview';
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
    const sqlite = /^(\d+)\.(\d+)\.(\d+)/.exec(process.versions.sqlite ?? '');
    if (!sqlite || !(Number(sqlite[1]) > 3 || Number(sqlite[1]) === 3 && (Number(sqlite[2]) > 51 || Number(sqlite[2]) === 51 && Number(sqlite[3]) >= 3))) throw new Error('Room state requires SQLite 3.51.3 or newer.');
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
    const messages = this.db.prepare('SELECT document FROM room_messages WHERE room_id=? ORDER BY sequence DESC LIMIT 101').all(id) as { document: string }[];
    room.messages = messages.slice(0, 100).reverse().map(row => JSON.parse(row.document) as RoomMessage);
    if (messages.length > 100) room.messages.unshift({ id: HISTORY_MARKER, author: 'system', kind: 'system',
      body: 'Showing the latest 100 messages. Earlier complete messages remain available through messagePage.', createdAt: room.createdAt });
    return room;
  }

  list(): RoomSummary[] {
    const rows = this.db.prepare('SELECT document FROM rooms ORDER BY updated_at DESC,id DESC LIMIT 1000').all() as { document: string }[];
    return rows.map(row => {
      const room = JSON.parse(row.document) as RoomRecord;
      return { id: room.id, backend: room.backend, title: room.title, status: room.status, taskCount: room.tasks.length,
        completedTasks: room.tasks.filter(task => task.status === 'completed').length, callsUsed: room.callsUsed,
        maxCalls: room.maxCalls, revision: room.revision, updatedAt: room.updatedAt };
    });
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

  cancelled(id: string, owner: string): boolean {
    const row = this.row(id);
    if (!row || row.owner !== owner) throw new Error('Room ownership was lost.');
    return row.cancel_requested === 1;
  }

  messagePage(id: string, limit = 100, cursor?: number): { messages: RoomMessage[]; nextCursor: number | null } {
    if (!Number.isInteger(limit) || limit < 1 || limit > 100 || (cursor !== undefined && (!Number.isSafeInteger(cursor) || cursor < 0))) throw new Error('Invalid message page bounds.');
    if (!this.row(id)) throw new Error('Room not found.');
    const rows = this.db.prepare('SELECT sequence,document FROM room_messages WHERE room_id=? AND sequence>? ORDER BY sequence LIMIT ?')
      .all(id, cursor ?? 0, limit + 1) as { sequence: number; document: string }[];
    return { messages: rows.slice(0, limit).map(row => JSON.parse(row.document) as RoomMessage),
      nextCursor: rows.length > limit ? rows[limit - 1]!.sequence : null };
  }

  recover(): void {
    const rows = this.db.prepare('SELECT id,owner,owner_pid FROM rooms WHERE owner IS NOT NULL').all() as { id: string; owner: string; owner_pid: number }[];
    for (const row of rows) {
      if (alive(row.owner_pid)) continue;
      this.transaction(() => {
        const current = this.row(row.id);
        if (!current || current.owner !== row.owner || alive(current.owner_pid!)) return;
        const room = JSON.parse(current.document) as RoomRecord;
        room.status = current.cancel_requested ? 'cancelled' : 'failed';
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
