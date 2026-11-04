import { randomUUID } from 'node:crypto';
import { chmodSync, mkdirSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { DatabaseSync } from 'node:sqlite';
import { parseSchema } from './schema.js';
import { delegationDefinition, delegationReportDefinition, delegationResolveDefinition, delegationWatchDefinition } from './delegation-schema.js';
export class DelegationRevisionConflict extends Error {
    currentRevision;
    code = 'COUNCIL_DELEGATION_REVISION_CONFLICT';
    status = 409;
    constructor(currentRevision) {
        super(`Delegation revision conflict: current revision is ${currentRevision}. Reload before saving.`);
        this.currentRevision = currentRevision;
        this.name = 'DelegationRevisionConflict';
    }
}
function nonempty(value, label) {
    if (!value.trim())
        throw new Error(`${label} must contain non-whitespace text.`);
}
function contention(error) {
    const code = error && typeof error === 'object' && 'errcode' in error ? error.errcode : undefined;
    return typeof code === 'number' && ((code & 0xff) === 5 || (code & 0xff) === 6);
}
function increment(value) {
    if (!Number.isSafeInteger(value) || value < 0 || value >= Number.MAX_SAFE_INTEGER)
        throw new Error('Delegation revision space is exhausted.');
    return value + 1;
}
/** Durable executive coordination. No providers, credentials, task permissions or worker prompts are changed. */
export class DelegationManager {
    service;
    db;
    shutdown = new AbortController();
    listeners = new Set();
    roomRecoveryAt = new Map();
    closed = false;
    constructor(service) {
        this.service = service;
        const sqlite = /^(\d+)\.(\d+)\.(\d+)/.exec(process.versions.sqlite ?? '');
        if (!sqlite || !(Number(sqlite[1]) > 3 || Number(sqlite[1]) === 3 && (Number(sqlite[2]) > 51 || Number(sqlite[2]) === 51 && Number(sqlite[3]) >= 3)))
            throw new Error('Delegation state requires SQLite 3.51.3 or newer.');
        mkdirSync(service.config.stateDir, { recursive: true, mode: 0o700 });
        const filename = join(resolve(service.config.stateDir), 'delegation.sqlite');
        this.db = new DatabaseSync(filename);
        try {
            this.db.exec('PRAGMA busy_timeout=250;');
            const version = Number(this.db.prepare('PRAGMA user_version').get().user_version);
            if (version > 2)
                throw new Error('Delegation state was created by a newer Council version.');
            if (process.platform !== 'win32')
                chmodSync(filename, 0o600);
            const journal = this.db.prepare('PRAGMA journal_mode').get();
            if (version < 2 || journal.journal_mode !== 'wal') {
                const deadline = performance.now() + 5000, wait = new Int32Array(new SharedArrayBuffer(4));
                for (;;) {
                    try {
                        this.db.exec(`PRAGMA busy_timeout=${Math.max(1, Math.ceil(deadline - performance.now()))}; PRAGMA journal_mode=WAL;`);
                        this.transaction(() => {
                            const current = Number(this.db.prepare('PRAGMA user_version').get().user_version);
                            if (current > 2)
                                throw new Error('Delegation state was created by a newer Council version.');
                            if (current === 2)
                                return;
                            if (current === 1) {
                                this.db.exec('ALTER TABLE delegation_reports ADD COLUMN change_revision INTEGER NOT NULL DEFAULT 0;');
                                let reportRevision = Number(this.db.prepare('SELECT report_revision FROM delegation_settings WHERE id=1').get().report_revision);
                                const rows = this.db.prepare('SELECT sequence FROM delegation_reports ORDER BY sequence').all();
                                for (const row of rows) {
                                    reportRevision = increment(reportRevision);
                                    this.db.prepare('UPDATE delegation_reports SET change_revision=? WHERE sequence=?').run(reportRevision, row.sequence);
                                }
                                this.db.prepare('UPDATE delegation_settings SET report_revision=? WHERE id=1').run(reportRevision);
                                this.db.exec('CREATE UNIQUE INDEX delegation_report_changes ON delegation_reports(change_revision); PRAGMA user_version=2;');
                                return;
                            }
                            this.db.exec(`CREATE TABLE delegation_settings(id INTEGER PRIMARY KEY CHECK(id=1), document TEXT NOT NULL, report_revision INTEGER NOT NULL, workspace_id TEXT NOT NULL);
                CREATE TABLE delegation_reports(sequence INTEGER PRIMARY KEY AUTOINCREMENT, id TEXT NOT NULL UNIQUE, kind TEXT NOT NULL,
                  resolved INTEGER NOT NULL DEFAULT 0, document TEXT NOT NULL, change_revision INTEGER NOT NULL);
                CREATE INDEX delegation_report_open ON delegation_reports(resolved,kind,sequence);
                CREATE UNIQUE INDEX delegation_report_changes ON delegation_reports(change_revision); PRAGMA user_version=2;`);
                            const initial = { mode: 'human', delegate: '', mandate: '', roomIds: [], revision: 0, updatedAt: new Date().toISOString() };
                            this.db.prepare('INSERT INTO delegation_settings(id,document,report_revision,workspace_id) VALUES(1,?,0,?)').run(JSON.stringify(initial), randomUUID());
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
            this.db.close();
            this.closed = true;
            throw error;
        }
    }
    open() { if (this.closed)
        throw new Error('Delegation manager is closed.'); }
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
    notify() { for (const listener of [...this.listeners]) {
        try {
            listener();
        }
        catch { }
    } }
    state() {
        this.open();
        const row = this.db.prepare('SELECT document,report_revision,workspace_id FROM delegation_settings WHERE id=1').get();
        return { configuration: JSON.parse(row.document), reportRevision: row.report_revision, workspaceId: row.workspace_id };
    }
    rooms(ids) {
        if (new Set(ids).size !== ids.length)
            throw new Error('Delegation room IDs must be unique.');
        return ids.map(id => this.service.rooms.metadata(id));
    }
    bumpReports() {
        const next = increment(this.state().reportRevision);
        this.db.prepare('UPDATE delegation_settings SET report_revision=? WHERE id=1').run(next);
        return next;
    }
    get() { return this.state().configuration; }
    overview() {
        this.open();
        return this.transaction(() => {
            const { configuration, reportRevision } = this.state();
            const openQuestions = Number(this.db.prepare("SELECT COUNT(*) AS count FROM delegation_reports WHERE resolved=0 AND kind IN ('decision','blocked')").get().count);
            return { mode: configuration.mode, delegate: configuration.delegate, configurationRevision: configuration.revision, reportRevision, openQuestions };
        }, false);
    }
    configure(value) {
        this.open();
        const input = parseSchema(delegationDefinition, value, 'delegation');
        if (input.mode === 'agent') {
            nonempty(input.delegate, 'Delegate');
            nonempty(input.mandate, 'Delegate mandate');
        }
        this.rooms(input.roomIds);
        const saved = this.transaction(() => {
            const current = this.get();
            if (current.revision !== input.expectedRevision)
                throw new DelegationRevisionConflict(current.revision);
            const { expectedRevision: _, ...settings } = input;
            const result = { ...settings, revision: increment(current.revision), updatedAt: new Date().toISOString() };
            this.db.prepare('UPDATE delegation_settings SET document=? WHERE id=1').run(JSON.stringify(result));
            return result;
        });
        const tracked = new Set(saved.roomIds);
        for (const id of this.roomRecoveryAt.keys())
            if (!tracked.has(id))
                this.roomRecoveryAt.delete(id);
        this.notify();
        return saved;
    }
    report(value) {
        this.open();
        const input = parseSchema(delegationReportDefinition, value, 'delegate report');
        nonempty(input.author, 'Report author');
        nonempty(input.body, 'Report body');
        const observedRooms = this.rooms(input.roomIds).map(room => ({ id: room.id, revision: room.revision, instructionRevision: room.instructionRevision ?? 0, status: room.status }));
        if (input.expectedRoomRevisions) {
            if (Object.keys(input.expectedRoomRevisions).some(id => !input.roomIds.includes(id)))
                throw new Error('Reviewed room revisions must refer to this report\'s room IDs.');
            for (const room of observedRooms) {
                if (!Object.hasOwn(input.expectedRoomRevisions, room.id))
                    throw new Error('Supply a reviewed revision for every report room.');
                if (input.expectedRoomRevisions[room.id] !== room.revision)
                    throw new Error(`Room ${room.id} changed after review. Read its latest state before reporting.`);
            }
        }
        const saved = this.transaction(() => {
            const { expectedRoomRevisions: _, ...details } = input;
            const report = { ...details, id: randomUUID(), sequence: 0, revision: 1, observedRooms, createdAt: new Date().toISOString() };
            const changeRevision = this.bumpReports();
            const row = this.db.prepare('INSERT INTO delegation_reports(id,kind,document,change_revision) VALUES(?,?,?,?)').run(report.id, report.kind, JSON.stringify(report), changeRevision);
            report.sequence = Number(row.lastInsertRowid);
            if (!Number.isSafeInteger(report.sequence))
                throw new Error('Delegate report sequence space is exhausted.');
            this.db.prepare('UPDATE delegation_reports SET document=? WHERE id=?').run(JSON.stringify(report), report.id);
            return report;
        });
        this.notify();
        return saved;
    }
    resolve(id, value) {
        this.open();
        if (typeof id !== 'string' || !id || id.length > 100)
            throw new Error('Delegate report not found (404).');
        const input = parseSchema(delegationResolveDefinition, value, 'delegate response');
        nonempty(input.author, 'Response author');
        nonempty(input.body, 'Response body');
        const saved = this.transaction(() => {
            const row = this.db.prepare('SELECT document FROM delegation_reports WHERE id=?').get(id);
            if (!row)
                throw new Error('Delegate report not found (404).');
            const report = JSON.parse(row.document);
            if (report.revision !== input.expectedRevision)
                throw new DelegationRevisionConflict(report.revision);
            if (report.resolution)
                throw new Error('This delegate report is already resolved.');
            if (!['decision', 'blocked'].includes(report.kind))
                throw new Error('Only a decision request or blocker can be resolved.');
            report.revision = increment(report.revision);
            report.resolution = { author: input.author, body: input.body, createdAt: new Date().toISOString() };
            const changeRevision = this.bumpReports();
            this.db.prepare('UPDATE delegation_reports SET resolved=1,document=?,change_revision=? WHERE id=?').run(JSON.stringify(report), changeRevision, id);
            return report;
        });
        this.notify();
        return saved;
    }
    reportsPage(options = {}) {
        this.open();
        const { limit = 100, cursor = 0, openOnly = false } = options;
        if (!Number.isSafeInteger(limit) || limit < 1 || limit > 100 || !Number.isSafeInteger(cursor) || cursor < 0 || typeof openOnly !== 'boolean')
            throw new Error('Invalid delegate report page bounds.');
        const rows = this.db.prepare(`SELECT sequence,document FROM delegation_reports WHERE sequence>? ${openOnly ? "AND resolved=0 AND kind IN ('decision','blocked')" : ''} ORDER BY sequence LIMIT ?`)
            .all(cursor, limit + 1);
        return { reports: rows.slice(0, limit).map(row => JSON.parse(row.document)), nextCursor: rows.length > limit ? rows[limit - 1].sequence : null };
    }
    reportChangesPage(options = {}) {
        this.open();
        const { after = 0, limit = 100 } = options;
        if (!Number.isSafeInteger(limit) || limit < 1 || limit > 100 || !Number.isSafeInteger(after) || after < 0)
            throw new Error('Invalid delegate change page bounds.');
        const rows = this.db.prepare('SELECT change_revision,document FROM delegation_reports WHERE change_revision>? ORDER BY change_revision LIMIT ?')
            .all(after, limit + 1);
        return { changes: rows.slice(0, limit).map(row => ({ revision: row.change_revision, report: JSON.parse(row.document) })),
            nextCursor: rows.length > limit ? rows[limit - 1].change_revision : null };
    }
    snapshot() {
        this.open();
        const { configuration, reportRevision, workspaceId, openQuestions } = this.transaction(() => ({ ...this.state(),
            openQuestions: Number(this.db.prepare("SELECT COUNT(*) AS count FROM delegation_reports WHERE resolved=0 AND kind IN ('decision','blocked')").get().count) }), false);
        const tracked = new Set(configuration.roomIds);
        for (const id of this.roomRecoveryAt.keys())
            if (!tracked.has(id))
                this.roomRecoveryAt.delete(id);
        const rooms = configuration.roomIds.map(id => {
            try {
                const checkedAt = this.roomRecoveryAt.get(id), now = performance.now();
                if (checkedAt === undefined || now - checkedAt >= 1000) {
                    this.service.rooms.recover(id);
                    this.roomRecoveryAt.set(id, now);
                }
                const room = this.service.rooms.metadata(id);
                return { id: room.id, title: room.title, revision: room.revision, instructionRevision: room.instructionRevision ?? 0, status: room.status,
                    taskCount: room.tasks.length, completedTasks: room.tasks.filter(task => task.status === 'completed').length,
                    failedTasks: room.tasks.filter(task => task.status === 'failed' || task.status === 'cancelled').length,
                    callsUsed: room.callsUsed, maxCalls: room.maxCalls, ...(room.error ? { error: room.error } : {}),
                    ...(room.pauseRequested ? { pauseRequested: true } : {}), actionable: room.status !== 'discussing' && room.status !== 'running' };
            }
            catch (error) {
                if (!(error instanceof Error) || !/not found \(404\)/i.test(error.message))
                    throw error;
                return { id, revision: 0, instructionRevision: 0, missing: true, error: error.message, actionable: true };
            }
        });
        return { configuration, rooms, openQuestions, cursor: { workspaceId, configurationRevision: configuration.revision, reportRevision,
                rooms: Object.fromEntries(rooms.map(room => [room.id, { revision: room.revision, instructionRevision: room.instructionRevision }])) } };
    }
    watch(value = {}, signal) {
        this.open();
        const input = parseSchema(delegationWatchDefinition, value, 'delegate watch');
        const cancellation = signal ? AbortSignal.any([signal, this.shutdown.signal]) : this.shutdown.signal;
        cancellation.throwIfAborted();
        const started = performance.now(), timeoutMs = input.timeoutMs;
        return new Promise((resolveValue, reject) => {
            let settled = false, running = false, rerun = false;
            let timer, unsubscribe;
            const cleanup = () => {
                if (timer)
                    clearTimeout(timer);
                timer = undefined;
                this.listeners.delete(check);
                unsubscribe?.();
                unsubscribe = undefined;
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
                        const snapshot = this.snapshot(), previous = input.cursor;
                        if (previous && previous.workspaceId !== snapshot.cursor.workspaceId)
                            throw new Error('Delegate watch cursor belongs to another workspace or an earlier delegation store. Read a new snapshot without a cursor.');
                        const changed = !previous || snapshot.cursor.configurationRevision !== previous.configurationRevision || snapshot.cursor.reportRevision !== previous.reportRevision
                            || snapshot.rooms.some(room => {
                                const before = previous.rooms[room.id];
                                return !before || before.instructionRevision !== room.instructionRevision || room.actionable && before.revision !== room.revision;
                            });
                        const remaining = timeoutMs - (performance.now() - started);
                        if (changed || remaining <= 0) {
                            settled = true;
                            cleanup();
                            resolveValue({ reason: changed ? 'changed' : 'timeout', ...snapshot });
                            return;
                        }
                    } while (rerun);
                    if (timer)
                        clearTimeout(timer);
                    // Cross-process SQLite updates do not emit this process's room events.
                    // The server waits here; no orchestrator or provider is polled.
                    timer = setTimeout(check, Math.max(1, Math.min(100, timeoutMs - (performance.now() - started))));
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
                this.listeners.add(check);
                unsubscribe = this.service.rooms.subscribe(check);
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
        this.shutdown.abort(new DOMException('Delegation manager is shutting down.', 'AbortError'));
        this.listeners.clear();
        this.roomRecoveryAt.clear();
        this.db.close();
    }
}
//# sourceMappingURL=delegation.js.map