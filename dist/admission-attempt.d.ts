import { DatabaseSync } from 'node:sqlite';
declare const connectionBrand: unique symbol;
declare const attemptBrand: unique symbol;
export type AdmissionConnection = {
    readonly [connectionBrand]: true;
};
export type AdmissionAttempt = {
    readonly [attemptBrand]: true;
};
type Connection = {
    db: DatabaseSync;
    role: 'state' | 'rooms';
};
export declare function admissionConnection(db: DatabaseSync, role: Connection['role']): AdmissionConnection;
export declare function newAdmissionAttempt(room: AdmissionConnection, state: AdmissionConnection, guard: () => void): AdmissionAttempt;
export declare function runAdmissionAttempt<T>(handle: AdmissionAttempt, operation: () => T): T;
export declare function consumeAdmissionRefusal(handle: AdmissionAttempt, error: unknown): boolean;
/** Fixed diagnostic metadata only; this never grants retry authority. */
export declare function admissionRefusalOrigin(error: unknown): {
    area: 'state';
    phase: 'begin';
    code: number;
} | undefined;
export declare function admissionTransaction<T>(db: DatabaseSync, operation: () => T, write?: boolean): T;
export {};
