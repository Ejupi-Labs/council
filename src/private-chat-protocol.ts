import { parseSchema } from './schema.js';

export interface PrivateChatRequest { peer: string; message: string; context: string }

/** Only an entire exact envelope yields control; examples and surrounding prose remain ordinary output. */
export function privateChatRequest(output: string): PrivateChatRequest | undefined {
  let value: unknown;
  try { value = JSON.parse(output); } catch { return undefined; }
  if (!value || typeof value !== 'object' || Array.isArray(value) || Object.keys(value).length !== 1 || !Object.hasOwn(value, 'council_private_chat_request')) return undefined;
  // JSON.parse accepts duplicate keys. An orchestration request must have one
  // unambiguous peer and message, including when keys use Unicode escapes.
  const scopes: Set<string>[] = [];
  for (let index = 0; index < output.length; index++) {
    if (output[index] === '{') scopes.push(new Set());
    else if (output[index] === '}') scopes.pop();
    else if (output[index] === '"') {
      const start = index;
      while (++index < output.length) {
        if (output[index] === '\\') index++;
        else if (output[index] === '"') break;
      }
      let next = index + 1;
      while (/\s/.test(output[next] ?? '')) next++;
      if (output[next] === ':') {
        const key = JSON.parse(output.slice(start, index + 1)) as string, scope = scopes.at(-1)!;
        if (scope.has(key)) throw new Error('Private chat request contains duplicate JSON object keys.');
        scope.add(key);
      }
    }
  }
  let request: PrivateChatRequest;
  try {
    request = parseSchema<PrivateChatRequest>({ type: 'object', additionalProperties: false, required: ['peer', 'message'], properties: {
      peer: { type: 'string', minLength: 1, maxLength: 64 }, message: { type: 'string', minLength: 1 }, context: { type: 'string', default: '' },
    } }, (value as Record<string, unknown>).council_private_chat_request, 'council_private_chat_request');
  } catch {
    // Schema diagnostics can echo unsupported property names containing private
    // evidence. Public workflow errors must describe the contract without it.
    throw new Error('Private chat request must contain only valid peer, message and context fields.');
  }
  if (!request.peer.trim() || !request.message.trim()) throw new Error('Private chat peer and message must contain non-whitespace text.');
  return request;
}

/** Detect a candidate even when its grammar is invalid, so private request bodies are not saved as public failed output. */
export function privateChatCandidate(output: string): boolean {
  try {
    const value: unknown = JSON.parse(output);
    if (value && typeof value === 'object' && !Array.isArray(value) && Object.hasOwn(value, 'council_private_chat_request')) return true;
  } catch { /* A cut-off envelope can still contain private evidence. */ }
  const match = /^\s*\{\s*("(?:[^"\\]|\\.)*")\s*:/.exec(output);
  if (!match) return false;
  try { return JSON.parse(match[1]!) === 'council_private_chat_request'; } catch { return false; }
}
