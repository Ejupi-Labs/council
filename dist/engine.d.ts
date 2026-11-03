import type { AgentRunner, CouncilConfig, RunRecord, WorkflowInput, WorkflowPlan } from './types.js';
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
    acquireSlot?(instanceId: string, runId: string, limit: number): string | undefined;
    releaseSlot?(token: string, instanceId: string): void;
    finalizeRun?(run: RunRecord): RunRecord;
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
    });
    plan(input: WorkflowInput): WorkflowPlan;
    run(input: WorkflowInput, options?: {
        id?: string;
        signal?: AbortSignal;
    }): Promise<RunRecord>;
}
