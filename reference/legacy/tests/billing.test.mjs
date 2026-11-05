import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, mkdirSync, readFileSync, rmSync, statSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { basename, dirname, join, resolve } from 'node:path';
import { DatabaseSync } from 'node:sqlite';
import { BillingManager, BillingRevisionConflict } from '../dist/billing.js';
import { loadConfig } from '../dist/config.js';

const NOW = Date.parse('2030-01-15T12:00:00.000Z');
const iso = offset => new Date(NOW + offset).toISOString();
const snapshot = (overrides = {}) => ({ mode: 'subscription', plan: 'User-configured plan',
  observedAt: iso(-1000), validUntil: iso(3600000), source: { kind: 'manual', label: 'PM observation' },
  windows: [{ id: 'quota', label: 'Configured request window', unit: 'requests', limit: 10, used: 5, remaining: 5, resetAt: iso(86400000) }], ...overrides });

function fixture(t) {
  const parent = resolve(tmpdir()), root = mkdtempSync(join(parent, 'council-billing-'));
  const config = loadConfig(root, { backend: 'mock', agents: {
    small: { enabled: true, profile: 'codex' }, large: { enabled: true, profile: 'claude' },
    disabled: { enabled: false, profile: 'junie' },
  } });
  const managers = [];
  t.after(() => {
    for (const manager of managers) manager.close();
    assert.equal(dirname(resolve(root)), parent); assert.ok(basename(root).startsWith('council-billing-'));
    rmSync(root, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 });
  });
  return { root, config, filename: join(config.stateDir, 'billing.sqlite'), open() { const manager = new BillingManager(config); managers.push(manager); return manager; } };
}

test('billing defaults are unknown, expose configured aliases only and never infer an unlimited allowance', t => {
  const billing = fixture(t).open();
  assert.deepEqual(billing.list().map(record => record.alias), ['small', 'large', 'disabled']);
  const record = billing.get('small');
  assert.equal(record.mode, 'unknown'); assert.equal(record.revision, 0); assert.equal(record.observedAt, null);
  assert.equal(record.prices, null); assert.equal(record.routing, null); assert.equal(record.skills, null);
  assert.equal(billing.get('disabled').enabled, false);
  assert.equal(billing.availability('small', NOW).status, 'unknown');
  assert.throws(() => billing.get('not-configured'), /Unknown agent alias/);
  assert.throws(() => billing.update('not-configured', snapshot()), /Unknown agent alias/);
  assert.equal(billing.db.prepare('SELECT count(*) AS count FROM billing_aliases').get().count, 0);
  for (const mode of ['api', 'subscription', 'local']) {
    billing.update('small', { mode }); assert.equal(billing.availability('small', NOW).status, 'unknown');
  }
});

test('manual billing edits persist cloned data, nullable prices and monotonic deletion revisions', t => {
  const setup = fixture(t), billing = setup.open();
  const saved = billing.update('small', { ...snapshot(), prices: { currency: 'EUR', inputPerMillion: 0, outputPerMillion: 2.5 },
    routing: { tier: 'small', capabilities: ['analysis'], costRank: 0.0001, qualityRank: 1000000 }, skills: ['council-advisor'] }, 0);
  assert.equal(saved.revision, 1); assert.equal(saved.prices.inputPerMillion, 0);
  saved.windows[0].remaining = 0; saved.routing.capabilities.push('caller mutation');
  assert.equal(billing.get('small').windows[0].remaining, 5); assert.deepEqual(billing.get('small').routing.capabilities, ['analysis']);
  const edited = billing.update('small', { prices: { outputPerMillion: null } }, 1);
  assert.deepEqual(edited.prices, { currency: 'EUR', inputPerMillion: 0, outputPerMillion: null });
  assert.equal(setup.open().get('small').revision, 2);
  const removed = billing.remove('small', 2);
  assert.equal(removed.revision, 3); assert.equal(removed.mode, 'unknown'); assert.equal(removed.skills, null); assert.equal(removed.windows.length, 0);
  assert.throws(() => billing.update('small', { mode: 'api' }, 2), error => error instanceof BillingRevisionConflict && error.status === 409 && error.currentRevision === 3);
  assert.equal(billing.update('small', { mode: 'api' }, 3).revision, 4);
});

