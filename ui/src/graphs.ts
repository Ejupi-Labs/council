import { addFact, api, ApiError, element, errorMessage, feedback, policyName, readSession, requestId, supportsManagedExecution, timeLabel, writeSession } from './api.js';
import { GraphComposer } from './graph-draft.js';
import { rememberGraphTask } from './graph-membership.js';
import { GraphInputs } from './graph-inputs.js';
import type { GraphAdmission, GraphAdvanceInput, GraphAdvanceResult, GraphReboundNode, GraphGuidanceInput, GraphGuidanceResult, GraphBudgetAmendmentInput, GraphBudgetAmendmentResult, GraphDefinitionView, GraphDetail, GraphNodeStopInput, GraphNodeSummary, GraphRef, GraphStartInput, GraphStopInput, GraphStopResult, GraphSummary, GraphWaitView } from './graph-types.js';
import type { Meta, Page, Policy } from './types.js';
import type { BudgetLedgerEntry, RoutingPolicy } from './routing-types.js';
import type { DeliveryPreference } from './workflow-types.js';
import type { TaskVersion } from './execution-types.js';
import { RecoveryView } from './recovery.js';

// Start and exact Start retries use the connected graph admission path. Other
// graph mutations retain their own supported controls and authority checks.
const graphStartRuntimeReady = true;
interface PendingStart {
  path: '/graphs'; body: GraphStartInput; projectId: string; editorRevision: number;
  selectionGeneration: number; unresolved: boolean;
}
interface PendingControl {
  kind: 'stop' | 'node-stop' | 'grant' | 'guidance' | 'advance'; path: string; operation: string; projectId: string; graphId: string; rootTaskId: string;
  nodeId: string | null; nodeKey: string | null; taskVersion: string | null;
  taskId?: string;
  rebindTargets?: RebindTarget[];
  body: GraphStopInput | GraphNodeStopInput | GraphBudgetAmendmentInput | GraphGuidanceInput | GraphAdvanceInput;
  selectionGeneration: number; detailGeneration: number; intentGeneration: number; unresolved: boolean;
}
interface GrantDraft { admissions: string; window: string }
interface GuidanceDraft { text: string; edited: boolean; version: string }
interface RebindTarget { nodeId: string; nodeKey: string; taskId: string; taskVersion: string; inputId: string }
interface RebindReceipt { graphId: string; requestId: string; targets: RebindTarget[]; nodes: GraphReboundNode[] }
const id = (value: unknown): value is string => typeof value === 'string' && /^[a-f0-9]{32}$/.test(value);
const count = (value: unknown): value is string => typeof value === 'string' && /^(0|[1-9][0-9]*)$/.test(value) && BigInt(value) <= 9223372036854775807n;
const revision = (value: unknown): value is string => count(value) && value !== '0';
const make = <K extends keyof HTMLElementTagNameMap>(tag: K, text = ''): HTMLElementTagNameMap[K] => {
  const result = document.createElement(tag); if (text) result.textContent = text; return result;
};
const observedWork = (graph: GraphSummary): boolean => ['preparing', 'queued', 'running', 'cancelling'].includes(graph.state)
  || (graph.state === 'blocked' && graph.nextOwner === 'council');

// Read responses contain intentional nulls. Validate identities and exact counters
// before using them as a selected graph or a future control authority.
function checkDetail(detail: GraphDetail, graphId: string, projectId: string): void {
  const graph = detail?.graph, budget = detail?.budget;
  if (!graph || graph.id !== graphId || graph.projectId !== projectId || !id(graph.rootTaskId)
    || !revision(graph.graphVersion) || !revision(graph.controlRevision) || !revision(graph.stateRevision) || !revision(graph.budgetAuthorityEpoch)
    || !['1', '2'].includes(graph.maxParallelism) || !['council', 'user'].includes(graph.nextOwner)
    || !['preparing', 'queued', 'running', 'blocked', 'cancelling', 'cancelled', 'failed', 'interrupted', 'completed'].includes(graph.state)
    || detail.definition?.graphId !== graphId || detail.definition.graphVersion !== graph.graphVersion
    || !budget || budget.graphId !== graphId || budget.budgetOwnerTaskId !== graph.rootTaskId || budget.budget?.taskId !== graph.rootTaskId
    || budget.budgetAuthorityEpoch !== graph.budgetAuthorityEpoch || !revision(budget.budget.revision) || !revision(budget.budget.currentWindow?.number)
    || ![budget.budget.grantedAdmissions, budget.budget.consumedAdmissions, budget.budget.remainingAdmissions, budget.budget.automaticAdmissions, budget.budget.historicalAdmissions].every(count)
    || !Array.isArray(detail.nodes) || detail.nodes.length < 1 || detail.nodes.length > 16 || !Array.isArray(detail.blockers) || !Array.isArray(detail.activeClaims)
    || !Array.isArray(budget.nodeConsumption)) throw new ApiError('invalid_response', 'The owner returned an incomplete or mismatched task-group detail. Existing text and loaded evidence are retained.');
  const nodeIds = new Set<string>(), taskIds = new Set<string>(), keys = new Set<string>();
  for (const node of detail.nodes) {
    if (!node || !id(node.id) || node.graphId !== graphId || !id(node.taskId) || node.taskId === graph.rootTaskId
      || nodeIds.has(node.id) || taskIds.has(node.taskId) || !/^[A-Za-z0-9][A-Za-z0-9_-]{0,47}$/.test(node.key) || keys.has(node.key)
      || !revision(node.taskVersion) || !revision(node.taskRevision) || !revision(node.controlRevision) || !revision(node.stateRevision)) throw new ApiError('invalid_response', 'The task-group node identities or versions do not match. Refresh this group before acting.');
    nodeIds.add(node.id); taskIds.add(node.taskId); keys.add(node.key);
  }
  if (graph.finalNodeId !== null && !nodeIds.has(graph.finalNodeId)) throw new ApiError('invalid_response', 'The final-output identity is not a member of this task group.');
  const consumption = new Set<string>();
  for (const item of budget.nodeConsumption) {
    if (!nodeIds.has(item.nodeId) || consumption.has(item.nodeId) || !detail.nodes.some(node => node.id === item.nodeId && node.taskId === item.taskId)
      || !count(item.consumedAdmissions) || !count(item.automaticAdmissions)) throw new ApiError('invalid_response', 'The shared budget contains a mismatched node breakdown.');
    consumption.add(item.nodeId);
  }
  rememberGraphTask(graph.rootTaskId, { graphId, rootTaskId: graph.rootTaskId, nodeId: null, kind: 'root' });
  for (const node of detail.nodes) rememberGraphTask(node.taskId, { graphId, rootTaskId: graph.rootTaskId, nodeId: node.id, kind: 'node' });
}

// A saved Start receipt freezes its original identities and authority. Validate
// that snapshot against its captured declaration, never today's owner/defaults.
function validAdmission(admission: GraphAdmission, input: GraphStartInput): boolean {
  const graph = admission?.graph, definition = admission?.definition, budget = admission?.budget, authority = admission?.dispatchAuthority;
  if (!graph || !id(graph.id) || !id(graph.rootTaskId) || graph.projectId !== input.projectId || !id(graph.rootInputId)
    || !id(admission.preparationAttemptId) || ![graph.graphVersion, graph.controlRevision, graph.stateRevision, graph.budgetAuthorityEpoch].every(revision)
    || graph.maxParallelism !== (input.maxParallelism ?? '2') || !['council', 'user'].includes(graph.nextOwner)
    || !['preparing', 'queued', 'running', 'blocked', 'cancelling', 'cancelled', 'failed', 'interrupted', 'completed'].includes(graph.state)
    || definition?.graphId !== graph.id || definition.graphVersion !== graph.graphVersion || typeof definition.declarationSha256 !== 'string' || !/^[a-f0-9]{64}$/.test(definition.declarationSha256)
    || !Array.isArray(admission.nodes) || admission.nodes.length !== input.nodes.length
    || !budget || budget.graphId !== graph.id || budget.budgetOwnerTaskId !== graph.rootTaskId || budget.budget?.taskId !== graph.rootTaskId
    || budget.budgetAuthorityEpoch !== graph.budgetAuthorityEpoch || !revision(budget.budget.revision) || !revision(budget.budget.currentWindow?.number)
    || ![budget.budget.grantedAdmissions, budget.budget.consumedAdmissions, budget.budget.remainingAdmissions, budget.budget.automaticAdmissions, budget.budget.historicalAdmissions].every(count)
    || !Array.isArray(budget.nodeConsumption) || budget.nodeConsumption.length !== input.nodes.length
    || !authority || !id(authority.ownerInstanceId) || !revision(authority.generation) || authority.graphControlRevision !== graph.controlRevision
    || authority.budgetAuthorityEpoch !== graph.budgetAuthorityEpoch || authority.windowNumber !== budget.budget.currentWindow.number
    || !['active', 'capacity_wait', 'suspended'].includes(authority.state) || !Array.isArray(authority.nodeAuthorities) || authority.nodeAuthorities.length !== input.nodes.length) return false;
  const nodes = new Map<string, GraphNodeSummary>(), tasks = new Set<string>();
  for (let i = 0; i < admission.nodes.length; i++) {
    const node = admission.nodes[i], declaration = input.nodes[i];
    if (!node || !declaration || !id(node.id) || nodes.has(node.id) || node.graphId !== graph.id || node.key !== declaration.key
      || !id(node.taskId) || node.taskId === graph.rootTaskId || tasks.has(node.taskId)
      || ![node.taskVersion, node.taskRevision, node.controlRevision, node.stateRevision].every(revision)) return false;
    nodes.set(node.id, node); tasks.add(node.taskId);
  }
  const final = input.finalNodeKey ? admission.nodes.find(node => node.key === input.finalNodeKey)?.id : null;
  if (graph.finalNodeId !== final) return false;
  const debitNodes = new Set<string>(), authorityNodes = new Set<string>();
  for (const debit of budget.nodeConsumption) {
    const node = debit && nodes.get(debit.nodeId);
    if (!node || debitNodes.has(node.id) || debit.taskId !== node.taskId || !count(debit.consumedAdmissions) || !count(debit.automaticAdmissions)) return false;
    debitNodes.add(node.id);
  }
  for (const captured of authority.nodeAuthorities) {
    const node = captured && nodes.get(captured.nodeId);
    if (!node || authorityNodes.has(node.id) || captured.taskVersion !== node.taskVersion || captured.nodeControlRevision !== node.controlRevision) return false;
    authorityNodes.add(node.id);
  }
  return true;
}

