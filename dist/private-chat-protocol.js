import { parseSchema } from './schema.js';
import { assertUniqueObjectKeys } from './control-json.js';
/** Only an entire exact envelope yields control; examples and surrounding prose remain ordinary output. */
export function privateChatRequest(output) {
    let value;
    try {
        value = JSON.parse(output);
    }
    catch {
        return undefined;
    }
    if (!value || typeof value !== 'object' || Array.isArray(value) || Object.keys(value).length !== 1 || !Object.hasOwn(value, 'council_private_chat_request'))
        return undefined;
    assertUniqueObjectKeys(output, 'Private chat request');
    let request;
    try {
        request = parseSchema({ type: 'object', additionalProperties: false, required: ['peer', 'message'], properties: {
                peer: { type: 'string', minLength: 1, maxLength: 64 }, message: { type: 'string', minLength: 1 }, context: { type: 'string', default: '' },
            } }, value.council_private_chat_request, 'council_private_chat_request');
    }
    catch {
        // Schema diagnostics can echo unsupported property names containing private
        // evidence. Public workflow errors must describe the contract without it.
        throw new Error('Private chat request must contain only valid peer, message and context fields.');
    }
    if (!request.peer.trim() || !request.message.trim())
        throw new Error('Private chat peer and message must contain non-whitespace text.');
    return request;
}
/** Detect a candidate even when its grammar is invalid, so private request bodies are not saved as public failed output. */
export function privateChatCandidate(output) {
    // Markdown wrappers are not valid requests, but a worker can add one by
    // mistake. Treat unmistakable whole-response control output as private so
    // malformed formatting cannot turn the pair's evidence into a public result.
    const fence = /^\s*```(?:json)?[ \t]*\r?\n/i.exec(output);
    const candidate = fence ? output.slice(fence[0].length) : output;
    try {
        const value = JSON.parse(candidate);
        return Boolean(value && typeof value === 'object' && !Array.isArray(value) && Object.hasOwn(value, 'council_private_chat_request'));
    }
    catch { /* A cut-off envelope can still contain private evidence. */ }
    const root = /^\s*\{/.exec(candidate);
    if (!root)
        return false;
    let depth = 1;
    for (let index = root[0].length; index < candidate.length && depth > 0; index++) {
        const character = candidate[index];
        if (character === '{' || character === '[')
            depth++;
        else if (character === '}' || character === ']')
            depth--;
        else if (character === '"') {
            const start = index;
            while (++index < candidate.length) {
                if (candidate[index] === '\\')
                    index++;
                else if (candidate[index] === '"')
                    break;
            }
            if (depth !== 1)
                continue;
            let next = index + 1;
            while (/\s/.test(candidate[next] ?? ''))
                next++;
            if (candidate[next] !== ':')
                continue;
            try {
                if (JSON.parse(candidate.slice(start, index + 1)) === 'council_private_chat_request')
                    return true;
            }
            catch { /* Invalid earlier fields do not create an accepted request. */ }
        }
    }
    return false;
}
//# sourceMappingURL=private-chat-protocol.js.map