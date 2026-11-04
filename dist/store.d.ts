import type { ContextEntry, EventRecord, Lease, Message, RunRecord, WorkspaceArtifact } from './types.js';
import { type EventPage, type PruneResult, type RunSummary } from './summary.js';
export declare class RunEventsUnavailableError extends Error {
    readonly status = 404;
    readonly code = "COUNCIL_RUN_EVENTS_UNAVAILABLE";
    constructor();
}
/** Durable collaboration state shared by all Council processes in one workspace. */
export declare class CouncilStore {
    private readonly db;
    private readonly databasePath;
    private readonly sqliteVersion;
    private readonly statements;
    private boundInstance?;
    private lastRecoveryAt;
    private activeBackups;
    constructor(stateDir: string);
    /** WAL lock upgrades can bypass SQLite's busy handler; retry only startup SQL within one deadline. */
    private retryStartup;
    /** SQLite inherits the database mode when creating WAL/SHM beside existing state directories. */
    private restrictDatabaseFiles;
    /** Fixed SQL only, with a bounded cache rather than re-preparing every polling query. */
    private statement;
    private migrateMetadata;
    private transaction;
    saveRun(run: RunRecord): void;
    /** Creation, ownership and the queued event commit together, leaving no unowned crash window. */
    createRun(run: RunRecord, instanceId: string, initialEvents?: Array<{
        type: string;
        data: unknown;
    }>): void;
    getRun(id: string): RunRecord | undefined;
    /** Read only the indexed identity; callers do not need a workflow transcript. */
    hasRun(id: string): boolean;
    listRuns(limit?: number): RunRecord[];
    getRunSummary(id: string): RunSummary | undefined;
    listRunSummaries(limit?: number, before?: {
        updatedAt: string;
        id: string;
    }): RunSummary[];
    appendEvent(runId: string, type: string, data: unknown): EventRecord;
    private writeEvent;
    events(runId: string, after?: number): EventRecord[];
    /** Complete event payloads, with indexed exact-type filtering and stable ascending cursors. */
    eventPage(runId: string, after?: number, limit?: number, type?: string): EventPage;
    compactEvents(runId: string, after?: number, limit?: number): EventRecord[];
    isCancellationRequested(runId: string): boolean;
    sendMessage(input: {
        from: string;
        to: string;
        subject: string;
        body: string;
        runId?: string;
    }): Message;
    inbox(agent: string, unreadOnly?: boolean): Message[];
    /** Complete message bodies with stable timestamp/ID keyset pagination. */
    inboxPage(agent: string, unreadOnly?: boolean, limit?: number, after?: {
        createdAt: string;
        id: string;
    }): Message[];
    private decodeMessages;
    acknowledge(id: string, agent: string): boolean;
    reserve(owner: string, paths: string[], ttlMs: number, runId?: string, instanceId?: string): Lease[];
    release(owner: string, paths?: string[], runId?: string): number;
    private readLeases;
    leases(): Lease[];
    putContext(key: string, value: string, author: string): ContextEntry;
    getContext(key: string): ContextEntry | undefined;
    listContext(): ContextEntry[];
    registerInstance(instanceId: string, pid: number, slotLimit?: number): void;
    bindInstance(instanceId: string): void;
    heartbeat(instanceId: string): boolean;
    unregisterInstance(instanceId: string): void;
    claimRun(runId: string, instanceId: string): void;
    requestCancellation(runId: string): boolean;
    /** Final completion and a concurrent cancellation request are ordered by one SQLite transaction. */
    finalizeRun(run: RunRecord): RunRecord;
    /** Explicit repair metadata only; the caller has already verified and exported the managed worktree. */
    updateArtifact(runId: string, artifact: WorkspaceArtifact): RunRecord;
    /** Caller holds the write transaction and has checked its narrow mutation authority. */
    private writeRun;
    /** Stable per-call IDs reserve free capacity for older durable waiters across processes. */
    acquireSlot(instanceId: string, runId: string, limit: number, waiterId?: string): string | undefined;
    releaseSlot(token: string, instanceId: string): void;
    /** Idempotent cleanup for a per-call wait that was aborted before admission. */
    cancelSlotWaiter(instanceId: string, waiterId: string): void;
    /** Heartbeat age alone is never proof that a process is dead. PID reuse is intentionally conservative. */
    recoverOrphans(): RunRecord[];
    private instance;
    private assertInstance;
    private ownedRun;
    private assertRunOwner;
    private legacyCancellation;
    private recoverDeadOwners;
    health(): {
        schemaVersion: number;
        sqliteVersion: string;
        journalMode: string;
        databaseBytes: number;
        walBytes: number;
        preparedStatements: number;
        workspaceActiveRunLimit: number;
        counts: {
            runs: number;
            activeRuns: number;
            events: number;
            messages: number;
            unreadMessages: number;
            leases: number;
            instances: number;
            slots: number;
            slotWaiters: number;
            context: number;
        };
        runStatuses: Record<string, number>;
    };
    checkIntegrity(): string[];
    /** Online SQLite backup, published atomically without ever opening the requested destination for writing. */
    backupTo(path: string, signal?: AbortSignal): Promise<string>;
    prune(options: {
        before: string;
        keepLatest?: number;
        dryRun?: boolean;
    }): PruneResult;
    close(): void;
}
