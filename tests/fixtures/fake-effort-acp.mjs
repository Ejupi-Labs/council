import { createInterface } from 'node:readline';
const send = event => process.stdout.write(JSON.stringify({ jsonrpc: '2.0', ...event }) + '\n');
for await (const line of createInterface({ input: process.stdin, crlfDelay: Infinity })) {
  const event = JSON.parse(line);
  if (event.method === 'initialize') send({ id: event.id, result: { protocolVersion: 1 } });
  else if (event.method === 'session/new') send({ id: event.id, result: { sessionId: 'effort-fixture' } });
  else if (event.method === 'session/prompt') {
    send({ method: 'session/update', params: { sessionId: 'effort-fixture', update: { sessionUpdate: 'agent_message_chunk', content: { type: 'text', text: JSON.stringify(process.argv.slice(2)) } } } });
    send({ id: event.id, result: { stopReason: 'end_turn' } });
  }
}
