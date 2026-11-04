let prompt = '';
for await (const chunk of process.stdin) prompt += chunk.toString();
const send = event => process.stdout.write(JSON.stringify(event) + '\n');
const message = text => send({ type: 'item.completed', item: { type: 'agent_message', text } });
send({ type: 'turn.started' });
message('I am inspecting the offline evidence before returning the final answer.\n');
const ballot = /ROOM_BALLOT_DATA_BEGIN\n([^\n]+)\nROOM_BALLOT_DATA_END/.exec(prompt);
if (prompt === 'empty-final') message('');
else if (ballot) {
  const data = JSON.parse(ballot[1]);
  message(JSON.stringify({ vote: 'approve', rationale: 'Complete offline evidence supports the assigned task.',
    assignments: Object.fromEntries(data.tasks.map(task => [task.id, data.members[0].agent])) }));
} else if (process.argv[2] === 'advisor') message('The independent offline evidence supports completing the task.');
else if (prompt.includes('CODEX_ADVISOR_FINAL_FIXTURE') && !prompt.includes('Resume your task after receiving the advisor answer')) {
  message(JSON.stringify({ council_advisor_request: { question: 'Check the offline acceptance evidence.', context: 'Complete fixture context.' } }));
} else if (prompt.includes('CODEX_ADVISOR_FINAL_FIXTURE')) message('Worker verified the task after assessing the advisor evidence.');
else message(JSON.stringify({ result: 'Complete offline final answer.' }));
send({ type: 'turn.completed' });
