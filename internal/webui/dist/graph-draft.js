import { ApiError, element, readSession, writeSession } from './api.js';
import { declarationJSON } from './workflow.js';
const blankNode = (key) => ({ key, originalText: '', harness: '', model: '', deliverable: '',
    conditions: '', conditionsProvided: false, timeout: '', checks: '', checksProvided: false, review: '', correctionCycles: '' });
const fresh = () => ({ format: 1, nextKey: 3, revision: 0, title: '', titleProvided: false,
    shared: '', sharedProvided: false, nodes: [blankNode('task_1'), blankNode('task_2')], dependencies: [],
    finalKey: '', delivery: '', external: '', policy: '', parallelism: '2', admissions: '', window: '' });
const make = (tag, text = '') => {
    const result = document.createElement(tag);
    if (text)
        result.textContent = text;
    return result;
};
const select = (choices, value) => {
    const result = make('select');
    for (const [key, label] of choices) {
        const option = make('option', label);
        option.value = key;
        result.append(option);
    }
    result.value = value;
    return result;
};
const field = (parent, label, control) => {
    const wrapper = make('label', label);
    wrapper.append(control);
    parent.append(wrapper);
};
// This editor owns a different project-keyed draft from the standalone Tasks composer.
// Its session cache is a convenience, never a durable admission acknowledgement.
export class GraphComposer {
    draft = fresh();
    drafts = new Map();
    projectId = null;
    deliveryPreference = null;
    onChange = () => { };
    constructor() {
        for (const [id, key, provided] of [
            ['graph-title', 'title', 'titleProvided'], ['graph-shared', 'shared', 'sharedProvided'],
        ])
            element(id).addEventListener('input', event => {
                this.draft[key] = event.target.value;
                this.draft[provided] = true;
                this.changed();
            });
        for (const [id, key] of [
            ['graph-final-node', 'finalKey'], ['graph-delivery', 'delivery'], ['graph-external', 'external'],
            ['graph-policy', 'policy'], ['graph-admissions', 'admissions'], ['graph-window', 'window'],
        ])
            element(id).addEventListener('input', event => {
                this.draft[key] = event.target.value;
                this.changed();
            });
        element('graph-parallelism').addEventListener('change', event => {
            this.draft.parallelism = event.target.value;
            this.changed();
        });
        element('graph-add-node').addEventListener('click', () => {
            if (this.draft.nodes.length >= 16)
                return;
            const key = 'task_' + this.draft.nextKey++;
            this.draft.nodes.push(blankNode(key));
            this.changed();
            this.renderNodes();
            this.focusNode(key);
        });
        element('graph-new-draft').addEventListener('click', () => {
            // An unresolved start belongs to GraphsView and is not discarded by a new editor.
            this.draft = { ...fresh(), revision: this.draft.revision + 1 };
            this.persist();
            this.render();
            this.onChange();
        });
        this.render();
    }
    get revision() { return this.draft.revision; }
    setProject(id) {
        if (this.projectId)
            this.drafts.set(this.projectId, this.draft);
        this.projectId = id;
        const retained = this.drafts.get(id) ?? readSession('graphDraft:' + id);
        // Cache data may be damaged. A malformed cache is never interpreted as a graph declaration.
        this.draft = this.validCache(retained) ? retained : fresh();
        this.drafts.set(id, this.draft);
        this.render();
        this.onChange();
    }
    setDeliveryPreference(preference) { this.deliveryPreference = preference; this.preview(); }
    issues() {
        const issues = [], nodes = this.draft.nodes, keys = new Set(nodes.map(node => node.key));
        if (!nodes.length || nodes.length > 16)
            issues.push('Include between 1 and 16 tasks.');
        for (const node of nodes)
            if (!node.originalText.trim())
                issues.push(node.key + ': enter the complete original instruction.');
        if (this.draft.dependencies.length > 64)
            issues.push('Use at most 64 dependencies.');
        const seen = new Set(), outgoing = new Map();
        for (const edge of this.draft.dependencies) {
            if (!keys.has(edge.from) || !keys.has(edge.to))
                issues.push('Dependency ' + edge.from + ' → ' + edge.to + ' refers to a removed or missing task. Remove it explicitly.');
            if (edge.from === edge.to)
                issues.push('A task cannot depend on itself.');
            const id = edge.from + '\n' + edge.to;
            if (seen.has(id))
                issues.push('Only one dependency may connect the same two tasks.');
            seen.add(id);
            outgoing.set(edge.from, [...(outgoing.get(edge.from) ?? []), edge.to]);
        }
        const visiting = new Set(), done = new Set();
        const cycle = (key) => {
            if (visiting.has(key))
                return true;
            if (done.has(key))
                return false;
            visiting.add(key);
            for (const to of outgoing.get(key) ?? [])
                if (cycle(to))
                    return true;
            visiting.delete(key);
            done.add(key);
            return false;
        };
        if (nodes.some(node => cycle(node.key)))
            issues.push('Dependencies contain a cycle. Remove an edge before starting.');
        if (this.draft.finalKey && !keys.has(this.draft.finalKey))
            issues.push('The selected final-output task was removed. Choose another final output or separate results.');
        if (!this.draft.finalKey && (this.draft.delivery || this.draft.external))
            issues.push('Explicit delivery requires a final-output task.');
        return [...new Set(issues)];
    }
    body(requestId, policy) {
        if (!this.projectId)
            throw new ApiError('invalid_input', 'Select a project first.');
        const issues = this.issues();
        if (issues.length)
            throw new ApiError('invalid_input', issues.join(' '));
        const nodes = this.draft.nodes.map(node => {
            const declaration = { key: node.key, originalText: node.originalText };
            if (node.harness || node.model)
                declaration.pins = {
                    ...(node.harness ? { harnessId: node.harness } : {}),
                    ...(node.model ? { model: node.model } : {}),
                };
            if (node.deliverable || node.conditionsProvided)
                declaration.acceptance = {
                    ...(node.deliverable ? { deliverable: node.deliverable } : {}),
                    ...(node.conditionsProvided ? { conditionsText: node.conditions } : {}),
                };
            if (node.timeout)
                declaration.timeoutSeconds = this.integer(node.timeout, 1, 86400, node.key + ' timeout');
            if (node.checksProvided || node.review || node.correctionCycles)
                declaration.workflow = {
                    ...(node.checksProvided ? { checks: declarationJSON(node.checks, true) } : {}),
                    ...(node.review ? { review: node.review } : {}),
                    ...(node.correctionCycles ? { maxCorrectionCycles: this.integer(node.correctionCycles, 0, 1, node.key + ' correction cycles') } : {}),
                };
            return declaration;
        });
        const body = { requestId, projectId: this.projectId, nodes,
            dependencies: this.draft.dependencies.map(edge => ({ ...edge })), maxParallelism: this.draft.parallelism,
            expectedPolicyRevision: policy.revision,
            ...(this.draft.titleProvided ? { title: this.draft.title } : {}),
            ...(this.draft.sharedProvided ? { sharedInstructions: this.draft.shared } : {}),
            ...(this.draft.policy ? { policyOverride: this.draft.policy } : {}),
        };
        if (this.draft.admissions || this.draft.window)
            body.routing = { budget: {
                    ...(this.draft.admissions ? { admissionLimit: String(this.integer(this.draft.admissions, 1, 16, 'Shared admissions')) } : {}),
                    ...(this.draft.window ? { windowSeconds: this.integer(this.draft.window, 1, 86400, 'Shared window') } : {}),
                } };
        if (this.draft.finalKey) {
            body.finalNodeKey = this.draft.finalKey;
            if (this.draft.delivery || this.draft.external)
                body.delivery = {
                    ...(this.draft.delivery ? { localMode: this.draft.delivery } : {}),
                    ...(this.draft.external ? { externalAction: declarationJSON(this.draft.external, false) } : {}),
                };
        }
        if (new TextEncoder().encode(JSON.stringify(body)).byteLength > 2 * 1024 * 1024)
            throw new ApiError('invalid_input', 'The complete declaration exceeds 2 MiB. Keep all originals and reduce this group explicitly.');
        return body;
    }
    integer(value, min, max, name) {
        if (!/^(0|[1-9][0-9]*)$/.test(value) || Number(value) < min || Number(value) > max)
            throw new ApiError('invalid_input', name + ' must be a whole number from ' + min + ' to ' + max + '.');
        return Number(value);
    }
    validCache(value) {
        if (!value || value.format !== 1 || !Number.isSafeInteger(value.nextKey) || value.nextKey < 1 || !Number.isSafeInteger(value.revision) || value.revision < 0
            || !Array.isArray(value.nodes) || !value.nodes.length || value.nodes.length > 16 || !Array.isArray(value.dependencies) || value.dependencies.length > 64)
            return false;
        const stringKeys = ['title', 'shared', 'finalKey', 'delivery', 'external', 'policy', 'admissions', 'window'];
        if (stringKeys.some(key => typeof value[key] !== 'string') || typeof value.titleProvided !== 'boolean' || typeof value.sharedProvided !== 'boolean' || !['1', '2'].includes(value.parallelism))
            return false;
        const keys = new Set();
        for (const node of value.nodes) {
            if (!node || !/^[A-Za-z0-9][A-Za-z0-9_-]{0,47}$/.test(node.key) || keys.has(node.key))
                return false;
            keys.add(node.key);
            if (Object.entries(blankNode(node.key)).some(([key, initial]) => typeof node[key] !== typeof initial))
                return false;
        }
        return value.dependencies.every(edge => edge && typeof edge.from === 'string' && typeof edge.to === 'string' && (edge.kind === 'files' || edge.kind === 'context'));
    }
    changed() { this.draft.revision++; this.persist(); this.preview(); this.onChange(); }
    persist() { if (this.projectId) {
        this.drafts.set(this.projectId, this.draft);
        writeSession('graphDraft:' + this.projectId, this.draft);
    } }
    focusNode(key) { element('graph-editor-nodes').querySelector('[data-graph-node="' + key + '"] textarea')?.focus(); }
    render() {
        element('graph-title').value = this.draft.title;
        element('graph-shared').value = this.draft.shared;
        for (const [id, key] of [['graph-delivery', 'delivery'], ['graph-external', 'external'], ['graph-policy', 'policy'], ['graph-parallelism', 'parallelism'], ['graph-admissions', 'admissions'], ['graph-window', 'window']])
            element(id).value = this.draft[key];
        this.renderNodes();
        this.preview();
    }
    renderNodes() {
        const list = element('graph-editor-nodes');
        list.replaceChildren();
        this.draft.nodes.forEach((node, index) => {
            const card = make('section');
            card.className = 'graph-editor-node';
            card.dataset.graphNode = node.key;
            const heading = make('div');
            heading.className = 'heading-between';
            const title = make('h4', 'Task ' + (index + 1));
            heading.append(title, make('span', node.key));
            card.append(heading);
            const original = make('textarea');
            original.rows = 5;
            original.value = node.originalText;
            original.dataset.graphField = 'originalText';
            original.addEventListener('input', () => { node.originalText = original.value; this.changed(); });
            field(card, 'Complete original instruction', original);
            const dependencies = make('details');
            const summary = make('summary', 'Depends on (optional)');
            dependencies.append(summary);
            const edgeList = make('ul');
            edgeList.className = 'graph-edges';
            for (const edge of this.draft.dependencies.filter(edge => edge.to === node.key)) {
                const item = make('li', edge.from + ' · ' + (edge.kind === 'context' ? 'Context only' : 'Use verified files') + (this.draft.nodes.some(other => other.key === edge.from) ? '' : ' · removed task'));
                const remove = make('button', 'Remove dependency');
                remove.type = 'button';
                remove.className = 'quiet compact';
                remove.addEventListener('click', () => { const position = this.draft.dependencies.indexOf(edge); if (position < 0)
                    return; this.draft.dependencies.splice(position, 1); this.changed(); this.renderNodes(); this.focusNode(node.key); });
                item.append(remove);
                edgeList.append(item);
            }
            dependencies.append(edgeList);
            const edgeFields = make('div');
            edgeFields.className = 'option-grid';
            const from = select([['', 'Choose another task'], ...this.draft.nodes.filter(other => other.key !== node.key).map(other => [other.key, other.key])], '');
            from.dataset.graphDependencyFrom = '';
            const kind = select([['files', 'Use verified files'], ['context', 'Context only']], 'files');
            kind.dataset.graphDependencyKind = '';
            field(edgeFields, 'Required task', from);
            field(edgeFields, 'Contribution', kind);
            dependencies.append(edgeFields);
            const add = make('button', 'Add dependency');
            add.type = 'button';
            add.className = 'secondary compact';
            add.addEventListener('click', () => {
                if (!from.value || this.draft.dependencies.length >= 64)
                    return;
                this.draft.dependencies.push({ from: from.value, to: node.key, kind: kind.value });
                this.changed();
                this.renderNodes();
                element('graph-editor-nodes').querySelector('[data-graph-node="' + node.key + '"] details')?.setAttribute('open', '');
            });
            dependencies.append(add);
            card.append(dependencies);
            const details = make('details');
            details.append(make('summary', 'Task details (optional)'));
            const options = make('div');
            options.className = 'option-grid';
            const bind = (control, key, provided) => {
                control.dataset.graphField = key;
                control.addEventListener('input', () => {
                    node[key] = control.value;
                    if (provided)
                        node[provided] = true;
                    this.changed();
                });
            };
            const harness = select([['', 'Auto'], ['codex', 'Codex'], ['claude-code', 'Claude Code']], node.harness);
            bind(harness, 'harness');
            field(options, 'Harness', harness);
            const model = make('input');
            model.value = node.model;
            model.placeholder = 'Auto';
            bind(model, 'model');
            field(options, 'Model override', model);
            const deliverable = select([['', 'Auto'], ['candidate', 'Verified file change'], ['response_only', 'Response only']], node.deliverable);
            bind(deliverable, 'deliverable');
            field(options, 'Expected result', deliverable);
            const timeout = make('input');
            timeout.inputMode = 'numeric';
            timeout.value = node.timeout;
            timeout.placeholder = 'Saved default';
            bind(timeout, 'timeout');
            field(options, 'Timeout (seconds)', timeout);
            const review = select([['', 'Auto'], ['required', 'Required review']], node.review);
            bind(review, 'review');
            field(options, 'Review', review);
            const corrections = make('input');
            corrections.inputMode = 'numeric';
            corrections.value = node.correctionCycles;
            corrections.placeholder = 'Default';
            bind(corrections, 'correctionCycles');
            field(options, 'Maximum correction cycles', corrections);
            details.append(options);
            const conditions = make('textarea');
            conditions.rows = 3;
            conditions.value = node.conditions;
            bind(conditions, 'conditions', 'conditionsProvided');
            field(details, 'Acceptance conditions', conditions);
            const checks = make('textarea');
            checks.rows = 3;
            checks.value = node.checks;
            checks.placeholder = '[]';
            checks.spellcheck = false;
            bind(checks, 'checks', 'checksProvided');
            field(details, 'Declared checks (JSON array, optional)', checks);
            details.append(make('p', 'An empty checks array explicitly declares no custom checks. Original instructions and acceptance conditions remain authoritative.'));
            card.append(details);
            const buttons = make('div');
            buttons.className = 'button-row';
            const move = (label, offset) => {
                const button = make('button', label);
                button.type = 'button';
                button.className = 'quiet compact';
                button.disabled = index + offset < 0 || index + offset >= this.draft.nodes.length;
                button.addEventListener('click', () => { this.draft.nodes.splice(index, 1); this.draft.nodes.splice(index + offset, 0, node); this.changed(); this.renderNodes(); this.focusNode(node.key); });
                buttons.append(button);
            };
            move('Move up', -1);
            move('Move down', 1);
            const remove = make('button', 'Remove task');
            remove.type = 'button';
            remove.className = 'quiet compact';
            remove.disabled = this.draft.nodes.length === 1;
            remove.addEventListener('click', () => { this.draft.nodes.splice(index, 1); this.changed(); this.renderNodes(); const next = this.draft.nodes[index] ?? this.draft.nodes[index - 1]; if (next)
                this.focusNode(next.key); });
            buttons.append(remove);
            card.append(buttons);
            list.append(card);
        });
        this.finalChoices();
        this.preview();
    }
    finalChoices() {
        const target = element('graph-final-node');
        target.replaceChildren();
        for (const [key, label] of [['', 'Separate reviewable results'], ...this.draft.nodes.map(node => [node.key, node.key])]) {
            const option = make('option', label);
            option.value = key;
            target.append(option);
        }
        if (this.draft.finalKey && !this.draft.nodes.some(node => node.key === this.draft.finalKey)) {
            const missing = make('option', this.draft.finalKey + ' (removed)');
            missing.value = this.draft.finalKey;
            target.append(missing);
        }
        target.value = this.draft.finalKey;
    }
    preview() {
        const final = !!this.draft.finalKey;
        const mode = this.draft.delivery || this.deliveryPreference?.localMode;
        const scope = final
            ? 'Final output: ' + this.draft.finalKey + '. ' + (mode === 'apply_verified_local' ? 'Only this final task may apply a verified cumulative local change.' : mode === 'review_only' ? 'Review only; no local application.' : 'Saved local-delivery preference has not been observed yet.')
            : 'Separate reviewable results. No local application and no external publication.';
        element('graph-scope-preview').textContent = scope + ' Every declared task is required. Intermediate tasks have no local or external delivery authority.'
            + (final && this.draft.external ? ' The explicit external action is limited to the final task.' : '');
        const issues = this.issues(), target = element('graph-draft-issues');
        target.replaceChildren();
        if (issues.length) {
            const list = make('ul');
            for (const issue of issues)
                list.append(make('li', issue));
            target.append(list);
        }
        const dangling = this.draft.dependencies.filter(edge => !this.draft.nodes.some(node => node.key === edge.from) || !this.draft.nodes.some(node => node.key === edge.to));
        for (const edge of dangling) {
            const button = make('button', 'Remove missing dependency ' + edge.from + ' → ' + edge.to);
            button.type = 'button';
            button.className = 'quiet compact';
            button.addEventListener('click', () => { const position = this.draft.dependencies.indexOf(edge); if (position < 0)
                return; this.draft.dependencies.splice(position, 1); this.changed(); this.renderNodes(); const first = this.draft.nodes[0]; if (first)
                this.focusNode(first.key); });
            target.append(button);
        }
        element('graph-add-node').disabled = this.draft.nodes.length >= 16;
    }
}
