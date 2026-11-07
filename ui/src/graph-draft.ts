import { ApiError, element, readSession, writeSession } from './api.js';
import { declarationJSON } from './workflow.js';
import type { GraphDependencyDeclaration, GraphNodeDeclaration, GraphStartInput } from './graph-types.js';
import type { Policy } from './types.js';
import type { DeliveryPreference, ExternalActionInput, LocalDeliveryMode } from './workflow-types.js';

interface NodeEditor {
  key: string; originalText: string; harness: string; model: string; deliverable: string;
  conditions: string; conditionsProvided: boolean; timeout: string; checks: string;
  checksProvided: boolean; review: string; correctionCycles: string;
}
interface Draft {
  format: 1; nextKey: number; revision: number; title: string; titleProvided: boolean;
  shared: string; sharedProvided: boolean; nodes: NodeEditor[]; dependencies: GraphDependencyDeclaration[];
  finalKey: string; delivery: string; external: string; policy: string; parallelism: '1' | '2';
  admissions: string; window: string;
}
const blankNode = (key: string): NodeEditor => ({ key, originalText: '', harness: '', model: '', deliverable: '',
  conditions: '', conditionsProvided: false, timeout: '', checks: '', checksProvided: false, review: '', correctionCycles: '' });
const fresh = (): Draft => ({ format: 1, nextKey: 3, revision: 0, title: '', titleProvided: false,
  shared: '', sharedProvided: false, nodes: [blankNode('task_1'), blankNode('task_2')], dependencies: [],
  finalKey: '', delivery: '', external: '', policy: '', parallelism: '2', admissions: '', window: '' });
const make = <K extends keyof HTMLElementTagNameMap>(tag: K, text = ''): HTMLElementTagNameMap[K] => {
  const result = document.createElement(tag); if (text) result.textContent = text; return result;
};
const select = (choices: [string, string][], value: string): HTMLSelectElement => {
  const result = make('select');
  for (const [key, label] of choices) { const option = make('option', label); option.value = key; result.append(option); }
  result.value = value; return result;
};
const field = (parent: HTMLElement, label: string, control: HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement): void => {
  const wrapper = make('label', label); wrapper.append(control); parent.append(wrapper);
};

// This editor owns a different project-keyed draft from the standalone Tasks composer.
// Its session cache is a convenience, never a durable admission acknowledgement.
export class GraphComposer {
  private draft = fresh();
  private readonly drafts = new Map<string, Draft>();
  private projectId: string | null = null;
  private deliveryPreference: DeliveryPreference | null = null;
  onChange = (): void => {};

  constructor() {
    for (const [id, key, provided] of [
      ['graph-title', 'title', 'titleProvided'], ['graph-shared', 'shared', 'sharedProvided'],
    ] as const) element<HTMLInputElement | HTMLTextAreaElement>(id).addEventListener('input', event => {
      this.draft[key] = (event.target as HTMLInputElement).value; this.draft[provided] = true; this.changed();
    });
    for (const [id, key] of [
      ['graph-final-node', 'finalKey'], ['graph-delivery', 'delivery'], ['graph-external', 'external'],
      ['graph-policy', 'policy'], ['graph-admissions', 'admissions'], ['graph-window', 'window'],
    ] as const) element<HTMLInputElement | HTMLSelectElement | HTMLTextAreaElement>(id).addEventListener('input', event => {
      this.draft[key] = (event.target as HTMLInputElement).value; this.changed();
    });
    element<HTMLSelectElement>('graph-parallelism').addEventListener('change', event => {
      this.draft.parallelism = (event.target as HTMLSelectElement).value as '1' | '2'; this.changed();
    });
    element('graph-add-node').addEventListener('click', () => {
      if (this.draft.nodes.length >= 16) return;
      const key = 'task_' + this.draft.nextKey++; this.draft.nodes.push(blankNode(key));
      this.changed(); this.renderNodes(); this.focusNode(key);
    });
    element('graph-new-draft').addEventListener('click', () => {
      // An unresolved start belongs to GraphsView and is not discarded by a new editor.
      this.draft = { ...fresh(), revision: this.draft.revision + 1 }; this.persist(); this.render(); this.onChange();
    });
    this.render();
  }

