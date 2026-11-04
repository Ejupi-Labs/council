import { randomUUID } from 'node:crypto';
import { setTimeout as delay } from 'node:timers/promises';
import { RoomStore } from './room-store.js';
import { objectSchema, parseSchema } from './schema.js';
import { REASONING_EFFORTS } from './types.js';
import { BUILT_IN_SKILL_IDS, validateSkills } from './skills.js';
const text = (maximum) => ({ type: 'string', minLength: 1, ...(maximum === undefined ? {} : { maxLength: maximum }), pattern: '^[^\\u0000]+$' });
const taskSchema = objectSchema({ title: text(200), brief: text(), permission: { type: 'string', enum: ['read', 'write'] },
    dependsOn: { type: 'array', items: text(100) } }, ['title', 'brief', 'permission']);
const memberSchema = objectSchema({ agent: text(64), role: { type: 'string', enum: ['lead', 'worker', 'advisor'] },
    weight: { type: 'number', exclusiveMinimum: 0 }, model: text(200),
    effort: { type: 'string', enum: REASONING_EFFORTS }, advisor: text(64), advisorMaxRequests: { type: 'integer', minimum: 1 },
    skills: { type: 'array', items: { type: 'string', enum: BUILT_IN_SKILL_IDS } } }, ['agent', 'role']);
const roomSchema = objectSchema({ title: text(200), brief: text(), members: { type: 'array', items: memberSchema, minItems: 1 },
    tasks: { type: 'array', items: taskSchema, default: [] },
    rounds: { type: 'integer', minimum: 1, default: 1 }, maxCalls: { type: 'integer', minimum: 1 },
    autoExecute: { type: 'boolean', default: true }, contextLimit: { type: 'integer', minimum: 1 } }, ['title', 'brief', 'members']);
const ballotSchema = objectSchema({ vote: { type: 'string', enum: ['approve', 'revise', 'abstain'] }, rationale: text(),
    assignments: { type: 'object', additionalProperties: text(64) } }, ['vote', 'rationale', 'assignments']);