export class GraphsView {
  private readonly recovery: RecoveryView;
  private readonly composer = new GraphComposer();
  private projectId: string | null = null;
  private policy: Policy | null = null;
  private routingPolicy: RoutingPolicy | null = null;
  private deliveryPreference: DeliveryPreference | null = null;
  private ready = false;
  private recoveryReady = false;
  private projectGeneration = 0;
  private selectionGeneration = 0;
  private listGeneration = 0;
  private detailGeneration = 0;
  private definitionGeneration = 0;
  private defaultsGeneration = 0;
  private observationGeneration = 0;
  private selectedId: string | null = null;
  private detail: GraphDetail | null = null;
  private definition: GraphDefinitionView | null = null;
  private renderedDefinitionKey: string | null = null;
  private readonly summaries = new Map<string, GraphSummary>();
  private cursor: string | null = null;
  private readonly busy = new Set<string>();
  private readonly pending = new Map<string, PendingStart>();
  private readonly inputs = new GraphInputs(element('graph-overview'));
  private readonly controlPending = new Map<string, PendingControl>();
  private readonly controlBusy = new Set<string>();
  private readonly grantDrafts = new Map<string, GrantDraft>();
  private readonly guidanceDrafts = new Map<string, GuidanceDraft>();
  private readonly guidanceSelections = new Map<string, string>();
  private readonly rebindSelections = new Map<string, Set<string>>();
  private readonly rebindReceipts = new Map<string, RebindReceipt>();
  private intentGeneration = 0;
  private controlsGraphId: string | null = null;
  private ledgerGeneration = 0;
  private ledgerBusy = false;
  private ledgerCursor: string | null = null;
  private readonly ledgerRows = new Map<string, BudgetLedgerEntry>();
  private readonly controlPanel = make('section');
  private readonly controlStatus = make('div');
  private readonly controlPendingList = make('ul');
  private readonly stopGroup = make('button', 'Stop group');
  private readonly advance = make('button', 'Advance graph');
  private readonly rebindChoices = make('div');
  private readonly rebindAdvance = make('button', 'Use updated predecessor inputs and advance');
  private readonly rebindReceiptList = make('ul');
  private readonly guidanceNode = make('select');
  private readonly guidanceText = make('textarea');
  private readonly guidanceCurrent = make('p');
  private readonly guidanceSave = make('button', 'Save node guidance');
  private readonly grantAdmissions = make('input');
  private readonly grantWindow = make('input');
  private readonly grantButton = make('button', 'Grant shared authority');
  private readonly ledgerLoad = make('button', 'Read shared budget ledger');
  private readonly ledgerMore = make('button', 'Load more shared ledger');
  private readonly ledgerList = make('ul');
  private readonly ledgerStatus = make('div');
  onOpenTask = (_taskId: string, _ref: GraphRef): void => {};
  onTasksChanged = (_projectId: string): void => {};

  constructor() {
    this.recovery = new RecoveryView(element('graph-overview'), 'group', () => { if (this.selectedId) void this.open(this.selectedId, false); });
    this.mountControls();
    const retainedControls = readSession<PendingControl[]>('graphPendingControls');
    if (Array.isArray(retainedControls) && retainedControls.length <= 32) for (const item of retainedControls) {
      if (this.validPendingControl(item)) this.controlPending.set(this.controlKey(item.kind, item.graphId, item.nodeId), { ...item, unresolved: true });
    }
    const retained = readSession<PendingStart[]>('graphPendingStarts');
    if (Array.isArray(retained) && retained.length <= 32) for (const pending of retained) {
      if (pending?.path === '/graphs' && id(pending.projectId) && pending.body?.projectId === pending.projectId && id(pending.body.requestId)
        && Number.isSafeInteger(pending.editorRevision) && Number.isSafeInteger(pending.selectionGeneration) && Array.isArray(pending.body.nodes))
        this.pending.set(pending.body.requestId, { ...pending, unresolved: true });
    }
    this.composer.onChange = () => this.controls();
    element('open-graph-composer').addEventListener('click', () => this.openComposer());
    element('open-graph-list').addEventListener('click', () => this.openList());
    element('close-graph-composer').addEventListener('click', () => { element('graphs-panel').hidden = true; element('open-graph-composer').focus(); });
    element('graph-start-form').addEventListener('submit', event => { event.preventDefault(); void this.start(); });
    element('graph-refresh').addEventListener('click', () => { void this.refresh(); if (this.selectedId) void this.open(this.selectedId, false); });
    element('graph-more').addEventListener('click', () => { void this.refresh(true); });
    element('graph-refresh-defaults').addEventListener('click', () => { void this.loadDefaults(); });
    this.renderPending(); this.renderControlPending(); this.controls();
  }

  setOwner(meta: Meta): void {
    this.ready = supportsManagedExecution(meta) && meta.graphContract === '006-v1';
    this.recoveryReady = this.ready && meta.recoveryContract === '007-v1';
    this.recovery.setGraph(this.detail, this.recoveryReady);
    ++this.defaultsGeneration; ++this.observationGeneration;
    this.controls(); if (this.ready) { void this.loadDefaults(); if (this.projectId) void this.refresh(); }
    if (this.ready && this.detail && observedWork(this.detail.graph)) this.observe(this.detail.graph.id);
  }
  disconnect(): void {
    this.ready = false; this.recoveryReady = false; ++this.listGeneration; ++this.detailGeneration; ++this.definitionGeneration; ++this.defaultsGeneration; ++this.observationGeneration;
    this.recovery.setGraph(this.detail, false);
    this.controls();
  }
  setPolicy(policy: Policy): void { this.policy = policy; this.defaults(); this.controls(); }
  setProject(projectId: string): void {
    if (this.projectId === projectId) return;
    ++this.projectGeneration; ++this.selectionGeneration; ++this.listGeneration; ++this.detailGeneration; ++this.definitionGeneration; ++this.observationGeneration;
    this.projectId = projectId; this.selectedId = null; this.detail = null; this.definition = null; this.summaries.clear(); this.cursor = null;
    this.composer.setProject(projectId); this.renderList(); this.renderDetail(); this.renderPending(); this.renderControlPending(); this.controls();
    if (this.ready) void this.refresh();
  }
  openComposer(): void {
    element('graphs-panel').hidden = false; element('graph-start-form').hidden = false;
    element('graph-editor-nodes').querySelector<HTMLTextAreaElement>('textarea')?.focus();
  }
  openList(): void {
    element('graphs-panel').hidden = false; element('graph-start-form').hidden = true;
    element('graphs-heading').focus(); if (this.ready && this.projectId) void this.refresh();
  }
  async open(graphId: string, changeSelection = true): Promise<void> {
    if (!this.ready || !this.projectId || !id(graphId)) return;
    const projectId = this.projectId, projectGeneration = this.projectGeneration;
    if (changeSelection || this.selectedId !== graphId) { ++this.selectionGeneration; ++this.observationGeneration; this.detail = null; this.definition = null; }
    this.selectedId = graphId;
    const generation = ++this.detailGeneration, selection = this.selectionGeneration;
    element('graphs-panel').hidden = false;
    if (!this.detail) this.renderDetail();
    feedback('graph-detail-status', 'Reading the task group…');
    try {
      const { data } = await api<GraphDetail>('/graphs/' + graphId);
      if (!this.current(projectId, projectGeneration, graphId, selection) || generation !== this.detailGeneration) return;
      checkDetail(data, graphId, projectId); if (this.older(data)) return;
      this.detail = data; this.summaries.set(graphId, data.graph); this.renderList(); this.renderDetail();
      feedback('graph-detail-status', 'Current group observed. Reading does not start or rebind any task.');
      await this.loadDefinition(data, projectGeneration, selection);
      if (!this.current(projectId, projectGeneration, graphId, selection) || generation !== this.detailGeneration) return;
      if (observedWork(data.graph)) this.observe(graphId);
    } catch (error) {
      if (this.current(projectId, projectGeneration, graphId, selection) && generation === this.detailGeneration)
        feedback('graph-detail-status', errorMessage(error) + ' Loaded results and the group draft remain available.', 'error');
    } finally { if (this.current(projectId, projectGeneration, graphId, selection) && generation === this.detailGeneration) this.controls(); }
  }

