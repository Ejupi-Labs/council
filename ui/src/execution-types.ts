import type { Policy } from './types.js';

export interface TaskSummary {
  id: string; projectId: string; currentVersion: string; revision: string; stateRevision: string;
  latestAttemptId: string | null; requestedMode: string; policySource: string; status: string; stage: string; nextAction: string;
}
export interface Task extends TaskSummary { originalText: string }
export interface TaskVersion {
  taskId: string; version: string; guidanceText: string; requestedPolicy: Policy; timeoutSeconds: number;
  pins: { model?: string }; acceptance: { deliverable: string; conditionsText: string; requiredEvidence: string[]; allowCandidatePreparation: boolean };
}
interface Fact { status: string; value: string | null; source: string | null; observedAt: string | null }
export interface Attempt {
  id: string; taskId: string; taskVersion: string; phase: string; outcome: string | null; revision: string;
  executionMode: string; deadlineAt: string; remainingSeconds: number; observedAt: string; settledAt: string | null;
  sourceSnapshotId: string | null; candidateId: string | null; responseArtifactId: string | null;
  evidenceStatus: string; authoredDelta: boolean;
  sourceSnapshot?: { baseCommit: string; branch: string | null; selectedRelativePath: string; manifestSha256: string };
  candidate?: { workspaceRef: string; hasDelta: boolean; hasIndexDelta: boolean; verification: string; integration: string };
  incident?: { id: string; code: string; message: string; createdAt: string };
  primaryFailure: { code: string; message: string } | null;
  stop: { acceptedAt: string } | null;
  cleanup: { state: string; scope: string; incidentId: string | null };
  requestedTarget: { model?: string };
  targetObservations: {
    provider: Fact; account: Fact; model: Fact; nativeSession: Fact;
    permissions: { requestedMode: string; configuredMode: string | null; nativeReportedMode: string | null; limitations: string[] };
    usage: { inputTokens: string | null; outputTokens: string | null; cachedInputTokens: string | null; source: string | null; observedAt: string | null; completeness: string };
  };
}
export interface TaskDetail { task: Task; version: TaskVersion; latestAttempt: Attempt | null; acceptance: { responseArtifactId: string; acceptedAt: string } | null }
export interface Admission { task: TaskSummary; version: TaskVersion; attempt: Attempt }
export interface Artifact { id: string; attemptId: string; kind: string; byteLength: string; closed: boolean; completeness: string; sha256: string | null; createdAt: string }
export interface ArtifactChunk { artifactId: string; offset: string; nextOffset: string; through: string; bytesBase64: string; eof: boolean; streamClosed: boolean; completeness: string; sha256: string | null }
export interface AttemptEvent { sequence: string; kind: string; observedAt: string; metadata: unknown }
