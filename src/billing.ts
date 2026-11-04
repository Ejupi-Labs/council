import { chmodSync, mkdirSync, realpathSync, existsSync } from 'node:fs';
import { dirname, isAbsolute, join, relative, resolve, sep } from 'node:path';
import { DatabaseSync } from 'node:sqlite';
import { parseSchema } from './schema.js';
import { requireSafeSqliteRuntime } from './sqlite-runtime.js';
import { billingDefinition } from './billing-schema.js';
import type { CouncilConfig } from './types.js';
import type { BillingAvailability, BillingAvailabilityStatus, BillingAvailabilityWindow, BillingInput, BillingPrices, BillingRecord, BillingRouting, BillingSource, BillingWindow, SubscriptionProtection } from './billing-types.js';

type Account = Pick<BillingRecord, 'mode' | 'plan' | 'source' | 'observedAt' | 'validUntil' | 'windows'> & { subscriptionExhausted?: Record<string, string> };
type AliasRow = { alias: string; revision: number; account_id: string | null; routing: string | null; skills: string | null; prices: string | null; stop_at_subscription_limit: number; subscription_window_ids: string | null };
const ACCOUNT_FIELDS = ['mode', 'plan', 'source', 'observedAt', 'validUntil', 'windows'] as const;
const emptyAccount = (): Account => ({ mode: 'unknown', plan: null, source: null, observedAt: null, validUntil: null, windows: [] });
const json = (value: unknown): string => JSON.stringify(value);

export class BillingRevisionConflict extends Error {
  readonly code = 'COUNCIL_BILLING_REVISION_CONFLICT';
  readonly status = 409;
  constructor(readonly currentRevision: number) { super(`Billing revision conflict: current revision is ${currentRevision}. Reload before saving.`); this.name = 'BillingRevisionConflict'; }
}
export class SubscriptionLimitError extends Error {
  readonly code = 'COUNCIL_SUBSCRIPTION_LIMIT';
  readonly status = 409;
  constructor(readonly receipt: SubscriptionProtection) {
    super(`[COUNCIL_SUBSCRIPTION_LIMIT] ${receipt.alias}: ${receipt.reasons.join(' ')}`);
    this.name = 'SubscriptionLimitError';
  }
}
function remaining(window: BillingWindow): number | null {
  return window.remaining ?? (window.limit !== null && window.used !== null ? Math.max(0, window.limit - window.used) : null);
}
function scope(window: BillingWindow, mode: Account['mode']): BillingWindow['scope'] { return window.scope ?? (mode === 'subscription' ? 'subscription' : 'other'); }

function nonempty(value: string, label: string): string {
  if (!value.trim() || value.includes('\0')) throw new Error(`${label} must contain non-whitespace text.`);
  return value;
}
function timestamp(value: string, label: string): string {
  const match = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.\d{1,3})?(?:Z|[+-]\d{2}:\d{2})$/.exec(value);
  const date = new Date(value);
  if (!match || !Number.isFinite(date.getTime())) throw new Error(`${label} must be a valid ISO timestamp with a timezone.`);
  const year = Number(match[1]), month = Number(match[2]), day = Number(match[3]);
  const leap = year % 4 === 0 && (year % 100 !== 0 || year % 400 === 0);
  const days = [31, leap ? 29 : 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31];
  if (month < 1 || month > 12 || day < 1 || day > days[month - 1]! || Number(match[4]) > 23 || Number(match[5]) > 59 || Number(match[6]) > 59) throw new Error(`${label} must be a valid ISO timestamp with a timezone.`);
  return date.toISOString();
}
function nowValue(now: Date | number | string): number {
  const value = now instanceof Date ? now.getTime() : typeof now === 'number' ? now : new Date(timestamp(now, 'Evaluation time')).getTime();
  if (!Number.isFinite(value) || !Number.isFinite(new Date(value).getTime())) throw new Error('Evaluation time must be a valid date.');
  return value;
}
function sharedKey(account: string): string | null { return account.startsWith('shared:') ? account.slice(7) : null; }
function contention(error: unknown): boolean {
  const code = error && typeof error === 'object' && 'errcode' in error ? error.errcode : undefined;
  return typeof code === 'number' && ((code & 0xff) === 5 || (code & 0xff) === 6);
}

