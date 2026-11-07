import { addFact, api, ApiError, errorMessage, timeLabel } from './api.js';
import type { Artifact, ArtifactChunk } from './execution-types.js';
import type { GraphArtifactRef, GraphDetail, GraphDispatchView, GraphInputDetail, GraphInputSummary, GraphMemberView, GraphNodeSummary, GraphSourceBasis } from './graph-types.js';
import type { Page } from './types.js';

const id = (v: unknown): v is string => typeof v === 'string' && /^[a-f0-9]{32}$/.test(v);
const count = (v: unknown): v is string => typeof v === 'string' && /^(0|[1-9][0-9]*)$/.test(v) && BigInt(v) <= 9223372036854775807n;
const sha = (v: unknown): v is string => typeof v === 'string' && /^[a-f0-9]{64}$/.test(v);
const ref = (v: GraphArtifactRef | null | undefined): v is GraphArtifactRef => !!v && id(v.artifactId) && id(v.producerAttemptId) && sha(v.sha256) && count(v.byteLength);
const timestamp = (value: unknown): value is string => {
  if (typeof value !== 'string') return false;
  const fields = /^([0-9]{4})-([0-9]{2})-([0-9]{2})T([0-9]{2}):([0-9]{2}):([0-9]{2})(?:[.]([0-9]{1,9}))?Z$/.exec(value);
  if (!fields) return false;
  const parsed = new Date(value);
  // Date.parse can normalize an impossible day into the following month.
  // Compare UTC calendar fields without rounding retained subsecond precision.
  return Number.isFinite(parsed.getTime()) && parsed.getUTCFullYear() === Number(fields[1]) && parsed.getUTCMonth() + 1 === Number(fields[2])
    && parsed.getUTCDate() === Number(fields[3]) && parsed.getUTCHours() === Number(fields[4]) && parsed.getUTCMinutes() === Number(fields[5]) && parsed.getUTCSeconds() === Number(fields[6]);
};
const sameRef = (a: GraphArtifactRef, b: GraphArtifactRef): boolean => a.artifactId === b.artifactId && a.producerAttemptId === b.producerAttemptId && a.sha256 === b.sha256 && a.byteLength === b.byteLength;
const sameBasis = (a: GraphSourceBasis, b: GraphSourceBasis): boolean => {
  const flat = (value: object | null): string => JSON.stringify(value === null ? null : Object.entries(value).sort(([a], [b]) => a.localeCompare(b)));
  return !!a && !!b && a.basisKind === b.basisKind && a.workflowId === b.workflowId && a.stepId === b.stepId && a.inputId === b.inputId && a.inputSha256 === b.inputSha256
    && ref(a.expectedManifest) && ref(b.expectedManifest) && sameRef(a.expectedManifest, b.expectedManifest) && ref(a.expectedIndex) && ref(b.expectedIndex) && sameRef(a.expectedIndex, b.expectedIndex)
    && a.expectedWorkingEntriesSha256 === b.expectedWorkingEntriesSha256 && a.expectedIndexEntriesSha256 === b.expectedIndexEntriesSha256
    && flat(a.candidateBinding) === flat(b.candidateBinding) && flat(a.responseBinding) === flat(b.responseBinding)
    && (a.finalVerificationRef === undefined && b.finalVerificationRef === undefined || ref(a.finalVerificationRef) && ref(b.finalVerificationRef) && sameRef(a.finalVerificationRef, b.finalVerificationRef));
};
const make = <K extends keyof HTMLElementTagNameMap>(tag: K, identity = '', text = ''): HTMLElementTagNameMap[K] => {
  const node = document.createElement(tag); if (identity) node.id = identity; if (text) node.textContent = text; return node;
};
const button = (identity: string, title: string, action: () => void): HTMLButtonElement => {
  const node = make('button', identity, title); node.type = 'button'; node.className = 'secondary'; node.addEventListener('click', action); return node;
};
const status = (identity: string): HTMLDivElement => { const node = make('div', identity); node.className = 'feedback'; node.setAttribute('role', 'status'); node.setAttribute('aria-live', 'polite'); return node; };
const pageCursor = <T>(page: Page<T>, cursor: string | null): void => {
  if (!page || !Array.isArray(page.items) || page.items.length > 100 || page.nextCursor !== null && (typeof page.nextCursor !== 'string' || !page.nextCursor || page.nextCursor.length > 4096 || page.nextCursor === cursor || !page.items.length))
    throw new ApiError('invalid_response', 'The complete retained page or its exact continuation is invalid.');
};

interface ByteRead { ref: GraphArtifactRef; offset: string; pieces: Uint8Array<ArrayBuffer>[]; text: string; decoder: TextDecoder; complete: boolean }

