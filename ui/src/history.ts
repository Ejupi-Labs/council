import { addFact, api, ApiError, element, errorMessage, feedback, readSession, requestId, writeSession } from './api.js';
import type { Meta } from './types.js';

interface Omission { reason: string; count: string }
interface Summary { snapshotId: string; revision: string; projectId: string; profile: string; completeness: string; notice: string; capturedAt: string; taskCount: string; attemptCount: string; includedCategories: string[]; omittedCategories: string[]; additionalOmissions: Omission[] }
interface Row { ordinal: string; taskId: string; projectId?: string; attemptId?: string; revision: string; stateRevision?: string; currentVersion?: string; taskVersion?: string; phase?: string }
interface RecordPage { snapshotId: string; revision: string; completeness: string; notice: string; kind: 'tasks' | 'attempts'; items: Row[]; nextCursor: string | null }
interface Preview { previewId: string; revision: string; snapshotId: string; snapshotRevision: string; filename: string; destinationKind: string; destinationDirectory: string; destinationPath: string; completeness: string; includedCategories: string[]; omittedCategories: string[]; additionalOmissions: Omission[]; targetState: string; createdAt: string }
interface Operation { operationId: string; requestId: string; revision: string; snapshotId: string; previewId: string; state: string; completeness: string; outcome: string | null; durability: null | { fileFlush: string; directorySync: string; powerLossNamespaceGuarantee: boolean }; blockers: string[]; nextAction: string; filename: string; destinationDirectory: string; destinationPath: string }
type MutationKind = 'capture' | 'preview' | 'export' | 'reconcile';
interface Pending { kind: MutationKind; path: string; body: Record<string, string>; projectId: string | null; snapshotId: string | null; generation: number; unresolved: boolean; snapshotInput: string; operationInput: string; destinationPath?: string; filename?: string; expectedOperationId?: string; operationSnapshotId?: string; operationPreviewId?: string; operationRequestId?: string }

const id = (value: unknown): value is string => typeof value === 'string' && /^[a-f0-9]{32}$/.test(value);
const historyNotice = 'Incomplete observational history. Not a backup, evidence export, restore, or proof of acceptance.';
const decimal = (value: unknown, zero = false): value is string => typeof value === 'string' && (zero && value === '0' || /^[1-9][0-9]{0,18}$/.test(value) && BigInt(value) <= 9223372036854775807n);
const timestamp = (value: unknown): value is string => typeof value === 'string' && /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}Z$/.test(value) && !Number.isNaN(Date.parse(value)) && new Date(value).toISOString() === value;
const filename = (value: string): boolean => /^history-[A-Za-z0-9][A-Za-z0-9_-]{0,50}\.json$/.test(value);
const stringList = (value: unknown): value is string[] => Array.isArray(value) && value.every(x => typeof x === 'string');
const categories = (value: { includedCategories: string[]; omittedCategories: string[]; additionalOmissions: Omission[] }): boolean =>
  JSON.stringify(value.includedCategories) === JSON.stringify(['project_identity', 'task_metadata', 'attempt_metadata']) &&
  JSON.stringify(value.omittedCategories) === JSON.stringify(['task_text', 'project_paths', 'user_files', 'artifact_payloads', 'provider_output', 'vendor_authentication', 'owner_authority', 'legacy_state']) &&
  Array.isArray(value.additionalOmissions) && value.additionalOmissions.length <= 3 && value.additionalOmissions.every((o, i, list) => !!o && ['unknown_origin', 'unavailable', 'invalid_link'].includes(o.reason) && decimal(o.count, true) && (i === 0 || ['unknown_origin', 'unavailable', 'invalid_link'].indexOf(list[i - 1]!.reason) < ['unknown_origin', 'unavailable', 'invalid_link'].indexOf(o.reason)));
