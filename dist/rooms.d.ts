import type { CouncilService } from './service.js';
import type { RoomInput, RoomMessage, RoomRecord, RoomSummary, RoomTaskInput } from './rooms-types.js';
/** A durable human-managed task board; provider calls use the existing workflow service. */
export declare class RoomManager {
    private readonly service;
    private readonly store;
    private readonly owner;
    private readonly jobs;
    private readonly listeners;
    private readonly emergency;
    private closing;
    private fault?;
    private closePromise?;
    private readonly shutdown;
    constructor(service: CouncilService);
    subscribe(listener: (room: RoomRecord) => void): () => void;
    list(): RoomSummary[];
    get(id: string): RoomRecord;
    messagePage(id: string, limit?: number, cursor?: number): {
        messages: RoomMessage[];
        nextCursor: number | null;
    };
    create(value: RoomInput): RoomRecord;
    postMessage(id: string, value: {
        author: string;
        body: string;
    }): RoomRecord;
    addTask(id: string, value: RoomTaskInput): RoomRecord;
    startDiscussion(id: string): RoomRecord;
    execute(id: string): RoomRecord;
    cancel(id: string): RoomRecord;
    wait(id: string, timeoutMs?: number, signal?: AbortSignal): Promise<RoomRecord>;
    close(): Promise<void>;
    private available;
    private assertBackend;
    private assertInstructions;
    private invalidateDecision;
    private publish;
    private change;
    private own;
    private abort;
    private check;
    private queue;
    private abortRuns;
    private validateMembers;
    private task;
    private validateDependencies;
    private settings;
    private taskInput;
    private discussionInput;
    private context;
    private cost;
    private reservePlan;
    private launch;
    private ballot;
    private decide;
    private discuss;
    private executeTasks;
}
