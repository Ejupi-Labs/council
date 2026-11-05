import { newAdmissionAttempt, runAdmissionAttempt, consumeAdmissionRefusal } from './admission-attempt.js';
import { randomUUID } from 'node:crypto';
import { setTimeout as delay } from 'node:timers/promises';
import { RoomStore } from './room-store.js';
import { assertUniqueObjectKeys } from './control-json.js';
import { checkAgentJsonStructure } from './agent-json.js';
import { objectSchema, parseSchema, type JsonSchema } from './schema.js';
import { roomMessageDefinition } from './rooms-schema.js';
import { REASONING_EFFORTS, type RunRecord, type WorkflowInput, type WorkspaceArtifact } from './types.js';
import { BUILT_IN_SKILL_IDS, validateSkills } from './skills.js';
import type { CouncilService } from './service.js';
import type { RoomDecision, RoomInput, RoomMember, RoomMessage, RoomObservation, RoomRecord, RoomSummary, RoomTask, RoomTaskInput } from './rooms-types.js';

const text = (maximum?: number): JsonSchema => ({ type: 'string', minLength: 1, ...(maximum === undefined ? {} : { maxLength: maximum }), pattern: '^[^\\u0000]+$' });
const taskSchema = objectSchema({ title: text(200), brief: text(), permission: { type: 'string', enum: ['read', 'write'] },
  dependsOn: { type: 'array', items: text(100) } }, ['title', 'brief', 'permission']);
const memberSchema = objectSchema({ agent: text(64), role: { type: 'string', enum: ['lead', 'worker', 'advisor'] },
  weight: { type: 'number', exclusiveMinimum: 0 }, model: text(200),
  effort: { type: 'string', enum: REASONING_EFFORTS }, advisor: text(64), advisorMaxRequests: { type: 'integer', minimum: 1 },
  skills: { type: 'array', items: { type: 'string', enum: BUILT_IN_SKILL_IDS } } }, ['agent', 'role']);
const roomSchema = objectSchema({ title: text(200), brief: text(), members: { type: 'array', items: memberSchema, minItems: 1 },
  tasks: { type: 'array', items: taskSchema, default: [] },
  rounds: { type: 'integer', minimum: 1, default: 1 }, maxCalls: { type: 'integer', minimum: 1 },
  autoExecute: { type: 'boolean', default: true }, contextLimit: { type: 'integer', minimum: 1 },
  privateChatMaxRequests: { type: 'integer', minimum: 1, maximum: Number.MAX_SAFE_INTEGER } }, ['title', 'brief', 'members']);
const ballotSchema = objectSchema({ vote: { type: 'string', enum: ['approve', 'revise', 'abstain'] }, rationale: text(),
  assignments: { type: 'object', additionalProperties: text(64) } }, ['vote', 'rationale', 'assignments']);
type Ballot = { vote: 'approve' | 'revise' | 'abstain'; rationale: string; assignments: Record<string, string> };
type PendingPublication = { runId: string; taskId?: string; estimated: number; dispatch: RoomMessage; retained: RoomMessage; result?: RunRecord; resultMessages?: RoomMessage[]; reconciled?: boolean; unverified?: boolean; conflict?: boolean };
type Job = { controller: AbortController; runs: Set<string>; publications: Map<string, PendingPublication>; callsUsed: number; done: Promise<void>; settled: AbortController; cancelRequested: boolean; acceptedCancellationReason?: Error; lastStateSuccess: number; snapshot: RoomRecord; instructionRevision: number; failure?: unknown };
const PUBLICATION_CONFLICT = 'A retained workflow conflicts with this task association. Inspect both linked workflows before replacing the task.';
const UNVERIFIED_COMPLETION = 'Retained workflow completion could not be verified; its estimated call reservation is retained. Inspect the linked workflow before replacing this task.';
function acceptedCancellation(job: Job, reason: unknown): boolean {
  return job.acceptedCancellationReason !== undefined && reason === job.acceptedCancellationReason;
}
class InstructionsChanged extends Error {
  constructor() { super('Project manager instructions changed. In-flight results are retained; start a new discussion before dispatching further tasks.'); }
}
class PauseRequested extends Error {
  constructor() { super('Room paused after admitted workflows drained. No completed workflow will be replayed.'); }
}
const active = (room: RoomRecord): boolean => room.status === 'discussing' || room.status === 'running';

function nonempty(value: string, label: string, bytes?: number): void {
  if (!value.trim() || bytes !== undefined && Buffer.byteLength(value) > bytes) throw new Error(`${label} must contain non-whitespace text${bytes === undefined ? '.' : ` within ${bytes} UTF-8 bytes.`}`);
}
function excerpt(value: string, maximum = 2000): string {
  if (value.length <= maximum) return value;
  const suffix = '\n[Preview truncated; full evidence is retained in the linked run.]';
  let result = value.slice(0, Math.max(0, maximum - suffix.length));
  if (/[\uD800-\uDBFF]$/.test(result)) result = result.slice(0, -1);
  return result + suffix;
}
function message(author: string, kind: RoomMessage['kind'], body: string, runId?: string): RoomMessage {
  return { id: randomUUID(), author, kind, body, createdAt: new Date().toISOString(), ...(runId ? { runId } : {}) };
}
function pending(room: RoomRecord): RoomTask[] { return room.tasks.filter(task => task.status === 'pending' || task.status === 'assigned'); }
function voters(room: RoomRecord): RoomMember[] { return room.members.filter(member => member.role !== 'advisor'); }
function decimalWeight(weight: number): { coefficient: bigint; exponent: number } {
  const [coefficient, exponent = '0'] = weight.toString().split('e');
  const [whole, fraction = ''] = coefficient!.split('.');
  return { coefficient: BigInt(whole! + fraction), exponent: Number(exponent) - fraction.length };
}
function displayedWeight(units: bigint, exponent: number): number | string {
  if (units === 0n) return 0;
  const digits = units.toString(), fraction = digits.slice(1).replace(/0+$/, '');
  const scientific = `${digits[0]}${fraction ? `.${fraction}` : ''}e${exponent + digits.length - 1}`;
  const number = Number(scientific);
  return Number.isFinite(number) ? number : scientific;
}
/** JSON.parse validates grammar first; reject ambiguous keys before trusting a ballot. */

/** A durable human-managed task board; provider calls use the existing workflow service. */
export class RoomManager {
  private readonly store: RoomStore;
  private readonly owner = randomUUID();
  private readonly jobs = new Map<string, Job>();
  private readonly listeners = new Set<(room: RoomRecord) => void>();
  private publicationVersion = 0n;
  private readonly emergency = new Map<string, RoomRecord>();
  private closing = false;
  private fault?: string;
  private closePromise?: Promise<void>;
  private readonly shutdown = new AbortController();

  constructor(private readonly service: CouncilService) { this.store = new RoomStore(service.config.stateDir); }

  subscribe(listener: (room: RoomRecord) => void): () => void { this.listeners.add(listener); return () => { this.listeners.delete(listener); }; }
  list(limit = 1000): RoomSummary[] { return this.store.list(limit).map(summary => {
    const room = this.emergency.get(summary.id);
    return room ? { id: room.id, backend: room.backend, title: room.title, status: room.status,
      taskCount: room.tasks.length, completedTasks: room.tasks.filter(task => task.status === 'completed').length,
      callsUsed: room.callsUsed, maxCalls: room.maxCalls, revision: room.revision, updatedAt: room.updatedAt } : summary;
  }); }