const validSummary = (s: Summary): boolean => !!s && id(s.snapshotId) && id(s.projectId) && s.revision === '1' && s.profile === 'metadata-history/1' && s.completeness === 'incomplete_history' && s.notice === historyNotice && decimal(s.taskCount, true) && BigInt(s.taskCount) <= 2048n && decimal(s.attemptCount, true) && BigInt(s.attemptCount) <= 8192n && timestamp(s.capturedAt) && categories(s);
const validDestination = (p: { filename: string; destinationDirectory: string; destinationPath: string }): boolean => typeof p.filename === 'string' && filename(p.filename) && typeof p.destinationDirectory === 'string' && typeof p.destinationPath === 'string' && (/^(?:[A-Za-z]:[\\/]|\/|\\\\)/.test(p.destinationDirectory)) && (p.destinationPath === p.destinationDirectory.replace(/[\\/]+$/, '') + '/' + p.filename || p.destinationPath === p.destinationDirectory.replace(/[\\/]+$/, '') + '\\' + p.filename || p.destinationDirectory === '/' && p.destinationPath === '/' + p.filename);
const validPreview = (p: Preview): boolean => !!p && id(p.previewId) && id(p.snapshotId) && p.revision === '1' && p.snapshotRevision === '1' && p.destinationKind === 'private_history_exports' && p.completeness === 'incomplete_history' && p.targetState === 'absent' && timestamp(p.createdAt) && validDestination(p) && categories(p);
const validOperation = (o: Operation): boolean => !!o && id(o.operationId) && id(o.requestId) && id(o.snapshotId) && id(o.previewId) && decimal(o.revision) && o.completeness === 'incomplete_history' && validDestination(o) && stringList(o.blockers) && new Set(o.blockers).size === o.blockers.length && o.blockers.every(b => ['owner_epoch_uncertain', 'journal_unproven', 'actor_not_drained', 'history_effect_uncertain', 'other_blocker', 'target_conflict', 'root_unavailable', 'request_interrupted'].includes(b)) && ['none', 'wait', 'inspect', 'reconcile', 'contact_support'].includes(o.nextAction) && (
  (['prepared', 'effect_started'].includes(o.state) && o.outcome === null && o.durability === null) ||
  (o.state === 'published' && o.outcome === 'published' && !!o.durability && o.durability.fileFlush === 'succeeded' && ['succeeded', 'unavailable'].includes(o.durability.directorySync) && o.durability.powerLossNamespaceGuarantee === false) ||
  (['uncertain', 'failed', 'no_publication'].includes(o.state) && o.outcome === o.state && o.durability === null));

const operationNames: Record<MutationKind, string> = { capture: 'history.snapshots.create', preview: 'history.exports.preview', export: 'history.exports.create', reconcile: 'history.operations.reconcile' };
const invalidAck = (): ApiError => new ApiError('invalid_response', 'The owner did not acknowledge the exact history request. Retry the same request.', '', undefined, true);
const pendingStorageKey = 'council.foundation.historyPending';
function validPending(value: unknown): value is Pending {
  if (!value || typeof value !== 'object') return false;
  const p = value as Pending;
  if (!p.body || typeof p.body !== 'object' || !id(p.body.requestId) || !(p.projectId === null || id(p.projectId)) || !(p.snapshotId === null || id(p.snapshotId)) || !Number.isSafeInteger(p.generation) || p.generation < 0 || typeof p.unresolved !== 'boolean' || typeof p.snapshotInput !== 'string' || typeof p.operationInput !== 'string') return false;
  const keys = Object.keys(p.body).sort().join(',');
  switch (p.kind) {
    case 'capture': return p.path === '/history/snapshots' && keys === 'projectId,requestId' && id(p.body.projectId);
    case 'preview': return p.path === '/history/exports/preview' && keys === 'filename,requestId,snapshotId,snapshotRevision' && id(p.body.snapshotId) && p.body.snapshotRevision === '1' && typeof p.body.filename === 'string' && filename(p.body.filename);
    case 'export': return p.path === '/history/exports' && keys === 'previewId,previewRevision,requestId' && id(p.body.previewId) && p.body.previewRevision === '1' && id(p.snapshotId) && typeof p.filename === 'string' && typeof p.destinationPath === 'string' && (p.expectedOperationId === undefined || id(p.expectedOperationId));
    case 'reconcile': return /^\/history\/operations\/[a-f0-9]{32}\/reconciliations$/.test(p.path) && keys === 'action,expectedRevision,requestId' && p.body.action === 'verify_no_publication' && decimal(p.body.expectedRevision) && id(p.operationSnapshotId) && id(p.operationPreviewId) && id(p.operationRequestId) && typeof p.filename === 'string' && typeof p.destinationPath === 'string';
    default: return false;
  }
}

