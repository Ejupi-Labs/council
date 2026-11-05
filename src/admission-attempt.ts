import { DatabaseSync } from 'node:sqlite';

declare const connectionBrand: unique symbol;
declare const attemptBrand: unique symbol;
export type AdmissionConnection = { readonly [connectionBrand]: true };
export type AdmissionAttempt = { readonly [attemptBrand]: true };
type Connection = { db: DatabaseSync; role: 'state' | 'rooms' };
type Attempt = { nonce: symbol; room?: Connection; state: Connection; guard: () => void; active: boolean; ended: boolean; used: boolean; invalid: boolean; didThrow: boolean; thrown?: unknown; refusal?: object };
const connections = new WeakMap<AdmissionConnection, Connection>();
const registered = new WeakMap<DatabaseSync, AdmissionConnection>();
const attempts = new WeakMap<AdmissionAttempt, Attempt>();
const refusals = new WeakMap<object, { attempt: Attempt; code: number }>();
const scopes: Attempt[] = [];
let newestNonce: symbol | undefined, finishedNonce: symbol | undefined;
const nativeExec = DatabaseSync.prototype.exec;
function transactionState(db: DatabaseSync): boolean | undefined {
  try {
    const descriptor = Object.getOwnPropertyDescriptor(db, 'isTransaction');
    return descriptor?.configurable === false && typeof descriptor.get === 'function' ? descriptor.get.call(db) : undefined;
  } catch { return undefined; }
}
function closed(db: DatabaseSync): boolean { return transactionState(db) === false; }

export function admissionConnection(db: DatabaseSync, role: Connection['role']): AdmissionConnection {
  const previous = registered.get(db);
  if (previous) { if (connections.get(previous)!.role !== role) throw new Error('Admission connection role changed.'); return previous; }
  const handle = Object.freeze({}) as AdmissionConnection;
  connections.set(handle, { db, role }); registered.set(db, handle); return handle;
}
export function newAdmissionAttempt(room: AdmissionConnection, state: AdmissionConnection, guard: () => void): AdmissionAttempt {
  const a = connections.get(room), b = connections.get(state);
  if (!a || !b || a.role !== 'rooms' || b.role !== 'state' || a.db === b.db) throw new Error('Invalid admission connection pair.');
  const handle = Object.freeze({}) as AdmissionAttempt;
  const attempt: Attempt = { nonce: Symbol('admission-attempt'), room: a, state: b, guard, active: false, ended: false, used: false, invalid: false, didThrow: false };
  attempts.set(handle, attempt); newestNonce = attempt.nonce; return handle;
}
/** A single call-slot admission needs only its exact state connection. */
export function newStateAdmissionAttempt(state: AdmissionConnection, guard: () => void): AdmissionAttempt {
  const connection = connections.get(state);
  if (!connection || connection.role !== 'state') throw new Error('Invalid state admission connection.');
  const handle = Object.freeze({}) as AdmissionAttempt;
  const attempt: Attempt = { nonce: Symbol('admission-attempt'), state: connection, guard, active: false, ended: false, used: false, invalid: false, didThrow: false };
  attempts.set(handle, attempt); newestNonce = attempt.nonce; return handle;
}
export function runAdmissionAttempt<T>(handle: AdmissionAttempt, operation: () => T): T {
  const attempt = attempts.get(handle);
  if (!attempt || attempt.active || attempt.ended) throw new Error('Admission attempt was already used.');
  attempt.active = true; scopes.push(attempt);
  try {
    const result = operation();
    attempt.invalid = true;
    return result;
  } catch (error) { attempt.didThrow = true; attempt.thrown = error; throw error; }
  finally {
    if (scopes.pop() !== attempt) attempt.invalid = true;
    attempt.active = false; attempt.ended = true; finishedNonce = attempt.nonce;
    if (attempt.refusal) queueMicrotask(() => { attempt.invalid = true; });
  }
}
export function consumeAdmissionRefusal(handle: AdmissionAttempt, error: unknown): boolean {
  const attempt = attempts.get(handle);
  if (!attempt || attempt.used) return false;
  attempt.used = true;
  return scopes.length === 0 && newestNonce === attempt.nonce && finishedNonce === attempt.nonce && attempt.ended && !attempt.active && !attempt.invalid && attempt.didThrow && attempt.thrown === error
    && typeof error === 'object' && error !== null && attempt.refusal === error && refusals.get(error)?.attempt === attempt
    && (!attempt.room || closed(attempt.room.db)) && closed(attempt.state.db);
}
/** Fixed diagnostic metadata only; this never grants retry authority. */
export function admissionRefusalOrigin(error: unknown): { area: 'state'; phase: 'begin'; code: number } | undefined {
  const proof = typeof error === 'object' && error !== null ? refusals.get(error) : undefined;
  return proof ? { area: 'state', phase: 'begin', code: proof.code } : undefined;
}
export function admissionTransaction<T>(db: DatabaseSync, operation: () => T, write = true): T {
  const attempt = scopes.at(-1), allowed = attempt && (attempt.room?.db === db || attempt.state.db === db);
  const initiallyClosed = Boolean(attempt && allowed && write && attempt.state.db === db) && closed(db);
  if (attempt && !allowed) attempt.invalid = true;
  if (attempt && allowed && write) attempt.guard();
  try {
    // Instance exec wrappers are not an authority for minting a BEGIN refusal.
    if (attempt && allowed && write) nativeExec.call(db, 'BEGIN IMMEDIATE');
    else db.exec(write ? 'BEGIN IMMEDIATE' : 'BEGIN');
  } catch (error) {
    const code = error instanceof Error && 'code' in error && error.code === 'ERR_SQLITE_ERROR' && 'errcode' in error ? error.errcode : undefined;
    if (attempt && allowed && write && attempt.state.db === db && initiallyClosed && !attempt.invalid && closed(db)
      && typeof code === 'number' && Number.isInteger(code) && [5, 6].includes(code & 0xff) && typeof error === 'object' && error !== null) {
      if (attempt.refusal) attempt.invalid = true;
      else { attempt.refusal = error; refusals.set(error, { attempt, code }); }
    }
    throw error;
  }
  try {
    // A blocking BEGIN may have outlived the per-launch contention window.
    if (attempt && allowed && write && attempt.state.db === db) attempt.guard();
    const result = operation();
    db.exec('COMMIT');
    // Known read-only preflight may commit before any refusal. After refusal,
    // every commit invalidates its chain; any successful write always does.
    if (attempt && (write || attempt.refusal !== undefined)) attempt.invalid = true;
    return result;
  } catch (error) {
    let verified = false;
    try {
      if (attempt ? transactionState(db) === true : db.isTransaction) {
        // Only an actual native rollback can retain authority from a nested BEGIN refusal.
        if (attempt?.refusal) nativeExec.call(db, 'ROLLBACK'); else db.exec('ROLLBACK');
        verified = closed(db);
      }
      // An ancestor that was already closed did not prove an explicit rollback.
    } catch { /* Preserve the original error and invalidate its proof. */ }
    if (attempt && !verified) attempt.invalid = true;
    throw error;
  }
}
