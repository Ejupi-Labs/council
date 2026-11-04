import type { CouncilConfig } from './types.js';
import type { BillingAvailability, BillingInput, BillingRecord } from './billing-types.js';
export declare class BillingRevisionConflict extends Error {
    readonly currentRevision: number;
    readonly code = "COUNCIL_BILLING_REVISION_CONFLICT";
    readonly status = 409;
    constructor(currentRevision: number);
}
/** User-supplied billing observations only: no credential access, vendor probes, or estimated spending. */
export declare class BillingManager {
    private readonly config;
    private readonly db;
    private closed;
    constructor(config: CouncilConfig, stateDir?: string);
    private open;
    private known;
    private transaction;
    private row;
    private account;
    private revision;
    get(alias: string): BillingRecord;
    list(): BillingRecord[];
    update(alias: string, input: BillingInput, expectedRevision?: number): BillingRecord;
    importSnapshot(alias: string, input: BillingInput, expectedRevision?: number): BillingRecord;
    private write;
    remove(alias: string, expectedRevision?: number): BillingRecord;
    availability(alias: string, now?: Date | number | string): BillingAvailability;
    private evaluate;
    close(): void;
}
