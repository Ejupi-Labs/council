import { AgentsView } from './agents.js';
import { TasksView } from './tasks.js';
import { DraftsView } from './drafts.js';
import { RoutingView } from './routing.js';
import { WorkflowView } from './workflow.js';
import { GraphsView } from './graphs.js';
import { addFact, api, ApiError, consumeAccessToken, element, errorMessage, feedback, policyName, readSession, requestId, supportsManagedExecution, timeLabel, verifyOwner, writeSession } from './api.js';
function isAbsolute(path) {
    return /^[a-zA-Z]:[\\/]/.test(path) || path.startsWith('/') || /^\\\\[^\\]+\\[^\\]+/.test(path);
}
class Workspace {
    projects = new Map();
    drafts = new DraftsView();
    routing = new RoutingView();
    workflow = new WorkflowView();
    tasks = new TasksView(this.drafts, this.routing, this.workflow);
    graphs = new GraphsView();
    agents = new AgentsView(id => this.tasks.selectHarness(id), capabilities => this.tasks.setCapabilities(capabilities));
    path = element('project-path');
    projectSelect = element('project-select');
    openButton = element('open-project');
    policyMode = element('policy-mode');
    policyButton = element('save-policy');
    selectedId = null;
    projectCursor = null;
    selectionGeneration = 0;
    projectsGeneration = 0;
    detailGeneration = 0;
    connectionGeneration = 0;
    policyReadGeneration = 0;
    policyEditGeneration = 0;
    policy = null;
    opening = false;
    savingPolicy = false;
    ready = false;
    pendingOpen = readSession('pendingOpen');
    pendingPolicy = readSession('pendingPolicy');
    constructor() {
        this.graphs.onOpenTask = (id, ref) => { void this.tasks.openGraphTask(id, ref); };
        this.graphs.onTasksChanged = projectId => { if (projectId === this.selectedId)
            void this.tasks.refreshTasks(); };
        this.tasks.onOpenGraph = id => { void this.graphs.open(id); };
        this.routing.onTaskChanged = id => this.tasks.refreshAfterRoutingMutation(id);
        this.routing.onTaskObserved = () => this.tasks.refreshRoutingControls();
        this.workflow.onTaskChanged = id => this.tasks.refreshAfterRoutingMutation(id);
        this.workflow.onObserved = () => this.tasks.refreshRoutingControls();
        this.workflow.onReadArtifact = id => { void this.tasks.openEvidence(id); };
        this.workflow.onSelectAttempt = id => { void this.tasks.inspectAttempt(id); };
        if (this.pendingOpen)
            this.pendingOpen.unresolved = true;
        if (this.pendingPolicy) {
            this.pendingPolicy.unresolved = true;
            this.policyMode.value = this.pendingPolicy.body.mode;
            this.policyEditGeneration = this.pendingPolicy.editGeneration;
        }
        this.path.value = readSession('projectPath') ?? this.pendingOpen?.body.selectedPath ?? '';
        this.path.addEventListener('input', () => { writeSession('projectPath', this.path.value); });
        element('project-form').addEventListener('submit', event => { event.preventDefault(); void this.openProject(); });
        this.projectSelect.addEventListener('change', () => {
            if (this.projectSelect.value)
                this.selectProject(this.projectSelect.value);
        });
        element('more-projects').addEventListener('click', () => { void this.loadProjects(true); });
        this.policyMode.addEventListener('change', () => {
            this.policyEditGeneration++;
            feedback('policy-status', 'Requested preference changed locally. Save it to retain the choice.');
        });
        element('policy-form').addEventListener('submit', event => { event.preventDefault(); void this.savePolicy(); });
        element('reload-policy').addEventListener('click', () => { void this.loadPolicy(); });
        element('reconnect').addEventListener('click', () => { void this.connect(); });
        window.addEventListener('hashchange', () => {
            if (location.hash.startsWith('#token=')) {
                consumeAccessToken();
                void this.connect();
            }
        });
        for (const anchor of document.querySelectorAll('nav a')) {
            anchor.addEventListener('click', () => {
                for (const other of document.querySelectorAll('nav a'))
                    other.removeAttribute('aria-current');
                anchor.setAttribute('aria-current', 'page');
            });
        }
        if (consumeAccessToken())
            void this.connect();
        else {
            feedback('app-status', 'Local access is required. Open this workspace from “council ui” to obtain the private launch link.', 'error');
            element('reconnect').hidden = false;
            feedback('policy-status', 'Saved preferences are unavailable until local access is established.');
        }
    }
    async connect() {
        const generation = ++this.connectionGeneration;
        this.routing.setReady(false);
        this.workflow.setReady(false);
        this.graphs.disconnect();
        feedback('app-status', 'Verifying access to the local owner…');
        element('reconnect').disabled = true;
        try {
            const meta = await verifyOwner();
            if (generation !== this.connectionGeneration)
                return;
            this.ready = true;
            this.updateControls();
            this.routing.setReady(supportsManagedExecution(meta));
            this.workflow.setReady(supportsManagedExecution(meta));
            this.tasks.setOwner(meta);
            this.graphs.setOwner(meta);
            feedback('app-status', supportsManagedExecution(meta) ? 'Connected to the local workspace. Auto uses finite verified workflows. Up to two managed workers can run; context, account and task limits may reduce concurrency.' : 'Connected to the local workspace. This owner does not support managed execution.');
            // Recovery never replays a cached mutation automatically; each retry remains explicit.
            await Promise.allSettled([this.loadPolicy(), this.loadProjects(), this.agents.refresh(), this.routing.refresh(), this.workflow.refreshPreference()]);
        }
        catch (error) {
            if (generation !== this.connectionGeneration)
                return;
            this.ready = false;
            this.routing.setReady(false);
            this.workflow.setReady(false);
            this.tasks.disconnect();
            this.graphs.disconnect();
            this.updateControls();
            feedback('app-status', errorMessage(error) + ' Open “council ui” for a current private launch link.', 'error');
        }
        finally {
            if (generation === this.connectionGeneration) {
                element('reconnect').hidden = false;
                element('reconnect').disabled = false;
            }
        }
    }
    updateControls() {
        this.openButton.disabled = !this.ready || this.opening;
        this.openButton.textContent = this.opening ? 'Opening…' : this.pendingOpen ? 'Retry opening project' : 'Open project';
        this.projectSelect.disabled = !this.ready || this.projects.size === 0;
        this.policyMode.disabled = !this.ready || !this.policy;
        this.policyButton.disabled = !this.ready || !this.policy || this.savingPolicy;
        this.policyButton.textContent = this.savingPolicy ? 'Saving…' : this.pendingPolicy ? 'Retry preference save' : 'Save preference';
    }
    async loadProjects(append = false) {
        if (append && !this.projectCursor)
            return;
        const generation = ++this.projectsGeneration;
        const selectionGeneration = this.selectionGeneration;
        const cursor = append ? this.projectCursor : null;
        try {
            const { data } = await api('/projects' + (cursor ? '?cursor=' + encodeURIComponent(cursor) : ''));
            if (generation !== this.projectsGeneration)
                return;
            for (const project of data.items)
                this.projects.set(project.id, project);
            this.projectCursor = data.nextCursor;
            this.renderProjects();
            if (!this.selectedId && selectionGeneration === this.selectionGeneration) {
                const remembered = readSession('selectedProject');
                if (remembered && /^[a-f0-9]{32}$/.test(remembered))
                    this.selectProject(remembered);
                else if (data.items[0])
                    this.selectProject(data.items[0].id);
            }
            if (!this.projects.size)
                feedback('project-status', 'Open an existing Git working folder to begin.');
        }
        catch (error) {
            if (generation === this.projectsGeneration)
                feedback('project-status', errorMessage(error) + ' The selected project and local task text are retained.', 'error');
        }
    }
    renderProjects() {
        this.projectSelect.replaceChildren();
        const placeholder = document.createElement('option');
        placeholder.value = '';
        placeholder.textContent = 'Choose an opened project';
        this.projectSelect.append(placeholder);
        for (const project of this.projects.values()) {
            const option = document.createElement('option');
            option.value = project.id;
            option.textContent = project.resolvedPath;
            this.projectSelect.append(option);
        }
        this.projectSelect.value = this.selectedId ?? '';
        element('more-projects').hidden = this.projectCursor === null;
        this.updateControls();
    }
    selectProject(id) {
        this.selectionGeneration++;
        this.selectedId = id;
        this.projectSelect.value = id;
        writeSession('selectedProject', id);
        this.drafts.setProject(id);
        this.tasks.setProject(id);
        this.graphs.setProject(id);
        const project = this.projects.get(id);
        if (project)
            this.renderProject({ project, observation: null });
        void this.loadProject(id);
    }
    async loadProject(id) {
        const generation = ++this.detailGeneration;
        feedback('project-status', 'Inspecting the selected working folder…');
        try {
            const { data } = await api('/projects/' + id);
            if (generation !== this.detailGeneration || id !== this.selectedId)
                return;
            this.projects.set(data.project.id, data.project);
            this.renderProjects();
            this.renderProject(data);
            feedback('project-status', data.observationError
                ? data.observationError.message + ' The retained project and drafts remain available.'
                : 'Project selected. Existing files, staging and branch are unchanged.', data.observationError ? 'error' : 'neutral');
        }
        catch (error) {
            if (generation === this.detailGeneration && id === this.selectedId)
                feedback('project-status', errorMessage(error) + ' Keep this selection or choose another opened project.', 'error');
        }
    }
    renderProject(detail) {
        const { project, observation } = detail;
        const facts = element('project-details');
        facts.replaceChildren();
        const context = observation ?? project;
        addFact(facts, 'Selected folder', project.selectedPath);
        addFact(facts, 'Resolved working folder', context.resolvedPath);
        addFact(facts, 'Repository root', context.repositoryRoot);
        addFact(facts, observation ? 'Observed branch' : 'Admission branch (retained)', context.branch ?? 'Detached HEAD');
        addFact(facts, 'Commit', context.commit ?? 'No commit observed (unborn repository)');
        addFact(facts, observation ? 'Observed at' : 'Admission observation (not refreshed)', timeLabel(context.observedAt));
    }
    async openProject() {
        if (!this.ready || this.opening)
            return;
        if (!this.pendingOpen) {
            if (!isAbsolute(this.path.value)) {
                feedback('project-status', 'Enter the full absolute path to an existing Git working folder. Relative paths are not accepted by this field.', 'error');
                this.path.focus();
                return;
            }
            this.pendingOpen = {
                body: { requestId: requestId(), selectedPath: this.path.value, basePath: this.path.value },
                selectionGeneration: this.selectionGeneration,
            };
        }
        const pending = this.pendingOpen;
        const wasUnresolved = pending.unresolved === true;
        pending.unresolved = true;
        const selectionGeneration = pending.selectionGeneration;
        this.opening = true;
        writeSession('pendingOpen', pending);
        this.updateControls();
        feedback('project-status', 'Opening the existing working folder. No agent is being started.');
        try {
            const { data } = await api('/projects/open', 'POST', pending.body);
            this.pendingOpen = null;
            writeSession('pendingOpen', null);
            this.projects.set(data.project.id, data.project);
            this.renderProjects();
            if (selectionGeneration === this.selectionGeneration && this.path.value === pending.body.selectedPath) {
                this.selectionGeneration++;
                this.detailGeneration++;
                this.selectedId = data.project.id;
                writeSession('selectedProject', data.project.id);
                this.drafts.setProject(data.project.id);
                this.tasks.setProject(data.project.id);
                this.graphs.setProject(data.project.id);
                this.renderProjects();
                this.renderProject(data);
                feedback('project-status', 'Project opened. Existing files, staging and branch are unchanged.', 'success');
            }
            else
                feedback('project-status', 'Project opened and retained. Your newer selection or folder input has been kept.', 'success');
        }
        catch (error) {
            const uncertain = wasUnresolved || (error instanceof ApiError && error.uncertain);
            feedback('project-status', errorMessage(error, pending.body.requestId)
                + (uncertain ? ' Retry opening this same project to reconcile the outcome.' : ' Correct the path or prerequisite and try again.'), 'error');
            if (!uncertain) {
                this.pendingOpen = null;
                writeSession('pendingOpen', null);
            }
        }
        finally {
            this.opening = false;
            this.updateControls();
        }
    }
    async loadPolicy() {
        const generation = ++this.policyReadGeneration;
        const editGeneration = this.policyEditGeneration;
        try {
            const { data } = await api('/policy');
            if (generation !== this.policyReadGeneration)
                return;
            this.policy = data;
            this.tasks.setPolicy(data);
            this.graphs.setPolicy(data);
            if (editGeneration === this.policyEditGeneration && !this.pendingPolicy)
                this.policyMode.value = data.mode;
            feedback('policy-status', 'Saved requested preference: ' + policyName(data.mode) + '. Revision ' + data.revision
                + ' · ' + (data.selectionSource === 'default' ? 'initial default' : 'owner selected') + '.'
                + (this.pendingPolicy ? ' A pending preference save still needs an explicit retry.' : ' Effective permissions remain unknown.'));
            element('reload-policy').hidden = true;
            this.updateControls();
        }
        catch (error) {
            if (generation === this.policyReadGeneration) {
                feedback('policy-status', errorMessage(error) + ' Your selected preference is retained.', 'error');
                element('reload-policy').hidden = false;
            }
        }
    }
    async savePolicy() {
        if (!this.ready || !this.policy || this.savingPolicy)
            return;
        if (!this.pendingPolicy)
            this.pendingPolicy = {
                body: { requestId: requestId(), expectedRevision: this.policy.revision, mode: this.policyMode.value },
                editGeneration: this.policyEditGeneration,
            };
        const pending = this.pendingPolicy;
        const wasUnresolved = pending.unresolved === true;
        pending.unresolved = true;
        this.policyReadGeneration++;
        this.savingPolicy = true;
        writeSession('pendingPolicy', pending);
        this.updateControls();
        feedback('policy-status', 'Saving the requested operating preference…');
        try {
            const { data } = await api('/policy', 'PUT', pending.body);
            this.policyReadGeneration++;
            this.policy = data;
            this.tasks.setPolicy(data);
            this.graphs.setPolicy(data);
            this.pendingPolicy = null;
            writeSession('pendingPolicy', null);
            if (this.policyEditGeneration === pending.editGeneration)
                this.policyMode.value = data.mode;
            feedback('policy-status', 'Saved requested preference: ' + policyName(data.mode) + '. Revision ' + data.revision + '.'
                + (this.policyMode.value !== data.mode ? ' Your newer local choice is retained and is not saved yet.' : ' Effective permissions remain unknown.'), 'success');
            element('reload-policy').hidden = true;
        }
        catch (error) {
            const uncertain = wasUnresolved || (error instanceof ApiError && error.uncertain);
            feedback('policy-status', errorMessage(error, pending.body.requestId)
                + (uncertain ? ' Retry this same preference save to reconcile its outcome.' : ' Your local choice is retained.'), 'error');
            if (!uncertain) {
                this.pendingPolicy = null;
                writeSession('pendingPolicy', null);
            }
            element('reload-policy').hidden = uncertain;
        }
        finally {
            this.savingPolicy = false;
            this.updateControls();
        }
    }
}
new Workspace();
