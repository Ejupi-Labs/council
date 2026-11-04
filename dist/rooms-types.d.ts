import type { CouncilConfig, Permission, ReasoningEffort, WorkspaceArtifact } from './types.js';
export type RoomStatus = 'idle' | 'discussing' | 'ready' | 'running' | 'paused' | 'completed' | 'failed' | 'cancelled' | 'needs_attention';
export type RoomCheckpoint = {
    phase: 'discussion';
    nextRound: number;
} | {
    phase: 'execution';
} | {
    phase: 'ready';
};
export interface RoomMember {
    agent: string;
    role: 'lead' | 'worker' | 'advisor';
    weight: number;
    model?: string;
    effort?: ReasoningEffort;
    advisor?: string;
    advisorMaxRequests?: number;
    skills?: string[];
}
export type RoomMemberInput = Omit<RoomMember, 'weight'> & {
    weight?: number;
};
export interface RoomTaskInput {
    title: string;
    brief: string;
    permission: Permission;
    dependsOn?: string[];
}
export interface RoomTask extends RoomTaskInput {
    id: string;
    status: 'pending' | 'assigned' | 'running' | 'completed' | 'failed' | 'cancelled';
    assignee?: string;
    runId?: string;
    /** Exact retained workflow artifact metadata; an empty array records a run without worktrees. */
    workspaces?: WorkspaceArtifact[];
    resultPreview?: string;
    error?: string;
}
export interface RoomMessage {
    id: string;
    author: string;
    kind: 'human' | 'opinion' | 'decision' | 'task' | 'advisor' | 'system';
    body: string;
    createdAt: string;
    runId?: string;
}
export interface RoomDecision {
    instructionRevision?: number;
    approved: boolean;
    approveWeight: number | string;
    totalWeight: number | string;
    assignments: Record<string, string>;
    rationale: string;
    dissent: string[];
}
export interface RoomInput {
    title: string;
    brief: string;
    members: RoomMemberInput[];
    tasks?: RoomTaskInput[];
    rounds?: number;
    maxCalls?: number;
    autoExecute?: boolean;
    contextLimit?: number | null;
    /** Optional per-task private consultation quota; omitted means disabled. */
    privateChatMaxRequests?: number;
}
export interface RoomRecord {
    privateChatMaxRequests?: number;
    instructionRevision?: number;
    /** A request drains already admitted workflows before releasing ownership. */
    pauseRequested?: boolean;
    /** Durable dispatch continuation; completed workflows are never replayed. */
    checkpoint?: RoomCheckpoint;
    id: string;
    backend?: CouncilConfig['backend'];
    title: string;
    brief: string;
    status: RoomStatus;
    members: RoomMember[];
    tasks: RoomTask[];
    messages: RoomMessage[];
    rounds: number;
    maxCalls: number;
    autoExecute: boolean;
    contextLimit: number | null;
    callsUsed: number;
    decision?: RoomDecision;
    error?: string;
    revision: number;
    createdAt: string;
    updatedAt: string;
}
export interface RoomSummary {
    id: string;
    backend?: CouncilConfig['backend'];
    title: string;
    status: RoomStatus;
    taskCount: number;
    completedTasks: number;
    callsUsed: number;
    maxCalls: number;
    revision: number;
    updatedAt: string;
}
/** Scalar coordination state; excludes task briefs, artifact records and chat history. */
export interface RoomObservation {
    id: string;
    title: string;
    status: RoomStatus;
    revision: number;
    instructionRevision: number;
    taskCount: number;
    completedTasks: number;
    failedTasks: number;
    callsUsed: number;
    maxCalls: number;
    error?: string;
    pauseRequested?: boolean;
}