// This inspector owns only observation state. A selected immutable input never
// substitutes its latest execution, another receipt or a new execution intent.
export class GraphInputs {
  private graph: GraphDetail | null = null;
  private ready = false;
  private generation = 0;
  private inputListGeneration = 0;
  private inputGeneration = 0;
  private dispatchGeneration = 0;
  private dispatchListGeneration = 0;
  private memberGeneration = 0;
  private byteGeneration = 0;
  private nodeId: string | null = null;
  private inputId: string | null = null;
  private input: GraphInputDetail | null = null;
  private inputFeedback = '';
  private dispatchId: string | null = null;
  private inputCursor: string | null = null;
  private dispatchCursor: string | null = null;
  private memberCursor: string | null = null;
  private inputBusy = false;
  private dispatchBusy = false;
  private memberBusy = false;
  private byteBusy = false;
  private byteRead: ByteRead | null = null;
  private downloadURL: string | null = null;
  private readonly summaries = new Map<string, GraphInputSummary>();
  private readonly dispatches = new Map<string, GraphDispatchView>();
  private readonly members = new Map<string, GraphMemberView>();
  private readonly section = make('section', 'graph-input-inspector');
  private readonly heading = make('h4', 'graph-input-heading', 'Inputs and context');
  private readonly select = make('select', 'graph-input-selector');
  private readonly inputMore = button('graph-input-more', 'Load more retained inputs', () => { void this.loadInputs(true); });
  private readonly inputRefresh = button('graph-input-refresh', 'Refresh selected input', () => { if (this.inputId) void this.loadInput(this.inputId); });
  private readonly facts = make('dl', 'graph-input-facts');
  private readonly feedback = status('graph-input-status');
  private readonly sources = make('ul', 'graph-input-sources');
  private readonly access = make('ul', 'graph-context-access');
  private readonly memberList = make('ul', 'graph-members-list');
  private readonly memberStatus = status('graph-members-status');
  private readonly memberLoad = button('graph-members-load', 'Read retained members and provenance', () => { void this.loadMembers(false); });
  private readonly memberMore = button('graph-members-more', 'Load more retained members', () => { void this.loadMembers(true); });
  private readonly dispatchSelect = make('select', 'graph-dispatch-selector');
  private readonly dispatchMore = button('graph-dispatch-more', 'Load more receiving executions', () => { void this.loadDispatches(true); });
  private readonly dispatchRefresh = button('graph-dispatch-refresh', 'Refresh selected execution receipt', () => { if (this.dispatchId) void this.loadDispatch(this.dispatchId); });
  private readonly dispatchFacts = make('dl', 'graph-dispatch-facts');
  private readonly artifact = make('pre', 'graph-input-artifact');
  private readonly artifactStatus = status('graph-input-artifact-status');
  private readonly artifactRetry = button('graph-input-artifact-retry', 'Retry exact retained byte read', () => { void this.readBytes(); });
  private readonly download = make('a', 'graph-input-artifact-download', 'Download exact bytes');

  constructor(host: HTMLElement) {
    this.heading.tabIndex = -1; this.section.hidden = true; this.facts.className = this.dispatchFacts.className = 'facts';
    const inputLabel = make('label', '', 'Retained input (current or historical)'); inputLabel.htmlFor = this.select.id;
    const dispatchLabel = make('label', '', 'Exact receiving execution'); dispatchLabel.htmlFor = this.dispatchSelect.id;
    this.select.addEventListener('change', () => { if (id(this.select.value)) void this.loadInput(this.select.value); });
    this.dispatchSelect.addEventListener('change', () => { if (id(this.dispatchSelect.value)) void this.loadDispatch(this.dispatchSelect.value); });
    this.section.append(this.heading, inputLabel, this.select, this.inputMore, this.inputRefresh, this.facts, this.feedback,
      make('h5', '', 'Complete originals and retained evidence'), this.sources,
      make('h5', '', 'Captured context access plan'), make('p', '', 'These are the supported complete sources in the retained plan. A plan or receipt does not establish that a model inspected them.'), this.access,
      this.memberLoad, this.memberList, this.memberMore, this.memberStatus,
      dispatchLabel, this.dispatchSelect, this.dispatchMore, this.dispatchRefresh, this.dispatchFacts,
      make('h5', '', 'Selected retained artifact bytes'), this.artifactStatus, this.artifact, this.artifactRetry, this.download);
    host.append(this.section); this.controls();
  }

