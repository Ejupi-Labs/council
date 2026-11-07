import { api, ApiError, errorMessage, readSession, requestId, writeSession } from './api.js';
import type { TaskDetail } from './execution-types.js';
import type { GraphDetail } from './graph-types.js';

type Scope = { kind: string; taskId?: string; taskVersion?: string; expectedTaskRevision?: string; graphId?: string; graphVersion?: string; expectedGraphControlRevision?: string; expectedBudgetAuthorityEpoch?: string; expectedWindowNumber?: string; nodeId?: string; nodeAuthorities?: unknown[] };
export interface RecoveryCheckpoint {
  id: string; scope: Scope; revision: string; state: string; blockers: string[]; nextAction: string;
  admittedAttemptIds: string[]; completedAttemptIds: string[]; unfinishedTaskIds: string[];
}
interface IncidentView { incident: { id: string; code: string; message: string }; revision: string; state: string; blockers: string[]; nextAction: string }
interface Pending { requestId: string; op: string; path: string; body: Record<string, unknown>; target: string; checkpointId?: string; unresolved: boolean }
const validId = (x: unknown): x is string => typeof x === 'string' && /^[a-f0-9]{32}$/.test(x);
const revision = (x: unknown): x is string => typeof x === 'string' && /^[1-9][0-9]{0,18}$/.test(x) && BigInt(x) <= 9223372036854775807n;
const el = <K extends keyof HTMLElementTagNameMap>(tag: K, text = ''): HTMLElementTagNameMap[K] => { const x = document.createElement(tag); x.textContent = text; return x; };
const key = 'recoveryPendingControls';
function sameNodeAuthorities(actual: unknown, expected: unknown): boolean {
  if (!Array.isArray(actual) || !Array.isArray(expected) || actual.length < 1 || actual.length > 16 || actual.length !== expected.length) return false;
  const fields = ['nodeId', 'taskVersion', 'nodeControlRevision'];
  return actual.every((value: unknown, index: number) => {
    const captured = expected[index] as unknown;
    if (!value || typeof value !== 'object' || Array.isArray(value) || !captured || typeof captured !== 'object' || Array.isArray(captured)) return false;
    const item = value as Record<string, unknown>, original = captured as Record<string, unknown>;
    return Object.keys(item).length === fields.length && fields.every(field => Object.hasOwn(item, field) && item[field] === original[field])
      && validId(item['nodeId']) && revision(item['taskVersion']) && revision(item['nodeControlRevision']);
  });
}
function pendingList(): Pending[] {
  const v = readSession<Pending[]>(key);
  return Array.isArray(v) ? v.filter(p => {
    if (!p || !validId(p.requestId) || !validId(p.target) || !p.body || p.body.requestId !== p.requestId) return false;
    return p.op === 'tasks.pause' && p.path === '/tasks/' + p.target + '/pause'
      || p.op === 'graphs.pause' && p.path === '/graphs/' + p.target + '/pause'
      || p.op === 'checkpoints.resume' && p.checkpointId === p.target && p.path === '/checkpoints/' + p.target + '/resume'
      || p.op === 'incidents.reconcile' && p.path === '/incidents/' + p.target + '/reconciliations';
  }) : [];
}
function scopeForGraph(g: GraphDetail): Record<string, unknown> {
  if (!validId(g.graph.id) || !revision(g.graph.graphVersion) || !revision(g.graph.controlRevision) || !revision(g.graph.budgetAuthorityEpoch) || !revision(g.budget.budget.currentWindow.number) || !Array.isArray(g.nodes) || g.nodes.length < 1 || g.nodes.length > 16) throw new ApiError('invalid_response', 'The current group authority is incomplete. Refresh before resuming.');
  const nodes = g.nodes.map(n => {
    if (!validId(n.id) || !revision(n.taskVersion) || !revision(n.controlRevision)) throw new ApiError('invalid_response', 'A group member authority is incomplete.');
    return { nodeId: n.id, taskVersion: n.taskVersion, nodeControlRevision: n.controlRevision };
  });
  if (new Set(nodes.map(n => n.nodeId)).size !== nodes.length) throw new ApiError('invalid_response', 'The group member authority is ambiguous.');
  return { kind: 'graph', graphId: g.graph.id, graphVersion: g.graph.graphVersion, expectedGraphControlRevision: g.graph.controlRevision, expectedBudgetAuthorityEpoch: g.graph.budgetAuthorityEpoch, expectedWindowNumber: g.budget.budget.currentWindow.number, nodeAuthorities: nodes };
}