  private current(projectId: string, generation: number, graphId: string, selection: number): boolean {
    return this.ready && this.projectId === projectId && this.projectGeneration === generation && this.selectedId === graphId && this.selectionGeneration === selection;
  }
  private older(detail: GraphDetail): boolean {
    const current = this.detail; if (!current || current.graph.id !== detail.graph.id) return false;
    if (BigInt(detail.graph.stateRevision) < BigInt(current.graph.stateRevision) || BigInt(detail.graph.controlRevision) < BigInt(current.graph.controlRevision)
      || BigInt(detail.graph.graphVersion) < BigInt(current.graph.graphVersion)) return true;
    return detail.nodes.some(node => { const prior = current.nodes.find(item => item.id === node.id); return !!prior && (BigInt(node.taskVersion) < BigInt(prior.taskVersion) || BigInt(node.stateRevision) < BigInt(prior.stateRevision) || BigInt(node.controlRevision) < BigInt(prior.controlRevision)); });
  }
  private async loadDefaults(): Promise<void> {
    if (!this.ready) return;
    const generation = ++this.defaultsGeneration;
    try {
      const [routing, delivery] = await Promise.all([api<RoutingPolicy>('/routing/policy'), api<DeliveryPreference>('/workflow/preferences')]);
      if (!this.ready || generation !== this.defaultsGeneration) return;
      if (!this.routingPolicy || BigInt(routing.data.revision) >= BigInt(this.routingPolicy.revision)) this.routingPolicy = routing.data;
      if (!this.deliveryPreference || BigInt(delivery.data.revision) >= BigInt(this.deliveryPreference.revision)) this.deliveryPreference = delivery.data;
      this.composer.setDeliveryPreference(this.deliveryPreference); this.defaults(); this.controls();
    } catch (error) { if (generation === this.defaultsGeneration) feedback('graph-defaults-status', errorMessage(error) + ' Refresh the saved defaults before a new start.', 'error'); }
  }
  private defaults(): void {
    const policy = this.policy ? policyName(this.policy.mode) + ' · revision ' + this.policy.revision : 'Saved autonomy has not been observed.';
    const budget = this.routingPolicy?.defaultBudget;
    element('graph-defaults-status').textContent = policy + (budget ? '. Shared finite allowance: ' + budget.admissionLimit + ' native admissions in ' + budget.windowSeconds + ' seconds, shared by the whole group; children receive no fresh allowance.' : ' Shared finite allowance has not been observed.')
      + ' Up to two workers, subject to context, account and group limits. No model planner is used.';
  }
  private async refresh(append = false): Promise<void> {
    if (!this.ready || !this.projectId || (append && !this.cursor)) return;
    const projectId = this.projectId, projectGeneration = this.projectGeneration, generation = ++this.listGeneration;
    const cursor = append ? this.cursor : null;
    try {
      const { data } = await api<Page<GraphSummary>>('/graphs?projectId=' + projectId + (cursor ? '&cursor=' + encodeURIComponent(cursor) : ''));
      if (this.projectId !== projectId || this.projectGeneration !== projectGeneration || this.listGeneration !== generation) return;
      if (!Array.isArray(data.items) || data.items.length > 100 || (data.nextCursor !== null && typeof data.nextCursor !== 'string') || data.items.some(graph => !id(graph.id) || graph.projectId !== projectId || !revision(graph.stateRevision))) throw new ApiError('invalid_response', 'The group list does not match this project.');
      if (!append) {
        const keep = this.detail?.graph; this.summaries.clear(); if (keep) this.summaries.set(keep.id, keep);
      }
      for (const graph of data.items) { const prior = this.summaries.get(graph.id); if (!prior || BigInt(graph.stateRevision) >= BigInt(prior.stateRevision)) this.summaries.set(graph.id, graph); }
      this.cursor = data.nextCursor; this.renderList();
      feedback('graph-list-status', this.summaries.size ? 'Groups for the selected project. Select one to inspect current state.' : 'No task groups are retained for this project. Standalone tasks remain available.');
    } catch (error) { if (this.projectId === projectId && this.projectGeneration === projectGeneration && this.listGeneration === generation) feedback('graph-list-status', errorMessage(error) + ' Previously loaded groups and your draft are retained.', 'error'); }
  }
  private async loadDefinition(detail: GraphDetail, projectGeneration: number, selection: number): Promise<void> {
    const generation = ++this.definitionGeneration, graph = detail.graph;
    try {
      const { data } = await api<GraphDefinitionView>('/graphs/' + graph.id + '/definition');
      if (!this.current(graph.projectId, projectGeneration, graph.id, selection) || generation !== this.definitionGeneration || this.detail?.graph.graphVersion !== graph.graphVersion) return;
      if (data.graphId !== graph.id || data.graphVersion !== graph.graphVersion || data.rootTaskId !== graph.rootTaskId || data.original?.projectId !== graph.projectId || data.delivery?.projectId !== graph.projectId
        || !Array.isArray(data.original.nodes) || data.original.nodes.length !== detail.nodes.length || data.original.nodes.some(node => !detail.nodes.some(item => item.key === node.key))) throw new ApiError('invalid_response', 'The retained definition does not match the selected task group.');
      this.definition = data; this.renderDetail();
      feedback('graph-definition-status', 'Retained definition ' + data.graphVersion + '. Original instructions are shown without summarizing or replacing them.');
    } catch (error) { if (this.current(graph.projectId, projectGeneration, graph.id, selection) && generation === this.definitionGeneration) feedback('graph-definition-status', errorMessage(error) + ' No definition from another group has been substituted.', 'error'); }
  }
  private observe(graphId: string): void {
    if (!this.detail || this.detail.graph.id !== graphId || !this.projectId) return;
    const generation = ++this.observationGeneration, projectId = this.projectId, projectGeneration = this.projectGeneration, selection = this.selectionGeneration;
    const loop = async (): Promise<void> => {
      while (generation === this.observationGeneration && this.current(projectId, projectGeneration, graphId, selection) && this.detail && observedWork(this.detail.graph)) {
        const after = this.detail.graph.stateRevision, detailGeneration = this.detailGeneration;
        try {
          const { data } = await api<GraphWaitView>('/graphs/' + graphId + '/wait?afterRevision=' + after + '&timeoutMs=20000');
          if (generation !== this.observationGeneration || !this.current(projectId, projectGeneration, graphId, selection)) return;
          if (detailGeneration !== this.detailGeneration) continue;
          checkDetail(data.graph, graphId, projectId); if (this.older(data.graph)) continue;
          const previousVersion = this.detail?.graph.graphVersion;
          this.detail = data.graph; this.summaries.set(graphId, data.graph.graph); this.renderList(); this.renderDetail();
          if (data.graph.graph.graphVersion !== previousVersion) await this.loadDefinition(data.graph, projectGeneration, selection);
          if (generation !== this.observationGeneration || !this.current(projectId, projectGeneration, graphId, selection)) return;
          feedback('graph-detail-status', 'Observed ' + data.graph.graph.state + '. Next action: ' + data.graph.graph.nextAction + ' · ' + data.graph.graph.nextOwner + '.');
          // A server may answer immediately while the caller already has this revision.
          if (!data.changed) await new Promise(resolve => setTimeout(resolve, 200));
        } catch (error) {
          if (generation === this.observationGeneration && this.current(projectId, projectGeneration, graphId, selection)) feedback('graph-detail-status', errorMessage(error) + ' Observation stopped. Refresh to read current state; nothing is retried automatically.', 'error');
          return;
        }
      }
    };
    void loop();
  }
  private async start(): Promise<void> {
    if (!this.ready || !graphStartRuntimeReady || !this.projectId || !this.policy || !this.routingPolicy || !this.deliveryPreference) return;
    try {
      const body = this.composer.body(requestId(), this.policy);
      body.routing = { ...body.routing, expectedPolicyRevision: this.routingPolicy.revision };
      if (this.pending.size >= 32) throw new ApiError('invalid_response', 'Reconcile an existing pending group request before adding another.');
      if ([...this.pending.values()].some(item => item.projectId === this.projectId && item.editorRevision === this.composer.revision)) throw new ApiError('invalid_response', 'This draft already has a pending start. Retry that exact request below.');
      const pending: PendingStart = { path: '/graphs', body, projectId: this.projectId, editorRevision: this.composer.revision,
        selectionGeneration: this.selectionGeneration, unresolved: false };
      this.pending.set(body.requestId, pending); this.persistPending(); this.renderPending(); await this.send(pending);
    } catch (error) { feedback('graph-start-status', errorMessage(error), 'error'); }
  }
  private async send(pending: PendingStart): Promise<void> {
    if (!this.ready || !graphStartRuntimeReady || this.busy.has(pending.body.requestId)) return;
    const key = pending.body.requestId, wasUnresolved = pending.unresolved;
    const projectGeneration = this.projectGeneration;
    pending.unresolved = true; this.busy.add(key); this.persistPending(); this.renderPending(); this.controls();
    try {
      const result = await api<GraphAdmission>(pending.path, 'POST', pending.body), data = result.data;
      if (result.requestId !== pending.body.requestId || result.receipt?.requestId !== pending.body.requestId || result.receipt.operation !== 'graphs.start'
        || !validAdmission(data, pending.body)) throw new ApiError('invalid_response', 'The acknowledgement does not identify this exact captured graph Start. Retry its retained request.', pending.body.requestId, undefined, true);
      // Reconcile this exact saved admission even if selection changed; its
      // historical authority cannot replace newer editor text or current state.
      if (this.pending.get(key) === pending) { this.pending.delete(key); this.persistPending(); }
      this.onTasksChanged(pending.projectId);
      if (this.projectId !== pending.projectId || projectGeneration !== this.projectGeneration) return;
      void this.refresh();
      if (this.selectionGeneration === pending.selectionGeneration && this.composer.revision === pending.editorRevision) {
        feedback('graph-start-status', 'Task group admitted. Preparation is recorded; a worker launch has not yet been observed.', 'success');
        if (id(data.graph?.id) && data.graph.projectId === pending.projectId) await this.open(data.graph.id);
      }
    } catch (error) {
      const uncertain = wasUnresolved || (error instanceof ApiError && error.uncertain);
      if (!uncertain && this.pending.get(key) === pending) { this.pending.delete(key); this.persistPending(); }
      if (this.projectId === pending.projectId && projectGeneration === this.projectGeneration && this.selectionGeneration === pending.selectionGeneration && this.composer.revision === pending.editorRevision)
        feedback('graph-start-status', errorMessage(error, key) + (uncertain ? ' Retry the exact retained request below. New typing is separate.' : ' Your original instructions and choices are retained.'), 'error');
    } finally { this.busy.delete(key); this.renderPending(); this.controls(); }
  }
  private persistPending(): void { writeSession('graphPendingStarts', [...this.pending.values()]); }
  private renderPending(): void {
    const target = element('graph-pending'); target.replaceChildren();
    for (const pending of this.pending.values()) {
      if (pending.projectId !== this.projectId) continue;
      const item = make('li'); item.className = 'graph-pending-item';
      item.append(make('p', 'Pending task-group start · request ' + pending.body.requestId));
      item.append(make('p', pending.body.nodes.length + ' tasks · ' + (pending.body.finalNodeKey ? 'final output ' + pending.body.finalNodeKey : 'Separate reviewable results') + '. These captured instructions are separate from the current editor.'));
      const details = make('details'); details.append(make('summary', 'Captured instructions'));
      if (pending.body.sharedInstructions !== undefined) details.append(make('pre', pending.body.sharedInstructions));
      for (const node of pending.body.nodes) { details.append(make('h5', node.key), make('pre', node.originalText)); } item.append(details);
      const retry = make('button', this.busy.has(pending.body.requestId) ? 'Reconciling…' : 'Retry exact group start'); retry.type = 'button'; retry.className = 'secondary';
      retry.disabled = !this.ready || !graphStartRuntimeReady || this.busy.has(pending.body.requestId); retry.addEventListener('click', () => { void this.send(pending); }); item.append(retry); target.append(item);
    }
    element('graph-pending-section').hidden = !target.childElementCount;
  }
  private renderList(): void {
    const target = element('graph-list'); target.replaceChildren();
    for (const graph of this.summaries.values()) {
      const item = make('li'), button = make('button'); button.type = 'button'; button.className = 'graph-list-button';
      button.append(make('strong', 'Task group · ' + graph.state), make('span', graph.id), make('span', 'Next: ' + graph.nextAction + ' · ' + graph.nextOwner + ' · ' + timeLabel(graph.updatedAt)));
      if (graph.id === this.selectedId) button.setAttribute('aria-current', 'true'); button.addEventListener('click', () => { void this.open(graph.id); }); item.append(button); target.append(item);
    }
    element('graph-more').hidden = !this.cursor;
  }
  private renderDetail(): void {
    this.recovery.setGraph(this.detail, this.recoveryReady);
    const target = element('graph-overview'), detail = this.detail; target.hidden = !detail;
    const focusedNodeControl = element('graph-node-list').contains(document.activeElement) ? (document.activeElement as HTMLElement | null)?.id : '';
    this.inputs.setGraph(detail); this.renderGraphControls();
    if (!detail) { element('graph-node-list').replaceChildren(); this.renderDefinition(); return; }
    const graph = detail.graph;
    element('graph-detail-heading').textContent = 'Task group · ' + graph.state;
    const facts = element('graph-facts'); facts.replaceChildren();
    addFact(facts, 'Group', graph.id); addFact(facts, 'Current definition', graph.graphVersion); addFact(facts, 'Next action', graph.nextAction + ' · ' + graph.nextOwner);
    addFact(facts, 'Required tasks', graph.counts.required + ' · completed ' + graph.counts.completed + ' · blocked ' + graph.counts.blocked);
    addFact(facts, 'Workers', 'Up to ' + graph.maxParallelism + '; occupied slots currently ' + detail.activeClaims.length + '. Context and account limits still apply.');
    const budget = detail.budget.budget;
    addFact(facts, 'Shared finite allowance', budget.consumedAdmissions + ' consumed / ' + budget.grantedAdmissions + ' granted · ' + budget.remainingAdmissions + ' remaining');
    addFact(facts, 'Shared window', budget.currentWindow.number + ' · ' + budget.currentWindow.windowSeconds + ' seconds · ' + (budget.currentWindow.deadlineAt ? 'deadline ' + timeLabel(budget.currentWindow.deadlineAt) : 'not started'));
    const breakdown = element('graph-budget-breakdown'); breakdown.replaceChildren();
    for (const node of detail.nodes) { const debit = detail.budget.nodeConsumption.find(item => item.nodeId === node.id); breakdown.append(make('li', node.key + ': ' + (debit?.consumedAdmissions ?? '0') + ' native admissions (' + (debit?.automaticAdmissions ?? '0') + ' automatic), included in the shared total.')); }
    const blockers = element('graph-blockers'); blockers.replaceChildren();
    for (const blocker of detail.blockers) blockers.append(make('li', blocker.message + ' · ' + blocker.code + (blocker.nodeId ? ' · node ' + (detail.nodes.find(node => node.id === blocker.nodeId)?.key ?? blocker.nodeId) : '')));
    const nodes = element('graph-node-list'); nodes.replaceChildren();
    for (const node of detail.nodes) {
      const card = make('li'); card.className = 'graph-node-card'; card.dataset.nodeKey = node.key; const title = make('h4', node.key + ' · ' + node.state); card.append(title);
      card.append(make('p', 'Current version ' + node.taskVersion + ' · next ' + node.nextAction + ' · ' + node.nextOwner));
      card.append(make('p', node.acceptedResult ? 'Accepted ' + node.acceptedResult.acceptanceKind + ' result · producer ' + node.acceptedResult.producerAttemptId + '. This acceptance alone does not imply file delivery.' : 'No accepted result is recorded for this node.'));
      const dependencies = this.definition?.original.dependencies?.filter(edge => edge.to === node.key) ?? [];
      if (dependencies.length) card.append(make('p', 'Depends on: ' + dependencies.map(edge => edge.from + ' (' + (edge.kind === 'context' ? 'context only' : 'verified files') + ')').join(', ')));
      const open = make('button', 'Open task ' + node.key); open.id = 'graph-node-open-' + node.id; open.type = 'button'; open.className = 'secondary';
      open.addEventListener('click', () => this.onOpenTask(node.taskId, { graphId: graph.id, rootTaskId: graph.rootTaskId, nodeId: node.id, kind: 'node' })); card.append(open);
      const inspect = make('button', 'Inputs and context ' + node.key); inspect.id = 'graph-node-inputs-' + node.id; inspect.type = 'button'; inspect.className = 'secondary'; inspect.disabled = !this.ready; inspect.addEventListener('click', () => { void this.inputs.open(node); }); card.append(inspect);
      const stop = make('button', 'Stop node ' + node.key); stop.id = 'graph-node-stop-' + node.id; stop.type = 'button'; stop.className = 'secondary'; stop.dataset.graphNodeStop = node.id; stop.disabled = !this.ready || !!node.stop; stop.addEventListener('click', () => { void this.captureControl('node-stop', node); }); card.append(stop);
      nodes.append(card);
    }
    this.renderDefinition(); this.controls();
    if (focusedNodeControl) {
      const restored = document.getElementById(focusedNodeControl);
      if (restored instanceof HTMLButtonElement && !restored.disabled) restored.focus({ preventScroll: true });
      else { const heading = element('graph-detail-heading'); heading.tabIndex = -1; heading.focus({ preventScroll: true }); }
    }
  }
  private renderDefinition(): void {
    const target = element('graph-definition'), definition = this.definition, detail = this.detail;
    if (!definition || !detail || definition.graphId !== detail.graph.id || definition.graphVersion !== detail.graph.graphVersion || definition.rootTaskId !== detail.graph.rootTaskId) {
      this.renderedDefinitionKey = null; target.replaceChildren(); return;
    }
    const key = definition.graphId + ':' + definition.rootTaskId + ':' + definition.graphVersion + ':' + detail.definition.declarationSha256;
    // Observation changes node/workflow state, not this immutable definition.
    // Keep its actual summary/PRE nodes, open state and user focus intact.
    if (key === this.renderedDefinitionKey) return;
    target.replaceChildren();
    if (definition.original.title !== undefined) target.append(make('h4', definition.original.title || '(Explicit empty title)'));
    target.append(make('p', definition.delivery.localMode === 'apply_verified_local' ? 'Only the selected final task may deliver a verified cumulative local change. Intermediate results remain review only.' : 'Separate reviewable results / review-only local scope. No local application is authorized.'));
    if (definition.delivery.externalAction) target.append(make('p', 'Explicit final-only external action: ' + definition.delivery.externalAction.action + ' → ' + definition.delivery.externalAction.destination));
    if (definition.original.sharedInstructions !== undefined) { target.append(make('h4', 'Complete shared original instructions'), make('pre', definition.original.sharedInstructions)); }
    for (const node of definition.original.nodes) {
      const details = make('details'); details.append(make('summary', node.key + ' · complete original instructions'), make('pre', node.originalText));
      if (node.acceptance?.conditionsText !== undefined) details.append(make('h5', 'Acceptance conditions'), make('pre', node.acceptance.conditionsText)); target.append(details);
    }
    this.renderedDefinitionKey = key;
  }
  private mountControls(): void {
    this.controlPanel.id = 'graph-current-controls'; this.controlPanel.append(make('h4', 'Current group controls'));
    this.stopGroup.id = 'graph-stop-group'; this.stopGroup.type = 'button'; this.stopGroup.className = 'secondary';
    this.stopGroup.addEventListener('click', () => { void this.captureControl('stop'); });
    this.controlPanel.append(this.stopGroup, make('p', 'Stop targets the current group. Cancellation, actual effects and cleanup remain separately observed.'));
    this.advance.id = 'graph-advance'; this.advance.type = 'button'; this.advance.className = 'secondary';
    this.advance.addEventListener('click', () => { void this.captureControl('advance'); });
    this.controlPanel.append(this.advance, make('p', 'Advance captures the currently displayed versions, controls and shared window for eligible unfinished work. It grants no units or renewed window, and never replays a completed execution. Only an explicit updated-input selection below can derive a new task version.'));
    this.rebindChoices.id = 'graph-rebind-nodes'; this.rebindAdvance.id = 'graph-rebind-advance'; this.rebindAdvance.type = 'button'; this.rebindAdvance.className = 'secondary';
    this.rebindAdvance.addEventListener('click', () => { void this.captureControl('advance', undefined, true); });
    this.rebindReceiptList.id = 'graph-rebind-receipts';
    this.controlPanel.append(make('h5', 'Stale dependency inputs'),
      make('p', 'Explicitly choose a stale receiver below. Rebinding preserves its instructions and captured choices in a derived task version; it does not edit old workflow authority. Advance may prepare or admit eligible unfinished work under the captured shared allowance. No units or renewed window are added.'),
      this.rebindChoices, this.rebindAdvance, this.rebindReceiptList);
    this.guidanceNode.id = 'graph-guidance-node'; this.guidanceText.id = 'graph-guidance-text'; this.guidanceText.rows = 4;
    const nodeLabel = make('label', 'Current task for guidance'); nodeLabel.htmlFor = this.guidanceNode.id;
    const textLabel = make('label', 'Complete replacement guidance'); textLabel.htmlFor = this.guidanceText.id;
    this.guidanceCurrent.id = 'graph-guidance-current';
    this.guidanceSave.id = 'graph-save-guidance'; this.guidanceSave.type = 'button'; this.guidanceSave.className = 'secondary';
    this.guidanceNode.addEventListener('change', () => {
      if (this.selectedId) this.guidanceSelections.set(this.selectedId, this.guidanceNode.value);
      ++this.intentGeneration; this.controlStatus.textContent = 'Guidance target changed locally; no version has been saved.'; this.renderGuidance();
    });
    this.guidanceText.addEventListener('input', () => {
      const graph = this.detail?.graph, node = this.detail?.nodes.find(item => item.id === this.guidanceNode.value);
      if (!graph || !node) return;
      this.guidanceDrafts.set(graph.id + ':' + node.id, { text: this.guidanceText.value, edited: true, version: node.taskVersion });
      ++this.intentGeneration; this.controlStatus.textContent = 'Guidance draft changed locally; no version has been saved.'; this.updateGraphControls();
    });
    this.guidanceSave.addEventListener('click', () => { const node = this.detail?.nodes.find(item => item.id === this.guidanceNode.value); if (node) void this.captureControl('guidance', node); });
    this.controlPanel.append(make('h5', 'Node guidance'), nodeLabel, this.guidanceNode, this.guidanceCurrent, textLabel, this.guidanceText, this.guidanceSave,
      make('p', 'Saving appends a current task version and invalidates affected input authority. An explicit empty edit clears guidance. Original instructions and omitted task options stay captured. Saving starts no worker and adds no allowance.'));
    this.grantAdmissions.id = 'graph-grant-admissions'; this.grantAdmissions.inputMode = 'numeric'; this.grantAdmissions.autocomplete = 'off';
    this.grantWindow.id = 'graph-grant-window'; this.grantWindow.inputMode = 'numeric'; this.grantWindow.autocomplete = 'off';
    const admissionsLabel = make('label', 'Additional shared native admissions (1–16)'); admissionsLabel.htmlFor = this.grantAdmissions.id;
    const windowLabel = make('label', 'Replacement window seconds (optional, 1–86400)'); windowLabel.htmlFor = this.grantWindow.id;
    this.grantButton.id = 'graph-grant-budget'; this.grantButton.type = 'button'; this.grantButton.className = 'secondary'; this.grantButton.addEventListener('click', () => { void this.captureControl('grant'); });
    const edit = (): void => {
      if (!this.selectedId) return;
      this.grantDrafts.set(this.selectedId, { admissions: this.grantAdmissions.value, window: this.grantWindow.value }); ++this.intentGeneration;
      this.controlStatus.textContent = 'Grant draft changed locally; no additional authority has been recorded.'; this.updateGraphControls();
    };
    this.grantAdmissions.addEventListener('input', edit); this.grantWindow.addEventListener('input', edit);
    this.controlPanel.append(make('h5', 'Shared budget authority'), admissionsLabel, this.grantAdmissions, windowLabel, this.grantWindow, this.grantButton,
      make('p', 'One grant applies to the shared root allowance. Node debits are included in its total. A grant starts no work and does not refresh a live execution registration.'));
    this.controlStatus.id = 'graph-control-status'; this.controlStatus.className = 'feedback'; this.controlStatus.setAttribute('role', 'status'); this.controlStatus.setAttribute('aria-live', 'polite'); this.controlPanel.append(this.controlStatus);
    this.ledgerLoad.id = 'graph-ledger-load'; this.ledgerLoad.type = 'button'; this.ledgerLoad.className = 'secondary'; this.ledgerLoad.addEventListener('click', () => { void this.loadLedger(false); });
    this.ledgerMore.id = 'graph-ledger-more'; this.ledgerMore.type = 'button'; this.ledgerMore.className = 'secondary'; this.ledgerMore.addEventListener('click', () => { void this.loadLedger(true); });
    this.ledgerList.id = 'graph-ledger-list'; this.ledgerStatus.id = 'graph-ledger-status'; this.ledgerStatus.className = 'feedback'; this.ledgerStatus.setAttribute('role', 'status');
    this.controlPanel.append(this.ledgerLoad, this.ledgerList, this.ledgerMore, this.ledgerStatus);
    element('graph-overview').insertBefore(this.controlPanel, element('graph-node-list'));
    const pending = make('section'); pending.id = 'graph-control-pending-section'; pending.append(make('h4', 'Pending graph controls'), make('p', 'Retry preserves the captured target, proof and request. New field edits are separate. Browser recovery is best effort; the saved owner receipt determines the outcome.'));
    this.controlPendingList.id = 'graph-control-pending'; pending.append(this.controlPendingList); element('graph-pending-section').insertAdjacentElement('afterend', pending);
  }
  private controlKey(kind: PendingControl['kind'], graphId: string, nodeId: string | null = null): string { return kind + ':' + graphId + ':' + (nodeId ?? 'group'); }
  private validPendingControl(item: PendingControl): boolean {
    if (!item || !['stop', 'node-stop', 'grant', 'guidance', 'advance'].includes(item.kind) || !id(item.projectId) || !id(item.graphId) || !id(item.rootTaskId)
      || ![item.selectionGeneration, item.detailGeneration, item.intentGeneration].every(value => Number.isSafeInteger(value) && value >= 0) || typeof item.unresolved !== 'boolean'
      || !item.body || typeof item.body !== 'object' || Array.isArray(item.body)) return false;
    if (item.kind !== 'advance' && item.rebindTargets !== undefined) return false;
    const body = item.body as unknown as Record<string, unknown>;
    if (!id(body['requestId']) || !revision(body['expectedGraphControlRevision'])) return false;
    let allowed: string[];
    if (item.kind === 'node-stop' || item.kind === 'guidance') {
      if (!id(item.nodeId) || typeof item.nodeKey !== 'string' || !/^[A-Za-z0-9][A-Za-z0-9_-]{0,47}$/.test(item.nodeKey) || !revision(item.taskVersion)
        || body['nodeId'] !== item.nodeId || !revision(body['expectedNodeControlRevision'])) return false;
      if (item.kind === 'node-stop') {
        if (item.operation !== 'graphs.nodes.stop' || item.path !== '/graphs/' + item.graphId + '/nodes/' + item.nodeKey + '/stop') return false;
        allowed = ['requestId', 'expectedGraphControlRevision', 'nodeId', 'expectedNodeControlRevision'];
      } else {
        if (!id(item.taskId) || body['taskVersion'] !== item.taskVersion || !revision(body['expectedRevision']) || typeof body['guidanceText'] !== 'string'
          || new TextEncoder().encode(body['guidanceText']).byteLength > 4 * 1024 * 1024 || item.operation !== 'graphs.nodes.guidance'
          || item.path !== '/graphs/' + item.graphId + '/nodes/' + item.nodeKey + '/guidance') return false;
        allowed = ['requestId', 'expectedGraphControlRevision', 'nodeId', 'expectedNodeControlRevision', 'taskVersion', 'expectedRevision', 'guidanceText'];
      }
    } else {
      if (item.nodeId !== null || item.nodeKey !== null || item.taskVersion !== null) return false;
      if (item.kind === 'stop') {
        if (item.operation !== 'graphs.stop' || item.path !== '/graphs/' + item.graphId + '/stop') return false;
        allowed = ['requestId', 'expectedGraphControlRevision'];
      } else if (item.kind === 'advance') {
        const nodes = body['nodeAuthorities'];
        if (item.operation !== 'graphs.advance' || item.path !== '/graphs/' + item.graphId + '/advance' || !revision(body['graphVersion'])
          || !revision(body['expectedBudgetAuthorityEpoch']) || !revision(body['expectedWindowNumber']) || !Array.isArray(nodes) || nodes.length < 1 || nodes.length > 16) return false;
        const seen = new Set<string>();
        for (const node of nodes) {
          if (!node || typeof node !== 'object' || Array.isArray(node) || !id(node.nodeId) || seen.has(node.nodeId) || !revision(node.taskVersion)
            || !revision(node.nodeControlRevision) || Object.keys(node).some(key => !['nodeId', 'taskVersion', 'nodeControlRevision'].includes(key))) return false;
          seen.add(node.nodeId);
        }
        const rebound = body['rebindNodeIds'];
        if (rebound === undefined) { if (item.rebindTargets !== undefined) return false; }
        else {
          if (!Array.isArray(rebound) || rebound.length < 1 || rebound.length > nodes.length || !rebound.every(id) || new Set(rebound).size !== rebound.length
            || !Array.isArray(item.rebindTargets) || item.rebindTargets.length !== rebound.length) return false;
          const targets = new Set<string>();
          for (const target of item.rebindTargets) {
            const authority = nodes.find(node => node.nodeId === target?.nodeId);
            if (!target || !id(target.nodeId) || !rebound.includes(target.nodeId) || targets.has(target.nodeId) || !id(target.taskId) || !id(target.inputId)
              || typeof target.nodeKey !== 'string' || !/^[A-Za-z0-9][A-Za-z0-9_-]{0,47}$/.test(target.nodeKey) || !authority || target.taskVersion !== authority.taskVersion
              || Object.keys(target).some(key => !['nodeId', 'nodeKey', 'taskId', 'taskVersion', 'inputId'].includes(key))) return false;
            targets.add(target.nodeId);
          }
        }
        allowed = ['requestId', 'expectedGraphControlRevision', 'graphVersion', 'expectedBudgetAuthorityEpoch', 'expectedWindowNumber', 'nodeAuthorities', 'rebindNodeIds'];
      } else {
        if (item.operation !== 'graphs.budget.amend' || item.path !== '/graphs/' + item.graphId + '/budget-amendments' || !revision(body['expectedBudgetAuthorityEpoch']) || !revision(body['expectedWindowNumber'])
          || !revision(body['additionalAdmissions']) || BigInt(body['additionalAdmissions']) > 16n || body['newWindowSeconds'] !== undefined && (!Number.isSafeInteger(body['newWindowSeconds']) || (body['newWindowSeconds'] as number) < 1 || (body['newWindowSeconds'] as number) > 86400)) return false;
        allowed = ['requestId', 'expectedGraphControlRevision', 'expectedBudgetAuthorityEpoch', 'expectedWindowNumber', 'additionalAdmissions', 'newWindowSeconds'];
      }
    }
    return Object.keys(body).every(key => allowed.includes(key));
  }
  private renderGraphControls(): void {
    const graphId = this.detail?.graph.id ?? null; this.controlPanel.hidden = !graphId;
    if (graphId !== this.controlsGraphId) {
      this.controlsGraphId = graphId; ++this.ledgerGeneration; this.ledgerBusy = false; this.ledgerCursor = null; this.ledgerRows.clear(); this.ledgerList.replaceChildren(); this.ledgerStatus.textContent = ''; this.controlStatus.textContent = '';
      const draft = graphId ? this.grantDrafts.get(graphId) : undefined; this.grantAdmissions.value = draft?.admissions ?? ''; this.grantWindow.value = draft?.window ?? '';
    }
    this.renderGuidance(); this.renderRebind(); this.updateGraphControls();
  }

