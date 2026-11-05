import { addFact, api, ApiError, element, errorMessage, feedback, policyName, readSession, requestId, timeLabel, writeSession } from './api.js';
import type { Draft, DraftSummary, Page, Policy } from './types.js';

interface PendingSave {
  path: string;
  method: 'POST' | 'PUT';
  body: { requestId: string; originalText: string; projectId?: string; expectedRevision?: string; useCurrentPolicy?: boolean };
  editGeneration: number;
  unresolved?: boolean;
}
interface Editor {
  projectId: string;
  draftId: string | null;
  revision: string | null;
  text: string;
  savedText: string | null;
  policy: Policy | null;
  editGeneration: number;
  useCurrentPolicy: boolean;
  historical: boolean;
  pending: PendingSave | null;
  conflict: boolean;
  recoveryText: string | null;
  status: string;
  tone: 'neutral' | 'success' | 'error';
}

function emptyEditor(projectId: string): Editor {
  return { projectId, draftId: null, revision: null, text: '', savedText: null, policy: null,
    editGeneration: 0, useCurrentPolicy: false, historical: false, pending: null, conflict: false,
    recoveryText: null, status: 'Describe the task, then save a draft. Nothing will run.', tone: 'neutral' };
}

function fromDraft(draft: Draft, historical = false): Editor {
  return { ...emptyEditor(draft.projectId), draftId: draft.id, revision: draft.revision,
    text: draft.originalText, savedText: draft.originalText, policy: draft.requestedPolicy,
    historical, status: historical ? 'Inspecting immutable saved revision ' + draft.revision + '. Return to the current draft to edit.'
      : 'Opened saved revision ' + draft.revision + '. Draft / Not run.' };
}

function dirty(editor: Editor): boolean {
  return editor.savedText === null ? editor.text.length > 0 : editor.text !== editor.savedText;
}

export class DraftsView {
  private projectId: string | null = null;
  private current: Editor | null = null;
  private readonly editors = new Map<string, Editor>();
  private readonly active = new Map<string, Editor>();
  private summaries: DraftSummary[] = [];
  private cursor: string | null = null;
  private readGeneration = 0;
  private listGeneration = 0;
  private readonly saving = new Set<Editor>();
  private readonly text = element<HTMLTextAreaElement>('task-text');
  private readonly save = element<HTMLButtonElement>('save-draft');
  private readonly usePolicy = element<HTMLInputElement>('use-current-policy');

  constructor() {
    const cache = readSession<Editor[]>('draftEditors');
    if (Array.isArray(cache)) {
      for (const item of cache) {
        if (item && /^[a-f0-9]{32}$/.test(item.projectId) && typeof item.text === 'string'
          && typeof item.editGeneration === 'number' && (item.draftId === null || /^[a-f0-9]{32}$/.test(item.draftId))) {
          if (item.pending) item.pending.unresolved = true;
          this.editors.set(this.key(item), item);
          this.active.set(item.projectId, item);
        }
      }
    }
    this.text.addEventListener('input', () => {
      if (!this.current) return;
      this.current.text = this.text.value;
      this.current.editGeneration++;
      this.current.status = this.current.pending
        ? 'Your newer text is retained. Retry the pending save first to reconcile its outcome.'
        : 'Local edits are not saved yet. Draft / Not run.';
      this.current.tone = 'neutral';
      this.persist();
      this.render();
    });
    this.usePolicy.addEventListener('change', () => {
      if (!this.current) return;
      this.current.useCurrentPolicy = this.usePolicy.checked;
      this.current.editGeneration++;
      this.persist();
    });
    element('draft-form').addEventListener('submit', event => { event.preventDefault(); void this.saveDraft(); });
    element('new-draft').addEventListener('click', () => this.newDraft());
    element('more-drafts').addEventListener('click', () => { void this.refreshList(true); });
    element('draft-history-form').addEventListener('submit', event => {
      event.preventDefault();
      const revision = element<HTMLInputElement>('draft-history-revision').value;
      if (!/^[1-9][0-9]*$/.test(revision) || revision.length > 19 || BigInt(revision) > 9223372036854775807n) {
        feedback('draft-status', 'Enter a saved revision from 1 through 9223372036854775807.', 'error'); return;
      }
      if (this.current?.draftId) void this.loadDraft(this.current.draftId, revision);
    });
    element('return-current-draft').addEventListener('click', () => {
      if (this.current?.draftId) void this.loadDraft(this.current.draftId);
    });
    element('apply-latest-revision').addEventListener('click', () => { void this.reconcile(true); });
    element('reload-saved-draft').addEventListener('click', () => { void this.reconcile(false); });
    element('restore-draft-text').addEventListener('click', () => {
      const editor = this.current;
      if (!editor || editor.recoveryText === null) return;
      editor.text = editor.recoveryText; editor.recoveryText = null; editor.editGeneration++;
      editor.status = 'Retained local text restored. Save a new revision when ready.'; editor.tone = 'neutral';
      this.persist(); this.render(); this.text.focus();
    });
    this.render();
  }