  get revision(): number { return this.draft.revision; }
  setProject(id: string): void {
    if (this.projectId) this.drafts.set(this.projectId, this.draft);
    this.projectId = id;
    const retained = this.drafts.get(id) ?? readSession<Draft>('graphDraft:' + id);
    // Cache data may be damaged. A malformed cache is never interpreted as a graph declaration.
    this.draft = this.validCache(retained) ? retained : fresh(); this.drafts.set(id, this.draft);
    this.render(); this.onChange();
  }
  setDeliveryPreference(preference: DeliveryPreference | null): void { this.deliveryPreference = preference; this.preview(); }

  issues(): string[] {
    const issues: string[] = [], nodes = this.draft.nodes, keys = new Set(nodes.map(node => node.key));
    if (!nodes.length || nodes.length > 16) issues.push('Include between 1 and 16 tasks.');
    for (const node of nodes) if (!node.originalText.trim()) issues.push(node.key + ': enter the complete original instruction.');
    if (this.draft.dependencies.length > 64) issues.push('Use at most 64 dependencies.');
    const seen = new Set<string>(), outgoing = new Map<string, string[]>();
    for (const edge of this.draft.dependencies) {
      if (!keys.has(edge.from) || !keys.has(edge.to)) issues.push('Dependency ' + edge.from + ' → ' + edge.to + ' refers to a removed or missing task. Remove it explicitly.');
      if (edge.from === edge.to) issues.push('A task cannot depend on itself.');
      const id = edge.from + '\n' + edge.to;
      if (seen.has(id)) issues.push('Only one dependency may connect the same two tasks.'); seen.add(id);
      outgoing.set(edge.from, [...(outgoing.get(edge.from) ?? []), edge.to]);
    }
    const visiting = new Set<string>(), done = new Set<string>();
    const cycle = (key: string): boolean => {
      if (visiting.has(key)) return true; if (done.has(key)) return false;
      visiting.add(key); for (const to of outgoing.get(key) ?? []) if (cycle(to)) return true;
      visiting.delete(key); done.add(key); return false;
    };
    if (nodes.some(node => cycle(node.key))) issues.push('Dependencies contain a cycle. Remove an edge before starting.');
    if (this.draft.finalKey && !keys.has(this.draft.finalKey)) issues.push('The selected final-output task was removed. Choose another final output or separate results.');
    if (!this.draft.finalKey && (this.draft.delivery || this.draft.external)) issues.push('Explicit delivery requires a final-output task.');
    return [...new Set(issues)];
  }

  body(requestId: string, policy: Policy): GraphStartInput {
    if (!this.projectId) throw new ApiError('invalid_input', 'Select a project first.');
    const issues = this.issues(); if (issues.length) throw new ApiError('invalid_input', issues.join(' '));
    const nodes: GraphNodeDeclaration[] = this.draft.nodes.map(node => {
      const declaration: GraphNodeDeclaration = { key: node.key, originalText: node.originalText };
      if (node.harness || node.model) declaration.pins = {
        ...(node.harness ? { harnessId: node.harness as 'codex' | 'claude-code' } : {}),
        ...(node.model ? { model: node.model } : {}),
      };
      if (node.deliverable || node.conditionsProvided) declaration.acceptance = {
        ...(node.deliverable ? { deliverable: node.deliverable as 'candidate' | 'response_only' } : {}),
        ...(node.conditionsProvided ? { conditionsText: node.conditions } : {}),
      };
      if (node.timeout) declaration.timeoutSeconds = this.integer(node.timeout, 1, 86400, node.key + ' timeout');
      if (node.checksProvided || node.review || node.correctionCycles) declaration.workflow = {
        ...(node.checksProvided ? { checks: declarationJSON(node.checks, true) as NonNullable<GraphNodeDeclaration['workflow']>['checks'] } : {}),
        ...(node.review ? { review: node.review as 'auto' | 'required' } : {}),
        ...(node.correctionCycles ? { maxCorrectionCycles: this.integer(node.correctionCycles, 0, 1, node.key + ' correction cycles') } : {}),
      };
      return declaration;
    });
    const body: GraphStartInput = { requestId, projectId: this.projectId, nodes,
      dependencies: this.draft.dependencies.map(edge => ({ ...edge })), maxParallelism: this.draft.parallelism,
      expectedPolicyRevision: policy.revision,
      ...(this.draft.titleProvided ? { title: this.draft.title } : {}),
      ...(this.draft.sharedProvided ? { sharedInstructions: this.draft.shared } : {}),
      ...(this.draft.policy ? { policyOverride: this.draft.policy as Policy['mode'] } : {}),
    };
    if (this.draft.admissions || this.draft.window) body.routing = { budget: {
      ...(this.draft.admissions ? { admissionLimit: String(this.integer(this.draft.admissions, 1, 16, 'Shared admissions')) } : {}),
      ...(this.draft.window ? { windowSeconds: this.integer(this.draft.window, 1, 86400, 'Shared window') } : {}),
    } };
    if (this.draft.finalKey) {
      body.finalNodeKey = this.draft.finalKey;
      if (this.draft.delivery || this.draft.external) body.delivery = {
        ...(this.draft.delivery ? { localMode: this.draft.delivery as LocalDeliveryMode } : {}),
        ...(this.draft.external ? { externalAction: declarationJSON(this.draft.external, false) as unknown as ExternalActionInput } : {}),
      };
    }
    if (new TextEncoder().encode(JSON.stringify(body)).byteLength > 2 * 1024 * 1024) throw new ApiError('invalid_input', 'The complete declaration exceeds 2 MiB. Keep all originals and reduce this group explicitly.');
    return body;
  }