  /** Connection-local refresh signal, including emergency outcomes that could not commit. */
  changeToken(): string { return `${this.store.changeToken()}:${this.publicationVersion}`; }
  get(id: string): RoomRecord {
    if (typeof id !== 'string' || !id || id.length > 100) throw new Error('Room not found (404).');
    const room = this.emergency.get(id) ?? this.store.get(id);
    if (!room) throw new Error(`Room not found (404): ${id}`);
    return structuredClone(room);
  }
  metadata(id: string): Omit<RoomRecord, 'messages'> {
    if (typeof id !== 'string' || !id || id.length > 100) throw new Error('Room not found (404).');
    const saved = this.emergency.get(id);
    const room = saved ?? this.store.metadata(id);
    if (!room) throw new Error(`Room not found (404): ${id}`);
    const { messages: _messages, ...metadata } = room as RoomRecord;
    return structuredClone(metadata);
  }

  observation(id: string): RoomObservation {
    if (typeof id !== 'string' || !id || id.length > 100) throw new Error('Room not found (404).');
    const saved = this.emergency.get(id);
    const value = saved ? { id: saved.id, title: saved.title, status: saved.status, revision: saved.revision,
      instructionRevision: saved.instructionRevision ?? 0, taskCount: saved.tasks.length,
      completedTasks: saved.tasks.filter(task => task.status === 'completed').length,
      failedTasks: saved.tasks.filter(task => task.status === 'failed' || task.status === 'cancelled').length,
      callsUsed: saved.callsUsed, maxCalls: saved.maxCalls, ...(saved.error ? { error: saved.error } : {}),
      ...(saved.pauseRequested ? { pauseRequested: true } : {}) } : this.store.observation(id);
    if (!value) throw new Error(`Room not found (404): ${id}`);
    return structuredClone(value);
  }

  private assertExists(id: string): void {
    if (typeof id !== 'string' || !id || id.length > 100) throw new Error('Room not found (404).');
    if (!this.emergency.has(id) && !this.store.has(id)) throw new Error(`Room not found (404): ${id}`);
  }
  /** Reconcile a proven-dead owner without reading history or replaying work. */
  recover(id: string): void { this.assertExists(id); this.store.recover(id); }
  messagePage(id: string, limit = 100, cursor?: number) { this.assertExists(id); return this.store.messagePage(id, limit, cursor); }

  create(value: RoomInput): RoomRecord {
    this.available();
    const normalized = value && typeof value === 'object' ? { ...value,
      ...(value.contextLimit === null ? { contextLimit: undefined } : {}),
      members: Array.isArray(value.members) ? value.members.map(member => member && typeof member === 'object' && !Array.isArray(member)
        ? { ...member, weight: member.weight === undefined ? this.service.config.agents[member.agent]?.authority ?? 1 : member.weight } : member) : value.members } : value;
    const input = parseSchema<Omit<RoomInput, 'members'> & { members: RoomMember[]; tasks: RoomTaskInput[]; rounds: number; autoExecute: boolean }>(roomSchema, normalized, 'room');
    if (!Number.isSafeInteger(input.rounds) || input.maxCalls !== undefined && !Number.isSafeInteger(input.maxCalls)) throw new Error('Room rounds and call budget must be positive safe integers.');
    nonempty(input.title, 'Room title', 800); nonempty(input.brief, 'Room brief');
    this.validateMembers(input.members);
    const now = new Date().toISOString();
    const room: RoomRecord = { id: randomUUID(), title: input.title, brief: input.brief, members: input.members, backend: this.service.config.backend,
      status: 'idle', tasks: input.tasks.map((task, index) => this.task(task, `task-${index + 1}`)),
      messages: [message('system', 'system', 'The project manager owns task briefs and permissions. Discussion is read-only; approved work uses the original task permissions.')],
      rounds: input.rounds, maxCalls: input.maxCalls ?? this.service.config.maxCalls, autoExecute: input.autoExecute, contextLimit: input.contextLimit ?? null,
      callsUsed: 0, revision: 1, instructionRevision: 0, createdAt: now, updatedAt: now,
      ...(input.privateChatMaxRequests !== undefined ? { privateChatMaxRequests: input.privateChatMaxRequests } : {}) };
    this.validateDependencies(room.tasks);
    this.store.insert(room); this.publish(room);
    return structuredClone(room);
  }

  postMessage(id: string, value: { author: string; body: string }): RoomRecord {
    this.available();
    const input = parseSchema<{ author: string; body: string }>(roomMessageDefinition, value, 'message');
    nonempty(input.author, 'Message author', 320); nonempty(input.body, 'Message body');
    return this.change(id, room => {
      room.messages.push(message(input.author, 'human', input.body));
      room.instructionRevision = (room.instructionRevision ?? 0) + 1;
      if (room.status === 'ready' || room.status === 'paused') {
        this.invalidateDecision(room); room.status = 'needs_attention'; room.error = new InstructionsChanged().message;
        delete room.checkpoint; delete room.pauseRequested;
      } else if (active(room)) room.error = new InstructionsChanged().message;
    });
  }

  addTask(id: string, value: RoomTaskInput): RoomRecord {
    this.available();
    return this.change(id, room => {
      if (room.status === 'failed' || room.status === 'cancelled') throw new Error('Failed or cancelled rooms cannot accept new tasks. Create a new room; unfinished writes will not be replayed.');
      const task = this.task(value, `task-${room.tasks.length + 1}`);
      room.tasks.push(task); this.validateDependencies(room.tasks);
      room.instructionRevision = (room.instructionRevision ?? 0) + 1;
      room.status = 'idle'; delete room.decision; delete room.error; delete room.checkpoint; delete room.pauseRequested;
      room.messages.push(message('project-manager', 'task', `Added ${task.id}: ${task.title} (${task.permission}).`));
    }, { requireIdle: true });
  }

  startDiscussion(id: string): RoomRecord {
    this.available();
    this.service.assertDispatchAllowed();
    if (this.jobs.size >= 20) throw new Error('This room manager already owns 20 active jobs.');
    const initial = this.get(id);
    this.assertBackend(initial);
    if (!['idle', 'ready', 'needs_attention'].includes(initial.status)) throw new Error('This room cannot start another discussion in its current state.');
    if (!pending(initial).length) throw new Error('Add at least one pending task before starting discussion.');
    this.reservePlan(initial, true);
    const room = this.change(id, value => {
      this.assertBackend(value);
      if (!['idle', 'ready', 'needs_attention'].includes(value.status)) throw new Error('A room discussion is already active or terminal.');
      this.reservePlan(value, true);
      value.status = 'discussing'; delete value.decision; delete value.error;
      value.checkpoint = { phase: 'discussion', nextRound: 1 }; delete value.pauseRequested;
      for (const task of pending(value)) { task.status = 'pending'; delete task.assignee; }
      value.messages.push(message('system', 'system', 'Read-only discussion queued. Every voting member must return a valid ballot before a decision can execute.'));
    }, { claim: { id: this.owner, pid: process.pid }, requireIdle: true }, room => this.queue(id, job => this.discuss(id, job), room));
    return room;
  }