/** User-supplied billing observations only: no credential access, vendor probes, or estimated spending. */
export class BillingManager {
  private readonly db: DatabaseSync;
  private closed = false;

  constructor(private readonly config: CouncilConfig, stateDir = config.stateDir) {
    requireSafeSqliteRuntime();
    const root = realpathSync(config.workspaceRoot), state = resolve(root, stateDir);
    const inside = (path: string): boolean => { const suffix = relative(root, path); return Boolean(suffix) && !isAbsolute(suffix) && suffix !== '..' && !suffix.startsWith(`..${sep}`); };
    if (!inside(state)) throw new Error('Billing state must stay inside the configured workspace state directory.');
    let ancestor = state; while (!existsSync(ancestor)) ancestor = dirname(ancestor);
    if (relative(root, realpathSync(ancestor)) && !inside(realpathSync(ancestor))) throw new Error('Billing state resolves outside the configured workspace.');
    mkdirSync(state, { recursive: true, mode: 0o700 });
    const filename = join(state, 'billing.sqlite'); this.db = new DatabaseSync(filename);
    try {
      this.db.exec('PRAGMA busy_timeout=250; PRAGMA foreign_keys=ON;');
      const version = Number(this.db.prepare('PRAGMA user_version').get()!.user_version);
      if (version > 3) throw new Error('Billing state was created by a newer Council version.');
      if (process.platform !== 'win32') chmodSync(filename, 0o600);
      const journal = this.db.prepare('PRAGMA journal_mode').get() as { journal_mode: string };
      if (version < 3 || journal.journal_mode !== 'wal') {
        const deadline = performance.now() + 5000, wait = new Int32Array(new SharedArrayBuffer(4));
        for (;;) {
          try {
            this.db.exec(`PRAGMA busy_timeout=${Math.max(1, Math.ceil(deadline - performance.now()))}; PRAGMA journal_mode=WAL;`);
            this.transaction(() => {
              const current = Number(this.db.prepare('PRAGMA user_version').get()!.user_version);
              if (current > 3) throw new Error('Billing state was created by a newer Council version.');
              if (current === 3) return;
              if (current === 0) {
                this.db.exec(`CREATE TABLE billing_accounts(account_id TEXT PRIMARY KEY, document TEXT NOT NULL);
                  CREATE TABLE billing_aliases(alias TEXT PRIMARY KEY, revision INTEGER NOT NULL, account_id TEXT REFERENCES billing_accounts(account_id), routing TEXT, skills TEXT, prices TEXT,
                    stop_at_subscription_limit INTEGER NOT NULL DEFAULT 0, subscription_window_ids TEXT);
                  CREATE INDEX billing_account_aliases ON billing_aliases(account_id); PRAGMA user_version=3;`);
                return;
              }
              // Version 1 stored model rates alongside shared account quotas.
              // Preserve each linked alias's previous rates before separating them.
              if (current === 1) {
                this.db.exec('ALTER TABLE billing_aliases ADD COLUMN prices TEXT;');
                const accounts = this.db.prepare('SELECT account_id,document FROM billing_accounts').all() as Array<{ account_id: string; document: string }>;
                for (const row of accounts) {
                  const account = JSON.parse(row.document) as Account & { prices?: BillingPrices | null };
                  this.db.prepare('UPDATE billing_aliases SET prices=? WHERE account_id=?').run(account.prices ? json(account.prices) : null, row.account_id);
                  delete account.prices;
                  this.db.prepare('UPDATE billing_accounts SET document=? WHERE account_id=?').run(json(account), row.account_id);
                }
              }
              this.db.exec('ALTER TABLE billing_aliases ADD COLUMN stop_at_subscription_limit INTEGER NOT NULL DEFAULT 0; ALTER TABLE billing_aliases ADD COLUMN subscription_window_ids TEXT; PRAGMA user_version=3;');
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
    } catch (error) { this.db.close(); this.closed = true; throw error; }
  }

  private open(): void { if (this.closed) throw new Error('Billing manager is closed.'); }
  private known(alias: string): void {
    this.open();
    if (typeof alias !== 'string' || !Object.hasOwn(this.config.agents, alias)) throw new Error(`Unknown agent alias: ${String(alias)}`);
  }
  private transaction<T>(action: () => T, write = true): T {
    this.db.exec(write ? 'BEGIN IMMEDIATE' : 'BEGIN');
    try { const value = action(); this.db.exec('COMMIT'); return value; }
    catch (error) { try { if (this.db.isTransaction) this.db.exec('ROLLBACK'); } catch {} throw error; }
  }
  private row(alias: string): AliasRow | undefined {
    return this.db.prepare('SELECT alias,revision,account_id,routing,skills,prices,stop_at_subscription_limit,subscription_window_ids FROM billing_aliases WHERE alias=?').get(alias) as AliasRow | undefined;
  }
  private account(id: string): Account {
    const row = this.db.prepare('SELECT document FROM billing_accounts WHERE account_id=?').get(id) as { document: string } | undefined;
    return row ? JSON.parse(row.document) as Account : emptyAccount();
  }
  private revision(expected: number | undefined, actual: number): void {
    if (expected !== undefined && (!Number.isSafeInteger(expected) || expected < 0)) throw new Error('Expected billing revision must be a nonnegative safe integer.');
    if (expected !== undefined && expected !== actual) throw new BillingRevisionConflict(actual);
  }

  get(alias: string): BillingRecord {
    this.known(alias);
    if (!this.db.isTransaction) return this.transaction(() => this.get(alias), false);
    const row = this.row(alias), id = row?.account_id ?? `agent:${alias}`, account = this.account(id), settings = this.config.agents[alias]!;
    const record: BillingRecord = { alias, enabled: settings.enabled, profile: settings.profile, model: settings.model ?? null,
      revision: row?.revision ?? 0, accountKey: sharedKey(id), ...account, routing: row?.routing ? JSON.parse(row.routing) as BillingRouting : null,
      windows: account.windows.map(window => ({ ...window, scope: scope(window, account.mode) })), subscriptionExhausted: { ...account.subscriptionExhausted },
      stopAtSubscriptionLimit: Boolean(row?.stop_at_subscription_limit), subscriptionWindowIds: row?.subscription_window_ids ? JSON.parse(row.subscription_window_ids) as string[] : null,
      skills: row?.skills ? JSON.parse(row.skills) as string[] : null,
      prices: row?.prices ? JSON.parse(row.prices) as BillingPrices : null, stale: false };
    const now = Date.now();
    record.stale = this.evaluate(record, now).windows.some(window => window.state === 'unknown')
      || !record.observedAt || new Date(record.observedAt).getTime() > now
      || Boolean(record.validUntil && new Date(record.validUntil).getTime() <= now);
    return structuredClone(record);
  }
  list(): BillingRecord[] {
    this.open();
    return this.transaction(() => Object.keys(this.config.agents).map(alias => this.get(alias)), false);
  }

  update(alias: string, input: BillingInput, expectedRevision?: number): BillingRecord { return this.write(alias, input, expectedRevision, false); }
  importSnapshot(alias: string, input: BillingInput, expectedRevision?: number): BillingRecord { return this.write(alias, input, expectedRevision, true); }

  private write(alias: string, value: BillingInput, expectedRevision: number | undefined, reported: boolean): BillingRecord {
    this.known(alias);
    const input = parseSchema<BillingInput>(billingDefinition, value, reported ? 'reported billing snapshot' : 'billing update');
    if (reported && (input.source?.kind !== 'reported' || !input.observedAt)) throw new Error('A reported snapshot requires an explicit reported source and observation timestamp.');
    if (!reported && input.source?.kind === 'reported') throw new Error('Use importSnapshot to import an explicitly reported observation.');
    if (input.accountKey !== undefined && input.accountKey !== null) input.accountKey = nonempty(input.accountKey, 'Account key').trim();
    if (input.plan !== undefined && input.plan !== null) nonempty(input.plan, 'Plan label');
    if (input.source) nonempty(input.source.label, 'Source label');
    if (input.observedAt) input.observedAt = timestamp(input.observedAt, 'Observation time');
    if (input.validUntil) input.validUntil = timestamp(input.validUntil, 'Observation validity');
    if (input.windows) {
      if (input.windows.length && !input.observedAt) throw new Error('Limit windows require an explicit observation timestamp; observations are never refreshed automatically.');
      if (new Set(input.windows.map(window => window.id)).size !== input.windows.length) throw new Error('Limit window IDs must be unique.');
      input.windows = input.windows.map(window => {
        nonempty(window.id, 'Window ID'); nonempty(window.label, 'Window label'); nonempty(window.unit, 'Window unit');
        return { ...window, limit: window.limit ?? null, remaining: window.remaining ?? null, used: window.used ?? null,
          resetAt: window.resetAt ? timestamp(window.resetAt, 'Window reset') : null };
      });
    }
    if (input.prices) {
      if (input.prices.currency !== undefined && input.prices.currency !== null) nonempty(input.prices.currency, 'Currency');
    }
    if (input.routing) {
      const capabilities = input.routing.capabilities ?? [];
      for (const capability of capabilities) nonempty(capability, 'Capability');
      if (new Set(capabilities).size !== capabilities.length) throw new Error('Routing capabilities must be unique.');
      input.routing = { ...input.routing, capabilities, costRank: input.routing.costRank ?? null, qualityRank: input.routing.qualityRank ?? null };
    }
    if (input.skills && new Set(input.skills).size !== input.skills.length) throw new Error('Billing skill overrides must be unique.');
    if (input.subscriptionWindowIds) {
      for (const id of input.subscriptionWindowIds) nonempty(id, 'Subscription window ID');
      if (new Set(input.subscriptionWindowIds).size !== input.subscriptionWindowIds.length) throw new Error('Subscription window IDs must be unique.');
    }
    const fields = ACCOUNT_FIELDS.filter(field => Object.hasOwn(input, field));
    return this.transaction(() => {
      const previous = this.row(alias); this.revision(expectedRevision, previous?.revision ?? 0);
      if (!Object.keys(input).length) return this.get(alias);
      const id = input.accountKey !== undefined ? input.accountKey === null ? `agent:${alias}` : `shared:${input.accountKey}` : previous?.account_id ?? `agent:${alias}`;
      const account = this.account(id);
      this.updateExhaustion(account);
      for (const field of fields) (account as unknown as Record<string, unknown>)[field] = input[field];
      if (fields.length && !Object.hasOwn(input, 'source')) account.source = { kind: reported ? 'reported' : 'manual', label: reported ? 'Reported snapshot' : 'Manual entry' };
      this.updateExhaustion(account);
      this.db.prepare('INSERT INTO billing_accounts(account_id,document) VALUES(?,?) ON CONFLICT(account_id) DO UPDATE SET document=excluded.document').run(id, json(account));
      if (fields.length) this.db.prepare('UPDATE billing_aliases SET revision=revision+1 WHERE account_id=?').run(id);
      const existing = this.row(alias);
      const revision = existing && existing.account_id === id && fields.length ? existing.revision : (existing?.revision ?? 0) + 1;
      const routing = Object.hasOwn(input, 'routing') ? input.routing === null ? null : json(input.routing) : existing?.routing ?? null;
      const skills = Object.hasOwn(input, 'skills') ? input.skills === null ? null : json(input.skills) : existing?.skills ?? null;
      const prices = Object.hasOwn(input, 'prices') ? input.prices === null ? null
        : json({ currency: null, inputPerMillion: null, outputPerMillion: null, ...(existing?.prices ? JSON.parse(existing.prices) as BillingPrices : {}), ...input.prices }) : existing?.prices ?? null;
      const stop = input.stopAtSubscriptionLimit === undefined ? existing?.stop_at_subscription_limit ?? 0 : Number(input.stopAtSubscriptionLimit);
      const windowIds = input.subscriptionWindowIds === undefined ? existing?.subscription_window_ids ?? null : input.subscriptionWindowIds === null ? null : json(input.subscriptionWindowIds);
      this.db.prepare('INSERT INTO billing_aliases(alias,revision,account_id,routing,skills,prices,stop_at_subscription_limit,subscription_window_ids) VALUES(?,?,?,?,?,?,?,?) ON CONFLICT(alias) DO UPDATE SET revision=excluded.revision,account_id=excluded.account_id,routing=excluded.routing,skills=excluded.skills,prices=excluded.prices,stop_at_subscription_limit=excluded.stop_at_subscription_limit,subscription_window_ids=excluded.subscription_window_ids')
        .run(alias, revision, id, routing, skills, prices, stop, windowIds);
      if (previous?.account_id && previous.account_id !== id) this.db.prepare('DELETE FROM billing_accounts WHERE account_id=? AND NOT EXISTS(SELECT 1 FROM billing_aliases WHERE account_id=?)').run(previous.account_id, previous.account_id);
      return this.get(alias);
    });
  }

  remove(alias: string, expectedRevision?: number): BillingRecord {
    this.known(alias);
    return this.transaction(() => {
      const row = this.row(alias); this.revision(expectedRevision, row?.revision ?? 0);
      const protectedAccount = row?.stop_at_subscription_limit ? this.account(row.account_id ?? `agent:${alias}`) : undefined;
      if (protectedAccount) this.updateExhaustion(protectedAccount);
      this.db.prepare('INSERT INTO billing_aliases(alias,revision,account_id,routing,skills,prices) VALUES(?,?,NULL,NULL,NULL,NULL) ON CONFLICT(alias) DO UPDATE SET revision=excluded.revision,account_id=NULL,routing=NULL,skills=NULL,prices=NULL').run(alias, (row?.revision ?? 0) + 1);
      // A detached alias must not accidentally rediscover its previous private quota.
      for (const account of new Set([`agent:${alias}`, ...(row?.account_id ? [row.account_id] : [])])) {
        this.db.prepare('DELETE FROM billing_accounts WHERE account_id=? AND NOT EXISTS(SELECT 1 FROM billing_aliases WHERE account_id=?)').run(account, account);
      }
      if (protectedAccount && Object.keys(protectedAccount.subscriptionExhausted ?? {}).length) {
        this.db.prepare('INSERT INTO billing_accounts(account_id,document) VALUES(?,?) ON CONFLICT(account_id) DO UPDATE SET document=excluded.document')
          .run(`agent:${alias}`, json({ ...emptyAccount(), subscriptionExhausted: protectedAccount.subscriptionExhausted }));
      }
      return this.get(alias);
    });
  }

  availability(alias: string, now: Date | number | string = Date.now()): BillingAvailability { return this.availabilityFor(this.get(alias), now); }
  /** Classify the supplied observation without rereading a different account revision. */
  availabilityFor(record: BillingRecord, now: Date | number | string = Date.now()): BillingAvailability { return this.evaluate(record, nowValue(now)); }
  private updateExhaustion(account: Account): void {
    const latches = Object.assign(Object.create(null) as Record<string, string>, account.subscriptionExhausted), now = Date.now();
    for (const window of account.windows) {
      if (scope(window, account.mode) !== 'subscription') continue;
      const value = remaining(window), observed = account.observedAt && new Date(account.observedAt).getTime() <= now
        ? account.observedAt : '0001-01-01T00:00:00.000Z';
      if (value !== null && value <= 0) {
        if (!latches[window.id] || observed > latches[window.id]!) latches[window.id] = observed;
      } else if (value !== null && value > 0 && account.source && account.observedAt
        && new Date(account.observedAt).getTime() <= now && (!account.validUntil || new Date(account.validUntil).getTime() > now)
        && (!window.resetAt || new Date(window.resetAt).getTime() > now) && (account.validUntil || window.resetAt)
        && (!latches[window.id] || account.observedAt >= latches[window.id]!)) delete latches[window.id];
    }
    account.subscriptionExhausted = latches;
  }
  subscriptionProtection(alias: string, now: Date | number | string = Date.now()): SubscriptionProtection { return this.subscriptionProtectionFor(this.get(alias), now); }
  /** Pure classification of the captured policy, observation and retained exhaustion. */
  subscriptionProtectionFor(record: BillingRecord, now: Date | number | string = Date.now()): SubscriptionProtection {
    const evaluated = nowValue(now), enabled = record.stopAtSubscriptionLimit === true, reasons: string[] = [];
    const ids = record.subscriptionWindowIds?.length ? record.subscriptionWindowIds : [...new Set([
      ...record.windows.filter(window => scope(window, record.mode) === 'subscription').map(window => window.id), ...Object.keys(record.subscriptionExhausted ?? {})])];
    const base = { enabled, alias: record.alias, accountKey: record.accountKey, evaluatedAt: new Date(evaluated).toISOString(), observedAt: record.observedAt, windowIds: [...ids] };
    if (!enabled) return { ...base, allowed: true, status: 'disabled', reasons: [] };
    if (record.mode !== 'subscription') reasons.push('Protection requires an explicitly recorded subscription account.');
    if (!ids.length) reasons.push('No included subscription allowance windows are configured; paid credits never count as included allowance.');
    const available = this.evaluate(record, evaluated), windows = ids.map(id => available.windows.find(window => window.id === id));
    let exhausted = false;
    ids.forEach((id, index) => {
      const window = windows[index];
      if (Object.hasOwn(record.subscriptionExhausted ?? {}, id) || window && scope(window, record.mode) === 'subscription' && remaining(window) !== null && remaining(window)! <= 0) {
        exhausted = true; reasons.push(`Subscription window ${id} is exhausted. A fresh positive observation is required; a reset date does not replenish it.`);
      } else if (!window || scope(window, record.mode) !== 'subscription') reasons.push(`Included subscription window ${id} is missing or is not subscription allowance.`);
      else if (window.state !== 'available') reasons.push(`Subscription window ${id} cannot be verified: ${window.reasons.join(' ')}`);
    });
    const allowed = !reasons.length;
    return { ...base, allowed, status: allowed ? 'available' : exhausted ? 'exhausted' : 'unknown', reasons };
  }
  assertSubscriptionAllowed(alias: string): void {
    const receipt = this.subscriptionProtection(alias);
    if (!receipt.allowed) throw new SubscriptionLimitError(receipt);
  }
  private evaluate(record: BillingRecord, now: number): BillingAvailability {
    const global: string[] = [];
    if (!record.observedAt) global.push('No quota observation has been recorded.');
    else if (new Date(record.observedAt).getTime() > now) global.push('The observation timestamp is in the future.');
    if (!record.source) global.push('The observation source is unknown.');
    if (record.validUntil && new Date(record.validUntil).getTime() <= now) global.push('The observation validity has expired; capacity is unknown.');
    const windows: BillingAvailabilityWindow[] = record.windows.map(window => {
      const reasons = [...global];
      if (window.resetAt && new Date(window.resetAt).getTime() <= now) reasons.push('The reported reset has passed; replenishment has not been observed.');
      if (!record.validUntil && !window.resetAt) reasons.push('No observation validity bound was provided; set validUntil or resetAt.');
      let remaining = window.remaining;
      if (remaining === null && window.limit !== null && window.used !== null) remaining = Math.max(0, window.limit - window.used);
      if (remaining === null) reasons.push('Remaining capacity is unknown.');
      let state: BillingAvailabilityStatus = 'unknown';
      if (!reasons.length) state = remaining! <= 0 ? 'exhausted' : 'available';
      return { ...window, state, effectiveRemaining: reasons.length ? null : remaining, reasons };
    });
    const capacityWindows = record.mode === 'subscription' ? windows.filter(window => scope(window, record.mode) === 'subscription') : windows;
    const reasons = [...global];
    if (!capacityWindows.length) reasons.push('No observed allowance windows are configured; no unlimited allowance is inferred.');
    for (const window of capacityWindows) for (const reason of window.reasons) if (!reasons.includes(reason)) reasons.push(reason);
    const status: BillingAvailabilityStatus = capacityWindows.some(window => window.state === 'exhausted') ? 'exhausted'
      : capacityWindows.length && capacityWindows.every(window => window.state === 'available') ? 'available' : 'unknown';
    return { status, alias: record.alias, accountKey: record.accountKey, observedAt: record.observedAt, evaluatedAt: new Date(now).toISOString(), reasons, windows };
  }

  close(): void {
    if (this.closed) return;
    this.db.close(); this.closed = true;
  }
}
