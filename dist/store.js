import { randomUUID } from 'node:crypto';
import { chmodSync, mkdirSync } from 'node:fs';
import { join } from 'node:path';
import { DatabaseSync } from 'node:sqlite';
const SCHEMA_VERSION = 2;
const MAX_JSON_BYTES = 4 * 1024 * 1024;
const MAX_RUN_JSON_BYTES = 16 * 1024 * 1024;
function text(value, label, maxBytes) {
    if (typeof value !== 'string' || !value.trim() || Buffer.byteLength(value, 'utf8') > maxBytes || value.includes('\0')) {
        throw new Error(`${label} must be non-empty text of at most ${maxBytes} UTF-8 bytes`);
    }
    return value;
}
function encode(value, maxBytes = MAX_JSON_BYTES) {
    const json = JSON.stringify(value);
    if (json === undefined || Buffer.byteLength(json, 'utf8') > maxBytes) {
        throw new Error(`Stored JSON must be serializable and no larger than ${maxBytes / (1024 * 1024)} MiB`);
    }
    return json;
}
function normalizePath(value) {
    text(value, 'Reservation path', 4096);
    const normalized = value.replaceAll('\\', '/');
    if (normalized.startsWith('/') || /^[a-z]:/i.test(normalized) || /[\x00-\x1f:]/.test(normalized)) {
        throw new Error('Reservation paths must be workspace-relative paths');
    }
    if (normalized === '**')
        return normalized;
    const subtree = normalized.endsWith('/**');
    const base = subtree ? normalized.slice(0, -3) : normalized;
    if (!base || base.split('/').some(part => !part || part === '..' || part === '.') || /[*?]/.test(base)) {
        throw new Error('Reservation paths cannot contain traversal, empty segments, or unsupported globs; use directory/**');
    }
    return normalized;
}
function overlaps(left, right) {
    const a = left.toLowerCase();
    const b = right.toLowerCase();
    if (a === b || a === '**' || b === '**')
        return true;
    const contains = (pattern, candidate) => {
        if (!pattern.endsWith('/**'))
            return false;
        const base = pattern.slice(0, -3);
        const candidateBase = candidate.endsWith('/**') ? candidate.slice(0, -3) : candidate;
        return candidateBase === base || candidateBase.startsWith(`${base}/`);
    };
    return contains(a, b) || contains(b, a);
}
function processAlive(pid) {
    if (pid === process.pid)
        return true;
    try {
        process.kill(pid, 0);
        return true;
    }
    catch (error) {
        return error.code !== 'ESRCH';
    }
}
/** Durable collaboration state shared by all Council processes in one workspace. */
export class CouncilStore {
    db;
    constructor(stateDir) {
        mkdirSync(stateDir, { recursive: true, mode: 0o700 });
        const databasePath = join(stateDir, 'state.db');
        this.db = new DatabaseSync(databasePath);
        try {
            this.db.exec('PRAGMA busy_timeout = 5000; PRAGMA journal_mode = WAL; PRAGMA foreign_keys = ON;');
            this.transaction(() => {
                // Recheck under the migration lock: another process may have upgraded while opening.
                const version = this.db.prepare('PRAGMA user_version').get();
                if (version.user_version > SCHEMA_VERSION)
                    throw new Error(`Council state schema ${version.user_version} is newer than supported schema ${SCHEMA_VERSION}`);
                this.db.exec(`
          CREATE TABLE IF NOT EXISTS runs (id TEXT PRIMARY KEY, updated_at TEXT NOT NULL, document TEXT NOT NULL);
          CREATE TABLE IF NOT EXISTS events (sequence INTEGER PRIMARY KEY AUTOINCREMENT, run_id TEXT NOT NULL, type TEXT NOT NULL, data TEXT NOT NULL, created_at TEXT NOT NULL);
          CREATE INDEX IF NOT EXISTS events_run_sequence ON events(run_id, sequence);
          CREATE TABLE IF NOT EXISTS messages (id TEXT PRIMARY KEY, sender TEXT NOT NULL, recipient TEXT NOT NULL, created_at TEXT NOT NULL, acknowledged_at TEXT, document TEXT NOT NULL);
          CREATE INDEX IF NOT EXISTS messages_inbox ON messages(recipient, created_at);
          CREATE TABLE IF NOT EXISTS leases (id TEXT PRIMARY KEY, owner TEXT NOT NULL, path TEXT NOT NULL, path_key TEXT NOT NULL UNIQUE, run_id TEXT, expires_at INTEGER NOT NULL);
          CREATE TABLE IF NOT EXISTS context (key TEXT PRIMARY KEY, value TEXT NOT NULL, author TEXT NOT NULL, updated_at TEXT NOT NULL);
        `);
                if (version.user_version < 2)
                    this.db.exec(`
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
            if (process.platform !== 'win32')
                chmodSync(databasePath, 0o600);
        }
        catch (error) {
            this.db.close();
            throw error;
        }
    }
    transaction(operation) {
        this.db.exec('BEGIN IMMEDIATE');
        try {
            const result = operation();
            this.db.exec('COMMIT');
            return result;
        }
        catch (error) {
            this.db.exec('ROLLBACK');
            throw error;
        }
    }
    saveRun(run) {
        text(run.id, 'Run id', 256);
        text(run.updatedAt, 'Run timestamp', 64);
        const document = encode(run, MAX_RUN_JSON_BYTES);
        this.transaction(() => {
            const existing = this.ownedRun(run.id);
            if (existing?.owner_instance)
                this.assertRunOwner(existing);
            // Cancellation and owner columns are deliberately absent from the partial-update clause.
            this.db.prepare('INSERT INTO runs(id, updated_at, document) VALUES (?, ?, ?) ON CONFLICT(id) DO UPDATE SET updated_at = excluded.updated_at, document = excluded.document')
                .run(run.id, run.updatedAt, document);
        });
    }
    getRun(id) {
        text(id, 'Run id', 256);
        const row = this.db.prepare('SELECT document FROM runs WHERE id = ?').get(id);
        return row ? JSON.parse(row.document) : undefined;
    }
    listRuns(limit = 20) {
        if (!Number.isInteger(limit) || limit < 1)
            throw new Error('Run list limit must be a positive integer');
        const rows = this.db.prepare('SELECT document FROM runs ORDER BY updated_at DESC, id LIMIT ?').all(Math.min(limit, 100));
        return rows.map(row => JSON.parse(row.document));
    }
    appendEvent(runId, type, data) {
        text(runId, 'Run id', 256);
        text(type, 'Event type', 128);
        const createdAt = new Date().toISOString();
        const result = this.db.prepare('INSERT INTO events(run_id, type, data, created_at) VALUES (?, ?, ?, ?)').run(runId, type, encode(data), createdAt);
        return { sequence: Number(result.lastInsertRowid), runId, type, data, createdAt };
    }
    events(runId, after = 0) {
        text(runId, 'Run id', 256);
        if (!Number.isSafeInteger(after) || after < 0)
            throw new Error('Event cursor must be a non-negative integer');
        const rows = this.db.prepare('SELECT sequence, run_id, type, data, created_at FROM events WHERE run_id = ? AND sequence > ? ORDER BY sequence LIMIT 1000').all(runId, after);
        return rows.map(row => ({ sequence: row.sequence, runId: row.run_id, type: row.type, data: JSON.parse(row.data), createdAt: row.created_at }));
    }
    sendMessage(input) {
        text(input.from, 'Sender', 128);
        text(input.to, 'Recipient', 128);
        text(input.subject, 'Message subject', 512);
        text(input.body, 'Message body', 32 * 1024);
        if (input.runId !== undefined)
            text(input.runId, 'Run id', 256);
        const message = { ...input, id: randomUUID(), createdAt: new Date().toISOString() };
        this.db.prepare('INSERT INTO messages(id, sender, recipient, created_at, document) VALUES (?, ?, ?, ?, ?)')
            .run(message.id, message.from, message.to, message.createdAt, encode(message));
        return message;
    }
    inbox(agent, unreadOnly = true) {
        text(agent, 'Agent', 128);
        const rows = this.db.prepare('SELECT document, acknowledged_at FROM messages WHERE recipient = ? AND (? = 0 OR acknowledged_at IS NULL) ORDER BY created_at, rowid LIMIT 100')
            .all(agent, unreadOnly ? 1 : 0);
        return rows.map(row => {
            const message = JSON.parse(row.document);
            if (row.acknowledged_at)
                message.acknowledgedAt = row.acknowledged_at;
            return message;
        });
    }
    acknowledge(id, agent) {
        text(id, 'Message id', 256);
        text(agent, 'Agent', 128);
        const result = this.db.prepare('UPDATE messages SET acknowledged_at = ? WHERE id = ? AND recipient = ? AND acknowledged_at IS NULL')
            .run(new Date().toISOString(), id, agent);
        return result.changes > 0;
    }
    reserve(owner, paths, ttlMs, runId) {
        text(owner, 'Lease owner', 128);
        if (!Array.isArray(paths) || paths.length < 1 || paths.length > 100)
            throw new Error('Reserve between 1 and 100 paths');
        if (!Number.isSafeInteger(ttlMs) || ttlMs < 1 || ttlMs > 24 * 60 * 60 * 1000)
            throw new Error('Lease TTL must be between 1 ms and 24 hours');
        if (runId !== undefined)
            text(runId, 'Run id', 256);
        const normalized = [...new Map(paths.map(path => { const value = normalizePath(path); return [value.toLowerCase(), value]; })).values()];
        return this.transaction(() => {
            const now = Date.now();
            this.db.prepare('DELETE FROM leases WHERE expires_at <= ?').run(now);
            const active = this.readLeases(now);
            for (const requested of normalized) {
                const conflict = active.find(lease => lease.owner !== owner && lease.paths.some(path => overlaps(path, requested)));
                if (conflict)
                    throw new Error(`Reservation conflict: ${requested} overlaps ${conflict.paths[0]} owned by ${conflict.owner} until ${conflict.expiresAt}`);
            }
            const expiresAt = new Date(now + ttlMs).toISOString();
            return normalized.map(path => {
                const existing = active.find(lease => lease.owner === owner && lease.paths[0]?.toLowerCase() === path.toLowerCase());
                const lease = { id: existing?.id ?? randomUUID(), owner, paths: [path], expiresAt, ...(runId === undefined ? {} : { runId }) };
                this.db.prepare('INSERT INTO leases(id, owner, path, path_key, run_id, expires_at) VALUES (?, ?, ?, ?, ?, ?) ON CONFLICT(path_key) DO UPDATE SET id = excluded.id, owner = excluded.owner, path = excluded.path, run_id = excluded.run_id, expires_at = excluded.expires_at')
                    .run(lease.id, owner, path, path.toLowerCase(), runId ?? null, now + ttlMs);
                return lease;
            });
        });
    }
    release(owner, paths) {
        text(owner, 'Lease owner', 128);
        if (paths === undefined)
            return Number(this.db.prepare('DELETE FROM leases WHERE owner = ?').run(owner).changes);
        if (!Array.isArray(paths) || paths.length > 100)
            throw new Error('Release at most 100 paths');
        const normalized = paths.map(normalizePath);
        return this.transaction(() => normalized.reduce((count, path) => count + Number(this.db.prepare('DELETE FROM leases WHERE owner = ? AND path_key = ?').run(owner, path.toLowerCase()).changes), 0));
    }
    readLeases(now = Date.now()) {
        const rows = this.db.prepare('SELECT id, owner, path, run_id, expires_at FROM leases WHERE expires_at > ? ORDER BY path_key').all(now);
        return rows.map(row => ({ id: row.id, owner: row.owner, paths: [row.path], expiresAt: new Date(row.expires_at).toISOString(), ...(row.run_id === null ? {} : { runId: row.run_id }) }));
    }
    leases() {
        return this.readLeases();
    }
    putContext(key, value, author) {
        text(key, 'Context key', 256);
        text(value, 'Context value', 64 * 1024);
        text(author, 'Context author', 128);
        const entry = { key, value, author, updatedAt: new Date().toISOString() };
        this.db.prepare('INSERT INTO context(key, value, author, updated_at) VALUES (?, ?, ?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value, author = excluded.author, updated_at = excluded.updated_at')
            .run(key, value, author, entry.updatedAt);
        return entry;
    }
    getContext(key) {
        text(key, 'Context key', 256);
        const row = this.db.prepare('SELECT key, value, author, updated_at FROM context WHERE key = ?').get(key);
        return row ? { key: row.key, value: row.value, author: row.author, updatedAt: row.updated_at } : undefined;
    }
    listContext() {
        const rows = this.db.prepare('SELECT key, value, author, updated_at FROM context ORDER BY updated_at DESC, key LIMIT 100').all();
        return rows.map(row => ({ key: row.key, value: row.value, author: row.author, updatedAt: row.updated_at }));
    }
    registerInstance(instanceId, pid) {
        text(instanceId, 'Instance id', 128);
        if (!Number.isSafeInteger(pid) || pid !== process.pid)
            throw new Error('A Council instance must register its own process PID.');
        this.transaction(() => {
            const existing = this.instance(instanceId);
            if (existing && existing.pid !== pid)
                throw new Error('This Council instance ID is already registered by another process.');
            const now = Date.now();
            this.db.prepare('INSERT INTO instances(id, pid, registered_at, heartbeat_at) VALUES (?, ?, ?, ?) ON CONFLICT(id) DO UPDATE SET heartbeat_at = excluded.heartbeat_at')
                .run(instanceId, pid, now, now);
        });
    }
    heartbeat(instanceId) {
        text(instanceId, 'Instance id', 128);
        return this.db.prepare('UPDATE instances SET heartbeat_at = ? WHERE id = ? AND pid = ?').run(Date.now(), instanceId, process.pid).changes > 0;
    }
    unregisterInstance(instanceId) {
        text(instanceId, 'Instance id', 128);
        this.transaction(() => {
            const instance = this.instance(instanceId);
            if (!instance)
                return;
            if (instance.pid !== process.pid)
                throw new Error('Only the owning process can unregister a Council instance.');
            const active = this.db.prepare("SELECT 1 FROM runs WHERE owner_instance = ? AND json_extract(document, '$.status') IN ('queued', 'running') LIMIT 1").get(instanceId);
            if (active) {
                throw new Error('Cannot unregister a Council instance while it owns unfinished runs.');
            }
            this.db.prepare('DELETE FROM instances WHERE id = ?').run(instanceId);
        });
    }
    claimRun(runId, instanceId) {
        text(runId, 'Run id', 256);
        text(instanceId, 'Instance id', 128);
        this.transaction(() => {
            const instance = this.assertInstance(instanceId);
            const row = this.ownedRun(runId);
            if (!row)
                throw new Error('Cannot claim a run that does not exist.');
            if (!['queued', 'running'].includes(JSON.parse(row.document).status))
                throw new Error('Cannot claim a run that already reached a terminal state.');
            if (row.owner_instance && row.owner_instance !== instanceId) {
                const owner = this.instance(row.owner_instance);
                if (owner && processAlive(owner.pid))
                    throw new Error('This run is already owned by a live Council instance.');
                throw new Error('This run has an orphaned owner; recover it before starting a replacement.');
            }
            this.db.prepare('UPDATE runs SET owner_instance = ?, owner_pid = ? WHERE id = ?').run(instanceId, instance.pid, runId);
        });
    }
    requestCancellation(runId) {
        text(runId, 'Run id', 256);
        return this.transaction(() => {
            const row = this.ownedRun(runId);
            if (!row || !['queued', 'running'].includes(JSON.parse(row.document).status) || row.cancel_requested)
                return false;
            this.db.prepare('UPDATE runs SET cancel_requested = 1 WHERE id = ?').run(runId);
            if (!this.legacyCancellation(runId))
                this.appendEvent(runId, 'cancel_requested', {});
            return true;
        });
    }
    /** Final completion and a concurrent cancellation request are ordered by one SQLite transaction. */
    finalizeRun(run) {
        text(run.id, 'Run id', 256);
        if (['queued', 'running'].includes(run.status))
            throw new Error('Only a terminal run can be finalized.');
        return this.transaction(() => {
            const row = this.ownedRun(run.id);
            if (!row)
                throw new Error('Cannot finalize a run that does not exist.');
            const existing = JSON.parse(row.document);
            if (row.owner_instance)
                this.assertRunOwner(row);
            // A previously settled failure/cancellation cannot be rewritten by a stale completion.
            if (['failed', 'cancelled'].includes(existing.status) && existing.status !== run.status)
                return existing;
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
    acquireSlot(instanceId, runId, limit) {
        text(instanceId, 'Instance id', 128);
        text(runId, 'Run id', 256);
        if (!Number.isInteger(limit) || limit < 1 || limit > 32)
            throw new Error('The shared concurrency limit must be from 1 to 32.');
        return this.transaction(() => {
            this.assertInstance(instanceId);
            this.recoverDeadOwners();
            const row = this.ownedRun(runId);
            if (!row || row.owner_instance !== instanceId)
                throw new Error('The Council instance does not own this run.');
            if (!['queued', 'running'].includes(JSON.parse(row.document).status))
                throw new Error('This run is no longer active.');
            if (row.cancel_requested || this.legacyCancellation(runId))
                throw new Error('Cancellation has been requested for this run.');
            this.db.prepare('UPDATE instances SET slot_limit = ? WHERE id = ?').run(limit, instanceId);
            const minimum = this.db.prepare('SELECT MIN(slot_limit) AS value FROM instances WHERE slot_limit IS NOT NULL').get();
            const active = this.db.prepare('SELECT COUNT(*) AS value FROM slots').get();
            if (active.value >= minimum.value)
                return undefined;
            const token = randomUUID();
            this.db.prepare('INSERT INTO slots(token, instance_id, run_id, acquired_at) VALUES (?, ?, ?, ?)').run(token, instanceId, runId, Date.now());
            return token;
        });
    }
    releaseSlot(token, instanceId) {
        text(token, 'Slot token', 256);
        text(instanceId, 'Instance id', 128);
        this.db.prepare('DELETE FROM slots WHERE token = ? AND instance_id = ?').run(token, instanceId);
    }
    /** Heartbeat age alone is never proof that a process is dead. PID reuse is intentionally conservative. */
    recoverOrphans() { return this.transaction(() => this.recoverDeadOwners()); }
    instance(instanceId) {
        return this.db.prepare('SELECT id, pid, heartbeat_at, slot_limit FROM instances WHERE id = ?').get(instanceId);
    }
    assertInstance(instanceId) {
        const instance = this.instance(instanceId);
        if (!instance || instance.pid !== process.pid || !processAlive(instance.pid))
            throw new Error('The Council owner instance is not registered to this live process.');
        return instance;
    }
    ownedRun(runId) {
        return this.db.prepare('SELECT document, owner_instance, owner_pid, cancel_requested FROM runs WHERE id = ?').get(runId);
    }
    assertRunOwner(row) {
        if (!row.owner_instance)
            return;
        const owner = this.instance(row.owner_instance);
        if (!owner || owner.pid !== process.pid || owner.pid !== row.owner_pid)
            throw new Error('This process no longer owns the Council run.');
    }
    legacyCancellation(runId) {
        return this.db.prepare("SELECT 1 FROM events WHERE run_id = ? AND type = 'cancel_requested' LIMIT 1").get(runId) !== undefined;
    }
    recoverDeadOwners() {
        const instances = this.db.prepare('SELECT id, pid, heartbeat_at, slot_limit FROM instances').all();
        const alive = new Map(instances.map(instance => [instance.id, processAlive(instance.pid)]));
        const dead = new Set(instances.filter(instance => !alive.get(instance.id)).map(instance => instance.id));
        const rows = this.db.prepare("SELECT document, owner_instance, owner_pid, cancel_requested FROM runs WHERE owner_instance IS NOT NULL AND json_extract(document, '$.status') IN ('queued', 'running')").all();
        const recovered = [];
        for (const row of rows) {
            const run = JSON.parse(row.document);
            if (!['queued', 'running'].includes(run.status) || !row.owner_instance || row.owner_pid === null)
                continue;
            const ownerAlive = alive.get(row.owner_instance) ?? processAlive(row.owner_pid);
            if (ownerAlive)
                continue;
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
    close() { this.db.close(); }
}
//# sourceMappingURL=store.js.map