  execute(id: string): RoomRecord {
    this.available();
    this.service.assertDispatchAllowed();
    if (this.jobs.size >= 20) throw new Error('This room manager already owns 20 active jobs.');
    this.assertBackend(this.get(id));
    const room = this.change(id, value => {
      this.assertBackend(value);
      if (value.status !== 'ready' || !value.decision?.approved) throw new Error('Only an approved ready room can execute.');
      if (value.decision.instructionRevision === undefined || value.decision.instructionRevision !== (value.instructionRevision ?? 0)) throw new Error('Room approval does not cover the current project manager instructions. Start a new discussion before executing.');
      this.reservePlan(value, false);
      value.status = 'running'; delete value.error;
      value.checkpoint = { phase: 'execution' }; delete value.pauseRequested;
    }, { claim: { id: this.owner, pid: process.pid }, requireIdle: true }, room => this.queue(id, job => this.executeTasks(id, job), room));
    return room;
  }

  cancel(id: string): RoomRecord {
    const requested = this.store.requestCancel(id);
    const job = this.jobs.get(id);
    if (job) this.abort(job);
    this.publish(requested); return requested;
  }

  pause(id: string): RoomRecord {
    this.available();
    const room = this.store.requestPause(id);
    const job = this.jobs.get(id); if (job) job.snapshot = structuredClone(room);
    this.publish(room); return room;
  }

  resume(id: string): RoomRecord {
    this.available();
    this.service.assertDispatchAllowed();
    const initial = this.get(id);
    this.assertBackend(initial);
    if (initial.status !== 'paused' || !initial.checkpoint) throw new Error('Only a durably paused room can resume.');
    const phase = initial.checkpoint.phase;
    if (phase !== 'ready' && this.jobs.size >= 20) throw new Error('This room manager already owns 20 active jobs.');
    const room = this.change(id, value => {
      this.assertBackend(value);
      if (value.status !== 'paused' || value.checkpoint?.phase !== phase) throw new Error('Room pause checkpoint changed before resume.');
      if (value.tasks.some(task => ['running', 'failed', 'cancelled'].includes(task.status))) throw new Error('Paused room contains incomplete or failed admitted work; it cannot be replayed.');
      if (phase !== 'discussion') {
        if (!value.decision?.approved || value.decision.instructionRevision === undefined
          || value.decision.instructionRevision !== (value.instructionRevision ?? 0)) throw new Error('Room approval does not cover current instructions. Start a new discussion before executing.');
        if (pending(value).some(task => !task.assignee || value.decision!.assignments[task.id] !== task.assignee)) throw new Error('Paused task assignments do not match the approved plan.');
      }
      if (phase === 'discussion') {
        const nextRound = value.checkpoint!.phase === 'discussion' ? value.checkpoint!.nextRound : 0;
        if (!Number.isSafeInteger(nextRound) || nextRound < 1 || nextRound > value.rounds) throw new Error('Room discussion continuation is invalid; start a new discussion.');
        this.reservePlan(value, true, nextRound);
      } else if (phase === 'execution') this.reservePlan(value, false);
      value.status = phase === 'discussion' ? 'discussing' : phase === 'execution' ? 'running' : 'ready';
      delete value.pauseRequested; delete value.error;
      if (phase === 'ready') delete value.checkpoint;
      value.messages.push(message('system', 'system', phase === 'ready' ? 'Room resumed to its approved ready plan. Execute explicitly to begin work.' : 'Room resumed from its durable dispatch checkpoint; completed workflows are retained.'));
    }, { requireIdle: true, ...(phase !== 'ready' ? { claim: { id: this.owner, pid: process.pid } } : {}) },
    phase === 'ready' ? undefined : room => this.queue(id, job => phase === 'discussion' ? this.discuss(id, job) : this.executeTasks(id, job), room));
    return room;
  }

  async wait(id: string, timeoutMs = 600000, signal?: AbortSignal): Promise<RoomRecord> {
    if (!Number.isFinite(timeoutMs) || timeoutMs < 0 || timeoutMs > 600000) throw new Error('Room wait timeout must be from 0 to 600000 ms.');
    const cancellation = signal ? AbortSignal.any([signal, this.shutdown.signal]) : this.shutdown.signal;
    cancellation.throwIfAborted();
    const initial = this.get(id);
    if (!active(initial) || timeoutMs === 0) return initial;
    const job = this.jobs.get(id);
    if (job) {
      const stop = AbortSignal.any([cancellation, job.settled.signal]);
      try { await delay(timeoutMs, undefined, { signal: stop }); }
      catch (error) { cancellation.throwIfAborted(); if (!job.settled.signal.aborted) throw error; }
      cancellation.throwIfAborted(); return this.get(id);
    }
    const deadline = performance.now() + timeoutMs;
    for (;;) {
      await delay(Math.max(1, Math.min(100, deadline - performance.now())), undefined, { signal: cancellation });
      cancellation.throwIfAborted();
      this.store.recover(id);
      const room = this.get(id);
      if (!active(room) || performance.now() >= deadline) return room;
    }
  }

  close(): Promise<void> {
    if (this.closePromise) return this.closePromise;
    this.closing = true;
    this.shutdown.abort(new DOMException('Room manager is shutting down.', 'AbortError'));
    this.closePromise = (async () => {
      for (const [id, job] of this.jobs) {
        try { this.store.requestCancel(id, { activeOnly: true }); } catch { /* Owned subprocesses must still drain when storage is unavailable. */ }
        this.abort(job);
      }
      await Promise.allSettled([...this.jobs.values()].map(job => job.done));
      let failure: unknown;
      const deadline = performance.now() + 5000;
      try {
        for (const [id, saved] of this.emergency) {
          if (performance.now() >= deadline) {
            failure ??= new Error('Failed room outcomes could not be durably finalized within the shutdown storage deadline. Inspect local state before replacing those tasks.');
            break;
          }
          for (;;) {
            try {
              const merge = (room: RoomRecord): void => {
                // Preserve current human guidance and append-only history. A failed
                // room cannot admit replacement calls; restore the exact drained ledger.
                this.assertBackend(room);
                room.status = 'failed'; room.error = saved.error ?? 'Room persistence failed; owned work was drained.';
                room.callsUsed = saved.callsUsed;
                for (const task of room.tasks) {
                  const evidence = saved.tasks.find(candidate => candidate.id === task.id);
                  if (evidence?.runId && (!task.runId || task.runId === evidence.runId)) {
                    task.runId = evidence.runId;
                    if (evidence.workspaces !== undefined) task.workspaces = structuredClone(evidence.workspaces);
                    if (evidence.resultPreview !== undefined) task.resultPreview = evidence.resultPreview;
                    if (['completed', 'failed', 'cancelled'].includes(evidence.status)) {
                      task.status = evidence.status;
                      if (evidence.error) task.error = evidence.error; else delete task.error;
                    }
                  } else if (evidence?.runId && task.runId !== evidence.runId) {
                    task.status = 'failed'; task.error = PUBLICATION_CONFLICT;
                    if (!room.error.includes(PUBLICATION_CONFLICT)) room.error += '\n' + PUBLICATION_CONFLICT;
                  }
                  if (['pending', 'assigned', 'running'].includes(task.status)) {
                    task.status = task.status === 'running' ? 'failed' : 'cancelled'; task.error = room.error;
                  }
                }
                room.messages.push(...saved.messages.filter(entry => entry.id !== 'history-preview'));
                room.messages.push(message('system', 'system', 'The failed room outcome was recovered after owned work drained. No tasks were replayed.'));
              };
              let repaired: RoomRecord;
              try { repaired = this.store.mutate(id, merge, { owner: this.owner, release: true, preserveFailure: true }); }
              catch (error) {
                if (!(error instanceof Error) || error.message !== 'Room ownership was lost.') throw error;
                // A failed release may have committed before its acknowledgement
                // was lost. Never reclaim an active, paused or foreign-owned room.
                repaired = this.store.mutate(id, room => {
                  if (room.status !== 'failed') throw new Error('Failed room recovery cannot replace another room outcome.');
                  merge(room);
                }, { requireIdle: true, release: true, preserveFailure: true });
              }
              this.emergency.delete(id); this.publish(repaired); break;
            } catch (error) {
              const code = error && typeof error === 'object' && 'errcode' in error ? error.errcode : undefined;
              const remaining = deadline - performance.now();
              if (typeof code === 'number' && ((code & 0xff) === 5 || (code & 0xff) === 6) && remaining > 0) {
                await delay(Math.min(50, remaining)); continue;
              }
              failure ??= new Error(`Failed room ${id} could not be durably finalized during shutdown. Its last durable state may remain active; inspect local storage before replacing the task.`, { cause: error });
              break;
            }
          }
        }
      } finally {
        this.listeners.clear();
        try { this.store.close(); } catch (error) { failure ??= error; }
      }
      if (failure) throw failure;
    })();
    return this.closePromise;
  }

