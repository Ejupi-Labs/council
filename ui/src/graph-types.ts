// Public 006 graph projections. IDs/counters/hashes remain lossless strings.
// Wire fields follow contracts/graphs.md and contracts/operations.md.
// This module is type-only; importing it cannot enable graph execution.
import type { Admission, ExecutionPins, TaskSummary, TaskVersion } from './execution-types.js';
import type { BudgetAmendment, ContextId, RoutingOptions, RoutingPolicy, TaskBudget } from './routing-types.js';
import type { BoundedError, Policy } from './types.js';
import type { CandidateBinding, EvidenceUse, ExternalActionInput, LocalDeliveryMode, ResponseBinding, WorkflowDeliveryInput, WorkflowInput, WorkflowStepAdmission } from './workflow-types.js';

export interface GraphRootRef { graphId: string; nodeId: null; rootTaskId: string; kind: 'root' }
export interface GraphNodeRef { graphId: string; nodeId: string; rootTaskId: string; kind: 'node' }
export type GraphRef = GraphRootRef | GraphNodeRef;
export interface GraphBudgetRef { graphId: string; budgetOwnerTaskId: string }

export type GraphNodeTaskSummary = TaskSummary & { graphRef: GraphNodeRef };
export type GraphRootTaskSummary = TaskSummary & { graphRef: GraphRootRef };
export interface GraphTaskBudgetView { budget: null; adoptionRequired: false; graphBudgetRef: GraphBudgetRef }
export type GraphExecutionAdmission = Omit<Admission, 'task' | 'budget'> & {
  task: GraphNodeTaskSummary; budget: null; graphBudgetRef: GraphBudgetRef;
};
export type GraphWorkflowStepAdmission = Omit<WorkflowStepAdmission, 'budget'> & {
  budget: null; graphBudgetRef: GraphBudgetRef;
};

export interface GraphAcceptanceInput {
  deliverable?: 'candidate' | 'response_only'; conditionsText?: string;
  requiredEvidence?: string[]; allowCandidatePreparation?: boolean;
}
export type GraphNodeRoutingInput = Omit<RoutingOptions, 'budget'>;
export type GraphNodeWorkflowInput = Omit<WorkflowInput, 'mode' | 'delivery'> & { mode?: 'auto' };
export interface GraphNodeDeclaration {
  key: string; originalText: string; pins?: ExecutionPins; acceptance?: GraphAcceptanceInput;
  timeoutSeconds?: number; fakeScenario?: string; routing?: GraphNodeRoutingInput; workflow?: GraphNodeWorkflowInput;
}
export interface GraphDependencyDeclaration { from: string; to: string; kind?: 'files' | 'context' }
export interface GraphStartInput {
  requestId: string; projectId: string; sharedInstructions?: string; title?: string;
  nodes: GraphNodeDeclaration[]; dependencies?: GraphDependencyDeclaration[]; finalNodeKey?: string;
  delivery?: WorkflowDeliveryInput; routing?: RoutingOptions; expectedPolicyRevision?: string;
  policyOverride?: Policy['mode']; executionMode?: 'native' | 'fake'; maxParallelism?: '1' | '2';
}

export interface GraphNodeAuthority { nodeId: string; taskVersion: string; nodeControlRevision: string }
export interface GraphAdvanceInput {
  requestId: string; graphVersion: string; expectedGraphControlRevision: string;
  expectedBudgetAuthorityEpoch: string; expectedWindowNumber: string; nodeAuthorities: GraphNodeAuthority[];
  rebindNodeIds?: string[];
}
export interface GraphStopInput { requestId: string; expectedGraphControlRevision: string }
export interface GraphNodeStopInput extends GraphStopInput { nodeId: string; expectedNodeControlRevision: string }
export interface GraphGuidanceInput extends GraphNodeStopInput {
  taskVersion: string; expectedRevision: string; guidanceText: string; useCurrentPolicy?: boolean;
  policyOverride?: Policy['mode']; clearPolicyOverride?: boolean; pins?: ExecutionPins;
  acceptance?: GraphAcceptanceInput; timeoutSeconds?: number; routing?: GraphNodeRoutingInput; workflow?: GraphNodeWorkflowInput;
}
export interface GraphBudgetAmendmentInput extends GraphStopInput {
  expectedBudgetAuthorityEpoch: string; expectedWindowNumber: string; additionalAdmissions: string; newWindowSeconds?: number;
}

