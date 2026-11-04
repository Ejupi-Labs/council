import { chmodSync, mkdirSync, realpathSync, existsSync } from 'node:fs';
import { dirname, isAbsolute, join, relative, resolve, sep } from 'node:path';
import { DatabaseSync } from 'node:sqlite';
import { parseSchema } from './schema.js';
import { billingDefinition } from './billing-schema.js';
const ACCOUNT_FIELDS = ['mode', 'plan', 'source', 'observedAt', 'validUntil', 'windows', 'prices'];
const emptyAccount = () => ({ mode: 'unknown', plan: null, source: null, observedAt: null, validUntil: null, windows: [], prices: null });
const json = (value) => JSON.stringify(value);
export class BillingRevisionConflict extends Error {
    currentRevision;
    code = 'COUNCIL_BILLING_REVISION_CONFLICT';
    status = 409;
    constructor(currentRevision) {
        super(`Billing revision conflict: current revision is ${currentRevision}. Reload before saving.`);
        this.currentRevision = currentRevision;
        this.name = 'BillingRevisionConflict';
    }
}
function nonempty(value, label) {
    if (!value.trim() || value.includes('\0'))
        throw new Error(`${label} must contain non-whitespace text.`);
    return value;
}
function timestamp(value, label) {
    const match = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.\d{1,3})?(?:Z|[+-]\d{2}:\d{2})$/.exec(value);
    const date = new Date(value);
    if (!match || !Number.isFinite(date.getTime()))
        throw new Error(`${label} must be a valid ISO timestamp with a timezone.`);
    const year = Number(match[1]), month = Number(match[2]), day = Number(match[3]);
    const leap = year % 4 === 0 && (year % 100 !== 0 || year % 400 === 0);
    const days = [31, leap ? 29 : 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31];
    if (month < 1 || month > 12 || day < 1 || day > days[month - 1] || Number(match[4]) > 23 || Number(match[5]) > 59 || Number(match[6]) > 59)
        throw new Error(`${label} must be a valid ISO timestamp with a timezone.`);
    return date.toISOString();
}
function nowValue(now) {
    const value = now instanceof Date ? now.getTime() : typeof now === 'number' ? now : new Date(timestamp(now, 'Evaluation time')).getTime();
    if (!Number.isFinite(value) || !Number.isFinite(new Date(value).getTime()))
        throw new Error('Evaluation time must be a valid date.');
    return value;
}
function sharedKey(account) { return account.startsWith('shared:') ? account.slice(7) : null; }
function contention(error) {
    const code = error && typeof error === 'object' && 'errcode' in error ? error.errcode : undefined;
    return typeof code === 'number' && ((code & 0xff) === 5 || (code & 0xff) === 6);
}
/** User-supplied billing observations only: no credential access, vendor probes, or estimated spending. */
export class BillingManager {
    config;
    db;
    closed = false;
    constructor(config, stateDir = config.stateDir) {
        this.config = config;
        const sqlite = /^(\d+)\.(\d+)\.(\d+)/.exec(process.versions.sqlite ?? '');
        if (!sqlite || !(Number(sqlite[1]) > 3 || Number(sqlite[1]) === 3 && (Number(sqlite[2]) > 51 || Number(sqlite[2]) === 51 && Number(sqlite[3]) >= 3)))
            throw new Error('Billing state requires SQLite 3.51.3 or newer.');
        const root = realpathSync(config.workspaceRoot), state = resolve(root, stateDir);
        const inside = (path) => { const suffix = relative(root, path); return Boolean(suffix) && !isAbsolute(suffix) && suffix !== '..' && !suffix.startsWith(`..${sep}`); };
        if (!inside(state))
            throw new Error('Billing state must stay inside the configured workspace state directory.');
        let ancestor = state;
        while (!existsSync(ancestor))
            ancestor = dirname(ancestor);
        if (relative(root, realpathSync(ancestor)) && !inside(realpathSync(ancestor)))
            throw new Error('Billing state resolves outside the configured workspace.');
        mkdirSync(state, { recursive: true, mode: 0o700 });
        const filename = join(state, 'billing.sqlite');
        this.db = new DatabaseSync(filename);
        try {
            this.db.exec('PRAGMA busy_timeout=250; PRAGMA foreign_keys=ON;');
            const version = Number(this.db.prepare('PRAGMA user_version').get().user_version);
            if (version > 1)
                throw new Error('Billing state was created by a newer Council version.');
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
                                throw new Error('Billing state was created by a newer Council version.');
                            if (current === 1)
                                return;
                            this.db.exec(`CREATE TABLE billing_accounts(account_id TEXT PRIMARY KEY, document TEXT NOT NULL);
                CREATE TABLE billing_aliases(alias TEXT PRIMARY KEY, revision INTEGER NOT NULL, account_id TEXT REFERENCES billing_accounts(account_id), routing TEXT, skills TEXT);
                CREATE INDEX billing_account_aliases ON billing_aliases(account_id); PRAGMA user_version=1;`);
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
        throw new Error('Billing manager is closed.'); }
    known(alias) {
        this.open();
        if (typeof alias !== 'string' || !Object.hasOwn(this.config.agents, alias))
            throw new Error(`Unknown agent alias: ${String(alias)}`);
    }
    transaction(action, write = true) {
        this.db.exec(write ? 'BEGIN IMMEDIATE' : 'BEGIN');
        try {
            const value = action();
            this.db.exec('COMMIT');
            return value;
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
    row(alias) {
        return this.db.prepare('SELECT alias,revision,account_id,routing,skills FROM billing_aliases WHERE alias=?').get(alias);
    }
    account(id) {
        const row = this.db.prepare('SELECT document FROM billing_accounts WHERE account_id=?').get(id);
        return row ? JSON.parse(row.document) : emptyAccount();
    }
    revision(expected, actual) {
        if (expected !== undefined && (!Number.isSafeInteger(expected) || expected < 0))
            throw new Error('Expected billing revision must be a nonnegative safe integer.');
        if (expected !== undefined && expected !== actual)
            throw new BillingRevisionConflict(actual);
    }
    get(alias) {
        this.known(alias);
        if (!this.db.isTransaction)
            return this.transaction(() => this.get(alias), false);
        const row = this.row(alias), id = row?.account_id ?? `agent:${alias}`, account = this.account(id), settings = this.config.agents[alias];
        const record = { alias, enabled: settings.enabled, profile: settings.profile, model: settings.model ?? null,
            revision: row?.revision ?? 0, accountKey: sharedKey(id), ...account, routing: row?.routing ? JSON.parse(row.routing) : null,
            skills: row?.skills ? JSON.parse(row.skills) : null, stale: false };
        const now = Date.now();
        record.stale = this.evaluate(record, now).windows.some(window => window.state === 'unknown')
            || !record.observedAt || new Date(record.observedAt).getTime() > now
            || Boolean(record.validUntil && new Date(record.validUntil).getTime() <= now);
        return structuredClone(record);
    }
    list() {
        this.open();
        return this.transaction(() => Object.keys(this.config.agents).map(alias => this.get(alias)), false);
    }
    update(alias, input, expectedRevision) { return this.write(alias, input, expectedRevision, false); }
    importSnapshot(alias, input, expectedRevision) { return this.write(alias, input, expectedRevision, true); }
    write(alias, value, expectedRevision, reported) {
        this.known(alias);
        const input = parseSchema(billingDefinition, value, reported ? 'reported billing snapshot' : 'billing update');
        if (reported && (input.source?.kind !== 'reported' || !input.observedAt))
            throw new Error('A reported snapshot requires an explicit reported source and observation timestamp.');
        if (!reported && input.source?.kind === 'reported')
            throw new Error('Use importSnapshot to import an explicitly reported observation.');
        if (input.accountKey !== undefined && input.accountKey !== null)
            input.accountKey = nonempty(input.accountKey, 'Account key').trim();
        if (input.plan !== undefined && input.plan !== null)
            nonempty(input.plan, 'Plan label');
        if (input.source)
            nonempty(input.source.label, 'Source label');
        if (input.observedAt)
            input.observedAt = timestamp(input.observedAt, 'Observation time');
        if (input.validUntil)
            input.validUntil = timestamp(input.validUntil, 'Observation validity');
        if (input.windows) {
            if (input.windows.length && !input.observedAt)
                throw new Error('Limit windows require an explicit observation timestamp; observations are never refreshed automatically.');
            if (new Set(input.windows.map(window => window.id)).size !== input.windows.length)
                throw new Error('Limit window IDs must be unique.');
            input.windows = input.windows.map(window => {
                nonempty(window.id, 'Window ID');
                nonempty(window.label, 'Window label');
                nonempty(window.unit, 'Window unit');
                return { ...window, limit: window.limit ?? null, remaining: window.remaining ?? null, used: window.used ?? null,
                    resetAt: window.resetAt ? timestamp(window.resetAt, 'Window reset') : null };
            });
        }
        if (input.prices) {
            if (input.prices.currency !== undefined && input.prices.currency !== null)
                nonempty(input.prices.currency, 'Currency');
        }
        if (input.routing) {
            const capabilities = input.routing.capabilities ?? [];
            for (const capability of capabilities)
                nonempty(capability, 'Capability');
            if (new Set(capabilities).size !== capabilities.length)
                throw new Error('Routing capabilities must be unique.');
            input.routing = { ...input.routing, capabilities, costRank: input.routing.costRank ?? null, qualityRank: input.routing.qualityRank ?? null };
        }
        if (input.skills && new Set(input.skills).size !== input.skills.length)
            throw new Error('Billing skill overrides must be unique.');
        const fields = ACCOUNT_FIELDS.filter(field => Object.hasOwn(input, field));
        return this.transaction(() => {
            const previous = this.row(alias);
            this.revision(expectedRevision, previous?.revision ?? 0);
            if (!Object.keys(input).length)
                return this.get(alias);
            const id = input.accountKey !== undefined ? input.accountKey === null ? `agent:${alias}` : `shared:${input.accountKey}` : previous?.account_id ?? `agent:${alias}`;
            const account = this.account(id);
            for (const field of fields) {
                if (field === 'prices' && input.prices)
                    account.prices = { currency: null, inputPerMillion: null, outputPerMillion: null, ...account.prices, ...input.prices };
                else
                    account[field] = input[field];
            }
            if (fields.length && !Object.hasOwn(input, 'source'))
                account.source = { kind: reported ? 'reported' : 'manual', label: reported ? 'Reported snapshot' : 'Manual entry' };
            this.db.prepare('INSERT INTO billing_accounts(account_id,document) VALUES(?,?) ON CONFLICT(account_id) DO UPDATE SET document=excluded.document').run(id, json(account));
            if (fields.length)
                this.db.prepare('UPDATE billing_aliases SET revision=revision+1 WHERE account_id=?').run(id);
            const existing = this.row(alias);
            const revision = existing && existing.account_id === id && fields.length ? existing.revision : (existing?.revision ?? 0) + 1;
            const routing = Object.hasOwn(input, 'routing') ? input.routing === null ? null : json(input.routing) : existing?.routing ?? null;
            const skills = Object.hasOwn(input, 'skills') ? input.skills === null ? null : json(input.skills) : existing?.skills ?? null;
            this.db.prepare('INSERT INTO billing_aliases(alias,revision,account_id,routing,skills) VALUES(?,?,?,?,?) ON CONFLICT(alias) DO UPDATE SET revision=excluded.revision,account_id=excluded.account_id,routing=excluded.routing,skills=excluded.skills')
                .run(alias, revision, id, routing, skills);
            if (previous?.account_id && previous.account_id !== id)
                this.db.prepare('DELETE FROM billing_accounts WHERE account_id=? AND NOT EXISTS(SELECT 1 FROM billing_aliases WHERE account_id=?)').run(previous.account_id, previous.account_id);
            return this.get(alias);
        });
    }
    remove(alias, expectedRevision) {
        this.known(alias);
        return this.transaction(() => {
            const row = this.row(alias);
            this.revision(expectedRevision, row?.revision ?? 0);
            this.db.prepare('INSERT INTO billing_aliases(alias,revision,account_id,routing,skills) VALUES(?,?,NULL,NULL,NULL) ON CONFLICT(alias) DO UPDATE SET revision=excluded.revision,account_id=NULL,routing=NULL,skills=NULL').run(alias, (row?.revision ?? 0) + 1);
            // A detached alias must not accidentally rediscover its previous private quota.
            for (const account of new Set([`agent:${alias}`, ...(row?.account_id ? [row.account_id] : [])])) {
                this.db.prepare('DELETE FROM billing_accounts WHERE account_id=? AND NOT EXISTS(SELECT 1 FROM billing_aliases WHERE account_id=?)').run(account, account);
            }
            return this.get(alias);
        });
    }
    availability(alias, now = Date.now()) { return this.evaluate(this.get(alias), nowValue(now)); }
    evaluate(record, now) {
        const global = [];
        if (!record.observedAt)
            global.push('No quota observation has been recorded.');
        else if (new Date(record.observedAt).getTime() > now)
            global.push('The observation timestamp is in the future.');
        if (!record.source)
            global.push('The observation source is unknown.');
        if (record.validUntil && new Date(record.validUntil).getTime() <= now)
            global.push('The observation validity has expired; capacity is unknown.');
        const windows = record.windows.map(window => {
            const reasons = [...global];
            if (window.resetAt && new Date(window.resetAt).getTime() <= now)
                reasons.push('The reported reset has passed; replenishment has not been observed.');
            if (!record.validUntil && !window.resetAt)
                reasons.push('No observation validity bound was provided; set validUntil or resetAt.');
            let remaining = window.remaining;
            if (remaining === null && window.limit !== null && window.used !== null)
                remaining = Math.max(0, window.limit - window.used);
            if (remaining === null)
                reasons.push('Remaining capacity is unknown.');
            let state = 'unknown';
            if (!reasons.length)
                state = remaining <= 0 ? 'exhausted' : 'available';
            return { ...window, state, effectiveRemaining: reasons.length ? null : remaining, reasons };
        });
        const reasons = [...global];
        if (!windows.length)
            reasons.push('No observed limit windows are configured; no unlimited allowance is inferred.');
        for (const window of windows)
            for (const reason of window.reasons)
                if (!reasons.includes(reason))
                    reasons.push(reason);
        const status = windows.some(window => window.state === 'exhausted') ? 'exhausted'
            : windows.length && windows.every(window => window.state === 'available') ? 'available' : 'unknown';
        return { status, alias: record.alias, accountKey: record.accountKey, observedAt: record.observedAt, evaluatedAt: new Date(now).toISOString(), reasons, windows };
    }
    close() {
        if (this.closed)
            return;
        this.db.close();
        this.closed = true;
    }
}
//# sourceMappingURL=billing.js.map