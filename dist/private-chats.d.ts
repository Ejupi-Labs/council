import type { CouncilService } from './service.js';
import type { PrivateChat, PrivateChatInput, PrivateChatMessage, PrivateChatPage, PrivateChatPageInput, PrivateChatSendInput, PrivateChatWatchInput, PrivateChatWatchResult } from './private-chats-types.js';
export declare class PrivateChatConflict extends Error {
    readonly code = "COUNCIL_PRIVATE_CHAT_CONFLICT";
    readonly status = 409;
    constructor(message: string);
}
/** Pair-scoped collaboration history. Actor labels are routing metadata, not an authentication boundary. */
export declare class PrivateChatManager {
    private readonly service;
    private readonly db;
    private readonly shutdown;
    private readonly listeners;
    private closed;
    constructor(service: CouncilService);
    private open;
    private transaction;
    private row;
    private member;
    private cursor;
    private notify;
    create(value: PrivateChatInput): PrivateChat;
    send(value: PrivateChatSendInput): PrivateChatMessage;
    page(value: PrivateChatPageInput): PrivateChatPage;
    list(actor: string, roomId?: string): PrivateChat[];
    actors(roomId?: string): string[];
    get(chatId: string, actor: string): PrivateChat;
    watch(value: PrivateChatWatchInput, signal?: AbortSignal): Promise<PrivateChatWatchResult>;
    close(): void;
}
