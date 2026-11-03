import { createInterface } from 'node:readline';

// Minimal local ACP server: this fixture does not import a vendor SDK or contact a model.
const send = event => process.stdout.write(JSON.stringify({ jsonrpc: '2.0', ...event }) + '\n');
let pendingPrompt;
for await (const line of createInterface({ input: process.stdin, crlfDelay: Infinity })) {
  const event = JSON.parse(line);
  if (event.method === 'initialize') {
    send({ id: event.id, result: { protocolVersion: event.params.protocolVersion, agentCapabilities: {},
      agentInfo: { name: 'council-local-fixture', version: '1.0.0' }, authMethods: [] } });
  } else if (event.method === 'session/new') {
    send({ id: event.id, result: { sessionId: 'council-fixture-session', models: {
      currentModelId: 'fixture-default', availableModels: [{ modelId: 'fixture-default', name: 'Fixture default' }, { modelId: 'fixture-alternate', name: 'Fixture alternate' }],
    } } });
  } else if (event.method === 'session/prompt') {
    pendingPrompt = event.id;
    if (!event.params.prompt.some(content => content.type === 'text' && content.text.includes('request-write-permission'))) {
      send({ method: 'session/update', params: { sessionId: 'council-fixture-session', update: { sessionUpdate: 'agent_message_chunk',
        content: { type: 'text', text: `Local ACP fixture completed; depth=${process.env.COUNCIL_DEPTH}` } } } });
      send({ id: pendingPrompt, result: { stopReason: 'end_turn' } });
      continue;
    }
    send({ id: 'fixture-permission', method: 'session/request_permission', params: { sessionId: 'council-fixture-session',
      toolCall: { toolCallId: 'fake-write', title: 'Write a fixture file', kind: 'edit', status: 'pending' },
      options: [{ optionId: 'allow', name: 'Allow once', kind: 'allow_once' }, { optionId: 'deny', name: 'Reject once', kind: 'reject_once' }],
    } });
  } else if (event.id === 'fixture-permission' && event.result) {
    send({ method: 'session/update', params: { sessionId: 'council-fixture-session', update: { sessionUpdate: 'agent_message_chunk',
      content: { type: 'text', text: `Local ACP fixture: ${JSON.stringify(event.result.outcome)}; depth=${process.env.COUNCIL_DEPTH}` } } } });
    send({ id: pendingPrompt, result: { stopReason: 'end_turn' } });
  } else if (event.method && event.id !== undefined) {
    send({ id: event.id, result: {} });
  }
}
