import { spawn } from 'node:child_process';
import { writeFileSync } from 'node:fs';
let prompt = '';
for await (const part of process.stdin) prompt += part.toString();
const claude = process.argv.includes('--output-format');
const send = event => process.stdout.write(JSON.stringify(event) + '\n');
const text = prompt === 'args' ? JSON.stringify(process.argv.slice(2)) : prompt === 'depth' ? process.env.COUNCIL_DEPTH : `Native 🌍 ${prompt}`;
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
    if (prompt !== 'partial') send({ type: 'result', subtype: 'success', is_error: false, result: text, ...(prompt === 'denied' ? { permission_denials: [{ tool_name: 'Edit' }] } : {}) });
    if (prompt === 'nonzero') process.exitCode = 2;
  }
} else {
  send({ type: 'thread.started', thread_id: 'fixture-thread' }); send({ type: 'turn.started' });
  if (prompt === 'auth-error') send({ type: 'turn.failed', error: { message: 'Authentication required SECRET_ACCESS_TOKEN=do-not-print' } });
  else {
    send({ type: 'item.completed', item: { type: 'reasoning', text: 'hidden' } });
    send({ type: 'item.completed', item: { type: 'agent_message', text } });
    if (prompt !== 'partial') send({ type: 'turn.completed', usage: { input_tokens: 1, output_tokens: 1 } });
    if (prompt === 'nonzero') process.exitCode = 2;
  }
}
