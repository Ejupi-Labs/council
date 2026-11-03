import { WorkflowEngine } from './engine.js';
import { CouncilStore } from './store.js';
import { type RunSummary } from './summary.js';
import type { AgentRunner, CouncilConfig, RunRecord, WorkflowInput, WorkspaceArtifact } from './types.js';
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
    private readonly maintenanceControllers;
    private readonly emergencyResults;
    private fault?;
    private lastHeartbeatAt;
    private closePromise?;
    private closing;
    constructor(config: CouncilConfig, runner?: AgentRunner);
    start(value: unknown, provenance?: {
        previousId: string;
    }): RunRecord;
    run(input: WorkflowInput): Promise<RunRecord>;
    completion(id: string): Promise<RunRecord>;
    /** Prefer an honest failed in-memory outcome if durable finalization was unavailable. */
    getRun(id: string): RunRecord | undefined;
    private degrade;
    private summary;
    waitSummary(id: string, timeoutMs?: number, signal?: AbortSignal): Promise<RunSummary>;
    wait(id: string, timeoutMs?: number, signal?: AbortSignal): Promise<RunRecord>;
    health(checkIntegrity?: boolean): {
        status: string;
        version: string;
        pid: number;
        uptimeSeconds: number;
        lastHeartbeatAt: string;
        activeLocalRuns: number;
        maintenanceOperations: number;
        emergencyResults: number;
        memory: NodeJS.MemoryUsage;
        limits: {
            concurrency: number;
            activeLocalRuns: number;
            activeWorkspaceRuns: number;
            callsPerRun: number;
            outputBytesPerCall: number;
        };
        state: {
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
                context: number;
            };
            runStatuses: Record<string, number>;
        } | undefined;
        integrity: string[] | undefined;
        issues: string[];
    };
    backup(path: string, signal?: AbortSignal): Promise<string>;
    prune(options: Parameters<CouncilStore['prune']>[0]): import("./summary.js").PruneResult;
    private trackMaintenance;
    runPage(limit?: number, cursor?: string): {
        runs: RunSummary[];
        nextCursor: string | null;
    };
    requestCancellation(id: string): {
        requested: boolean;
        run: RunSummary;
    };
    cancel(id: string): RunRecord;
    continue(previousId: string, value: unknown): RunRecord;
    exportArtifacts(id: string, signal?: AbortSignal): Promise<{
        runId: string;
        artifacts: WorkspaceArtifact[];
    }>;
    private exportArtifactsWork;
    cleanup(id: string, options: {
        reviewed: true;
        discardChanges?: boolean;
    }, signal?: AbortSignal): Promise<{
        removed: string[];
        alreadyRemoved: string[];
    }>;
    private cleanupWork;
    close(): Promise<void>;
}
