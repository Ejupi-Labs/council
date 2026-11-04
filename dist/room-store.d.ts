import type { RoomMessage, RoomRecord, RoomSummary } from './rooms-types.js';
/** Room state has its own database so optional UI features never migrate workflow storage. */
export declare class RoomStore {
    private readonly db;
    constructor(stateDir: string);
    private transaction;
    private row;
    private append;
    insert(room: RoomRecord): void;
    get(id: string): RoomRecord | undefined;
    /** Read coordination state without fetching or decoding any chat history. */
    metadata(id: string): Omit<RoomRecord, 'messages'> | undefined;
    list(): RoomSummary[];
    mutate(id: string, change: (room: RoomRecord) => void, options?: {
        owner?: string;
        claim?: {
            id: string;
            pid: number;
        };
        release?: boolean;
        requireIdle?: boolean;
        preserveFailure?: boolean;
    }): RoomRecord;
    requestCancel(id: string, options?: {
        activeOnly?: boolean;
    }): RoomRecord;
    requestPause(id: string): RoomRecord;
    cancelled(id: string, owner: string): boolean;
    messagePage(id: string, limit?: number, cursor?: number): {
        messages: RoomMessage[];
        nextCursor: number | null;
    };
    recover(id?: string): void;
    close(): void;
}