function loadHistoryPending(): { pending: Pending | null; unavailable: boolean } {
  try {
    const raw = sessionStorage.getItem(pendingStorageKey);
    if (raw === null) return { pending: null, unavailable: false };
    const value: unknown = JSON.parse(raw);
    return validPending(value) ? { pending: value, unavailable: false } : { pending: null, unavailable: true };
  } catch { return { pending: null, unavailable: true }; }
}

// Never overwrite a different saved intent. A successful set without a matching
// readback is still uncertain, so no request may be sent in that case.
function saveHistoryPending(pending: Pending): boolean {
  try {
    const value = JSON.stringify(pending);
    const previous = sessionStorage.getItem(pendingStorageKey);
    if (previous !== null && previous !== value) return false;
    sessionStorage.setItem(pendingStorageKey, value);
    return sessionStorage.getItem(pendingStorageKey) === value;
  } catch { return false; }
}

function clearHistoryPending(pending: Pending): boolean {
  try {
    const previous = sessionStorage.getItem(pendingStorageKey);
    if (previous !== JSON.stringify(pending)) return false;
    sessionStorage.removeItem(pendingStorageKey);
    return sessionStorage.getItem(pendingStorageKey) === null;
  } catch { return false; }
}

export class HistoryView {
  private project: string | null = null;
  private generation = 0;
  private readGeneration = 0;
  private ready = false;
  private busy = false;
  private summary: Summary | null = null;
  private preview: Preview | null = null;
  private operation: Operation | null = null;
  private page: RecordPage | null = null;
  private readonly savedPending = loadHistoryPending();
  private pending: Pending | null = this.savedPending.pending;
  private readonly damagedPending = this.savedPending.unavailable;
  private readonly snapshotInput = element<HTMLInputElement>('history-snapshot-id');
  private readonly filenameInput = element<HTMLInputElement>('history-filename');
  private readonly operationInput = element<HTMLInputElement>('history-operation-id');

  constructor() {
    if (this.pending) this.pending.unresolved = true;
    this.snapshotInput.value = readSession<string>('historySnapshotInput') ?? '';
    this.filenameInput.value = readSession<string>('historyFilename') ?? 'history-project.json';
    this.operationInput.value = readSession<string>('historyOperationInput') ?? '';
    this.snapshotInput.addEventListener('input', () => writeSession('historySnapshotInput', this.snapshotInput.value));
    this.filenameInput.addEventListener('input', () => { writeSession('historyFilename', this.filenameInput.value); this.controls(); });
    this.operationInput.addEventListener('input', () => writeSession('historyOperationInput', this.operationInput.value));
    element('history-capture').addEventListener('click', () => { if (this.project) void this.mutate('capture', '/history/snapshots', { requestId: requestId(), projectId: this.project }); });
    element('history-retry').addEventListener('click', () => { if (this.pending) void this.send(this.pending); });
    element('history-open-form').addEventListener('submit', e => { e.preventDefault(); void this.openSnapshot(); });
    element('history-load-tasks').addEventListener('click', () => { void this.records('tasks'); });
    element('history-load-attempts').addEventListener('click', () => { void this.records('attempts'); });
    element('history-more').addEventListener('click', () => { if (this.page?.nextCursor) void this.records(this.page.kind, this.page.nextCursor); });
    element('history-preview-form').addEventListener('submit', e => { e.preventDefault(); void this.createPreview(); });
    element('history-export').addEventListener('click', () => { if (this.preview && this.filenameInput.value === this.preview.filename) void this.mutate('export', '/history/exports', { requestId: requestId(), previewId: this.preview.previewId, previewRevision: this.preview.revision }); });
    element('history-operation-form').addEventListener('submit', e => { e.preventDefault(); void this.openOperation(); });
    element('history-replay-export').addEventListener('click', () => {
      const operation = this.operation;
      if (operation) void this.mutate('export', '/history/exports', { requestId: operation.requestId, previewId: operation.previewId, previewRevision: '1' }, operation);
    });
    element('history-reconcile').addEventListener('click', () => { if (this.operation) void this.mutate('reconcile', '/history/operations/' + this.operation.operationId + '/reconciliations', { requestId: requestId(), expectedRevision: this.operation.revision, action: 'verify_no_publication' }); });
    this.controls();
  }

