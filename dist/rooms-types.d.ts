import type { CouncilConfig, Permission, ReasoningEffort } from './types.js';
export type RoomStatus = 'idle' | 'discussing' | 'ready' | 'running' | 'completed' | 'failed' | 'cancelled' | 'needs_attention';
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
}
export interface RoomRecord {
    instructionRevision?: number;
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