  private integer(value: string, min: number, max: number, name: string): number {
    if (!/^(0|[1-9][0-9]*)$/.test(value) || Number(value) < min || Number(value) > max) throw new ApiError('invalid_input', name + ' must be a whole number from ' + min + ' to ' + max + '.');
    return Number(value);
  }
  private validCache(value: Draft | null): value is Draft {
    if (!value || value.format !== 1 || !Number.isSafeInteger(value.nextKey) || value.nextKey < 1 || !Number.isSafeInteger(value.revision) || value.revision < 0
      || !Array.isArray(value.nodes) || !value.nodes.length || value.nodes.length > 16 || !Array.isArray(value.dependencies) || value.dependencies.length > 64) return false;
    const stringKeys = ['title', 'shared', 'finalKey', 'delivery', 'external', 'policy', 'admissions', 'window'] as const;
    if (stringKeys.some(key => typeof value[key] !== 'string') || typeof value.titleProvided !== 'boolean' || typeof value.sharedProvided !== 'boolean' || !['1', '2'].includes(value.parallelism)) return false;
    const keys = new Set<string>();
    for (const node of value.nodes) {
      if (!node || !/^[A-Za-z0-9][A-Za-z0-9_-]{0,47}$/.test(node.key) || keys.has(node.key)) return false;
      keys.add(node.key);
      if (Object.entries(blankNode(node.key)).some(([key, initial]) => typeof node[key as keyof NodeEditor] !== typeof initial)) return false;
    }
    return value.dependencies.every(edge => edge && typeof edge.from === 'string' && typeof edge.to === 'string' && (edge.kind === 'files' || edge.kind === 'context'));
  }
  private changed(): void { this.draft.revision++; this.persist(); this.preview(); this.onChange(); }
  private persist(): void { if (this.projectId) { this.drafts.set(this.projectId, this.draft); writeSession('graphDraft:' + this.projectId, this.draft); } }
  private focusNode(key: string): void { element('graph-editor-nodes').querySelector<HTMLTextAreaElement>('[data-graph-node="' + key + '"] textarea')?.focus(); }
  private render(): void {
    element<HTMLInputElement>('graph-title').value = this.draft.title; element<HTMLTextAreaElement>('graph-shared').value = this.draft.shared;
    for (const [id, key] of [['graph-delivery', 'delivery'], ['graph-external', 'external'], ['graph-policy', 'policy'], ['graph-parallelism', 'parallelism'], ['graph-admissions', 'admissions'], ['graph-window', 'window']] as const)
      element<HTMLInputElement | HTMLSelectElement | HTMLTextAreaElement>(id).value = this.draft[key];
    this.renderNodes(); this.preview();
  }
  private renderNodes(): void {
    const list = element('graph-editor-nodes'); list.replaceChildren();
    this.draft.nodes.forEach((node, index) => {
      const card = make('section'); card.className = 'graph-editor-node'; card.dataset.graphNode = node.key;
      const heading = make('div'); heading.className = 'heading-between';
      const title = make('h4', 'Task ' + (index + 1)); heading.append(title, make('span', node.key)); card.append(heading);
      const original = make('textarea'); original.rows = 5; original.value = node.originalText; original.dataset.graphField = 'originalText';
      original.addEventListener('input', () => { node.originalText = original.value; this.changed(); }); field(card, 'Complete original instruction', original);
      const dependencies = make('details'); const summary = make('summary', 'Depends on (optional)'); dependencies.append(summary);
      const edgeList = make('ul'); edgeList.className = 'graph-edges';
      for (const edge of this.draft.dependencies.filter(edge => edge.to === node.key)) {
        const item = make('li', edge.from + ' · ' + (edge.kind === 'context' ? 'Context only' : 'Use verified files') + (this.draft.nodes.some(other => other.key === edge.from) ? '' : ' · removed task'));
        const remove = make('button', 'Remove dependency'); remove.type = 'button'; remove.className = 'quiet compact';
        remove.addEventListener('click', () => { const position = this.draft.dependencies.indexOf(edge); if (position < 0) return; this.draft.dependencies.splice(position, 1); this.changed(); this.renderNodes(); this.focusNode(node.key); }); item.append(remove); edgeList.append(item);
      }
      dependencies.append(edgeList);
      const edgeFields = make('div'); edgeFields.className = 'option-grid';
      const from = select([['', 'Choose another task'], ...this.draft.nodes.filter(other => other.key !== node.key).map(other => [other.key, other.key] as [string, string])], ''); from.dataset.graphDependencyFrom = '';
      const kind = select([['files', 'Use verified files'], ['context', 'Context only']], 'files'); kind.dataset.graphDependencyKind = '';
      field(edgeFields, 'Required task', from); field(edgeFields, 'Contribution', kind); dependencies.append(edgeFields);
      const add = make('button', 'Add dependency'); add.type = 'button'; add.className = 'secondary compact';
      add.addEventListener('click', () => {
        if (!from.value || this.draft.dependencies.length >= 64) return;
        this.draft.dependencies.push({ from: from.value, to: node.key, kind: kind.value as 'files' | 'context' }); this.changed(); this.renderNodes();
        element('graph-editor-nodes').querySelector<HTMLDetailsElement>('[data-graph-node="' + node.key + '"] details')?.setAttribute('open', '');
      }); dependencies.append(add); card.append(dependencies);
      const details = make('details'); details.append(make('summary', 'Task details (optional)'));
      const options = make('div'); options.className = 'option-grid';
      const bind = (control: HTMLInputElement | HTMLSelectElement | HTMLTextAreaElement, key: keyof NodeEditor, provided?: 'conditionsProvided' | 'checksProvided'): void => {
        control.dataset.graphField = key; control.addEventListener('input', () => {
          (node as unknown as Record<string, unknown>)[key] = control.value; if (provided) node[provided] = true; this.changed();
        });
      };
      const harness = select([['', 'Auto'], ['codex', 'Codex'], ['claude-code', 'Claude Code']], node.harness); bind(harness, 'harness'); field(options, 'Harness', harness);
      const model = make('input'); model.value = node.model; model.placeholder = 'Auto'; bind(model, 'model'); field(options, 'Model override', model);
      const deliverable = select([['', 'Auto'], ['candidate', 'Verified file change'], ['response_only', 'Response only']], node.deliverable); bind(deliverable, 'deliverable'); field(options, 'Expected result', deliverable);
      const timeout = make('input'); timeout.inputMode = 'numeric'; timeout.value = node.timeout; timeout.placeholder = 'Saved default'; bind(timeout, 'timeout'); field(options, 'Timeout (seconds)', timeout);
      const review = select([['', 'Auto'], ['required', 'Required review']], node.review); bind(review, 'review'); field(options, 'Review', review);
      const corrections = make('input'); corrections.inputMode = 'numeric'; corrections.value = node.correctionCycles; corrections.placeholder = 'Default'; bind(corrections, 'correctionCycles'); field(options, 'Maximum correction cycles', corrections); details.append(options);
      const conditions = make('textarea'); conditions.rows = 3; conditions.value = node.conditions; bind(conditions, 'conditions', 'conditionsProvided'); field(details, 'Acceptance conditions', conditions);
      const checks = make('textarea'); checks.rows = 3; checks.value = node.checks; checks.placeholder = '[]'; checks.spellcheck = false; bind(checks, 'checks', 'checksProvided'); field(details, 'Declared checks (JSON array, optional)', checks);
      details.append(make('p', 'An empty checks array explicitly declares no custom checks. Original instructions and acceptance conditions remain authoritative.')); card.append(details);
      const buttons = make('div'); buttons.className = 'button-row';
      const move = (label: string, offset: number): void => {
        const button = make('button', label); button.type = 'button'; button.className = 'quiet compact'; button.disabled = index + offset < 0 || index + offset >= this.draft.nodes.length;
        button.addEventListener('click', () => { this.draft.nodes.splice(index, 1); this.draft.nodes.splice(index + offset, 0, node); this.changed(); this.renderNodes(); this.focusNode(node.key); }); buttons.append(button);
      };
      move('Move up', -1); move('Move down', 1);
      const remove = make('button', 'Remove task'); remove.type = 'button'; remove.className = 'quiet compact'; remove.disabled = this.draft.nodes.length === 1;
      remove.addEventListener('click', () => { this.draft.nodes.splice(index, 1); this.changed(); this.renderNodes(); const next = this.draft.nodes[index] ?? this.draft.nodes[index - 1]; if (next) this.focusNode(next.key); }); buttons.append(remove); card.append(buttons); list.append(card);
    });
    this.finalChoices(); this.preview();
  }
  private finalChoices(): void {
    const target = element<HTMLSelectElement>('graph-final-node'); target.replaceChildren();
    for (const [key, label] of [['', 'Separate reviewable results'] as [string, string], ...this.draft.nodes.map(node => [node.key, node.key] as [string, string])]) {
      const option = make('option', label); option.value = key; target.append(option);
    }
    if (this.draft.finalKey && !this.draft.nodes.some(node => node.key === this.draft.finalKey)) {
      const missing = make('option', this.draft.finalKey + ' (removed)'); missing.value = this.draft.finalKey; target.append(missing);
    }
    target.value = this.draft.finalKey;
  }
  private preview(): void {
    const final = !!this.draft.finalKey;
    const mode = this.draft.delivery || this.deliveryPreference?.localMode;
    const scope = final
      ? 'Final output: ' + this.draft.finalKey + '. ' + (mode === 'apply_verified_local' ? 'Only this final task may apply a verified cumulative local change.' : mode === 'review_only' ? 'Review only; no local application.' : 'Saved local-delivery preference has not been observed yet.')
      : 'Separate reviewable results. No local application and no external publication.';
    element('graph-scope-preview').textContent = scope + ' Every declared task is required. Intermediate tasks have no local or external delivery authority.'
      + (final && this.draft.external ? ' The explicit external action is limited to the final task.' : '');
    const issues = this.issues(), target = element('graph-draft-issues'); target.replaceChildren();
    if (issues.length) { const list = make('ul'); for (const issue of issues) list.append(make('li', issue)); target.append(list); }
    const dangling = this.draft.dependencies.filter(edge => !this.draft.nodes.some(node => node.key === edge.from) || !this.draft.nodes.some(node => node.key === edge.to));
    for (const edge of dangling) {
      const button = make('button', 'Remove missing dependency ' + edge.from + ' → ' + edge.to); button.type = 'button'; button.className = 'quiet compact';
      button.addEventListener('click', () => { const position = this.draft.dependencies.indexOf(edge); if (position < 0) return; this.draft.dependencies.splice(position, 1); this.changed(); this.renderNodes(); const first = this.draft.nodes[0]; if (first) this.focusNode(first.key); }); target.append(button);
    }
    element<HTMLButtonElement>('graph-add-node').disabled = this.draft.nodes.length >= 16;
  }
}