test('aliases sharing an account use one pool and shared edits invalidate every linked revision', t => {
  const billing = fixture(t).open();
  billing.update('small', { ...snapshot(), accountKey: 'same-subscription', skills: [] }, 0);
  billing.update('large', { accountKey: 'same-subscription', routing: { tier: 'large', costRank: 5, qualityRank: 8 } }, 0);
  assert.equal(billing.db.prepare('SELECT count(*) AS count FROM billing_accounts').get().count, 1);
  assert.equal(billing.get('large').windows[0].remaining, 5);
  const large = billing.get('large'), small = billing.get('small');
  billing.update('large', { observedAt: iso(0), windows: [{ ...large.windows[0], remaining: 2, used: 8 }] }, large.revision);
  assert.equal(billing.get('small').revision, small.revision + 1);
  assert.equal(billing.get('large').revision, large.revision + 1);
  assert.equal(billing.get('small').windows[0].remaining, 2); assert.deepEqual(billing.get('small').skills, []);
  assert.equal(billing.get('small').routing, null); assert.equal(billing.get('large').routing.tier, 'large');
  assert.throws(() => billing.update('small', { plan: 'Stale replacement' }, small.revision), BillingRevisionConflict);
  assert.equal(billing.db.prepare('SELECT count(*) AS count FROM billing_accounts').get().count, 1);
});

test('model prices stay alias-local while quota observations and their revision conflicts remain shared', t => {
  const billing = fixture(t).open();
  billing.importSnapshot('small', { ...snapshot(), accountKey: 'same-account', source: { kind: 'reported', label: 'Shared quota report' },
    prices: { currency: 'USD', inputPerMillion: 1, outputPerMillion: 2 } });
  const small = billing.get('small');
  const large = billing.update('large', { accountKey: 'same-account', prices: { currency: 'EUR', inputPerMillion: 10, outputPerMillion: 20 } });
  assert.deepEqual(billing.get('small'), small, 'Linking another model with its own rates does not rewrite the shared observation.');
  assert.deepEqual(large.prices, { currency: 'EUR', inputPerMillion: 10, outputPerMillion: 20 });
  assert.deepEqual(large.windows, small.windows); assert.deepEqual(large.source, small.source);
  const edited = billing.update('large', { prices: { outputPerMillion: 25 } }, large.revision);
  assert.equal(edited.revision, large.revision + 1);
  assert.deepEqual(edited.prices, { currency: 'EUR', inputPerMillion: 10, outputPerMillion: 25 });
  assert.deepEqual(billing.get('small'), small);
  assert.deepEqual(edited.source, small.source); assert.equal(edited.observedAt, small.observedAt);
  assert.throws(() => billing.update('large', { prices: null }, large.revision), BillingRevisionConflict);
  billing.update('small', { observedAt: iso(0), windows: [{ ...small.windows[0], remaining: 1, used: 9 }] }, small.revision);
  assert.equal(billing.get('large').revision, edited.revision + 1);
  assert.deepEqual(billing.get('large').prices, edited.prices);
  assert.equal(billing.get('large').windows[0].remaining, 1);
  assert.throws(() => billing.update('large', { prices: null }, edited.revision), BillingRevisionConflict);
});

test('model prices survive account relinking and detaching, and only explicit removal clears them', t => {
  const billing = fixture(t).open();
  billing.update('small', { ...snapshot(), prices: { currency: 'USD', inputPerMillion: 1, outputPerMillion: 2 } });
  billing.update('large', { ...snapshot(), accountKey: 'shared', prices: { currency: 'EUR', inputPerMillion: 10, outputPerMillion: 20 } });
  const ownPrices = billing.get('small').prices, sharedPrices = billing.get('large').prices;
  assert.deepEqual(billing.update('small', { accountKey: 'shared' }).prices, ownPrices);
  assert.deepEqual(billing.update('small', { accountKey: null }).prices, ownPrices);
  assert.deepEqual(billing.get('large').prices, sharedPrices);
  assert.equal(billing.update('small', { prices: null }).prices, null);
  billing.update('small', { prices: ownPrices });
  assert.equal(billing.remove('small').prices, null);
  assert.deepEqual(billing.get('large').prices, sharedPrices);
});

