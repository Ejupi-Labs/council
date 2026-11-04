import { spawn } from 'node:child_process';
import { writeFileSync } from 'node:fs';
let prompt = '';
for await (const part of process.stdin) prompt += part.toString();
const claude = process.argv.includes('--output-format');
const send = event => process.stdout.write(JSON.stringify(event) + '\n');
const text = prompt === 'args' ? JSON.stringify(process.argv.slice(2)) : prompt === 'depth' ? process.env.COUNCIL_DEPTH
  : prompt === 'git-env' ? JSON.stringify(Object.fromEntries(['GIT_DIR', 'GIT_WORK_TREE', 'GIT_INDEX_FILE', 'GIT_CONFIG_COUNT', 'GIT_CONFIG_KEY_0', 'GIT_CONFIG_VALUE_0', 'COUNCIL_TEST_KEEP'].filter(key => process.env[key] !== undefined).map(key => [key, process.env[key]])))
  : `Native 🌍 ${prompt}`;
const structure = () => {
  if (prompt === 'structure-deep') process.stdout.write('{"type":"progress","payload":' + '['.repeat(300_000) + '0' + ']'.repeat(300_000) + '}\n');
  else if (prompt === 'structure-wide') send({ type: 'progress', payload: Array(70_000).fill(0) });
  else if (prompt === 'structure-quoted') send({ type: 'progress', payload: '[{"nested":"\\\"🌍"}]'.repeat(10_000) });
};
if (prompt === 'hang' || prompt === 'tree-hang') {
  if (prompt === 'tree-hang') {
    const child = spawn(process.execPath, ['-e', 'setInterval(() => {}, 1000)'], { stdio: 'ignore', windowsHide: true });
    if (process.env.COUNCIL_TEST_PID_FILE) writeFileSync(process.env.COUNCIL_TEST_PID_FILE, JSON.stringify({ vendor: process.pid, grandchild: child.pid }));
  }
  setInterval(() => { if (process.env.COUNCIL_TEST_CHATTER === '1') send({ type: 'progress', text: 'Still running.' }); }, 20);
} else if (prompt === 'malformed') process.stdout.write('{broken');
else if (prompt === 'invalid-utf8') {
  send({ type: 'turn.started' });
  process.stdout.write(Buffer.concat([Buffer.from('{"type":"item.completed","item":{"type":"agent_message","text":"'), Buffer.from([0xc3, 0x28]), Buffer.from('"}}\n')]));
  send({ type: 'turn.completed' });
} else if (prompt === 'frame-overflow') {
  send({ type: 'turn.started' });
  send({ type: 'item.started', item: { type: 'tool_call', output: 'x'.repeat(4 * 1024 * 1024 + 1) } });
  send({ type: 'turn.completed' });
}
else if (claude) {
  if (prompt === 'auth-error') send({ type: 'result', subtype: 'error_during_execution', is_error: true, result: 'Not logged in SECRET_ACCESS_TOKEN=do-not-print' });
  else if (prompt === 'forbidden-tool') send({ type: 'assistant', message: { content: [{ type: 'tool_use', name: 'Bash', input: {} }] } });
  else {
    send({ type: 'assistant', message: { content: [{ type: 'thinking', thinking: 'hidden' }, { type: 'text', text }] } });
    structure();
    if (prompt === 'success-without-result') send({ type: 'result', subtype: 'success', is_error: false });
    else if (prompt === 'success-with-invalid-result') send({ type: 'result', subtype: 'success', is_error: false, result: { text } });
    else if (prompt === 'success-with-invalid-error-flag') send({ type: 'result', subtype: 'success', is_error: 'true', result: text });
    else if (prompt === 'success-with-invalid-denials') send({ type: 'result', subtype: 'success', is_error: false, result: text, permission_denials: { tool_name: 'Edit' } });
    else if (prompt !== 'partial') send({ type: 'result', subtype: 'success', is_error: false, result: text, ...(prompt === 'denied' ? { permission_denials: [{ tool_name: 'Edit' }] } : {}) });
    if (prompt === 'nonzero') process.exitCode = 2;
  }
} else {
  send({ type: 'thread.started', thread_id: 'fixture-thread' }); send({ type: 'turn.started' });
  if (prompt === 'auth-error') send({ type: 'turn.failed', error: { message: 'Authentication required SECRET_ACCESS_TOKEN=do-not-print' } });
  else {
    send({ type: 'item.completed', item: { type: 'reasoning', text: 'hidden' } });
    send({ type: 'item.completed', item: { type: 'agent_message', text } });
    structure();
    if (prompt !== 'partial') send({ type: 'turn.completed', usage: { input_tokens: 1, output_tokens: 1 } });
    if (prompt === 'nonzero') process.exitCode = 2;
  }
}
