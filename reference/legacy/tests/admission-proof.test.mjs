import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, realpathSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { basename, dirname, join } from 'node:path';
import { DatabaseSync, constants } from 'node:sqlite';
import { admissionConnection, admissionTransaction, newAdmissionAttempt, runAdmissionAttempt, consumeAdmissionRefusal, admissionRefusalOrigin } from '../dist/admission-attempt.js';

function fixture(t) {
  const parent = realpathSync.native(tmpdir()), root = mkdtempSync(join(parent, 'council-begin-proof-'));
  const room = new DatabaseSync(join(root, 'rooms.sqlite')), state = new DatabaseSync(join(root, 'state.sqlite')), holder = new DatabaseSync(join(root, 'state.sqlite'));
  for (const db of [room, state, holder]) db.exec('PRAGMA journal_mode=WAL; PRAGMA busy_timeout=250; PRAGMA synchronous=FULL; CREATE TABLE IF NOT EXISTS evidence(value INTEGER)');
  const roomHandle = admissionConnection(room, 'rooms'), stateHandle = admissionConnection(state, 'state');
  t.after(() => {
    for (const db of [room, state, holder]) { if (db.isTransaction) db.exec('ROLLBACK'); db.close(); }
    assert.equal(dirname(realpathSync.native(root)), parent); assert.ok(basename(root).startsWith('council-begin-proof-'));
    rmSync(root, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 });
  });
  const attempt = () => newAdmissionAttempt(roomHandle, stateHandle, () => {});
  const refusal = handle => {
    let error, entered = false;
    try { runAdmissionAttempt(handle, () => admissionTransaction(room, () => admissionTransaction(state, () => { entered = true; }))); }
    catch (failure) { error = failure; }
    assert.equal(entered, false); assert.equal(error.errcode & 0xff, 5); return error;
  };
  return { root, room, state, holder, attempt, refusal, roomHandle, stateHandle };
}

test('actual state BEGIN refusal binds one synchronous attempt and consumes once after verified ancestor rollback', t => {
  const wire = fixture(t); wire.holder.exec('BEGIN IMMEDIATE');
  const attempt = wire.attempt(), error = wire.refusal(attempt);
  assert.equal(wire.room.isTransaction, false); assert.equal(wire.state.isTransaction, false);
  assert.deepEqual(admissionRefusalOrigin(error), { area: 'state', phase: 'begin', code: error.errcode });
  assert.equal(consumeAdmissionRefusal(attempt, error), true); assert.equal(consumeAdmissionRefusal(attempt, error), false);
});

test('known read-only preflight before the refusal does not invalidate an untouched admission attempt', t => {
  const wire = fixture(t); wire.holder.exec('BEGIN IMMEDIATE');
  const attempt = wire.attempt(); let error;
  try { runAdmissionAttempt(attempt, () => admissionTransaction(wire.room, () => {
    admissionTransaction(wire.state, () => wire.state.prepare('SELECT COUNT(*) FROM evidence').get(), false);
    admissionTransaction(wire.state, () => { throw new Error('No state write callback allowed.'); });
  })); } catch (failure) { error = failure; }
  assert.equal(consumeAdmissionRefusal(attempt, error), true);
});

test('a successful read-only transaction after a genuine refusal invalidates its propagation chain', t => {
  const wire = fixture(t); wire.holder.exec('BEGIN IMMEDIATE');
  const attempt = wire.attempt(); let saved;
  assert.throws(() => runAdmissionAttempt(attempt, () => admissionTransaction(wire.room, () => {
    try { admissionTransaction(wire.state, () => {}); } catch (error) { saved = error; }
    admissionTransaction(wire.state, () => wire.state.prepare('SELECT COUNT(*) FROM evidence').get(), false);
    throw saved;
  })), error => error === saved);
  assert.equal(consumeAdmissionRefusal(attempt, saved), false);
});

test('genuine refusal swallowed before an ancestor successful COMMIT cannot authorize a postcommit rethrow', t => {
  const wire = fixture(t); wire.holder.exec('BEGIN IMMEDIATE');
  const attempt = wire.attempt(); let saved;
  assert.throws(() => runAdmissionAttempt(attempt, () => {
    admissionTransaction(wire.room, () => {
      try { admissionTransaction(wire.state, () => { throw new Error('No callback allowed.'); }); } catch (error) { saved = error; }
      wire.room.prepare('INSERT INTO evidence VALUES(1)').run();
    });
    throw saved;
  }), error => error === saved);
  assert.equal(wire.room.isTransaction, false); assert.equal(wire.room.prepare('SELECT COUNT(*) AS count FROM evidence').get().count, 1);
  assert.equal(consumeAdmissionRefusal(attempt, saved), false);
});