  private available(): void {
    if (this.closing) throw new Error('Rooms are shutting down.');
    if (this.fault) throw new Error(this.fault);
  }
  private assertBackend(room: RoomRecord): void {
    if (!room.backend) throw new Error('Room backend provenance is unknown. Recreate this legacy room with the intended backend before starting provider work.');
    if (room.backend !== this.service.config.backend) throw new Error(`Room backend ${room.backend} differs from this service backend ${this.service.config.backend}. Open the matching backend or recreate the room with the intended backend.`);
  }
  private assertInstructions(room: RoomRecord, job: Job): void {
    if ((room.instructionRevision ?? 0) !== job.instructionRevision) throw new InstructionsChanged();
  }
  private invalidateDecision(room: RoomRecord): void {
    delete room.decision;
    for (const task of room.tasks) if (task.status === 'assigned') { task.status = 'pending'; delete task.assignee; }
  }
  private publish(room: RoomRecord): void {
    this.publicationVersion++;
    for (const listener of this.listeners) { try { listener(structuredClone(room)); } catch { /* A disconnected UI cannot alter room persistence. */ } }
  }
  private change(id: string, action: (room: RoomRecord) => void, options?: Parameters<RoomStore['mutate']>[2], beforePublish?: (room: RoomRecord) => void): RoomRecord {
    const room = this.store.mutate(id, action, options);
    const job = this.jobs.get(id); if (job) job.snapshot = structuredClone(room);
    // Mutation has committed. Track admitted work before a subscriber can close
    // or reenter this manager in response to the published active state.
    beforePublish?.(room);
    this.publish(room); return room;
  }
  private own(id: string, action: (room: RoomRecord) => void, release = false, preserveFailure = false): RoomRecord {
    return this.change(id, action, { owner: this.owner, ...(release ? { release: true } : {}), ...(preserveFailure ? { preserveFailure: true } : {}) });
  }
  private abort(job: Job): void {
    job.cancelRequested = true;
    if (!job.controller.signal.aborted) {
      job.acceptedCancellationReason = new Error('Room cancelled.');
      job.controller.abort(job.acceptedCancellationReason);
    }
    for (const run of job.runs) { try { this.service.cancel(run); } catch { /* Completion will retain the authoritative workflow failure. */ } }
  }
  private check(id: string, job: Job): void {
    if (this.store.cancelled(id, this.owner)) this.abort(job);
    job.lastStateSuccess = performance.now();
    job.controller.signal.throwIfAborted();
  }
  private queue(id: string, operation: (job: Job) => Promise<void>, snapshot: RoomRecord): void {
    const job: Job = { controller: new AbortController(), settled: new AbortController(), runs: new Set(), publications: new Map(), callsUsed: snapshot.callsUsed, done: Promise.resolve(), cancelRequested: false,
      lastStateSuccess: performance.now(), snapshot: structuredClone(snapshot), instructionRevision: snapshot.instructionRevision ?? 0 };
    this.jobs.set(id, job);
    const poll = setInterval(() => {
      try { this.check(id, job); }
      catch (error) {
        if (acceptedCancellation(job, error)) return;
        const code = error && typeof error === 'object' && 'errcode' in error ? error.errcode : undefined;
        if (typeof code === 'number' && ((code & 0xff) === 5 || (code & 0xff) === 6) && performance.now() - job.lastStateSuccess < 5000) return;
        if (job.failure === undefined) job.failure = error;
        if (!job.controller.signal.aborted) job.controller.abort(job.failure);
        this.abortRuns(job);
      }
    }, 100);
    poll.unref();
    job.done = Promise.resolve().then(() => operation(job)).catch(async error => {
      const shutdownRefusal = this.closing && job.acceptedCancellationReason !== undefined && error instanceof Error && error.message === 'Council is shutting down.';
      const wasCancellation = acceptedCancellation(job, error) || shutdownRefusal;
      // An accepted cancellation can be followed by a real cross-room/poll fault.
      // Preserve a received real error, but never report only cancellation text for that fault.
      if (wasCancellation && job.failure !== undefined) error = job.failure;
      const cancelled = wasCancellation && job.failure === undefined;
      const instructionsChanged = error instanceof InstructionsChanged && job.failure === undefined && !cancelled;
      const paused = error instanceof PauseRequested && job.failure === undefined && !cancelled;
      if (!instructionsChanged && !paused) this.abortRuns(job);
      await Promise.allSettled([...job.runs].map(run => this.service.completion(run)));
      await this.collectPublications(job);
      const unverified = [...job.publications.values()].some(publication => publication.unverified);
      try {
        this.own(id, room => {
          room.callsUsed = job.callsUsed;
          for (const publication of job.publications.values()) this.mergePublication(room, publication, true);
          const conflict = [...job.publications.values()].some(publication => publication.conflict);
          if (!unverified && !conflict && (instructionsChanged || paused && (room.instructionRevision ?? 0) !== job.instructionRevision)) {
            this.invalidateDecision(room); room.status = 'needs_attention'; room.error = error instanceof Error ? error.message : new InstructionsChanged().message;
            if (!instructionsChanged) room.error = new InstructionsChanged().message;
            room.messages.push(message('system', 'decision', room.error)); return;
          }
          if (paused && !unverified && !conflict) {
            this.assertInstructions(room, job);
            if (!room.pauseRequested || !room.checkpoint) throw new Error('Room pause continuation could not be verified.');
            room.status = 'paused'; delete room.error;
            room.messages.push(message('system', 'system', error.message)); return;
          }
          room.status = cancelled ? 'cancelled' : 'failed';
          room.error = cancelled ? 'Room cancelled; partial results and isolated worktrees are retained.' : excerpt(error instanceof Error ? error.message : 'Room execution failed.');
          if (unverified) { room.status = 'failed'; room.error += '\n' + UNVERIFIED_COMPLETION; }
          if (conflict) { room.status = 'failed'; room.error += '\n' + PUBLICATION_CONFLICT; }
          for (const task of room.tasks) if (task.status === 'running' || task.status === 'assigned' || task.status === 'pending') {
            task.status = cancelled || task.status !== 'running' ? 'cancelled' : 'failed'; task.error = room.error;
          }
          room.messages.push(message('system', 'system', room.error));
        }, true, true);
      } catch {
        this.fault = 'Room persistence failed. Inspect local state before starting new room work.';
        const room = structuredClone(job.snapshot); room.status = 'failed'; room.error = this.fault;
        room.callsUsed = job.callsUsed;
        for (const publication of job.publications.values()) this.mergePublication(room, publication, true);
        if (unverified) room.error += '\n' + UNVERIFIED_COMPLETION;
        if ([...job.publications.values()].some(publication => publication.conflict)) room.error += '\n' + PUBLICATION_CONFLICT;
        delete room.pauseRequested; delete room.checkpoint;
        room.revision++; room.updatedAt = new Date().toISOString();
        for (const task of room.tasks) if (['pending', 'assigned', 'running'].includes(task.status)) {
          task.status = task.status === 'running' ? 'failed' : 'cancelled'; task.error = this.fault;
        }
        room.messages.push(message('system', 'system', this.fault));
        for (const runId of job.runs) if (!room.messages.some(entry => entry.runId === runId)) {
          room.messages.push(message('system', 'system', 'Retained workflow evidence from an owned call whose room publication failed.', runId));
        }
        this.emergency.set(id, room); this.publish(room);
        for (const other of this.jobs.values()) {
          if (other.failure === undefined) other.failure = new Error(this.fault);
          other.controller.abort(other.failure); this.abortRuns(other);
        }
      }
    }).finally(() => { clearInterval(poll); if (this.jobs.get(id) === job) this.jobs.delete(id); job.settled.abort(); });
  }
  private abortRuns(job: Job): void { for (const id of job.runs) { try { this.service.cancel(id); } catch { /* Retain any completed run. */ } } }