export type GraphState = 'preparing' | 'queued' | 'running' | 'blocked' | 'cancelling' | 'cancelled' | 'failed' | 'interrupted' | 'completed';
export type GraphNodeState = GraphState | 'waiting' | 'ready';
export type GraphNextOwner = 'council' | 'user';
export interface GraphDefinitionRef { graphId: string; graphVersion: string; declarationSha256: string }
export interface GraphDeliveryScopeView {
  localMode: LocalDeliveryMode; preferenceRevision?: string; source: string; projectId: string;
  selectedRelativePath: string; branch: string | null; destination: string; externalAction?: ExternalActionInput;
}
export interface GraphDefinitionView {
  graphId: string; graphVersion: string; rootTaskId: string; original: GraphStartInput;
  requestedPolicy: Policy; routingPolicy: RoutingPolicy; delivery: GraphDeliveryScopeView; createdAt: string;
}
export interface GraphCounts {
  required: string; waiting: string; ready: string; preparing: string; queued: string; running: string;
  blocked: string; cancelling: string; cancelled: string; failed: string; interrupted: string; completed: string;
}
export interface GraphStopRecord {
  requestId: string; acceptedAt: string; graphControlRevision: string; nodeControlRevision: string | null; targetAttemptIds: string[];
}
export interface GraphSummary {
  id: string; rootTaskId: string; projectId: string; graphVersion: string; controlRevision: string;
  stateRevision: string; budgetAuthorityEpoch: string; maxParallelism: '1' | '2'; state: GraphState;
  nextAction: string; nextOwner: GraphNextOwner; rootInputId: string | null; finalNodeId: string | null;
  stop: GraphStopRecord | null; counts: GraphCounts; createdAt: string; updatedAt: string;
}
export type GraphAcceptedResultRef = {
  acceptanceKind: 'workflow' | 'response'; acceptanceId: string; taskVersion: string; producerAttemptId: string; inputId: string;
} & ({ candidateId: string; responseArtifactId: null } | { candidateId: null; responseArtifactId: string });
export interface GraphNodeSummary {
  id: string; graphId: string; key: string; taskId: string; taskVersion: string; taskRevision: string;
  controlRevision: string; stateRevision: string; state: GraphNodeState; nextAction: string; nextOwner: GraphNextOwner;
  inputId: string | null; nodeAdmissionId: string | null; latestAttemptId: string | null;
  acceptedResult: GraphAcceptedResultRef | null; stop: GraphStopRecord | null;
}
export interface GraphDispatchAuthorityView {
  ownerInstanceId: string; generation: string; graphControlRevision: string; budgetAuthorityEpoch: string;
  windowNumber: string; nodeAuthorities: GraphNodeAuthority[]; state: 'active' | 'capacity_wait' | 'suspended';
}
export interface GraphClaimView {
  slot: '1' | '2'; attemptId: string; graphId: string | null; nodeId: string | null; executorKind: string;
  contextId: ContextId | null; capturedDeclaredAccountId: string | null; capturedLinkRevision: string | null; legacyBarrier: boolean;
}
export interface GraphNodeConsumption { nodeId: string; taskId: string; consumedAdmissions: string; automaticAdmissions: string }
export interface GraphBudgetView {
  graphId: string; budgetOwnerTaskId: string; budgetAuthorityEpoch: string; budget: TaskBudget; nodeConsumption: GraphNodeConsumption[];
}
export interface GraphBlockerSummary { code: string; message: string; nodeId: string | null; inputId: string | null; artifactId: string | null }
export interface GraphAdmission {
  graph: GraphSummary; definition: GraphDefinitionRef; nodes: GraphNodeSummary[];
  preparationAttemptId: string; budget: GraphBudgetView; dispatchAuthority: GraphDispatchAuthorityView;
}
export interface GraphDetail {
  graph: GraphSummary; definition: GraphDefinitionRef; nodes: GraphNodeSummary[]; budget: GraphBudgetView;
  activeClaims: GraphClaimView[]; dispatchAuthority: GraphDispatchAuthorityView | null; blockers: GraphBlockerSummary[];
}
export interface GraphWaitView { graph: GraphDetail; changed: boolean; timedOut: boolean }
export interface GraphReboundNode {
  nodeId: string; taskId: string; fromTaskVersion: string; toTaskVersion: string;
  provenance: { kind: 'dependency_rebind'; requestId: string; previousInputId: string; contributorVectorSha256: string };
}
export interface GraphStopResult {
  graph: GraphSummary; node: GraphNodeSummary | null; stopAccepted: boolean;
  reason: 'accepted' | 'already_stopped' | 'already_settled'; targetAttemptIds: string[]; affectedNodeIds: string[];
}
export interface GraphGuidanceResult { graph: GraphSummary; node: GraphNodeSummary; result: { task: GraphNodeTaskSummary; version: TaskVersion } }
export interface GraphBudgetAmendmentResult { graph: GraphSummary; budget: GraphBudgetView; amendment: BudgetAmendment }