  setProject(projectId: string): void {
    if (projectId === this.projectId) return;
    this.projectId = projectId;
    this.readGeneration++;
    this.current = this.active.get(projectId) ?? emptyEditor(projectId);
    this.remember(this.current);
    this.summaries = []; this.cursor = null; this.render(); this.renderList();
    void this.refreshList();
  }

  private key(editor: Editor): string {
    return editor.projectId + ':' + (editor.draftId ?? 'new') + (editor.historical ? '@' + editor.revision : '');
  }

  private remember(editor: Editor): void {
    this.editors.set(this.key(editor), editor);
    this.active.set(editor.projectId, editor);
    this.persist();
  }

  private persist(): void {
    // This is tab-scoped recovery, never an acknowledgement of a durable save.
    const activeValues = [...this.active.values()];
    const values = [...this.editors.values()].filter(editor => !activeValues.includes(editor));
    values.push(...activeValues);
    writeSession('draftEditors', values);
  }

  private newDraft(): void {
    if (!this.projectId || this.current?.pending) return;
    this.readGeneration++;
    const existing = this.editors.get(this.projectId + ':new');
    this.current = existing ?? emptyEditor(this.projectId);
    this.remember(this.current); this.render(); this.renderList(); this.text.focus();
  }

  private render(): void {
    const editor = this.current;
    this.text.disabled = !editor;
    this.text.readOnly = editor?.historical ?? false;
    const displayText = editor?.text ?? '';
    // Do not assign value during typing; preserve the caret and original stored text.
    if (this.text.value !== displayText.replace(/\r\n?/g, '\n')) this.text.value = displayText;
    this.save.disabled = !editor || editor.historical || this.saving.has(editor)
      || (!editor.pending && !/\S/u.test(editor.text));
    this.save.textContent = editor && this.saving.has(editor) ? 'Saving…' : editor?.pending ? 'Retry pending save' : 'Save draft';
    element<HTMLButtonElement>('new-draft').disabled = !editor || !!editor.pending;
    this.usePolicy.disabled = !editor || !editor.draftId || editor.historical;
    this.usePolicy.checked = editor?.useCurrentPolicy ?? false;
    element('draft-history-form').hidden = !editor?.draftId;
    element('draft-conflict').hidden = !editor?.conflict;
    element('restore-draft-text').hidden = editor?.recoveryText === null || editor?.recoveryText === undefined;
    feedback('draft-status', editor?.status ?? 'Open a project to prepare a task.', editor?.tone ?? 'neutral');
    const facts = element('draft-meta'); facts.replaceChildren();
    if (editor?.draftId) {
      addFact(facts, 'Draft identity', editor.draftId);
      addFact(facts, editor.historical ? 'Historical revision' : 'Saved revision', editor.revision ?? 'Unknown');
      addFact(facts, 'Saved requested preference', editor.policy ? policyName(editor.policy.mode) : 'Not established');
      addFact(facts, 'Selection and execution', 'Auto · Draft / Not run');
      if (editor.policy) addFact(facts, 'Preference snapshot', 'Revision ' + editor.policy.revision + ' · ' + editor.policy.selectionSource);
    }
    if (editor?.pending) addFact(facts, 'Pending save request', editor.pending.body.requestId);
  }