test('alias-local routing and skill overrides never refresh or rewrite another alias shared observation', t => {
  const billing = fixture(t).open();
  billing.importSnapshot('small', { ...snapshot(), accountKey: 'shared', source: { kind: 'reported', label: 'Explicit normalized report' } });
  billing.update('large', { accountKey: 'shared' });
  const before = billing.get('small');
  billing.update('large', { routing: { tier: 'standard', capabilities: ['review'], costRank: null, qualityRank: 2 }, skills: ['council-implementation-review'] });
  const after = billing.get('small');
  assert.equal(after.revision, before.revision); assert.deepEqual(after.source, before.source); assert.equal(after.observedAt, before.observedAt);
  assert.equal(after.routing, null); assert.equal(after.skills, null);
  assert.deepEqual(billing.get('large').skills, ['council-implementation-review']);
  assert.deepEqual(billing.update('large', { skills: [] }).skills, []);
  assert.equal(billing.update('large', { skills: null }).skills, null);
});

test('unlinking and deleting aliases preserve a linked pool but never resurrect a deleted private or orphan quota', t => {
  const billing = fixture(t).open();
  billing.update('small', snapshot());
  billing.update('small', { ...snapshot(), accountKey: 'shared' });
  billing.update('large', { accountKey: 'shared' });
  const detached = billing.update('small', { accountKey: null });
  assert.equal(detached.mode, 'unknown'); assert.equal(detached.windows.length, 0);
  assert.equal(billing.get('large').windows[0].remaining, 5);
  billing.remove('large');
  assert.equal(billing.update('small', { accountKey: 'shared' }).windows.length, 0);
});

test('invalid billing data, unknown skills and inconsistent import provenance fail atomically', t => {
  const billing = fixture(t).open(); billing.update('small', snapshot());
  const before = billing.get('small');
  for (const input of [{ unknown: true }, { mode: 'unlimited' }, { accountKey: ' ' }, { observedAt: '2030-02-30T00:00:00Z' },
    { observedAt: '2030-01-01' }, { prices: { inputPerMillion: -1 } }, { prices: { outputPerMillion: Infinity } },
    { routing: { tier: 'large', costRank: 0 } }, { routing: { tier: 'small', qualityRank: NaN } },
    { routing: { tier: 'small', capabilities: ['duplicate', 'duplicate'] } }, { skills: ['../../arbitrary-skill'] },
    { skills: ['council-advisor', 'council-advisor'] }, { windows: [{ id: 'quota', label: 'Quota', unit: 'requests', remaining: 1 }] },
    { observedAt: iso(0), windows: [{ id: 'quota', label: 'Quota', unit: 'requests', remaining: -1 }] },
    { observedAt: iso(0), windows: [{ id: 'quota', label: 'Quota', unit: 'requests' }, { id: 'quota', label: 'Quota', unit: 'requests' }] },
    { source: { kind: 'reported', label: 'Not an explicit import' } }]) assert.throws(() => billing.update('small', input));
  assert.throws(() => billing.importSnapshot('small', { mode: 'api' }), /reported source/);
  assert.throws(() => billing.importSnapshot('small', { source: { kind: 'manual', label: 'Not reported' }, observedAt: iso(0) }), /reported source/);
  assert.deepEqual(billing.get('small'), before);
});

test('quota observations are never automatically refreshed when prices or routing metadata change', t => {
  const billing = fixture(t).open(); billing.importSnapshot('small', { ...snapshot(), source: { kind: 'reported', label: 'Normalized input only' } });
  const { observedAt, source } = billing.get('small');
  billing.update('small', { prices: { currency: 'USD', inputPerMillion: 1.25 } });
  assert.equal(billing.get('small').observedAt, observedAt); assert.deepEqual(billing.get('small').source, source);
  billing.update('small', { routing: { tier: 'small' } }); assert.equal(billing.get('small').observedAt, observedAt);
  assert.equal(billing.get('small').windows[0].remaining, 5);
});

test('availability uses only bounded observations and never decrements stored capacity', t => {
  const billing = fixture(t).open(); billing.update('small', snapshot());
  const current = billing.availability('small', NOW);
  assert.equal(current.status, 'available'); assert.equal(current.windows[0].effectiveRemaining, 5);
  assert.equal(billing.availability('small', NOW + 1000).status, 'available'); assert.equal(billing.get('small').windows[0].remaining, 5);
  billing.update('small', snapshot({ windows: [{ id: 'w', label: 'Exhausted', unit: 'requests', limit: 10, used: 10 }] }));
  assert.equal(billing.availability('small', NOW).status, 'exhausted');
  billing.update('small', snapshot({ windows: [{ id: 'w', label: 'Derived remaining', unit: 'tokens', limit: 0.5, used: 0.2 }] }));
  assert.equal(billing.availability('small', NOW).status, 'available');
  billing.update('small', snapshot({ windows: [{ id: 'w', label: 'Unknown remaining', unit: 'requests', limit: 10 }] }));
  assert.equal(billing.availability('small', NOW).status, 'unknown');
});

