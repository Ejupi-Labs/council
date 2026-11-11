import { addFact, api, ApiError, element, errorMessage, feedback, policyName, readSession, requestId, supportsManagedExecution, timeLabel, writeSession } from './api.js';
import { checkGraphTask, rememberGraphTask } from './graph-membership.js';
import { appendConnectionFacts, factLabel, harnessName, renderNativeModels, usageLabel } from './native-observations.js';
import { RecoveryView } from './recovery.js';
const input = (id) => element(id).value;
const checked = (id) => element(id).checked;
function composerHarnessName(id) { return id ? harnessName(id) : 'Auto · economical routing'; }
function label(attempt) {
    if (attempt.outcome === 'succeeded')
        return attempt.workflowId ? 'Stage succeeded' : 'Verification pending';
    return attempt.outcome ?? attempt.phase;
}
function row(list, title, action) {
    const li = document.createElement('li');
    const button = document.createElement('button');
    button.type = 'button';
    button.textContent = title;
    button.addEventListener('click', action);
    li.append(button);
    list.append(li);
}
export class TasksView {
    drafts;
    routing;
    workflow;
    onOpenGraph = (_graphId) => { };
    async openGraphTask(id, ref) {
        try {
            rememberGraphTask(id, ref);
            await this.openTask(id);
            element('task-detail').scrollIntoView({ block: 'start' });
        }
        catch (error) {
            feedback('execution-status', errorMessage(error), 'error');
        }
    }
    projectId = null;
    policy = null;
    available = false;
    recoveryAvailable = false;
    admission = 'open';
    capabilities = null;
    busy = false;
    projectGeneration = 0;
    detailGeneration = 0;
    attemptGeneration = 0;
    artifactGeneration = 0;
    evidenceSelection = 0;
    listGeneration = 0;
    detail = null;
    attempt = null;
    explicitAttempt = null;
    attemptSelection = 0;
    selectedTaskId = null;
    tasks = [];
    attempts = [];
    artifacts = [];
    events = [];
    taskCursor = null;
    attemptCursor = null;
    artifactCursor = null;
    eventCursor = null;
    pendingStart = readSession('pendingExecutionStart');
    pending = readSession('pendingExecutionControls') ?? {};
    mutationBusy = new Set();
    readArtifact = null;
    downloadURL = null;
    guidanceEdited = false;
    guidanceEditors = readSession('taskGuidanceEditors') ?? {};
    guidanceHarnessEditors = readSession('taskGuidanceHarnessEditors') ?? {};
    recovery;
    globalRecovery;
    constructor(drafts, routing, workflow) {
        this.drafts = drafts;
        this.routing = routing;
        this.workflow = workflow;
        this.recovery = new RecoveryView(element('task-detail'), 'task', () => { if (this.selectedTaskId)
            void this.openTask(this.selectedTaskId, true); });
        this.globalRecovery = new RecoveryView(element('execution-status').parentElement, 'global', () => { });
        const responseArea = element('response-area');
        const readButtonRow = element('read-response').parentElement;
        for (const node of [readButtonRow, element('artifact-status'), element('artifact-output'), element('response-acceptance')])
            responseArea.append(node);
        if (this.pendingStart)
            this.pendingStart.unresolved = true;
        for (const pending of Object.values(this.pending))
            pending.unresolved = true;
        const savedHarness = readSession('taskHarnessChoice');
        if (savedHarness && ['codex', 'claude-code'].includes(savedHarness))
            element('task-harness').value = savedHarness;
        element('composer-target').textContent = composerHarnessName(input('task-harness'));
        element('task-harness').addEventListener('change', () => {
            writeSession('taskHarnessChoice', input('task-harness'));
            element('composer-target').textContent = composerHarnessName(input('task-harness'));
        });
        element('guidance-harness').addEventListener('change', () => {
            if (this.selectedTaskId) {
                this.guidanceHarnessEditors[this.selectedTaskId] = input('guidance-harness');
                writeSession('taskGuidanceHarnessEditors', this.guidanceHarnessEditors);
            }
        });
        element('start-task').addEventListener('click', () => { void this.start(); });
        element('refresh-tasks').addEventListener('click', () => { void this.refreshTasks(); });
        element('more-tasks').addEventListener('click', () => { void this.refreshTasks(true); });
        element('more-attempts').addEventListener('click', () => { void this.readMore('attempts'); });
        element('more-artifacts').addEventListener('click', () => { void this.readMore('artifacts'); });
        element('more-events').addEventListener('click', () => { void this.readMore('events'); });
        element('refresh-attempt').addEventListener('click', () => { if (this.selectedTaskId)
            void this.openTask(this.selectedTaskId); });
        element('stop-attempt').addEventListener('click', () => { void this.stop(); });
        element('run-attempt').addEventListener('click', () => { void this.run(); });
        element('guidance-form').addEventListener('submit', e => { e.preventDefault(); void this.guidance(); });
        element('task-guidance').addEventListener('input', () => { this.guidanceEdited = true; if (this.selectedTaskId) {
            this.guidanceEditors[this.selectedTaskId] = input('task-guidance');
            writeSession('taskGuidanceEditors', this.guidanceEditors);
        } });
        element('read-response').addEventListener('click', () => { const id = this.attempt?.responseArtifactId; if (id)
            void this.readBytes(id); });
        element('download-artifact').addEventListener('click', () => {
            if (!this.downloadURL || !this.readArtifact)
                return;
            const a = document.createElement('a');
            a.href = this.downloadURL;
            a.download = this.readArtifact.kind + '-' + this.readArtifact.id + '.bin';
            a.click();
        });
        element('reviewed-response').addEventListener('change', () => this.controls());
        element('accept-response').addEventListener('click', () => { void this.accept(); });
        if (this.pendingStart)
            feedback('execution-status', 'A pending start is retained. Reconnect and explicitly retry that same clicked task; its outcome is unresolved.');
        this.controls();
    }
    selectHarness(id) {
        element('task-harness').value = id;
        writeSession('taskHarnessChoice', id);
        element('composer-target').textContent = harnessName(id);
        element('execution-options').open = true;
        element('task-harness').focus();
        element('task-harness').scrollIntoView({ block: 'center' });
        feedback('execution-status', harnessName(id) + ' selected for the next Start.' + (this.pendingStart ? ' The pending start keeps its original clicked target.' : ' Enter a task and start explicitly when ready.'));
    }
    setCapabilities(capabilities) {
        this.capabilities = capabilities;
        if (this.attempt)
            this.renderAttempt();
    }
    setOwner(meta) {
        this.admission = meta.admission?.state ?? 'open';
        this.available = supportsManagedExecution(meta);
        this.recoveryAvailable = this.available && meta.recoveryContract === '007-v1';
        this.globalRecovery.setGlobalIncident(meta.admission?.incidentId ?? null, this.recoveryAvailable);
        if (meta.admission && meta.admission.state !== 'open')
            feedback('execution-status', 'Dispatch is ' + meta.admission.state + (meta.admission.incidentId ? ' · incident ' + meta.admission.incidentId : '') + '. Inspect retained work; no automatic replay occurs.' + (meta.admission.incident ? ' ' + meta.admission.incident.code + ': ' + meta.admission.incident.message + ' Preserve the workspace and evidence. Inspect the incident before reconciliation.' : ''), 'error');
        if (!this.available)
            feedback('execution-status', 'This owner does not advertise the required 006-v1 execution contracts. Reopen with a compatible Council owner.', 'error');
        this.recovery.setTask(this.detail, this.recoveryAvailable);
        this.controls();
        if (this.available && this.projectId)
            void this.refreshTasks();
    }
    disconnect() { this.available = false; this.recoveryAvailable = false; this.attemptGeneration++; this.detailGeneration++; this.listGeneration++; this.recovery.setTask(this.detail, false); this.globalRecovery.setGlobalIncident(null, false); this.controls(); }
    setPolicy(policy) { this.policy = policy; this.controls(); }
    setProject(id) {
        if (id === this.projectId)
            return;
        this.projectId = id;
        this.projectGeneration++;
        this.detailGeneration++;
        this.attemptGeneration++;
        this.clearAttempt();
        this.selectedTaskId = null;
        this.detail = null;
        this.recovery.setTask(null, this.recoveryAvailable);
        this.tasks = [];
        this.taskCursor = null;
        this.routing?.clearTask();
        this.workflow?.clearTask();
        this.explicitAttempt = null;
        element('task-detail').hidden = true;
        element('execution-link').hidden = true;
        element('task-list').replaceChildren();
        this.controls();
        if (this.available)
            void this.refreshTasks();
    }
    controls() {
        const start = element('start-task');
        start.disabled = !this.available || !this.projectId || this.busy || this.admission !== 'open' && !this.pendingStart;
        start.textContent = this.busy ? 'Starting…' : this.pendingStart ? 'Retry pending start' : 'Start';
        const a = this.attempt;
        const current = !!a && a.taskVersion === this.detail?.task.currentVersion;
        const stop = this.stopSelection();
        const runKey = this.detail ? 'run:' + this.detail.task.id : '';
        const currentWorkflow = this.workflow?.workflow;
        const startAuthor = this.workflow?.authorStartReady === true;
        const graphMember = !!this.detail?.task.graphRef;
        element('stop-attempt').disabled = graphMember || !this.available || !stop || (!!stop.attempt?.settledAt && !this.pending[stop.key]) || !!stop && this.mutationBusy.has(stop.key);
        element('stop-attempt').textContent = stop && this.pending[stop.key] ? 'Retry pending Stop · ' + stop.id.slice(0, 8) : stop?.attempt?.stop ? 'Stop accepted · refresh cleanup' : currentWorkflow ? 'Stop current workflow stage' : 'Stop';
        element('run-attempt').disabled = graphMember || !this.available || !this.detail || (!!this.detail.latestAttempt && !this.detail.latestAttempt.settledAt && !this.pending[runKey]) || this.mutationBusy.has(runKey) || this.admission !== 'open' && !this.pending[runKey] || !!this.detail && this.routing?.requiresAdoption(this.detail.task.id) === true && !this.pending[runKey] || !!currentWorkflow && !startAuthor && !this.pending[runKey];
        element('run-attempt').textContent = this.pending[runKey] ? 'Retry pending execution' : startAuthor ? 'Run task' : currentWorkflow ? 'Use current workflow control above' : this.detail && this.routing?.requiresAdoption(this.detail.task.id) ? 'Adopt budget before new execution' : 'Start another execution';
        element('save-guidance').textContent = this.detail && this.pending['guidance:' + this.detail.task.id] ? 'Retry pending guidance save' : 'Save guidance';
        element('save-guidance').disabled = graphMember || !this.available || !this.detail || this.mutationBusy.has('guidance:' + this.detail.task.id);
        element('read-response').disabled = !a?.responseArtifactId;
        const v = this.detail?.version;
        const response = this.readArtifact;
        const workflow = this.workflow?.workflow;
        const binding = this.workflow?.responseBinding;
        const subjectEligible = workflow ? workflow.taskVersion === this.detail?.task.currentVersion && !workflow.hasTaskDelta && !workflow.hasTaskIndexDelta && this.workflow?.responseAcceptanceReady && binding?.producerAttemptId === a?.id && binding?.responseArtifactId === response?.id && binding?.responseSha256 === response?.sha256 : a?.id === this.detail?.task.latestAttemptId && v?.acceptance.requiredEvidence.length === 0;
        const eligible = !!a && current && !!subjectEligible && a.outcome === 'succeeded' && a.cleanup.state === 'verified' && a.evidenceStatus === 'complete' && !a.authoredDelta && v?.acceptance.deliverable === 'response_only' && response?.id === a.responseArtifactId && response.closed && response.completeness === 'complete' && !!response.sha256 && !this.detail?.acceptance;
        element('response-acceptance').hidden = !eligible;
        element('accept-response').disabled = !this.available || !eligible || !checked('reviewed-response') || this.mutationBusy.has('accept:' + a?.id);
    }
    stopSelection() {
        const taskId = this.selectedTaskId;
        if (!taskId)
            return null;
        const candidates = [...this.attempts, ...(this.attempt ? [this.attempt] : []), ...(this.detail?.latestAttempt ? [this.detail.latestAttempt] : [])];
        const pending = Object.entries(this.pending).find(([key, operation]) => key.startsWith('stop:') && (operation.taskId === taskId || !operation.taskId && candidates.some(attempt => key === 'stop:' + attempt.id && attempt.taskId === taskId)));
        if (pending) {
            const id = pending[0].slice(5);
            return { id, key: pending[0], attempt: candidates.find(attempt => attempt.id === id) ?? null };
        }
        const attempt = this.workflow?.workflow ? this.detail?.latestAttempt ?? this.attempt : this.attempt;
        return attempt ? { id: attempt.id, key: 'stop:' + attempt.id, attempt } : null;
    }
    async inspectAttempt(id) {
        const selection = ++this.attemptSelection;
        const taskId = this.selectedTaskId;
        const generation = this.detailGeneration;
        if (!taskId)
            return;
        try {
            const { data } = await api('/attempts/' + id);
            if (taskId !== this.selectedTaskId || generation !== this.detailGeneration || selection !== this.attemptSelection || data.taskId !== taskId)
                return;
            await this.selectAttempt(data, true);
        }
        catch (error) {
            if (generation === this.detailGeneration)
                feedback('attempt-status', errorMessage(error), 'error');
        }
    }
    async openEvidence(id) {
        const selection = ++this.attemptSelection;
        const evidence = ++this.evidenceSelection;
        const taskId = this.selectedTaskId;
        const generation = this.detailGeneration;
        if (!taskId)
            return;
        try {
            const { data: artifact } = await api('/artifacts/' + id);
            const { data: attempt } = await api('/attempts/' + artifact.attemptId);
            if (taskId !== this.selectedTaskId || generation !== this.detailGeneration || selection !== this.attemptSelection || evidence !== this.evidenceSelection || attempt.taskId !== taskId)
                return;
            await this.selectAttempt(attempt, true);
            if (generation === this.detailGeneration && selection + 1 === this.attemptSelection && evidence === this.evidenceSelection && this.attempt?.id === attempt.id)
                await this.readBytes(id);
        }
        catch (error) {
            if (generation === this.detailGeneration)
                feedback('artifact-status', errorMessage(error) + ' Existing evidence remains retained.', 'error');
        }
    }
    startOptions() {
        const timeout = input('task-timeout');
        if (timeout && (!/^\d+$/.test(timeout) || Number(timeout) < 1 || Number(timeout) > 86400))
            throw new ApiError('invalid_input', 'Set a deadline from 1 through 86400 seconds or leave it at the default.');
        const required = input('task-required-evidence').split(/\r?\n/).map(v => v.trim()).filter(Boolean);
        if (required.some(v => !['check', 'independent_review', 'local_integration', 'external_delivery'].includes(v)) || new Set(required).size !== required.length)
            throw new ApiError('invalid_input', 'Required evidence entries are unique values: check, independent_review, local_integration, external_delivery.');
        const routing = this.routing?.composerOptions();
        const workflow = this.workflow?.composerOptions();
        return {
            ...(workflow ? { workflow } : {}),
            ...(routing ? { routing } : {}),
            ...(input('task-harness') || input('task-model') ? { pins: { ...(input('task-harness') ? { harnessId: input('task-harness') } : {}), ...(input('task-model') ? { model: input('task-model') } : {}) } } : {}),
            ...(timeout ? { timeoutSeconds: Number(timeout) } : {}),
            acceptance: { deliverable: input('task-deliverable'), conditionsText: input('task-conditions'), requiredEvidence: required, allowCandidatePreparation: !checked('task-final-only') },
            ...(checked('task-fake') ? { executionMode: 'fake', fakeScenario: input('task-fake-scenario') } : {}),
        };
    }
    async start() {
        if (!this.available || this.busy)
            return;
        try {
            if (!this.pendingStart) {
                const intent = this.drafts.executionIntent();
                this.pendingStart = { intent, body: { requestId: requestId(), projectId: intent.projectId, originalText: intent.originalText, ...this.startOptions(),
                        ...(intent.sourceDraftRef ? { sourceDraftRef: intent.sourceDraftRef } : this.policy ? { expectedPolicyRevision: this.policy.revision } : {}) } };
            }
        }
        catch (e) {
            feedback('execution-status', errorMessage(e), 'error');
            return;
        }
        const pending = this.pendingStart;
        const wasUnresolved = pending.unresolved === true;
        const detailGeneration = this.detailGeneration;
        const projectGeneration = this.projectGeneration;
        const selectedTaskId = this.selectedTaskId;
        const attemptSelection = this.attemptSelection;
        pending.unresolved = true;
        this.busy = true;
        this.controls();
        writeSession('pendingExecutionStart', pending);
        feedback('execution-status', 'Preparing this exact clicked task. Saving a draft and admitting an execution have separate acknowledgements.');
        try {
            if (pending.intent.save) {
                pending.intent.save.unresolved = true;
                this.drafts.reserveExecutionSave(pending.intent);
                const { data } = await api(pending.intent.save.path, pending.intent.save.method, pending.intent.save.body);
                this.drafts.finishExecutionSave(pending.intent, data);
                pending.body['sourceDraftRef'] = { draftId: data.id, revision: data.revision };
                delete pending.intent.save;
                writeSession('pendingExecutionStart', pending);
            }
            const { data } = await api('/tasks', 'POST', pending.body);
            this.pendingStart = null;
            writeSession('pendingExecutionStart', null);
            feedback('execution-status', 'Admitted task ' + data.task.id + ' · ' + data.attempt.phase + '. Captured workflow authority determines later stages; completion requires its retained evidence.', 'success');
            if (data.task.projectId === this.projectId) {
                await this.refreshTasks();
                // A late acknowledgement must not replace a selection made during admission or refresh.
                if (detailGeneration === this.detailGeneration && projectGeneration === this.projectGeneration && selectedTaskId === this.selectedTaskId && attemptSelection === this.attemptSelection)
                    await this.openTask(data.task.id);
            }
        }
        catch (e) {
            const uncertain = wasUnresolved || e instanceof ApiError && e.uncertain;
            this.drafts.finishExecutionSave(pending.intent, null, uncertain);
            feedback('execution-status', errorMessage(e, String(pending.body['requestId'])) + (uncertain ? ' Retry this same start to reconcile; do not create a replacement request.' : ' Your clicked text and local edits are retained.'), 'error');
            if (!uncertain) {
                this.pendingStart = null;
                writeSession('pendingExecutionStart', null);
            }
        }
        finally {
            this.busy = false;
            this.controls();
        }
    }
    refreshRoutingControls() { this.controls(); }
    async refreshAfterRoutingMutation(id) {
        const generation = this.detailGeneration;
        const projectGeneration = this.projectGeneration;
        await this.refreshTasks();
        if (this.selectedTaskId === id && generation === this.detailGeneration && projectGeneration === this.projectGeneration)
            await this.openTask(id, true);
    }
    async refreshTasks(append = false) {
        const project = this.projectId;
        if (!project || !this.available || append && !this.taskCursor)
            return;
        const generation = ++this.listGeneration;
        try {
            const { data } = await api('/tasks?projectId=' + project + (append && this.taskCursor ? '&cursor=' + encodeURIComponent(this.taskCursor) : ''));
            if (generation !== this.listGeneration || project !== this.projectId)
                return;
            for (const task of data.items)
                checkGraphTask(task);
            this.tasks = append ? [...this.tasks, ...data.items] : data.items;
            this.taskCursor = data.nextCursor;
            const list = element('task-list');
            list.replaceChildren();
            for (const task of this.tasks)
                row(list, 'Task ' + task.id.slice(0, 8) + ' · ' + task.status.replaceAll('_', ' ') + ' · ' + task.stage.replaceAll('_', ' ') + ' · ' + task.nextAction, () => { void this.openTask(task.id); });
            element('more-tasks').hidden = !this.taskCursor;
            feedback('task-list-status', this.tasks.length ? 'Select retained work to inspect its original text, executions and evidence.' : 'No executions in this project yet. Describe a task above and Start when ready.');
        }
        catch (e) {
            if (generation === this.listGeneration)
                feedback('task-list-status', errorMessage(e) + ' Retained work is not replayed.', 'error');
        }
    }
    async openTask(id, keepAttempt = false) {
        const generation = ++this.detailGeneration;
        const projectGeneration = this.projectGeneration;
        const changed = id !== this.selectedTaskId;
        if (!keepAttempt || changed)
            this.explicitAttempt = null;
        const selection = this.attemptSelection;
        if (changed) {
            this.routing?.clearTask();
            this.workflow?.clearTask();
            this.selectedTaskId = id;
            this.attemptGeneration++;
            this.clearAttempt();
            this.guidanceEdited = false;
            this.detail = null;
            this.recovery.setTask(null, this.recoveryAvailable);
            element('task-detail').hidden = true;
            element('execution-link').hidden = true;
            this.controls();
        }
        try {
            const { data } = await api('/tasks/' + id);
            if (generation !== this.detailGeneration || projectGeneration !== this.projectGeneration || id !== this.selectedTaskId)
                return;
            if (data.task.id !== id || data.task.projectId !== this.projectId)
                throw new Error('The task detail does not identify this selected task and project.');
            checkGraphTask(data.task);
            this.detail = data;
            element('task-detail').hidden = false;
            element('execution-link').hidden = false;
            element('task-detail-heading').textContent = 'Task ' + id.slice(0, 8) + ' · version ' + data.task.currentVersion;
            this.renderTaskFacts(data);
            await this.workflow?.setTask(data);
            if (generation !== this.detailGeneration || projectGeneration !== this.projectGeneration || id !== this.selectedTaskId)
                return;
            let selected = data.latestAttempt;
            const producerId = this.explicitAttempt ?? await this.workflow?.producerAttemptId();
            if (producerId && producerId !== selected?.id)
                selected = (await api('/attempts/' + producerId)).data;
            if (generation !== this.detailGeneration || projectGeneration !== this.projectGeneration || id !== this.selectedTaskId)
                return;
            if (selection === this.attemptSelection) {
                if (selected && selected.taskId === id)
                    void this.selectAttempt(selected);
                else if (!selected) {
                    this.clearAttempt();
                    this.controls();
                }
            }
            try {
                await this.loadAttempts();
            }
            catch (error) {
                if (generation === this.detailGeneration)
                    element('usage-total').textContent = errorMessage(error) + ' Execution history is temporarily unavailable; current execution detail remains available.';
            }
            this.controls();
        }
        catch (e) {
            if (generation === this.detailGeneration)
                feedback('execution-status', errorMessage(e), 'error');
        }
    }
    renderTaskFacts(data) {
        this.recovery.setTask(data, this.recoveryAvailable);
        element('task-detail-heading').textContent = 'Task ' + data.task.id.slice(0, 8) + ' · version ' + data.task.currentVersion;
        const facts = element('task-facts');
        facts.replaceChildren();
        addFact(facts, 'Task identity', data.task.id);
        addFact(facts, 'Captured requested preference', policyName(data.version.requestedPolicy.mode) + ' · revision ' + data.version.requestedPolicy.revision);
        addFact(facts, 'Captured harness choice', harnessName(data.version.pins.harnessId));
        addFact(facts, 'Requested model', data.version.pins.model || 'Auto · no literal model pin; see the admitted resolution separately');
        addFact(facts, 'Current task version', data.task.currentVersion);
        const membership = element('task-graph-membership');
        membership.replaceChildren();
        membership.hidden = !data.task.graphRef;
        if (data.task.graphRef) {
            const ref = data.task.graphRef;
            addFact(facts, 'Task group', ref.graphId + ' · ' + ref.kind + (ref.nodeId ? ' · node ' + ref.nodeId : ''));
            const note = document.createElement('p');
            note.textContent = 'This task uses shared group authority. Run, Stop, guidance and additional allowance belong to its current task group; historical producer evidence remains readable.';
            const open = document.createElement('button');
            open.type = 'button';
            open.className = 'secondary';
            open.textContent = 'Open current task group';
            open.addEventListener('click', () => this.onOpenGraph(ref.graphId));
            membership.append(note, open);
        }
        element('guidance-keep-target').textContent = 'Keep captured target · ' + harnessName(data.version.pins.harnessId);
        element('guidance-harness').value = this.guidanceHarnessEditors[data.task.id] ?? 'keep';
        addFact(facts, 'Response acceptance', data.acceptance ? 'Reviewed response accepted · ' + timeLabel(data.acceptance.acceptedAt) : 'Not accepted');
        void this.routing?.setTask(data);
        element('task-original').textContent = data.task.originalText;
        element('task-conditions-view').textContent = data.version.acceptance.conditionsText + (data.version.acceptance.requiredEvidence.length ? '\nRequired evidence: ' + data.version.acceptance.requiredEvidence.join(', ') : '');
        if (!this.guidanceEdited) {
            const cached = this.guidanceEditors[data.task.id];
            element('task-guidance').value = cached ?? data.version.guidanceText;
            this.guidanceEdited = cached !== undefined && cached !== data.version.guidanceText;
        }
    }
    async loadAttempts(append = false) {
        const id = this.selectedTaskId;
        const generation = this.detailGeneration;
        if (!id || append && !this.attemptCursor)
            return;
        const { data } = await api('/tasks/' + id + '/attempts' + (append && this.attemptCursor ? '?cursor=' + encodeURIComponent(this.attemptCursor) : ''));
        if (id !== this.selectedTaskId || generation !== this.detailGeneration)
            return;
        this.attempts = append ? [...this.attempts, ...data.items] : data.items;
        this.attemptCursor = data.nextCursor;
        const list = element('attempt-list');
        list.replaceChildren();
        let total = 0n;
        let unknown = false;
        for (const attempt of this.attempts) {
            const local = !!attempt.executorKind && attempt.executorKind !== 'native';
            row(list, attempt.id.slice(0, 8) + ' · task version ' + attempt.taskVersion + ' · ' + label(attempt) + ' · ' + (local ? 'Local executor · ' + attempt.executorKind + ' · ' + attempt.role : (attempt.executionMode === 'fake' ? 'Local fake · ' : '') + attempt.connectionId), () => { void this.selectAttempt(attempt, true); });
            if (local)
                continue;
            const usage = attempt.targetObservations.usage;
            for (const count of [usage.inputTokens, usage.outputTokens]) {
                if (count !== null && /^\d+$/.test(count))
                    total += BigInt(count);
                else
                    unknown = true;
            }
        }
        element('more-attempts').hidden = !this.attemptCursor;
        if (this.routing) {
            element('usage-total').textContent = 'Execution history is not a token or cost total. Read a sourced usage page in Routing decisions, budget authority and usage for exact per-scope counters, page coverage and unknown charges.';
            return;
        }
        element('usage-total').textContent = 'Reported input + output across loaded executions (native scopes may differ): ' + total + ' tokens.' + (unknown ? ' Some usage is unknown.' : '') + (this.attemptCursor ? ' More history is available.' : '') + ' Cached input is not added twice. Whole-tree previews are not added to these counters. Native call counts, cost and allowance remain unknown.';
    }
    async selectAttempt(attempt, explicit = false) {
        if (attempt.taskId !== this.selectedTaskId)
            return;
        if (explicit) {
            this.explicitAttempt = attempt.id;
            this.attemptSelection++;
        }
        if (attempt.id === this.attempt?.id && BigInt(attempt.revision) < BigInt(this.attempt.revision))
            return;
        const changed = attempt.id !== this.attempt?.id;
        const phaseChanged = attempt.phase !== this.attempt?.phase;
        const generation = ++this.attemptGeneration;
        if (changed)
            this.clearArtifact();
        this.attempt = attempt;
        this.renderAttempt();
        if (changed || phaseChanged)
            void this.refreshTasks();
        await Promise.allSettled([this.loadEvents(), this.loadArtifacts()]);
        if (!attempt.settledAt)
            void this.observe(attempt.id, generation);
    }
    renderAttempt() {
        this.recovery.setIncident(this.attempt?.incident?.id ?? this.attempt?.cleanup.incidentId ?? null);
        const a = this.attempt;
        if (!a)
            return;
        const usage = a.targetObservations.usage;
        const local = !!a.executorKind && a.executorKind !== 'native';
        const reportedUsage = local ? 'Not applicable to managed native-model counters · local executor' : usageLabel(usage);
        feedback('attempt-status', (local ? 'LOCAL EXECUTOR · ' + a.executorKind + ' · ' : a.executionMode === 'fake' ? 'LOCAL FAKE · ' : '') + label(a) + (a.taskVersion !== this.detail?.task.currentVersion ? ' · Historical task version; current guidance needs an explicit new execution.' : '')
            + (a.primaryFailure ? ' · ' + a.primaryFailure.code + ': ' + a.primaryFailure.message : '') + (a.stop ? ' · Stop accepted; cleanup remains ' + a.cleanup.state + '.' : '')
            + (a.candidateId && a.authoredDelta ? a.workflowId ? ' · Candidate retained; current qualification and delivery are shown above.' : ' · Candidate retained. Verification pending; integration not performed.' : a.candidateId ? a.workflowId ? ' · This stage added no source or staging delta; complete task scope is shown above.' : ' · No authored source or staging changes.' : ''), a.primaryFailure || a.cleanup.state === 'unknown' ? 'error' : 'neutral');
        const summary = element('attempt-summary');
        summary.replaceChildren();
        addFact(summary, 'Cleanup and evidence', a.cleanup.state.replaceAll('_', ' ') + ' · ' + a.evidenceStatus);
        addFact(summary, 'Deadline', timeLabel(a.deadlineAt) + ' · ' + a.remainingSeconds + ' seconds remaining at ' + timeLabel(a.observedAt));
        addFact(summary, 'Captured connection', local ? 'Local executor · no native model connection' : a.connectionId + ' · ' + (a.executionMode === 'fake' ? 'Local fake' : 'Native execution'));
        addFact(summary, 'Reported usage', reportedUsage);
        if (a.workflowId)
            addFact(summary, 'Workflow stage', (a.role ?? 'Unknown role') + ' · ' + a.executorKind + ' · step ' + a.stepId);
        const facts = element('attempt-facts');
        facts.replaceChildren();
        addFact(facts, 'Execution identity', a.id);
        addFact(facts, 'Authoring outcome', label(a));
        addFact(facts, 'Deadline', timeLabel(a.deadlineAt) + ' · ' + a.remainingSeconds + ' seconds remaining at ' + timeLabel(a.observedAt));
        addFact(facts, 'Cleanup', a.cleanup.state + ' · ' + a.cleanup.scope.replaceAll('_', ' ') + (a.cleanup.incidentId ? ' · incident ' + a.cleanup.incidentId : ''));
        addFact(facts, 'Evidence', a.evidenceStatus);
        addFact(facts, 'Source snapshot', a.sourceSnapshotId ?? 'Not captured');
        addFact(facts, 'Candidate', a.candidateId ?? 'No candidate retained');
        addFact(facts, 'Captured connection', a.connectionId);
        addFact(facts, 'Requested harness choice', harnessName(a.requestedTarget.harnessId));
        addFact(facts, 'Requested model selector', a.requestedTarget.model || 'Auto · no literal model pin');
        if (a.resolvedTarget) {
            addFact(facts, 'Frozen resolution for this attempt', a.resolvedTarget.contextId + ' · ' + (a.resolvedTarget.modelSelector || 'Native configured default') + ' · profile ' + (a.resolvedTarget.profileId || 'uncatalogued literal selector'));
            addFact(facts, 'Routing decision reference', a.routingDecisionId || 'Not recorded');
            addFact(facts, 'Captured declared account association', a.resolvedTarget.declaredAccountAssociation ? a.resolvedTarget.declaredAccountAssociation.label + ' · ' + a.resolvedTarget.declaredAccountAssociation.accountId + ' · user declaration, actual login unknown' : 'None · actual login remains separately observed or unknown');
        }
        addFact(facts, 'Observed provider', factLabel(a.targetObservations.provider));
        addFact(facts, 'Observed account', factLabel(a.targetObservations.account));
        addFact(facts, 'Observed actual primary model', factLabel(a.targetObservations.model));
        addFact(facts, 'Native configured model setting', factLabel(a.targetObservations.configuredModel));
        addFact(facts, 'Observed invocation version', factLabel(a.targetObservations.nativeVersion));
        addFact(facts, 'Observed native session ID', factLabel(a.targetObservations.nativeSession));
        appendConnectionFacts(facts, this.capabilities?.connections.find(connection => connection.connectionId === a.connectionId));
        renderNativeModels(a, id => { void this.readBytes(id); });
        if (a.sourceSnapshot) {
            addFact(facts, 'Captured source base', a.sourceSnapshot.baseCommit);
            addFact(facts, 'Captured source branch', a.sourceSnapshot.branch ?? 'Detached HEAD');
            addFact(facts, 'Selected source folder', a.sourceSnapshot.selectedRelativePath || '.');
            addFact(facts, 'Source manifest SHA256', a.sourceSnapshot.manifestSha256);
        }
        if (a.candidate) {
            addFact(facts, 'Retained managed workspace', a.candidate.workspaceRef);
            addFact(facts, 'Candidate changes at creation', a.candidate.hasDelta ? 'Authored changes retained.' + (a.candidate.hasIndexDelta ? ' Staged index changes are included.' : '') : 'No authored source or staging changes.');
            addFact(facts, 'Candidate status at creation', 'Verification ' + a.candidate.verification.replaceAll('_', ' ') + ' · integration ' + a.candidate.integration.replaceAll('_', ' ') + '. Immutable creation facts.' + (a.workflowId ? ' Current task qualification and delivery are shown in Verified workflow above.' : ' Later qualification and delivery require separate evidence.'));
        }
        if (a.incident)
            addFact(facts, 'Retained incident', a.incident.code + ': ' + a.incident.message + ' Preserve this workspace and evidence. Inspect the incident before reconciliation; uncertain ownership does not authorize replay.');
        const permissions = a.targetObservations.permissions;
        addFact(facts, 'Permissions', 'Requested ' + permissions.requestedMode + ' · configured ' + (permissions.configuredMode ?? 'unknown') + ' · native reported ' + (permissions.nativeReportedMode ?? 'unknown'));
        if (permissions.limitations.length)
            addFact(facts, 'Permission scope', permissions.limitations.join(' '));
        addFact(facts, 'Reported usage', reportedUsage);
        addFact(facts, 'Usage and billing limits', 'Top-level counters and whole-tree model previews have separate scopes. Native client estimates are not billed charges, subscription allowance or an independent accounting receipt.');
        this.controls();
    }
    async observe(id, generation) {
        while (this.available && generation === this.attemptGeneration && this.attempt?.id === id && !this.attempt.settledAt) {
            try {
                const { data } = await api('/attempts/' + id + '/wait?afterRevision=' + this.attempt.revision + '&timeoutMs=30000');
                if (generation !== this.attemptGeneration || this.attempt?.id !== id)
                    return;
                const phaseChanged = data.attempt.phase !== this.attempt.phase;
                this.attempt = data.attempt;
                this.renderAttempt();
                if (phaseChanged && !data.attempt.settledAt)
                    void this.refreshTasks();
                if (data.changed)
                    await Promise.allSettled([this.loadEvents(), this.loadArtifacts()]);
                if (data.attempt.settledAt) {
                    await this.loadAttempts();
                    const detailGeneration = this.detailGeneration;
                    const projectGeneration = this.projectGeneration;
                    const { data: detail } = await api('/tasks/' + data.attempt.taskId);
                    if (generation !== this.attemptGeneration || detailGeneration !== this.detailGeneration || projectGeneration !== this.projectGeneration || this.selectedTaskId !== detail.task.id || this.detail?.task.id === detail.task.id && BigInt(detail.task.currentVersion) < BigInt(this.detail.task.currentVersion))
                        return;
                    // This terminal read cannot replace a newer detail request or version.
                    // Current-version metadata refresh does not rewrite the guidance editor.
                    checkGraphTask(detail.task);
                    this.detail = detail;
                    this.renderTaskFacts(detail);
                    this.renderAttempt();
                    await this.workflow?.setTask(detail);
                    if (generation !== this.attemptGeneration || detailGeneration !== this.detailGeneration || projectGeneration !== this.projectGeneration || this.selectedTaskId !== detail.task.id)
                        return;
                    const { data: meta } = await api('/meta');
                    if (generation === this.attemptGeneration && detailGeneration === this.detailGeneration && projectGeneration === this.projectGeneration && this.selectedTaskId === detail.task.id)
                        this.setOwner(meta);
                    void this.refreshTasks();
                    return;
                }
            }
            catch (e) {
                if (generation === this.attemptGeneration)
                    feedback('attempt-status', errorMessage(e) + ' Observation ended; the task was not cancelled. Refresh detail to reconnect.', 'error');
                return;
            }
        }
    }
    async readMore(kind) {
        try {
            if (kind === 'attempts')
                await this.loadAttempts(true);
            else if (kind === 'artifacts')
                await this.loadArtifacts(true);
            else
                await this.loadEvents(true);
        }
        catch (error) {
            feedback('attempt-status', errorMessage(error) + ' Retained work and prior evidence remain available.', 'error');
        }
    }
    async loadArtifacts(append = false) {
        const a = this.attempt;
        const generation = this.attemptGeneration;
        if (!a || append && !this.artifactCursor)
            return;
        const { data } = await api('/attempts/' + a.id + '/artifacts' + (append && this.artifactCursor ? '?cursor=' + encodeURIComponent(this.artifactCursor) : ''));
        if (generation !== this.attemptGeneration || a.id !== this.attempt?.id)
            return;
        this.artifacts = append ? [...this.artifacts, ...data.items] : data.items;
        this.artifactCursor = data.nextCursor;
        const list = element('artifact-list');
        list.replaceChildren();
        for (const artifact of this.artifacts)
            row(list, artifact.kind + ' · ' + artifact.byteLength + ' bytes · ' + artifact.completeness, () => { void this.readBytes(artifact.id); });
        element('more-artifacts').hidden = !this.artifactCursor;
    }
    async loadEvents(append = false) {
        const a = this.attempt;
        const generation = this.attemptGeneration;
        if (!a || append && !this.eventCursor)
            return;
        const { data } = await api('/attempts/' + a.id + '/events' + (append && this.eventCursor ? '?cursor=' + encodeURIComponent(this.eventCursor) : ''));
        if (generation !== this.attemptGeneration || a.id !== this.attempt?.id)
            return;
        this.events = append ? [...this.events, ...data.items] : data.items;
        this.eventCursor = data.nextCursor;
        const list = element('attempt-events');
        list.replaceChildren();
        for (const event of this.events) {
            const li = document.createElement('li');
            li.textContent = event.sequence + ' · ' + event.kind + ' · ' + timeLabel(event.observedAt) + ' · ' + JSON.stringify(event.metadata);
            list.append(li);
        }
        element('more-events').hidden = !this.eventCursor;
    }
    // Empties every execution panel and its pagination so a different task never shows (or pages
    // with) another task's execution identity, evidence, events or cursors.
    clearAttempt() {
        this.attempt = null;
        this.attempts = [];
        this.artifacts = [];
        this.events = [];
        this.attemptCursor = null;
        this.artifactCursor = null;
        this.eventCursor = null;
        for (const id of ['attempt-summary', 'attempt-facts', 'attempt-list', 'attempt-events', 'artifact-list', 'native-models-facts', 'native-models-list', 'native-models-evidence'])
            element(id).replaceChildren();
        for (const id of ['more-attempts', 'more-events', 'more-artifacts', 'native-models-panel'])
            element(id).hidden = true;
        feedback('attempt-status', '');
        this.clearArtifact();
    }
    clearArtifact() {
        this.artifactGeneration++;
        this.readArtifact = null;
        if (this.downloadURL)
            URL.revokeObjectURL(this.downloadURL);
        this.downloadURL = null;
        element('artifact-output').textContent = '';
        element('artifact-status').textContent = '';
        element('download-artifact').disabled = true;
        element('reviewed-response').checked = false;
    }
    async readBytes(id) {
        this.evidenceSelection++;
        this.clearArtifact();
        const generation = this.artifactGeneration;
        const attemptId = this.attempt?.id;
        feedback('artifact-status', 'Reading a fixed durable byte snapshot…');
        try {
            const { data: artifact } = await api('/artifacts/' + id);
            if (generation !== this.artifactGeneration || attemptId !== this.attempt?.id)
                return;
            if (artifact.attemptId !== attemptId)
                throw new ApiError('artifact_mismatch', 'This artifact belongs to another execution.');
            let offset = '0';
            let through = null;
            const decoder = new TextDecoder();
            const pieces = [];
            let text = '';
            while (true) {
                const result = await api('/artifacts/' + id + '/bytes?offset=' + offset + (through === null ? '' : '&through=' + through));
                const data = result.data;
                if (generation !== this.artifactGeneration || attemptId !== this.attempt?.id)
                    return;
                if (data.artifactId !== id || data.offset !== offset || through !== null && data.through !== through || !/^\d+$/.test(data.through) || !/^\d+$/.test(data.nextOffset) || BigInt(data.through) > 64n * 1024n * 1024n)
                    throw new ApiError('invalid_response', 'The evidence snapshot changed or exceeded its read bound.');
                const bound = through ?? data.through;
                through = bound;
                const bytes = Uint8Array.from(atob(data.bytesBase64), c => c.charCodeAt(0));
                if (BigInt(data.nextOffset) !== BigInt(offset) + BigInt(bytes.length) || BigInt(data.nextOffset) > BigInt(bound) || data.eof !== (data.nextOffset === bound) || !data.eof && data.nextOffset === offset)
                    throw new ApiError('invalid_response', 'The evidence byte cursor did not advance consistently.');
                pieces.push(bytes);
                text += decoder.decode(bytes, { stream: !data.eof });
                element('artifact-output').textContent = text;
                offset = data.nextOffset;
                if (data.eof) {
                    this.readArtifact = { ...artifact, closed: data.streamClosed, completeness: data.completeness, sha256: data.sha256 };
                    this.downloadURL = URL.createObjectURL(new Blob(pieces, { type: 'application/octet-stream' }));
                    element('download-artifact').disabled = false;
                    feedback('artifact-status', 'Read ' + offset + ' of ' + through + ' snapshot bytes · ' + data.completeness + (data.streamClosed ? ' · stream closed.' : ' · stream still collecting; read again for a newer prefix.') + ' Text is decoded for display; download preserves exact bytes.');
                    this.controls();
                    return;
                }
            }
        }
        catch (e) {
            if (generation === this.artifactGeneration)
                feedback('artifact-status', errorMessage(e) + ' No response acceptance was recorded.', 'error');
        }
    }
    async mutate(key, path, body) {
        if (!this.available || this.mutationBusy.has(key))
            return null;
        const pending = this.pending[key] ?? { path, method: 'POST', body, ...(this.selectedTaskId ? { taskId: this.selectedTaskId } : {}) };
        const wasUnresolved = pending.unresolved === true;
        pending.unresolved = true;
        this.pending[key] = pending;
        this.mutationBusy.add(key);
        writeSession('pendingExecutionControls', this.pending);
        this.controls();
        try {
            const { data } = await api(pending.path, pending.method, pending.body);
            delete this.pending[key];
            return data;
        }
        catch (e) {
            if (!wasUnresolved && !(e instanceof ApiError && e.uncertain))
                delete this.pending[key];
            // A late failure for task A must not paint into task B's status region; A's retained
            // pending operation is shown again when A is reopened.
            if (!pending.taskId || pending.taskId === this.selectedTaskId)
                feedback('attempt-status', errorMessage(e, String(pending.body['requestId'])) + (this.pending[key] ? ' Retry this same pending operation to reconcile.' : ' Your input is retained.'), 'error');
            return null;
        }
        finally {
            this.mutationBusy.delete(key);
            writeSession('pendingExecutionControls', this.pending);
            this.controls();
        }
    }
    async stop() {
        const target = this.stopSelection();
        const taskId = this.selectedTaskId;
        if (!target || !taskId || this.detail?.task.graphRef)
            return;
        const result = await this.mutate(target.key, '/attempts/' + target.id + '/stop', { requestId: requestId() });
        if (!result || taskId !== this.selectedTaskId)
            return;
        feedback('attempt-status', result.stopAccepted ? 'Stop accepted for captured stage ' + target.id + '. Waiting for actual process, output and cleanup settlement.' : 'Captured execution ' + target.id + ' was already settled. Its outcome is retained; this acknowledgement does not retarget another stage.');
        try {
            const { data } = await api('/attempts/' + target.id);
            if (taskId !== this.selectedTaskId)
                return;
            if (this.detail?.latestAttempt?.id === data.id)
                this.detail.latestAttempt = data;
            if (data.id === this.attempt?.id)
                void this.selectAttempt(data);
            else
                await this.refreshAfterRoutingMutation(taskId);
        }
        catch (error) {
            if (taskId === this.selectedTaskId)
                feedback('attempt-status', errorMessage(error) + ' The exact Stop acknowledgement is retained; refresh to observe actual cleanup.', 'error');
        }
    }
    async run() {
        const detail = this.detail;
        if (!detail || detail.task.graphRef)
            return;
        const workflow = this.workflow?.workflow;
        const key = 'run:' + detail.task.id;
        if (workflow && !this.workflow?.authorStartReady && !this.pending[key])
            return;
        const execution = workflow ? { executionMode: workflow.authority.executionMode, ...(workflow.authority.fakeScenario ? { fakeScenario: workflow.authority.fakeScenario } : {}) } : checked('task-fake') ? { executionMode: 'fake', fakeScenario: input('task-fake-scenario') } : {};
        const result = await this.mutate(key, '/tasks/' + detail.task.id + '/attempts', { requestId: requestId(), expectedRevision: detail.task.revision, taskVersion: detail.task.currentVersion, ...execution });
        if (result && this.selectedTaskId === detail.task.id)
            await this.openTask(detail.task.id);
    }
    async guidance() {
        const detail = this.detail;
        if (!detail || detail.task.graphRef)
            return;
        const key = 'guidance:' + detail.task.id;
        let p = this.pending[key];
        if (!p) {
            try {
                const target = input('guidance-harness');
                const pins = { ...detail.version.pins };
                if (target !== 'keep') {
                    if (target)
                        pins.harnessId = target;
                    else
                        delete pins.harnessId;
                }
                const modelMode = this.routing ? input('guidance-model-mode') : 'keep';
                if (modelMode === 'auto')
                    delete pins.model;
                if (modelMode === 'exact') {
                    if (!input('guidance-model'))
                        throw new ApiError('invalid_input', 'Supply the literal selector or choose Auto / keep captured.');
                    pins.model = input('guidance-model');
                }
                const routing = this.routing?.guidanceOptions();
                const guidanceSnapshot = this.routing?.guidanceSnapshot();
                p = { path: '/tasks/' + detail.task.id + '/guidance', method: 'PUT', body: { requestId: requestId(), expectedRevision: detail.task.revision, guidanceText: input('task-guidance'), ...(target !== 'keep' || modelMode !== 'keep' ? { pins } : {}), ...(routing ? { routing } : {}) }, ...(guidanceSnapshot ? { guidanceSnapshot } : {}) };
                this.pending[key] = p;
            }
            catch (error) {
                feedback('guidance-status', errorMessage(error), 'error');
                return;
            }
        }
        const clickedText = String(p.body['guidanceText']);
        const clickedTarget = p.guidanceSnapshot?.['guidance-harness'] ?? (p.body['pins'] ? p.body['pins'].harnessId ?? '' : 'keep');
        const result = await this.mutate(key, p.path, p.body);
        if (!result)
            return;
        if (this.guidanceEditors[detail.task.id] === clickedText) {
            delete this.guidanceEditors[detail.task.id];
            writeSession('taskGuidanceEditors', this.guidanceEditors);
        }
        if (this.guidanceHarnessEditors[detail.task.id] === clickedTarget) {
            delete this.guidanceHarnessEditors[detail.task.id];
            writeSession('taskGuidanceHarnessEditors', this.guidanceHarnessEditors);
        }
        if (p.guidanceSnapshot)
            this.routing?.finishGuidance(detail.task.id, p.guidanceSnapshot);
        if (this.selectedTaskId === detail.task.id) {
            this.guidanceEdited = input('task-guidance') !== clickedText;
            await this.openTask(detail.task.id);
            feedback('guidance-status', 'Guidance and explicit routing/target choices saved as a new immutable version. Prior outcomes, cumulative use and deadlines remain. Start another execution explicitly when ready.');
        }
        await this.refreshTasks();
    }
    async accept() {
        const d = this.detail;
        const a = this.attempt;
        const response = this.readArtifact;
        if (!d || !a || !response?.sha256 || !checked('reviewed-response'))
            return;
        const result = await this.mutate('accept:' + a.id, '/tasks/' + d.task.id + '/response-acceptance', { requestId: requestId(), expectedRevision: d.task.revision, taskVersion: d.task.currentVersion, attemptId: a.id, responseArtifactId: response.id, responseSha256: response.sha256, confirmation: 'reviewed_response_and_conditions' });
        if (result) {
            if (this.selectedTaskId === d.task.id)
                await this.openTask(d.task.id);
            await this.refreshTasks();
        }
    }
}
