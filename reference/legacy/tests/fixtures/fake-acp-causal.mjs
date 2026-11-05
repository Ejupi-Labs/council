import { createInterface } from 'node:readline';

// Local protocol fixture only: no vendor SDK, model or credentials.
const sessionId = 'causal-fixture-session';
const send = value => process.stdout.write(JSON.stringify({ jsonrpc: '2.0', ...value }) + '\n');
for await (const line of createInterface({ input: process.stdin, crlfDelay: Infinity })) {
  const request = JSON.parse(line);
  if (request.method === 'initialize') send({ id: request.id, result: { protocolVersion: 1 } });
  else if (request.method === 'session/new') send({ id: request.id, result: { sessionId } });
  else if (request.method === 'session/prompt') {
    const prompt = request.params.prompt[0].text;
    if (prompt === 'permission') {
      send({ id: 'permission', method: 'session/request_permission', params: { sessionId,
        toolCall: { kind: 'read', title: 'PRIVATE_TOOL_TITLE' },
        options: [{ kind: 'allow_once', optionId: 'allow' }, { kind: 'reject_once', optionId: 'reject' }] } });
    } else {
      send({ method: 'session/update', params: { sessionId, update: { sessionUpdate: 'agent_message_chunk',
        content: { type: 'text', text: 'Partial causal fixture evidence.' } } } });
      send({ id: request.id, result: { stopReason: prompt === 'cancelled' ? 'cancelled' : 'end_turn',
        status: 'failed', error: 'PROVIDER_SPOOFED_CAUSE', trustedRunnerFailure: 'failed' } });
    }
  }
}