test('expired reset and validity observations remain unknown even when their stored remaining count is zero', t => {
  const billing = fixture(t).open();
  for (const override of [{ validUntil: iso(0) }, { validUntil: iso(3600000), windows: [{ id: 'w', label: 'Reset passed', unit: 'requests', remaining: 0, resetAt: iso(0) }] }]) {
    billing.update('small', snapshot({ windows: [{ id: 'w', label: 'Zero snapshot', unit: 'requests', remaining: 0, resetAt: iso(86400000) }], ...override }));
    const value = billing.availability('small', NOW);
    assert.equal(value.status, 'unknown'); assert.equal(value.windows[0].effectiveRemaining, null);
    assert.equal(billing.get('small').windows[0].remaining, 0);
  }
});

test('unbounded or future observations do not imply a current allowance', t => {
  const billing = fixture(t).open();
  billing.update('small', snapshot({ validUntil: null, windows: [{ id: 'w', label: 'No validity bound', unit: 'requests', remaining: 10 }] }));
  assert.equal(billing.availability('small', NOW).status, 'unknown');
  assert.ok(billing.availability('small', NOW).reasons.some(reason => reason.includes('validity bound')));
  billing.update('small', snapshot({ observedAt: iso(1000), windows: [{ id: 'w', label: 'Future observation', unit: 'requests', remaining: 0 }] }));
  assert.equal(billing.availability('small', NOW).status, 'unknown');
  assert.throws(() => billing.availability('small', NaN), /valid date/);
});

test('multiple windows distinguish a current exhausted pool from obsolete exhaustion', t => {
  const billing = fixture(t).open();
  billing.update('small', snapshot({ windows: [
    { id: 'old', label: 'Old exhaustion', unit: 'messages', remaining: 0, resetAt: iso(-1) },
    { id: 'current', label: 'Current positive capacity', unit: 'messages', remaining: 8, resetAt: iso(10000) },
  ] }));
  assert.equal(billing.availability('small', NOW).status, 'unknown');
  billing.update('small', snapshot({ windows: [
    { id: 'unknown', label: 'Unknown counter', unit: 'messages' },
    { id: 'current', label: 'Current exhaustion', unit: 'messages', remaining: 0, resetAt: iso(10000) },
  ] }));
  assert.equal(billing.availability('small', NOW).status, 'exhausted');
});

test('separate connections enforce stale-revision conflicts and bounded runtime contention', t => {
  const setup = fixture(t), first = setup.open(), second = setup.open(); first.update('small', snapshot());
  const revision = second.get('small').revision; first.update('small', { plan: 'First connection won' }, revision);
  assert.throws(() => second.update('small', { plan: 'Stale editor' }, revision), error => error.code === 'COUNCIL_BILLING_REVISION_CONFLICT' && error.status === 409);
  const writer = new DatabaseSync(setup.filename); writer.exec('BEGIN IMMEDIATE');
  try {
    const started = performance.now(); assert.throws(() => second.update('small', { mode: 'api' }), error => (error.errcode & 0xff) === 5);
    assert.ok(performance.now() - started < 1200);
  } finally { writer.exec('ROLLBACK'); writer.close(); }
  assert.equal(second.get('small').plan, 'First connection won'); assert.equal(second.db.prepare('PRAGMA busy_timeout').get().timeout, 250);
});

test('a publication failure rolls back shared account data and every linked revision together', t => {
  const billing = fixture(t).open(); billing.update('small', { ...snapshot(), accountKey: 'shared' }); billing.update('large', { accountKey: 'shared' });
  const before = billing.list(), nativePrepare = billing.db.prepare.bind(billing.db);
  billing.db.prepare = sql => {
    const statement = nativePrepare(sql);
    if (sql.startsWith('INSERT INTO billing_aliases')) statement.run = () => { throw new Error('Injected atomic alias publication failure.'); };
    return statement;
  };
  try { assert.throws(() => billing.update('large', { plan: 'Must roll back' }), /publication failure/); }
  finally { billing.db.prepare = nativePrepare; }
  assert.deepEqual(billing.list(), before);
});

