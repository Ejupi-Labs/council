import { WorkflowEngine } from './engine.js';
import { CouncilStore } from './store.js';
import { type EventPage, type RunSummary } from './summary.js';
import { RoomManager } from './rooms.js';
import { DelegationManager } from './delegation.js';
import { PrivateChatManager } from './private-chats.js';
import { BillingManager } from './billing.js';
import type { RoutingInput } from './routing-types.js';
import type { AgentRunner, CouncilConfig, RunRecord, WorkflowInput, WorkspaceArtifact } from './types.js';
export declare class CouncilService {
    readonly config: CouncilConfig;
    /** Retry local state initialization only; no workflow or provider call is replayed. */
    static open(config: CouncilConfig, runner?: AgentRunner, signal?: AbortSignal): Promise<CouncilService>;
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
    private lastHeartbeatElapsed;
    private heartbeatContentions;
    private closePromise?;
    private closing;
    private readonly shutdownWaits;
    private roomManager?;
    private delegationManager?;
    private billingManager?;
    private privateChatManager?;
    private dashboard?;
    get rooms(): RoomManager;
    get billing(): BillingManager;
    get delegation(): DelegationManager;
    get privateChats(): PrivateChatManager;
    billingView(): {
        agents: {
            alias: string;
            enabled: boolean;
            profile: string;
            model: string | null;
            revision: number;
            mode: import("./billing-types.js").BillingMode;
            accountKey: string | null;
            plan: string | null;
            source: import("./billing-types.js").BillingSource | null;
            observedAt: string | null;
            validUntil: string | null;
            stale: boolean;
            windows: import("./billing-types.js").BillingWindow[];
            prices: import("./billing-types.js").BillingPrices | null;
            routing: import("./billing-types.js").BillingRouting | null;
            skills: string[] | null;
            stopAtSubscriptionLimit: boolean;
            subscriptionWindowIds: string[] | null;
            subscriptionExhausted: Record<string, string>;
            effectiveSkills: string[];
            availability: import("./billing-types.js").BillingAvailability;
            subscriptionProtection: import("./billing-types.js").SubscriptionProtection;
        }[];
        skills: ({
            id: "dont-kill-an-ant-with-a-bazooka";
            name: "Don't kill an ant with a bazooka";
            description: 'Right-size models, effort and collaboration to the task while preserving verification and explicit model choices.';
        } | {
            id: "council-implementation-review";
            name: 'Implementation and review';
            description: 'Separate implementation, independent review and revision for work that benefits from a second perspective.';
        } | {
            id: "council-independent-perspectives";
            name: 'Independent perspectives';
            description: 'Keep parallel analyses independent and make disagreements and evidence explicit.';
        } | {
            id: "council-advisor";
            name: 'Focused advisor';
            description: 'Use a focused read-only consultation when a specific uncertainty warrants another model.';
        })[];
    };
    routePlan(input: RoutingInput): import("./routing-types.js").RoutingPlan;
    routeRun(input: RoutingInput): {
        run: RunRecord;
        selection: import("./routing-types.js").RoutingPlan;
    };
    openDashboard(port?: number): Promise<{
        url: string;
    }>;
    constructor(config: CouncilConfig, runner?: AgentRunner);
    private heartbeatTick;
    start(value: unknown, provenance?: {
        previousId?: string;
        routing?: unknown;
        roomId?: string;
    }): RunRecord;
    run(input: WorkflowInput): Promise<RunRecord>;
    completion(id: string): Promise<RunRecord>;
    /** Prefer an honest failed in-memory outcome if durable finalization was unavailable. */
    getRun(id: string): RunRecord | undefined;
    /** Test workflow identity without loading complete prompts, results or artifacts. */
    hasRun(id: string): boolean;
    private degrade;
    private summary;
    waitSummary(id: string, timeoutMs?: number, signal?: AbortSignal): Promise<RunSummary>;
    wait(id: string, timeoutMs?: number, signal?: AbortSignal): Promise<RunRecord>;
    /** Await results inside the server; elapsed idle time does not dispatch a model. */
    awaitRuns(ids: string[], timeoutMs?: number, signal?: AbortSignal): Promise<{
        runs: RunSummary[];
        timedOut: boolean;
        waitingInServer: true;
    }>;
    /** Full selected answers and step metadata, each taken from one consistent run snapshot. */
    awaitResults(ids: string[], timeoutMs?: number, signal?: AbortSignal): Promise<{
        waitingInServer: true;
        runs: {
            id: string;
            status: import("./types.js").RunStatus;
            error: string | undefined;
            output: string | undefined;
            outputTruncated: false;
            steps: {
                id: string;
                agent: string;
                role: string;
                status: import("./types.js").ResultStatus;
                durationMs: number;
                error: string | undefined;
            }[];
            fullResultResource: string;
        }[];
        timedOut: boolean;
    }>;
    /** A bounded read-only consultation, independent of the requester's harness. */
    advise(input: {
        requester: string;
        advisor: string;
        question: string;
        context?: string;
        model?: string;
        effort?: import('./types.js').ReasoningEffort;
    }): RunRecord;
    health(checkIntegrity?: boolean): {
        status: string;
        version: string;
        pid: number;
        uptimeSeconds: number;
        lastHeartbeatAt: string;
        heartbeatContentions: number;
        filesystem: {
            pending: number;
            limit: number;
        };
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
            heartbeatGraceMs: number;
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
                slotWaiters: number;
                context: number;
            };
            runStatuses: Record<string, number>;
        } | undefined;
        integrity: string[] | undefined;
        databases: import("./state-snapshot.js").StateFileHealth[];
        storageBytes: number;
        issues: string[];
    };
    backup(path: string, signal?: AbortSignal): Promise<string>;
    /** Back up every present collaboration database; file artifacts require a separate copy. */
    snapshot(path: string, signal?: AbortSignal): Promise<{
        destination: string;
        manifest: import("./state-snapshot.js").StateSnapshotManifest;
    }>;
    prune(options: Parameters<CouncilStore['prune']>[0]): import("./summary.js").PruneResult;
    private trackMaintenance;
    runPage(limit?: number, cursor?: string): {
        runs: RunSummary[];
        nextCursor: string | null;
    };
    /** Read complete retained events without scheduling or changing workflow state. */
    eventPage(id: string, after?: number, limit?: number, type?: string): EventPage;
    inboxPage(agent: string, unreadOnly?: boolean, limit?: number, cursor?: string): {
        messages: import("./types.js").Message[];
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