// One panel per existing task/group view. Pending bodies are retained separately
// from current selection; a retry never rebuilds a captured authority tuple.
export class RecoveryView {
  private readonly section = el('section');
  private readonly content = el('div');
  private readonly pending = el('ul');
  private readonly status = el('p');
  private task: TaskDetail | null = null;
  private graph: GraphDetail | null = null;
  private generation = 0;
  private available = false;
  private busy = new Set<string>();
  private incident: IncidentView | null = null;
  private selectedIncident: string | null = null;
  private globalIncident: string | null = null;
  private rendered = '';

  constructor(host: HTMLElement, private readonly label: 'task' | 'group' | 'global', private readonly refresh: () => void) {
    this.section.className = 'panel'; this.section.id = label + '-recovery';
    this.section.append(el('h4', 'Pause and recovery'), el('p', 'Pause stops new admissions while admitted work settles. Resume is explicit; restart does not repeat work.'));
    this.status.setAttribute('role', 'status'); this.status.setAttribute('aria-live', 'polite'); this.status.className = 'feedback';
    this.pending.setAttribute('aria-label', 'Pending recovery requests');
    this.section.append(this.content, this.pending, this.status); host.append(this.section); this.render();
  }

  setTask(task: TaskDetail | null, available: boolean): void {
    if (this.task?.task.id !== task?.task.id) { this.generation++; this.incident = null; this.selectedIncident = null; this.status.textContent = ''; }
    this.task = task; this.graph = null; this.available = available; this.render();
  }
  setGraph(graph: GraphDetail | null, available: boolean): void {
    if (this.graph?.graph.id !== graph?.graph.id) { this.generation++; this.incident = null; this.status.textContent = ''; }
    this.graph = graph; this.task = null; this.available = available; this.render();
  }
  setIncident(id: string | null): void {
    const next = validId(id) ? id : null;
    if (next !== this.selectedIncident) { this.generation++; this.selectedIncident = next; this.incident = null; this.render(); }
  }
  setGlobalIncident(id: string | null, available: boolean): void {
    const next = validId(id) ? id : null;
    if (next !== this.globalIncident) { this.generation++; this.globalIncident = next; this.incident = null; this.status.textContent = ''; }
    this.available = available; this.render();
  }
  private button(text: string, action: () => void, disabled = false): HTMLButtonElement {
    const b = el('button', text); b.type = 'button'; b.className = 'secondary'; b.disabled = disabled; b.dataset.recoveryFocus = text; b.addEventListener('click', action); return b;
  }
  private currentTarget(): string | null { return this.task?.task.id ?? this.graph?.graph.id ?? this.globalIncident; }
  private render(): void {
    const retained = pendingList(), task = this.task?.task, graph = this.graph?.graph;
    // Long-poll updates often change unrelated graph state. Keep the actual
    // focused controls mounted when their recovery authority has not changed.
    const fingerprint = JSON.stringify({
      available: this.available, globalIncident: this.globalIncident, selectedIncident: this.selectedIncident,
      task: task ? [task.id, task.currentVersion, task.revision, task.graphRef?.kind, this.task?.latestAttempt?.incident?.id, this.task?.latestAttempt?.cleanup.incidentId] : null,
      graph: graph && this.graph ? [graph.id, graph.graphVersion, graph.controlRevision, graph.budgetAuthorityEpoch, this.graph.budget.budget.currentWindow.number,
        this.graph.nodes.map(n => [n.id, n.key, n.taskId, n.taskVersion, n.taskRevision, n.controlRevision])] : null,
      checkpoints: this.task?.checkpoints ?? this.graph?.checkpoints ?? [], incident: this.incident,
      pending: retained.map(p => [p.requestId, p.op, p.target, p.unresolved]), busy: [...this.busy].sort(),
    });
    if (fingerprint === this.rendered) return;
    this.rendered = fingerprint;
    const active = document.activeElement instanceof HTMLElement && this.section.contains(document.activeElement) ? document.activeElement.dataset.recoveryFocus : undefined;
    this.content.replaceChildren(); this.pending.replaceChildren();
    const target = this.currentTarget();
    this.section.hidden = !target && (this.label !== 'global' || retained.length === 0);
    if (!target && this.label !== 'global') return;
    if (!this.available) this.content.append(el('p', 'Pause and reconciliation are unavailable on this owner. Reconnect to a compatible recovery-enabled owner.'));
    if (this.label === 'global' && this.globalIncident) this.content.append(this.button('Inspect global recovery incident', () => { if (this.globalIncident) void this.inspectIncident(this.globalIncident); }, !this.available));
    if (this.task) {
      const task = this.task.task;
      const alreadyPaused = (this.task.checkpoints ?? []).some(cp => cp?.scope?.taskId === task.id && ['pausing', 'paused'].includes(cp.state));
      if (task.graphRef?.kind !== 'root') this.content.append(this.button(task.graphRef ? 'Pause this group member' : 'Pause task', () => this.pauseTask(task.id, task.currentVersion, task.revision), !this.available || alreadyPaused));
      const incidentId = this.selectedIncident ?? this.task.latestAttempt?.incident?.id ?? this.task.latestAttempt?.cleanup.incidentId;
      if (validId(incidentId)) this.content.append(this.button('Inspect recovery incident', () => { void this.inspectIncident(incidentId); }, !this.available));
    }
    if (this.graph) {
      const g = this.graph;
      const active = g.checkpoints ?? [];
      this.content.append(this.button('Pause group', () => this.pauseGraph(g), !this.available || active.some(cp => cp?.scope?.kind === 'graph' && ['pausing', 'paused'].includes(cp.state))));
      for (const n of g.nodes) this.content.append(this.button('Pause member ' + n.key, () => this.pauseTask(n.taskId, n.taskVersion, n.taskRevision), !this.available || active.some(cp => cp?.scope?.taskId === n.taskId && ['pausing', 'paused'].includes(cp.state))));
    }
    for (const cp of (this.task?.checkpoints ?? this.graph?.checkpoints ?? [])) {
      if (!cp || !validId(cp.id) || !revision(cp.revision) || !cp.scope) continue;
      const item = el('div'); item.append(el('p', 'Checkpoint ' + cp.id + ' · ' + cp.scope.kind + ' · ' + cp.state + '. ' + cp.nextAction));
      if (Array.isArray(cp.blockers) && cp.blockers.length) item.append(el('p', 'Blockers: ' + cp.blockers.join(', ')));
      const inspect = this.button('Inspect checkpoint ' + cp.id.slice(0, 8), () => { void this.inspectCheckpoint(cp.id); }, !this.available);
      const resume = this.button('Resume checkpoint ' + cp.id.slice(0, 8), () => { void this.prepareResume(cp.id); }, !this.available || cp.state !== 'paused' || cp.scope.kind === 'graph' && !!this.task);
      if (cp.scope.kind === 'graph' && this.task) item.append(el('p', 'Open the current task group to resume this group checkpoint.'));
      resume.dataset.recoveryFocus = 'resume-' + cp.id; item.append(inspect, resume); this.content.append(item);
    }
    if (this.incident) {
      const i = this.incident; this.content.append(el('p', 'Incident ' + i.incident.id + ' · ' + i.state + ' · ' + i.incident.code + ': ' + i.incident.message), el('p', i.nextAction));
      if (i.blockers.length) this.content.append(el('p', 'Blockers: ' + i.blockers.join(', ')));
      const label = i.state === 'reconciled' ? 'Recheck remaining fence' : 'Reconcile incident ' + i.incident.id.slice(0, 8);
      this.content.append(this.button(label, () => this.sendNew('incidents.reconcile', '/incidents/' + i.incident.id + '/reconciliations', { requestId: requestId(), expectedIncidentRevision: i.revision, action: 'verify_and_reconcile' }, i.incident.id), !this.available || i.state === 'reconciled' && i.blockers.length === 0));
    }
    for (const p of retained) {
      const row = el('li', 'Pending ' + p.op + ' · ' + p.target + ' · request ' + p.requestId + '. ');
      row.dataset.recoveryRequestId = p.requestId;
      const retry = this.button('Retry exact ' + p.op + ' request', () => { void this.send(p); }, !this.available || this.busy.has(p.requestId));
      retry.dataset.recoveryFocus = 'retry-' + p.requestId; row.append(retry); this.pending.append(row);
    }
    if (active) this.section.querySelector<HTMLElement>('[data-recovery-focus="' + CSS.escape(active) + '"]')?.focus({ preventScroll: true });
  }
  private pauseTask(taskId: string, taskVersion: string, taskRevision: string): void {
    if (!validId(taskId) || !revision(taskVersion) || !revision(taskRevision)) { this.status.textContent = 'Task authority is incomplete. Refresh before pausing.'; return; }
    this.sendNew('tasks.pause', '/tasks/' + taskId + '/pause', { requestId: requestId(), taskVersion, expectedTaskRevision: taskRevision }, taskId);
  }
  private pauseGraph(g: GraphDetail): void {
    try {
      const authority = scopeForGraph(g); delete authority['kind']; delete authority['graphId'];
      this.sendNew('graphs.pause', '/graphs/' + g.graph.id + '/pause', { requestId: requestId(), ...authority }, g.graph.id);
    } catch (e) { this.status.textContent = errorMessage(e); }
  }
  private async inspectCheckpoint(id: string): Promise<RecoveryCheckpoint | null> {
    const generation = this.generation;
    try {
      const { data } = await api<RecoveryCheckpoint>('/checkpoints/' + id);
      if (!data || data.id !== id || !revision(data.revision) || !data.scope || !['task', 'member', 'graph'].includes(data.scope.kind)) throw new ApiError('invalid_response', 'Checkpoint identity or authority is incomplete.');
      if (generation === this.generation) this.status.textContent = 'Checkpoint ' + id + ' · ' + data.state + '. ' + data.nextAction + (data.blockers?.length ? ' Blockers: ' + data.blockers.join(', ') : '');
      return data;
    } catch (e) { if (generation === this.generation) this.status.textContent = errorMessage(e); return null; }
  }
  private async inspectIncident(id: string): Promise<void> {
    const generation = this.generation;
    try {
      const { data } = await api<IncidentView>('/incidents/' + id);
      if (!data || data.incident?.id !== id || !revision(data.revision) || !Array.isArray(data.blockers) || typeof data.nextAction !== 'string') throw new ApiError('invalid_response', 'Incident revision and evidence are unavailable. No reconciliation was requested.');
      if (generation === this.generation) { this.incident = data; this.render(); }
    } catch (e) { if (generation === this.generation) this.status.textContent = errorMessage(e); }
  }
  private async prepareResume(id: string): Promise<void> {
    const generation = this.generation, target = this.currentTarget();
    const cp = await this.inspectCheckpoint(id);
    if (!cp || generation !== this.generation || cp.state !== 'paused') return;
    try {
      let scope: Record<string, unknown>;
      if (cp.scope.kind === 'task') {
        if (!validId(cp.scope.taskId) || cp.scope.taskId !== target) throw new ApiError('invalid_response', 'The checkpoint does not belong to this selected task.');
        const { data } = await api<TaskDetail>('/tasks/' + cp.scope.taskId);
        if (data.task.id !== cp.scope.taskId || !revision(data.task.currentVersion) || !revision(data.task.revision)) throw new ApiError('invalid_response', 'Current task authority is unavailable.');
        scope = { kind: 'task', taskId: data.task.id, taskVersion: data.task.currentVersion, expectedTaskRevision: data.task.revision };
      } else {
        if (!validId(cp.scope.graphId)) throw new ApiError('invalid_response', 'The checkpoint group identity is unavailable.');
        const { data: graph } = await api<GraphDetail>('/graphs/' + cp.scope.graphId);
        if (graph.graph.id !== cp.scope.graphId) throw new ApiError('invalid_response', 'The current group does not match this checkpoint.');
        const whole = scopeForGraph(graph);
        if (cp.scope.kind === 'graph') {
          if (target !== cp.scope.graphId) throw new ApiError('invalid_response', 'Select the checkpoint group before resuming.');
          scope = whole;
        } else {
          const node = graph.nodes.find(n => n.id === cp.scope.nodeId && n.taskId === cp.scope.taskId);
          if (!node || !validId(node.taskId) || !revision(node.taskRevision) || !revision(node.taskVersion) || !revision(node.controlRevision) || target !== node.taskId && target !== graph.graph.id) throw new ApiError('invalid_response', 'The checkpoint member authority no longer matches this selection.');
          const { nodeAuthorities: _unused, kind: _kind, ...parent } = whole;
          scope = { ...parent, kind: 'member', taskId: node.taskId, taskVersion: node.taskVersion, expectedTaskRevision: node.taskRevision, nodeId: node.id, expectedNodeControlRevision: node.controlRevision };
        }
      }
      if (generation !== this.generation) return;
      this.sendNew('checkpoints.resume', '/checkpoints/' + id + '/resume', { requestId: requestId(), expectedCheckpointRevision: cp.revision, scope }, id, id);
    } catch (e) { if (generation === this.generation) this.status.textContent = errorMessage(e); }
  }
  private sendNew(op: string, path: string, body: Record<string, unknown>, target: string, checkpointId?: string): void {
    if (!this.available || !validId(body['requestId']) || !validId(target)) return;
    const pending = pendingList();
    if (pending.some(p => p.op === op && p.target === target)) { this.status.textContent = 'Retry the existing exact request before creating another.'; return; }
    const p: Pending = { requestId: body['requestId'], op, path, body, target, checkpointId, unresolved: false };
    writeSession(key, [...pending, p]);
    if (!pendingList().some(saved => JSON.stringify(saved) === JSON.stringify(p))) { this.status.textContent = 'Cannot retain this exact request in browser session storage. Reconnect with session storage available before proceeding.'; return; }
    this.render(); void this.send(p);
  }
  private async send(p: Pending): Promise<void> {
    if (!this.available || this.busy.has(p.requestId)) return;
    const generation = this.generation, selection = this.currentTarget(), wasUnresolved = p.unresolved;
    p.unresolved = true; writeSession(key, pendingList().map(item => item.requestId === p.requestId ? p : item));
    if (!pendingList().some(saved => JSON.stringify(saved) === JSON.stringify(p))) { this.status.textContent = 'Cannot retain this exact request before sending it. Reconnect with session storage available.'; return; }
    this.busy.add(p.requestId); this.render();
    try {
      const result = await api<{ checkpoint?: RecoveryCheckpoint; incident?: IncidentView; decision?: { incidentId: string; requestId: string } }>(p.path, 'POST', p.body);
      if (result.requestId !== p.requestId || result.receipt?.requestId !== p.requestId || result.receipt.operation !== p.op) throw new ApiError('invalid_response', 'The acknowledgement does not match the exact retained request.', p.requestId, undefined, true);
      const cp = result.data?.checkpoint;
      if (p.op === 'tasks.pause' && (!cp?.scope || !validId(cp.id) || !revision(cp.revision) || !['task', 'member'].includes(cp.scope.kind) || cp.scope.taskId !== p.target || cp.scope.taskVersion !== p.body['taskVersion'] || cp.scope.expectedTaskRevision !== p.body['expectedTaskRevision'])
        || p.op === 'graphs.pause' && (!cp?.scope || !validId(cp.id) || !revision(cp.revision) || cp.scope.kind !== 'graph' || cp.scope.graphId !== p.target || cp.scope.graphVersion !== p.body['graphVersion'] || cp.scope.expectedGraphControlRevision !== p.body['expectedGraphControlRevision'] || cp.scope.expectedBudgetAuthorityEpoch !== p.body['expectedBudgetAuthorityEpoch'] || cp.scope.expectedWindowNumber !== p.body['expectedWindowNumber'] || !sameNodeAuthorities(cp.scope.nodeAuthorities, p.body['nodeAuthorities']))
        || p.op === 'checkpoints.resume' && (!cp || cp.id !== p.checkpointId || cp.state !== 'resumed' || !revision(cp.revision))
        || p.op === 'incidents.reconcile' && (result.data?.incident?.incident?.id !== p.target || !revision(result.data?.incident?.revision) || !Array.isArray(result.data?.incident?.blockers) || typeof result.data?.incident?.nextAction !== 'string' || result.data?.decision?.incidentId !== p.target || result.data?.decision?.requestId !== p.requestId)) throw new ApiError('invalid_response', 'The acknowledgement target does not match the retained request.', p.requestId, undefined, true);
      writeSession(key, pendingList().filter(item => item.requestId !== p.requestId));
      if (generation === this.generation && selection === this.currentTarget()) {
        if (p.op === 'incidents.reconcile' && result.data.incident && revision(result.data.incident.revision) && Array.isArray(result.data.incident.blockers)) this.incident = result.data.incident;
        this.status.textContent = 'The exact ' + p.op + ' request was acknowledged. Refreshing the retained state.'; this.refresh();
      }
    } catch (e) {
      const retained = wasUnresolved || e instanceof ApiError && e.uncertain;
      if (!retained) writeSession(key, pendingList().filter(item => item.requestId !== p.requestId));
      if (generation === this.generation && selection === this.currentTarget()) this.status.textContent = errorMessage(e, p.requestId) + (retained ? ' Retry the exact retained request to reconcile its outcome.' : ' Refresh before choosing a new action.');
    } finally { this.busy.delete(p.requestId); this.render(); }
  }
}