test('startup retries immediate WAL contention and restores its short runtime timeout', t => {
  const setup = fixture(t), nativeExec = DatabaseSync.prototype.exec; let attempts = 0;
  DatabaseSync.prototype.exec = function (sql) {
    if (sql.includes('PRAGMA journal_mode=WAL;') && ++attempts <= 2) throw Object.assign(new Error('Injected immediate startup contention.'), { errcode: 5 });
    return nativeExec.call(this, sql);
  };
  let billing;
  try { billing = setup.open(); } finally { DatabaseSync.prototype.exec = nativeExec; }
  assert.equal(attempts, 3); assert.equal(billing.db.prepare('PRAGMA busy_timeout').get().timeout, 250);
  assert.equal(billing.db.prepare('PRAGMA journal_mode').get().journal_mode, 'wal');
});

test('version 1 migration preserves shared model rates on every linked alias without changing quotas or revisions', t => {
  const setup = fixture(t); mkdirSync(setup.config.stateDir, { recursive: true });
  const raw = new DatabaseSync(setup.filename), account = { ...snapshot(), prices: { currency: 'USD', inputPerMillion: 1, outputPerMillion: 3 } };
  raw.exec(`PRAGMA journal_mode=WAL;
    CREATE TABLE billing_accounts(account_id TEXT PRIMARY KEY, document TEXT NOT NULL);
    CREATE TABLE billing_aliases(alias TEXT PRIMARY KEY, revision INTEGER NOT NULL, account_id TEXT REFERENCES billing_accounts(account_id), routing TEXT, skills TEXT);
    CREATE INDEX billing_account_aliases ON billing_aliases(account_id); PRAGMA user_version=1;`);
  raw.prepare('INSERT INTO billing_accounts VALUES(?,?)').run('shared:legacy', JSON.stringify(account));
  const insert = raw.prepare('INSERT INTO billing_aliases VALUES(?,?,?,?,?)');
  insert.run('small', 7, 'shared:legacy', JSON.stringify({ tier: 'small', capabilities: ['read'], costRank: 1, qualityRank: 1 }), JSON.stringify([]));
  insert.run('large', 11, 'shared:legacy', null, JSON.stringify(['council-advisor']));
  insert.run('disabled', 3, null, null, null); raw.close();
  const billing = setup.open(), small = billing.get('small'), large = billing.get('large');
  assert.equal(billing.db.prepare('PRAGMA user_version').get().user_version, 3);
  assert.equal(small.revision, 7); assert.equal(large.revision, 11); assert.equal(billing.get('disabled').revision, 3);
  for (const record of [small, large]) {
    assert.deepEqual(record.prices, account.prices); assert.deepEqual(record.windows, account.windows.map(window => ({ ...window, scope: 'subscription' })));
    assert.deepEqual(record.source, account.source); assert.equal(record.observedAt, account.observedAt);
    assert.equal(record.validUntil, account.validUntil); assert.equal(record.accountKey, 'legacy');
  }
  assert.deepEqual(small.skills, []); assert.deepEqual(large.skills, ['council-advisor']); assert.equal(small.routing.tier, 'small');
  assert.equal(billing.get('disabled').prices, null);
  const stored = JSON.parse(billing.db.prepare('SELECT document FROM billing_accounts WHERE account_id=?').get('shared:legacy').document);
  assert.equal(Object.hasOwn(stored, 'prices'), false);
  billing.update('large', { prices: { outputPerMillion: 9 } }, large.revision);
  assert.deepEqual(billing.get('small'), small); assert.equal(billing.get('large').prices.outputPerMillion, 9);
  assert.deepEqual(setup.open().get('small'), small, 'Reopening an upgraded database does not migrate or revise it again.');
});

