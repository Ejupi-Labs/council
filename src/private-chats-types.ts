/** Participant names identify task instances, not authenticated users or provider aliases. */
export interface PrivateChatInput {
  participants: [string, string];
  roomId?: string;
}
export interface PrivateChat {
  id: string;
  participants: [string, string];
  roomId?: string;
  createdAt: string;
  lastMessageAt?: string;
  messageCount: number;
  lastSequence: number;
}
export interface PrivateChatSendInput {
  chatId: string;
  from: string;
  body: string;
  runId?: string;
  /** Optional retry key, scoped to this chat and sender. Reusing it with different content fails. */
  idempotencyKey?: string;
}
export interface PrivateChatMessage extends Omit<PrivateChatSendInput, 'idempotencyKey'> {
  id: string;
  sequence: number;
  createdAt: string;
}
export interface PrivateChatPageInput {
  chatId: string;
  actor: string;
  after?: number;
  limit?: number;
}
export interface PrivateChatPage {
  messages: PrivateChatMessage[];
  nextCursor: number | null;
}
export interface PrivateChatWatchInput {
  chatId: string;
  actor: string;
  after?: number;
  timeoutMs?: number;
}
export interface PrivateChatWatchResult {
  reason: 'changed' | 'timeout';
  chat: PrivateChat;
  lastSequence: number;
}