  private renderGuidance(): void {
    const detail = this.detail, prior = detail ? this.guidanceSelections.get(detail.graph.id) ?? this.guidanceNode.value : '';
    this.guidanceNode.replaceChildren();
    for (const node of detail?.nodes ?? []) this.guidanceNode.append(new Option(node.key + ' · current version ' + node.taskVersion, node.id));
    if (!detail) { this.guidanceText.value = ''; this.guidanceCurrent.textContent = ''; this.updateGraphControls(); return; }
    const node = detail.nodes.find(item => item.id === prior) ?? detail.nodes[0];
    if (!node) { this.updateGraphControls(); return; }
    this.guidanceNode.value = node.id; this.guidanceSelections.set(detail.graph.id, node.id);
    const key = detail.graph.id + ':' + node.id;
    let draft = this.guidanceDrafts.get(key);
    if (!draft || !draft.edited && draft.version !== node.taskVersion) {
      draft = { text: '', edited: false, version: node.taskVersion }; this.guidanceDrafts.set(key, draft);
      void this.readGuidance(detail.graph.id, node, draft);
    }
    this.guidanceText.value = draft.text;
    this.guidanceCurrent.textContent = 'Captured target: ' + node.key + ' · current version ' + node.taskVersion + ' · task revision ' + node.taskRevision
      + (draft.edited ? '. Local replacement draft; saving captures these current controls.' : '. Reading retained guidance; edit explicitly before saving.');
    this.updateGraphControls();
  }