  setOwner(meta: Meta): void {
    this.ready = meta.historyContract === '007a-v1' && Array.isArray(meta.capabilities) && meta.capabilities.includes('metadata_history');
    feedback('history-status', this.damagedPending ? 'Browser session storage or the saved history request is unavailable. Restore session storage and reload; inspect any existing owner operation through the CLI. No new history mutation will be sent.' : this.ready ? 'Choose a project and capture or inspect incomplete history. No agent is started.' : 'This owner does not support the metadata history contract.', this.damagedPending ? 'error' : 'neutral');
    this.controls();
  }
  disconnect(): void { this.ready = false; this.generation++; this.readGeneration++; this.controls(); }
  setProject(project: string): void {
    if (project !== this.project) {
      this.summary = null; this.preview = null; this.page = null;
      element('history-snapshot').hidden = true; element('history-preview').hidden = true; element('history-more').hidden = true;
      element('history-facts').replaceChildren(); element('history-records').replaceChildren();
      element('history-omissions').textContent = ''; element('history-preview-facts').replaceChildren();
    }
    this.project = project; this.generation++; this.readGeneration++; this.controls();
  }

  private controls(): void {
    const blocked = !this.ready || this.busy || !!this.pending || this.damagedPending;
    element<HTMLButtonElement>('history-capture').disabled = blocked || !this.project;
    const retry = element<HTMLButtonElement>('history-retry'); retry.hidden = !this.pending; retry.disabled = !this.ready || this.busy;
    element('history-pending').textContent = this.pending ? 'Pending ' + this.pending.kind + ' request ' + this.pending.body.requestId + '. Retry sends its original captured input; later edits are retained.' : '';
    const exportButton = element<HTMLButtonElement>('history-export');
    exportButton.disabled = blocked || !this.preview || !this.summary || this.summary.projectId !== this.project || this.preview.snapshotId !== this.summary.snapshotId || this.filenameInput.value !== this.preview.filename;
    element<HTMLButtonElement>('history-reconcile').disabled = blocked || !this.operation || !['prepared', 'effect_started', 'uncertain'].includes(this.operation.state);
    element<HTMLButtonElement>('history-replay-export').disabled = blocked || !this.operation;
    for (const button of ['history-load-tasks', 'history-load-attempts', 'history-more']) element<HTMLButtonElement>(button).disabled = !this.ready || this.busy || !this.summary || this.summary.projectId !== this.project;
    for (const form of ['history-open-form', 'history-operation-form']) element<HTMLButtonElement>(form).querySelector<HTMLButtonElement>('button')!.disabled = !this.ready || this.busy;
    element<HTMLButtonElement>('history-preview-form').querySelector<HTMLButtonElement>('button')!.disabled = blocked || !this.summary || this.summary.projectId !== this.project;
  }

  private async mutate(kind: MutationKind, path: string, body: Record<string, string>, replayOperation?: Operation): Promise<void> {
    if (!this.ready || this.busy || this.pending || this.damagedPending) return;
    const selectedExport = replayOperation ?? this.preview;
    const pending: Pending = { kind, path, body, projectId: this.project, snapshotId: replayOperation?.snapshotId ?? this.summary?.snapshotId ?? null, generation: this.generation, unresolved: false, snapshotInput: this.snapshotInput.value, operationInput: this.operationInput.value, ...(kind === 'export' && selectedExport ? { destinationPath: selectedExport.destinationPath, filename: selectedExport.filename } : {}), ...(replayOperation ? { expectedOperationId: replayOperation.operationId } : {}), ...(kind === 'reconcile' && this.operation ? { operationSnapshotId: this.operation.snapshotId, operationPreviewId: this.operation.previewId, operationRequestId: this.operation.requestId, destinationPath: this.operation.destinationPath, filename: this.operation.filename } : {}) };
    this.pending = pending;
    await this.send(pending);
  }