  private validateMembers(members: RoomMember[]): void {
    if (new Set(members.map(member => member.agent)).size !== members.length) throw new Error('Each room member must have a unique agent alias.');
    const workers = members.filter(member => member.role !== 'advisor');
    if (!workers.length) throw new Error('Select at least one voting member.');
    if (workers.filter(member => member.role === 'lead').length > 1) throw new Error('Select at most one room lead.');
    for (const member of members) {
      if (!Object.hasOwn(this.service.config.agents, member.agent) || !this.service.config.agents[member.agent]?.enabled) throw new Error(`Agent is unavailable or disabled: ${member.agent}`);
      if (member.advisor && (member.role === 'advisor' || !members.some(candidate => candidate.agent === member.advisor && candidate.role === 'advisor'))) throw new Error('A worker advisor must name a distinct advisor member.');
      if (member.advisorMaxRequests !== undefined && (!member.advisor || !Number.isSafeInteger(member.advisorMaxRequests))) throw new Error('Advisor request quota requires a configured advisor and a positive safe integer.');
      if (member.model) nonempty(member.model, 'Model', 800);
      if (member.skills !== undefined) validateSkills(member.skills);
    }
  }
  private task(value: RoomTaskInput, id: string): RoomTask {
    const input = parseSchema<RoomTaskInput>(taskSchema, value, 'task');
    nonempty(input.title, 'Task title', 800); nonempty(input.brief, 'Task brief');
    if (new Set(input.dependsOn).size !== (input.dependsOn?.length ?? 0)) throw new Error('Task dependencies must be unique.');
    return { ...input, id, status: 'pending' };
  }
  private validateDependencies(tasks: RoomTask[]): void {
    const byId = new Map(tasks.map(task => [task.id, task]));
    const done = new Set<string>(), visiting = new Set<string>();
    const visit = (id: string): void => {
      if (done.has(id)) return;
      if (visiting.has(id)) throw new Error('Task dependencies must form an acyclic graph.');
      const task = byId.get(id); if (!task) throw new Error(`Task dependency not found: ${id}`);
      visiting.add(id); for (const dependency of task.dependsOn ?? []) visit(dependency);
      visiting.delete(id); done.add(id);
    };
    for (const task of tasks) visit(task.id);
  }
  private settings(room: RoomRecord, agents: string[]): NonNullable<WorkflowInput['agentSettings']> {
    return Object.fromEntries(room.members.filter(member => agents.includes(member.agent)).map(member => [member.agent,
      { ...(member.model ? { model: member.model } : {}), ...(member.effort ? { effort: member.effort } : {}), ...(member.skills !== undefined ? { skills: [...member.skills] } : {}) }]));
  }
  private taskInput(room: RoomRecord, task: RoomTask, member: RoomMember, sharedContext?: string): WorkflowInput {
    this.validatePrerequisiteEvidence(room, task);
    const peers = room.privateChatMaxRequests ? room.members.filter(candidate => candidate.agent !== member.agent).map(candidate => candidate.agent) : [];
    const names = [member.agent, ...(member.advisor ? [member.advisor] : []), ...peers];
    return { mode: 'delegate', agents: [member.agent], permission: task.permission,
      task: `Room task ${task.id}: ${task.title}\n\n${task.brief}\n\nComplete only this project-manager task. Preserve its stated permission and report verification evidence.`
        + ((task.dependsOn?.length ?? 0) > 0 ? `\n\nPrerequisite tasks: ${task.dependsOn!.join(', ')}. Their complete reports and retained workflow/worktree/patch references are in shared context. Prerequisite worktrees are separate evidence and are never merged into your current workspace automatically. Do not edit those worktrees or assume their changes are already present. Inspect the referenced evidence within your permission and tool policy, and verify the actual files you rely on. If a sandbox or file policy blocks access, report that limitation and the unverified work as incomplete; do not claim it was inspected or integrated.` : ''),
      context: sharedContext ?? this.context(room), contextLimit: room.contextLimit, agentSettings: this.settings(room, names),
      ...(member.advisor ? { advisors: { [member.agent]: { agent: member.advisor, maxRequests: member.advisorMaxRequests ?? 1 } } } : {}),
      ...(peers.length ? { privateChats: { [member.agent]: { peers, maxRequests: room.privateChatMaxRequests! } } } : {}) };
  }
  private discussionInput(room: RoomRecord, round: number, sharedContext?: string): WorkflowInput {
    const members = voters(room), tasks = pending(room);
    const data = { roomId: room.id, round, members: members.map(({ agent, role, weight }) => ({ agent, role, weight })),
      tasks: tasks.map(({ id, title, permission, dependsOn }) => ({ id, title, permission, dependsOn: dependsOn ?? [] })) };
    return { mode: 'parallel', agents: members.map(member => member.agent), permission: 'read',
      task: `Discuss the project manager's pending tasks without changing files or permissions. Independently vote and propose assignments.\nROOM_BALLOT_DATA_BEGIN\n${JSON.stringify(data)}\nROOM_BALLOT_DATA_END\nReturn exactly one JSON object, optionally in one json code fence: {"vote":"approve"|"revise"|"abstain","rationale":"your complete evidence and dissent","assignments":{"task-id":"voting-agent-alias"}}. Approve requires an assignment for EVERY pending task; all aliases and task IDs must come from the data. Rationale must be nonempty. Across the entire turn, assistant text must contain only this ballot. Do not send acknowledgements, progress updates or commentary before, between or after tool calls. Put evidence, decision reasons, uncertainty and dissent inside rationale. Tool use remains subject to the stated read policy. No other fields or surrounding prose.`,
      context: sharedContext ?? this.context(room), contextLimit: room.contextLimit, agentSettings: this.settings(room, members.map(member => member.agent)) };
  }
  private context(room: RoomRecord): string {
    const sections: string[] = [];
    let length = 0;
    const append = (section: string): void => {
      const required = length + (sections.length ? 1 : 0) + section.length;
      if (room.contextLimit !== null && required > room.contextLimit) throw new Error(`Room context requires at least ${required} characters, exceeding the configured context budget ${room.contextLimit}; increase the context budget or use full context.`);
      sections.push(section); length = required;
    };
    append(JSON.stringify({ roomBrief: room.brief }));
    for (const task of room.tasks) {
      const run = task.runId ? this.service.getRun(task.runId) : undefined;
      const workspaces = task.workspaces ?? run?.workspaces ?? (run?.input.permission === 'read' ? [] : undefined);
      append(JSON.stringify({ task: task.id, brief: task.brief, status: task.status, dependsOn: task.dependsOn, runId: task.runId,
        workflowRecordAvailable: task.runId ? Boolean(run) : undefined,
        fullResultResource: run ? `council://runs/${run.id}` : undefined, workspaces,
        workspaceEvidence: !task.runId ? undefined : workspaces === undefined ? 'Workspace metadata is unavailable; do not assume prerequisite changes are present.'
          : workspaces.length ? 'Retained export metadata; inspect paths to verify current access and contents.' : 'The workflow recorded no retained worktrees.' }));
    }
    let cursor: number | undefined;
    do {
      const page = this.store.messagePage(room.id, 100, cursor);
      for (const value of page.messages) append(JSON.stringify({ author: value.author, kind: value.kind, body: value.body, runId: value.runId }));
      if (page.nextCursor === null) break;
      cursor = page.nextCursor;
    } while (true);
    return sections.join('\n');
  }
  private taskWorkspaces(task: RoomTask): WorkspaceArtifact[] | undefined {
    if (task.workspaces !== undefined) return task.workspaces;
    if (!task.runId) return undefined;
    const run = this.service.getRun(task.runId);
    return run?.workspaces ?? (run?.input.permission === 'read' ? [] : undefined);
  }
  private validatePrerequisiteEvidence(room: RoomRecord, task: RoomTask): void {
    for (const id of task.dependsOn ?? []) {
      const prerequisite = room.tasks.find(candidate => candidate.id === id);
      if (!prerequisite || prerequisite.status !== 'completed') continue; // Pending prerequisites have no result yet during planning.
      if (!prerequisite.runId) throw new Error(`Prerequisite ${id} has no retained workflow identity; its completion cannot be verified.`);
      const run = this.service.getRun(prerequisite.runId);
      if (run && run.status !== 'completed') throw new Error(`Prerequisite ${id} workflow ${run.id} is ${run.status}; no dependent task will be dispatched.`);
      const artifacts = run?.workspaces ?? this.taskWorkspaces(prerequisite);
      if (artifacts === undefined) throw new Error(`Prerequisite ${id} has no retained artifact metadata; inspect its workflow before continuing.`);
      if (artifacts.some(artifact => artifact.exportError || artifact.exportStatus !== 'ready')) throw new Error(`Prerequisite ${id} has an unverified or failed workspace export; no dependent task will be dispatched.`);
    }
  }
  private cost(input: WorkflowInput): number { return this.service.engine.plan(input).estimatedCalls; }
  private reservePlan(room: RoomRecord, discussion: boolean, nextRound = 1): void {
    const members = voters(room);
    for (const task of pending(room)) for (const dependency of task.dependsOn ?? []) {
      const prerequisite = room.tasks.find(candidate => candidate.id === dependency);
      if (!prerequisite || prerequisite.status === 'failed' || prerequisite.status === 'cancelled') throw new Error(`Task ${task.id} has an unavailable prerequisite ${dependency}. Create a new task plan before starting provider work.`);
    }
    // One synchronous preflight shares immutable context across candidate plans.
    // Actual dispatch still reads fresh state and checks the instruction revision.
    const sharedContext = this.context(room);
    let required = discussion ? this.cost(this.discussionInput(room, nextRound, sharedContext)) * (room.rounds - nextRound + 1) : 0;
    // Manual discussion stops at ready; execution is admitted and budgeted separately.
    for (const task of !discussion || room.autoExecute ? pending(room) : []) {
      const candidates = !discussion && task.assignee ? members.filter(member => member.agent === task.assignee) : members;
      if (!candidates.length) throw new Error('Task has no valid voting-member assignee.');
      required += Math.max(...candidates.map(member => this.cost(this.taskInput(room, task, member, sharedContext))));
    }
    if (room.callsUsed + required > room.maxCalls) throw new Error(`Room requires up to ${required} further calls, exceeding its remaining call budget ${room.maxCalls - room.callsUsed}.`);
  }