  private async readGuidance(graphId: string, node: GraphNodeSummary, draft: GuidanceDraft): Promise<void> {
    try {
      const { data } = await api<{ taskId: string; originalText: string; version: TaskVersion }>('/tasks/' + node.taskId + '/versions/' + node.taskVersion);
      if (data?.taskId !== node.taskId || data.version?.version !== node.taskVersion || typeof data.version.guidanceText !== 'string') throw new ApiError('invalid_response', 'Retained guidance does not match this exact task version.');
      if (this.guidanceDrafts.get(graphId + ':' + node.id) !== draft || draft.edited) return;
      draft.text = data.version.guidanceText;
      if (this.detail?.graph.id === graphId && this.guidanceNode.value === node.id && this.detail.nodes.some(current => current.id === node.id && current.taskVersion === node.taskVersion)) {
        this.guidanceText.value = draft.text; this.guidanceCurrent.textContent = 'Retained guidance for ' + node.key + ' · current version ' + node.taskVersion + '. Edit explicitly to replace it; an empty edit clears it.';
      }
    } catch (error) {
      if (this.guidanceDrafts.get(graphId + ':' + node.id) === draft && !draft.edited && this.detail?.graph.id === graphId && this.guidanceNode.value === node.id)
        this.guidanceCurrent.textContent = errorMessage(error) + ' A complete replacement may still be typed explicitly; no guidance has been saved.';
    }
  }
  private rebindKey(node: GraphNodeSummary): string {
    return node.id + ':' + node.taskVersion + ':' + node.inputId;
  }
  private rebindNodes(detail: GraphDetail): GraphNodeSummary[] {
    if (detail.graph.stop) return [];
    return detail.nodes.filter(node => node.state === 'blocked' && !node.stop && node.acceptedResult === null && !!node.inputId && !!node.nodeAdmissionId
      && !detail.activeClaims.some(claim => claim.nodeId === node.id)
      && detail.blockers.some(blocker => blocker.nodeId === node.id && blocker.code === 'graph_input_stale'));
  }
  private selectedRebindNodes(detail: GraphDetail): GraphNodeSummary[] {
    const selected = this.rebindSelections.get(detail.graph.id);
    return this.rebindNodes(detail).filter(node => selected?.has(this.rebindKey(node)));
  }
  private renderRebind(): void {
    const detail = this.detail, focused = this.rebindChoices.contains(document.activeElement) ? (document.activeElement as HTMLElement)?.id : '';
    this.rebindChoices.replaceChildren(); this.rebindReceiptList.replaceChildren();
    if (!detail) return;
    const eligible = this.rebindNodes(detail), selected = this.rebindSelections.get(detail.graph.id) ?? new Set<string>();
    this.rebindSelections.set(detail.graph.id, selected);
    const choices: Array<{ target: RebindTarget; pending: boolean }> = eligible.map(node => ({ target: { nodeId: node.id, nodeKey: node.key, taskId: node.taskId, taskVersion: node.taskVersion, inputId: node.inputId! }, pending: false }));
    const retained = this.controlPending.get(this.controlKey('advance', detail.graph.id));
    for (const target of retained?.rebindTargets ?? []) {
      const index = choices.findIndex(choice => choice.target.nodeId === target.nodeId);
      if (index >= 0) choices.splice(index, 1, { target, pending: true }); else choices.push({ target, pending: true });
    }
    // A live read can observe the committed derived version before its held POST
    // acknowledgement arrives. Keep the captured choice available until that
    // exact request reconciles; it never becomes a new version's authority.
    const current = new Set(choices.map(choice => choice.target.nodeId + ':' + choice.target.taskVersion + ':' + choice.target.inputId));
    for (const key of selected) if (!current.has(key)) selected.delete(key);
    if (!choices.length) this.rebindChoices.append(make('p', 'No settled stale receiver is currently available for explicit rebinding. Refresh reads current state; it never opts a node into rebinding.'));
    for (const { target, pending } of choices) {
      const key = target.nodeId + ':' + target.taskVersion + ':' + target.inputId;
      const choice = make('input'); choice.type = 'checkbox'; choice.id = 'graph-rebind-node-' + target.nodeId; choice.dataset.graphRebindNode = target.nodeId;
      choice.checked = selected.has(key); choice.disabled = !this.ready;
      const label = make('label'); label.className = 'checkbox-label graph-rebind-choice'; label.htmlFor = choice.id;
      label.append(choice, make('span', target.nodeKey + ' · version ' + target.taskVersion + ' · earlier input ' + target.inputId
        + (pending ? '. Captured in the pending request. This local selection can change; exact retry still sends only the saved request, never newer versions.' : '. Use updated accepted predecessor inputs in a derived version; the service rechecks eligibility.')));
      choice.addEventListener('change', () => {
        if (choice.checked) selected.add(key); else selected.delete(key);
        ++this.intentGeneration; this.controlStatus.textContent = 'Rebind selection changed locally. This edit changes no saved request or execution authority.'; this.updateGraphControls();
      });
      this.rebindChoices.append(label);
    }
    for (const receipt of this.rebindReceipts.values()) {
      if (receipt.graphId !== detail.graph.id) continue;
      for (const rebound of receipt.nodes) {
        const target = receipt.targets.find(item => item.nodeId === rebound.nodeId); if (!target) continue;
        const item = make('li'); item.className = 'graph-rebind-receipt';
        item.append(make('p', 'Acknowledged ' + target.nodeKey + ': version ' + rebound.fromTaskVersion + ' → version ' + rebound.toTaskVersion
          + ' · request ' + receipt.requestId + ' · dependency_rebind · earlier input ' + rebound.provenance.previousInputId + '. This is not proof of worker launch; input preparation and execution are separately observed.'));
        const input = make('button', 'Inspect earlier input ' + target.inputId); input.type = 'button'; input.className = 'quiet compact';
        input.addEventListener('click', () => {
          const node = this.detail?.graph.id === receipt.graphId ? this.detail.nodes.find(node => node.id === target.nodeId && node.taskId === target.taskId) : undefined;
          if (node) void this.inputs.open({ ...node, inputId: target.inputId });
        });
        const history = make('button', 'Open task history ' + target.nodeKey); history.type = 'button'; history.className = 'quiet compact';
        history.addEventListener('click', () => this.onOpenTask(target.taskId, { graphId: detail.graph.id, rootTaskId: detail.graph.rootTaskId, nodeId: target.nodeId, kind: 'node' }));
        item.append(input, history); this.rebindReceiptList.append(item);
      }
    }
    this.rebindReceiptList.hidden = !this.rebindReceiptList.childElementCount;
    if (focused) document.getElementById(focused)?.focus({ preventScroll: true });
  }

