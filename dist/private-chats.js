import { createHash, randomUUID } from 'node:crypto';
import { chmodSync, mkdirSync, realpathSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { DatabaseSync } from 'node:sqlite';
import { parseSchema } from './schema.js';
import { requireSafeSqliteRuntime } from './sqlite-runtime.js';
import { checkAgentJsonStructure } from './agent-json.js';
import { assertUniqueObjectKeys } from './control-json.js';
import { privateChatActorsDefinition, privateChatActorsPageDefinition, privateChatCreateDefinition, privateChatGetDefinition, privateChatListDefinition, privateChatListPageDefinition, privateChatPageDefinition, privateChatSendDefinition, privateChatWatchDefinition } from './private-chats-schema.js';
const METADATA_SHELL_BYTES = Buffer.byteLength('{"id":,"participants":[,],"createdAt":,"messageCount":,"lastSequence":}');
const METADATA_BYTES_SQL = `${METADATA_SHELL_BYTES}
  +length(CAST(json_quote(id) AS BLOB))
  +length(CAST(json_quote(participant_a) AS BLOB))+length(CAST(json_quote(participant_b) AS BLOB))
  +length(CAST(json_quote(created_at) AS BLOB))
  +length(CAST(message_count AS TEXT))+length(CAST(last_sequence AS TEXT))
  +CASE WHEN room_id<>'' THEN 10+length(CAST(json_quote(room_id) AS BLOB)) ELSE 0 END
  +CASE WHEN last_message_at IS NOT NULL AND last_message_at<>'' THEN 17+length(CAST(json_quote(last_message_at) AS BLOB)) ELSE 0 END`;
export class PrivateChatConflict extends Error {
    code = 'COUNCIL_PRIVATE_CHAT_CONFLICT';
    status = 409;
    constructor(message) { super(message); this.name = 'PrivateChatConflict'; }
}
function nonempty(value, label) {
    if (!value.trim())
        throw new Error(`${label} must contain non-whitespace text.`);
}
function actorLabel(value, label) {
    nonempty(value, label);
    if (Buffer.from(value, 'utf8').toString('utf8') !== value)
        throw new Error(`${label} must contain valid Unicode.`);
}
function contention(error) {
    const code = error && typeof error === 'object' && 'errcode' in error ? error.errcode : undefined;
    return typeof code === 'number' && ((code & 0xff) === 5 || (code & 0xff) === 6);
}
function metadata(row) {
    return { id: row.id, participants: [row.participant_a, row.participant_b], ...(row.room_id ? { roomId: row.room_id } : {}),
        createdAt: row.created_at, ...(row.last_message_at ? { lastMessageAt: row.last_message_at } : {}),
        messageCount: row.message_count, lastSequence: row.last_sequence };
}
/** Pair-scoped collaboration history. Actor labels are routing metadata, not an authentication boundary. */
export class PrivateChatManager {
    service;
    db;
    shutdown = new AbortController();
    listeners = new Map();
    closed = false;
    constructor(service) {
        this.service = service;
        requireSafeSqliteRuntime();
        mkdirSync(service.config.stateDir, { recursive: true, mode: 0o700 });
        const filename = join(resolve(service.config.stateDir), 'private-chats.sqlite');
        this.db = new DatabaseSync(filename);
        try {
            this.db.exec('PRAGMA busy_timeout=250; PRAGMA foreign_keys=ON;');
            const version = Number(this.db.prepare('PRAGMA user_version').get().user_version);
            if (version > 1)
                throw new Error('Private chat state was created by a newer Council version.');
            if (process.platform !== 'win32')
                chmodSync(filename, 0o600);
            const journal = this.db.prepare('PRAGMA journal_mode').get();
            if (version === 0 || journal.journal_mode !== 'wal') {
                const deadline = performance.now() + 5000, wait = new Int32Array(new SharedArrayBuffer(4));
                for (;;) {
                    try {
                        this.db.exec(`PRAGMA busy_timeout=${Math.max(1, Math.ceil(deadline - performance.now()))}; PRAGMA journal_mode=WAL;`);
                        this.transaction(() => {
                            const current = Number(this.db.prepare('PRAGMA user_version').get().user_version);
                            if (current > 1)
                                throw new Error('Private chat state was created by a newer Council version.');
                            if (current === 1)
                                return;
                            this.db.exec(`CREATE TABLE private_chats(id TEXT PRIMARY KEY, participant_a TEXT NOT NULL, participant_b TEXT NOT NULL,
                room_id TEXT NOT NULL, created_at TEXT NOT NULL, last_message_at TEXT, message_count INTEGER NOT NULL DEFAULT 0,
                last_sequence INTEGER NOT NULL DEFAULT 0, UNIQUE(participant_a,participant_b,room_id));
                CREATE INDEX private_chat_actor_a ON private_chats(participant_a,created_at,id);
                CREATE INDEX private_chat_actor_b ON private_chats(participant_b,created_at,id);
                CREATE TABLE private_messages(sequence INTEGER PRIMARY KEY AUTOINCREMENT, id TEXT NOT NULL UNIQUE,
                  chat_id TEXT NOT NULL REFERENCES private_chats(id), sender TEXT NOT NULL, retry_key TEXT,
                  document TEXT NOT NULL, UNIQUE(chat_id,sender,retry_key));
                CREATE INDEX private_message_page ON private_messages(chat_id,sequence); PRAGMA user_version=1;`);
                        });
                        break;
                    }
                    catch (error) {
                        const remaining = deadline - performance.now();
                        if (!contention(error) || remaining <= 0)
                            throw error;
                        Atomics.wait(wait, 0, 0, Math.min(25, remaining));
                    }
                }
                this.db.exec('PRAGMA busy_timeout=250;');
            }
            if (process.platform !== 'win32')
                for (const path of [`${filename}-wal`, `${filename}-shm`]) {
                    try {
                        chmodSync(path, 0o600);
                    }
                    catch (error) {
                        if (error.code !== 'ENOENT')
                            throw error;
                    }
                }
        }
        catch (error) {
            this.closed = true;
            this.db.close();
            throw error;
        }
    }
    open() { if (this.closed)
        throw new Error('Private chat manager is closed.'); }
    transaction(action, write = true) {
        this.db.exec(write ? 'BEGIN IMMEDIATE' : 'BEGIN');
        try {
            const result = action();
            this.db.exec('COMMIT');
            return result;
        }
        catch (error) {
            try {
                if (this.db.isTransaction)
                    this.db.exec('ROLLBACK');
            }
            catch { }
            throw error;
        }
    }
    row(id) {
        const row = this.db.prepare('SELECT * FROM private_chats WHERE id=?').get(id);
        if (!row)
            throw new Error('Private chat not found (404).');
        return row;
    }
    member(chat, actor) {
        actorLabel(actor, 'Private chat actor');
        if (actor !== chat.participant_a && actor !== chat.participant_b)
            throw new Error('The selected actor is not a participant in this private chat.');
    }
    cursor(chatId, after) {
        if (after !== 0 && !this.db.prepare('SELECT 1 FROM private_messages WHERE chat_id=? AND sequence=?').get(chatId, after)) {
            throw new Error('The private message cursor does not belong to this chat. Read its history from cursor zero.');
        }
    }
    discoveryScope(kind, actor, roomId) {
        return createHash('sha256').update(JSON.stringify([realpathSync.native(this.service.config.stateDir), kind, actor ?? null, roomId ?? null])).digest('hex');
    }
    readDiscoveryCursor(value, kind, scope) {
        if (value === undefined)
            return undefined;
        try {
            if (value.length > 1024 || !/^[a-zA-Z0-9_-]+$/.test(value))
                throw new Error();
            const bytes = Buffer.from(value, 'base64url'), text = bytes.toString('utf8');
            if (bytes.toString('base64url') !== value || !Buffer.from(text).equals(bytes))
                throw new Error();
            checkAgentJsonStructure(bytes);
            const parsed = JSON.parse(text);
            assertUniqueObjectKeys(text, 'Private discovery cursor');
            const cursor = parseSchema({
                type: 'object', additionalProperties: false, required: ['version', 'kind', 'scope', 'chatId'],
                properties: {
                    version: { type: 'integer', const: 1 }, kind: { type: 'string', enum: ['threads', 'actors'] },
                    scope: { type: 'string', pattern: '^[a-f0-9]{64}$', minLength: 64, maxLength: 64 },
                    chatId: { type: 'string', minLength: 1, maxLength: 100, pattern: '^[a-zA-Z0-9_-]+$' },
                    side: { type: 'string', enum: ['a', 'b'] },
                },
            }, parsed, 'private discovery cursor');
            if (cursor.kind !== kind || cursor.scope !== scope || (kind === 'actors') !== (cursor.side !== undefined))
                throw new Error();
            return cursor;
        }
        catch {
            throw new Error('Invalid private discovery cursor or mismatched participant, room or workspace scope. Start from the first page.');
        }
    }
    writeDiscoveryCursor(kind, scope, chatId, side) {
        return Buffer.from(JSON.stringify({ version: 1, kind, scope, chatId, ...(side ? { side } : {}) })).toString('base64url');
    }
    listPage(value) {
        this.open();
        const input = parseSchema(privateChatListPageDefinition, value, 'private thread page');
        actorLabel(input.actor, 'Private chat actor');
        if (input.roomId !== undefined)
            this.service.rooms.observation(input.roomId);
        const scope = this.discoveryScope('threads', input.actor, input.roomId), cursor = this.readDiscoveryCursor(input.cursor, 'threads', scope);
        return this.transaction(() => {
            let createdAt = '', id = '';
            if (cursor) {
                const anchor = this.db.prepare('SELECT id,created_at,room_id,(participant_a=? OR participant_b=?) AS member FROM private_chats WHERE id=?')
                    .get(input.actor, input.actor, cursor.chatId);
                if (!anchor || !anchor.member || input.roomId !== undefined && anchor.room_id !== input.roomId)
                    throw new Error('Private discovery cursor is unavailable or outside this scope. Start from the first page.');
                createdAt = anchor.created_at;
                id = anchor.id;
            }
            const parameters = input.roomId === undefined ? [input.actor, input.actor] : [input.actor, input.actor, input.roomId];
            const rows = this.db.prepare(`SELECT id,${METADATA_BYTES_SQL} AS bytes FROM private_chats
        WHERE (participant_a=? OR participant_b=?) ${input.roomId === undefined ? '' : 'AND room_id=?'}
        AND (created_at>? OR (created_at=? AND id>?)) ORDER BY created_at,id LIMIT ?`)
                .all(...parameters, createdAt, createdAt, id, input.limit + 1);
            const chats = [];
            let bytes = 2;
            for (const row of rows.slice(0, input.limit)) {
                const size = row.bytes + (chats.length ? 1 : 0);
                if (chats.length && bytes + size > 1024 * 1024)
                    break;
                chats.push(metadata(this.row(row.id)));
                bytes += size;
                if (bytes >= 1024 * 1024)
                    break;
            }
            return { chats, nextCursor: rows.length > chats.length ? this.writeDiscoveryCursor('threads', scope, chats.at(-1).id) : null };
        }, false);
    }
    actorsPage(value = {}) {
        this.open();
        const input = parseSchema(privateChatActorsPageDefinition, value, 'private participant page');
        if (input.roomId !== undefined)
            this.service.rooms.observation(input.roomId);
        const scope = this.discoveryScope('actors', undefined, input.roomId), cursor = this.readDiscoveryCursor(input.cursor, 'actors', scope);
        return this.transaction(() => {
            let after = '';
            if (cursor) {
                const field = cursor.side === 'a' ? 'participant_a' : 'participant_b';
                const anchor = this.db.prepare(`SELECT ${field} AS actor,room_id FROM private_chats WHERE id=?`).get(cursor.chatId);
                if (!anchor || input.roomId !== undefined && anchor.room_id !== input.roomId)
                    throw new Error('Private discovery cursor is unavailable or outside this scope. Start from the first page.');
                after = anchor.actor;
            }
            const condition = input.roomId === undefined ? '' : 'WHERE room_id=?';
            const parameters = input.roomId === undefined ? [] : [input.roomId, input.roomId];
            const rows = this.db.prepare(`SELECT MIN(id||side) AS anchor,length(CAST(json_quote(actor) AS BLOB)) AS bytes FROM (
        SELECT participant_a AS actor,id,'a' AS side FROM private_chats ${condition}
        UNION ALL SELECT participant_b AS actor,id,'b' AS side FROM private_chats ${condition})
        WHERE actor>? COLLATE BINARY GROUP BY actor COLLATE BINARY ORDER BY actor COLLATE BINARY LIMIT ?`)
                .all(...parameters, after, input.limit + 1);
            const actors = [];
            let bytes = 2, last;
            for (const row of rows.slice(0, input.limit)) {
                const size = row.bytes + (actors.length ? 1 : 0);
                if (actors.length && bytes + size > 1024 * 1024)
                    break;
                const id = row.anchor.slice(0, -1), side = row.anchor.at(-1), field = side === 'a' ? 'participant_a' : 'participant_b';
                const payload = this.db.prepare(`SELECT ${field} AS actor FROM private_chats WHERE id=?`).get(id);
                if (!payload)
                    throw new Error('Private participant metadata is unavailable in the current storage snapshot.');
                actors.push(payload.actor);
                bytes += size;
                last = { id, side };
                if (bytes >= 1024 * 1024)
                    break;
            }
            return { actors, nextCursor: rows.length > actors.length ? this.writeDiscoveryCursor('actors', scope, last.id, last.side) : null };
        }, false);
    }
    notify(chatId) {
        for (const listener of [...this.listeners.get(chatId) ?? []]) {
            try {
                listener();
            }
            catch { }
        }
    }
    create(value) {
        this.open();
        const input = parseSchema(privateChatCreateDefinition, value, 'private chat');
        for (const actor of input.participants)
            actorLabel(actor, 'Private chat participant');
        if (input.participants[0] === input.participants[1])
            throw new Error('A private chat needs two distinct task actors.');
        if (input.roomId)
            this.service.rooms.metadata(input.roomId);
        const [first, second] = [...input.participants].sort();
        return this.transaction(() => {
            const existing = this.db.prepare('SELECT * FROM private_chats WHERE participant_a=? AND participant_b=? AND room_id=?')
                .get(first, second, input.roomId ?? '');
            if (existing)
                return metadata(existing);
            const id = randomUUID(), createdAt = new Date().toISOString();
            this.db.prepare('INSERT INTO private_chats(id,participant_a,participant_b,room_id,created_at) VALUES(?,?,?,?,?)')
                .run(id, first, second, input.roomId ?? '', createdAt);
            return metadata(this.row(id));
        });
    }
    send(value) {
        this.open();
        const input = parseSchema(privateChatSendDefinition, value, 'private message');
        actorLabel(input.from, 'Private message sender');
        nonempty(input.body, 'Private message body');
        if (input.idempotencyKey !== undefined)
            actorLabel(input.idempotencyKey, 'Private message retry key');
        const saved = this.transaction(() => {
            const chat = this.row(input.chatId);
            this.member(chat, input.from);
            if (input.idempotencyKey !== undefined) {
                const duplicate = this.db.prepare('SELECT document FROM private_messages WHERE chat_id=? AND sender=? AND retry_key=?')
                    .get(input.chatId, input.from, input.idempotencyKey);
                if (duplicate) {
                    const message = JSON.parse(duplicate.document);
                    if (message.body !== input.body || message.runId !== input.runId)
                        throw new PrivateChatConflict('This private message retry key was already used with different content.');
                    return message;
                }
            }
            if (input.runId !== undefined && !this.service.hasRun(input.runId))
                throw new Error('Referenced private-message workflow not found (404).');
            if (!Number.isSafeInteger(chat.message_count) || chat.message_count >= Number.MAX_SAFE_INTEGER)
                throw new Error('Private message count space is exhausted.');
            const { idempotencyKey: _, ...details } = input;
            const message = { ...details, id: randomUUID(), sequence: 0, createdAt: new Date().toISOString() };
            const inserted = this.db.prepare('INSERT INTO private_messages(id,chat_id,sender,retry_key,document) VALUES(?,?,?,?,?)')
                .run(message.id, message.chatId, message.from, input.idempotencyKey ?? null, JSON.stringify(message));
            message.sequence = Number(inserted.lastInsertRowid);
            if (!Number.isSafeInteger(message.sequence))
                throw new Error('Private message sequence space is exhausted.');
            this.db.prepare('UPDATE private_messages SET document=? WHERE id=?').run(JSON.stringify(message), message.id);
            this.db.prepare('UPDATE private_chats SET last_message_at=?,message_count=?,last_sequence=? WHERE id=?')
                .run(message.createdAt, chat.message_count + 1, message.sequence, chat.id);
            return message;
        });
        this.notify(input.chatId);
        return saved;
    }
    page(value) {
        this.open();
        const input = parseSchema(privateChatPageDefinition, value, 'private message page');
        return this.transaction(() => {
            const chat = this.row(input.chatId);
            this.member(chat, input.actor);
            this.cursor(chat.id, input.after);
            // Read lengths before bodies so later large messages do not consume the
            // page budget or get decoded merely to decide where the next page starts.
            const rows = this.db.prepare('SELECT sequence,length(CAST(document AS BLOB)) AS bytes FROM private_messages WHERE chat_id=? AND sequence>? ORDER BY sequence LIMIT ?')
                .all(chat.id, input.after, input.limit + 1);
            const messages = [];
            let bytes = 2;
            for (const row of rows.slice(0, input.limit)) {
                const messageBytes = row.bytes + (messages.length ? 1 : 0);
                if (messages.length && bytes + messageBytes > 1024 * 1024)
                    break;
                const payload = this.db.prepare('SELECT document FROM private_messages WHERE chat_id=? AND sequence=?')
                    .get(chat.id, row.sequence);
                if (!payload)
                    throw new Error('Private message payload is unavailable in the current storage snapshot.');
                messages.push(JSON.parse(payload.document));
                bytes += messageBytes;
                // One oversized message remains complete and is returned on its own.
                if (bytes >= 1024 * 1024)
                    break;
            }
            return { messages, nextCursor: rows.length > messages.length ? messages.at(-1).sequence : null };
        }, false);
    }
    list(actor, roomId) {
        this.open();
        const input = parseSchema(privateChatListDefinition, { actor, ...(roomId === undefined ? {} : { roomId }) }, 'private chat list');
        actorLabel(input.actor, 'Private chat actor');
        const rows = input.roomId === undefined
            ? this.db.prepare('SELECT * FROM private_chats WHERE participant_a=? OR participant_b=? ORDER BY created_at,id').all(input.actor, input.actor)
            : this.db.prepare('SELECT * FROM private_chats WHERE (participant_a=? OR participant_b=?) AND room_id=? ORDER BY created_at,id').all(input.actor, input.actor, input.roomId);
        return rows.map(metadata);
    }
    actors(roomId) {
        this.open();
        const input = parseSchema(privateChatActorsDefinition, roomId === undefined ? {} : { roomId }, 'private chat actors');
        if (input.roomId)
            this.service.rooms.metadata(input.roomId);
        const rows = input.roomId === undefined
            ? this.db.prepare('SELECT participant_a AS actor FROM private_chats UNION SELECT participant_b AS actor FROM private_chats').all()
            : this.db.prepare('SELECT participant_a AS actor FROM private_chats WHERE room_id=? UNION SELECT participant_b AS actor FROM private_chats WHERE room_id=?').all(input.roomId, input.roomId);
        return rows.map(row => row.actor).sort();
    }
    get(chatId, actor) {
        this.open();
        const input = parseSchema(privateChatGetDefinition, { chatId, actor }, 'private chat metadata');
        const row = this.row(input.chatId);
        this.member(row, input.actor);
        return metadata(row);
    }
    watch(value, signal) {
        this.open();
        const input = parseSchema(privateChatWatchDefinition, value, 'private chat watch');
        const cancellation = signal ? AbortSignal.any([signal, this.shutdown.signal]) : this.shutdown.signal;
        cancellation.throwIfAborted();
        const started = performance.now();
        return new Promise((resolveValue, reject) => {
            let settled = false, running = false, rerun = false, validated = false;
            let timer;
            const cleanup = () => {
                if (timer)
                    clearTimeout(timer);
                timer = undefined;
                const listeners = this.listeners.get(input.chatId);
                listeners?.delete(check);
                if (listeners?.size === 0)
                    this.listeners.delete(input.chatId);
                cancellation.removeEventListener('abort', abort);
            };
            const abort = () => { if (settled)
                return; settled = true; cleanup(); reject(cancellation.reason); };
            const check = () => {
                if (settled)
                    return;
                if (running) {
                    rerun = true;
                    return;
                }
                running = true;
                try {
                    do {
                        rerun = false;
                        cancellation.throwIfAborted();
                        this.open();
                        const chat = this.transaction(() => {
                            const row = this.row(input.chatId);
                            this.member(row, input.actor);
                            if (!validated) {
                                this.cursor(row.id, input.after);
                                validated = true;
                            }
                            return metadata(row);
                        }, false);
                        const changed = chat.lastSequence > input.after, remaining = input.timeoutMs - (performance.now() - started);
                        if (changed || remaining <= 0) {
                            settled = true;
                            cleanup();
                            resolveValue({ reason: changed ? 'changed' : 'timeout', chat, lastSequence: chat.lastSequence });
                            return;
                        }
                    } while (rerun);
                    if (timer)
                        clearTimeout(timer);
                    timer = setTimeout(check, Math.max(1, Math.min(100, input.timeoutMs - (performance.now() - started))));
                }
                catch (error) {
                    settled = true;
                    cleanup();
                    reject(error);
                }
                finally {
                    running = false;
                }
            };
            try {
                let listeners = this.listeners.get(input.chatId);
                if (!listeners) {
                    listeners = new Set();
                    this.listeners.set(input.chatId, listeners);
                }
                listeners.add(check);
                cancellation.addEventListener('abort', abort, { once: true });
                if (cancellation.aborted)
                    abort();
                else
                    check();
            }
            catch (error) {
                settled = true;
                cleanup();
                reject(error);
            }
        });
    }
    close() {
        if (this.closed)
            return;
        this.closed = true;
        this.shutdown.abort(new DOMException('Private chat manager is shutting down.', 'AbortError'));
        this.listeners.clear();
        this.db.close();
    }
}
//# sourceMappingURL=private-chats.js.map