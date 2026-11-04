export const AGENTS = ['codex', 'claude', 'deepseek-harness', 'junie', 'antigravity'] as const;
export const MODES = ['delegate', 'review', 'debate', 'parallel', 'pipeline', 'council'] as const;
export type Mode = typeof MODES[number];
export type Permission = 'read' | 'write';
export type RunStatus = 'queued' | 'running' | 'completed' | 'failed' | 'cancelled';
export type ResultStatus = 'completed' | 'failed' | 'cancelled' | 'timeout';
export type ReasoningEffort = 'low' | 'medium' | 'high' | 'xhigh' | 'max';
export const REASONING_EFFORTS: readonly ReasoningEffort[] = ['low', 'medium', 'high', 'xhigh', 'max'];

export interface AgentConfig {
  enabled: boolean; profile: string; model?: string; effort?: ReasoningEffort; authority?: number;
  transport?: 'native' | 'acp'; command?: string; args?: string[];
  skills?: string[];
}
export interface CouncilConfig {
  version: 1;
  workspaceRoot: string;
  stateDir: string;
  backend: 'native' | 'acpx' | 'mock';
  maxConcurrency: number;
  timeoutMs: number;
  maxOutputBytes: number;
  maxCalls: number;
  agents: Record<string, AgentConfig>;
}
export interface AgentRequest {
  agent: string; prompt: string; cwd: string; permission: Permission;
  timeoutMs: number; signal?: AbortSignal; model?: string; effort?: ReasoningEffort;
  /** Internal admission snapshot: undefined model/effort retain native defaults. */
  resolvedSettings?: true;
}
export interface AgentResult {
  agent: string; status: ResultStatus; output: string; durationMs: number;
  error?: string; stopReason?: string;
}
export interface AgentHealth {
  agent: string; enabled: boolean; available: boolean; detail: string;
}
export interface AgentRunner {
  /** Enforce timeoutMs, honor cancellation and settle only after owned work is quiescent. */
  run(request: AgentRequest): Promise<AgentResult>;
  doctor(): Promise<AgentHealth[]>;
}
export interface WorkflowInput {
  mode: Mode; task: string; agents: string[]; judge?: string;
  rounds?: number; permission: Permission; context?: string; maxCalls?: number;
  assignments?: Record<string, string>;
  agentSettings?: Record<string, { model?: string; effort?: ReasoningEffort; skills?: string[] }>;
  advisors?: Record<string, { agent: string; maxRequests?: number }>;
  /** Explicit bounded private coordination; each request allows a peer reply and sender resumption. */
  privateChats?: Record<string, { peers: string[]; maxRequests?: number }>;
  contextLimit?: number | null;
}
export interface WorkflowStep {
  id: string; agent: string; role: string; result: AgentResult;
}
export interface WorkspaceArtifact {
  agent: string; path: string; patchPath: string; changed: boolean;
  baseCommit?: string;
  exportStatus?: 'ready' | 'pending' | 'failed';
  exportError?: string;
}
export interface RunRecord {
  id: string; input: WorkflowInput; status: RunStatus;
  createdAt: string; updatedAt: string; steps: WorkflowStep[];
  output?: string; error?: string; workspaces?: WorkspaceArtifact[];
}
export interface WorkflowPlan {
  mode: Mode; agents: string[]; judge?: string; estimatedCalls: number;
  maxConcurrency: number; permission: Permission; isolation: string;
  stages: string[];
}
export interface EventRecord {
  sequence: number; runId: string; type: string; data: unknown; createdAt: string;
}
export interface Message {
  id: string; from: string; to: string; subject: string; body: string;
  runId?: string; createdAt: string; acknowledgedAt?: string;
}
export interface Lease {
  id: string; owner: string; paths: string[]; runId?: string; instanceId?: string; expiresAt: string;
}
export interface ContextEntry {
  key: string; value: string; author: string; updatedAt: string;
}