  private retainResult(job: Job, publication: PendingPublication, result: RunRecord): void {
    publication.result = result;
    if (!['completed', 'failed', 'cancelled'].includes(result.status)) throw new Error(UNVERIFIED_COMPLETION);
    if (!publication.reconciled) {
      // Subtract the reservation first: both operands remain exact near MAX_SAFE_INTEGER.
      const calls = BigInt(job.callsUsed) - BigInt(publication.estimated) + BigInt(result.steps.length);
      if (calls < 0n || calls > BigInt(Number.MAX_SAFE_INTEGER)) throw new Error('Room call accounting could not be represented safely. Inspect the retained workflow evidence.');
      job.callsUsed = Number(calls); publication.reconciled = true; delete publication.unverified;
    }
  }

  private mergePublication(room: RoomRecord, publication: PendingPublication, recovery = false): void {
    if (recovery) for (const entry of [publication.dispatch, publication.retained]) {
      if (!room.messages.some(value => value.id === entry.id)) room.messages.push(entry);
    }
    if (!publication.taskId) return;
    const task = room.tasks.find(value => value.id === publication.taskId);
    if (!task) return;
    if (task.runId && task.runId !== publication.runId) {
      publication.conflict = true;
      task.status = 'failed'; task.error = PUBLICATION_CONFLICT;
      return;
    }
    task.runId = publication.runId;
    const result = publication.result;
    if (!result || !['completed', 'failed', 'cancelled'].includes(result.status) || publication.unverified) {
      task.status = 'failed'; task.error = UNVERIFIED_COMPLETION; return;
    }
    task.status = result.status === 'completed' ? 'completed' : result.status === 'cancelled' ? 'cancelled' : 'failed';
    task.workspaces = structuredClone(result.workspaces ?? []);
    task.resultPreview = excerpt(result.output ?? '');
    if (result.error) task.error = result.error; else delete task.error;
    publication.resultMessages ??= [
      message(task.assignee ?? 'system', 'task', `${task.id}: ${task.status}\n${result.output ?? ''}`, result.id),
      ...result.steps.filter(step => step.role.startsWith('advisor-for-')).map(step => message(step.agent, 'advisor', step.result.output, result.id)),
    ];
    for (const entry of publication.resultMessages) if (!room.messages.some(value => value.id === entry.id)) room.messages.push(entry);
  }

