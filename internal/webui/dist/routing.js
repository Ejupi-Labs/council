import { addFact, api, ApiError, element, errorMessage, feedback, readSession, requestId, timeLabel, writeSession } from './api.js';
import { usageLabel } from './native-observations.js';
const value = (id) => element(id).value;
const checked = (id) => element(id).checked;
const subjectKey = (s) => s.kind + ':' + s.id;
const subjectQuery = (s) => (s.kind === 'context' ? 'contextId=' : 'accountId=') + encodeURIComponent(s.id);
const targetLabel = (d) => d.selectedTarget.contextId + ' · ' + (d.selectedTarget.modelSelector || 'native configured default') + (d.selectedTarget.profileId ? ' · ' + d.selectedTarget.profileId : '');
function options(id, items, fallback = '') {
    const select = element(id);
    const previous = select.selectedIndex < 0 ? fallback : select.value;
    select.replaceChildren(...items.map(item => { const option = document.createElement('option'); option.value = item.value; option.textContent = item.label; return option; }));
    select.value = items.some(item => item.value === previous) ? previous : fallback;
}
function exactUnits(id, optional = false, maximum = 16) {
    const text = value(id);
    if (!text && optional)
        return;
    if (!/^[1-9][0-9]*$/.test(text) || BigInt(text) > BigInt(maximum))
        throw new ApiError('invalid_input', 'Admission units must be a whole number from 1 through ' + maximum + '.');
    return text;
}
function seconds(id, optional = false) {
    const text = value(id);
    if (!text && optional)
        return;
    if (!/^[1-9][0-9]*$/.test(text) || BigInt(text) > 86400n)
        throw new ApiError('invalid_input', 'Set seconds from 1 through 86400 or leave the optional value empty.');
    return Number(text);
}
function evidence(list, title, data) {
    const item = document.createElement('li');
    const details = document.createElement('details');
    const summary = document.createElement('summary');
    summary.textContent = title;
    const pre = document.createElement('pre');
    pre.className = 'evidence-text';
    pre.tabIndex = 0;
    pre.textContent = JSON.stringify(data, null, 2);
    details.append(summary, pre);
    item.append(details);
    list.append(item);
}
export class RoutingView {
    ready = false;
    generation = 0;
    capacityGeneration = 0;
    taskGeneration = 0;
    historyGeneration = 0;
    policy = null;
    contexts = [];
    profiles = [];
    accounts = [];
    accountsCursor = null;
    reportCursor = null;
    capacity = null;
    detail = null;
    taskRouting = null;
    taskBudget = null;
    budgetEditorTaskId = null;
    cursors = {};
    busy = new Set();
    pending = readSession('routingPending') ?? {};
    adoptStart = readSession('routingAdoptStart');
    editors = readSession('routingEditors') ?? {};
    composer = readSession('routingComposer') ?? {};
    guidanceEditors = readSession('routingGuidanceEditors') ?? {};
    onTaskChanged = async () => { };
    onTaskObserved = () => { };
    constructor() {
        for (const pending of Object.values(this.pending))
            pending.unresolved = true;
        if (this.adoptStart)
            this.adoptStart.amendment.unresolved = true;
        const composerIds = ['task-suitability', 'task-profile', 'task-budget-admissions', 'task-budget-window', 'task-automatic-admissions', 'task-allow-replacement', 'task-allow-escalation', 'task-model', 'task-timeout', 'task-deliverable', 'task-conditions', 'task-required-evidence', 'task-final-only', 'task-fake', 'task-fake-scenario'];
        for (const id of composerIds) {
            this.restore(id, this.composer[id]);
            element(id).addEventListener('change', () => { this.composer[id] = this.readEditor(id); writeSession('routingComposer', this.composer); });
            element(id).addEventListener('input', () => { this.composer[id] = this.readEditor(id); writeSession('routingComposer', this.composer); });
        }
        for (const node of document.querySelectorAll('[data-routing-editor]')) {
            this.restore(node.id, this.editors[this.editorKey(node.id)]);
            const save = () => { this.editors[this.editorKey(node.id)] = this.readEditor(node.id); writeSession('routingEditors', this.editors); };
            node.addEventListener('input', save);
            node.addEventListener('change', save);
        }
        for (const node of document.querySelectorAll('[data-routing-guidance]')) {
            node.addEventListener('change', () => { if (!this.detail)
                return; const saved = this.guidanceEditors[this.detail.task.id] ?? {}; saved[node.id] = this.readEditor(node.id); this.guidanceEditors[this.detail.task.id] = saved; writeSession('routingGuidanceEditors', this.guidanceEditors); });
            node.addEventListener('input', () => { if (!this.detail)
                return; const saved = this.guidanceEditors[this.detail.task.id] ?? {}; saved[node.id] = this.readEditor(node.id); this.guidanceEditors[this.detail.task.id] = saved; writeSession('routingGuidanceEditors', this.guidanceEditors); });
        }
        element('refresh-routing').addEventListener('click', () => { void this.refresh(); });
        element('routing-subject').addEventListener('change', () => { void this.loadCapacity(); });
        element('routing-report-dimension').addEventListener('change', () => this.reportStates());
        element('routing-report-form').addEventListener('submit', event => { event.preventDefault(); void this.saveReport(); });
        element('routing-account-form').addEventListener('submit', event => { event.preventDefault(); void this.saveAccount(); });
        element('routing-account-edit').addEventListener('change', () => this.accountFields());
        element('routing-link-form').addEventListener('submit', event => { event.preventDefault(); void this.saveLink(); });
        element('routing-context-form').addEventListener('submit', event => { event.preventDefault(); void this.saveContext(); });
        element('routing-policy-form').addEventListener('submit', event => { event.preventDefault(); void this.savePolicy(); });
        element('routing-profile-form').addEventListener('submit', event => { event.preventDefault(); void this.saveProfile(); });
        element('routing-profile-edit').addEventListener('change', () => this.profileFields());
        element('routing-link-context').addEventListener('change', () => this.contextFields());
        element('more-routing-accounts').addEventListener('click', () => { void this.loadAccounts(); });
        element('load-routing-reports').addEventListener('click', () => { void this.loadReports(false); });
        element('more-routing-reports').addEventListener('click', () => { void this.loadReports(true); });
        element('grant-task-budget').addEventListener('click', () => { void this.amend('grant'); });
        element('adopt-task-budget').addEventListener('click', () => { void this.amend('adopt'); });
        element('adopt-start-task').addEventListener('click', () => { void this.adoptAndStart(); });
        for (const kind of ['decisions', 'ledger', 'usage']) {
            element('load-task-' + kind).addEventListener('click', () => { void this.loadHistory(kind, false); });
            element('more-task-' + kind).addEventListener('click', () => { void this.loadHistory(kind, true); });
        }
        this.reportStates();
        this.controls();
    }
    editorKey(id) {
        if (['routing-context-label', 'routing-context-enabled', 'routing-link-account'].includes(id))
            return 'context:' + value('routing-link-context') + ':' + id;
        if (['routing-profile-enabled', 'routing-reference-price', 'routing-clear-price'].includes(id))
            return 'profile:' + value('routing-profile-edit') + ':' + id;
        if (['routing-account-label', 'routing-account-provider', 'routing-account-entitlement'].includes(id))
            return 'account:' + value('routing-account-edit') + ':' + id;
        if (['budget-additional-admissions', 'budget-new-window'].includes(id))
            return 'task:' + this.detail?.task.id + ':' + id;
        return id;
    }
    readEditor(id) { const node = element(id); return node.type === 'checkbox' ? node.checked : node.value; }
    restore(id, saved) { if (saved === undefined)
        return; const node = element(id); if (typeof saved === 'boolean')
        node.checked = saved;
    else if (!(node instanceof HTMLSelectElement) || Array.from(node.options).some(option => option.value === saved))
        node.value = saved; }
    setReady(ready) { this.ready = ready; if (!ready) {
        this.generation++;
        this.capacityGeneration++;
        this.taskGeneration++;
        this.historyGeneration++;
        this.busy.delete('refresh');
    } this.controls(); }
    composerOptions() {
        const routing = {};
        if (value('task-suitability'))
            routing.suitability = value('task-suitability');
        if (value('task-profile'))
            routing.targetProfileId = value('task-profile');
        const admissions = exactUnits('task-budget-admissions', true);
        const window = seconds('task-budget-window', true);
        if (admissions || window)
            routing.budget = { ...(admissions ? { admissionLimit: admissions } : {}), ...(window ? { windowSeconds: window } : {}) };
        const automatic = value('task-automatic-admissions');
        if (automatic || checked('task-allow-replacement') || checked('task-allow-escalation')) {
            if (!/^(0|[1-9][0-9]*)$/.test(automatic || '0') || BigInt(automatic || '0') > 15n)
                throw new ApiError('invalid_input', 'Automatic admissions must be from 0 through 15.');
            routing.continuation = { maxAutomaticAdmissions: automatic || '0', categories: automatic && automatic !== '0' ? ['pre_launch_ineligible'] : [], allowReplacement: checked('task-allow-replacement'), allowEscalation: checked('task-allow-escalation') };
        }
        return Object.keys(routing).length ? routing : undefined;
    }
    guidanceOptions() {
        const useCurrent = checked('guidance-current-routing');
        const suitability = value('guidance-suitability');
        const profile = value('guidance-profile');
        if (!useCurrent && suitability === 'keep' && profile === 'keep')
            return;
        if (!this.taskRouting?.authority)
            throw new ApiError('routing_adoption_required', 'Wait for captured routing authority; legacy tasks require explicit budget adoption before new routing authority.');
        const captured = this.taskRouting.authority.optionSnapshot;
        const routing = { ...captured, ...(useCurrent ? { useCurrentPolicy: true, ...(this.policy ? { expectedPolicyRevision: this.policy.revision } : {}) } : {}) };
        delete routing.budget;
        if (!useCurrent) {
            delete routing.expectedPolicyRevision;
            delete routing.useCurrentPolicy;
        }
        if (suitability !== 'keep')
            routing.suitability = suitability;
        if (profile !== 'keep') {
            if (profile)
                routing.targetProfileId = profile;
            else
                delete routing.targetProfileId;
        }
        return routing;
    }
    guidanceSnapshot() {
        return Object.fromEntries(Array.from(document.querySelectorAll('[data-routing-guidance]')).map(node => [node.id, this.readEditor(node.id)]));
    }
    finishGuidance(taskId, clicked) {
        const current = this.detail?.task.id === taskId;
        const saved = this.guidanceEditors[taskId] ?? {};
        for (const [id, original] of Object.entries(clicked))
            if ((current ? this.readEditor(id) : saved[id]) === original) {
                delete saved[id];
                if (current)
                    this.restore(id, id === 'guidance-current-routing' ? false : id === 'guidance-model' ? '' : 'keep');
            }
        if (Object.keys(saved).length)
            this.guidanceEditors[taskId] = saved;
        else
            delete this.guidanceEditors[taskId];
        writeSession('routingGuidanceEditors', this.guidanceEditors);
    }
    async refresh() {
        if (!this.ready)
            return;
        const generation = ++this.generation;
        this.busy.add('refresh');
        this.controls();
        try {
            const [policy, contexts, profiles, accounts] = await Promise.all([api('/routing/policy'), api('/routing/contexts'), api('/routing/profiles'), api('/routing/accounts')]);
            if (generation !== this.generation)
                return;
            this.policy = policy.data;
            this.contexts = contexts.data.items;
            this.profiles = profiles.data.items;
            this.accounts = accounts.data.items;
            this.accountsCursor = accounts.data.nextCursor;
            this.renderConfiguration();
            await this.loadCapacity();
            feedback('routing-config-status', 'Current routing declarations refreshed. No provider access, login or model call was made.');
        }
        catch (error) {
            if (generation === this.generation)
                feedback('routing-config-status', errorMessage(error) + ' Prior declarations may be stale; local input is retained.', 'error');
        }
        finally {
            if (generation === this.generation) {
                this.busy.delete('refresh');
                this.controls();
            }
        }
    }
    renderConfiguration() {
        const p = this.policy;
        if (!p)
            return;
        element('routing-policy-summary').textContent = 'Auto prefers suitable economical profiles. Unknown availability: ' + p.unknownAvailability + '. Default budget: ' + p.defaultBudget.admissionLimit + ' admitted attempts / ' + p.defaultBudget.windowSeconds + ' wall-clock seconds. Automatic continuation: ' + (p.continuation.maxAutomaticAdmissions === '0' ? 'OFF' : p.continuation.maxAutomaticAdmissions + ' maximum') + '. Current policy revision ' + p.revision + '. Access and native charges remain unknown.';
        element('task-budget-admissions').placeholder = p.defaultBudget.admissionLimit;
        element('task-budget-window').placeholder = String(p.defaultBudget.windowSeconds);
        element('task-automatic-admissions').placeholder = p.continuation.maxAutomaticAdmissions;
        const profiles = this.profiles.map(profile => ({ value: profile.id, label: profile.id + (profile.enabled ? '' : ' · disabled') }));
        options('task-profile', [{ value: '', label: 'Auto · suitable economical profile' }, ...profiles]);
        this.restore('task-profile', this.composer['task-profile']);
        options('guidance-profile', [{ value: 'keep', label: 'Keep captured profile choice' }, { value: '', label: 'Auto · no explicit profile' }, ...profiles], 'keep');
        options('routing-profile-edit', profiles, profiles[0]?.value ?? '');
        options('routing-link-context', this.contexts.map(context => ({ value: context.id, label: context.label })), 'codex-native');
        const retainedAccountIds = [String(this.editors['routing-account-edit'] ?? ''), ...this.contexts.map(context => context.accountLink.declaredAccountId ?? ''), ...this.contexts.map(context => String(this.editors['context:' + context.id + ':routing-link-account'] ?? '')), String(this.editors['routing-subject'] ?? '').replace(/^account:/, '')].filter(id => /^[a-f0-9]{32}$/.test(id));
        const accountOptions = this.accounts.map(account => ({ value: account.id, label: account.label + ' · declared account' }));
        for (const id of new Set(retainedAccountIds))
            if (!this.accounts.some(account => account.id === id))
                accountOptions.push({ value: id, label: id + ' · declared identity; load more declarations for current label/edit revision' });
        options('routing-link-account', [{ value: '', label: 'No declared association' }, ...accountOptions]);
        options('routing-account-edit', [{ value: '', label: 'Create a declaration' }, ...accountOptions]);
        options('routing-subject', [...this.contexts.map(context => ({ value: 'context:' + context.id, label: context.label + ' · context' })), ...accountOptions.map(account => ({ value: 'account:' + account.value, label: account.label }))], 'context:codex-native');
        for (const id of ['routing-subject', 'routing-link-context', 'routing-link-account', 'routing-account-edit', 'routing-profile-edit'])
            this.restore(id, this.editors[id]);
        for (const [id, fallback] of [['routing-unknown-availability', p.unknownAvailability], ['routing-preference-order', p.economyOrder.join('\n')], ['routing-default-admissions', p.defaultBudget.admissionLimit], ['routing-default-window', String(p.defaultBudget.windowSeconds)], ['routing-default-automatic', p.continuation.maxAutomaticAdmissions]])
            this.restore(id, this.editors[id] ?? fallback);
        this.restore('routing-default-replacement', this.editors['routing-default-replacement'] ?? p.continuation.allowReplacement);
        this.restore('routing-default-escalation', this.editors['routing-default-escalation'] ?? p.continuation.allowEscalation);
        this.contextFields();
        this.accountFields();
        this.profileFields();
        element('more-routing-accounts').hidden = !this.accountsCursor;
    }
    contextFields() {
        const context = this.contexts.find(item => item.id === value('routing-link-context'));
        if (!context)
            return;
        this.restore('routing-context-label', this.editors[this.editorKey('routing-context-label')] ?? context.label);
        this.restore('routing-context-enabled', this.editors[this.editorKey('routing-context-enabled')] ?? context.enabled);
        this.restore('routing-link-account', this.editors[this.editorKey('routing-link-account')] ?? context.accountLink.declaredAccountId ?? '');
        element('routing-link-facts').textContent = 'Context revision ' + context.revision + '; association revision ' + context.accountLink.revision + '. Association is user-declared; actual native login is unknown. Unlinking does not clear retained negatives.';
    }
    accountFields() {
        const account = this.accounts.find(item => item.id === value('routing-account-edit'));
        this.restore('routing-account-label', this.editors[this.editorKey('routing-account-label')] ?? account?.label ?? '');
        this.restore('routing-account-provider', this.editors[this.editorKey('routing-account-provider')] ?? account?.providerLabel ?? '');
        this.restore('routing-account-entitlement', this.editors[this.editorKey('routing-account-entitlement')] ?? account?.entitlement ?? 'unknown');
    }
    profileFields() {
        const profile = this.profiles.find(item => item.id === value('routing-profile-edit'));
        if (!profile)
            return;
        this.restore('routing-profile-enabled', this.editors[this.editorKey('routing-profile-enabled')] ?? profile.enabled);
        this.restore('routing-reference-price', this.editors[this.editorKey('routing-reference-price')] ?? '');
        this.restore('routing-clear-price', this.editors[this.editorKey('routing-clear-price')] ?? false);
        const list = element('routing-profile-facts');
        list.replaceChildren();
        addFact(list, 'Fixed selector', profile.modelSelector || 'Native configured default');
        addFact(list, 'Declared role', profile.role);
        addFact(list, 'Source assumption', profile.source.label + ' · read ' + profile.source.readDate + ' · minimum version ' + (profile.source.minimumNativeVersion || 'unknown'));
        addFact(list, 'Actual model access', 'Unknown; a catalog declaration is not entitlement or qualification.');
        addFact(list, 'Reference price', profile.referencePrice ? JSON.stringify(profile.referencePrice) : 'Unknown · no price reference');
    }
    subject() { const [kind, id] = value('routing-subject').split(':'); if ((kind !== 'context' && kind !== 'account') || !id)
        throw new ApiError('invalid_input', 'Choose a context or declared account.'); return { kind, id }; }
    async loadCapacity() {
        if (!this.ready)
            return;
        const subject = this.subject();
        const generation = ++this.capacityGeneration;
        this.capacity = null;
        this.reportCursor = null;
        element('routing-negative-evidence').replaceChildren();
        element('routing-report-detail').hidden = true;
        element('routing-report-detail').textContent = '';
        element('routing-reports-list').replaceChildren();
        element('more-routing-reports').hidden = true;
        this.controls();
        try {
            const { data } = await api('/routing/capacity?' + subjectQuery(subject));
            if (generation !== this.capacityGeneration || subjectKey(subject) !== value('routing-subject'))
                return;
            if (subjectKey(data.subject) !== subjectKey(subject))
                throw new ApiError('invalid_response', 'The capacity response does not identify the selected subject.');
            this.capacity = data;
            this.renderCapacity();
        }
        catch (error) {
            if (generation === this.capacityGeneration)
                feedback('routing-report-status', errorMessage(error) + ' Refresh before recording a new observation.', 'error');
        }
        finally {
            if (generation === this.capacityGeneration)
                this.controls();
        }
    }
    renderCapacity() {
        const capacity = this.capacity;
        if (!capacity)
            return;
        const facts = element('routing-capacity-facts');
        facts.replaceChildren();
        addFact(facts, 'Subject', subjectKey(capacity.subject));
        addFact(facts, 'Current capacity revision', capacity.revision + ' · as of ' + timeLabel(capacity.asOf));
        addFact(facts, 'Actual login', 'Unknown · declarations and associations cannot select native authentication.');
        const refs = [];
        for (const [dimension, title] of [['occupancy', 'External occupancy'], ['allowance', 'Allowance'], ['service', 'Service state']]) {
            const state = capacity.dimensions[dimension];
            const report = state.latestReport;
            addFact(facts, title, state.effectiveState + (report ? ' · latest report ' + report.reportId + ' ' + report.state + ' · ' + report.source + ' · observed ' + timeLabel(report.observedAt) + ' · ' + report.freshness + ' · expires ' + (report.expiresAt ? timeLabel(report.expiresAt) : 'unknown') : ' · no sourced report'));
            for (const denial of state.blockingReports) {
                refs.push(denial);
                addFact(facts, 'Retained blocking report', denial.reportId + ' · ' + denial.state + ' · ' + denial.source + ' · observed ' + timeLabel(denial.observedAt) + ' · ' + denial.freshness + ' · expires ' + (denial.expiresAt ? timeLabel(denial.expiresAt) : 'unknown') + ' · origin ' + subjectKey(denial.subject) + (denial.inheritedFrom ? ' · inherited from ' + denial.inheritedFrom.accountId + ' at link revision ' + denial.inheritedFrom.linkRevision : ''));
            }
        }
        const evidenceButtons = element('routing-negative-evidence');
        evidenceButtons.replaceChildren();
        for (const ref of new Map(refs.map(ref => [ref.reportId, ref])).values()) {
            const button = document.createElement('button');
            button.type = 'button';
            button.className = 'quiet';
            button.textContent = 'Read exact blocking report ' + ref.reportId.slice(0, 8);
            button.addEventListener('click', () => { void this.readReport(ref); });
            evidenceButtons.append(button);
        }
        addFact(facts, 'Reported eligibility', (capacity.reportEligibility.permitted ? 'Permitted under report policy' : 'Blocked under report policy') + ' · ' + capacity.reportEligibility.policy + ' · occupancy positive ' + capacity.reportEligibility.positiveCoverage.occupancy + ' · allowance positive ' + capacity.reportEligibility.positiveCoverage.allowance + ' · ' + capacity.reportEligibility.reasonCodes.join(', ') + '. Authentication and model access remain unknown.');
        addFact(facts, 'Council managed occupancy', capacity.managedOccupancy.activeAttemptId ? 'One worker claimed by ' + capacity.managedOccupancy.activeAttemptId + ' · context ' + (capacity.managedOccupancy.activeContextId || 'unknown') + ' · applies to this subject ' + capacity.managedOccupancy.claimsSubject : 'No Council worker claim. External idleness is not established.');
        const corrections = [{ value: '', label: 'New observation (no correction)' }, ...Array.from(new Map(refs.map(ref => [ref.reportId, ref])).values()).map(ref => ({ value: ref.reportId, label: ref.reportId + ' · ' + ref.state + ' · origin ' + subjectKey(ref.subject) }))];
        const localCorrection = this.editors['routing-report-correction'];
        if (typeof localCorrection === 'string' && /^[a-f0-9]{32}$/.test(localCorrection) && !corrections.some(option => option.value === localCorrection))
            corrections.push({ value: localCorrection, label: localCorrection + ' · retained local choice; not a current blocking reference' });
        options('routing-report-correction', corrections);
        this.restore('routing-report-correction', localCorrection);
    }
    async readReport(ref) {
        const generation = this.capacityGeneration;
        try {
            const { data } = await api('/routing/reports/' + ref.reportId);
            if (generation !== this.capacityGeneration)
                return;
            if (data.report.id !== ref.reportId || subjectKey(data.report.subject) !== subjectKey(ref.subject))
                throw new ApiError('invalid_response', 'This report does not match the selected evidence reference.');
            const output = element('routing-report-detail');
            output.hidden = false;
            output.textContent = JSON.stringify(data, null, 2);
        }
        catch (error) {
            if (generation === this.capacityGeneration)
                feedback('routing-report-status', errorMessage(error), 'error');
        }
    }
    reportStates() {
        const dimension = value('routing-report-dimension');
        const states = dimension === 'occupancy' ? ['unknown', 'busy', 'available'] : dimension === 'allowance' ? ['unknown', 'available', 'exhausted'] : ['unknown', 'temporary_throttle', 'temporary_overload', 'authentication_denied', 'model_denied', 'permission_denied'];
        options('routing-report-state', states.map(state => ({ value: state, label: state })), 'unknown');
        this.restore('routing-report-state', this.editors['routing-report-state']);
    }
    async mutate(key, build, status) {
        if (!this.ready || this.busy.has(key))
            return null;
        this.busy.add(key);
        this.controls();
        let pending = this.pending[key];
        const wasUnresolved = pending?.unresolved === true;
        try {
            if (!pending) {
                pending = await build();
                this.pending[key] = pending;
            }
            pending.unresolved = true;
            writeSession('routingPending', this.pending);
            this.controls();
            const { data } = await api(pending.path, pending.method, pending.body);
            delete this.pending[key];
            return data;
        }
        catch (error) {
            const uncertain = wasUnresolved || error instanceof ApiError && error.uncertain;
            if (!uncertain)
                delete this.pending[key];
            feedback(status, errorMessage(error, pending ? String(pending.body['requestId']) : undefined) + (uncertain ? ' Retry this exact pending operation to reconcile. Newer input is retained.' : ' Local input is retained. Refresh current revisions before a new request.'), 'error');
            return null;
        }
        finally {
            this.busy.delete(key);
            writeSession('routingPending', this.pending);
            this.controls();
        }
    }
    async saveReport() {
        const result = await this.mutate('report', async () => {
            const capacity = this.capacity;
            if (!capacity || subjectKey(capacity.subject) !== value('routing-subject'))
                throw new ApiError('invalid_input', 'Wait for this subject’s current capacity before reporting.');
            const dimension = value('routing-report-dimension');
            const correction = value('routing-report-correction');
            const origin = Object.values(capacity.dimensions).flatMap(d => d.blockingReports).find(ref => ref.reportId === correction);
            if (correction && !origin)
                throw new ApiError('report_conflict', 'This selected correction is no longer a retained negative in the current view. Inspect its exact history before choosing a new correction.');
            const subject = origin?.subject ?? capacity.subject;
            const body = { requestId: requestId(), subject, expectedRevision: capacity.revision, dimension, state: value('routing-report-state'), validForSeconds: seconds('routing-report-validity'),
                ...(value('routing-report-note') ? { note: value('routing-report-note') } : {}), ...(value('routing-report-observed') ? { observedAt: value('routing-report-observed') } : {}), ...(value('routing-report-retry-after') ? { retryAfter: value('routing-report-retry-after') } : {}),
                ...(correction ? { correctionOf: correction, correctionAction: value('routing-correction-action') } : {}) };
            if (origin && subjectKey(subject) !== subjectKey(capacity.subject)) {
                const current = await api('/routing/capacity?' + subjectQuery(subject));
                body['expectedRevision'] = current.data.revision;
            }
            return { path: '/routing/reports', method: 'POST', body };
        }, 'routing-report-status');
        if (result) {
            feedback('routing-report-status', 'User report recorded. Original source, time, bindings and unresolved negatives remain inspectable.', 'success');
            await this.loadCapacity();
        }
    }
    async saveAccount() {
        const result = await this.mutate('account', () => {
            const id = value('routing-account-edit');
            const account = this.accounts.find(item => item.id === id);
            if (id && !account)
                throw new ApiError('invalid_input', 'Load this declaration’s current page/revision before editing it. The selected identity and local fields are retained.');
            return { path: '/routing/accounts' + (id ? '/' + id : ''), method: id ? 'PUT' : 'POST', body: { requestId: requestId(), ...(id ? { expectedRevision: account?.revision } : {}), label: value('routing-account-label'), ...(id || value('routing-account-provider') ? { providerLabel: value('routing-account-provider') } : {}), entitlement: value('routing-account-entitlement') } };
        }, 'routing-config-status');
        if (result) {
            feedback('routing-config-status', 'Account declaration saved. Actual login and enforceable account selection remain unknown/unsupported.', 'success');
            await this.refresh();
        }
    }
    async saveLink() {
        const result = await this.mutate('link', () => { const context = this.contexts.find(item => item.id === value('routing-link-context')); if (!context)
            throw new ApiError('invalid_input', 'Refresh the context first.'); return { path: '/routing/contexts/' + context.id + '/account-link', method: 'PUT', body: { requestId: requestId(), expectedRevision: context.accountLink.revision, ...(value('routing-link-account') ? { declaredAccountId: value('routing-link-account') } : { unlink: true }) } }; }, 'routing-config-status');
        if (result) {
            feedback('routing-config-status', 'Declared association saved without changing native login or clearing retained negatives.', 'success');
            await this.refresh();
        }
    }
    async saveContext() {
        const result = await this.mutate('context', () => { const context = this.contexts.find(item => item.id === value('routing-link-context')); if (!context)
            throw new ApiError('invalid_input', 'Refresh the context first.'); return { path: '/routing/contexts/' + context.id, method: 'PUT', body: { requestId: requestId(), expectedRevision: context.revision, label: value('routing-context-label'), enabled: checked('routing-context-enabled') } }; }, 'routing-config-status');
        if (result) {
            feedback('routing-config-status', 'Context declaration saved. Existing admitted targets are unchanged.', 'success');
            await this.refresh();
        }
    }
    async saveProfile() {
        const result = await this.mutate('profile', () => {
            const profile = this.profiles.find(item => item.id === value('routing-profile-edit'));
            if (!profile)
                throw new ApiError('invalid_input', 'Refresh the profile first.');
            let price;
            if (value('routing-reference-price')) {
                try {
                    price = JSON.parse(value('routing-reference-price'));
                }
                catch {
                    throw new ApiError('invalid_input', 'Reference price must be a valid object with the documented string lexemes.');
                }
            }
            return { path: '/routing/profiles/' + profile.id, method: 'PUT', body: { requestId: requestId(), expectedRevision: profile.revision, enabled: checked('routing-profile-enabled'), ...(price !== undefined ? { referencePrice: price } : {}), ...(checked('routing-clear-price') ? { clearReferencePrice: true } : {}) } };
        }, 'routing-config-status');
        if (result) {
            feedback('routing-config-status', 'Profile preference/reference saved. This is not a verified price, model access or measured charge.', 'success');
            await this.refresh();
        }
    }
    async savePolicy() {
        const result = await this.mutate('policy', () => {
            if (!this.policy)
                throw new ApiError('invalid_input', 'Refresh the saved routing policy first.');
            const automatic = value('routing-default-automatic');
            if (!/^(0|[1-9][0-9]*)$/.test(automatic) || BigInt(automatic) > 15n)
                throw new ApiError('invalid_input', 'Automatic admissions must be from 0 through 15.');
            return { path: '/routing/policy', method: 'PUT', body: { requestId: requestId(), expectedRevision: this.policy.revision, unknownAvailability: value('routing-unknown-availability'), economyOrder: value('routing-preference-order').split(/\r?\n/).filter(Boolean), defaultBudget: { admissionLimit: exactUnits('routing-default-admissions'), windowSeconds: seconds('routing-default-window') }, continuation: { maxAutomaticAdmissions: automatic, categories: automatic === '0' ? [] : ['pre_launch_ineligible'], allowReplacement: checked('routing-default-replacement'), allowEscalation: checked('routing-default-escalation') } } };
        }, 'routing-config-status');
        if (result) {
            feedback('routing-config-status', 'Saved routing defaults changed for future capture. Existing task authority, budget and deadlines are unchanged.', 'success');
            await this.refresh();
        }
    }
    async loadAccounts() {
        if (!this.accountsCursor)
            return;
        const generation = this.generation;
        try {
            const { data } = await api('/routing/accounts?cursor=' + encodeURIComponent(this.accountsCursor));
            if (generation !== this.generation)
                return;
            this.accounts.push(...data.items);
            this.accountsCursor = data.nextCursor;
            this.renderConfiguration();
        }
        catch (error) {
            feedback('routing-config-status', errorMessage(error), 'error');
        }
    }
    async loadReports(append) {
        const subject = this.subject();
        const generation = this.capacityGeneration;
        if (append && !this.reportCursor)
            return;
        try {
            const { data } = await api('/routing/reports?' + subjectQuery(subject) + (append ? '&cursor=' + encodeURIComponent(this.reportCursor) : ''));
            if (generation !== this.capacityGeneration || subjectKey(subject) !== value('routing-subject'))
                return;
            const list = element('routing-reports-list');
            if (!append)
                list.replaceChildren();
            for (const report of data.items)
                evidence(list, report.id + ' · ' + report.dimension + ' ' + report.state + ' · ' + report.source + ' · observed ' + timeLabel(report.observedAt), report);
            this.reportCursor = data.nextCursor;
            element('more-routing-reports').hidden = !data.nextCursor;
        }
        catch (error) {
            if (generation === this.capacityGeneration)
                feedback('routing-report-status', errorMessage(error), 'error');
        }
    }
    requiresAdoption(taskId) { return this.detail?.task.id === taskId && this.taskRouting?.adoptionRequired === true; }
    clearTask() { this.budgetEditorTaskId = null; this.detail = null; this.taskRouting = null; this.taskBudget = null; this.taskGeneration++; this.historyGeneration++; this.cursors = {}; for (const id of ['task-route-facts', 'task-budget-facts', 'task-decisions-list', 'task-ledger-list', 'task-usage-list', 'task-usage-summary'])
        element(id).replaceChildren(); this.controls(); }
    async setTask(detail) {
        if (!this.ready)
            return;
        const changed = this.detail?.task.id !== detail.task.id || this.detail.task.currentVersion !== detail.task.currentVersion;
        if (changed)
            this.clearTask();
        this.detail = detail;
        if (changed) {
            const saved = this.guidanceEditors[detail.task.id] ?? {};
            for (const node of document.querySelectorAll('[data-routing-guidance]'))
                this.restore(node.id, saved[node.id] ?? (node.id === 'guidance-current-routing' ? false : node.id === 'guidance-model' ? '' : 'keep'));
        }
        const generation = ++this.taskGeneration;
        this.controls();
        try {
            const [routing, budget] = await Promise.all([api('/tasks/' + detail.task.id + '/routing'), api('/tasks/' + detail.task.id + '/budget')]);
            if (generation !== this.taskGeneration || this.detail?.task.id !== detail.task.id)
                return;
            if (routing.data.authority && routing.data.authority.taskVersion !== detail.task.currentVersion)
                throw new ApiError('revision_conflict', 'The task version changed during this routing read. Refresh detail; local input is retained.');
            if (routing.data.authority && routing.data.authority.taskId !== detail.task.id || routing.data.latestDecision && routing.data.latestDecision.taskId !== detail.task.id || budget.data.budget && budget.data.budget.taskId !== detail.task.id)
                throw new ApiError('invalid_response', 'The routing/budget observation does not identify this selected task.');
            this.taskRouting = routing.data;
            this.taskBudget = budget.data;
            this.renderTask();
            this.onTaskObserved();
        }
        catch (error) {
            if (generation === this.taskGeneration)
                feedback('task-routing-status', errorMessage(error) + ' Routing/budget observations are unavailable; prior snapshots are not current authority.', 'error');
        }
        finally {
            if (generation === this.taskGeneration)
                this.controls();
        }
    }
    renderTask() {
        const routing = this.taskRouting;
        const budget = this.taskBudget?.budget;
        const facts = element('task-route-facts');
        facts.replaceChildren();
        addFact(facts, 'Task identity', this.detail.task.id);
        if (routing?.latestDecision) {
            const decision = routing.latestDecision;
            addFact(facts, 'Admitted target', targetLabel(decision));
            addFact(facts, 'Immutable route decision', decision.id + ' · attempt ' + decision.attemptId + ' · task version ' + decision.taskVersion + ' · ' + decision.kind);
            addFact(facts, 'Declared account association', decision.selectedTarget.declaredAccountAssociation ? decision.selectedTarget.declaredAccountAssociation.label + ' · actual login unknown' : 'None · actual login unknown');
            addFact(facts, 'Captured uncertainty', decision.uncertainties.join('; ') || 'Native access, actual model, allowance and charges remain unknown unless separately observed.');
        }
        else
            addFact(facts, 'Routing authority', routing?.adoptionRequired ? 'Legacy task · explicit adoption required for NEW execution. Read, Stop, acceptance and exact old replay remain available.' : 'No admitted routing decision');
        if (routing?.authority) {
            const authority = routing.authority;
            addFact(facts, 'Captured suitability', authority.suitabilityAssessment.classification + ' · ' + authority.suitabilityAssessment.source);
            addFact(facts, 'Captured routing policy', authority.policySnapshot.revision + ' · ' + timeLabel(authority.capturedAt));
            addFact(facts, 'Automatic continuation', (authority.optionSnapshot.continuation?.maxAutomaticAdmissions ?? authority.policySnapshot.continuation.maxAutomaticAdmissions) === '0' ? 'OFF' : JSON.stringify(authority.optionSnapshot.continuation ?? authority.policySnapshot.continuation));
        }
        const bf = element('task-budget-facts');
        bf.replaceChildren();
        if (!budget)
            addFact(bf, 'Budget', 'Not adopted · no new execution authority; this is not an exhausted zero.');
        else {
            for (const [title, key] of [['Granted admissions', 'grantedAdmissions'], ['Consumed admissions', 'consumedAdmissions'], ['Remaining admissions', 'remainingAdmissions'], ['Automatic admissions', 'automaticAdmissions'], ['Historical admissions', 'historicalAdmissions']])
                addFact(bf, title, budget[key]);
            addFact(bf, 'Budget revision', budget.revision);
            addFact(bf, 'Common task window', budget.currentWindow.windowSeconds + ' wall-clock seconds · number ' + budget.currentWindow.number);
            addFact(bf, 'First admission', budget.currentWindow.firstAdmissionAt ? timeLabel(budget.currentWindow.firstAdmissionAt) : 'Not started');
            addFact(bf, 'Common deadline', budget.currentWindow.deadlineAt ? timeLabel(budget.currentWindow.deadlineAt) : 'Not started');
        }
        if (this.budgetEditorTaskId !== this.detail.task.id) {
            this.restore('budget-additional-admissions', this.editors[this.editorKey('budget-additional-admissions')] ?? (routing?.adoptionRequired ? '2' : '1'));
            this.restore('budget-new-window', this.editors[this.editorKey('budget-new-window')] ?? (routing?.adoptionRequired ? '1800' : ''));
            this.budgetEditorTaskId = this.detail.task.id;
        }
        const pendingAdopt = !!this.pending['adopt:' + this.detail.task.id];
        element('task-adoption-controls').hidden = !routing?.adoptionRequired && !pendingAdopt && !this.adoptStart;
        element('adopt-task-budget').hidden = !routing?.adoptionRequired && !pendingAdopt;
        element('adopt-start-task').hidden = !routing?.adoptionRequired && !this.adoptStart;
        element('task-grant-controls').hidden = !!routing?.adoptionRequired;
    }
    amendment(action) {
        if (!this.detail || !this.taskBudget)
            throw new ApiError('invalid_input', 'Wait for current task/budget revisions.');
        const body = { requestId: requestId(), action, expectedRevision: this.detail.task.revision, additionalAdmissions: exactUnits('budget-additional-admissions') };
        const window = seconds('budget-new-window', true);
        if (action === 'grant') {
            if (!this.taskBudget.budget)
                throw new ApiError('routing_adoption_required', 'Adopt this legacy task explicitly first.');
            body['expectedBudgetRevision'] = this.taskBudget.budget.revision;
        }
        else if (!window)
            throw new ApiError('invalid_input', 'Legacy adoption requires an explicit finite new window.');
        if (window)
            body['newWindowSeconds'] = window;
        return { path: '/tasks/' + this.detail.task.id + '/budget-amendments', method: 'POST', body };
    }
    async amend(action) {
        const id = this.detail?.task.id;
        if (!id)
            return;
        const result = await this.mutate(action + ':' + id, () => this.amendment(action), 'task-routing-status');
        if (result) {
            feedback('task-routing-status', 'Explicit ' + action + ' recorded. No worker was started and no old continuation proof was re-armed.', 'success');
            await this.onTaskChanged(id);
        }
    }
    async adoptAndStart() {
        const id = this.detail?.task.id;
        if (!id || this.busy.has('adoptStart'))
            return;
        try {
            if (!this.adoptStart)
                this.adoptStart = { taskId: id, amendment: this.amendment('adopt'), start: { requestId: requestId(), ...(checked('task-fake') ? { executionMode: 'fake', fakeScenario: value('task-fake-scenario') } : {}) } };
        }
        catch (error) {
            feedback('task-routing-status', errorMessage(error), 'error');
            return;
        }
        const pending = this.adoptStart;
        const wasUnresolved = pending.amendment.unresolved === true;
        pending.amendment.unresolved = true;
        this.busy.add('adoptStart');
        writeSession('routingAdoptStart', pending);
        this.controls();
        try {
            if (!pending.adoptionAcknowledged) {
                const { data } = await api(pending.amendment.path, pending.amendment.method, pending.amendment.body);
                pending.start['expectedRevision'] = data.task.revision;
                pending.start['taskVersion'] = data.task.currentVersion;
                pending.adoptionAcknowledged = true;
                writeSession('routingAdoptStart', pending);
            }
            await api('/tasks/' + pending.taskId + '/attempts', 'POST', pending.start);
            this.adoptStart = null;
            writeSession('routingAdoptStart', null);
            feedback('task-routing-status', 'Adoption acknowledged, then the separately captured explicit Start acknowledged. Each has its own retained request identity.', 'success');
            await this.onTaskChanged(pending.taskId);
        }
        catch (error) {
            const uncertain = wasUnresolved || error instanceof ApiError && error.uncertain;
            if (!uncertain) {
                this.adoptStart = null;
                writeSession('routingAdoptStart', null);
            }
            feedback('task-routing-status', errorMessage(error) + (uncertain ? ' Retry this same two-phase intent to reconcile its captured task and identities.' : pending.adoptionAcknowledged ? ' Adoption remains acknowledged; no replacement Start is automatic. Inspect current authority before an explicit new Start.' : ' Adoption was not acknowledged. Correct the retained input before a new request.') + ' Newer input/selection is retained.', 'error');
        }
        finally {
            this.busy.delete('adoptStart');
            this.controls();
        }
    }
    async loadHistory(kind, append) {
        const detail = this.detail;
        if (!detail || append && !this.cursors[kind])
            return;
        const key = 'history:' + detail.task.id + ':' + kind;
        if (this.busy.has(key))
            return;
        this.busy.add(key);
        this.controls();
        const generation = this.taskGeneration;
        const historyGeneration = this.historyGeneration;
        const route = kind === 'decisions' ? 'routing/decisions' : kind === 'ledger' ? 'budget/ledger' : 'usage';
        try {
            const { data } = await api('/tasks/' + detail.task.id + '/' + route + (append ? '?cursor=' + encodeURIComponent(this.cursors[kind]) : ''));
            if (generation !== this.taskGeneration || historyGeneration !== this.historyGeneration || this.detail?.task.id !== detail.task.id)
                return;
            this.cursors[kind] = data.nextCursor;
            element('more-task-' + kind).hidden = !data.nextCursor;
            const list = element('task-' + kind + '-list');
            if (!append)
                list.replaceChildren();
            if (kind === 'usage') {
                if (data.taskId !== detail.task.id)
                    throw new ApiError('invalid_response', 'This usage page does not identify the selected task.');
                this.renderUsage(data, list);
                return;
            }
            if (data.items.some(item => item.taskId !== detail.task.id))
                throw new ApiError('invalid_response', 'This history page contains a different task.');
            for (const item of data.items)
                evidence(list, item.id + ('selectedTarget' in item ? ' · admitted ' + targetLabel(item) : ' · ' + item.kind + ' · granted ' + item.grantedDelta + ' / consumed ' + item.consumedDelta), item);
        }
        catch (error) {
            if (generation === this.taskGeneration)
                feedback('task-routing-status', errorMessage(error) + ' Read failed without changing task authority.', 'error');
        }
        finally {
            this.busy.delete(key);
            this.controls();
        }
    }
    renderUsage(page, list) {
        const facts = element('task-usage-summary');
        facts.replaceChildren();
        addFact(facts, 'Coverage for returned attempts', page.coverage.status + ' · included ' + page.coverage.includedAttemptIds.join(', ') + ' · unknown ' + (page.coverage.unknownAttemptIds.join(', ') || 'none in this page') + ' · fake unmeasured ' + (page.coverage.excludedFakeAttemptIds.join(', ') || 'none in this page'));
        addFact(facts, 'Charges', 'Unknown · no native charge receipts. Empty receipts are not zero cost.');
        for (const scope of ['top_level', 'whole_tree'])
            for (const [counter, subtotal] of Object.entries(page.knownSubtotal.byUsageScope[scope]))
                addFact(facts, scope + ' ' + counter + ' · returned attempts only', (subtotal.knownValue ?? 'Unknown') + ' · contributors ' + (subtotal.contributingAttemptIds.join(', ') || 'none') + ' · missing ' + (subtotal.missingAttemptIds.join(', ') || 'none'));
        for (const item of page.items)
            evidence(list, item.attemptId + ' · ' + item.outcome + ' · ' + item.selectedUsageScope + ' · ' + usageLabel(item.usage), item);
        for (const estimate of page.estimates)
            evidence(list, 'Native estimate, not a charge · ' + estimate.value + ' ' + estimate.currency + ' · ' + estimate.model + ' · ' + estimate.scope, estimate);
        for (const price of page.referencePrices)
            evidence(list, 'Captured user reference price · decision ' + price.decisionId + ' · not a charge or measured saving', price);
    }
    controls() {
        element('refresh-routing').disabled = !this.ready || this.busy.has('refresh');
        for (const kind of ['decisions', 'ledger', 'usage'])
            for (const prefix of ['load', 'more'])
                element(prefix + '-task-' + kind).disabled = !this.ready || !this.detail || this.busy.has('history:' + this.detail.task.id + ':' + kind);
        for (const [id, key, title] of [['save-routing-report', 'report', 'Record user report'], ['save-routing-account', 'account', 'Save declaration'], ['save-routing-link', 'link', 'Save association'], ['save-routing-context', 'context', 'Save context'], ['save-routing-profile', 'profile', 'Save profile preference'], ['save-routing-policy', 'policy', 'Save routing defaults']]) {
            const button = element(id);
            button.disabled = !this.ready || this.busy.has(key) || key === 'report' && !this.capacity && !this.pending[key];
            button.textContent = this.pending[key] ? 'Retry pending ' + title.toLowerCase() : title;
        }
        for (const [id, action] of [['grant-task-budget', 'grant'], ['adopt-task-budget', 'adopt']]) {
            const key = action + ':' + this.detail?.task.id;
            const button = element(id);
            button.disabled = !this.ready || !this.detail || !this.taskBudget || this.busy.has(key);
            button.textContent = this.pending[key] ? 'Retry pending ' + action : action === 'grant' ? 'Grant additional authority' : 'Adopt task budget';
        }
        element('adopt-start-task').disabled = !this.ready || !this.detail || this.busy.has('adoptStart') || !this.taskBudget;
        element('adopt-start-task').textContent = this.adoptStart ? 'Retry pending adoption and Start · ' + this.adoptStart.taskId.slice(0, 8) : 'Adopt and start this captured version';
        element('routing-pending-summary').textContent = Object.values(this.pending).map(pending => 'Pending exact ' + pending.method + ' ' + pending.path + ' · request ' + pending.body['requestId']).join('\n') + (this.adoptStart ? '\nPending adoption/Start for task ' + this.adoptStart.taskId + ' · adoption ' + this.adoptStart.amendment.body['requestId'] + ' · Start ' + this.adoptStart.start['requestId'] + ' · adoption acknowledged ' + !!this.adoptStart.adoptionAcknowledged : '');
    }
}
