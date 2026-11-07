import type { GraphBudgetRef, GraphClaimView, GraphRef } from './graph-types.js';

export type Revision = string;
export interface Policy {
  mode: 'autonomous' | 'supervised';
  selectionSource: 'default' | 'user';
  revision: Revision;
  updatedAt: string;
}
export interface Project {
  id: string;
  selectedPath: string;
  resolvedPath: string;
  repositoryRoot: string;
  branch: string | null;
  commit: string | null;
  revision: Revision;
  createdAt: string;
  updatedAt: string;
  observedAt: string;
}
export interface ProjectObservation {
  resolvedPath: string;
  repositoryRoot: string;
  branch: string | null;
  commit: string | null;
  observedAt: string;
}
export interface BoundedError { code: string; message: string; currentRevision?: Revision; graphRef?: GraphRef; graphBudgetRef?: GraphBudgetRef }
export interface ProjectDetail {
  project: Project;
  observation: ProjectObservation | null;
  observationError?: BoundedError | null;
}
export interface ProjectOpen extends ProjectDetail {
  openSelection: { selectedPath: string; basePath: string };
}
export interface Page<T> { items: T[]; nextCursor: string | null }
export interface DraftSummary {
  id: string;
  projectId: string;
  revision: Revision;
  status: 'draft';
  selection: 'auto';
  requestedPolicy: Policy;
  createdAt: string;
  updatedAt: string;
}
export interface Draft extends DraftSummary { originalText: string }
export interface Meta {
  apiVersion: 1;
  appVersion: string;
  ownerInstanceId: string;
  scope: 'workspace_foundation' | 'managed_execution';
  managedExecution: 'not_implemented' | 'native_codex' | 'native_codex_claude';
  capabilities?: string[];
  executionContract?: string;
  workflowContract?: string;
  graphContract?: string;
  maintenanceContract?: string;
  capacity?: { resourceId?: string; connectionId?: string; limit: number; activeAttemptId: string | null; occupiedSlots?: '0' | '1' | '2'; activeClaims?: GraphClaimView[] };
  admission?: { state: string; incidentId: string | null; incident?: { code: string; message: string } };
}
export interface Receipt { requestId: string; operation: string; committedAt: string }
export interface Success<T> { apiVersion: 1; requestId: string; data: T; receipt?: Receipt }
export type HarnessId = 'codex' | 'claude-code' | 'deepseek-harness' | 'junie' | 'antigravity';
export interface ConnectionObservation {
  harnessId: HarnessId;
  installationStatus: 'installed' | 'missing' | 'unsupported_launcher';
  launcherPath?: string;
  observedAt: string;
  source: 'path_and_native_install_metadata';
  managedExecution: 'not_implemented' | 'native_supported_live_unqualified';
  authentication: 'unknown';
  provider: 'unknown';
  account: 'unknown';
  model: 'unknown';
  effectivePermissions: 'unknown';
  externalOccupancy: 'unknown';
  allowance: 'unknown';
  usage: 'unknown';
  nextAction: 'install_with_vendor' | 'review_launcher_support' | 'await_managed_execution' | 'start_explicit_task';
}
export interface DiscoveryEntry {
  harnessId: HarnessId;
  observation: ConnectionObservation | null;
  error: BoundedError | null;
}
export interface DiscoveryResult {
  observedAt: string;
  source: 'path_and_native_install_metadata';
  observations: DiscoveryEntry[];
}
