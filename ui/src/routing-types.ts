import type { ConnectionCapability, Fact, NativeModelsObservation, TaskSummary, TaskVersion, UsageObservation } from './execution-types.js';

// API 1 / 004-v1 DTO mirror of internal/domain/routing.go. Exact counts and price
// lexemes stay strings; declarations and reports do not establish native access.
export type ContextId = 'codex-native' | 'claude-native';
export type Suitability = 'unknown' | 'routine' | 'general';
export interface RoutingBudgetInput { admissionLimit?: string; windowSeconds?: number }
export interface ContinuationInput { maxAutomaticAdmissions?: string; categories?: string[]; allowReplacement?: boolean; allowEscalation?: boolean }
export interface ContinuationPolicy { maxAutomaticAdmissions: string; categories: string[]; allowReplacement: boolean; allowEscalation: boolean }
export interface RoutingOptions {
  suitability?: Suitability; targetProfileId?: string; expectedPolicyRevision?: string;
  budget?: RoutingBudgetInput; continuation?: ContinuationInput; useCurrentPolicy?: boolean;
}
export interface RoutingPolicy {
  revision: string; economyOrder: string[]; unknownAvailability: 'allow' | 'require_positive';
  defaultBudget: { admissionLimit: string; windowSeconds: number }; continuation: ContinuationPolicy; updatedAt: string;
}
export interface ReferencePrice {
  currency: string; unit: 'per_million_tokens' | null; basis: 'api_list' | 'declared_marginal' | null;
  inputPerMillion: string | null; outputPerMillion: string | null; sourceKind: 'user_reference';
  sourceLabel: string; sourceUrl: string | null; observedAt: string; validUntil: string | null;
}
export interface TargetProfile {
  id: string; contextId: ContextId; modelSelector: string; role: 'native_default' | 'economy' | 'general';
  enabled: boolean; revision: string; updatedAt: string; referencePrice: ReferencePrice | null;
  source: { kind: string; label: string; url: string; readDate: string; minimumNativeVersion: string | null };
}
export interface DeclaredAccount { id: string; label: string; providerLabel: string | null; entitlement: string; revision: string; createdAt: string; updatedAt: string }
export interface AccountLink { contextId: ContextId; declaredAccountId: string | null; revision: string; updatedAt: string; source: string }
export interface CapacitySubject { kind: 'context' | 'account'; id: string }
export type ReportDimension = 'occupancy' | 'allowance' | 'service';
export interface ReportRef {
  reportId: string; subject: CapacitySubject; dimension: ReportDimension; state: string; source: string;
  observedAt: string; receivedAt: string; expiresAt: string | null; retryAfter: string | null;
  freshness: string; retained: boolean; inheritedFrom: { accountId: string; linkRevision: string; linkedAt: string } | null;
}
export interface CapacityDimension { effectiveState: string; latestReport: ReportRef | null; blockingReports: ReportRef[]; positiveReportIds: string[] }
export interface CapacityView {
  subject: CapacitySubject; revision: string; asOf: string; accountLink: AccountLink | null;
  dimensions: { occupancy: CapacityDimension; allowance: CapacityDimension; service: CapacityDimension };
  reportEligibility: { policy: 'allow' | 'require_positive'; permitted: boolean; positiveCoverage: { occupancy: boolean; allowance: boolean }; reasonCodes: string[] };
  managedOccupancy: { resourceId: 'managed-native'; limit: 1; activeAttemptId: string | null; activeContextId: ContextId | null; claimsSubject: boolean };
}
export interface ContextView { id: ContextId; harnessId: 'codex' | 'claude-code'; connectionId: string; label: string; enabled: boolean; revision: string; updatedAt: string; capabilities: ConnectionCapability; accountLink: AccountLink; capacity: CapacityView }
export interface CapacityReport {
  id: string; sequence: string; requestId: string; subject: CapacitySubject; dimension: ReportDimension; state: string;
  source: string; observedAt: string; receivedAt: string; validForSeconds: number; expiresAt: string | null;
  retryAfter: string | null; correctionOf: string | null; correctionAction: string | null;
  bindings: string[]; resolvedScopeKeys: string[]; note: string;
}
export interface ResolvedTarget {
  contextId: ContextId; connectionId: string; harnessId: 'codex' | 'claude-code'; modelSelector: string; profileId: string | null;
  declaredAccountAssociation: { accountId: string; label: string; linkRevision: string; source: string } | null;
}
export interface SuitabilityAssessment {
  classification: Suitability; source: string; ruleId: string | null; reasonCodes: string[];
  sourceProof: { relativePath: string; sha256: string; physicalIdentity: string; byteLength: string } | null;
}
export interface TaskRoutingAuthority { id: string; taskId: string; taskVersion: string; capturedAt: string; policySnapshot: RoutingPolicy; optionSnapshot: RoutingOptions; suitabilityAssessment: SuitabilityAssessment; createdBy: string }
export interface CandidateAssessment { profileId: string | null; profileSnapshot: TargetProfile | null; contextId: ContextId; eligible: boolean; reasonCodes: string[]; uncertainties: string[] }
export interface CandidateCapacityEvidence { contextId: ContextId; revision: string; asOf: string; declaredAccountId: string | null; accountLinkRevision: string | null; effectiveStates: { occupancy: string; allowance: string; service: string }; positiveCoverage: { occupancy: boolean; allowance: boolean }; permitted: boolean; blockingReportIds: string[]; supportingReportIds: string[] }
export interface RoutingFacts { assessment: SuitabilityAssessment; contexts: { contextId: ContextId; installationStatus: string; nativeVersion: Fact | null; observedAt: string; source: string }[] }
export interface BudgetWindow { number: string; windowSeconds: number; firstAdmissionAt: string | null; deadlineAt: string | null }
export interface TaskBudget { taskId: string; revision: string; grantedAdmissions: string; consumedAdmissions: string; remainingAdmissions: string; automaticAdmissions: string; historicalAdmissions: string; currentWindow: BudgetWindow; latestAmendmentId: string | null; updatedAt: string }
export interface RoutingDecision {
  id: string; taskId: string; taskVersion: string; attemptId: string; requestId: string; authorityId: string;
  kind: string; previousAttemptId: string | null; createdAt: string; candidateAssessments: CandidateAssessment[];
  selectedTarget: ResolvedTarget; budgetEffect: { admissionUnits: string; remainingAdmissions: string; budgetRevision: string; window: BudgetWindow };
  capacityEvidence: CandidateCapacityEvidence[]; evidenceSnapshot: RoutingFacts; uncertainties: string[];
}
export interface TaskRoutingView { authority: TaskRoutingAuthority | null; latestDecision: RoutingDecision | null; adoptionRequired: boolean }
export interface TaskBudgetView { budget: TaskBudget | null; adoptionRequired: boolean }
export interface BudgetAmendment { id: string; taskId: string; requestId: string; action: string; additionalAdmissions: string; newWindowSeconds: number | null; priorBudget: TaskBudget | null; budget: TaskBudget; authorityId: string | null; taskVersion: string; createdAt: string }
export interface BudgetAmendmentResult { task: TaskSummary; version?: TaskVersion; amendment: BudgetAmendment; budget: TaskBudget; authority?: TaskRoutingAuthority }
export interface BudgetLedgerEntry { id: string; sequence: string; taskId: string; budgetRevision: string; kind: string; requestId: string; attemptId: string | null; grantedDelta: string; consumedDelta: string; window: BudgetWindow; createdAt: string }
export interface UsageItem { attemptId: string; attemptSequence: string; executionMode: 'native' | 'fake'; outcome: string | null; selectedUsageScope: 'top_level' | 'whole_tree' | 'unknown' | 'fake_unmeasured'; usage: UsageObservation; nativeModels: NativeModelsObservation | null; evidenceArtifactIds: string[] }
export interface CounterSubtotal { knownValue: string | null; contributingAttemptIds: string[]; missingAttemptIds: string[] }
export interface ScopeSubtotal { inputTokens: CounterSubtotal; outputTokens: CounterSubtotal; cachedInputTokens: CounterSubtotal; cacheCreationInputTokens: CounterSubtotal }
export interface UsagePage {
  taskId: string; items: UsageItem[]; nextCursor: string | null;
  knownSubtotal: { scope: 'returned_attempts'; byUsageScope: { top_level: ScopeSubtotal; whole_tree: ScopeSubtotal } };
  coverage: { scope: 'returned_attempts'; status: string; includedAttemptIds: string[]; unknownAttemptIds: string[]; excludedFakeAttemptIds: string[] };
  estimates: { attemptId: string; model: string; currency: string; value: string; scope: string; source: string; observedAt: string; evidenceArtifactIds: string[]; previewCompleteness: string }[];
  referencePrices: { attemptId: string; decisionId: string; profileId: string | null; referencePrice: ReferencePrice }[];
  charges: { status: 'unknown'; items: [] };
}