  private async collectPublications(job: Job): Promise<void> {
    // Workflow reads and drainage must never await while a room writer is held.
    await Promise.all([...job.publications.values()].map(async publication => {
      if (publication.result && publication.reconciled) return;
      let result: RunRecord | undefined;
      try { result = await this.service.completion(publication.runId); }
      catch { try { result = this.service.getRun(publication.runId); } catch { /* Retain the reservation and link. */ } }
      if (result && ['completed', 'failed', 'cancelled'].includes(result.status)) {
        try { this.retainResult(job, publication, result); return; } catch { /* Unsafe accounting cannot imply verified completion. */ }
      }
      publication.unverified = true;
    }));
  }

  private async launch(id: string, job: Job, input: WorkflowInput, taskId?: string, assertAdmission?: () => void): Promise<RunRecord> {
    this.check(id, job);
    const estimated = this.cost(input);
    let run!: RunRecord;
    const publication: PendingPublication = { runId: '', ...(taskId ? { taskId } : {}), estimated,
      dispatch: message('system', taskId ? 'task' : 'system', taskId ? `Dispatched ${taskId}.` : 'Discussion round dispatched.'),
      retained: message('system', 'system', 'Retained workflow evidence from an owned call whose room publication failed.') };
    let deadline: number | undefined;
    const guard = (): void => {
      if (run) return;
      job.controller.signal.throwIfAborted(); assertAdmission?.();
      if (deadline !== undefined && performance.now() >= deadline) throw new Error('Room workflow admission remained blocked by SQLite contention beyond its five-second retry window. No workflow was admitted for this launch.');
    };
    for (;;) {
      guard();
      const attempt = newAdmissionAttempt(this.store.admissionConnection(), this.service.store.admissionConnection(), guard);
      try {
        runAdmissionAttempt(attempt, () => this.own(id, room => {
          this.available(); this.assertInstructions(room, job);
          if (room.pauseRequested) throw new PauseRequested();
          this.check(id, job); this.service.assertDispatchAllowed(); guard();
          if (estimated > room.maxCalls - job.callsUsed) throw new Error('Room call budget exhausted before dispatch.');
          run = this.service.start(input, { roomId: id });
          publication.runId = run.id; publication.dispatch.runId = run.id; publication.retained.runId = run.id;
          job.runs.add(run.id); job.publications.set(run.id, publication); job.callsUsed += estimated;
          room.callsUsed = job.callsUsed;
          if (taskId) { const task = room.tasks.find(value => value.id === taskId)!; task.status = 'running'; task.runId = run.id; }
          room.messages.push(publication.dispatch);
        }));
        break;
      } catch (error) {
        if (run || !consumeAdmissionRefusal(attempt, error)) throw error;
        deadline ??= performance.now() + 5000;
        guard();
        try { await delay(Math.min(25, Math.max(1, deadline - performance.now())), undefined, { signal: job.controller.signal }); }
        catch (waitingError) { job.controller.signal.throwIfAborted(); throw waitingError; }
        guard();
      }
    }
    try {
      const result = await this.service.completion(run.id);
      this.retainResult(job, publication, result);
      this.own(id, room => { room.callsUsed = job.callsUsed; });
      if (!taskId) job.publications.delete(run.id);
      return result;
    } finally { job.runs.delete(run.id); }
  }
  private ballot(output: string, room: RoomRecord): Ballot {
    const source = output.trim();
    const fenced = /^```(?:json)?\s*\n([\s\S]*?)\n```$/i.exec(source);
    const json = fenced ? fenced[1]! : source;
    checkAgentJsonStructure(Buffer.from(json));
    let value: unknown;
    try { value = JSON.parse(json); } catch { throw new Error('Agent did not return one complete JSON ballot.'); }
    assertUniqueObjectKeys(json, 'Ballot');
    const ballot = parseSchema<Ballot>(ballotSchema, value, 'ballot'); nonempty(ballot.rationale, 'Ballot rationale');
    const taskIds = new Set(pending(room).map(task => task.id)), aliases = new Set(voters(room).map(member => member.agent));
    for (const [task, agent] of Object.entries(ballot.assignments)) if (!taskIds.has(task) || !aliases.has(agent)) throw new Error('Ballot assignment names an unknown task or voting member.');
    if (ballot.vote === 'approve' && Object.keys(ballot.assignments).length !== taskIds.size) throw new Error('An approve ballot must assign every pending task.');
    return ballot;
  }
  private decide(room: RoomRecord, ballots: Map<string, Ballot>): RoomDecision {
    const members = voters(room);
    const approved = members.filter(member => ballots.get(member.agent)!.vote === 'approve');
    const winner = approved.sort((a, b) => b.weight - a.weight || Number(b.role === 'lead') - Number(a.role === 'lead') || room.members.indexOf(a) - room.members.indexOf(b))[0];
    // Align exact decimal coefficients across scientific notation and subnormal
    // values. Neither binary rounding nor a floating-point overflow can approve a tie.
    const weights = members.map(member => decimalWeight(member.weight));
    const exponent = Math.min(...weights.map(weight => weight.exponent));
    const units = new Map(members.map((member, index) => [member.agent,
      weights[index]!.coefficient * 10n ** BigInt(weights[index]!.exponent - exponent)]));
    const totalUnits = members.reduce((sum, member) => sum + units.get(member.agent)!, 0n);
    const approvedUnits = approved.reduce((sum, member) => sum + units.get(member.agent)!, 0n);
    const approveWeight = displayedWeight(approvedUnits, exponent), totalWeight = displayedWeight(totalUnits, exponent);
    const assignments = winner ? ballots.get(winner.agent)!.assignments : {};
    return { approved: approvedUnits * 2n > totalUnits, approveWeight, totalWeight, instructionRevision: room.instructionRevision ?? 0,
      assignments,
      rationale: winner ? ballots.get(winner.agent)!.rationale : 'No voting member approved the proposed task assignments.',
      dissent: members.filter(member => { const ballot = ballots.get(member.agent)!; return ballot.vote !== 'approve'
        || Object.keys(assignments).some(task => ballot.assignments[task] !== assignments[task]); })
        .map(member => { const ballot = ballots.get(member.agent)!; return `${member.agent} (${ballot.vote}${ballot.vote === 'approve' ? `; alternative assignments ${JSON.stringify(ballot.assignments)}` : ''}): ${ballot.rationale}`; }) };
  }
  private async discuss(id: string, job: Job): Promise<void> {
    const checkpoint = this.get(id).checkpoint;
    for (let round = checkpoint?.phase === 'discussion' ? checkpoint.nextRound : 1; round <= this.get(id).rounds; round++) {
      this.check(id, job);
      this.assertInstructions(this.get(id), job);
      const before = this.get(id), run = await this.launch(id, job, this.discussionInput(before, round));
      this.check(id, job);
      const ballots = new Map<string, Ballot>();
      const failures = run.steps.filter(step => step.role === 'worker' && step.result.status !== 'completed')
        .map(step => `${step.agent} ${step.result.status}${step.result.error ? `: ${step.result.error}` : ''}`);
      let invalid: string | undefined = run.status === 'completed' ? undefined
        : `Discussion workflow ${run.id} ${run.status}; no tasks will execute.\n${excerpt(failures.join('; ') || run.error || 'A discussion ballot did not complete.')}`;
      for (const member of voters(before)) {
        const steps = run.steps.filter(step => step.agent === member.agent && step.role === 'worker');
        const step = steps[0];
        if (run.status !== 'completed' || steps.length !== 1 || !step || step.result.status !== 'completed') {
          invalid ??= `${member.agent} did not return one completed ballot in discussion workflow ${run.id}; no tasks will execute.`;
          if (step) this.own(id, room => { room.messages.push(message(member.agent, 'opinion', `${step.result.status}: ${step.result.output}${step.result.error ? `\n\nError: ${step.result.error}` : ''}`, run.id)); });
          continue;
        }
        let ballot: Ballot;
        try { ballot = this.ballot(step.result.output, before); }
        catch (error) {
          invalid ??= `Invalid ballot from ${member.agent} in discussion workflow ${run.id}: ${error instanceof Error ? error.message : 'Invalid JSON ballot.'}`;
          this.own(id, room => { room.messages.push(message(member.agent, 'opinion', `Invalid ballot: ${step.result.output}`, run.id)); });
          continue;
        }
        ballots.set(member.agent, ballot);
        this.own(id, room => { room.messages.push(message(member.agent, 'opinion', JSON.stringify(ballot), run.id)); });
      }
      this.assertInstructions(this.get(id), job);
      if (invalid) {
        this.own(id, room => { room.status = 'needs_attention'; room.error = invalid; room.messages.push(message('system', 'decision', invalid!)); }, true); return;
      }
      const decision = this.decide(before, ballots);
      this.own(id, room => {
        this.assertInstructions(room, job);
        room.decision = decision;
        room.checkpoint = { phase: 'discussion', nextRound: round + 1 };
        room.messages.push(message('system', 'decision', `Round ${round}: ${decision.approved ? 'approved' : 'revision required'} (${decision.approveWeight}/${decision.totalWeight} approval weight). ${decision.rationale}`));
        if (decision.approved) {
          for (const task of pending(room)) { task.assignee = decision.assignments[task.id]; task.status = 'assigned'; }
          room.status = room.autoExecute ? 'running' : 'ready';
          room.checkpoint = { phase: room.autoExecute ? 'execution' : 'ready' };
        }
      });
      if (decision.approved) {
        if (before.autoExecute) await this.executeTasks(id, job);
        else this.own(id, () => {}, true);
        return;
      }
    }
    this.own(id, room => { room.status = 'needs_attention'; room.error = 'No weighted majority approved the tasks within the configured discussion rounds.'; }, true);
  }
  private async executeTasks(id: string, job: Job): Promise<void> {
    this.check(id, job);
    this.assertInstructions(this.get(id), job);
    this.reservePlan(this.get(id), false);
    const running = new Map<string, Promise<string>>();
    let stopped: { reason: unknown; wasAcceptedCancellation: boolean } | undefined;
    const retainFailure = (reason: unknown): void => {
      // Capture cancellation classification now: a later cancel must not relabel
      // an earlier real fault that already became the controller's abort reason.
      const wasAcceptedCancellation = acceptedCancellation(job, reason);
      const upgradesCancellation = stopped?.wasAcceptedCancellation && !wasAcceptedCancellation && reason !== stopped.reason
        && !(reason instanceof PauseRequested) && !(reason instanceof InstructionsChanged);
      if (!stopped || stopped.reason instanceof PauseRequested || upgradesCancellation
        || stopped.reason instanceof InstructionsChanged && !(reason instanceof PauseRequested)) stopped = { reason, wasAcceptedCancellation };
    };
    const executeTask = async (room: RoomRecord, task: RoomTask): Promise<void> => {
      const member = room.members.find(value => value.agent === task.assignee && value.role !== 'advisor');
      if (!member) throw new Error('Approved task assignee is unavailable.');
      const run = await this.launch(id, job, this.taskInput(room, task, member), task.id, () => { if (stopped) throw stopped.reason; });
      const publication = job.publications.get(run.id)!;
      this.own(id, current => { this.mergePublication(current, publication); });
      if (publication.conflict) throw new Error(PUBLICATION_CONFLICT);
      job.publications.delete(run.id);
      if (run.status !== 'completed') {
        if (run.status === 'cancelled') {
          if (job.acceptedCancellationReason !== undefined) throw job.acceptedCancellationReason;
          if (job.failure !== undefined) throw job.failure;
        }
        throw new Error(`Task ${task.id} ${run.status}: ${run.error ?? 'Incomplete workflow.'}`);
      }
    };
    for (;;) {
      if (!stopped) {
        try {
          this.check(id, job);
          const room = this.get(id);
          this.assertInstructions(room, job);
          const remaining = pending(room);
          if (room.pauseRequested && remaining.length) throw new PauseRequested();
          const ready = remaining.filter(task => !running.has(task.id) && (task.dependsOn ?? []).every(dependency => room.tasks.find(value => value.id === dependency)?.status === 'completed'))
            .slice(0, Math.max(0, this.service.config.maxConcurrency - running.size));
          if (remaining.length && !ready.length && !running.size) throw new Error('Pending task dependencies cannot be satisfied; no further task was dispatched.');
          for (const task of ready) {
            // Handle rejection immediately, before another settled task can
            // release a slot. No rejected call is left unobserved while draining.
            const done = executeTask(room, task).then(() => task.id, error => { retainFailure(error); return task.id; });
            running.set(task.id, done);
          }
        } catch (error) { retainFailure(error); }
      }
      if (!running.size) break;
      // Refill a free slot as soon as any prerequisite completes, rather than
      // waiting for an unrelated slow member of a fixed dispatch wave.
      running.delete(await Promise.race(running.values()));
    }
    // A real task/publication failure must survive a cancellation accepted while draining.
    if (stopped && !stopped.wasAcceptedCancellation && !(stopped.reason instanceof PauseRequested) && !(stopped.reason instanceof InstructionsChanged)) throw stopped.reason;
    this.check(id, job);
    if (stopped) throw stopped.reason;
    this.own(id, room => {
      this.assertInstructions(room, job);
      if (room.tasks.some(task => task.status !== 'completed')) throw new Error('Not every room task completed.');
      room.status = 'completed'; delete room.error;
      room.messages.push(message('system', 'system', 'All room tasks completed. Workflow records retain the reported results and checks; review them before accepting changes. Write changes remain in isolated worktrees.'));
    }, true);
  }
}
