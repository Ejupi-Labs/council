import { writeFileSync } from 'node:fs';
import { setTimeout as delay } from 'node:timers/promises';

let prompt = '';
for await (const chunk of process.stdin) prompt += chunk.toString();
const send = event => process.stdout.write(JSON.stringify(event) + '\n');
send({ type: 'thread.started', thread_id: 'offline-room-fixture' });
send({ type: 'turn.started' });
const ballot = /ROOM_BALLOT_DATA_BEGIN\n([^\n]+)\nROOM_BALLOT_DATA_END/.exec(prompt);
if (ballot) {
  const data = JSON.parse(ballot[1]);
  send({ type: 'item.completed', item: { type: 'agent_message', text: JSON.stringify({ vote: 'approve', rationale: 'Offline fixture approval.', assignments: Object.fromEntries(data.tasks.map(task => [task.id, data.members[0].agent])) }) } });
  send({ type: 'turn.completed' });
} else {
  const marker = process.argv.indexOf('--started-file');
  if (marker >= 0) writeFileSync(process.argv[marker + 1], JSON.stringify({ pid: process.pid }));
  const delayOption = process.argv.indexOf('--task-delay-ms');
  if (process.argv.includes('--delay-task')) await delay(delayOption >= 0 ? Number(process.argv[delayOption + 1]) : 1500);
  if (process.argv.includes('--fail-task')) send({ type: 'turn.failed', error: { message: 'Intentional offline room task failure.' } });
  else {
    send({ type: 'item.completed', item: { type: 'agent_message', text: 'Offline task completed without modifying project files.' } });
    send({ type: 'turn.completed' });
  }
}
