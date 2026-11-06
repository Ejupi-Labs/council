import type { Policy } from './types.js';

export interface TaskSummary {
  id: string; projectId: string; currentVersion: string; revision: string; stateRevision: string;
  latestAttemptId: string | null; requestedMode: string; policySource: string; status: string; stage: string; nextAction: string;
}
export interface Task extends TaskSummary { originalText: string }
export interface TaskVersion {
  taskId: string; version: string; guidanceText: string; requestedPolicy: Policy; timeoutSeconds: number;
  pins: ExecutionPins; acceptance: { deliverable: string; conditionsText: string; requiredEvidence: string[]; allowCandidatePreparation: boolean };
}
export interface ExecutionPins { harnessId?: string; model?: string; accountId?: string; paymentPath?: string }
export interface Fact { status: 'observed' | 'unknown'; value: string | null; source: string | null; observedAt: string | null }
export interface UsageObservation {
  inputTokens: string | null; outputTokens: string | null; cachedInputTokens: string | null;
  cacheCreationInputTokens?: string | null; scope?: 'top_level' | 'whole_tree' | 'unknown';
  source: string | null; observedAt: string | null; completeness: string;
}
export interface NativeModelUsage {
  model: string; inputTokens: string | null; outputTokens: string | null; cachedInputTokens: string | null;
  cacheCreationInputTokens: string | null; costEstimateUsd: string | null;
}
export interface NativeModelsObservation {
  scope: 'whole_tree'; source: string; observedAt: string; completeness: 'complete' | 'partial' | 'unknown';
  evidenceArtifactIds: string[]; omittedModelCount: string | null; models: NativeModelUsage[];
}
export interface ConnectionCapability {
  connectionId: string; harnessId: string; interface: 'native_exec'; nativeContract: string;
  minimumVersion: string | null; documentedVersion: string | null; versionSource: string;
  supportedPolicyModes: string[]; modelSelection: string; accountSelection: string;
  sessionContinuity: string; sessionPersistence: string; cancellationScope: string;
  liveQualification: string; fakeMode: boolean; limits: Record<string, number>;
}
export interface ExecutionCapabilities { resourceId: 'managed-native'; limit: 1; defaultHarnessId: 'codex'; connections: ConnectionCapability[] }
export interface Attempt {
  id: string; taskId: string; taskVersion: string; connectionId: string; phase: string; outcome: string | null; revision: string;
  executionMode: string; deadlineAt: string; remainingSeconds: number; observedAt: string; settledAt: string | null;
  sourceSnapshotId: string | null; candidateId: string | null; responseArtifactId: string | null;
  evidenceStatus: string; authoredDelta: boolean;
  sourceSnapshot?: { baseCommit: string; branch: string | null; selectedRelativePath: string; manifestSha256: string };
  candidate?: { workspaceRef: string; hasDelta: boolean; hasIndexDelta: boolean; verification: string; integration: string };
  incident?: { id: string; code: string; message: string; createdAt: string };
  primaryFailure: { code: string; message: string } | null;
  stop: { acceptedAt: string } | null;
  cleanup: { state: string; scope: string; incidentId: string | null };
  requestedTarget: ExecutionPins;
  targetObservations: {
    provider: Fact; account: Fact; model: Fact; nativeSession: Fact;
    configuredModel?: Fact; nativeVersion?: Fact; nativeModels?: NativeModelsObservation;
    permissions: { requestedMode: string; configuredMode: string | null; nativeReportedMode: string | null; limitations: string[] };
    usage: UsageObservation;
  };
}
export interface TaskDetail { task: Task; version: TaskVersion; latestAttempt: Attempt | null; acceptance: { responseArtifactId: string; acceptedAt: string } | null }
export interface Admission { task: TaskSummary; version: TaskVersion; attempt: Attempt }
export interface Artifact { id: string; attemptId: string; kind: string; byteLength: string; closed: boolean; completeness: string; sha256: string | null; createdAt: string }
export interface ArtifactChunk { artifactId: string; offset: string; nextOffset: string; through: string; bytesBase64: string; eof: boolean; streamClosed: boolean; completeness: string; sha256: string | null }
export interface AttemptEvent { sequence: string; kind: string; observedAt: string; metadata: unknown }