export interface GraphArtifactRef { artifactId: string; producerAttemptId: string; sha256: string; byteLength: string }
// Controller-only physical directory identities are intentionally absent.
export interface GraphRootBinding {
  graphId: string; rootTaskId: string; operationAttemptId: string; sourceSnapshotId: string;
  manifest: GraphArtifactRef; index: GraphArtifactRef; stagedBlobs: GraphArtifactRef; projectId: string;
  selectedRelativePath: string; baseCommit: string; branch: string | null; basisSha256: string;
}
export type GraphOutputClosure = {
  graphId: string; nodeId: string; receivedInputId: string; producerAttemptId: string;
  manifest: GraphArtifactRef; index: GraphArtifactRef; stagedBlobs: GraphArtifactRef; closureSha256: string;
} & ({ kind: 'candidate'; candidateId: string; responseArtifact: null }
  | { kind: 'response'; candidateId: null; responseArtifact: GraphArtifactRef });
export type GraphOutputState = 'complete' | 'incomplete' | 'uncertain';
export interface GraphCandidateOutputObservation {
  candidateId: string; state: GraphOutputState; closureRef: GraphArtifactRef | null; finalVerificationRef: GraphArtifactRef | null;
}
export interface GraphResponseOutputObservation { responseArtifactId: string; state: GraphOutputState; closureRef: GraphArtifactRef | null }
export interface GraphOutputObservation {
  producerAttemptId: string; candidate: GraphCandidateOutputObservation | null; response: GraphResponseOutputObservation | null;
}
export interface GraphSourceBasis {
  basisKind: 'graph_input' | 'node_subject'; workflowId: string; stepId: string; inputId: string; inputSha256: string;
  candidateBinding: CandidateBinding | null; responseBinding: ResponseBinding | null;
  expectedManifest: GraphArtifactRef; expectedIndex: GraphArtifactRef; finalVerificationRef?: GraphArtifactRef;
  expectedWorkingEntriesSha256: string; expectedIndexEntriesSha256: string;
}
export interface GraphMaterializationReceipt {
  id: string; graphId: string; inputId: string; inputSha256: string; operationAttemptId: string; receivingAttemptId: string;
  sourceBasis: GraphSourceBasis; sourceSnapshotId: string | null; observedManifest: GraphArtifactRef | null; observedIndex: GraphArtifactRef | null;
  observedWorkingEntriesSha256: string | null; observedIndexEntriesSha256: string | null; stagedBlobsSha256: string | null;
  operationEffects: GraphArtifactRef | null; effectCount: string; effectsKnown: boolean; evidenceStatus: string;
  failure: BoundedError | null; state: 'complete' | 'blocked' | 'incomplete' | 'uncertain'; completedAt: string;
}
export interface GraphScopedReference {
  ref: string; artifactRef: GraphArtifactRef; authorityScope: string; nodeId: string | null; taskVersion: string | null; paths: string[];
}
export interface GraphContributedEvidence {
  ref: string; edgeId: string; sourceNodeId: string; acceptanceKind: 'workflow' | 'response'; acceptanceId: string; producerAttemptId: string;
  resultKind: 'candidate' | 'response'; resultId: string; artifactRef: GraphArtifactRef; scope: string; paths: string[];
}
export interface GraphProvidedAccess { ref: string; artifactRef: GraphArtifactRef; scope: string; paths: string[]; mode: string }
export interface GraphContextPacket {
  id: string; graphId: string; inputId: string; inputSha256: string; receivingTaskVersion: string;
  graphInstructionsRef: GraphArtifactRef; nodeInstructionsRef: GraphArtifactRef; guidanceRef: GraphArtifactRef | null;
  acceptanceRef: GraphArtifactRef | null; constraintRefs: GraphScopedReference[]; contributedEvidence: GraphContributedEvidence[];
  derivedDigestRef: GraphArtifactRef | null; providedAccess: GraphProvidedAccess[]; packetSha256: string;
}
export interface GraphConditionSource {
  ref: string; authorityScope: string; nodeId: string | null; taskVersion: string | null;
  originalRef: GraphArtifactRef; guidanceRef: GraphArtifactRef | null; acceptanceRef: GraphArtifactRef | null;
}
export interface GraphConditionsBinding {
  graphId: string; graphVersion: string; receiverTaskId: string; receiverTaskVersion: string; sources: GraphConditionSource[]; conditionsSha256: string;
}

export type GraphResultBinding = { candidateBinding: CandidateBinding; responseBinding: null }
  | { candidateBinding: null; responseBinding: ResponseBinding };
