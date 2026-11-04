import type { AgentRunner, CouncilConfig, RunRecord, WorkflowInput, WorkflowPlan } from './types.js';
/** Legacy sizing constants retained for callers; these no longer limit workflow inputs. */
export declare const MAX_TASK_CHARS = 32000;
export declare const MAX_CONTEXT_CHARS = 64000;
export interface WorkflowStore {
    saveRun(run: RunRecord): void;
    getRun?(runId: string): RunRecord | undefined;
    appendEvent(runId: string, type: string, data: unknown): unknown;
    events?(runId: string, after?: number): Array<{
        type: string;
        sequence?: number;
    }>;
    isCancellationRequested?(runId: string): boolean;
    acquireSlot?(instanceId: string, runId: string, limit: number, waiterId?: string): string | undefined;
    cancelSlotWaiter?(instanceId: string, waiterId: string): void;
    releaseSlot?(token: string, instanceId: string): void;
    finalizeRun?(run: RunRecord): RunRecord;
}
/** Retain the last in-memory evidence when the durable terminal write fails. */
export declare class WorkflowPersistenceError extends Error {
    readonly run: RunRecord;
    constructor(run: RunRecord, cause: unknown);
}
export declare class WorkflowEngine {
    private readonly config;
    private readonly runner;
    private readonly store;
    private readonly options;
    private readonly semaphore;
    private readonly workspaces;
    constructor(config: CouncilConfig, runner: AgentRunner, store: WorkflowStore, options?: {
        instanceId?: string;
        artifactTimeoutMs?: number;
        cancellationArtifactTimeoutMs?: number;
        onStateFailure?: (error: Error) => void;
    });
    plan(input: WorkflowInput): WorkflowPlan;
    run(input: WorkflowInput, options?: {
        id?: string;
        signal?: AbortSignal;
    }): Promise<RunRecord>;
}
