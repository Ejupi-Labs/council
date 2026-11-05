/** Fixed metadata only: cleanup uncertainty never proves a surviving orphan. */
export const PROCESS_CLEANUP_STAGES = ['agent_call', 'git_create', 'git_export', 'maintenance_export', 'maintenance_cleanup'] as const;
export type ProcessCleanupStage = typeof PROCESS_CLEANUP_STAGES[number];

export const QUARANTINE_PERSISTENCE_WARNING = 'The cleanup incident could not be persisted. Other hosts may be unaware; a shared dispatch block is not guaranteed.';
export const QUARANTINE_INSPECTION_REQUIRED = 'Subprocess cleanup was not verified in this workspace. Inspect OS processes and retained worktrees before accepting changes or starting more work.';

export interface ProcessCleanupIncidentInput {
  reportKey: string;
  instanceId: string;
  runId?: string;
  stage: ProcessCleanupStage;
}

export interface ProcessCleanupIncident extends ProcessCleanupIncidentInput {
  id: string;
  generation: string;
  reason: 'process_cleanup_unverified';
  observedAt: string;
  /** Historical Council PID captured by storage, never a current identity/liveness claim. */
  reportingPid?: number;
  acknowledgedAt?: string;
  acknowledgmentId?: string;
}

export interface DispatchQuarantineReceipt {
  blocked: boolean;
  /** Canonical decimal SQLite generation, preserving integers beyond JS precision. */
  currentRevision: string;
  openCount: number;
}

export interface DispatchQuarantinePage extends DispatchQuarantineReceipt {
  incidents: ProcessCleanupIncident[];
  nextCursor: string | null;
}

export interface DispatchQuarantineAcknowledgment {
  id: string;
  revision: string;
  acknowledgedAt: string;
  incidentCount: number;
  unownedUnfinishedRuns: number;
}

export interface DispatchQuarantineAcknowledgmentResult extends DispatchQuarantinePage {
  unownedUnfinishedRuns: number;
  acknowledgment?: DispatchQuarantineAcknowledgment;
}

/** Expected operational refusal; it does not imply unavailable or corrupt storage. */
export class DispatchQuarantinedError extends Error {
  readonly status = 409;
  readonly code = 'COUNCIL_DISPATCH_QUARANTINED';
  readonly receipt: Readonly<DispatchQuarantineReceipt & { blocked: true }>;

  constructor(receipt: DispatchQuarantineReceipt) {
    if (typeof receipt?.currentRevision !== 'string' || !/^(?:0|[1-9][0-9]{0,18})$/.test(receipt.currentRevision) || BigInt(receipt.currentRevision) > 9223372036854775807n
      || !Number.isSafeInteger(receipt.openCount) || receipt.openCount < 1) throw new Error('Invalid dispatch quarantine receipt.');
    super(`Workflow dispatch is quarantined by ${receipt.openCount} unresolved cleanup incident(s) at revision ${receipt.currentRevision}. ${QUARANTINE_INSPECTION_REQUIRED}`);
    this.name = 'DispatchQuarantinedError';
    this.receipt = Object.freeze({ blocked: true, currentRevision: receipt.currentRevision, openCount: receipt.openCount });
  }
}
