import type { RoomStatus } from './rooms-types.js';

export interface DelegationInput {
  mode: 'human' | 'agent';
  delegate: string;
  mandate: string;
  roomIds: string[];
  expectedRevision: number;
}
export interface DelegationConfiguration extends Omit<DelegationInput, 'expectedRevision'> {
  revision: number;
  updatedAt: string;
}
export interface DelegationObservation {
  id: string;
  revision: number;
  instructionRevision: number;
  status: RoomStatus;
}
export interface DelegationReportInput {
  author: string;
  kind: 'update' | 'decision' | 'blocked' | 'completion';
  body: string;
  roomIds?: string[];
  /** Optional revisions actually reviewed by the reporting agent; stale observations are rejected. */
  expectedRoomRevisions?: Record<string, number>;
}
export interface DelegationReport extends Omit<DelegationReportInput, 'roomIds' | 'expectedRoomRevisions'> {
  id: string;
  sequence: number;
  revision: number;
  roomIds: string[];
  /** Captured when the report is submitted; not a claim that its author reviewed a revision. */
  observedRooms: DelegationObservation[];
  createdAt: string;
  resolution?: { author: string; body: string; createdAt: string };
}
export interface DelegationResolutionInput {
  author: string;
  body: string;
  expectedRevision: number;
}
export interface DelegationCursor {
  workspaceId: string;
  configurationRevision: number;
  reportRevision: number;
  rooms: Record<string, { revision: number; instructionRevision: number }>;
}
export interface DelegationRoom {
  id: string;
  title?: string;
  revision: number;
  instructionRevision: number;
  status?: RoomStatus;
  missing?: boolean;
  error?: string;
  taskCount?: number;
  completedTasks?: number;
  failedTasks?: number;
  callsUsed?: number;
  maxCalls?: number;
  pauseRequested?: boolean;
  actionable: boolean;
}
export interface DelegationWatchInput {
  cursor?: DelegationCursor;
  timeoutMs?: number;
}
export interface DelegationWatchResult {
  reason: 'changed' | 'timeout';
  configuration: DelegationConfiguration;
  rooms: DelegationRoom[];
  openQuestions: number;
  cursor: DelegationCursor;
}
export interface DelegationReportsPage {
  reports: DelegationReport[];
  nextCursor: number | null;
}
export interface DelegationChangesPage {
  /** Latest retained state of reports changed after the supplied revision, including human replies. */
  changes: { revision: number; report: DelegationReport }[];
  nextCursor: number | null;
}
