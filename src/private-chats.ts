import { randomUUID } from 'node:crypto';
import { chmodSync, mkdirSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { DatabaseSync } from 'node:sqlite';
import type { CouncilService } from './service.js';
import { parseSchema } from './schema.js';
import { requireSafeSqliteRuntime } from './sqlite-runtime.js';
import { privateChatActorsDefinition, privateChatCreateDefinition, privateChatGetDefinition, privateChatListDefinition, privateChatPageDefinition,
  privateChatSendDefinition, privateChatWatchDefinition } from './private-chats-schema.js';
import type { PrivateChat, PrivateChatInput, PrivateChatMessage, PrivateChatPage, PrivateChatPageInput,
  PrivateChatSendInput, PrivateChatWatchInput, PrivateChatWatchResult } from './private-chats-types.js';

interface ChatRow {
  id: string; participant_a: string; participant_b: string; room_id: string; created_at: string;
  last_message_at: string | null; message_count: number; last_sequence: number;
}
export class PrivateChatConflict extends Error {
  readonly code = 'COUNCIL_PRIVATE_CHAT_CONFLICT';
  readonly status = 409;
  constructor(message: string) { super(message); this.name = 'PrivateChatConflict'; }
}
function nonempty(value: string, label: string): void {
  if (!value.trim()) throw new Error(`${label} must contain non-whitespace text.`);
}
function actorLabel(value: string, label: string): void {
  nonempty(value, label);
  if (Buffer.from(value, 'utf8').toString('utf8') !== value) throw new Error(`${label} must contain valid Unicode.`);
}
function contention(error: unknown): boolean {
  const code = error && typeof error === 'object' && 'errcode' in error ? error.errcode : undefined;
  return typeof code === 'number' && ((code & 0xff) === 5 || (code & 0xff) === 6);
}
function metadata(row: ChatRow): PrivateChat {
  return { id: row.id, participants: [row.participant_a, row.participant_b], ...(row.room_id ? { roomId: row.room_id } : {}),
    createdAt: row.created_at, ...(row.last_message_at ? { lastMessageAt: row.last_message_at } : {}),
    messageCount: row.message_count, lastSequence: row.last_sequence };
}

/** Pair-scoped collaboration history. Actor labels are routing metadata, not an authentication boundary. */
export class PrivateChatManager {
  private readonly db: DatabaseSync;
  private readonly shutdown = new AbortController();
  private readonly listeners = new Map<string, Set<() => void>>();
  private closed = false;

  constructor(private readonly service: CouncilService) {
    requireSafeSqliteRuntime();
    mkdirSync(service.config.stateDir, { recursive: true, mode: 0o700 });
    const filename = join(resolve(service.config.stateDir), 'private-chats.sqlite');
    this.db = new DatabaseSync(filename);
    try {
      this.db.exec('PRAGMA busy_timeout=250; PRAGMA foreign_keys=ON;');
      const version = Number(this.db.prepare('PRAGMA user_version').get()!.user_version);
      if (version > 1) throw new Error('Private chat state was created by a newer Council version.');
      if (process.platform !== 'win32') chmodSync(filename, 0o600);
      const journal = this.db.prepare('PRAGMA journal_mode').get() as { journal_mode: string };
      if (version === 0 || journal.journal_mode !== 'wal') {
        const deadline = performance.now() + 5000, wait = new Int32Array(new SharedArrayBuffer(4));
        for (;;) {
          try {
            this.db.exec(`PRAGMA busy_timeout=${Math.max(1, Math.ceil(deadline - performance.now()))}; PRAGMA journal_mode=WAL;`);
            this.transaction(() => {
              const current = Number(this.db.prepare('PRAGMA user_version').get()!.user_version);
              if (current > 1) throw new Error('Private chat state was created by a newer Council version.');
              if (current === 1) return;
              this.db.exec(`CREATE TABLE private_chats(id TEXT PRIMARY KEY, participant_a TEXT NOT NULL, participant_b TEXT NOT NULL,
                room_id TEXT NOT NULL, created_at TEXT NOT NULL, last_message_at TEXT, message_count INTEGER NOT NULL DEFAULT 0,
                last_sequence INTEGER NOT NULL DEFAULT 0, UNIQUE(participant_a,participant_b,room_id));
                CREATE INDEX private_chat_actor_a ON private_chats(participant_a,created_at,id);
                CREATE INDEX private_chat_actor_b ON private_chats(participant_b,created_at,id);
                CREATE TABLE private_messages(sequence INTEGER PRIMARY KEY AUTOINCREMENT, id TEXT NOT NULL UNIQUE,
                  chat_id TEXT NOT NULL REFERENCES private_chats(id), sender TEXT NOT NULL, retry_key TEXT,
                  document TEXT NOT NULL, UNIQUE(chat_id,sender,retry_key));
                CREATE INDEX private_message_page ON private_messages(chat_id,sequence); PRAGMA user_version=1;`);
            }); break;
          } catch (error) {
            const remaining = deadline - performance.now();
            if (!contention(error) || remaining <= 0) throw error;
            Atomics.wait(wait, 0, 0, Math.min(25, remaining));
          }
        }
        this.db.exec('PRAGMA busy_timeout=250;');
      }
      if (process.platform !== 'win32') for (const path of [`${filename}-wal`, `${filename}-shm`]) {
        try { chmodSync(path, 0o600); } catch (error) { if ((error as NodeJS.ErrnoException).code !== 'ENOENT') throw error; }
      }
    } catch (error) { this.closed = true; this.db.close(); throw error; }
  }

  private open(): void { if (this.closed) throw new Error('Private chat manager is closed.'); }
  private transaction<T>(action: () => T, write = true): T {
    this.db.exec(write ? 'BEGIN IMMEDIATE' : 'BEGIN');
    try { const result = action(); this.db.exec('COMMIT'); return result; }
    catch (error) { try { if (this.db.isTransaction) this.db.exec('ROLLBACK'); } catch {} throw error; }
  }
  private row(id: string): ChatRow {
    const row = this.db.prepare('SELECT * FROM private_chats WHERE id=?').get(id) as unknown as ChatRow | undefined;
    if (!row) throw new Error('Private chat not found (404).');
    return row;
  }
  private member(chat: ChatRow, actor: string): void {
    actorLabel(actor, 'Private chat actor');
    if (actor !== chat.participant_a && actor !== chat.participant_b) throw new Error('The selected actor is not a participant in this private chat.');
  }
  private cursor(chatId: string, after: number): void {
    if (after !== 0 && !this.db.prepare('SELECT 1 FROM private_messages WHERE chat_id=? AND sequence=?').get(chatId, after)) {
      throw new Error('The private message cursor does not belong to this chat. Read its history from cursor zero.');
    }
  }
  private notify(chatId: string): void {
    for (const listener of [...this.listeners.get(chatId) ?? []]) { try { listener(); } catch {} }
  }

  create(value: PrivateChatInput): PrivateChat {
    this.open();
    const input = parseSchema<PrivateChatInput>(privateChatCreateDefinition, value, 'private chat');
    for (const actor of input.participants) actorLabel(actor, 'Private chat participant');
    if (input.participants[0] === input.participants[1]) throw new Error('A private chat needs two distinct task actors.');
    if (input.roomId) this.service.rooms.metadata(input.roomId);
    const [first, second] = [...input.participants].sort() as [string, string];
    return this.transaction(() => {
      const existing = this.db.prepare('SELECT * FROM private_chats WHERE participant_a=? AND participant_b=? AND room_id=?')
        .get(first, second, input.roomId ?? '') as unknown as ChatRow | undefined;
      if (existing) return metadata(existing);
      const id = randomUUID(), createdAt = new Date().toISOString();
      this.db.prepare('INSERT INTO private_chats(id,participant_a,participant_b,room_id,created_at) VALUES(?,?,?,?,?)')
        .run(id, first, second, input.roomId ?? '', createdAt);
      return metadata(this.row(id));
    });
  }

  send(value: PrivateChatSendInput): PrivateChatMessage {
    this.open();
    const input = parseSchema<PrivateChatSendInput>(privateChatSendDefinition, value, 'private message');
    actorLabel(input.from, 'Private message sender'); nonempty(input.body, 'Private message body');
    if (input.idempotencyKey !== undefined) actorLabel(input.idempotencyKey, 'Private message retry key');
    const saved = this.transaction(() => {
      const chat = this.row(input.chatId); this.member(chat, input.from);
      if (input.idempotencyKey !== undefined) {
        const duplicate = this.db.prepare('SELECT document FROM private_messages WHERE chat_id=? AND sender=? AND retry_key=?')
          .get(input.chatId, input.from, input.idempotencyKey) as { document: string } | undefined;
        if (duplicate) {
          const message = JSON.parse(duplicate.document) as PrivateChatMessage;
          if (message.body !== input.body || message.runId !== input.runId) throw new PrivateChatConflict('This private message retry key was already used with different content.');
          return message;
        }
      }
      if (input.runId !== undefined && !this.service.hasRun(input.runId)) throw new Error('Referenced private-message workflow not found (404).');
      if (!Number.isSafeInteger(chat.message_count) || chat.message_count >= Number.MAX_SAFE_INTEGER) throw new Error('Private message count space is exhausted.');
      const { idempotencyKey: _, ...details } = input;
      const message: PrivateChatMessage = { ...details, id: randomUUID(), sequence: 0, createdAt: new Date().toISOString() };
      const inserted = this.db.prepare('INSERT INTO private_messages(id,chat_id,sender,retry_key,document) VALUES(?,?,?,?,?)')
        .run(message.id, message.chatId, message.from, input.idempotencyKey ?? null, JSON.stringify(message));
      message.sequence = Number(inserted.lastInsertRowid);
      if (!Number.isSafeInteger(message.sequence)) throw new Error('Private message sequence space is exhausted.');
      this.db.prepare('UPDATE private_messages SET document=? WHERE id=?').run(JSON.stringify(message), message.id);
      this.db.prepare('UPDATE private_chats SET last_message_at=?,message_count=?,last_sequence=? WHERE id=?')
        .run(message.createdAt, chat.message_count + 1, message.sequence, chat.id);
      return message;
    });
    this.notify(input.chatId); return saved;
  }

  page(value: PrivateChatPageInput): PrivateChatPage {
    this.open();
    const input = parseSchema<Required<PrivateChatPageInput>>(privateChatPageDefinition, value, 'private message page');
    return this.transaction(() => {
      const chat = this.row(input.chatId); this.member(chat, input.actor); this.cursor(chat.id, input.after);
      // Read lengths before bodies so later large messages do not consume the
      // page budget or get decoded merely to decide where the next page starts.
      const rows = this.db.prepare('SELECT sequence,length(CAST(document AS BLOB)) AS bytes FROM private_messages WHERE chat_id=? AND sequence>? ORDER BY sequence LIMIT ?')
        .all(chat.id, input.after, input.limit + 1) as { sequence: number; bytes: number }[];
      const messages: PrivateChatMessage[] = [];
      let bytes = 2;
      for (const row of rows.slice(0, input.limit)) {
        const messageBytes = row.bytes + (messages.length ? 1 : 0);
        if (messages.length && bytes + messageBytes > 1024 * 1024) break;
        const payload = this.db.prepare('SELECT document FROM private_messages WHERE chat_id=? AND sequence=?')
          .get(chat.id, row.sequence) as { document: string } | undefined;
        if (!payload) throw new Error('Private message payload is unavailable in the current storage snapshot.');
        messages.push(JSON.parse(payload.document) as PrivateChatMessage);
        bytes += messageBytes;
        // One oversized message remains complete and is returned on its own.
        if (bytes >= 1024 * 1024) break;
      }
      return { messages, nextCursor: rows.length > messages.length ? messages.at(-1)!.sequence : null };
    }, false);
  }

  list(actor: string, roomId?: string): PrivateChat[] {
    this.open();
    const input = parseSchema<{ actor: string; roomId?: string }>(privateChatListDefinition, { actor, ...(roomId === undefined ? {} : { roomId }) }, 'private chat list');
    actorLabel(input.actor, 'Private chat actor');
    const rows = input.roomId === undefined
      ? this.db.prepare('SELECT * FROM private_chats WHERE participant_a=? OR participant_b=? ORDER BY created_at,id').all(input.actor, input.actor)
      : this.db.prepare('SELECT * FROM private_chats WHERE (participant_a=? OR participant_b=?) AND room_id=? ORDER BY created_at,id').all(input.actor, input.actor, input.roomId);
    return (rows as unknown as ChatRow[]).map(metadata);
  }

  actors(roomId?: string): string[] {
    this.open();
    const input = parseSchema<{ roomId?: string }>(privateChatActorsDefinition, roomId === undefined ? {} : { roomId }, 'private chat actors');
    if (input.roomId) this.service.rooms.metadata(input.roomId);
    const rows = input.roomId === undefined
      ? this.db.prepare('SELECT participant_a AS actor FROM private_chats UNION SELECT participant_b AS actor FROM private_chats').all()
      : this.db.prepare('SELECT participant_a AS actor FROM private_chats WHERE room_id=? UNION SELECT participant_b AS actor FROM private_chats WHERE room_id=?').all(input.roomId, input.roomId);
    return (rows as { actor: string }[]).map(row => row.actor).sort();
  }

  get(chatId: string, actor: string): PrivateChat {
    this.open();
    const input = parseSchema<{ chatId: string; actor: string }>(privateChatGetDefinition, { chatId, actor }, 'private chat metadata');
    const row = this.row(input.chatId); this.member(row, input.actor);
    return metadata(row);
  }

  watch(value: PrivateChatWatchInput, signal?: AbortSignal): Promise<PrivateChatWatchResult> {
    this.open();
    const input = parseSchema<Required<PrivateChatWatchInput>>(privateChatWatchDefinition, value, 'private chat watch');
    const cancellation = signal ? AbortSignal.any([signal, this.shutdown.signal]) : this.shutdown.signal;
    cancellation.throwIfAborted();
    const started = performance.now();
    return new Promise<PrivateChatWatchResult>((resolveValue, reject) => {
      let settled = false, running = false, rerun = false, validated = false;
      let timer: ReturnType<typeof setTimeout> | undefined;
      const cleanup = (): void => {
        if (timer) clearTimeout(timer); timer = undefined;
        const listeners = this.listeners.get(input.chatId); listeners?.delete(check);
        if (listeners?.size === 0) this.listeners.delete(input.chatId);
        cancellation.removeEventListener('abort', abort);
      };
      const abort = (): void => { if (settled) return; settled = true; cleanup(); reject(cancellation.reason); };
      const check = (): void => {
        if (settled) return;
        if (running) { rerun = true; return; }
        running = true;
        try {
          do {
            rerun = false; cancellation.throwIfAborted();
            this.open();
            const chat = this.transaction(() => {
              const row = this.row(input.chatId); this.member(row, input.actor);
              if (!validated) { this.cursor(row.id, input.after); validated = true; }
              return metadata(row);
            }, false);
            const changed = chat.lastSequence > input.after, remaining = input.timeoutMs - (performance.now() - started);
            if (changed || remaining <= 0) {
              settled = true; cleanup(); resolveValue({ reason: changed ? 'changed' : 'timeout', chat, lastSequence: chat.lastSequence }); return;
            }
          } while (rerun);
          if (timer) clearTimeout(timer);
          timer = setTimeout(check, Math.max(1, Math.min(100, input.timeoutMs - (performance.now() - started))));
        } catch (error) { settled = true; cleanup(); reject(error); }
        finally { running = false; }
      };
      try {
        let listeners = this.listeners.get(input.chatId);
        if (!listeners) { listeners = new Set(); this.listeners.set(input.chatId, listeners); }
        listeners.add(check); cancellation.addEventListener('abort', abort, { once: true });
        if (cancellation.aborted) abort(); else check();
      } catch (error) { settled = true; cleanup(); reject(error); }
    });
  }

  close(): void {
    if (this.closed) return;
    this.closed = true;
    this.shutdown.abort(new DOMException('Private chat manager is shutting down.', 'AbortError'));
    this.listeners.clear(); this.db.close();
  }
}