export type GraphContributorBinding = {
  nodeId: string; taskId: string; taskVersion: string; acceptanceKind: 'workflow' | 'response';
  acceptanceId: string; producerAttemptId: string; inputId: string; outputClosureRef: GraphArtifactRef;
} & GraphResultBinding;
export type AcceptedContribution = {
  edgeId: string; edgeKind: 'files' | 'context'; fromNodeId: string; taskId: string; taskVersion: string;
  acceptanceKind: 'workflow' | 'response'; acceptanceId: string; producerAttemptId: string; inputId: string;
  outputClosureRef: GraphArtifactRef; constraintRefs: GraphScopedReference[]; evidenceRefs: GraphContributedEvidence[];
} & GraphResultBinding;
export interface GraphInputBinding {
  id: string; graphId: string; graphVersion: string; rootBasisSha256: string; operationAttemptId: string;
  receivingNodeId: string; receivingTaskId: string; receivingTaskVersion: string;
  contributions: AcceptedContribution[]; contributorVector: GraphContributorBinding[];
  manifest: GraphArtifactRef; index: GraphArtifactRef; stagedBlobs: GraphArtifactRef;
  provenance: GraphArtifactRef; contextPacket: GraphArtifactRef; inputSha256: string;
}
export interface GraphDispatch {
  id: string; graphId: string; graphVersion: string; nodeId: string; taskId: string; taskVersion: string;
  nodeAdmissionId: string; workflowId: string; stepId: string; attemptId: string; executorKind: string; role: string;
  inputId: string; inputSha256: string; contextPacket: GraphArtifactRef; contributorVector: GraphContributorBinding[];
  ownerInstanceId: string; graphControlRevision: string; nodeControlRevision: string;
  budgetOwnerTaskId: string; budgetAuthorityEpoch: string; windowNumber: string; observedBudgetRevision: string;
  sourceBasis: GraphSourceBasis; freshTriggerId: string; logicalSlot: string; executionOrdinal: string;
  nativeConsumptionKey: string | null; createdAt: string;
}
export interface GraphDispatchView {
  dispatch: GraphDispatch; materialization: GraphMaterializationReceipt | null; outputObservation: GraphOutputObservation | null;
}
export interface GraphAdvanceResult {
  graph: GraphSummary; preparationAttemptIds: string[]; dispatches: GraphDispatchView[]; reboundNodes: GraphReboundNode[];
  budget: GraphBudgetView; dispatchAuthority: GraphDispatchAuthorityView | null;
}
export type GraphRootView = GraphRootBinding;
export interface GraphInputSummary {
  id: string; graphId: string; graphVersion: string; rootTaskId: string; operationAttemptId: string;
  kind: string; state: string; current: boolean; receivingNodeId: string | null; receivingTaskId: string | null;
  receivingTaskVersion: string | null; rootBasisSha256: string | null; inputSha256: string | null;
  failure: BoundedError | null; createdAt: string; updatedAt: string;
}
export interface GraphInputDetail {
  summary: GraphInputSummary; rootBinding: GraphRootView | null; inputBinding: GraphInputBinding | null;
  conditionsRef: GraphArtifactRef | null; contextPacket: GraphContextPacket | null;
  operationEffects: GraphArtifactRef | null; conflictArtifact: GraphArtifactRef | null;
}
export interface GraphMemberOrigin {
  kind: string; nodeId: string | null; taskId: string; taskVersion: string | null; acceptanceKind: string | null;
  acceptanceId: string | null; producerAttemptId: string; inputId: string; stateSha256: string;
}
export interface GraphMemberView {
  dimension: string; path: string; state: string; type: string | null; mode: string | null; byteLength: string | null;
  sha256: string | null; target: string | null; objectId: string | null; artifactRef: GraphArtifactRef | null; origins: GraphMemberOrigin[];
}
export interface GraphFinalVerificationBinding {
  graphId: string; graphVersion: string; finalNodeId: string; finalTaskId: string; finalTaskVersion: string; finalWorkflowId: string;
  rootBinding: GraphRootView; finalCandidateBinding: CandidateBinding; outputClosureRef: GraphArtifactRef;
  finalInputId: string; contributorVector: GraphContributorBinding[]; graphConditionsRef: GraphArtifactRef; graphConditionsSha256: string;
  originalAccess: GraphProvidedAccess[]; cumulativeDeltaRef: GraphArtifactRef;
  hasCumulativeWorkingDelta: boolean; hasCumulativeIndexDelta: boolean; bindingSha256: string;
}
export interface GraphDeliveryBinding {
  graphId: string; graphVersion: string; graphControlRevision: string; rootBinding: GraphRootView;
  finalNodeId: string; finalTaskId: string; finalTaskVersion: string; finalWorkflowId: string;
  finalCandidateBinding: CandidateBinding; finalInputId: string; contributorVector: GraphContributorBinding[];
  finalVerificationRef: GraphArtifactRef; otherNodeAcceptances: GraphContributorBinding[];
  preDeliveryEvidenceUses: EvidenceUse[]; deliveryScopeSha256: string; bindingSha256: string;
}
