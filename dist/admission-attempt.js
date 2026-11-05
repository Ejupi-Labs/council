import { DatabaseSync } from 'node:sqlite';
const connections = new WeakMap();
const registered = new WeakMap();
const attempts = new WeakMap();
const refusals = new WeakMap();
const scopes = [];
let newestNonce, finishedNonce;
const nativeExec = DatabaseSync.prototype.exec;
function transactionState(db) {
    try {
        const descriptor = Object.getOwnPropertyDescriptor(db, 'isTransaction');
        return descriptor?.configurable === false && typeof descriptor.get === 'function' ? descriptor.get.call(db) : undefined;
    }
    catch {
        return undefined;
    }
}
function closed(db) { return transactionState(db) === false; }
export function admissionConnection(db, role) {
    const previous = registered.get(db);
    if (previous) {
        if (connections.get(previous).role !== role)
            throw new Error('Admission connection role changed.');
        return previous;
    }
    const handle = Object.freeze({});
    connections.set(handle, { db, role });
    registered.set(db, handle);
    return handle;
}
export function newAdmissionAttempt(room, state, guard) {
    const a = connections.get(room), b = connections.get(state);
    if (!a || !b || a.role !== 'rooms' || b.role !== 'state' || a.db === b.db)
        throw new Error('Invalid admission connection pair.');
    const handle = Object.freeze({});
    const attempt = { nonce: Symbol('admission-attempt'), room: a, state: b, guard, active: false, ended: false, used: false, invalid: false, didThrow: false };
    attempts.set(handle, attempt);
    newestNonce = attempt.nonce;
    return handle;
}
/** A single call-slot admission needs only its exact state connection. */
export function newStateAdmissionAttempt(state, guard) {
    const connection = connections.get(state);
    if (!connection || connection.role !== 'state')
        throw new Error('Invalid state admission connection.');
    const handle = Object.freeze({});
    const attempt = { nonce: Symbol('admission-attempt'), state: connection, guard, active: false, ended: false, used: false, invalid: false, didThrow: false };
    attempts.set(handle, attempt);
    newestNonce = attempt.nonce;
    return handle;
}
export function runAdmissionAttempt(handle, operation) {
    const attempt = attempts.get(handle);
    if (!attempt || attempt.active || attempt.ended)
        throw new Error('Admission attempt was already used.');
    attempt.active = true;
    scopes.push(attempt);
    try {
        const result = operation();
        attempt.invalid = true;
        return result;
    }
    catch (error) {
        attempt.didThrow = true;
        attempt.thrown = error;
        throw error;
    }
    finally {
        if (scopes.pop() !== attempt)
            attempt.invalid = true;
        attempt.active = false;
        attempt.ended = true;
        finishedNonce = attempt.nonce;
        if (attempt.refusal)
            queueMicrotask(() => { attempt.invalid = true; });
    }
}
export function consumeAdmissionRefusal(handle, error) {
    const attempt = attempts.get(handle);
    if (!attempt || attempt.used)
        return false;
    attempt.used = true;
    return scopes.length === 0 && newestNonce === attempt.nonce && finishedNonce === attempt.nonce && attempt.ended && !attempt.active && !attempt.invalid && attempt.didThrow && attempt.thrown === error
        && typeof error === 'object' && error !== null && attempt.refusal === error && refusals.get(error)?.attempt === attempt
        && (!attempt.room || closed(attempt.room.db)) && closed(attempt.state.db);
}
/** Fixed diagnostic metadata only; this never grants retry authority. */
export function admissionRefusalOrigin(error) {
    const proof = typeof error === 'object' && error !== null ? refusals.get(error) : undefined;
    return proof ? { area: 'state', phase: 'begin', code: proof.code } : undefined;
}
export function admissionTransaction(db, operation, write = true) {
    const attempt = scopes.at(-1), allowed = attempt && (attempt.room?.db === db || attempt.state.db === db);
    const initiallyClosed = Boolean(attempt && allowed && write && attempt.state.db === db) && closed(db);
    if (attempt && !allowed)
        attempt.invalid = true;
    if (attempt && allowed && write)
        attempt.guard();
    try {
        // Instance exec wrappers are not an authority for minting a BEGIN refusal.
        if (attempt && allowed && write)
            nativeExec.call(db, 'BEGIN IMMEDIATE');
        else
            db.exec(write ? 'BEGIN IMMEDIATE' : 'BEGIN');
    }
    catch (error) {
        const code = error instanceof Error && 'code' in error && error.code === 'ERR_SQLITE_ERROR' && 'errcode' in error ? error.errcode : undefined;
        if (attempt && allowed && write && attempt.state.db === db && initiallyClosed && !attempt.invalid && closed(db)
            && typeof code === 'number' && Number.isInteger(code) && [5, 6].includes(code & 0xff) && typeof error === 'object' && error !== null) {
            if (attempt.refusal)
                attempt.invalid = true;
            else {
                attempt.refusal = error;
                refusals.set(error, { attempt, code });
            }
        }
        throw error;
    }
    try {
        // A blocking BEGIN may have outlived the per-launch contention window.
        if (attempt && allowed && write && attempt.state.db === db)
            attempt.guard();
        const result = operation();
        db.exec('COMMIT');
        // Known read-only preflight may commit before any refusal. After refusal,
        // every commit invalidates its chain; any successful write always does.
        if (attempt && (write || attempt.refusal !== undefined))
            attempt.invalid = true;
        return result;
    }
    catch (error) {
        let verified = false;
        try {
            if (attempt ? transactionState(db) === true : db.isTransaction) {
                // Only an actual native rollback can retain authority from a nested BEGIN refusal.
                if (attempt?.refusal)
                    nativeExec.call(db, 'ROLLBACK');
                else
                    db.exec('ROLLBACK');
                verified = closed(db);
            }
            // An ancestor that was already closed did not prove an explicit rollback.
        }
        catch { /* Preserve the original error and invalidate its proof. */ }
        if (attempt && !verified)
            attempt.invalid = true;
        throw error;
    }
}
//# sourceMappingURL=admission-attempt.js.map