test('an instance rollback wrapper cannot replace native unwind with a successful commit', t => {
  const wire = fixture(t); wire.holder.exec('BEGIN IMMEDIATE');
  const exec = wire.room.exec; let intercepted = false;
  wire.room.exec = function (sql, ...args) { if (sql === 'ROLLBACK') { intercepted = true; return exec.call(this, 'COMMIT'); } return exec.call(this, sql, ...args); };
  const attempt = wire.attempt(); let error;
  try { runAdmissionAttempt(attempt, () => admissionTransaction(wire.room, () => {
    wire.room.prepare('INSERT INTO evidence VALUES(1)').run(); admissionTransaction(wire.state, () => {});
  })); } catch (failure) { error = failure; }
  wire.room.exec = exec;
  assert.equal(intercepted, false); assert.equal(wire.room.isTransaction, false);
  assert.equal(wire.room.prepare('SELECT COUNT(*) AS count FROM evidence').get().count, 0);
  assert.equal(consumeAdmissionRefusal(attempt, error), true);
});

test('genuine native rollback refusal invalidates proof even if the ancestor is later closed', t => {
  const wire = fixture(t); wire.holder.exec('BEGIN IMMEDIATE');
  wire.room.setAuthorizer((action, operation) => action === constants.SQLITE_TRANSACTION && operation === 'ROLLBACK' ? constants.SQLITE_DENY : constants.SQLITE_OK);
  const attempt = wire.attempt(), error = wire.refusal(attempt);
  assert.equal(wire.room.isTransaction, true); wire.room.setAuthorizer(null); wire.room.exec('ROLLBACK');
  assert.equal(wire.room.isTransaction, false); assert.equal(consumeAdmissionRefusal(attempt, error), false);
});

test('proof expires at an async yield and at the creation of another attempt', async t => {
  const wire = fixture(t); wire.holder.exec('BEGIN IMMEDIATE');
  const first = wire.attempt(), error = wire.refusal(first); await Promise.resolve();
  assert.equal(consumeAdmissionRefusal(first, error), false);
  const second = wire.attempt(), later = wire.refusal(second); wire.attempt();
  assert.equal(consumeAdmissionRefusal(second, later), false);
});

test('old genuine errors, copied shapes, wrapped causes and another connection never acquire current-attempt proof', t => {
  const wire = fixture(t); wire.holder.exec('BEGIN IMMEDIATE');
  const original = wire.attempt(), saved = wire.refusal(original); assert.equal(consumeAdmissionRefusal(original, saved), true);
  for (const error of [saved, Object.assign(new Error('BUSY shape'), { code: 'ERR_SQLITE_ERROR', errcode: 5 }), new Error('Wrapped', { cause: saved })]) {
    const attempt = wire.attempt(); assert.throws(() => runAdmissionAttempt(attempt, () => { throw error; }), value => value === error);
    assert.equal(consumeAdmissionRefusal(attempt, error), false);
  }
  const other = new DatabaseSync(join(wire.root, 'state.sqlite'));
  try {
    other.exec('PRAGMA busy_timeout=250');
    const attempt = newAdmissionAttempt(wire.roomHandle, admissionConnection(other, 'state'), () => {}), error = wire.refusal(attempt);
    assert.equal(consumeAdmissionRefusal(attempt, error), false);
  } finally { other.close(); }
});

test('instance BEGIN wrappers, body errors and COMMIT/postcommit errors cannot mint authority', t => {
  const wire = fixture(t), fake = Object.assign(new Error('Synthetic BUSY'), { code: 'ERR_SQLITE_ERROR', errcode: 5 });
  const exec = wire.state.exec; let intercepted = false;
  wire.state.exec = function (sql, ...args) { if (sql.startsWith('BEGIN')) { intercepted = true; throw fake; } return exec.call(this, sql, ...args); };
  const normal = wire.attempt(); runAdmissionAttempt(normal, () => admissionTransaction(wire.room, () => admissionTransaction(wire.state, () => {})));
  assert.equal(intercepted, false); assert.equal(consumeAdmissionRefusal(normal, fake), false); wire.state.exec = exec;
  for (const phase of ['body', 'postcommit']) {
    if (phase === 'postcommit') wire.state.exec = function (sql, ...args) { const value = exec.call(this, sql, ...args); if (sql === 'COMMIT') throw fake; return value; };
    const attempt = wire.attempt();
    assert.throws(() => runAdmissionAttempt(attempt, () => admissionTransaction(wire.room, () => admissionTransaction(wire.state, () => { if (phase === 'body') throw fake; }))), value => value === fake);
    assert.equal(consumeAdmissionRefusal(attempt, fake), false); wire.state.exec = exec;
  }
});