test('a failed model-price migration rolls back the schema, rates and alias revisions together', t => {
  const setup = fixture(t); mkdirSync(setup.config.stateDir, { recursive: true });
  const raw = new DatabaseSync(setup.filename), account = { ...snapshot(), prices: { currency: null, inputPerMillion: 0, outputPerMillion: 3 } };
  raw.exec(`CREATE TABLE billing_accounts(account_id TEXT PRIMARY KEY, document TEXT NOT NULL);
    CREATE TABLE billing_aliases(alias TEXT PRIMARY KEY, revision INTEGER NOT NULL, account_id TEXT REFERENCES billing_accounts(account_id), routing TEXT, skills TEXT);
    CREATE INDEX billing_account_aliases ON billing_aliases(account_id); PRAGMA user_version=1;`);
  raw.prepare('INSERT INTO billing_accounts VALUES(?,?)').run('agent:small', JSON.stringify(account));
  raw.prepare('INSERT INTO billing_aliases VALUES(?,?,?,?,?)').run('small', 4, 'agent:small', null, null); raw.close();
  const prepare = DatabaseSync.prototype.prepare;
  DatabaseSync.prototype.prepare = function (sql) {
    if (sql.startsWith('UPDATE billing_accounts SET document=')) throw new Error('Injected model-price migration failure.');
    return prepare.call(this, sql);
  };
  try { assert.throws(() => setup.open(), /migration failure/); }
  finally { DatabaseSync.prototype.prepare = prepare; }
  const inspect = new DatabaseSync(setup.filename);
  try {
    assert.equal(inspect.prepare('PRAGMA user_version').get().user_version, 1);
    assert.equal(inspect.prepare('PRAGMA table_info(billing_aliases)').all().some(column => column.name === 'prices'), false);
    assert.equal(inspect.prepare('SELECT revision FROM billing_aliases WHERE alias=?').get('small').revision, 4);
    assert.deepEqual(JSON.parse(inspect.prepare('SELECT document FROM billing_accounts').get().document), account);
  } finally { inspect.close(); }
  const recovered = setup.open().get('small');
  assert.equal(recovered.revision, 4); assert.deepEqual(recovered.prices, account.prices); assert.deepEqual(recovered.windows, account.windows.map(window => ({ ...window, scope: 'subscription' })));
});

test('future billing schemas are refused before data, permissions or persistent journal mode changes', t => {
  const setup = fixture(t); mkdirSync(setup.config.stateDir, { recursive: true });
  const raw = new DatabaseSync(setup.filename);
  raw.exec("PRAGMA user_version=999; CREATE TABLE protected(value TEXT); INSERT INTO protected VALUES('unchanged');"); raw.close();
  const before = readFileSync(setup.filename), mode = statSync(setup.filename).mode;
  assert.throws(() => setup.open(), /newer Council version/);
  assert.deepEqual(readFileSync(setup.filename), before); assert.equal(statSync(setup.filename).mode, mode);
  const inspect = new DatabaseSync(setup.filename);
  try { assert.equal(inspect.prepare('PRAGMA journal_mode').get().journal_mode, 'delete'); assert.equal(inspect.prepare('SELECT value FROM protected').get().value, 'unchanged'); }
  finally { inspect.close(); }
});

test('current billing state opens beside a WAL writer and repairs a changed journal without losing observations', t => {
  const setup = fixture(t), first = setup.open(); first.update('small', snapshot());
  const writer = new DatabaseSync(setup.filename); writer.exec('BEGIN IMMEDIATE');
  let second;
  try { second = setup.open(); assert.equal(second.get('small').windows[0].remaining, 5); }
  finally { writer.exec('ROLLBACK'); writer.close(); }
  first.close(); second.close();
  const raw = new DatabaseSync(setup.filename); raw.exec('PRAGMA journal_mode=DELETE'); raw.close();
  const repaired = setup.open(); assert.equal(repaired.db.prepare('PRAGMA journal_mode').get().journal_mode, 'wal');
  assert.equal(repaired.get('small').windows[0].remaining, 5);
});

test('POSIX billing databases and sidecars remain private', { skip: process.platform === 'win32' }, t => {
  const setup = fixture(t), billing = setup.open(); billing.update('small', snapshot());
  for (const filename of [setup.filename, setup.filename + '-wal', setup.filename + '-shm']) assert.equal(statSync(filename).mode & 0o777, 0o600);
});

test('billing close failures stay observable and a later successful close is idempotent', t => {
  const billing = fixture(t).open(), close = billing.db.close.bind(billing.db);
  billing.db.close = () => { throw new Error('Injected billing close failure.'); };
  assert.throws(() => billing.close(), /close failure/); assert.equal(billing.get('small').mode, 'unknown');
  billing.db.close = close; billing.close(); billing.close();
  assert.throws(() => billing.get('small'), /closed/); assert.throws(() => billing.list(), /closed/);
});
