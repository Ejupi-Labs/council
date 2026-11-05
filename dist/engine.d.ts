import type { AgentRunner, CouncilConfig, RunRecord, WorkflowInput, WorkflowPlan } from './types.js';
import { DispatchQuarantinedError, type ProcessCleanupIncident, type ProcessCleanupIncidentInput } from './dispatch-quarantine.js';
import type { PrivateChatManager } from './private-chats.js';
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
    releaseSlotWithRetry?(token: string, instanceId: string): void | Promise<void>;
    finalizeRun?(run: RunRecord): RunRecord;
    reportProcessCleanupIncident?(input: ProcessCleanupIncidentInput): ProcessCleanupIncident;
    assertDispatchUnblocked?(): void;
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
    private processCleanupFailure?;
    private incidentPersistenceFailed;
    private foreignQuarantine?;
    constructor(config: CouncilConfig, runner: AgentRunner, store: WorkflowStore, options?: {
        instanceId?: string;
        artifactTimeoutMs?: number;
        cancellationArtifactTimeoutMs?: number;
        onStateFailure?: (error: Error) => void;
        onProcessCleanupFailure?: (error: Error) => void;
        onIncidentPersistenceFailure?: (error: Error) => void;
        assertAgentDispatch?: (agent: string) => void;
        privateChats?: () => Pick<PrivateChatManager, 'create' | 'send' | 'page'>;
    });
    /** Stop this host after a trusted owned-process cleanup failure. Inspection and a fresh host are required. */
    fenceProcessCleanup(): void;
    /** Fence from an existing workspace incident without reporting a new owned failure. */
    fenceDispatchQuarantine(error: DispatchQuarantinedError): void;
    /** Retain failed mandatory incident persistence without reporting another incident. */
    noteIncidentPersistenceFailure(): void;
    private cleanupFailureMessage;
    plan(input: WorkflowInput, options?: {
        resolvedSettings?: true;
    }): WorkflowPlan;
    run(input: WorkflowInput, options?: {
        id?: string;
        signal?: AbortSignal;
        resolvedSettings?: true;
        privateChatRoomId?: string;
    }): Promise<RunRecord>;
}