  private updateGraphControls(): void {
    const detail = this.detail, available = this.ready && !!detail;
    const key = detail ? this.controlKey('grant', detail.graph.id) : '';
    const pending = this.controlPending.get(key);
    const units = this.grantAdmissions.value, seconds = this.grantWindow.value;
    const valid = revision(units) && BigInt(units) <= 16n && (seconds === '' || /^[1-9][0-9]{0,4}$/.test(seconds) && Number(seconds) <= 86400);
    this.stopGroup.disabled = !available || !!detail?.graph.stop || !!detail && this.controlBusy.has(this.controlKey('stop', detail.graph.id));
    this.grantButton.disabled = !available || !!detail?.graph.stop || !!detail && ['completed', 'cancelled', 'cancelling'].includes(detail.graph.state) || this.controlBusy.has(key) || !pending && !valid;
    this.grantButton.textContent = pending ? 'Retry pending Grant · captured request ' + pending.body.requestId.slice(0, 8) : 'Grant shared authority';
    this.grantAdmissions.disabled = this.grantWindow.disabled = !available;
    const advanceKey = detail ? this.controlKey('advance', detail.graph.id) : '', advancePending = this.controlPending.get(advanceKey);
    this.advance.disabled = !available || this.controlBusy.has(advanceKey) || !advancePending && (!!detail?.graph.stop || !!detail && ['completed', 'cancelled', 'cancelling'].includes(detail.graph.state));
    this.advance.textContent = advancePending ? 'Retry pending ' + ((advancePending.body as GraphAdvanceInput).rebindNodeIds?.length ? 'Rebind and Advance' : 'Advance') + ' · captured request ' + advancePending.body.requestId.slice(0, 8) : 'Advance graph';
    const pendingRebind = advancePending && (advancePending.body as GraphAdvanceInput).rebindNodeIds?.length;
    this.rebindAdvance.disabled = !available || this.controlBusy.has(advanceKey) || !!advancePending && !pendingRebind || !advancePending && (!detail || this.selectedRebindNodes(detail).length === 0);
    this.rebindAdvance.textContent = pendingRebind ? 'Retry pending Rebind and Advance · captured request ' + advancePending.body.requestId.slice(0, 8) : 'Use updated predecessor inputs and advance';
    for (const choice of this.rebindChoices.querySelectorAll<HTMLInputElement>('[data-graph-rebind-node]')) choice.disabled = !available;
    const node = detail?.nodes.find(item => item.id === this.guidanceNode.value), guidanceKey = detail && node ? this.controlKey('guidance', detail.graph.id, node.id) : '';
    const guidancePending = this.controlPending.get(guidanceKey), draft = detail && node ? this.guidanceDrafts.get(detail.graph.id + ':' + node.id) : undefined;
    this.guidanceNode.disabled = this.guidanceText.disabled = !available || !node;
    this.guidanceSave.disabled = !available || !node || this.controlBusy.has(guidanceKey) || !guidancePending && (!!detail?.graph.stop || !!node.stop || !draft?.edited);
    this.guidanceSave.textContent = guidancePending ? 'Retry pending Guidance · captured request ' + guidancePending.body.requestId.slice(0, 8) : 'Save node guidance';
    this.ledgerLoad.disabled = !available || this.ledgerBusy; this.ledgerMore.hidden = !this.ledgerCursor; this.ledgerMore.disabled = !available || this.ledgerBusy;
    for (const button of document.querySelectorAll<HTMLButtonElement>('[data-graph-node-stop]')) { const node = detail?.nodes.find(item => item.id === button.dataset.graphNodeStop); button.disabled = !available || !node || !!node.stop || !!detail && this.controlBusy.has(this.controlKey('node-stop', detail.graph.id, node?.id ?? null)); }
  }
  private async captureControl(kind: PendingControl['kind'], node?: GraphNodeSummary, useUpdatedInputs = false): Promise<void> {
    const detail = this.detail; if (!this.ready || !this.projectId || !detail) return;
    const key = this.controlKey(kind, detail.graph.id, node?.id ?? null), retained = this.controlPending.get(key);
    if (retained) { await this.sendControl(retained); return; }
    try {
      if (this.controlPending.size >= 32) throw new ApiError('invalid_request', 'Reconcile a retained graph control before adding another.');
      if (node && !detail.nodes.some(current => current.id === node.id && current.key === node.key && current.taskVersion === node.taskVersion && current.controlRevision === node.controlRevision)) throw new ApiError('graph_input_stale', 'The current node controls changed. Read them again before choosing a new control.');
      const identity = requestId(); let body: PendingControl['body']; let rebindTargets: RebindTarget[] | undefined;
      if (kind === 'stop') body = { requestId: identity, expectedGraphControlRevision: detail.graph.controlRevision };
      else if (kind === 'node-stop' && node) body = { requestId: identity, expectedGraphControlRevision: detail.graph.controlRevision, nodeId: node.id, expectedNodeControlRevision: node.controlRevision };
      else if (kind === 'guidance' && node) {
        const draft = this.guidanceDrafts.get(detail.graph.id + ':' + node.id);
        if (!draft?.edited || this.guidanceNode.value !== node.id) throw new ApiError('invalid_request', 'Edit the complete guidance for this selected current node before saving.');
        if (new TextEncoder().encode(draft.text).byteLength > 4 * 1024 * 1024) throw new ApiError('invalid_request', 'Complete guidance exceeds the 4 MiB bound. Reduce it explicitly; no text was clipped.');
        body = { requestId: identity, expectedGraphControlRevision: detail.graph.controlRevision, nodeId: node.id, expectedNodeControlRevision: node.controlRevision,
          taskVersion: node.taskVersion, expectedRevision: node.taskRevision, guidanceText: draft.text };
      } else if (kind === 'advance') {
        const selected = useUpdatedInputs ? this.selectedRebindNodes(detail) : [];
        if (useUpdatedInputs && !selected.length) throw new ApiError('graph_input_stale', 'Explicitly select an eligible stale receiver before using updated predecessor inputs.');
        if (selected.length) rebindTargets = selected.map(node => ({ nodeId: node.id, nodeKey: node.key, taskId: node.taskId, taskVersion: node.taskVersion, inputId: node.inputId! }));
        body = { requestId: identity, graphVersion: detail.graph.graphVersion, expectedGraphControlRevision: detail.graph.controlRevision,
          expectedBudgetAuthorityEpoch: detail.graph.budgetAuthorityEpoch, expectedWindowNumber: detail.budget.budget.currentWindow.number,
          nodeAuthorities: detail.nodes.map(current => ({ nodeId: current.id, taskVersion: current.taskVersion, nodeControlRevision: current.controlRevision })),
          ...(rebindTargets ? { rebindNodeIds: rebindTargets.map(target => target.nodeId) } : {}) };
      } else if (kind === 'grant') {
        const units = this.grantAdmissions.value, seconds = this.grantWindow.value;
        if (!revision(units) || BigInt(units) > 16n || seconds !== '' && (!/^[1-9][0-9]{0,4}$/.test(seconds) || Number(seconds) > 86400)) throw new ApiError('invalid_request', 'Use 1–16 additional admissions and an optional whole replacement window from 1 to 86400 seconds.');
        body = { requestId: identity, expectedGraphControlRevision: detail.graph.controlRevision, expectedBudgetAuthorityEpoch: detail.graph.budgetAuthorityEpoch, expectedWindowNumber: detail.budget.budget.currentWindow.number, additionalAdmissions: units, ...(seconds ? { newWindowSeconds: Number(seconds) } : {}) };
      } else return;
      const suffix = kind === 'stop' ? '/stop' : kind === 'node-stop' ? '/nodes/' + node!.key + '/stop' : kind === 'guidance' ? '/nodes/' + node!.key + '/guidance' : kind === 'advance' ? '/advance' : '/budget-amendments';
      const operation = kind === 'stop' ? 'graphs.stop' : kind === 'node-stop' ? 'graphs.nodes.stop' : kind === 'guidance' ? 'graphs.nodes.guidance' : kind === 'advance' ? 'graphs.advance' : 'graphs.budget.amend';
      const pending: PendingControl = { kind, path: '/graphs/' + detail.graph.id + suffix, operation,
        projectId: this.projectId, graphId: detail.graph.id, rootTaskId: detail.graph.rootTaskId, nodeId: node?.id ?? null, nodeKey: node?.key ?? null, taskVersion: node?.taskVersion ?? null,
        ...(kind === 'guidance' ? { taskId: node!.taskId } : {}), ...(rebindTargets ? { rebindTargets } : {}),
        body: Object.freeze(body), selectionGeneration: this.selectionGeneration, detailGeneration: this.detailGeneration, intentGeneration: ++this.intentGeneration, unresolved: false };
      if (!this.validPendingControl(pending)) throw new ApiError('invalid_response', 'The displayed controls do not form a complete bounded request. Read the group again.');
      this.controlPending.set(key, pending); this.persistControls(); this.renderControlPending(); await this.sendControl(pending);
    } catch (error) { this.controlStatus.textContent = errorMessage(error) + ' Current fields are retained; no replacement request was sent.'; }
  }
  private persistControls(): void { writeSession('graphPendingControls', [...this.controlPending.values()]); }
  private renderControlPending(): void {
    const active = document.activeElement;
    const focusedRow = active instanceof HTMLButtonElement && this.controlPendingList.contains(active) ? active.closest('li') : null;
    const focusedKey = focusedRow?.dataset.graphControlKey, focusedRequestId = focusedRow?.dataset.graphControlRequestId;
    let restoreRetry: HTMLButtonElement | undefined;
    this.controlPendingList.replaceChildren();
    for (const pending of this.controlPending.values()) {
      if (pending.projectId !== this.projectId) continue;
      const key = this.controlKey(pending.kind, pending.graphId, pending.nodeId), title = pending.kind === 'stop' ? 'Stop group' : pending.kind === 'node-stop' ? 'Stop node ' + pending.nodeKey : pending.kind === 'guidance' ? 'Guidance node ' + pending.nodeKey : pending.kind === 'advance' ? ((pending.body as GraphAdvanceInput).rebindNodeIds?.length ? 'Rebind and Advance' : 'Advance') : 'Grant';
      const row = make('li'); row.dataset.graphControlKey = key; row.dataset.graphControlRequestId = pending.body.requestId;
      row.append(make('p', 'Pending ' + title + ' · graph ' + pending.graphId + ' · request ' + pending.body.requestId));
      const captured = make('details'); captured.append(make('summary', 'Captured target and request'), make('pre', JSON.stringify(pending.body, null, 2))); row.append(captured);
      const busy = this.controlBusy.has(key), retry = make('button', busy ? 'Reconciling ' + title + '…' : 'Retry exact ' + title);
      retry.type = 'button'; retry.className = 'secondary'; retry.disabled = !this.ready;
      // Busy retries remain a focusable, announced action. sendControl's exact
      // busy-key guard prevents duplicate dispatch while the reply is pending.
      retry.setAttribute('aria-disabled', String(!this.ready || busy));
      retry.addEventListener('click', () => { void this.sendControl(pending); }); row.append(retry); this.controlPendingList.append(row);
      if (focusedKey === key && focusedRequestId === pending.body.requestId) restoreRetry = retry;
    }
    element('graph-control-pending-section').hidden = !this.controlPendingList.childElementCount;
    // Only repair focus displaced synchronously by this render. An actual user
    // move outside the pending request is never a reason to restore it later.
    if (restoreRetry && !restoreRetry.disabled && document.activeElement === document.body) restoreRetry.focus({ preventScroll: true });
  }
  private controlPaintCurrent(pending: PendingControl): boolean {
    return this.projectId === pending.projectId && this.selectedId === pending.graphId && this.selectionGeneration === pending.selectionGeneration && this.detailGeneration === pending.detailGeneration && this.intentGeneration === pending.intentGeneration
      && (!pending.nodeId || this.detail?.nodes.some(node => node.id === pending.nodeId && node.taskVersion === pending.taskVersion) === true)
      && (pending.kind !== 'guidance' || this.guidanceNode.value === pending.nodeId);
  }
  private async sendControl(pending: PendingControl): Promise<void> {
    const key = this.controlKey(pending.kind, pending.graphId, pending.nodeId); if (!this.ready || this.controlBusy.has(key)) return;
    const wasUnresolved = pending.unresolved; pending.unresolved = true; this.controlBusy.add(key); this.persistControls(); this.renderControlPending(); this.updateGraphControls();
    try {
      const result = await api<GraphStopResult | GraphBudgetAmendmentResult | GraphGuidanceResult | GraphAdvanceResult>(pending.path, 'POST', pending.body), data = result.data;
      const uncertain = (): never => { throw new ApiError('invalid_response', 'The acknowledgement does not identify this exact captured graph control. Retry its retained request.', pending.body.requestId, undefined, true); };
      if (result.receipt?.operation !== pending.operation || result.requestId !== pending.body.requestId || data?.graph?.id !== pending.graphId || data.graph.rootTaskId !== pending.rootTaskId) uncertain();
      let message: string;
      if (pending.kind === 'grant') {
        const grant = data as GraphBudgetAmendmentResult, input = pending.body as GraphBudgetAmendmentInput;
        if (grant.budget?.graphId !== pending.graphId || grant.budget.budgetOwnerTaskId !== pending.rootTaskId || grant.budget.budget.taskId !== pending.rootTaskId || grant.budget.budgetAuthorityEpoch !== (BigInt(input.expectedBudgetAuthorityEpoch) + 1n).toString() || grant.graph.budgetAuthorityEpoch !== grant.budget.budgetAuthorityEpoch || grant.graph.controlRevision !== input.expectedGraphControlRevision
          || grant.amendment?.requestId !== input.requestId || grant.amendment.taskId !== pending.rootTaskId || grant.amendment.additionalAdmissions !== input.additionalAdmissions || (grant.amendment.newWindowSeconds ?? undefined) !== input.newWindowSeconds) uncertain();
        message = 'Shared allowance grant recorded. This acknowledgement starts no work; current window and execution eligibility remain separately observed.';
      } else if (pending.kind === 'guidance') {
        const guided = data as GraphGuidanceResult, input = pending.body as GraphGuidanceInput;
        const next = (BigInt(input.taskVersion) + 1n).toString();
        if (guided.node?.id !== pending.nodeId || guided.node.key !== pending.nodeKey || guided.node.taskId !== pending.taskId || guided.node.taskVersion !== next
          || guided.node.graphId !== pending.graphId || guided.node.controlRevision !== (BigInt(input.expectedNodeControlRevision) + 1n).toString()
          || guided.result?.task?.id !== pending.taskId || guided.result.task.currentVersion !== next || guided.result.task.revision !== guided.node.taskRevision
          || guided.result.version?.taskId !== pending.taskId || guided.result.version.version !== next || guided.result.version.guidanceText !== input.guidanceText) uncertain();
        const draft = this.guidanceDrafts.get(pending.graphId + ':' + pending.nodeId);
        if (draft?.edited && draft.text === input.guidanceText && draft.version === input.taskVersion) draft.edited = false;
        message = 'Guidance version ' + next + ' recorded for node ' + pending.nodeKey + '. Old results remain history. No worker, additional allowance or renewed window was created.';
      } else if (pending.kind === 'advance') {
        const advanced = data as GraphAdvanceResult, input = pending.body as GraphAdvanceInput, registration = advanced.dispatchAuthority;
        if (advanced.graph.graphVersion !== input.graphVersion || advanced.graph.controlRevision !== input.expectedGraphControlRevision
          || advanced.graph.budgetAuthorityEpoch !== input.expectedBudgetAuthorityEpoch || advanced.budget?.graphId !== pending.graphId
          || advanced.budget.budgetOwnerTaskId !== pending.rootTaskId || advanced.budget.budget.taskId !== pending.rootTaskId
          || advanced.budget.budgetAuthorityEpoch !== input.expectedBudgetAuthorityEpoch || advanced.budget.budget.currentWindow.number !== input.expectedWindowNumber
          || !Array.isArray(advanced.preparationAttemptIds) || advanced.preparationAttemptIds.length > 16 || !advanced.preparationAttemptIds.every(id)
          || new Set(advanced.preparationAttemptIds).size !== advanced.preparationAttemptIds.length || !Array.isArray(advanced.reboundNodes) || advanced.reboundNodes.length !== (input.rebindNodeIds?.length ?? 0)
          || !Array.isArray(advanced.dispatches) || advanced.dispatches.length > 16 || advanced.preparationAttemptIds.length < advanced.reboundNodes.length) uncertain();
        if (registration === null && (advanced.preparationAttemptIds.length > 0 || advanced.dispatches.length > 0 || advanced.reboundNodes.length > 0)) uncertain();
        const acknowledged = new Map(input.nodeAuthorities.map(node => [node.nodeId, { ...node }]));
        const reboundIds = new Set<string>();
        for (const rebound of advanced.reboundNodes) {
          const target = pending.rebindTargets?.find(target => target.nodeId === rebound?.nodeId), authority = acknowledged.get(rebound?.nodeId);
          if (!authority) return uncertain();
          if (!target || !authority || !input.rebindNodeIds?.includes(rebound.nodeId) || reboundIds.has(rebound.nodeId)
            || rebound.taskId !== target.taskId || rebound.fromTaskVersion !== target.taskVersion
            || rebound.toTaskVersion !== (BigInt(target.taskVersion) + 1n).toString()
            || rebound.provenance?.kind !== 'dependency_rebind' || rebound.provenance.requestId !== input.requestId
            || rebound.provenance.previousInputId !== target.inputId || !/^[a-f0-9]{64}$/.test(rebound.provenance.contributorVectorSha256)) uncertain();
          reboundIds.add(rebound.nodeId);
          authority.taskVersion = rebound.toTaskVersion; authority.nodeControlRevision = (BigInt(authority.nodeControlRevision) + 1n).toString();
        }
        const dispatchIds = new Set<string>(), attemptIds = new Set<string>();
        for (const view of advanced.dispatches) {
          const dispatch = view?.dispatch, node = dispatch ? acknowledged.get(dispatch.nodeId) : undefined;
          if (!dispatch || !id(dispatch.id) || dispatchIds.has(dispatch.id) || !id(dispatch.attemptId) || attemptIds.has(dispatch.attemptId)
            || !node || dispatch.graphId !== pending.graphId || dispatch.graphVersion !== input.graphVersion || dispatch.taskVersion !== node.taskVersion
            || dispatch.nodeControlRevision !== node.nodeControlRevision || dispatch.graphControlRevision !== input.expectedGraphControlRevision
            || dispatch.budgetOwnerTaskId !== pending.rootTaskId || dispatch.budgetAuthorityEpoch !== input.expectedBudgetAuthorityEpoch
            || dispatch.windowNumber !== input.expectedWindowNumber || dispatch.freshTriggerId !== input.requestId) uncertain();
          dispatchIds.add(dispatch.id); attemptIds.add(dispatch.attemptId);
        }
        if (registration !== null) {
          if (!registration || !id(registration.ownerInstanceId) || !revision(registration.generation) || !['active', 'capacity_wait', 'suspended'].includes(registration.state)
            || registration.graphControlRevision !== input.expectedGraphControlRevision || registration.budgetAuthorityEpoch !== input.expectedBudgetAuthorityEpoch
            || registration.windowNumber !== input.expectedWindowNumber || !Array.isArray(registration.nodeAuthorities) || registration.nodeAuthorities.length !== input.nodeAuthorities.length) uncertain();
          const seen = new Set<string>();
          for (const node of registration.nodeAuthorities) {
            const expected = acknowledged.get(node?.nodeId);
            if (!expected || seen.has(node.nodeId) || node.taskVersion !== expected.taskVersion || node.nodeControlRevision !== expected.nodeControlRevision) uncertain();
            seen.add(node.nodeId);
          }
        }
        if (advanced.reboundNodes.length && !this.rebindReceipts.has(input.requestId)) {
          this.rebindReceipts.set(input.requestId, { graphId: pending.graphId, requestId: input.requestId, targets: pending.rebindTargets!, nodes: advanced.reboundNodes });
          if (this.rebindReceipts.size > 32) { const first = this.rebindReceipts.keys().next().value; if (first) this.rebindReceipts.delete(first); }
          if (this.detail?.graph.id === pending.graphId) this.renderRebind();
        }
        message = 'Advance acknowledged: ' + advanced.preparationAttemptIds.length + ' local preparations and ' + advanced.dispatches.length
          + ' stages admitted. Acknowledgement alone does not prove launch, successful output or cleanup. No allowance or window was added.';
      } else {
        const stopped = data as GraphStopResult;
        if (typeof stopped.stopAccepted !== 'boolean' || !['accepted', 'already_stopped', 'already_settled'].includes(stopped.reason) || !Array.isArray(stopped.targetAttemptIds) || stopped.targetAttemptIds.length > 256 || !stopped.targetAttemptIds.every(id)
          || !Array.isArray(stopped.affectedNodeIds) || stopped.affectedNodeIds.length > 16 || !stopped.affectedNodeIds.every(id) || pending.kind === 'node-stop' && (stopped.node?.id !== pending.nodeId || stopped.node.key !== pending.nodeKey || stopped.node.taskVersion !== pending.taskVersion)) uncertain();
        const scope = pending.kind === 'node-stop' ? 'node ' + pending.nodeKey + ' in group ' + pending.graphId : 'group ' + pending.graphId;
        message = stopped.stopAccepted ? 'Stop accepted for ' + scope + '; cancellation requested. Actual output, effects and cleanup remain separately observed.' : 'Stop receipt recorded for ' + scope + ': ' + stopped.reason + '. Actual cleanup remains separately observed.';
      }
      if (this.controlPending.get(key) === pending) { this.controlPending.delete(key); this.persistControls(); }
      if (this.controlPaintCurrent(pending)) this.controlStatus.textContent = message;
      this.onTasksChanged(pending.projectId);
      if (this.projectId === pending.projectId) { void this.refresh(); if (this.selectedId === pending.graphId) void this.open(pending.graphId, false); }
    } catch (error) {
      const unresolved = wasUnresolved || error instanceof ApiError && error.uncertain;
      if (!unresolved && this.controlPending.get(key) === pending) { this.controlPending.delete(key); this.persistControls(); }
      if (this.controlPaintCurrent(pending)) this.controlStatus.textContent = errorMessage(error, pending.body.requestId) + (unresolved ? ' Retry the exact captured request; new field edits are separate.' : ' Current fields are retained.');
    } finally { this.controlBusy.delete(key); this.renderControlPending(); this.updateGraphControls(); }
  }
  private async loadLedger(more: boolean): Promise<void> {
    const detail = this.detail; if (!this.ready || !detail || this.ledgerBusy || more && !this.ledgerCursor) return;
    const graphId = detail.graph.id, rootTaskId = detail.graph.rootTaskId, projectGeneration = this.projectGeneration, selection = this.selectionGeneration, read = ++this.ledgerGeneration, cursor = more ? this.ledgerCursor : null;
    const current = (): boolean => this.ready && this.detail?.graph.id === graphId && projectGeneration === this.projectGeneration && selection === this.selectionGeneration && read === this.ledgerGeneration;
    this.ledgerBusy = true; this.updateGraphControls();
    try {
      const { data } = await api<Page<BudgetLedgerEntry>>('/graphs/' + graphId + '/budget-ledger?limit=30' + (cursor ? '&cursor=' + encodeURIComponent(cursor) : ''));
      if (!current()) return;
      if (!Array.isArray(data.items) || data.items.length > 100 || data.nextCursor !== null && (typeof data.nextCursor !== 'string' || !data.nextCursor || data.nextCursor === cursor || !data.items.length)) throw new ApiError('invalid_response', 'The shared ledger page or continuation is invalid.');
      const seen = new Set<string>();
      for (const row of data.items) if (!id(row.id) || row.taskId !== rootTaskId || !revision(row.sequence) || !revision(row.budgetRevision) || !revision(row.window?.number) || seen.has(row.id) || more && this.ledgerRows.has(row.id)) throw new ApiError('invalid_response', 'The ledger contains another owner or duplicated rows.'); else seen.add(row.id);
      if (!more) this.ledgerRows.clear(); for (const row of data.items) this.ledgerRows.set(row.id, row); this.ledgerCursor = data.nextCursor;
      this.ledgerList.replaceChildren(); for (const row of this.ledgerRows.values()) this.ledgerList.append(make('li', row.kind + ' · granted ' + row.grantedDelta + ' · consumed ' + row.consumedDelta + ' · window ' + row.window.number + (row.attemptId ? ' · actual attempt ' + row.attemptId : '') + ' · ledger ' + row.id));
      this.ledgerStatus.textContent = this.ledgerRows.size + ' complete shared ledger rows loaded. Node debits are included in this root ledger.' + (this.ledgerCursor ? ' More retained rows remain.' : ' End of this fixed ledger view; refresh reads newer entries.');
    } catch (error) { if (current()) this.ledgerStatus.textContent = errorMessage(error) + ' Loaded ledger rows and the exact failed cursor are retained.'; }
    finally { if (current()) { this.ledgerBusy = false; this.updateGraphControls(); } }
  }

  private controls(): void {
    this.inputs.setReady(this.ready); this.updateGraphControls();
    const available = this.ready && !!this.projectId;
    element<HTMLButtonElement>('open-graph-composer').disabled = !available; element<HTMLButtonElement>('open-graph-list').disabled = !available;
    element<HTMLButtonElement>('graph-refresh').disabled = !available; element<HTMLButtonElement>('graph-more').disabled = !available;
    element<HTMLButtonElement>('graph-refresh-defaults').disabled = !this.ready;
    element<HTMLButtonElement>('graph-start').disabled = !available || !graphStartRuntimeReady || !this.policy || !this.routingPolicy || !this.deliveryPreference
      || this.composer.issues().length > 0 || [...this.pending.values()].some(item => item.projectId === this.projectId && item.editorRevision === this.composer.revision);
    element('graph-runtime-note').textContent = graphStartRuntimeReady ? 'Start captures this complete declaration. Later edits form a separate draft; reconnecting never starts work.' : 'The task-group editor and reads are available. Starting a group is unavailable until graph execution is connected in this build.';
  }
}
