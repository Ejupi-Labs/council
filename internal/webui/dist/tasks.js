import { addFact, api, ApiError, element, errorMessage, feedback, policyName, readSession, requestId, supportsManagedExecution, timeLabel, writeSession } from './api.js';
import { appendConnectionFacts, factLabel, harnessName, renderNativeModels, usageLabel } from './native-observations.js';
const input = (id) => element(id).value;
const checked = (id) => element(id).checked;
function label(attempt) {
    if (attempt.outcome === 'succeeded')
        return 'Verification pending';
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
    projectId = null;
    policy = null;
    available = false;
    admission = 'open';
    capabilities = null;
    busy = false;
    projectGeneration = 0;
    detailGeneration = 0;
    attemptGeneration = 0;
    artifactGeneration = 0;
    listGeneration = 0;
    detail = null;
    attempt = null;
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
    constructor(drafts) {
        this.drafts = drafts;
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
        element('composer-target').textContent = harnessName(input('task-harness'));
        element('task-harness').addEventListener('change', () => {
            writeSession('taskHarnessChoice', input('task-harness'));
            element('composer-target').textContent = harnessName(input('task-harness'));
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
        if (meta.admission && meta.admission.state !== 'open')
            feedback('execution-status', 'Dispatch is ' + meta.admission.state + (meta.admission.incidentId ? ' · incident ' + meta.admission.incidentId : '') + '. Inspect retained work; no automatic replay occurs.' + (meta.admission.incident ? ' ' + meta.admission.incident.code + ': ' + meta.admission.incident.message + ' Preserve the workspace and evidence. Incident recovery is unavailable in this preview.' : ''), 'error');
        if (!this.available)
            feedback('execution-status', 'This owner does not advertise managed execution 003-v1. Reopen with a compatible Council owner.', 'error');
        this.controls();
        if (this.available && this.projectId)
            void this.refreshTasks();
    }
    disconnect() { this.available = false; this.attemptGeneration++; this.controls(); }
    setPolicy(policy) { this.policy = policy; this.controls(); }
    setProject(id) {
        if (id === this.projectId)
            return;
        this.projectId = id;
        this.projectGeneration++;
        this.detailGeneration++;
        this.attemptGeneration++;
        this.clearArtifact();
        this.selectedTaskId = null;
        this.detail = null;
        this.attempt = null;
        this.tasks = [];
        this.taskCursor = null;
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
        const stopKey = a ? 'stop:' + a.id : '';
        const runKey = this.detail ? 'run:' + this.detail.task.id : '';
        element('stop-attempt').disabled = !this.available || !a || (!!a.settledAt && !this.pending[stopKey]) || this.mutationBusy.has(stopKey);
        element('stop-attempt').textContent = this.pending[stopKey] ? 'Retry pending Stop' : a?.stop ? 'Stop accepted · refresh cleanup' : 'Stop';
        element('run-attempt').disabled = !this.available || !this.detail || (!!this.detail.latestAttempt && !this.detail.latestAttempt.settledAt && !this.pending[runKey]) || this.mutationBusy.has(runKey) || this.admission !== 'open' && !this.pending[runKey];
        element('run-attempt').textContent = this.pending[runKey] ? 'Retry pending execution' : 'Start another execution';
        element('save-guidance').textContent = this.detail && this.pending['guidance:' + this.detail.task.id] ? 'Retry pending guidance save' : 'Save guidance';
        element('save-guidance').disabled = !this.available || !this.detail || this.mutationBusy.has('guidance:' + this.detail.task.id);
        element('read-response').disabled = !a?.responseArtifactId;
        const v = this.detail?.version;
        const response = this.readArtifact;
        const eligible = !!a && current && a.id === this.detail?.task.latestAttemptId && a.outcome === 'succeeded' && a.cleanup.state === 'verified' && a.evidenceStatus === 'complete' && !a.authoredDelta && v?.acceptance.deliverable === 'response_only' && v.acceptance.requiredEvidence.length === 0 && response?.id === a.responseArtifactId && response.closed && response.completeness === 'complete' && !!response.sha256 && !this.detail?.acceptance;
        element('response-acceptance').hidden = !eligible;
        element('accept-response').disabled = !this.available || !eligible || !checked('reviewed-response') || this.mutationBusy.has('accept:' + a?.id);
    }
    startOptions() {
        const timeout = input('task-timeout');
        if (timeout && (!/^\d+$/.test(timeout) || Number(timeout) < 1 || Number(timeout) > 86400))
            throw new ApiError('invalid_input', 'Set a deadline from 1 through 86400 seconds or leave it at the default.');
        const required = input('task-required-evidence').split(/\r?\n/).map(v => v.trim()).filter(Boolean);
        if (required.some(v => !['check', 'independent_review', 'local_integration', 'external_delivery'].includes(v)) || new Set(required).size !== required.length)
            throw new ApiError('invalid_input', 'Required evidence entries are unique values: check, independent_review, local_integration, external_delivery.');
        return {
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
            feedback('execution-status', 'Admitted task ' + data.task.id + ' · ' + data.attempt.phase + '. One explicit worker; completion is not yet accepted.', 'success');
            if (data.task.projectId === this.projectId) {
                await this.refreshTasks();
                // A late acknowledgement must not replace a selection made during admission or refresh.
                if (detailGeneration === this.detailGeneration && projectGeneration === this.projectGeneration && selectedTaskId === this.selectedTaskId)
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
    async refreshTasks(append = false) {
        const project = this.projectId;
        if (!project || !this.available || append && !this.taskCursor)
            return;
        const generation = ++this.listGeneration;
        try {
            const { data } = await api('/tasks?projectId=' + project + (append && this.taskCursor ? '&cursor=' + encodeURIComponent(this.taskCursor) : ''));
            if (generation !== this.listGeneration || project !== this.projectId)
                return;
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
    async openTask(id) {
        const generation = ++this.detailGeneration;
        const projectGeneration = this.projectGeneration;
        const changed = id !== this.selectedTaskId;
        if (changed) {
            this.selectedTaskId = id;
            this.attemptGeneration++;
            this.clearArtifact();
            this.guidanceEdited = false;
            this.detail = null;
            this.attempt = null;
            element('task-detail').hidden = true;
            element('execution-link').hidden = true;
            this.controls();
        }
        try {
            const { data } = await api('/tasks/' + id);
            if (generation !== this.detailGeneration || projectGeneration !== this.projectGeneration || id !== this.selectedTaskId)
                return;
            this.detail = data;
            element('task-detail').hidden = false;
            element('execution-link').hidden = false;
            element('task-detail-heading').textContent = 'Task ' + id.slice(0, 8) + ' · version ' + data.task.currentVersion;
            this.renderTaskFacts(data);
            if (data.latestAttempt)
                void this.selectAttempt(data.latestAttempt);
            else {
                this.attempt = null;
                this.controls();
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
        element('task-detail-heading').textContent = 'Task ' + data.task.id.slice(0, 8) + ' · version ' + data.task.currentVersion;
        const facts = element('task-facts');
        facts.replaceChildren();
        addFact(facts, 'Task identity', data.task.id);
        addFact(facts, 'Captured requested preference', policyName(data.version.requestedPolicy.mode) + ' · revision ' + data.version.requestedPolicy.revision);
        addFact(facts, 'Captured harness choice', harnessName(data.version.pins.harnessId));
        addFact(facts, 'Requested model', data.version.pins.model || 'Native configured default');
        addFact(facts, 'Current task version', data.task.currentVersion);
        element('guidance-keep-target').textContent = 'Keep captured target · ' + harnessName(data.version.pins.harnessId);
        element('guidance-harness').value = this.guidanceHarnessEditors[data.task.id] ?? 'keep';
        addFact(facts, 'Acceptance', data.acceptance ? 'Reviewed response accepted · ' + timeLabel(data.acceptance.acceptedAt) : 'Not accepted');
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
            row(list, attempt.id.slice(0, 8) + ' · task version ' + attempt.taskVersion + ' · ' + label(attempt) + ' · ' + (attempt.executionMode === 'fake' ? 'Local fake · ' : '') + attempt.connectionId, () => { void this.selectAttempt(attempt); });
            const usage = attempt.targetObservations.usage;
            for (const count of [usage.inputTokens, usage.outputTokens]) {
                if (count !== null && /^\d+$/.test(count))
                    total += BigInt(count);
                else
                    unknown = true;
            }
        }
        element('more-attempts').hidden = !this.attemptCursor;
        element('usage-total').textContent = 'Reported input + output across loaded executions (native scopes may differ): ' + total + ' tokens.' + (unknown ? ' Some usage is unknown.' : '') + (this.attemptCursor ? ' More history is available.' : '') + ' Cached input is not added twice. Whole-tree previews are not added to these counters. Native call counts, cost and allowance remain unknown.';
    }
    async selectAttempt(attempt) {
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
        const a = this.attempt;
        if (!a)
            return;
        const usage = a.targetObservations.usage;
        feedback('attempt-status', (a.executionMode === 'fake' ? 'LOCAL FAKE · ' : '') + label(a) + (a.taskVersion !== this.detail?.task.currentVersion ? ' · Historical task version; current guidance needs an explicit new execution.' : '')
            + (a.primaryFailure ? ' · ' + a.primaryFailure.code + ': ' + a.primaryFailure.message : '') + (a.stop ? ' · Stop accepted; cleanup remains ' + a.cleanup.state + '.' : '')
            + (a.candidateId && a.authoredDelta ? ' · Candidate retained. Verification pending; integration not performed.' : a.candidateId ? ' · No authored source or staging changes.' : ''), a.primaryFailure || a.cleanup.state === 'unknown' ? 'error' : 'neutral');
        const summary = element('attempt-summary');
        summary.replaceChildren();
        addFact(summary, 'Cleanup and evidence', a.cleanup.state.replaceAll('_', ' ') + ' · ' + a.evidenceStatus);
        addFact(summary, 'Deadline', timeLabel(a.deadlineAt) + ' · ' + a.remainingSeconds + ' seconds remaining at ' + timeLabel(a.observedAt));
        addFact(summary, 'Captured connection', a.connectionId + ' · ' + (a.executionMode === 'fake' ? 'Local fake' : 'Native execution'));
        addFact(summary, 'Reported usage', usageLabel(usage));
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
        addFact(facts, 'Requested model selector', a.requestedTarget.model || 'Native configured default');
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
            addFact(facts, 'Candidate changes', a.candidate.hasDelta ? 'Authored changes retained.' + (a.candidate.hasIndexDelta ? ' Staged index changes are included.' : '') : 'No authored source or staging changes.');
            addFact(facts, 'Candidate verification and integration', a.candidate.verification.replaceAll('_', ' ') + ' · ' + a.candidate.integration.replaceAll('_', ' '));
        }
        if (a.incident)
            addFact(facts, 'Retained incident', a.incident.code + ': ' + a.incident.message + ' Preserve this workspace and evidence. Inspect the incident before an explicit recovery; this preview does not repair uncertain ownership or replay work.');
        const permissions = a.targetObservations.permissions;
        addFact(facts, 'Permissions', 'Requested ' + permissions.requestedMode + ' · configured ' + (permissions.configuredMode ?? 'unknown') + ' · native reported ' + (permissions.nativeReportedMode ?? 'unknown'));
        if (permissions.limitations.length)
            addFact(facts, 'Permission scope', permissions.limitations.join(' '));
        addFact(facts, 'Reported usage', usageLabel(usage));
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
                    const { data: detail } = await api('/tasks/' + data.attempt.taskId);
                    if (generation !== this.attemptGeneration || this.selectedTaskId !== detail.task.id)
                        return;
                    // Current-version metadata refresh does not rewrite the guidance editor.
                    this.detail = detail;
                    this.renderTaskFacts(detail);
                    this.renderAttempt();
                    const { data: meta } = await api('/meta');
                    if (generation === this.attemptGeneration)
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
        const pending = this.pending[key] ?? { path, method: 'POST', body };
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
        const a = this.attempt;
        if (!a)
            return;
        const result = await this.mutate('stop:' + a.id, '/attempts/' + a.id + '/stop', { requestId: requestId() });
        if (!result || a.id !== this.attempt?.id)
            return;
        feedback('attempt-status', result.stopAccepted ? 'Stop accepted. Waiting for actual process, output and cleanup settlement.' : 'Execution was already settled. Its outcome is retained.');
        try {
            const { data } = await api('/attempts/' + a.id);
            if (a.id === this.attempt?.id)
                void this.selectAttempt(data);
        }
        catch (error) {
            feedback('attempt-status', errorMessage(error) + ' The Stop acknowledgement is retained; refresh to observe actual cleanup.', 'error');
        }
    }
    async run() {
        const detail = this.detail;
        if (!detail)
            return;
        const result = await this.mutate('run:' + detail.task.id, '/tasks/' + detail.task.id + '/attempts', { requestId: requestId(), expectedRevision: detail.task.revision, taskVersion: detail.task.currentVersion,
            ...(checked('task-fake') ? { executionMode: 'fake', fakeScenario: input('task-fake-scenario') } : {}) });
        if (result && this.selectedTaskId === detail.task.id)
            await this.openTask(detail.task.id);
    }
    async guidance() {
        const detail = this.detail;
        if (!detail)
            return;
        const key = 'guidance:' + detail.task.id;
        const target = input('guidance-harness');
        const pins = { ...detail.version.pins };
        if (target !== 'keep') {
            if (target)
                pins.harnessId = target;
            else
                delete pins.harnessId;
        }
        const p = this.pending[key] ?? { path: '/tasks/' + detail.task.id + '/guidance', method: 'PUT', body: { requestId: requestId(), expectedRevision: detail.task.revision, guidanceText: input('task-guidance'), ...(target !== 'keep' ? { pins } : {}) } };
        this.pending[key] = p;
        const clickedText = String(p.body['guidanceText']);
        const clickedTarget = p.body['pins'] ? p.body['pins'].harnessId ?? '' : 'keep';
        const result = await this.mutate(key, p.path, p.body);
        if (result && this.selectedTaskId === detail.task.id) {
            this.guidanceEdited = input('task-guidance') !== clickedText;
            if (!this.guidanceEdited) {
                delete this.guidanceEditors[detail.task.id];
                writeSession('taskGuidanceEditors', this.guidanceEditors);
            }
            if (input('guidance-harness') === clickedTarget) {
                delete this.guidanceHarnessEditors[detail.task.id];
                writeSession('taskGuidanceHarnessEditors', this.guidanceHarnessEditors);
            }
            await this.openTask(detail.task.id);
            feedback('guidance-status', 'Guidance and any explicit target change saved as a new immutable version. Prior outcomes remain historical. Start another execution explicitly when ready.');
        }
        if (result)
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