  private async send(pending: Pending): Promise<void> {
    if (!this.ready || this.busy || this.pending !== pending) return;
    const wasUnresolved = pending.unresolved;
    let received = false;
    pending.unresolved = true; this.busy = true; this.readGeneration++;
    if (!saveHistoryPending(pending)) {
      this.busy = false; this.controls();
      feedback('history-status', 'Browser session storage could not safely retain the exact history request. No request was sent. Restore session storage, then retry this same request; any existing saved request is preserved.', 'error');
      return;
    }
    this.controls();
    feedback('history-status', 'Sending the captured history request ' + pending.body.requestId + '…');
    try {
      const result = await api<{ snapshot?: Summary; preview?: Preview; operation?: Operation; decision?: string; observationCodes?: string[] }>(pending.path, 'POST', pending.body);
      received = true;
      if (result.requestId !== pending.body.requestId || result.receipt?.operation !== operationNames[pending.kind] || result.receipt.requestId !== pending.body.requestId || !timestamp(result.receipt.committedAt) || !result.data || typeof result.data !== 'object') throw invalidAck();
      const data = result.data;
      if (pending.kind === 'capture') {
        if (!data.snapshot || !validSummary(data.snapshot) || data.snapshot.projectId !== pending.body.projectId) throw invalidAck();
      } else if (pending.kind === 'preview') {
        if (!data.preview || !validPreview(data.preview) || data.preview.snapshotId !== pending.body.snapshotId || data.preview.snapshotRevision !== pending.body.snapshotRevision || data.preview.filename !== pending.body.filename) throw invalidAck();
      } else {
        if (!data.operation || !validOperation(data.operation) || (pending.kind === 'export' && (data.operation.requestId !== pending.body.requestId || data.operation.previewId !== pending.body.previewId || data.operation.snapshotId !== pending.snapshotId || data.operation.filename !== pending.filename || data.operation.destinationPath !== pending.destinationPath || pending.expectedOperationId !== undefined && data.operation.operationId !== pending.expectedOperationId)) || (pending.kind === 'reconcile' && (pending.path !== '/history/operations/' + data.operation.operationId + '/reconciliations' || data.operation.snapshotId !== pending.operationSnapshotId || data.operation.previewId !== pending.operationPreviewId || data.operation.requestId !== pending.operationRequestId || data.operation.filename !== pending.filename || data.operation.destinationPath !== pending.destinationPath || BigInt(data.operation.revision) <= BigInt(pending.body.expectedRevision!) || !['no_publication', 'unresolved'].includes(data.decision ?? '') || !stringList(data.observationCodes) || new Set(data.observationCodes).size !== data.observationCodes.length || data.observationCodes.some(code => !['no_permit_proven', 'permit_present_or_uncertain', 'actor_not_drained', 'owner_history_unproven', 'journal_unproven', 'other_blocker'].includes(code)) || data.decision === 'no_publication' && (data.operation.state !== 'no_publication' || data.observationCodes.length !== 1 || data.observationCodes[0] !== 'no_permit_proven') || data.decision === 'unresolved' && (data.operation.state === 'no_publication' || data.observationCodes.length === 0)))) throw invalidAck();
      }
      if (!clearHistoryPending(pending)) throw new ApiError('history_cache_unavailable', 'The owner acknowledged this request, but its saved retry could not be cleared. Restore session storage and retry the same request.', pending.body.requestId, undefined, true);
      this.pending = null;
      this.readGeneration++;
      const current = pending.generation === this.generation && pending.projectId === this.project;
      if (pending.kind === 'capture' && data.snapshot) {
        const selectionCurrent = current && this.snapshotInput.value === pending.snapshotInput;
        if (selectionCurrent) { this.summary = data.snapshot; this.preview = null; this.page = null; this.snapshotInput.value = data.snapshot.snapshotId; writeSession('historySnapshotInput', this.snapshotInput.value); this.renderSnapshot(); }
        feedback('history-status', 'Captured incomplete snapshot ' + data.snapshot.snapshotId + (selectionCurrent ? '.' : '. Your newer selection is retained; inspect this identity to view it.'), 'success');
      } else if (pending.kind === 'preview' && data.preview) {
        if (current && this.summary?.snapshotId === data.preview.snapshotId && this.filenameInput.value === pending.body.filename) { this.preview = data.preview; this.renderPreview(); }
        feedback('history-status', 'Saved preview ' + data.preview.previewId + (current && this.filenameInput.value === pending.body.filename ? '. Review its destination before exporting.' : '. Your newer selection or filename is retained.'), 'success');
      } else if (data.operation) {
        const selectionCurrent = current && this.operationInput.value === pending.operationInput && (!this.operation || this.operation.operationId !== data.operation.operationId || BigInt(this.operation.revision) <= BigInt(data.operation.revision));
        if (selectionCurrent) { this.operation = data.operation; this.operationInput.value = data.operation.operationId; writeSession('historyOperationInput', this.operationInput.value); this.renderOperation(); }
        feedback('history-status', 'Operation ' + data.operation.operationId + ': ' + data.operation.state + (selectionCurrent ? '.' : '. Your newer selection is retained; inspect this identity to view it.'), data.operation.state === 'published' ? 'success' : 'neutral');
      }
    } catch (error) {
      const uncertain = received || wasUnresolved || error instanceof ApiError && error.uncertain;
      const cleared = !uncertain && clearHistoryPending(pending);
      if (cleared) this.pending = null;
      feedback('history-status', errorMessage(error, pending.body.requestId) + (uncertain ? ' Retry the exact saved request; no automatic replay occurs.' : ' Your selection and inputs remain available.'), 'error');
      if (!uncertain && !cleared) feedback('history-status', 'The request was rejected, but its saved intent could not be safely cleared. Restore session storage and retry this same request.', 'error');
    } finally { this.busy = false; this.controls(); }
  }