class InstructionsChanged extends Error {
    constructor() { super('Project manager instructions changed. In-flight results are retained; start a new discussion before dispatching further tasks.'); }
}
const active = (room) => room.status === 'discussing' || room.status === 'running';
function nonempty(value, label, bytes) {
    if (!value.trim() || bytes !== undefined && Buffer.byteLength(value) > bytes)
        throw new Error(`${label} must contain non-whitespace text${bytes === undefined ? '.' : ` within ${bytes} UTF-8 bytes.`}`);
}
function excerpt(value, maximum = 2000) {
    if (value.length <= maximum)
        return value;
    const suffix = '\n[Preview truncated; full evidence is retained in the linked run.]';
    let result = value.slice(0, Math.max(0, maximum - suffix.length));
    if (/[\uD800-\uDBFF]$/.test(result))
        result = result.slice(0, -1);
    return result + suffix;
}
function message(author, kind, body, runId) {
    return { id: randomUUID(), author, kind, body, createdAt: new Date().toISOString(), ...(runId ? { runId } : {}) };
}
function pending(room) { return room.tasks.filter(task => task.status === 'pending' || task.status === 'assigned'); }
function voters(room) { return room.members.filter(member => member.role !== 'advisor'); }
function decimalWeight(weight) {
    const [coefficient, exponent = '0'] = weight.toString().split('e');
    const [whole, fraction = ''] = coefficient.split('.');
    return { coefficient: BigInt(whole + fraction), exponent: Number(exponent) - fraction.length };
}
function displayedWeight(units, exponent) {
    if (units === 0n)
        return 0;
    const digits = units.toString(), fraction = digits.slice(1).replace(/0+$/, '');
    const scientific = `${digits[0]}${fraction ? `.${fraction}` : ''}e${exponent + digits.length - 1}`;
    const number = Number(scientific);
    return Number.isFinite(number) ? number : scientific;
}
/** JSON.parse validates grammar first; reject ambiguous keys before trusting a ballot. */
function rejectDuplicateKeys(source) {
    const objects = [];
    for (let index = 0; index < source.length; index++) {
        if (source[index] === '{')
            objects.push(new Set());
        else if (source[index] === '}')
            objects.pop();
        else if (source[index] === '"') {
            const start = index;
            while (++index < source.length) {
                if (source[index] === '\\')
                    index++;
                else if (source[index] === '"')
                    break;
            }
            let next = index + 1;
            while (/\s/.test(source[next] ?? ''))
                next++;
            if (source[next] === ':') {
                const key = JSON.parse(source.slice(start, index + 1));
                const scope = objects.at(-1);
                if (scope.has(key))
                    throw new Error('Ballot contains duplicate JSON object keys.');
                scope.add(key);
            }
        }
    }
}
/** A durable human-managed task board; provider calls use the existing workflow service. */
export class RoomManager {
    service;
    store;
    owner = randomUUID();
    jobs = new Map();
    listeners = new Set();
    emergency = new Map();
    closing = false;
    fault;
    closePromise;
    shutdown = new AbortController();
    constructor(service) {
        this.service = service;
        this.store = new RoomStore(service.config.stateDir);
    }
    subscribe(listener) { this.listeners.add(listener); return () => { this.listeners.delete(listener); }; }
    list() {
        return this.store.list().map(summary => {
            const room = this.emergency.get(summary.id);
            return room ? { ...summary, status: room.status, revision: room.revision, updatedAt: room.updatedAt } : summary;
        });
    }
    get(id) {
        if (typeof id !== 'string' || !id || id.length > 100)
            throw new Error('Room not found (404).');
        const room = this.emergency.get(id) ?? this.store.get(id);
        if (!room)
            throw new Error(`Room not found (404): ${id}`);
        return structuredClone(room);
    }
    messagePage(id, limit = 100, cursor) { this.get(id); return this.store.messagePage(id, limit, cursor); }
    create(value) {
        this.available();
        const normalized = value && typeof value === 'object' ? { ...value,
            ...(value.contextLimit === null ? { contextLimit: undefined } : {}),
            members: Array.isArray(value.members) ? value.members.map(member => member && typeof member === 'object' && !Array.isArray(member)
                ? { ...member, weight: member.weight === undefined ? this.service.config.agents[member.agent]?.authority ?? 1 : member.weight } : member) : value.members } : value;
        const input = parseSchema(roomSchema, normalized, 'room');
        if (!Number.isSafeInteger(input.rounds) || input.maxCalls !== undefined && !Number.isSafeInteger(input.maxCalls))
            throw new Error('Room rounds and call budget must be positive safe integers.');
        nonempty(input.title, 'Room title', 800);
        nonempty(input.brief, 'Room brief');
        this.validateMembers(input.members);
        const now = new Date().toISOString();
        const room = { id: randomUUID(), title: input.title, brief: input.brief, members: input.members, backend: this.service.config.backend,
            status: 'idle', tasks: input.tasks.map((task, index) => this.task(task, `task-${index + 1}`)),
            messages: [message('system', 'system', 'The project manager owns task briefs and permissions. Discussion is read-only; approved work uses the original task permissions.')],
            rounds: input.rounds, maxCalls: input.maxCalls ?? this.service.config.maxCalls, autoExecute: input.autoExecute, contextLimit: input.contextLimit ?? null,
            callsUsed: 0, revision: 1, instructionRevision: 0, createdAt: now, updatedAt: now };
        this.validateDependencies(room.tasks);
        this.store.insert(room);
        this.publish(room);
        return structuredClone(room);
    }
    postMessage(id, value) {
        this.available();
        const input = parseSchema(objectSchema({ author: text(80), body: text() }, ['author', 'body']), value, 'message');
        nonempty(input.author, 'Message author', 320);
        nonempty(input.body, 'Message body');
        return this.change(id, room => {
            room.messages.push(message(input.author, 'human', input.body));
            room.instructionRevision = (room.instructionRevision ?? 0) + 1;
            if (room.status === 'ready') {
                this.invalidateDecision(room);
                room.status = 'needs_attention';
                room.error = new InstructionsChanged().message;
            }
            else if (active(room))
                room.error = new InstructionsChanged().message;
        });
    }
    addTask(id, value) {
        this.available();
        return this.change(id, room => {
            if (room.status === 'failed' || room.status === 'cancelled')
                throw new Error('Failed or cancelled rooms cannot accept new tasks. Create a new room; unfinished writes will not be replayed.');
            const task = this.task(value, `task-${room.tasks.length + 1}`);
            room.tasks.push(task);
            this.validateDependencies(room.tasks);
            room.instructionRevision = (room.instructionRevision ?? 0) + 1;
            room.status = 'idle';
            delete room.decision;
            delete room.error;
            room.messages.push(message('project-manager', 'task', `Added ${task.id}: ${task.title} (${task.permission}).`));
        }, { requireIdle: true });
    }
    startDiscussion(id) {
        this.available();
        if (this.jobs.size >= 20)
            throw new Error('This room manager already owns 20 active jobs.');
        const initial = this.get(id);
        this.assertBackend(initial);
        if (!['idle', 'ready', 'needs_attention'].includes(initial.status))
            throw new Error('This room cannot start another discussion in its current state.');
        if (!pending(initial).length)
            throw new Error('Add at least one pending task before starting discussion.');
        this.reservePlan(initial, true);
        const room = this.change(id, value => {
            this.assertBackend(value);
            if (!['idle', 'ready', 'needs_attention'].includes(value.status))
                throw new Error('A room discussion is already active or terminal.');
            this.reservePlan(value, true);
            value.status = 'discussing';
            delete value.decision;
            delete value.error;
            for (const task of pending(value)) {
                task.status = 'pending';
                delete task.assignee;
            }
            value.messages.push(message('system', 'system', 'Read-only discussion queued. Every voting member must return a valid ballot before a decision can execute.'));
        }, { claim: { id: this.owner, pid: process.pid }, requireIdle: true });
        this.queue(id, job => this.discuss(id, job), room);
        return room;
    }
    execute(id) {
        this.available();
        if (this.jobs.size >= 20)
            throw new Error('This room manager already owns 20 active jobs.');
        this.assertBackend(this.get(id));
        const room = this.change(id, value => {
            this.assertBackend(value);
            if (value.status !== 'ready' || !value.decision?.approved)
                throw new Error('Only an approved ready room can execute.');
            if (value.decision.instructionRevision === undefined || value.decision.instructionRevision !== (value.instructionRevision ?? 0))
                throw new Error('Room approval does not cover the current project manager instructions. Start a new discussion before executing.');
            this.reservePlan(value, false);
            value.status = 'running';
            delete value.error;
        }, { claim: { id: this.owner, pid: process.pid }, requireIdle: true });
        this.queue(id, job => this.executeTasks(id, job), room);
        return room;
    }
    cancel(id) {
        const requested = this.store.requestCancel(id);
        const job = this.jobs.get(id);
        if (job)
            this.abort(job);
        this.publish(requested);
        return requested;
    }
    async wait(id, timeoutMs = 600000, signal) {
        if (!Number.isFinite(timeoutMs) || timeoutMs < 0 || timeoutMs > 600000)
            throw new Error('Room wait timeout must be from 0 to 600000 ms.');
        const cancellation = signal ? AbortSignal.any([signal, this.shutdown.signal]) : this.shutdown.signal;
        cancellation.throwIfAborted();
        const initial = this.get(id);
        if (!active(initial) || timeoutMs === 0)
            return initial;
        const job = this.jobs.get(id);
        if (job) {
            const stop = AbortSignal.any([cancellation, job.settled.signal]);
            try {
                await delay(timeoutMs, undefined, { signal: stop });
            }
            catch (error) {
                cancellation.throwIfAborted();
                if (!job.settled.signal.aborted)
                    throw error;
            }
            cancellation.throwIfAborted();
            return this.get(id);
        }
        const deadline = performance.now() + timeoutMs;
        for (;;) {
            await delay(Math.max(1, Math.min(100, deadline - performance.now())), undefined, { signal: cancellation });
            cancellation.throwIfAborted();
            this.store.recover();
            const room = this.get(id);
            if (!active(room) || performance.now() >= deadline)
                return room;
        }
    }
    close() {
        if (this.closePromise)
            return this.closePromise;
        this.closing = true;
        this.shutdown.abort(new DOMException('Room manager is shutting down.', 'AbortError'));
        this.closePromise = (async () => {
            for (const [id, job] of this.jobs) {
                try {
                    this.store.requestCancel(id, { activeOnly: true });
                }
                catch { /* Owned subprocesses must still drain when storage is unavailable. */ }
                this.abort(job);
            }
            await Promise.allSettled([...this.jobs.values()].map(job => job.done));
            let failure;
            const deadline = performance.now() + 5000;
            try {
                for (const [id, saved] of this.emergency) {
                    if (performance.now() >= deadline) {
                        failure ??= new Error('Failed room outcomes could not be durably finalized within the shutdown storage deadline. Inspect local state before replacing those tasks.');
                        break;
                    }
                    for (;;) {
                        try {
                            const repaired = this.store.mutate(id, room => {
                                // Preserve current human guidance and all append-only history; only
                                // settle work this manager already drained, never replay a workflow.
                                room.status = 'failed';
                                room.error = saved.error ?? 'Room persistence failed; owned work was drained.';
                                room.callsUsed = Math.max(room.callsUsed, saved.callsUsed);
                                for (const task of room.tasks) {
                                    const evidence = saved.tasks.find(candidate => candidate.id === task.id);
                                    if (evidence?.runId && !task.runId)
                                        task.runId = evidence.runId;
                                    if (evidence?.resultPreview && !task.resultPreview)
                                        task.resultPreview = evidence.resultPreview;
                                    if (['pending', 'assigned', 'running'].includes(task.status)) {
                                        task.status = task.status === 'running' ? 'failed' : 'cancelled';
                                        task.error = room.error;
                                    }
                                }
                                room.messages.push(...saved.messages.filter(entry => entry.id !== 'history-preview'));
                                room.messages.push(message('system', 'system', 'The failed room outcome was recovered after owned work drained. No tasks were replayed.'));
                            }, { owner: this.owner, release: true, preserveFailure: true });
                            this.emergency.delete(id);
                            this.publish(repaired);
                            break;
                        }
                        catch (error) {
                            const code = error && typeof error === 'object' && 'errcode' in error ? error.errcode : undefined;
                            const remaining = deadline - performance.now();
                            if (typeof code === 'number' && ((code & 0xff) === 5 || (code & 0xff) === 6) && remaining > 0) {
                                await delay(Math.min(50, remaining));
                                continue;
                            }
                            failure ??= new Error(`Failed room ${id} could not be durably finalized during shutdown. Its last durable state may remain active; inspect local storage before replacing the task.`, { cause: error });
                            break;
                        }
                    }
                }
            }
            finally {
                this.listeners.clear();
                try {
                    this.store.close();
                }
                catch (error) {
                    failure ??= error;
                }
            }
            if (failure)
                throw failure;
        })();
        return this.closePromise;
    }
    available() {
        if (this.closing)
            throw new Error('Rooms are shutting down.');
        if (this.fault)
            throw new Error(this.fault);
    }
    assertBackend(room) {
        if (!room.backend)
            throw new Error('Room backend provenance is unknown. Recreate this legacy room with the intended backend before starting provider work.');
        if (room.backend !== this.service.config.backend)
            throw new Error(`Room backend ${room.backend} differs from this service backend ${this.service.config.backend}. Open the matching backend or recreate the room with the intended backend.`);
    }
    assertInstructions(room, job) {
        if ((room.instructionRevision ?? 0) !== job.instructionRevision)
            throw new InstructionsChanged();
    }
    invalidateDecision(room) {
        delete room.decision;
        for (const task of room.tasks)
            if (task.status === 'assigned') {
                task.status = 'pending';
                delete task.assignee;
            }
    }
    publish(room) { for (const listener of this.listeners) {
        try {
            listener(structuredClone(room));
        }
        catch { /* A disconnected UI cannot alter room persistence. */ }
    } }
    change(id, action, options) {
        const room = this.store.mutate(id, action, options);
        const job = this.jobs.get(id);
        if (job)
            job.snapshot = structuredClone(room);
        this.publish(room);
        return room;
    }
    own(id, action, release = false) {
        return this.change(id, action, { owner: this.owner, ...(release ? { release: true } : {}) });
    }
    abort(job) {
        job.cancelRequested = true;
        job.controller.abort(new Error('Room cancelled.'));
        for (const run of job.runs) {
            try {
                this.service.cancel(run);
            }
            catch { /* Completion will retain the authoritative workflow failure. */ }
        }
    }
    check(id, job) {
        if (this.store.cancelled(id, this.owner))
            this.abort(job);
        job.lastStateSuccess = performance.now();
        job.controller.signal.throwIfAborted();
    }
    queue(id, operation, snapshot) {
        const job = { controller: new AbortController(), settled: new AbortController(), runs: new Set(), done: Promise.resolve(), cancelRequested: false,
            lastStateSuccess: performance.now(), snapshot: structuredClone(snapshot), instructionRevision: snapshot.instructionRevision ?? 0 };
        this.jobs.set(id, job);
        const poll = setInterval(() => {
            try {
                this.check(id, job);
            }
            catch (error) {
                if (job.cancelRequested && job.controller.signal.aborted && error === job.controller.signal.reason)
                    return;
                const code = error && typeof error === 'object' && 'errcode' in error ? error.errcode : undefined;
                if (typeof code === 'number' && ((code & 0xff) === 5 || (code & 0xff) === 6) && performance.now() - job.lastStateSuccess < 5000)
                    return;
                job.failure = error;
                if (!job.controller.signal.aborted)
                    job.controller.abort(error);
                this.abortRuns(job);
            }
        }, 100);
        poll.unref();
        job.done = Promise.resolve().then(() => operation(job)).catch(async (error) => {
            const cancelled = job.cancelRequested && error === job.controller.signal.reason && !job.failure;
            const instructionsChanged = error instanceof InstructionsChanged && !job.failure && !cancelled;
            if (!instructionsChanged)
                this.abortRuns(job);
            await Promise.allSettled([...job.runs].map(run => this.service.completion(run)));
            try {
                this.own(id, room => {
                    if (instructionsChanged) {
                        this.invalidateDecision(room);
                        room.status = 'needs_attention';
                        room.error = error.message;
                        room.messages.push(message('system', 'decision', error.message));
                        return;
                    }
                    room.status = cancelled ? 'cancelled' : 'failed';
                    room.error = cancelled ? 'Room cancelled; partial results and isolated worktrees are retained.' : excerpt(error instanceof Error ? error.message : 'Room execution failed.');
                    for (const task of room.tasks)
                        if (task.status === 'running' || task.status === 'assigned' || task.status === 'pending') {
                            task.status = cancelled || task.status !== 'running' ? 'cancelled' : 'failed';
                            task.error = room.error;
                        }
                    room.messages.push(message('system', 'system', room.error));
                }, true);
            }
            catch {
                this.fault = 'Room persistence failed. Inspect local state before starting new room work.';
                const room = structuredClone(job.snapshot);
                room.status = 'failed';
                room.error = this.fault;
                room.revision++;
                room.updatedAt = new Date().toISOString();
                for (const task of room.tasks)
                    if (['pending', 'assigned', 'running'].includes(task.status)) {
                        task.status = task.status === 'running' ? 'failed' : 'cancelled';
                        task.error = this.fault;
                    }
                room.messages.push(message('system', 'system', this.fault));
                for (const runId of job.runs)
                    if (!room.messages.some(entry => entry.runId === runId)) {
                        room.messages.push(message('system', 'system', 'Retained workflow evidence from an owned call whose room publication failed.', runId));
                    }
                this.emergency.set(id, room);
                this.publish(room);
                for (const other of this.jobs.values()) {
                    other.controller.abort(new Error(this.fault));
                    this.abortRuns(other);
                }
            }
        }).finally(() => { clearInterval(poll); this.jobs.delete(id); job.settled.abort(); });
    }
    abortRuns(job) { for (const id of job.runs) {
        try {
            this.service.cancel(id);
        }
        catch { /* Retain any completed run. */ }
    } }
    validateMembers(members) {
        if (new Set(members.map(member => member.agent)).size !== members.length)
            throw new Error('Each room member must have a unique agent alias.');
        const workers = members.filter(member => member.role !== 'advisor');
        if (!workers.length)
            throw new Error('Select at least one voting member.');
        if (workers.filter(member => member.role === 'lead').length > 1)
            throw new Error('Select at most one room lead.');
        for (const member of members) {
            if (!Object.hasOwn(this.service.config.agents, member.agent) || !this.service.config.agents[member.agent]?.enabled)
                throw new Error(`Agent is unavailable or disabled: ${member.agent}`);
            if (member.advisor && (member.role === 'advisor' || !members.some(candidate => candidate.agent === member.advisor && candidate.role === 'advisor')))
                throw new Error('A worker advisor must name a distinct advisor member.');
            if (member.advisorMaxRequests !== undefined && (!member.advisor || !Number.isSafeInteger(member.advisorMaxRequests)))
                throw new Error('Advisor request quota requires a configured advisor and a positive safe integer.');
            if (member.model)
                nonempty(member.model, 'Model', 800);
            if (member.skills !== undefined)
                validateSkills(member.skills);
        }
    }
    task(value, id) {
        const input = parseSchema(taskSchema, value, 'task');
        nonempty(input.title, 'Task title', 800);
        nonempty(input.brief, 'Task brief');
        if (new Set(input.dependsOn).size !== (input.dependsOn?.length ?? 0))
            throw new Error('Task dependencies must be unique.');
        return { ...input, id, status: 'pending' };
    }
    validateDependencies(tasks) {
        const byId = new Map(tasks.map(task => [task.id, task]));
        const done = new Set(), visiting = new Set();
        const visit = (id) => {
            if (done.has(id))
                return;
            if (visiting.has(id))
                throw new Error('Task dependencies must form an acyclic graph.');
            const task = byId.get(id);
            if (!task)
                throw new Error(`Task dependency not found: ${id}`);
            visiting.add(id);
            for (const dependency of task.dependsOn ?? [])
                visit(dependency);
            visiting.delete(id);
            done.add(id);
        };
        for (const task of tasks)
            visit(task.id);
    }
    settings(room, agents) {
        return Object.fromEntries(room.members.filter(member => agents.includes(member.agent)).map(member => [member.agent,
            { ...(member.model ? { model: member.model } : {}), ...(member.effort ? { effort: member.effort } : {}), ...(member.skills !== undefined ? { skills: [...member.skills] } : {}) }]));
    }
    taskInput(room, task, member) {
        const names = [member.agent, ...(member.advisor ? [member.advisor] : [])];
        return { mode: 'delegate', agents: [member.agent], permission: task.permission,
            task: `Room task ${task.id}: ${task.title}\n\n${task.brief}\n\nComplete only this project-manager task. Preserve its stated permission and report verification evidence.`,
            context: this.context(room), contextLimit: room.contextLimit, agentSettings: this.settings(room, names),
            ...(member.advisor ? { advisors: { [member.agent]: { agent: member.advisor, maxRequests: member.advisorMaxRequests ?? 1 } } } : {}) };
    }
    discussionInput(room, round) {
        const members = voters(room), tasks = pending(room);
        const data = { roomId: room.id, round, members: members.map(({ agent, role, weight }) => ({ agent, role, weight })),
            tasks: tasks.map(({ id, title, permission, dependsOn }) => ({ id, title, permission, dependsOn: dependsOn ?? [] })) };
        return { mode: 'parallel', agents: members.map(member => member.agent), permission: 'read',
            task: `Discuss the project manager's pending tasks without changing files or permissions. Independently vote and propose assignments.\nROOM_BALLOT_DATA_BEGIN\n${JSON.stringify(data)}\nROOM_BALLOT_DATA_END\nReturn exactly one JSON object, optionally in one json code fence: {"vote":"approve"|"revise"|"abstain","rationale":"your complete evidence and dissent","assignments":{"task-id":"voting-agent-alias"}}. Approve requires an assignment for EVERY pending task; all aliases and task IDs must come from the data. Rationale must be nonempty. No other fields or surrounding prose.`,
            context: this.context(room), contextLimit: room.contextLimit, agentSettings: this.settings(room, members.map(member => member.agent)) };
    }
    context(room) {
        const history = [];
        let cursor;
        do {
            const page = this.store.messagePage(room.id, 100, cursor);
            history.push(...page.messages);
            cursor = page.nextCursor ?? undefined;
            if (page.nextCursor === null)
                break;
        } while (true);
        const sections = [JSON.stringify({ roomBrief: room.brief }), ...room.tasks.map(task => JSON.stringify({ task: task.id, brief: task.brief,
                status: task.status, dependsOn: task.dependsOn, runId: task.runId })),
            ...history.map(value => JSON.stringify({ author: value.author, kind: value.kind, body: value.body }))];
        const context = sections.join('\n');
        if (room.contextLimit !== null && context.length > room.contextLimit)
            throw new Error(`Room context requires ${context.length} characters, exceeding the configured context budget ${room.contextLimit}; increase the context budget or use full context.`);
        return context;
    }
    cost(input) { return this.service.engine.plan(input).estimatedCalls; }
    reservePlan(room, discussion) {
        const members = voters(room);
        for (const task of pending(room))
            for (const dependency of task.dependsOn ?? []) {
                const prerequisite = room.tasks.find(candidate => candidate.id === dependency);
                if (!prerequisite || prerequisite.status === 'failed' || prerequisite.status === 'cancelled')
                    throw new Error(`Task ${task.id} has an unavailable prerequisite ${dependency}. Create a new task plan before starting provider work.`);
            }
        let required = discussion ? this.cost(this.discussionInput(room, 1)) * room.rounds : 0;
        for (const task of pending(room)) {
            const candidates = !discussion && task.assignee ? members.filter(member => member.agent === task.assignee) : members;
            if (!candidates.length)
                throw new Error('Task has no valid voting-member assignee.');
            required += Math.max(...candidates.map(member => this.cost(this.taskInput(room, task, member))));
        }
        if (room.callsUsed + required > room.maxCalls)
            throw new Error(`Room requires up to ${required} further calls, exceeding its remaining call budget ${room.maxCalls - room.callsUsed}.`);
    }
    async launch(id, job, input, taskId) {
        this.check(id, job);
        const estimated = this.cost(input);
        let run;
        this.own(id, room => {
            this.assertInstructions(room, job);
            if (room.callsUsed + estimated > room.maxCalls)
                throw new Error('Room call budget exhausted before dispatch.');
            this.check(id, job);
            room.callsUsed += estimated;
            run = this.service.start(input);
            job.runs.add(run.id);
            if (taskId) {
                const task = room.tasks.find(value => value.id === taskId);
                task.status = 'running';
                task.runId = run.id;
            }
            room.messages.push(message('system', taskId ? 'task' : 'system', taskId ? `Dispatched ${taskId}.` : 'Discussion round dispatched.', run.id));
        });
        try {
            const result = await this.service.completion(run.id);
            this.own(id, room => { room.callsUsed = Math.max(0, room.callsUsed - estimated + result.steps.length); });
            return result;
        }
        finally {
            job.runs.delete(run.id);
        }
    }
    ballot(output, room) {
        const source = output.trim();
        const fenced = /^```(?:json)?\s*\n([\s\S]*?)\n```$/i.exec(source);
        const json = fenced ? fenced[1] : source;
        let value;
        try {
            value = JSON.parse(json);
        }
        catch {
            throw new Error('Agent did not return one complete JSON ballot.');
        }
        rejectDuplicateKeys(json);
        const ballot = parseSchema(ballotSchema, value, 'ballot');
        nonempty(ballot.rationale, 'Ballot rationale');
        const taskIds = new Set(pending(room).map(task => task.id)), aliases = new Set(voters(room).map(member => member.agent));
        for (const [task, agent] of Object.entries(ballot.assignments))
            if (!taskIds.has(task) || !aliases.has(agent))
                throw new Error('Ballot assignment names an unknown task or voting member.');
        if (ballot.vote === 'approve' && Object.keys(ballot.assignments).length !== taskIds.size)
            throw new Error('An approve ballot must assign every pending task.');
        return ballot;
    }
    decide(room, ballots) {
        const members = voters(room);
        const approved = members.filter(member => ballots.get(member.agent).vote === 'approve');
        const winner = approved.sort((a, b) => b.weight - a.weight || Number(b.role === 'lead') - Number(a.role === 'lead') || room.members.indexOf(a) - room.members.indexOf(b))[0];
        // Align exact decimal coefficients across scientific notation and subnormal
        // values. Neither binary rounding nor a floating-point overflow can approve a tie.
        const weights = members.map(member => decimalWeight(member.weight));
        const exponent = Math.min(...weights.map(weight => weight.exponent));
        const units = new Map(members.map((member, index) => [member.agent,
            weights[index].coefficient * 10n ** BigInt(weights[index].exponent - exponent)]));
        const totalUnits = members.reduce((sum, member) => sum + units.get(member.agent), 0n);
        const approvedUnits = approved.reduce((sum, member) => sum + units.get(member.agent), 0n);
        const approveWeight = displayedWeight(approvedUnits, exponent), totalWeight = displayedWeight(totalUnits, exponent);
        const assignments = winner ? ballots.get(winner.agent).assignments : {};
        return { approved: approvedUnits * 2n > totalUnits, approveWeight, totalWeight, instructionRevision: room.instructionRevision ?? 0,
            assignments,
            rationale: winner ? ballots.get(winner.agent).rationale : 'No voting member approved the proposed task assignments.',
            dissent: members.filter(member => {
                const ballot = ballots.get(member.agent);
                return ballot.vote !== 'approve'
                    || Object.keys(assignments).some(task => ballot.assignments[task] !== assignments[task]);
            })
                .map(member => { const ballot = ballots.get(member.agent); return `${member.agent} (${ballot.vote}${ballot.vote === 'approve' ? `; alternative assignments ${JSON.stringify(ballot.assignments)}` : ''}): ${ballot.rationale}`; }) };
    }
    async discuss(id, job) {
        for (let round = 1; round <= this.get(id).rounds; round++) {
            this.check(id, job);
            this.assertInstructions(this.get(id), job);
            const before = this.get(id), run = await this.launch(id, job, this.discussionInput(before, round));
            this.check(id, job);
            const ballots = new Map();
            let invalid;
            for (const member of voters(before)) {
                const steps = run.steps.filter(step => step.agent === member.agent && step.role === 'worker');
                const step = steps[0];
                if (run.status !== 'completed' || steps.length !== 1 || !step || step.result.status !== 'completed') {
                    invalid = 'A discussion ballot failed or did not complete; no task will execute.';
                    if (step)
                        this.own(id, room => { room.messages.push(message(member.agent, 'opinion', `${step.result.status}: ${step.result.output}`, run.id)); });
                    continue;
                }
                let ballot;
                try {
                    ballot = this.ballot(step.result.output, before);
                }
                catch (error) {
                    invalid = `Invalid ballot from ${member.agent}: ${error instanceof Error ? error.message : 'Invalid JSON ballot.'}`;
                    this.own(id, room => { room.messages.push(message(member.agent, 'opinion', `Invalid ballot: ${step.result.output}`, run.id)); });
                    continue;
                }
                ballots.set(member.agent, ballot);
                this.own(id, room => { room.messages.push(message(member.agent, 'opinion', JSON.stringify(ballot), run.id)); });
            }
            this.assertInstructions(this.get(id), job);
            if (invalid) {
                this.own(id, room => { room.status = 'needs_attention'; room.error = invalid; room.messages.push(message('system', 'decision', invalid)); }, true);
                return;
            }
            const decision = this.decide(before, ballots);
            this.own(id, room => {
                this.assertInstructions(room, job);
                room.decision = decision;
                room.messages.push(message('system', 'decision', `Round ${round}: ${decision.approved ? 'approved' : 'revision required'} (${decision.approveWeight}/${decision.totalWeight} approval weight). ${decision.rationale}`));
                if (decision.approved) {
                    for (const task of pending(room)) {
                        task.assignee = decision.assignments[task.id];
                        task.status = 'assigned';
                    }
                    room.status = room.autoExecute ? 'running' : 'ready';
                }
            });
            if (decision.approved) {
                if (before.autoExecute)
                    await this.executeTasks(id, job);
                else
                    this.own(id, () => { }, true);
                return;
            }
        }
        this.own(id, room => { room.status = 'needs_attention'; room.error = 'No weighted majority approved the tasks within the configured discussion rounds.'; }, true);
    }
    async executeTasks(id, job) {
        this.check(id, job);
        this.assertInstructions(this.get(id), job);
        this.reservePlan(this.get(id), false);
        while (pending(this.get(id)).length) {
            this.check(id, job);
            const room = this.get(id);
            this.assertInstructions(room, job);
            const ready = pending(room).filter(task => (task.dependsOn ?? []).every(dependency => room.tasks.find(value => value.id === dependency)?.status === 'completed'))
                .slice(0, this.service.config.maxConcurrency);
            if (!ready.length)
                throw new Error('Pending task dependencies cannot be satisfied; no further task was dispatched.');
            const results = await Promise.allSettled(ready.map(async (task) => {
                const member = room.members.find(value => value.agent === task.assignee && value.role !== 'advisor');
                if (!member)
                    throw new Error('Approved task assignee is unavailable.');
                const run = await this.launch(id, job, this.taskInput(room, task, member), task.id);
                this.own(id, current => {
                    const target = current.tasks.find(value => value.id === task.id);
                    target.status = run.status === 'completed' ? 'completed' : run.status === 'cancelled' ? 'cancelled' : 'failed';
                    target.resultPreview = excerpt(run.output ?? '');
                    if (run.error)
                        target.error = run.error;
                    current.messages.push(message(member.agent, 'task', `${task.id}: ${target.status}\n${run.output ?? ''}`, run.id));
                    for (const step of run.steps)
                        if (step.role.startsWith('advisor-for-'))
                            current.messages.push(message(step.agent, 'advisor', step.result.output, run.id));
                });
                if (run.status !== 'completed')
                    throw new Error(`Task ${task.id} ${run.status}: ${run.error ?? 'Incomplete workflow.'}`);
            }));
            this.check(id, job);
            const rejected = results.find(result => result.status === 'rejected' && !(result.reason instanceof InstructionsChanged))
                ?? results.find(result => result.status === 'rejected');
            if (rejected?.status === 'rejected')
                throw rejected.reason;
            this.assertInstructions(this.get(id), job);
        }
        this.check(id, job);
        this.own(id, room => {
            this.assertInstructions(room, job);
            if (room.tasks.some(task => task.status !== 'completed'))
                throw new Error('Not every room task completed.');
            room.status = 'completed';
            delete room.error;
            room.messages.push(message('system', 'system', 'All room tasks completed. Workflow records contain full verification evidence; write changes remain in isolated worktrees for review.'));
        }, true);
    }
}
//# sourceMappingURL=rooms.js.map