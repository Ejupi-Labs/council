import { addFact, api, ApiError, element, errorMessage, feedback, readSession, requestId, writeSession } from './api.js';
const resources = ['steps', 'reviews', 'checks', 'findings', 'deliveries', 'candidates'];
const composerIds = ['task-workflow', 'task-delivery', 'task-review', 'task-max-corrections', 'task-checks', 'task-delivery-declaration'];
const value = (id) => element(id).value;
const modeName = (mode) => mode === 'apply_verified_local' ? 'Apply verified local changes' : 'Review only';
const continuesObservedWork = (workflow) => !!workflow && (['queued', 'running', 'cancelling'].includes(workflow.state) || workflow.state === 'blocked' && workflow.nextOwner === 'council');
const object = (input) => input && typeof input === 'object' && !Array.isArray(input) ? input : {};
const text = (input) => input === null || input === undefined ? 'Not recorded' : String(input);
// JSON.parse alone would silently erase duplicate members before the strict owner sees them.
export function declarationJSON(raw, array) {
    if (new TextEncoder().encode(raw).byteLength > 65536)
        throw new ApiError('input_too_large', 'Reduce this declaration to 64 KiB or less.');
    let parsed;
    try {
        parsed = JSON.parse(raw);
        const tokens = [...raw.matchAll(/"(?:\\.|[^"\\])*"|[{}\[\],:]/g)].map(match => match[0]);
        const stack = [];
        for (let i = 0; i < tokens.length; i++) {
            const token = tokens[i];
            if (token === '{')
                stack.push(new Set());
            else if (token === '[')
                stack.push(null);
            else if (token === '}' || token === ']')
                stack.pop();
            else if (token.startsWith('"') && tokens[i + 1] === ':') {
                const key = JSON.parse(token);
                const keys = stack[stack.length - 1];
                if (!keys || keys.has(key))
                    throw new Error('duplicate');
                keys.add(key);
            }
        }
    }
    catch {
        throw new ApiError('invalid_input', 'Supply valid JSON with unique object members. Your declaration is retained.');
    }
    if (array ? !Array.isArray(parsed) : !parsed || typeof parsed !== 'object' || Array.isArray(parsed))
        throw new ApiError('invalid_input', array ? 'Supply a JSON array of explicit check definitions.' : 'Supply a non-null delivery declaration object.');
    return parsed;
}
export class WorkflowView {
    onTaskChanged;
    onReadArtifact;
    onSelectAttempt;
    onObserved;
    ready = false;
    task = null;
    current = null;
    selected = null;
    selector = '';
    generation = 0;
    preferenceGeneration = 0;
    preferenceEdit = 0;
    preference = null;
    preferenceEdited = false;
    pending = readSession('workflowPending') ?? {};
    busy = new Set();
    cursors = {};
    histories = {};
    historyGenerations = {};
    candidateId = null;
    composer = readSession('workflowComposer') ?? {};
    constructor() {
        for (const pending of Object.values(this.pending))
            pending.unresolved = true;
        for (const id of composerIds) {
            if (this.composer[id] !== undefined)
                element(id).value = this.composer[id];
            element(id).addEventListener('input', () => { this.composer[id] = value(id); writeSession('workflowComposer', this.composer); });
            element(id).addEventListener('change', () => { this.composer[id] = value(id); writeSession('workflowComposer', this.composer); });
        }
        const editor = readSession('deliveryPreferenceEditor');
        if (editor !== null) {
            element('delivery-preference').value = editor;
            this.preferenceEdited = true;
        }
        element('delivery-preference').addEventListener('change', () => {
            this.preferenceEdit++;
            this.preferenceEdited = true;
            writeSession('deliveryPreferenceEditor', value('delivery-preference'));
            feedback('delivery-preference-status', 'Local delivery choice changed. Save it once to authorize this scope for future tasks. Existing tasks keep their captured scope.');
        });
        element('save-delivery-preference').addEventListener('click', () => { void this.savePreference(); });
        element('reload-delivery-preference').addEventListener('click', () => { this.preferenceEdited = false; writeSession('deliveryPreferenceEditor', null); void this.refreshPreference(); });
        element('adopt-task-workflow').addEventListener('click', () => { void this.mutateTask('adopt'); });
        element('advance-task-workflow').addEventListener('click', () => { void this.mutateTask('advance'); });
        element('recheck-task-workflow').addEventListener('click', () => { void this.mutateTask('check'); });
        element('workflow-check-definition').addEventListener('change', () => this.controls());
        element('load-workflow-version').addEventListener('click', () => { void this.selectHistory(); });
        element('return-current-workflow').addEventListener('click', () => { this.selector = ''; void this.refreshSelected(); });
        for (const kind of resources) {
            element('load-workflow-' + kind).addEventListener('click', () => { void this.loadHistory(kind); });
            element('more-workflow-' + kind).addEventListener('click', () => { void this.loadHistory(kind, true); });
        }
        element('more-workflow-changes').addEventListener('click', () => { if (this.candidateId)
            void this.loadChanges(this.candidateId, true); });
        this.controls();
    }
    setReady(ready) { this.ready = ready; if (!ready)
        this.generation++; this.controls(); }
    get workflow() { return this.current?.workflow ?? null; }
    get responseBinding() { return this.workflow?.selectedResponseBinding; }
    get authorStartReady() {
        const workflow = this.workflow;
        return !!workflow && workflow.taskVersion === this.task?.task.currentVersion && workflow.nextAction === 'start_author' && workflow.nextOwner === 'user' && !workflow.currentStepId && !workflow.selectedCandidateId && !workflow.selectedResponseBinding;
    }
    get responseAcceptanceReady() { return !!this.workflow && this.workflow.nextAction === 'response_acceptance_pending' && this.workflow.nextOwner === 'user'; }
    async producerAttemptId() {
        const workflow = this.workflow;
        if (!workflow)
            return null;
        if (workflow.selectedResponseBinding)
            return workflow.selectedResponseBinding.producerAttemptId;
        const generation = this.generation;
        let cursor = null;
        let latestProducer = null;
        do {
            const { data } = await api('/tasks/' + workflow.taskId + '/workflow/steps?workflowId=' + workflow.id + '&limit=100' + (cursor ? '&cursor=' + encodeURIComponent(cursor) : ''));
            if (generation !== this.generation || workflow.id !== this.workflow?.id)
                return null;
            for (const step of data.items)
                if (['author', 'correction'].includes(step.role) && step.attemptId) {
                    latestProducer = step.attemptId;
                    if (workflow.selectedCandidateId && step.outputCandidateId === workflow.selectedCandidateId)
                        return step.attemptId;
                }
            cursor = data.nextCursor;
        } while (cursor);
        return latestProducer;
    }
    composerOptions() {
        const mode = value('task-workflow');
        const delivery = value('task-delivery');
        const review = value('task-review');
        const cycles = value('task-max-corrections');
        const checks = value('task-checks');
        const declaration = value('task-delivery-declaration');
        if (mode === 'author_only') {
            if (delivery || review || cycles || checks.trim() || declaration.trim())
                throw new ApiError('invalid_input', 'Author only cannot also declare automatic review, checks, correction or delivery. Clear those optional declarations or choose Auto.');
            return { mode: 'author_only' };
        }
        const options = {};
        if (delivery && declaration.trim())
            throw new ApiError('invalid_input', 'Choose the local delivery override or the advanced delivery declaration, then clear the other.');
        if (delivery)
            options.delivery = { localMode: delivery };
        if (declaration.trim())
            options.delivery = declarationJSON(declaration, false);
        if (review)
            options.review = review;
        if (cycles) {
            if (!/^[01]$/.test(cycles))
                throw new ApiError('invalid_input', 'Allow zero or one correction cycle.');
            options.maxCorrectionCycles = Number(cycles);
        }
        if (checks.trim()) {
            const definitions = declarationJSON(checks, true);
            if (definitions.length > 16)
                throw new ApiError('invalid_input', 'Declare at most sixteen checks.');
            options.checks = definitions;
        }
        return Object.keys(options).length ? options : undefined;
    }
    async refreshPreference() {
        const generation = ++this.preferenceGeneration;
        const edit = this.preferenceEdit;
        try {
            const { data } = await api('/workflow/preferences');
            if (generation !== this.preferenceGeneration)
                return;
            if (!this.preference || BigInt(data.revision) >= BigInt(this.preference.revision))
                this.preference = data;
            if (!this.preferenceEdited && edit === this.preferenceEdit)
                element('delivery-preference').value = this.preference.localMode;
            feedback('delivery-preference-status', 'Saved for future tasks: ' + modeName(this.preference.localMode) + '. Revision ' + this.preference.revision + '. Existing task scope is unchanged.');
            this.controls();
        }
        catch (error) {
            if (generation === this.preferenceGeneration)
                feedback('delivery-preference-status', errorMessage(error), 'error');
        }
    }
    async savePreference() {
        if (!this.ready || !this.preference)
            return;
        const key = 'preference';
        if (!this.pending[key])
            this.pending[key] = { path: '/workflow/preferences', method: 'PATCH', body: { requestId: requestId(), expectedRevision: this.preference.revision, localMode: value('delivery-preference') }, editGeneration: this.preferenceEdit };
        const pending = this.pending[key];
        const data = await this.perform(key, pending, 'delivery-preference-status');
        if (!data)
            return;
        if (!this.preference || BigInt(data.revision) >= BigInt(this.preference.revision))
            this.preference = data;
        if (this.preferenceEdit === pending.editGeneration) {
            this.preferenceEdited = false;
            element('delivery-preference').value = this.preference.localMode;
            writeSession('deliveryPreferenceEditor', null);
        }
        feedback('delivery-preference-status', 'Saved prospectively for future tasks: ' + modeName(data.localMode) + '. Existing tasks keep their captured scope.' + (this.preferenceEdited ? ' Your newer local choice is retained and has not been saved.' : ''), 'success');
        this.controls();
    }
    clearTask() {
        this.generation++;
        this.task = null;
        this.current = null;
        this.selected = null;
        this.selector = '';
        this.histories = {};
        this.cursors = {};
        this.candidateId = null;
        element('workflow-panel').hidden = true;
        for (const kind of resources) {
            element('workflow-' + kind + '-list').replaceChildren();
            element('more-workflow-' + kind).hidden = true;
        }
        element('workflow-changes-list').replaceChildren();
        element('more-workflow-changes').hidden = true;
        element('workflow-acceptance-record').replaceChildren();
        element('workflow-check-definition').replaceChildren(new Option('Choose a captured required check', ''));
        this.controls();
    }
    async setTask(detail) {
        if (this.task?.task.id !== detail.task.id || this.task.task.currentVersion !== detail.task.currentVersion)
            this.clearTask();
        this.task = detail;
        await this.refreshSelected();
    }
    async refreshSelected() {
        const task = this.task;
        if (!task || !this.ready)
            return;
        const generation = ++this.generation;
        const selector = this.selector;
        try {
            const { data: current } = await api('/tasks/' + task.task.id + '/workflow');
            if (generation !== this.generation || task.task.id !== this.task?.task.id)
                return;
            this.current = current;
            const selected = selector ? (await api('/tasks/' + task.task.id + '/workflow?' + selector)).data : current;
            if (generation !== this.generation)
                return;
            const changedSelection = this.selected?.taskVersion !== selected.taskVersion || this.selected?.workflow?.id !== selected.workflow?.id;
            this.selected = selected;
            if (changedSelection) {
                this.histories = {};
                this.cursors = {};
                for (const kind of resources) {
                    element('workflow-' + kind + '-list').replaceChildren();
                    element('more-workflow-' + kind).hidden = true;
                }
                element('workflow-changes-list').replaceChildren();
                element('more-workflow-changes').hidden = true;
                this.candidateId = null;
            }
            this.render();
            this.onObserved?.();
            if (!selector && continuesObservedWork(current.workflow))
                void this.observe(generation);
        }
        catch (error) {
            if (generation === this.generation)
                feedback('workflow-status', errorMessage(error) + ' Retained work remains under its continuing owner.', 'error');
        }
    }
    async observe(generation) {
        while (this.ready && generation === this.generation && this.task && !this.selector) {
            await new Promise(resolve => setTimeout(resolve, 500));
            if (generation !== this.generation || !this.task)
                return;
            try {
                const id = this.task.task.id;
                const { data } = await api('/tasks/' + id + '/workflow');
                if (generation !== this.generation || id !== this.task?.task.id)
                    return;
                const changed = data.workflow?.stateRevision !== this.current?.workflow?.stateRevision || data.workflow?.currentStepId !== this.current?.workflow?.currentStepId;
                this.current = data;
                this.selected = data;
                this.render();
                this.onObserved?.();
                if (changed && this.onTaskChanged) {
                    await this.onTaskChanged(id);
                    return;
                }
                if (!continuesObservedWork(data.workflow))
                    return;
            }
            catch (error) {
                if (generation === this.generation)
                    feedback('workflow-status', errorMessage(error) + ' Observation stopped; it did not cancel or replay work.', 'error');
                return;
            }
        }
    }
    async selectHistory() {
        const version = value('workflow-history-version');
        const id = value('workflow-history-id');
        if (version && id || version && !/^[1-9]\d*$/.test(version) || id && !/^[a-f0-9]{32}$/.test(id)) {
            feedback('workflow-status', 'Choose one exact retained task version or workflow identity.', 'error');
            return;
        }
        this.selector = id ? 'workflowId=' + id : version ? 'taskVersion=' + version : '';
        await this.refreshSelected();
    }
    isHistorical() { return !!this.selected && this.selected.taskVersion !== this.task?.task.currentVersion; }
    selectedQuery() { return this.selected?.workflow ? 'workflowId=' + this.selected.workflow.id : 'taskVersion=' + this.selected?.taskVersion; }
    render() {
        if (!this.selected)
            return;
        element('workflow-panel').hidden = false;
        const workflow = this.selected.workflow;
        const historical = this.isHistorical();
        const facts = element('workflow-facts');
        facts.replaceChildren();
        const qualification = element('workflow-acceptance-record');
        qualification.replaceChildren();
        const retainedAcceptance = this.selected.acceptance ?? workflow?.completion ?? (workflow ? { workflowId: workflow.id, evidenceUses: workflow.evidenceUses } : undefined);
        if (retainedAcceptance) {
            const details = document.createElement('details');
            const heading = document.createElement('summary');
            heading.textContent = 'Server-recorded acceptance and complete evidence uses';
            const pre = document.createElement('pre');
            pre.className = 'evidence-text';
            pre.tabIndex = 0;
            pre.textContent = JSON.stringify(retainedAcceptance, null, 2);
            details.append(heading, pre);
            qualification.append(details);
            this.artifactButtons(qualification, retainedAcceptance);
        }
        addFact(facts, 'Selected task version', this.selected.taskVersion + (historical ? ' · Historical; read only' : ' · Current'));
        if (!workflow) {
            feedback('workflow-status', (historical ? 'Historical standalone task. ' : 'Standalone task. ') + 'No verified workflow authority was created for this version. Explicit adoption creates a new version and starts no work.');
            this.controls();
            return;
        }
        const authority = workflow.authority;
        addFact(facts, 'Workflow identity', workflow.id);
        addFact(facts, 'State', workflow.state);
        addFact(facts, 'Execution mode', authority.executionMode === 'fake' ? 'LOCAL FAKE · provider/model/usage facts remain unmeasured' : 'Native · inspect actual stage observations');
        for (const gap of authority.checkGaps ?? [])
            addFact(facts, 'Captured check gap', gap.code + ': ' + gap.message);
        addFact(facts, 'Next action and owner', workflow.nextAction + ' · ' + workflow.nextOwner);
        addFact(facts, 'Captured local delivery', modeName(authority.delivery.localMode));
        addFact(facts, 'Authorized local destination', authority.delivery.destination + ' · branch ' + (authority.delivery.branch ?? 'detached HEAD'));
        addFact(facts, 'Correction authority', workflow.correctionCyclesUsed + ' used / ' + authority.correction.maxCycles + ' captured cycles');
        addFact(facts, 'Selected candidate', workflow.selectedCandidateId ?? 'No candidate selected');
        addFact(facts, 'Complete task change scope', (workflow.hasTaskDelta ? 'Root-to-selected file changes retained' : 'No root-to-selected file changes') + (workflow.hasTaskIndexDelta ? ' · task index delta present' : ' · no task index delta'));
        addFact(facts, 'Current server-recorded evidence uses', String(workflow.evidenceUses.length) + ' · inspect the complete retained bindings below');
        if (workflow.selectedResponseBinding)
            addFact(facts, 'Deliverable response producer', workflow.selectedResponseBinding.producerAttemptId + ' · artifact ' + workflow.selectedResponseBinding.responseArtifactId);
        if (authority.reasonCodes.length)
            addFact(facts, 'Qualification reasons', authority.reasonCodes.join(' · '));
        const acceptance = this.selected.acceptance;
        if (acceptance) {
            addFact(facts, 'Server-recorded qualification', acceptance.deliveryResult + ' · ' + acceptance.acceptedAt + ' · acceptance ' + acceptance.id);
            addFact(facts, 'Recorded evidence use', acceptance.reviewUses.length + ' reviews · ' + acceptance.checkUses.length + ' checks · ' + acceptance.findingResolutionIds.length + ' finding resolutions. These immutable uses do not attest a fresh external filesystem inspection.');
        }
        addFact(facts, 'Native-model accounting', 'Only native agent stages consume model admissions. Local check/application stages do not supply native token or charge measurements.');
        if (authority.delivery.externalAction)
            addFact(facts, 'Explicit external action', authority.delivery.externalAction.action + ' · ' + authority.delivery.externalAction.destination + ' · command completion only; remote publication is unverified');
        const completion = workflow.completion;
        const result = completion ? 'User accepted the selected response' : authority.delivery.localMode === 'apply_verified_local' ? 'Applied verified local change' : 'Reviewable verified change; applying remains an explicit remaining action';
        feedback('workflow-status', (historical ? 'Historical · ' : '') + (workflow.state === 'completed' ? 'Completed · ' + result : !historical && workflow.nextAction === 'start_author' && !workflow.currentStepId ? 'Ready · Use Run task to start the first author under this captured scope. Adoption started no work.' : workflow.nextAction === 'response_acceptance_pending' ? 'Response ready · Review the full selected response and accept explicitly.' : workflow.state + ' · ' + workflow.nextAction + ' · next owner: ' + workflow.nextOwner), ['failed', 'interrupted', 'blocked'].includes(workflow.state) && !['response_acceptance_pending', 'start_author'].includes(workflow.nextAction) ? 'error' : workflow.state === 'completed' ? 'success' : 'neutral');
        const definitions = element('workflow-check-definition');
        const prior = definitions.selectedIndex >= 0 ? definitions.value : '';
        definitions.replaceChildren(new Option('Choose a captured required check', ''), ...authority.checks.filter(check => check.required).map(check => new Option(check.key + ' · ' + check.kind, check.id)));
        if ([...definitions.options].some(option => option.value === prior))
            definitions.value = prior;
        this.controls();
    }
    controls() {
        element('save-delivery-preference').disabled = !this.ready || !this.preference || this.busy.has('preference');
        element('save-delivery-preference').textContent = this.pending['preference'] ? 'Retry pending delivery save' : 'Save future delivery scope';
        element('delivery-preference').disabled = !this.ready;
        const task = this.task?.task;
        const current = this.selected && !this.isHistorical();
        const workflow = this.selected?.workflow;
        const active = !!this.task?.latestAttempt && !this.task.latestAttempt.settledAt;
        for (const action of ['adopt', 'advance', 'check']) {
            const key = action + ':' + task?.id;
            const id = action === 'adopt' ? 'adopt-task-workflow' : action === 'advance' ? 'advance-task-workflow' : 'recheck-task-workflow';
            const button = element(id);
            button.disabled = !this.ready || !task || !current || this.busy.has(key) || !this.pending[key] && (active || (action === 'adopt' ? !!workflow : !workflow || !workflow.currentStepId || workflow.state === 'completed' || action === 'check' && !value('workflow-check-definition')));
            button.textContent = this.pending[key] ? 'Retry pending ' + action : action === 'adopt' ? 'Adopt verified workflow' : action === 'advance' ? 'Advance current next step' : 'Recheck captured definition';
            button.hidden = action === 'adopt' ? !!workflow : !workflow;
        }
        for (const kind of resources)
            element('load-workflow-' + kind).disabled = !this.ready || !this.selected?.workflow;
        element('workflow-pending-summary').textContent = Object.values(this.pending).map(pending => 'Pending ' + pending.method + ' ' + pending.path + ' · request ' + pending.body['requestId'] + ' · captured task version ' + (pending.body['taskVersion'] ?? 'future preference')).join('\n');
    }
    async perform(key, pending, status) {
        if (!this.ready || this.busy.has(key))
            return null;
        const wasUnresolved = pending.unresolved === true;
        pending.unresolved = true;
        this.pending[key] = pending;
        this.busy.add(key);
        writeSession('workflowPending', this.pending);
        this.controls();
        try {
            const { data } = await api(pending.path, pending.method, pending.body);
            delete this.pending[key];
            return data;
        }
        catch (error) {
            if (!wasUnresolved && !(error instanceof ApiError && error.uncertain))
                delete this.pending[key];
            feedback(status, errorMessage(error, String(pending.body['requestId'])) + (this.pending[key] ? ' Retry this same captured operation to reconcile.' : ' Your inputs are retained.'), 'error');
            return null;
        }
        finally {
            this.busy.delete(key);
            writeSession('workflowPending', this.pending);
            this.controls();
        }
    }
    async mutateTask(action) {
        const task = this.task?.task;
        const workflow = this.selected?.workflow;
        if (!task || this.isHistorical())
            return;
        const generation = this.generation;
        const selector = this.selector;
        const selectedVersion = this.selected?.taskVersion;
        const key = action + ':' + task.id;
        if (!this.pending[key]) {
            if (action !== 'adopt' && (!workflow || !workflow.currentStepId))
                return;
            const body = { requestId: requestId(), expectedRevision: task.revision, taskVersion: task.currentVersion };
            if (action === 'adopt')
                body['workflow'] = { mode: 'auto' };
            else {
                body['workflowRevision'] = workflow.revision;
                if (action === 'check')
                    body['checkId'] = value('workflow-check-definition');
            }
            this.pending[key] = { path: '/tasks/' + task.id + '/workflow/' + action, method: 'POST', body };
        }
        const data = await this.perform(key, this.pending[key], 'workflow-status');
        if (data !== null) {
            if (generation === this.generation && task.id === this.task?.task.id && selector === this.selector && selectedVersion === this.selected?.taskVersion)
                feedback('workflow-status', action === 'adopt' ? 'Workflow adopted in a new captured version. No work was started. Use Run task to start its first author under this captured scope.' : 'Operation acknowledged. Its actual stage, cleanup and final delivery remain separately observed.', 'success');
            await this.onTaskChanged?.(task.id);
        }
    }
    async loadHistory(kind, append = false) {
        const taskId = this.task?.task.id;
        if (!taskId || !this.selected?.workflow || append && !this.cursors[kind])
            return;
        const generation = this.generation;
        const read = (this.historyGenerations[kind] ?? 0) + 1;
        this.historyGenerations[kind] = read;
        const selector = this.selectedQuery();
        try {
            const { data } = await api('/tasks/' + taskId + '/workflow/' + kind + '?' + selector + (append ? '&cursor=' + encodeURIComponent(this.cursors[kind]) : ''));
            if (generation !== this.generation || read !== this.historyGenerations[kind])
                return;
            this.histories[kind] = append ? [...(this.histories[kind] ?? []), ...data.items] : data.items;
            this.cursors[kind] = data.nextCursor;
            const list = element('workflow-' + kind + '-list');
            list.replaceChildren();
            for (const item of this.histories[kind])
                this.renderItem(list, kind, item);
            element('more-workflow-' + kind).hidden = !data.nextCursor;
            if (!this.histories[kind].length) {
                const li = document.createElement('li');
                li.textContent = 'No retained ' + kind + ' for this selected workflow.';
                list.append(li);
            }
        }
        catch (error) {
            if (generation === this.generation)
                feedback('workflow-status', errorMessage(error) + ' Existing evidence and current work are retained.', 'error');
        }
    }
    renderItem(list, kind, input) {
        const item = object(input);
        const li = document.createElement('li');
        const heading = document.createElement('p');
        if (kind === 'steps')
            heading.textContent = text(item['ordinal']) + ' · ' + text(item['role']) + ' · ' + text(item['executorKind']) + ' · ' + text(item['status']) + (item['executorKind'] === 'native' ? ' · native model admission' : ' · zero model admissions; native token/charge telemetry not applicable');
        else if (kind === 'reviews') {
            const report = object(item['report']);
            heading.textContent = text(item['outcome']) + ' · subject ' + text(object(report['subject'])['id']) + ' · inspection reported for ' + (Array.isArray(report['inspectedEvidence']) ? report['inspectedEvidence'].length : 0) + ' evidence references (model-reported inspected scope); controller supplied ' + (Array.isArray(item['suppliedEvidence']) ? item['suppliedEvidence'].length : 0) + ' references, with exact scopes in the retained record';
        }
        else if (kind === 'checks')
            heading.textContent = text(item['definitionId']) + ' · ' + text(item['outcome']) + (item['builtinCompleted'] ? ' · observed controller proof; no process or exit code' : ' · observed exit ' + text(item['exitCode']));
        else if (kind === 'findings') {
            const finding = object(item['finding']);
            heading.textContent = text(item['id']) + ' · ' + text(finding['severity']) + ' · ' + text(finding['summary']) + ' · retained qualified resolutions: ' + (Array.isArray(item['resolutions']) ? item['resolutions'].length : 0);
        }
        else if (kind === 'deliveries') {
            const operation = object(item['operation']);
            const external = object(item['external']);
            const acceptance = object(item['acceptance']);
            heading.textContent = text(item['kind']) + ' · ' + text(operation['status'] ?? external['outcome'] ?? acceptance['deliveryResult']) + (item['kind'] === 'external_command' ? ' · declared command completion; remote publication remains unverified' : '');
        }
        else
            heading.textContent = text(item['relativePath'] ?? item['path'] ?? item['id']) + ' · ' + text(item['action'] ?? item['completeness'] ?? item['kind']);
        li.append(heading);
        if (typeof item['attemptId'] === 'string')
            this.button(li, 'Inspect this stage', () => this.onSelectAttempt?.(item['attemptId']));
        if (kind === 'candidates' && typeof item['id'] === 'string')
            this.button(li, 'Read changed file metadata', () => { void this.loadChanges(item['id']); });
        this.artifactButtons(li, input);
        const details = document.createElement('details');
        const summary = document.createElement('summary');
        summary.textContent = 'Complete retained record and applicability';
        const pre = document.createElement('pre');
        pre.className = 'evidence-text';
        pre.tabIndex = 0;
        pre.textContent = JSON.stringify(input, null, 2);
        details.append(summary, pre);
        li.append(details);
        list.append(li);
    }
    artifactButtons(parent, input) {
        const ids = new Set();
        const visit = (entry) => { if (!entry || typeof entry !== 'object')
            return; for (const [key, child] of Object.entries(entry)) {
            if (/artifactId$/i.test(key) && typeof child === 'string' && /^[a-f0-9]{32}$/.test(child))
                ids.add(child);
            else if (/artifactIds$/i.test(key) && Array.isArray(child))
                for (const id of child) {
                    if (typeof id === 'string' && /^[a-f0-9]{32}$/.test(id))
                        ids.add(id);
                }
            else if (child && typeof child === 'object')
                visit(child);
        } };
        visit(input);
        for (const id of ids)
            this.button(parent, 'Read full evidence ' + id.slice(0, 8), () => this.onReadArtifact?.(id));
    }
    button(parent, label, action) { const button = document.createElement('button'); button.type = 'button'; button.className = 'quiet compact'; button.textContent = label; button.addEventListener('click', action); parent.append(button); }
    async loadChanges(id, append = false) {
        if (append && (id !== this.candidateId || !this.cursors['changes']))
            return;
        const generation = this.generation;
        const read = (this.historyGenerations['changes'] ?? 0) + 1;
        this.historyGenerations['changes'] = read;
        this.candidateId = id;
        try {
            const { data } = await api('/candidates/' + id + '/changes' + (append ? '?cursor=' + encodeURIComponent(this.cursors['changes']) : ''));
            if (generation !== this.generation || read !== this.historyGenerations['changes'])
                return;
            const list = element('workflow-changes-list');
            if (!append)
                list.replaceChildren();
            for (const item of data.items)
                this.renderItem(list, 'changes', item);
            this.cursors['changes'] = data.nextCursor;
            element('more-workflow-changes').hidden = !data.nextCursor;
        }
        catch (error) {
            if (generation === this.generation)
                feedback('workflow-status', errorMessage(error) + ' Full file bytes remain artifact-backed; no inline truncation was accepted.', 'error');
        }
    }
}
