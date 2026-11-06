// Exact public JSON mirror of internal/domain/workflow.go. Counters and hashes stay strings.
import type { Attempt } from './execution-types.js';
import type { RoutingDecision, TaskBudget } from './routing-types.js';

export type LocalDeliveryMode = 'review_only' | 'apply_verified_local';
export interface ExternalCommandInput { executable: string; arguments: string[]; timeoutSeconds?: number }
export interface ExternalActionInput { action: string; destination: string; acceptanceBasis: 'command_completion'; command: ExternalCommandInput }
export interface WorkflowDeliveryInput { localMode?: LocalDeliveryMode; externalAction?: ExternalActionInput }
export interface CheckScope { kind: 'whole_candidate' | 'declared_subset'; paths: string[] }
export interface CheckDeclaration {
  key: string; executable: string; arguments: string[]; relativeWorkingDirectory?: string; scope?: CheckScope;
  coveredConditionKeys?: string[]; timeoutSeconds?: number; successExitCodes?: number[]; required?: boolean;
}
export interface WorkflowInput { mode?: 'auto' | 'author_only'; delivery?: WorkflowDeliveryInput; checks?: CheckDeclaration[]; review?: 'auto' | 'required'; maxCorrectionCycles?: number }
export interface DeliveryPreference { localMode: LocalDeliveryMode; selectionSource: string; revision: string; updatedAt: string }
export interface CommandDefinition { executable: string; arguments: string[]; relativeWorkingDirectory: string; timeoutSeconds: number; successExitCodes: number[]; physicalIdentity: string }
export interface EvidenceReference { ref: string; artifactId?: string; sha256?: string; relativePath?: string }
export interface CheckDefinition { id: string; key: string; kind: string; source: string; recognizerId: string; builtinId?: string; command?: CommandDefinition; scope: CheckScope; required: boolean; coveredConditionKeys: string[]; definitionSha256: string; evidenceRefs: EvidenceReference[] }
export interface ConditionDefinition { key: string; kind: string; source: string; evidenceRefs: string[] }
export interface CandidateBinding { rootSourceSnapshotId: string; candidateId: string; sourceSnapshotId: string; manifestArtifactId: string; manifestSha256: string; indexArtifactId: string; indexSha256: string; conditionsSha256: string }
export interface ResponseBinding { producerAttemptId: string; responseArtifactId: string; responseSha256: string; responseByteLength: string; sourceSnapshotId: string; manifestSha256: string; indexArtifactId: string; indexSha256: string; conditionsSha256: string }
export interface Applicability { scopeKind: string; paths: string[]; conditionKeys: string[]; definitionSha256: string; basisSha256: string }
export interface DeliveryScope { localMode: LocalDeliveryMode; preferenceRevision?: string; source: string; projectId: string; projectIdentity: string; repositoryIdentity: string; selectedRelativePath: string; branch: string | null; destination: string; externalAction?: ExternalActionInput }
export interface WorkflowGap { key: string; code: string; path: string; message: string }
export interface WorkflowAuthority { executionMode: string; fakeScenario?: string; checkGaps?: WorkflowGap[]; id: string; taskId: string; taskVersion: string; capturedAt: string; createdByRequestId: string; instructionSha256: string; conditionsSha256: string; routingAuthorityId: string; optionSnapshot: WorkflowInput; conditions: ConditionDefinition[]; delivery: DeliveryScope; checks: CheckDefinition[]; review: string; reasonCodes: string[]; correction: { maxCycles: number; triggers: string[] } }
export interface EvidenceUse { kind: string; evidenceId: string; candidateId?: string; responseArtifactId?: string; applicability: Applicability }
export interface WorkflowCompletion { kind: string; responseAcceptanceId: string; responseBinding: ResponseBinding; reviewUses: EvidenceUse[]; checkUses: EvidenceUse[]; findingResolutionIds: string[]; externalResultRefs: string[] }
export interface Workflow {
  evidenceUses: EvidenceUse[]; hasTaskDelta: boolean; hasTaskIndexDelta: boolean;
  id: string; taskId: string; taskVersion: string; authority: WorkflowAuthority; revision: string; stateRevision: string; state: string;
  selectedCandidateId: string | null; selectedResponseBinding?: ResponseBinding; currentStepId: string | null; nextAction: string; nextOwner: string;
  correctionCyclesUsed: number; latestAcceptanceId: string | null; completion?: WorkflowCompletion; createdAt: string; updatedAt: string;
}
export interface WorkflowDetail { taskId: string; taskVersion: string; workflow: Workflow | null; acceptance?: WorkflowAcceptance | null }
export interface WorkflowStep {
  reviewId?: string; checkId?: string; externalResult?: ExternalDeliveryRecord;
  id: string; workflowId: string; ordinal: string; executionOrdinal: string; replacementOfStepId: string | null;
  role: string; executorKind: string; cycle: number; logicalKey: string; reasonCodes: string[]; definitionId: string | null;
  inputCandidateId: string | null; outputCandidateId: string | null; inputResponseBinding?: ResponseBinding; outputResponseBinding?: ResponseBinding;
  predecessorStepId: string | null; attemptId: string | null; trigger: string; status: string; createdAt: string; startedAt: string | null; settledAt: string | null; advanceProof?: WorkflowAdvanceProof;
}
export interface ReviewSubject { kind: 'candidate' | 'response'; id: string; sha256: string }
export interface ReviewFinding { key: string; severity: string; conditionKeys: string[]; locations: { path: string; startLine: number; endLine: number }[]; summary: string; evidenceRefs: string[] }
export interface ReviewReport { schemaVersion: '005-review-v1'; subject: ReviewSubject; conditionsSHA256: string; outcome: string; inspectedEvidence: { ref: string; scope: string; paths: string[] }[]; findings: ReviewFinding[]; gaps: { key: string; conditionKeys: string[]; reason: string; evidenceRefs: string[] }[]; resolvedFindingIds: string[] }
export interface WorkflowFinding { resolutions?: FindingResolution[]; id: string; modelKey: string; reviewId?: string; checkId?: string; finding: ReviewFinding }
export interface ReviewRecord { suppliedEvidence: { ref: string; scope: string; paths: string[] }[]; id: string; workflowId: string; stepId: string; attemptId: string; candidateBinding?: CandidateBinding; responseBinding?: ResponseBinding; reviewContract: string; resultArtifactId: string; resultSha256: string; applicability: Applicability; report: ReviewReport; findings: WorkflowFinding[]; outcome: string; createdAt: string }
export interface FindingResolution { id: string; workflowId: string; findingId: string; originReviewId: string | null; originCheckId: string | null; correctedCandidateId: string | null; correctedResponseBinding?: ResponseBinding; correctionStepId: string; qualifyingReviewIds: string[]; qualifyingCheckIds: string[]; evidenceUses: EvidenceUse[]; createdAt: string }
export interface CheckReceipt { finding?: WorkflowFinding; id: string; workflowId: string; stepId: string; attemptId: string; definitionId: string; definitionSha256: string; inputCandidateBinding?: CandidateBinding; inputResponseBinding?: ResponseBinding; applicability: Applicability; observedExecutable?: CommandDefinition; startedAt: string | null; finishedAt: string | null; outcome: string; exitCode: number | null; stdoutArtifactId: string | null; stderrArtifactId: string | null; rootTerminal: boolean; inputComplete: boolean; outputComplete: boolean; builtinCompleted: boolean; cleanup: { state: string; scope: string; incidentId: string | null }; resultingCandidateId: string | null; sourceChanged: boolean; createdAt: string }
export interface DestinationEvidence { projectIdentity: string; repositoryIdentity: string; branch: string | null; baseCommit: string; manifestArtifactId: string; manifestSha256: string }
export interface DeliveryOperation { id: string; workflowId: string; stepId: string; attemptId: string; requestId: string; candidateBinding: CandidateBinding; evidenceUses: EvidenceUse[]; scope: DeliveryScope; authoritySha256: string; status: string; beforeDestination: DestinationEvidence | null; afterDestination: DestinationEvidence | null; createdAt: string; finishedAt: string | null; lastEffectSequence: string; conflicts: { relativePath: string; code: string; evidenceArtifactId: string | null }[]; conflictCount: string; incidentId: string | null; nextAction: string }
export interface ExternalDeliveryRecord { workflowId: string; stepId: string; attemptId: string; action: ExternalActionInput; command: CommandDefinition; outcome: string; stdoutArtifactId: string | null; stderrArtifactId: string | null; createdAt: string }
export interface WorkflowAcceptance { id: string; workflowId: string; taskId: string; taskVersion: string; candidateBinding: CandidateBinding; authoritySha256: string; reviewUses: EvidenceUse[]; checkUses: EvidenceUse[]; findingResolutionIds: string[]; deliveryOperationId: string | null; externalResultRefs: string[]; deliveryResult: string; acceptedAt: string; acceptedByRequestId: string }
export interface WorkflowStepAdmission { workflow: Workflow; step: WorkflowStep; attempt: Attempt; budget: TaskBudget; routingDecision?: RoutingDecision }

export interface WorkflowDeliveryView { id: string; kind: 'local_application' | 'external_command' | 'review_only'; stepId: string | null; attemptId: string | null; operation?: DeliveryOperation; external?: ExternalDeliveryRecord; acceptance?: WorkflowAcceptance }
export interface WorkflowAdvanceProof { predecessorStepId: string; ownerInstanceId: string; taskRevision: string; taskVersion: string; workflowRevision: string; budgetRevision: string; windowNumber: string; candidateId: string | null; responseBinding?: ResponseBinding; nextLogicalKey: string; trigger: string }