  async refreshList(append = false): Promise<void> {
    const projectId = this.projectId;
    if (!projectId || (append && !this.cursor)) return;
    const generation = ++this.listGeneration;
    const cursor = append ? this.cursor : null;
    element('draft-list').setAttribute('aria-busy', 'true');
    feedback('draft-list-status', 'Loading saved drafts…');
    try {
      const { data } = await api<Page<DraftSummary>>('/drafts?projectId=' + encodeURIComponent(projectId)
        + (cursor ? '&cursor=' + encodeURIComponent(cursor) : ''));
      if (generation !== this.listGeneration || projectId !== this.projectId) return;
      this.summaries = append ? [...this.summaries, ...data.items] : data.items;
      this.cursor = data.nextCursor;
      this.renderList();
      feedback('draft-list-status', this.summaries.length ? 'Select a draft to reopen its full original text.' : 'No saved drafts in this project yet.');
    } catch (error) {
      if (generation === this.listGeneration && projectId === this.projectId) feedback('draft-list-status', errorMessage(error) + ' Existing local text is retained.', 'error');
    } finally {
      if (generation === this.listGeneration) element('draft-list').setAttribute('aria-busy', 'false');
    }
  }

  private renderList(): void {
    const list = element('draft-list'); list.replaceChildren();
    for (const draft of this.summaries) {
      const row = document.createElement('li');
      const button = document.createElement('button'); button.type = 'button'; button.dataset['draftId'] = draft.id;
      button.setAttribute('aria-current', String(draft.id === this.current?.draftId));
      const title = document.createElement('span'); title.className = 'draft-id'; title.textContent = 'Draft ' + draft.id.slice(0, 8);
      const revision = document.createElement('span'); revision.textContent = 'Revision ' + draft.revision + ' · Draft / Not run';
      const date = document.createElement('span'); date.textContent = timeLabel(draft.updatedAt);
      button.append(title, revision, date);
      button.addEventListener('click', () => { void this.loadDraft(draft.id); });
      row.append(button); list.append(row);
    }
    element('more-drafts').hidden = this.cursor === null;
  }

  private async loadDraft(id: string, revision?: string): Promise<void> {
    if (this.current?.pending) {
      feedback('draft-status', 'Retry the pending save before opening another draft. Your text is retained.', 'error'); return;
    }
    const projectId = this.projectId;
    const previous = this.current;
    const editGeneration = previous?.editGeneration;
    const generation = ++this.readGeneration;
    feedback('draft-status', 'Loading saved ' + (revision ? 'revision ' + revision : 'draft') + '…');
    try {
      const { data } = await api<Draft>('/drafts/' + id + (revision ? '?revision=' + encodeURIComponent(revision) : ''));
      if (generation !== this.readGeneration || projectId !== this.projectId || previous !== this.current) return;
      if (previous?.editGeneration !== editGeneration) {
        previous!.status = 'Your newer local edits were retained. Select the saved draft again when ready.';
        this.render(); return;
      }
      if (data.projectId !== projectId) throw new ApiError('invalid_response', 'The saved draft belongs to another project. Choose its project first.');
      const cached = this.editors.get(projectId + ':' + id + (revision ? '@' + revision : ''));
      this.current = cached && dirty(cached) ? cached : fromDraft(data, revision !== undefined);
      if (cached && dirty(cached)) this.current.status = 'Your unsaved local text is retained. Saved revisions remain available below.';
      this.remember(this.current); this.render(); this.renderList();
      element<HTMLInputElement>('draft-history-revision').value = revision ?? data.revision;
      this.text.focus();
    } catch (error) {
      if (generation === this.readGeneration && projectId === this.projectId && previous === this.current) {
        if (this.current) { this.current.status = errorMessage(error) + ' Your local text is retained.'; this.current.tone = 'error'; }
        this.render();
      }
    }
  }