  setReady(ready: boolean): void { if (this.ready && !ready) { ++this.generation; this.inputBusy = this.dispatchBusy = this.memberBusy = this.byteBusy = false; } this.ready = ready; this.controls(); }
  setGraph(graph: GraphDetail | null): void {
    if (this.graph?.graph.id !== graph?.graph.id) {
      ++this.generation; this.nodeId = this.inputId = this.dispatchId = null; this.input = null;
      this.summaries.clear(); this.dispatches.clear(); this.members.clear(); this.inputCursor = this.memberCursor = this.dispatchCursor = null;
      this.section.hidden = true; this.clearBytes(); this.facts.replaceChildren(); this.sources.replaceChildren(); this.access.replaceChildren(); this.memberList.replaceChildren();
    }
    this.graph = graph; this.renderInputObservation(); this.controls();
  }
  async open(node: GraphNodeSummary): Promise<void> {
    if (!this.ready || !this.graph || !this.graph.nodes.some(item => item.id === node.id && item.key === node.key)) return;
    ++this.generation; this.nodeId = node.id; this.inputId = this.dispatchId = null; this.input = null;
    this.summaries.clear(); this.dispatches.clear(); this.members.clear(); this.inputCursor = this.dispatchCursor = this.memberCursor = null;
    this.inputBusy = this.dispatchBusy = this.memberBusy = false;
    this.section.hidden = false; this.heading.textContent = 'Inputs and context · ' + node.key; this.heading.focus();
    this.facts.replaceChildren(); this.sources.replaceChildren(); this.access.replaceChildren(); this.memberList.replaceChildren(); this.clearBytes();
    this.dispatchFacts.replaceChildren(); addFact(this.dispatchFacts, 'Materialization', 'Not observed; select an exact receiving execution.');
    this.setFeedback(node.inputId ? 'Reading this exact retained input…' : 'No composed input has been recorded for this node. The root basis is a separate retained resource.');
    this.renderInputs(); this.renderDispatches();
    const initial = node.inputId ? this.loadInput(node.inputId) : Promise.resolve();
    await Promise.all([this.loadInputs(false), initial]);
  }
  private current(generation: number, inputId?: string): boolean { return this.ready && generation === this.generation && !!this.graph && (!inputId || this.inputId === inputId); }
  private controls(): void {
    const ready = this.ready && !!this.graph && !this.section.hidden;
    this.select.disabled = !ready; this.inputMore.hidden = !this.inputCursor; this.inputMore.disabled = !ready || this.inputBusy;
    this.inputRefresh.disabled = !ready || !this.inputId;
    this.memberLoad.disabled = !ready || !this.input || this.input.summary.state !== 'ready' || this.memberBusy;
    this.memberMore.hidden = !this.memberCursor; this.memberMore.disabled = !ready || this.memberBusy;
    this.dispatchSelect.disabled = !ready || !this.inputId; this.dispatchMore.hidden = !this.dispatchCursor; this.dispatchMore.disabled = !ready || this.dispatchBusy;
    this.dispatchRefresh.disabled = !ready || !this.dispatchId;
    this.artifactRetry.hidden = !this.byteRead || this.byteRead.complete; this.artifactRetry.disabled = !ready || this.byteBusy;
    this.download.hidden = !this.downloadURL;
  }
  private validSummary(summary: GraphInputSummary): boolean {
    return !!this.graph && !!summary && id(summary.id) && summary.graphId === this.graph.graph.id && summary.rootTaskId === this.graph.graph.rootTaskId
      && count(summary.graphVersion) && summary.graphVersion !== '0' && id(summary.operationAttemptId) && typeof summary.current === 'boolean'
      && ['root_basis', 'node_input'].includes(summary.kind) && ['preparing', 'ready', 'blocked', 'conflicted', 'incomplete', 'uncertain', 'interrupted'].includes(summary.state)
      && (summary.kind === 'root_basis' ? summary.receivingNodeId === null && summary.receivingTaskId === null && summary.receivingTaskVersion === null
        : id(summary.receivingNodeId) && id(summary.receivingTaskId) && count(summary.receivingTaskVersion) && summary.receivingTaskVersion !== '0');
  }
  // Retained response flags describe their read snapshot. Current labels also
  // need the latest graph observation, including stale descendants whose own
  // input ID and task version have not changed. This grants no execution scope.
  private observedCurrent(summary: GraphInputSummary): boolean {
    const graph = this.graph;
    if (!graph || !summary.current || this.summaries.get(summary.id)?.current === false || summary.graphId !== graph.graph.id || summary.graphVersion !== graph.graph.graphVersion || summary.rootTaskId !== graph.graph.rootTaskId) return false;
    if (summary.kind === 'root_basis') return summary.id === graph.graph.rootInputId;
    const node = graph.nodes.find(item => item.id === summary.receivingNodeId);
    return !!node && node.taskId === summary.receivingTaskId && node.taskVersion === summary.receivingTaskVersion && node.inputId === summary.id
      && !graph.blockers.some(blocker => blocker.code === 'graph_input_stale' && blocker.nodeId === node.id && (blocker.inputId === null || blocker.inputId === undefined || blocker.inputId === summary.id));
  }
  private inputLabel(summary: GraphInputSummary): string {
    return summary.kind + ' · ' + summary.id + ' · ' + (this.observedCurrent(summary) ? 'current' : 'historical or stale; read only') + ' · ' + summary.state;
  }
  private setFeedback(message: string): void { this.inputFeedback = message; this.renderInputFeedback(); }
  private renderInputFeedback(): void {
    const selected = this.inputId ? this.input?.summary ?? this.summaries.get(this.inputId) : null;
    this.feedback.textContent = this.inputFeedback + (selected ? ' ' + (this.observedCurrent(selected) ? 'Current composition.' : 'Historical or stale input; read only.') : '');
  }
  private renderInputObservation(): void {
    // Preserve source/access buttons, focus, byte state and exact receipt DOM
    // when only the current graph observation changes.
    for (const option of this.select.options) {
      const summary = this.summaries.get(option.value);
      if (summary) option.textContent = this.inputLabel(summary);
    }
    if (this.input) {
      const scope = [...this.facts.querySelectorAll('dt')].find(term => term.textContent === 'Scope')?.nextElementSibling;
      if (scope) scope.textContent = this.observedCurrent(this.input.summary) ? 'Current retained input' : 'Historical or stale input; read only';
    }
    this.renderInputFeedback();
  }
  private renderInputs(): void {
    this.select.replaceChildren(new Option('Select an exact retained input', ''));
    for (const item of this.summaries.values()) if (item.kind === 'root_basis' || item.receivingNodeId === this.nodeId)
      this.select.add(new Option(this.inputLabel(item), item.id));
    if (this.inputId && ![...this.select.options].some(option => option.value === this.inputId)) this.select.add(new Option('Exact selected input · ' + this.inputId, this.inputId));
    this.select.value = this.inputId ?? ''; this.renderInputObservation(); this.controls();
  }
  private async loadInputs(more: boolean): Promise<void> {
    if (!this.ready || !this.graph || this.inputBusy || more && !this.inputCursor) return;
    const graphId = this.graph.graph.id, generation = this.generation, read = ++this.inputListGeneration, cursor = more ? this.inputCursor : null;
    this.inputBusy = true; this.controls();
    try {
      const { data } = await api<Page<GraphInputSummary>>('/graphs/' + graphId + '/inputs?limit=30' + (cursor ? '&cursor=' + encodeURIComponent(cursor) : ''));
      if (!this.current(generation) || read !== this.inputListGeneration) return;
      pageCursor(data, cursor);
      if (!data.items.every(item => this.validSummary(item))) throw new ApiError('invalid_response', 'The retained input page contains another graph or unsupported state.');
      if (!more) this.summaries.clear();
      for (const item of data.items) this.summaries.set(item.id, item);
      this.inputCursor = data.nextCursor; this.renderInputs();
    } catch (error) { if (this.current(generation) && read === this.inputListGeneration) this.setFeedback(errorMessage(error) + ' Loaded input history and the exact failed cursor are retained.'); }
    finally { if (this.current(generation) && read === this.inputListGeneration) { this.inputBusy = false; this.controls(); } }
  }
  private async loadInput(inputId: string): Promise<void> {
    if (!this.ready || !this.graph || !id(inputId)) return;
    const generation = this.generation, graphId = this.graph.graph.id, read = ++this.inputGeneration;
    const changed = this.inputId !== inputId, previous = changed ? null : this.input;
    this.inputId = inputId;
    if (changed) {
      this.input = null; this.dispatchId = null; ++this.dispatchGeneration; ++this.dispatchListGeneration; ++this.memberGeneration;
      this.memberBusy = this.dispatchBusy = false; this.memberCursor = this.dispatchCursor = null; this.members.clear(); this.dispatches.clear();
      this.clearBytes(); this.facts.replaceChildren(); this.sources.replaceChildren(); this.access.replaceChildren(); this.memberList.replaceChildren(); this.renderDispatches();
      this.dispatchFacts.replaceChildren(); addFact(this.dispatchFacts, 'Materialization', 'Not observed; select an exact receiving execution.');
    }
    this.renderInputs();
    this.setFeedback('Reading exact input ' + inputId + '…');
    try {
      const { data } = await api<GraphInputDetail>('/graphs/' + graphId + '/inputs/' + inputId);
      if (!this.current(generation, inputId) || read !== this.inputGeneration) return;
      if (!this.validSummary(data.summary) || data.summary.id !== inputId || data.summary.kind === 'node_input' && data.summary.receivingNodeId !== this.nodeId)
        throw new ApiError('invalid_response', 'The selected retained input does not match this graph/node.');
      const b = data.inputBinding, p = data.contextPacket;
      if (b && (!sha(b.inputSha256) || !sha(b.rootBasisSha256) || data.rootBinding !== null || b.id !== inputId || b.graphId !== graphId || b.receivingNodeId !== data.summary.receivingNodeId || b.receivingTaskId !== data.summary.receivingTaskId || b.receivingTaskVersion !== data.summary.receivingTaskVersion || b.inputSha256 !== data.summary.inputSha256)
        || p && (!b || p.graphId !== graphId || p.inputId !== inputId || p.inputSha256 !== b.inputSha256 || p.receivingTaskVersion !== b.receivingTaskVersion || !Array.isArray(p.providedAccess) || p.providedAccess.length > 64))
        throw new ApiError('invalid_response', 'The retained context or binding belongs to another input/version.');
      if (data.rootBinding && (data.rootBinding.graphId !== graphId || data.rootBinding.rootTaskId !== this.graph!.graph.rootTaskId)) throw new ApiError('invalid_response', 'The retained root basis does not match.');
      if (previous?.inputBinding && previous.inputBinding.inputSha256 !== b?.inputSha256 || previous?.rootBinding && previous.rootBinding.basisSha256 !== data.rootBinding?.basisSha256) throw new ApiError('invalid_response', 'An immutable retained input changed its binding.');
      const references = [data.conditionsRef, data.operationEffects, data.conflictArtifact, b?.manifest, b?.index, b?.stagedBlobs, b?.provenance, b?.contextPacket, data.rootBinding?.manifest, data.rootBinding?.index, data.rootBinding?.stagedBlobs];
      if (p) {
        if (!Array.isArray(p.constraintRefs) || p.constraintRefs.length > 64 || !Array.isArray(p.contributedEvidence) || p.contributedEvidence.length > 64) throw new ApiError('invalid_response', 'The complete context attribution is malformed.');
        references.push(p.graphInstructionsRef, p.nodeInstructionsRef, p.guidanceRef, p.acceptanceRef, p.derivedDigestRef, ...p.constraintRefs.map(item => item.artifactRef), ...p.contributedEvidence.map(item => item.artifactRef));
        if (!p.providedAccess.every(item => ref(item.artifactRef) && ['file', 'inline'].includes(item.mode) && typeof item.scope === 'string' && Array.isArray(item.paths) && item.paths.every(path => typeof path === 'string'))) throw new ApiError('invalid_response', 'The complete context access plan is unsupported or malformed.');
      }
      if (references.some(value => value !== undefined && value !== null && !ref(value))) throw new ApiError('invalid_response', 'A retained source has an invalid exact artifact reference.');
      this.input = data; this.summaries.set(inputId, data.summary); this.renderInputs(); this.renderInput();
      this.setFeedback('Exact retained input observed. Execution/materialization remains a separate selection.');
      if (changed || !this.dispatches.size) await this.loadDispatches(false);
      if (!this.current(generation, inputId) || read !== this.inputGeneration) return;
    } catch (error) { if (this.current(generation, inputId) && read === this.inputGeneration) this.setFeedback(errorMessage(error) + ' Existing history and evidence remain retained.'); }
    finally { if (this.current(generation, inputId) && read === this.inputGeneration) this.controls(); }
  }
  private source(title: string, artifact: GraphArtifactRef | null | undefined): void {
    if (!artifact) return;
    if (!ref(artifact)) throw new ApiError('invalid_response', 'A retained source has an invalid exact artifact reference.');
    const row = make('li'); row.append(make('p', '', title + ' · ' + artifact.artifactId + ' · ' + artifact.byteLength + ' bytes'), button('', title, () => { void this.openBytes(artifact); })); this.sources.append(row);
  }
  private renderInput(): void {
    const input = this.input!; const s = input.summary;
    this.facts.replaceChildren(); addFact(this.facts, 'Input', s.id); addFact(this.facts, 'Definition / version', s.graphVersion + ' · task version ' + (s.receivingTaskVersion ?? 'root'));
    addFact(this.facts, 'State', s.state === 'ready' ? s.kind === 'root_basis' ? 'Retained root basis ready' : 'Composed input ready' : s.state);
    addFact(this.facts, 'Scope', this.observedCurrent(s) ? 'Current retained input' : 'Historical or stale input; read only'); addFact(this.facts, 'Local operation', s.operationAttemptId);
    if (s.failure) addFact(this.facts, 'Retained blocker', s.failure.code + ' · ' + s.failure.message);
    this.sources.replaceChildren(); this.access.replaceChildren();
    const binding = input.inputBinding, root = input.rootBinding, packet = input.contextPacket;
    for (const [title, artifact] of [['Read retained manifest', binding?.manifest ?? root?.manifest], ['Read retained index', binding?.index ?? root?.index], ['Read retained staged blobs', binding?.stagedBlobs ?? root?.stagedBlobs], ['Read complete member provenance', binding?.provenance], ['Read retained operation effects', input.operationEffects], ['Read retained conflict', input.conflictArtifact], ['Read complete applicable conditions', input.conditionsRef]] as const) this.source(title, artifact);
    if (packet) {
      this.source('Read complete shared instructions', packet.graphInstructionsRef); this.source('Read complete receiver original', packet.nodeInstructionsRef);
      this.source('Read complete receiver guidance', packet.guidanceRef); this.source('Read complete receiver acceptance', packet.acceptanceRef);
      this.source('Read labelled derived context', packet.derivedDigestRef);
      for (const item of packet.constraintRefs) this.source('Read attributed constraint ' + item.ref + ' · ' + item.authorityScope, item.artifactRef);
      for (const item of packet.contributedEvidence) this.source('Read contributed evidence ' + item.ref + ' · ' + item.scope, item.artifactRef);
      for (const item of packet.providedAccess) {
        if (!ref(item.artifactRef) || !['file', 'inline'].includes(item.mode) || typeof item.scope !== 'string' || !Array.isArray(item.paths) || !item.paths.every(path => typeof path === 'string')) throw new ApiError('invalid_response', 'The complete context access plan is unsupported or malformed.');
        this.access.append(make('li', '', item.ref + ' · ' + item.scope + ' · ' + item.mode + ' · ' + item.artifactRef.artifactId + ' · ' + (item.paths.length ? 'paths: ' + item.paths.join(', ') : 'complete source')));
      }
      for (const contribution of binding?.contributions ?? []) this.access.append(make('li', '', 'Predecessor ' + contribution.fromNodeId + ' · ' + contribution.edgeKind + ' edge · version ' + contribution.taskVersion + ' · acceptance ' + contribution.acceptanceId + ' · producer ' + contribution.producerAttemptId));
    } else this.access.append(make('li', '', 'No complete receiving context packet is observed for this input.'));
  }
  private async loadMembers(more: boolean): Promise<void> {
    if (!this.ready || !this.graph || !this.input || this.memberBusy || more && !this.memberCursor) return;
    const graphId = this.graph.graph.id, inputId = this.input.summary.id, generation = this.generation, read = ++this.memberGeneration, cursor = more ? this.memberCursor : null;
    this.memberBusy = true; this.controls();
    try {
      const { data } = await api<Page<GraphMemberView>>('/graphs/' + graphId + '/inputs/' + inputId + '/members?limit=30' + (cursor ? '&cursor=' + encodeURIComponent(cursor) : ''));
      if (!this.current(generation, inputId) || read !== this.memberGeneration) return;
      pageCursor(data, cursor);
      const seen = new Set<string>();
      for (const row of data.items) {
        const key = row.dimension + ':' + row.path;
        if (!['working', 'index'].includes(row.dimension) || typeof row.path !== 'string' || !row.path || !['present', 'absent'].includes(row.state) || seen.has(key) || more && this.members.has(key)
          || !Array.isArray(row.origins) || !row.origins.length || row.origins.length > 17 || row.artifactRef !== null && !ref(row.artifactRef)) throw new ApiError('invalid_response', 'The complete member page has foreign, duplicated or unsupported rows.');
        if (row.byteLength !== null && !count(row.byteLength) || row.sha256 !== null && !sha(row.sha256) || row.target !== null && typeof row.target !== 'string' || row.artifactRef && (row.artifactRef.sha256 !== row.sha256 || row.artifactRef.byteLength !== row.byteLength)) throw new ApiError('invalid_response', 'The retained member bytes do not match their exact reference.');
        for (const origin of row.origins) {
          if (!['root', 'node'].includes(origin.kind) || !id(origin.taskId) || !id(origin.producerAttemptId) || !id(origin.inputId) || !sha(origin.stateSha256)) throw new ApiError('invalid_response', 'The member source attribution is incomplete.');
          if (origin.kind === 'root') {
            if (origin.nodeId !== null || origin.taskVersion !== null || origin.acceptanceId !== null || origin.acceptanceKind !== null || origin.taskId !== this.graph!.graph.rootTaskId || origin.inputId !== this.graph!.graph.rootInputId || origin.stateSha256 !== (this.input!.inputBinding?.rootBasisSha256 ?? this.input!.rootBinding?.basisSha256)) throw new ApiError('invalid_response', 'The root source attribution belongs to another retained basis.');
          } else {
            const known = this.input!.inputBinding?.contributorVector.find(item => item.nodeId === origin.nodeId);
            if (!known || known.taskId !== origin.taskId || known.taskVersion !== origin.taskVersion || known.acceptanceKind !== origin.acceptanceKind || known.acceptanceId !== origin.acceptanceId || known.producerAttemptId !== origin.producerAttemptId || known.inputId !== origin.inputId) throw new ApiError('invalid_response', 'The member source is not the exact captured historical contributor.');
          }
        }
        seen.add(key);
      }
      if (!more) this.members.clear();
      for (const row of data.items) this.members.set(row.dimension + ':' + row.path, row);
      this.memberCursor = data.nextCursor; this.memberList.replaceChildren();
      for (const row of this.members.values()) {
        const item = make('li'); item.append(make('p', '', row.dimension + ' · ' + row.path + ' · ' + row.state + (row.mode ? ' · mode ' + (row.dimension === 'working' && count(row.mode) ? BigInt(row.mode).toString(8) : row.mode) : '') + (row.sha256 ? ' · SHA256 ' + row.sha256 : '')));
        if (row.target !== null) item.append(make('p', '', 'Retained symlink target: ' + row.target));
        for (const origin of row.origins) item.append(make('p', '', origin.kind + ' source · task ' + origin.taskId + (origin.nodeId ? ' · node ' + origin.nodeId + ' · version ' + origin.taskVersion + ' · acceptance ' + origin.acceptanceId : '') + ' · producer ' + origin.producerAttemptId + ' · input ' + origin.inputId + ' · sealed head ' + origin.stateSha256));
        if (row.artifactRef) item.append(button('', 'Read exact ' + row.dimension + ' bytes for ' + row.path, () => { void this.openBytes(row.artifactRef!); }));
        this.memberList.append(item);
      }
      this.memberStatus.textContent = this.members.size + ' complete retained members loaded' + (this.memberCursor ? '; more remain.' : '; end of this exact input.');
    } catch (error) { if (this.current(generation, inputId) && read === this.memberGeneration) this.memberStatus.textContent = errorMessage(error) + ' Loaded rows and the exact failed cursor are retained.'; }
    finally { if (this.current(generation, inputId) && read === this.memberGeneration) { this.memberBusy = false; this.controls(); } }
  }
  private validateDispatch(view: GraphDispatchView): void {
    const d = view?.dispatch, input = this.input, graph = this.graph;
    if (!d || !input || !graph || !id(d.id) || d.graphId !== graph.graph.id || d.inputId !== input.summary.id || d.graphVersion !== input.summary.graphVersion || d.nodeId !== input.summary.receivingNodeId || d.taskId !== input.summary.receivingTaskId || d.taskVersion !== input.summary.receivingTaskVersion
      || !id(d.attemptId) || !id(d.nodeAdmissionId) || !id(d.workflowId) || !id(d.stepId) || !ref(d.contextPacket) || !input.inputBinding || d.inputSha256 !== input.inputBinding.inputSha256 || !sameRef(d.contextPacket, input.inputBinding.contextPacket)
      || !d.sourceBasis || d.sourceBasis.finalVerificationRef !== undefined && (!ref(d.sourceBasis.finalVerificationRef) || d.sourceBasis.basisKind !== 'node_subject') || !ref(d.sourceBasis.expectedManifest) || !ref(d.sourceBasis.expectedIndex) || !sha(d.sourceBasis.expectedWorkingEntriesSha256) || !sha(d.sourceBasis.expectedIndexEntriesSha256) || !['graph_input', 'node_subject'].includes(d.sourceBasis.basisKind) || d.sourceBasis.inputId !== d.inputId || d.sourceBasis.inputSha256 !== d.inputSha256 || d.sourceBasis.workflowId !== d.workflowId || d.sourceBasis.stepId !== d.stepId)
      throw new ApiError('invalid_response', 'The selected dispatch does not match the exact input, receiver or execution.');
    const m = view.materialization;
    if (m && (!id(m.id) || !timestamp(m.completedAt) || m.sourceSnapshotId !== null && !id(m.sourceSnapshotId) || m.graphId !== d.graphId || m.inputId !== d.inputId || m.inputSha256 !== d.inputSha256 || m.receivingAttemptId !== d.attemptId || m.operationAttemptId !== d.attemptId || !sameBasis(m.sourceBasis, d.sourceBasis)
      || !['complete', 'blocked', 'incomplete', 'uncertain'].includes(m.state) || !count(m.effectCount) || typeof m.effectsKnown !== 'boolean' || !['complete', 'incomplete', 'uncertain'].includes(m.evidenceStatus)
      || [m.observedManifest, m.observedIndex, m.operationEffects].some(value => value !== null && (!ref(value) || value.producerAttemptId !== d.attemptId))
      || [m.observedWorkingEntriesSha256, m.observedIndexEntriesSha256, m.stagedBlobsSha256].some(value => value !== null && !sha(value))
      || m.state === 'complete' && (m.failure !== null || !m.effectsKnown || m.evidenceStatus !== 'complete' || !sha(m.stagedBlobsSha256) || !id(m.sourceSnapshotId) || !ref(m.observedManifest) || !ref(m.observedIndex) || !ref(m.operationEffects) || m.observedWorkingEntriesSha256 !== d.sourceBasis.expectedWorkingEntriesSha256 || m.observedIndexEntriesSha256 !== d.sourceBasis.expectedIndexEntriesSha256)))
      throw new ApiError('invalid_response', 'The materialization receipt belongs to another execution or does not match its captured basis.');
    if (view.outputObservation && view.outputObservation.producerAttemptId !== d.attemptId) throw new ApiError('invalid_response', 'The output observation belongs to another producer.');
  }
  private renderDispatches(): void {
    this.dispatchSelect.replaceChildren(new Option('Select an exact receiving execution', ''));
    for (const value of this.dispatches.values()) this.dispatchSelect.add(new Option(value.dispatch.role + ' · ' + value.dispatch.attemptId + ' · ' + (value.materialization?.state ?? 'materialization not observed'), value.dispatch.id));
    this.dispatchSelect.value = this.dispatchId ?? ''; this.controls();
  }
  private async loadDispatches(more: boolean): Promise<void> {
    if (!this.ready || !this.graph || !this.input || this.dispatchBusy || more && !this.dispatchCursor) return;
    const graphId = this.graph.graph.id, inputId = this.input.summary.id, generation = this.generation, read = ++this.dispatchListGeneration, cursor = more ? this.dispatchCursor : null;
    this.dispatchBusy = true; this.controls();
    try {
      const { data } = await api<Page<GraphDispatchView>>('/graphs/' + graphId + '/dispatches?inputId=' + inputId + '&limit=30' + (cursor ? '&cursor=' + encodeURIComponent(cursor) : ''));
      if (!this.current(generation, inputId) || read !== this.dispatchListGeneration) return;
      pageCursor(data, cursor); for (const item of data.items) this.validateDispatch(item);
      if (!more) this.dispatches.clear();
      for (const item of data.items) this.dispatches.set(item.dispatch.id, item);
      this.dispatchCursor = data.nextCursor; this.renderDispatches();
    } catch (error) { if (this.current(generation, inputId) && read === this.dispatchListGeneration) this.setFeedback(errorMessage(error) + ' Loaded exact execution history and its cursor are retained.'); }
    finally { if (this.current(generation, inputId) && read === this.dispatchListGeneration) { this.dispatchBusy = false; this.controls(); } }
  }
  private async loadDispatch(dispatchId: string): Promise<void> {
    if (!this.ready || !this.graph || !this.input || !id(dispatchId)) return;
    const graphId = this.graph.graph.id, inputId = this.input.summary.id, generation = this.generation, read = ++this.dispatchGeneration;
    const changed = this.dispatchId !== dispatchId; this.dispatchId = dispatchId; this.renderDispatches();
    if (changed) { this.clearBytes(); this.dispatchFacts.replaceChildren(); addFact(this.dispatchFacts, 'Selected execution', 'Reading dispatch ' + dispatchId + '; another receipt does not qualify it.'); }
    try {
      const { data } = await api<GraphDispatchView>('/graphs/' + graphId + '/dispatches/' + dispatchId);
      if (!this.current(generation, inputId) || read !== this.dispatchGeneration || this.dispatchId !== dispatchId) return;
      this.validateDispatch(data); if (data.dispatch.id !== dispatchId) throw new ApiError('invalid_response', 'The returned dispatch does not match the selected execution.');
      this.dispatches.set(dispatchId, data); this.renderDispatches(); this.dispatchFacts.replaceChildren();
      const d = data.dispatch, m = data.materialization;
      addFact(this.dispatchFacts, 'Exact dispatch / receiving attempt', d.id + ' · ' + d.attemptId); addFact(this.dispatchFacts, 'Role / executor', d.role + ' · ' + d.executorKind); addFact(this.dispatchFacts, 'Receiving basis', d.sourceBasis.basisKind);
      addFact(this.dispatchFacts, 'Materialization', m ? m.state === 'complete' ? 'Files materialized for receiving attempt ' + m.receivingAttemptId : m.state + ' · effects known: ' + m.effectsKnown + ' · evidence ' + m.evidenceStatus : 'Not observed for this selected execution.');
      const artifactLink = (title: string, artifact: GraphArtifactRef | null | undefined): void => {
        if (!artifact) return;
        if (!ref(artifact)) throw new ApiError('invalid_response', 'The exact execution artifact reference is malformed.');
        const term = make('dt', '', title), value = make('dd'); value.append(button('', 'Read ' + title + ' · ' + artifact.artifactId, () => { void this.openBytes(artifact); })); this.dispatchFacts.append(term, value);
      };
      artifactLink('captured cumulative verification basis', d.sourceBasis.finalVerificationRef);
      if (m) { addFact(this.dispatchFacts, 'Receipt / settlement', m.id + ' · ' + timeLabel(m.completedAt)); if (m.failure) addFact(this.dispatchFacts, 'Materialization blocker', m.failure.code + ' · ' + m.failure.message);
        artifactLink('receiving manifest', m.observedManifest); artifactLink('receiving index', m.observedIndex); artifactLink('receiving operation effects', m.operationEffects);
      }
      const output = data.outputObservation;
      if (output?.candidate) addFact(this.dispatchFacts, 'Actual candidate history', output.candidate.candidateId + ' · ' + output.candidate.state + ' · acceptance remains separately recorded');
      if (output?.response) addFact(this.dispatchFacts, 'Actual response history', output.response.responseArtifactId + ' · ' + output.response.state + ' · selected deliverable remains separately recorded');
      artifactLink('candidate output closure', output?.candidate?.closureRef); artifactLink('response output closure', output?.response?.closureRef); artifactLink('whole-graph final verification', output?.candidate?.finalVerificationRef);
      this.setFeedback('Exact receiving execution observed. Its receipt is not reused for another execution.');
    } catch (error) { if (this.current(generation, inputId) && read === this.dispatchGeneration && this.dispatchId === dispatchId) this.setFeedback(errorMessage(error) + ' Previously loaded matching execution facts are retained.'); }
    finally { if (this.current(generation, inputId) && read === this.dispatchGeneration) this.controls(); }
  }
  private clearBytes(): void {
    ++this.byteGeneration; this.byteRead = null; this.byteBusy = false; this.artifact.textContent = ''; this.artifactStatus.textContent = '';
    if (this.downloadURL) URL.revokeObjectURL(this.downloadURL); this.downloadURL = null; this.download.removeAttribute('href'); this.controls();
  }
  private async openBytes(artifact: GraphArtifactRef): Promise<void> {
    if (!this.ready || !ref(artifact)) return;
    if (BigInt(artifact.byteLength) > 64n * 1024n * 1024n) { this.artifactStatus.textContent = 'This complete artifact exceeds the supported 64 MiB byte view. Its exact reference remains retained; nothing was clipped or reported as fully read.'; return; }
    this.clearBytes(); this.byteRead = { ref: { ...artifact }, offset: '0', pieces: [], text: '', decoder: new TextDecoder('utf-8', { ignoreBOM: true }), complete: false };
    await this.readBytes();
  }
  private async readBytes(): Promise<void> {
    const state = this.byteRead; if (!this.ready || !state || state.complete || this.byteBusy) return;
    const generation = this.generation, read = this.byteGeneration; this.byteBusy = true; this.controls();
    this.artifactStatus.textContent = 'Reading exact artifact ' + state.ref.artifactId + ' · retained offset ' + state.offset;
    try {
      const { data: metadata } = await api<Artifact>('/artifacts/' + state.ref.artifactId);
      if (!this.current(generation) || read !== this.byteGeneration || this.byteRead !== state) return;
      if (metadata.id !== state.ref.artifactId || metadata.attemptId !== state.ref.producerAttemptId || metadata.sha256 !== state.ref.sha256 || metadata.byteLength !== state.ref.byteLength || !metadata.closed || metadata.completeness !== 'complete') throw new ApiError('invalid_response', 'The retained artifact no longer matches its exact closed reference.');
      while (true) {
        const { data } = await api<ArtifactChunk>('/artifacts/' + state.ref.artifactId + '/bytes?offset=' + state.offset + '&through=' + state.ref.byteLength);
        if (!this.current(generation) || read !== this.byteGeneration || this.byteRead !== state) return;
        if (data.artifactId !== state.ref.artifactId || data.offset !== state.offset || data.through !== state.ref.byteLength || !count(data.nextOffset) || !data.streamClosed || data.completeness !== 'complete' || data.sha256 !== state.ref.sha256) throw new ApiError('invalid_response', 'The exact retained byte snapshot changed.');
        const bytes = Uint8Array.from(atob(data.bytesBase64), character => character.charCodeAt(0));
        if (BigInt(data.nextOffset) !== BigInt(state.offset) + BigInt(bytes.length) || BigInt(data.nextOffset) > BigInt(state.ref.byteLength) || data.eof !== (data.nextOffset === state.ref.byteLength) || !data.eof && data.nextOffset === state.offset) throw new ApiError('invalid_response', 'The exact retained byte cursor did not advance consistently.');
        state.pieces.push(bytes); state.text += state.decoder.decode(bytes, { stream: !data.eof }); state.offset = data.nextOffset; this.artifact.textContent = state.text;
        if (!data.eof) continue;
        const blob = new Blob(state.pieces, { type: 'application/octet-stream' }), body = await blob.arrayBuffer();
        const digest = [...new Uint8Array(await crypto.subtle.digest('SHA-256', body))].map(value => value.toString(16).padStart(2, '0')).join('');
        if (!this.current(generation) || read !== this.byteGeneration || this.byteRead !== state) return;
        if (digest !== state.ref.sha256) throw new ApiError('invalid_response', 'The complete retained artifact failed its expected SHA256.');
        state.complete = true; this.downloadURL = URL.createObjectURL(blob); this.download.href = this.downloadURL; this.download.download = state.ref.artifactId + '.bin';
        this.artifactStatus.textContent = 'Complete exact artifact · ' + state.offset + ' bytes · SHA256 ' + digest + '. Text is decoded for display; download preserves exact bytes.'; break;
      }
    } catch (error) { if (this.current(generation) && read === this.byteGeneration && this.byteRead === state) this.artifactStatus.textContent = errorMessage(error) + ' Loaded bytes and the exact retained byte cursor are preserved for retry.'; }
    finally { if (this.current(generation) && read === this.byteGeneration) { this.byteBusy = false; this.controls(); } }
  }
}
