import { createInterface } from 'node:readline';

// Minimal local ACP server: this fixture does not import a vendor SDK or contact a model.
const send = event => process.stdout.write(JSON.stringify({ jsonrpc: '2.0', ...event }) + '\n');
let pendingPrompt;
for await (const line of createInterface({ input: process.stdin, crlfDelay: Infinity })) {
  const event = JSON.parse(line);
  if (event.method === 'initialize') {
    if (!event.params.clientCapabilities.fs.readTextFile || event.params.clientCapabilities.terminal) throw Error('Only the bounded filesystem capability is supported');
    send({ id: event.id, result: { protocolVersion: process.env.COUNCIL_TEST_ACP_CASE === 'version' ? 99 : event.params.protocolVersion, agentCapabilities: {},
      agentInfo: { name: 'council-local-fixture', version: '1.0.0' }, authMethods: [] } });
  } else if (event.method === 'session/new') {
    if (process.env.COUNCIL_TEST_ACP_CASE === 'auth') { send({ id: event.id, error: { code: -32000, message: 'auth_required SECRET_ACCESS_TOKEN=do-not-print' } }); continue; }
    send({ id: event.id, result: { sessionId: 'council-fixture-session', models: {
      currentModelId: 'fixture-default', availableModels: [{ modelId: 'fixture-default', name: 'Fixture default' }, { modelId: 'fixture-alternate', name: 'Fixture alternate' }],
    } } });
  } else if (event.method === 'session/prompt') {
    pendingPrompt = event.id;
    if (process.env.COUNCIL_TEST_ACP_CASE === 'codex-routing-env') {
      const text = JSON.stringify(Object.fromEntries(['CODEX_WINDOWS_REGISTERED_CORE', 'CODEX_WINDOWS_SANDBOX_PACKAGE_FAMILY', 'CODEX_HOME', 'COUNCIL_DEPTH'].map(key => [key, process.env[key]])));
      send({ method: 'session/update', params: { sessionId: 'council-fixture-session', update: { sessionUpdate: 'agent_message_chunk', content: { type: 'text', text } } } });
      send({ id: pendingPrompt, result: { stopReason: 'end_turn' } });
      continue;
    }
    if (process.env.COUNCIL_TEST_ACP_CASE?.startsWith('structure-')) {
      send({ method: 'session/update', params: { sessionId: 'council-fixture-session', update: { sessionUpdate: 'agent_message_chunk', content: { type: 'text', text: 'Partial offline evidence.\n' } } } });
      if (process.env.COUNCIL_TEST_ACP_CASE === 'structure-deep') process.stdout.write('{"jsonrpc":"2.0","method":"fixture/progress","params":{"payload":' + '['.repeat(300_000) + '0' + ']'.repeat(300_000) + '}}\n');
      else if (process.env.COUNCIL_TEST_ACP_CASE === 'structure-wide') send({ method: 'fixture/progress', params: { payload: Array(70_000).fill(0) } });
      else send({ method: 'fixture/progress', params: { payload: '[{"nested":"\\\"🌍"}]'.repeat(10_000) } });
    }
    if (['fs-read', 'fs-write', 'fs-flood', 'fs-session'].includes(process.env.COUNCIL_TEST_ACP_CASE)) {
      const kind = process.env.COUNCIL_TEST_ACP_CASE;
      for (let index = 0; index < (kind === 'fs-flood' ? 9 : 1); index++) send({ id: `fixture-file-${index}`, method: kind === 'fs-write' ? 'fs/write_text_file' : 'fs/read_text_file',
        params: { sessionId: kind === 'fs-session' ? 'wrong-session' : 'council-fixture-session', path: process.env.COUNCIL_TEST_FILE,
          ...(kind === 'fs-write' ? { content: 'changed by ACP fixture' } : { line: null, limit: null }) } });
      continue;
    }
    if (process.env.COUNCIL_TEST_ACP_CASE === 'unknown-request') {
      send({ id: 'fs-request', method: 'fs/write_text_file', params: { sessionId: 'council-fixture-session', path: '/not-used', content: 'Never written' } }); continue;
    }
    if (process.env.COUNCIL_TEST_ACP_CASE === 'hang') { setInterval(() => {}, 1000); continue; }
    if (['provider-auth-answer', 'provider-auth-example'].includes(process.env.COUNCIL_TEST_ACP_CASE)) {
      const text = process.env.COUNCIL_TEST_ACP_CASE === 'provider-auth-answer' ? 'Authentication data is invalid or missing'
        : 'An example authentication diagnostic is "Authentication data is invalid or missing". Sign in again to resolve it.';
      send({ method: 'session/update', params: { sessionId: 'council-fixture-session', update: { sessionUpdate: 'agent_message_chunk', content: { type: 'text', text } } } });
      send({ id: pendingPrompt, result: { stopReason: 'end_turn' } });
      continue;
    }
    if (!event.params.prompt.some(content => content.type === 'text' && content.text.includes('request-write-permission'))) {
      send({ method: 'session/update', params: { sessionId: 'council-fixture-session', update: { sessionUpdate: 'agent_message_chunk',
        content: { type: 'text', text: `Local ACP fixture completed; depth=${process.env.COUNCIL_DEPTH}` } } } });
      send({ id: pendingPrompt, result: { stopReason: 'end_turn' }, ...(process.env.COUNCIL_TEST_ACP_CASE === 'mixed-result-error' ? { error: 'provider failure' } : {}) });
      continue;
    }
    send({ id: 'fixture-permission', method: 'session/request_permission', params: { sessionId: 'council-fixture-session',
      toolCall: { toolCallId: 'fake-write', title: 'Write a fixture file', kind: process.env.COUNCIL_TEST_ACP_CASE === 'read-permission' ? 'read' : 'edit', status: 'pending' },
      options: [{ optionId: process.env.COUNCIL_TEST_ACP_CASE === 'question' ? 'interaction_yes' : 'allow', name: 'Allow once', kind: 'allow_once' }, { optionId: 'deny', name: 'Reject once', kind: 'reject_once' }],
    } });
  } else if (typeof event.id === 'string' && event.id.startsWith('fixture-file-') && event.result) {
    send({ method: 'session/update', params: { sessionId: 'council-fixture-session', update: { sessionUpdate: 'agent_message_chunk', content: { type: 'text', text: JSON.stringify(event.result) } } } });
    send({ id: pendingPrompt, result: { stopReason: 'end_turn' } });
  } else if (event.id === 'fixture-permission' && event.result) {
    send({ method: 'session/update', params: { sessionId: 'council-fixture-session', update: { sessionUpdate: 'agent_message_chunk',
      content: { type: 'text', text: `Local ACP fixture: ${JSON.stringify(event.result.outcome)}; depth=${process.env.COUNCIL_DEPTH}` } } } });
    send({ id: pendingPrompt, result: { stopReason: 'end_turn' } });
  } else if (event.method && event.id !== undefined) {
    send({ id: event.id, result: {} });
  }
}
