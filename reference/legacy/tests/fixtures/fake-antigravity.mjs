let input = '';
for await (const part of process.stdin) input += part.toString();
const message = JSON.parse(input);
if (message.event !== 'user' || typeof message.message?.content !== 'string') throw new Error('Expected a stdin user message.');
const prompt = message.message.content;
const send = event => process.stdout.write(JSON.stringify(event) + '\n');
const conversation_id = 'offline-antigravity-conversation';
const output = prompt === 'args' ? JSON.stringify(process.argv.slice(2)) : `Native Antigravity 🌍 ${prompt}`;
send({ event: 'init', conversation_id, init: { cwd: process.cwd(), tools: [], permission_mode: 'request-review' } });
send({ event: 'step_update', step_update: { conversation_id, step_index: 0, state: 'DONE', step_type: 'user_input' } });
if (prompt === 'native-permission-diagnostic') process.stderr.write('SECRET_ACCESS_TOKEN required a permission that headless mode cannot prompt for, so it was auto-denied. SECRET_ACCESS_TOKEN\n');
if (prompt === 'denied-error-step' || prompt === 'denied-error-step-wait') {
  send({ event: 'step_update', step_update: { conversation_id, step_index: 6, state: 'ACTIVE', step_type: 'tool',
    tool_name: 'read_file', tool_info: { name: 'read_file', parameters: { Path: 'SECRET_ACCESS_TOKEN' } } } });
  send({ event: 'step_update', step_update: { conversation_id, step_index: 6, state: 'ERROR', step_type: 'tool',
    tool_name: 'read_file', tool_info: { name: 'read_file', parameters: { Path: 'SECRET_ACCESS_TOKEN' },
      error: { type: 'permission_denied', message: 'SECRET_ACCESS_TOKEN' } } } });
  send({ event: 'result', result: { conversation_id, status: 'SUCCESS', response: '' } });
  if (prompt === 'denied-error-step-wait') {
    // Keep the causal-stop fixture alive until Council stops its owned tree.
    // Spontaneous-exit coverage continues to use denied-error-step.
    setInterval(() => {}, 1000);
    await new Promise(() => {});
  }
  process.exit(0);
}
if (prompt === 'invalid-step') send({ event: 'step_update', step_update: { conversation_id, step_index: 1, state: 'SECRET_ACCESS_TOKEN', step_type: 'agent_response',
  text_delta: 'SECRET_ACCESS_TOKEN', tool_info: { parameters: { command: 'SECRET_ACCESS_TOKEN', path: 'SECRET_ACCESS_TOKEN' } } } });
send({ event: 'step_update', step_update: { conversation_id, step_index: 1, state: 'ACTIVE', step_type: 'agent_response', text_delta: output } });
send({ event: 'step_update', step_update: { conversation_id, step_index: 1, state: 'DONE', step_type: 'agent_response', text_delta: '' } });
if (prompt === 'diagnostics') process.stderr.write('Tool permission was soft-denied: SECRET_ACCESS_TOKEN=do-not-print\n');
if (prompt !== 'partial') send({ event: 'result', result: { conversation_id, status: prompt === 'cancelled' ? 'CANCELED' : 'SUCCESS',
  response: prompt === 'mismatch' ? 'Contradictory final answer' : output,
  ...(prompt === 'cutoff' ? { finished_reason: 'print_timeout' } : {}) } });
if (prompt === 'late') send({ event: 'step_update', step_update: { conversation_id, step_index: 2, state: 'DONE', step_type: 'agent_response', text_delta: 'Late text' } });
if (prompt === 'nonzero') process.exitCode = 2;