  private async openSnapshot(): Promise<void> {
    const selected = this.snapshotInput.value;
    if (!this.ready || !id(selected)) { feedback('history-status', 'Enter a valid snapshot identity.', 'error'); return; }
    const generation = ++this.readGeneration; const projectGeneration = this.generation;
    try {
      const { data } = await api<{ snapshot: Summary }>('/history/snapshots/' + selected);
      if (!validSummary(data.snapshot) || data.snapshot.snapshotId !== selected) throw new Error('The returned snapshot did not match the selection.');
      if (generation !== this.readGeneration || projectGeneration !== this.generation || this.snapshotInput.value !== selected) return;
      if (!this.project || data.snapshot.projectId !== this.project) throw new ApiError('history_scope_conflict', 'This snapshot belongs to a different project. Select its project before inspecting it.');
      this.summary = data.snapshot; this.preview = null; this.page = null; this.renderSnapshot();
      feedback('history-status', 'Inspecting incomplete snapshot ' + selected + '. No agent was started.');
    } catch (error) { if (generation === this.readGeneration) feedback('history-status', errorMessage(error), 'error'); }
    this.controls();
  }

  private renderSnapshot(): void {
    if (!this.summary) return;
    const s = this.summary; const facts = element('history-facts'); facts.replaceChildren();
    addFact(facts, 'Snapshot', s.snapshotId); addFact(facts, 'Project', s.projectId); addFact(facts, 'Captured', s.capturedAt); addFact(facts, 'Tasks', s.taskCount); addFact(facts, 'Attempts', s.attemptCount); addFact(facts, 'Scope', s.notice);
    element('history-omissions').textContent = 'Omitted: ' + s.omittedCategories.join(', ') + '. Additional omissions: ' + (s.additionalOmissions.map(o => o.reason + ' (' + o.count + ')').join(', ') || 'none reported') + '. Outcomes, dependency topology and current authority are unavailable.';
    element('history-records').replaceChildren(); element('history-more').hidden = true; element('history-preview').hidden = true; element('history-snapshot').hidden = false; this.controls();
  }

