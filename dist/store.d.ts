import type { ContextEntry, EventRecord, Lease, Message, RunRecord } from './types.js';
/** Durable collaboration state shared by all Council processes in one workspace. */
export declare class CouncilStore {
    private readonly db;
    constructor(stateDir: string);
    private transaction;
    saveRun(run: RunRecord): void;
    getRun(id: string): RunRecord | undefined;
    listRuns(limit?: number): RunRecord[];
    appendEvent(runId: string, type: string, data: unknown): EventRecord;
    events(runId: string, after?: number): EventRecord[];
    sendMessage(input: {
        from: string;
        to: string;
        subject: string;
        body: string;
        runId?: string;
    }): Message;
    inbox(agent: string, unreadOnly?: boolean): Message[];
    acknowledge(id: string, agent: string): boolean;
    reserve(owner: string, paths: string[], ttlMs: number, runId?: string): Lease[];
    release(owner: string, paths?: string[]): number;
    private readLeases;
    leases(): Lease[];
    putContext(key: string, value: string, author: string): ContextEntry;
    getContext(key: string): ContextEntry | undefined;
    listContext(): ContextEntry[];
    registerInstance(instanceId: string, pid: number): void;
    heartbeat(instanceId: string): boolean;
    unregisterInstance(instanceId: string): void;
    claimRun(runId: string, instanceId: string): void;
    requestCancellation(runId: string): boolean;
    /** Final completion and a concurrent cancellation request are ordered by one SQLite transaction. */
    finalizeRun(run: RunRecord): RunRecord;
    acquireSlot(instanceId: string, runId: string, limit: number): string | undefined;
    releaseSlot(token: string, instanceId: string): void;
    /** Heartbeat age alone is never proof that a process is dead. PID reuse is intentionally conservative. */
    recoverOrphans(): RunRecord[];
    private instance;
    private assertInstance;
    private ownedRun;
    private assertRunOwner;
    private legacyCancellation;
    private recoverDeadOwners;
    close(): void;
}
