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
export interface BoundedError { code: string; message: string; currentRevision?: Revision }
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
  scope: 'workspace_foundation';
  managedExecution: 'not_implemented';
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
  managedExecution: 'not_implemented';
  authentication: 'unknown';
  provider: 'unknown';
  account: 'unknown';
  model: 'unknown';
  effectivePermissions: 'unknown';
  externalOccupancy: 'unknown';
  allowance: 'unknown';
  usage: 'unknown';
  nextAction: 'install_with_vendor' | 'review_launcher_support' | 'await_managed_execution';
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