  private async saveDraft(): Promise<void> {
    const editor = this.current;
    if (!editor || editor.historical || this.saving.has(editor)) return;
    if (!editor.pending) {
      if (!/\S/u.test(editor.text)) { feedback('draft-status', 'Enter task text before saving a draft.', 'error'); return; }
      const id = requestId();
      editor.pending = editor.draftId ? {
        path: '/drafts/' + editor.draftId, method: 'PUT', editGeneration: editor.editGeneration,
        body: { requestId: id, expectedRevision: editor.revision!, originalText: editor.text, useCurrentPolicy: editor.useCurrentPolicy },
      } : {
        path: '/drafts', method: 'POST', editGeneration: editor.editGeneration,
        body: { requestId: id, projectId: editor.projectId, originalText: editor.text },
      };
    }
    const pending = editor.pending;
    const wasUnresolved = pending.unresolved === true;
    // Persist uncertainty before dispatch so a reload cannot forget a possible commit.
    pending.unresolved = true;
    this.readGeneration++;
    this.saving.add(editor); editor.status = 'Saving draft. No agent task will run.'; editor.tone = 'neutral';
    this.persist(); this.render();
    try {
      const { data } = await api<Draft>(pending.path, pending.method, pending.body);
      const previousKey = this.key(editor);
      editor.draftId = data.id; editor.revision = data.revision; editor.savedText = data.originalText;
      editor.policy = data.requestedPolicy; editor.pending = null; editor.conflict = false;
      if (editor.editGeneration === pending.editGeneration) { editor.text = data.originalText; editor.useCurrentPolicy = false; }
      editor.status = 'Saved revision ' + data.revision + '. Draft / Not run.'
        + (dirty(editor) ? ' Newer local edits are retained and are not saved yet.' : '');
      editor.tone = 'success';
      if (this.editors.get(previousKey) === editor) this.editors.delete(previousKey);
      this.remember(editor);
      if (this.projectId === editor.projectId) void this.refreshList();
    } catch (error) {
      const uncertain = wasUnresolved || (error instanceof ApiError && error.uncertain);
      editor.status = errorMessage(error, pending.body.requestId)
        + (uncertain ? ' Save outcome is unresolved. Retry the pending save with the same request.' : ' Your text is retained.');
      editor.tone = 'error';
      if (!uncertain) editor.pending = null;
      editor.conflict = error instanceof ApiError && error.code === 'revision_conflict';
      this.persist();
    } finally {
      this.saving.delete(editor);
      if (this.current === editor) { this.render(); this.renderList(); }
    }
  }

  private async reconcile(keepText: boolean): Promise<void> {
    const editor = this.current;
    if (!editor?.draftId || editor.pending) return;
    const generation = ++this.readGeneration;
    const editGeneration = editor.editGeneration;
    feedback('draft-status', 'Reading the current saved revision to reconcile…');
    try {
      const { data } = await api<Draft>('/drafts/' + editor.draftId);
      if (generation !== this.readGeneration || this.current !== editor || editor.editGeneration !== editGeneration) return;
      if (!keepText) {
        editor.recoveryText = editor.text; editor.text = data.originalText;
      }
      editor.savedText = data.originalText; editor.revision = data.revision; editor.policy = data.requestedPolicy;
      editor.conflict = false; editor.historical = false; editor.editGeneration++;
      editor.status = keepText ? 'Latest revision ' + data.revision + ' loaded. Your local text is retained; save to create a new revision.'
        : 'Saved revision ' + data.revision + ' reloaded. Your previous local text can still be restored.';
      editor.tone = 'neutral'; this.remember(editor); this.render(); this.text.focus();
    } catch (error) {
      if (generation === this.readGeneration && this.current === editor) {
        editor.status = errorMessage(error) + ' Your local text is retained.'; editor.tone = 'error'; this.render();
      }
    }
  }
}
