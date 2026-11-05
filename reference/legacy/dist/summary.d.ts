import type { EventRecord, RunRecord, RunStatus, WorkspaceArtifact } from './types.js';
export declare const MAX_RUN_SUMMARY_BYTES: number;
export declare const MAX_COMPACT_EVENT_DATA_BYTES: number;
/** Paging bounds response materialization, never the contents of an individual event. */
export declare const MAX_EVENT_PAGE_BYTES: number;
export interface RunSummary {
    id: string;
    status: RunStatus;
    mode: RunRecord['input']['mode'];
    agents: string[];
    createdAt: string;
    updatedAt: string;
    revision: number;
    cancellationRequested?: boolean;
    steps: Array<{
        id: string;
        agent: string;
        role: string;
        status: string;
        durationMs: number;
        error?: string;
    }>;
    output?: string;
    outputTruncated: boolean;
    error?: string;
    workspaces?: WorkspaceArtifact[];
    fullResultResource: string;
    summaryTruncated?: boolean;
    workspacesTruncated?: boolean;
}
export interface EventPage {
    events: EventRecord[];
    nextAfter: number;
    hasMore: boolean;
    nextCursor: number | null;
}
export interface PruneResult {
    candidateIds: string[];
    deletedRuns: number;
    deletedEvents: number;
    skippedArtifacts: number;
}
export declare function summarizeRun(run: RunRecord, revision?: number): RunSummary;
/** Event discovery never needs complete peer responses or repeated task/context briefs. */
export declare function summarizeEvent(event: EventRecord): EventRecord;
