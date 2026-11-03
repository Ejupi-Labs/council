import { WorkflowEngine } from './engine.js';
import { CouncilStore } from './store.js';
import type { AgentRunner, CouncilConfig, RunRecord, WorkflowInput } from './types.js';
export declare class CouncilService {
    readonly config: CouncilConfig;
    readonly store: CouncilStore;
    readonly runner: AgentRunner;
    readonly engine: WorkflowEngine;
    readonly instanceId: `${string}-${string}-${string}-${string}-${string}`;
    readonly recoveredRuns: RunRecord[];
    private readonly jobs;
    private readonly maintenance;
    private readonly heartbeat;
    private closePromise?;
    private closing;
    constructor(config: CouncilConfig, runner?: AgentRunner);
    start(value: unknown): RunRecord;
    run(input: WorkflowInput): Promise<RunRecord>;
    completion(id: string): Promise<RunRecord>;
    wait(id: string, timeoutMs?: number, signal?: AbortSignal): Promise<RunRecord>;
    cancel(id: string): RunRecord;
    continue(previousId: string, value: unknown): RunRecord;
    cleanup(id: string, options: {
        reviewed: true;
        discardChanges?: boolean;
    }): Promise<{
        removed: string[];
        alreadyRemoved: string[];
    }>;
    private cleanupWork;
    close(): Promise<void>;
}
