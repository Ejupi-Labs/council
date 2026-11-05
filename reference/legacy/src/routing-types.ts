import type { AgentConfig, Permission, ReasoningEffort, WorkflowInput, WorkflowPlan } from './types.js';

export type RoutingTier = 'small' | 'standard' | 'large';
export type RoutingTaskClass = 'routine' | 'standard' | 'complex';
export type RoutingStrategy = 'economy' | 'balanced' | 'quality';
export type RoutingCollaboration = 'direct' | 'review' | 'parallel' | 'council';
export interface RoutingProfile {
  tier: RoutingTier;
  capabilities: string[];
  /** Relative user ranks, not prices. Lower cost and higher quality are preferred. */
  costRank: number | null;
  qualityRank: number | null;
}
export interface RoutingQuotaView {
  availability: 'available' | 'exhausted' | 'unknown';
  fresh: boolean;
  reason?: string;
  /** A dispatch policy can block unknown capacity without claiming an observed exhaustion. */
  dispatchBlocked?: boolean;
}
export type RoutingModelSettings = Record<string, NonNullable<WorkflowInput['agentSettings']>[string] & { skills?: string[] }>;
export interface RoutingInput {
  task: string;
  taskClass: RoutingTaskClass;
  strategy?: RoutingStrategy;
  collaboration?: RoutingCollaboration;
  requiredCapabilities?: string[];
  agents?: string[];
  modelSettings?: RoutingModelSettings;
  skills?: string[];
  permission?: Permission;
  context?: string;
  contextLimit?: number | null;
  maxCalls?: number;
  rounds?: number;
  participants?: number;
  allowEscalation?: boolean;
}
export interface RoutingContext {
  agents: Record<string, AgentConfig & { routing?: RoutingProfile | null }>;
  availability?: Record<string, RoutingQuotaView | undefined> | ((alias: string) => RoutingQuotaView | undefined);
  /** Pure workflow validation and call estimation; this must not dispatch work. */
  plan: (workflow: WorkflowInput) => WorkflowPlan;
}
export interface RoutingSelection {
  alias: string;
  tier: RoutingTier | null;
  costRank: number | null;
  qualityRank: number | null;
  quota: RoutingQuotaView;
  model?: string;
  effort?: ReasoningEffort;
}
export interface RoutingAlternative {
  alias: string;
  eligible: boolean;
  selected: boolean;
  tier: RoutingTier | null;
  costRank: number | null;
  qualityRank: number | null;
  quota: RoutingQuotaView;
  reason: string;
}
export interface RoutingEscalation {
  preferredTier: RoutingTier;
  targetTiers: RoutingTier[];
  allowed: boolean;
  reason: string;
}
export interface RoutingRoomMember {
  agent: string;
  role: 'lead' | 'worker';
  weight: number;
  model?: string;
  effort?: ReasoningEffort;
  skills?: string[];
}
export interface RoutingPlan {
  workflow: WorkflowInput;
  selectedAliases: string[];
  selected: RoutingSelection[];
  reasons: string[];
  alternatives: RoutingAlternative[];
  warnings: string[];
  estimatedCalls: number;
  suggestedRoomMembers: RoutingRoomMember[];
  escalation?: RoutingEscalation;
}
export type RoutingErrorCode = 'ROUTING_INVALID_INPUT' | 'ROUTING_CONFIG_REQUIRED' | 'ROUTING_UNAVAILABLE' | 'ROUTING_ESCALATION_REQUIRED';

export class RoutingError extends Error {
  constructor(readonly code: RoutingErrorCode, message: string, readonly alternatives: RoutingAlternative[] = [], readonly escalation?: RoutingEscalation) {
    super(message); this.name = 'RoutingError';
  }
}
