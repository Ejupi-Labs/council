import { spawn } from 'node:child_process';
import { writeFileSync } from 'node:fs';

let prompt = '';
for await (const chunk of process.stdin) prompt += chunk.toString();
const send = event => process.stdout.write(JSON.stringify({ jsonrpc: '2.0', ...event }) + '\n');
const request = () => send({ id: 'prompt-1', method: 'session/prompt', params: { sessionId: 'fake-session', prompt: [{ type: 'text', text: prompt }] } });
const chunk = (text, kind = 'agent_message_chunk') => send({ method: 'session/update', params: { sessionId: 'fake-session', update: { sessionUpdate: kind, content: { type: 'text', text } } } });
const complete = (stopReason = 'end_turn', id = 'prompt-1') => send({ id, result: { stopReason } });

if (prompt === 'hang' || prompt === 'tree-hang') {
  if (prompt === 'tree-hang') {
    const child = spawn(process.execPath, ['-e', 'setInterval(() => {}, 1000)'], { stdio: 'ignore', windowsHide: true });
    if (process.env.COUNCIL_TEST_PID_FILE) writeFileSync(process.env.COUNCIL_TEST_PID_FILE, String(child.pid));
  }
  setInterval(() => {}, 1000);
} else if (prompt === 'malformed') {
  request(); chunk('partial answer'); process.stdout.write('{broken');
} else if (prompt === 'error') {
  request(); send({ id: 'prompt-1', error: { code: -32000, message: 'SECRET_ACCESS_TOKEN=do-not-print' } });
} else if (prompt === 'dependency-error') {
  request(); send({ id: 'prompt-1', error: { code: -32603, message: 'ETARGET No matching version found for dependency. SECRET_ACCESS_TOKEN=do-not-print', data: { detailCode: 'AGENT_STARTUP_FAILED' } } });
} else if (prompt === 'provider-model-error') {
  request(); chunk('Warning: Model metadata for `fixture-model` not found. Defaulting to fallback metadata; this can degrade performance and cause issues.\n\n');
  chunk(JSON.stringify({ type: 'error', status: 400, error: { type: 'invalid_request_error', code: 'model_not_found', message: 'SECRET_ACCESS_TOKEN=do-not-print' } })); complete();
} else if (prompt === 'provider-error-example') {
  request(); chunk('The API can return this example:\n```json\n{"type":"error","status":400,"error":{"code":"model_not_found"}}\n```\nCheck the configured model to resolve it.'); complete();
} else if (prompt === 'oversize') {
  process.stdout.write('x'.repeat(5 * 1024 * 1024)); setInterval(() => {}, 1000);
} else if (prompt === 'stderr-oversize') {
  process.stderr.write('SECRET_ACCESS_TOKEN=do-not-print'.repeat(5000)); setInterval(() => {}, 1000);
} else if (prompt === 'answer-oversize') {
  request(); chunk('x'.repeat(20_000)); complete();
} else if (prompt === 'nonzero') {
  request(); chunk('looks finished'); complete(); process.stderr.write('SECRET_ACCESS_TOKEN=do-not-print'); process.exitCode = 2;
} else if (prompt === 'partial') {
  request(); chunk('unfinished');
} else if (prompt === 'empty') {
  request(); complete();
} else if (prompt === 'max-tokens') {
  request(); chunk('incomplete'); complete('max_tokens');
} else if (prompt === 'cancelled') {
  request(); chunk('cancelled'); complete('cancelled');
} else if (prompt === 'wrong-id') {
  request(); chunk('wrong completion'); complete('end_turn', 'other');
} else if (prompt === 'late-text') {
  request(); chunk('first'); complete(); chunk('late');
} else {
  request(); chunk('hidden thought', 'agent_thought_chunk');
  send({ method: 'session/update', params: { sessionId: 'fake-session', update: { sessionUpdate: 'tool_call', content: { type: 'text', text: 'hidden tool' } } } });
  const text = prompt === 'args' ? JSON.stringify(process.argv.slice(2))
    : prompt === 'depth' ? process.env.COUNCIL_DEPTH
    : prompt.startsWith('large-input:') ? 'Large input accepted.' : `Hello 🌍: ${prompt}`;
  const wire = JSON.stringify({ jsonrpc: '2.0', method: 'session/update', params: { sessionId: 'fake-session', update: { sessionUpdate: 'agent_message_chunk', content: { type: 'text', text } } } }) + '\n';
  const bytes = Buffer.from(wire);
  const split = Math.max(1, bytes.indexOf(Buffer.from('🌍')) + 1);
  process.stdout.write(bytes.subarray(0, split));
  await new Promise(resolve => setTimeout(resolve, 10));
  process.stdout.write(bytes.subarray(split));
  complete();
}