  private async records(kind: 'tasks' | 'attempts', cursor?: string): Promise<void> {
    const snapshot = this.summary; if (!this.ready || !snapshot || snapshot.projectId !== this.project) return;
    const generation = ++this.readGeneration; const projectGeneration = this.generation;
    try {
      const { data } = await api<RecordPage>('/history/snapshots/' + snapshot.snapshotId + '/records?kind=' + kind + (cursor ? '&cursor=' + encodeURIComponent(cursor) : ''));
      if (generation !== this.readGeneration || projectGeneration !== this.generation || this.summary?.snapshotId !== snapshot.snapshotId) return;
      if (data.snapshotId !== snapshot.snapshotId || data.revision !== snapshot.revision || data.completeness !== 'incomplete_history' || data.notice !== historyNotice || data.kind !== kind || !Array.isArray(data.items) || data.items.length > 100 || data.nextCursor !== null && (typeof data.nextCursor !== 'string' || data.nextCursor.length > 512)) throw new Error('The history page did not match this snapshot.');
      for (const row of data.items) if (!decimal(row.ordinal) || !id(row.taskId) || !decimal(row.revision) || (kind === 'tasks' ? row.projectId !== snapshot.projectId || !decimal(row.stateRevision) || !decimal(row.currentVersion) : !id(row.attemptId) || !decimal(row.taskVersion) || !['queued', 'preparing', 'running', 'cancelling', 'settled'].includes(row.phase ?? ''))) throw new Error('The history page contains invalid metadata.');
      const list = element('history-records'); if (!cursor) list.replaceChildren();
      for (const row of data.items) { const li = document.createElement('li'); li.textContent = kind === 'tasks' ? `Task ${row.ordinal}: ${row.taskId} · version ${row.currentVersion} · revision ${row.revision}` : `Attempt ${row.ordinal}: ${row.attemptId} · task ${row.taskId} · phase ${row.phase} · revision ${row.revision}`; list.append(li); }
      this.page = data; element('history-more').hidden = data.nextCursor === null; feedback('history-status', data.notice + ' Showing captured ' + kind + '. Recorded phase does not establish an outcome.');
    } catch (error) { if (generation === this.readGeneration) feedback('history-status', errorMessage(error), 'error'); }
  }

  private async createPreview(): Promise<void> {
    if (!this.summary || this.summary.projectId !== this.project || !filename(this.filenameInput.value)) { feedback('history-status', 'Choose the captured project and a valid history-…json filename.', 'error'); return; }
    await this.mutate('preview', '/history/exports/preview', { requestId: requestId(), snapshotId: this.summary.snapshotId, snapshotRevision: this.summary.revision, filename: this.filenameInput.value });
  }
  private renderPreview(): void {
    if (!this.preview) return;
    const p = this.preview; const facts = element('history-preview-facts'); facts.replaceChildren();
    addFact(facts, 'Snapshot', p.snapshotId); addFact(facts, 'Preview', p.previewId); addFact(facts, 'Full destination', p.destinationPath); addFact(facts, 'Private directory', p.destinationDirectory); addFact(facts, 'Included', p.includedCategories.join(', ')); addFact(facts, 'Omitted', p.omittedCategories.join(', '));
    element('history-preview').hidden = false; this.controls();
  }
  private async openOperation(): Promise<void> {
    const selected = this.operationInput.value; if (!this.ready || !id(selected)) { feedback('history-status', 'Enter a valid operation identity.', 'error'); return; }
    const generation = ++this.readGeneration; const projectGeneration = this.generation;
    try {
      const { data } = await api<{ operation: Operation }>('/history/operations/' + selected);
      if (!validOperation(data.operation) || data.operation.operationId !== selected) throw new Error('The returned operation did not match the selection.');
      if (generation !== this.readGeneration || projectGeneration !== this.generation || this.operationInput.value !== selected) return;
      this.operation = data.operation; this.renderOperation(); feedback('history-status', 'Operation ' + selected + ': ' + data.operation.state + '.');
    } catch (error) { if (generation === this.readGeneration) feedback('history-status', errorMessage(error), 'error'); }
    this.controls();
  }
  private renderOperation(): void {
    if (!this.operation) return;
    const o = this.operation; const facts = element('history-operation-facts'); facts.replaceChildren();
    addFact(facts, 'Operation', o.operationId); addFact(facts, 'Request', o.requestId); addFact(facts, 'Snapshot', o.snapshotId); addFact(facts, 'State', o.state); addFact(facts, 'Outcome', o.outcome ?? 'Not yet known'); addFact(facts, 'Full destination', o.destinationPath); addFact(facts, 'Blockers', o.blockers.join(', ') || 'None reported'); addFact(facts, 'Next action', o.nextAction);
    element('history-operation-note').textContent = o.durability ? 'File flush: ' + o.durability.fileFlush + '. Directory sync: ' + o.durability.directorySync + '. Namespace survival after power loss is not guaranteed.' : 'The owner has not reported a completed file flush. A partial file may remain; it is not automatically removed or retried.';
    element('history-operation').hidden = false; this.controls();
  }
}
