import { CouncilService } from '../../dist/service.js';
import { loadConfig } from '../../dist/config.js';
import { PROCESS_CLEANUP_ERROR } from '../../dist/process.js';

export const PRIVATE_SENTINEL = 'PRIVATE_QUARANTINE_TEST_PAIR_BODY_91d890';
const root = process.argv[2], concurrency = Number(process.argv[3]);
const calls = new Map();
let sequence = 0, behavior = 'success', privateTurn = 0, service, closing = false;
const send = value => new Promise((resolve, reject) => {
  if (!process.connected) { resolve(); return; }
  process.send(value, error => error ? reject(error) : resolve());
});
const emit = value => { void send(value).catch(() => {}); };
const failure = error => ({ message: error instanceof Error ? error.message : 'Child command failed.',
  ...(typeof error?.code === 'string' ? { code: error.code } : {}),
  ...(error?.receipt ? { receipt: error.receipt } : {}),
});

try {
  service = new CouncilService(loadConfig(root, { backend: 'mock', maxConcurrency: concurrency, maxCalls: 20 }), {
    doctor: async () => [],
    run: async request => {
      const call = ++sequence, mode = behavior, turn = mode === 'private-cleanup' ? ++privateTurn : 0;
      let release;
      const released = new Promise(resolve => { release = resolve; });
      const abort = () => emit({ event: 'runner-aborted', call });
      calls.set(call, release);
      request.signal?.addEventListener('abort', abort, { once: true });
      emit({ event: 'runner-entered', call, agent: request.agent });
      try {
        await released;
        if (mode === 'private-cleanup' && turn === 1) return { agent: request.agent, status: 'completed', durationMs: 1,
          output: JSON.stringify({ council_private_chat_request: { peer: 'claude', message: PRIVATE_SENTINEL, context: 'Private pair evidence ' + PRIVATE_SENTINEL } }) };
        if (mode === 'cleanup' || mode === 'private-cleanup') return { agent: request.agent, status: 'failed', durationMs: 1,
          output: mode === 'private-cleanup' ? PRIVATE_SENTINEL : 'Retained partial offline evidence.',
          error: PROCESS_CLEANUP_ERROR + (mode === 'private-cleanup' ? ' ' + PRIVATE_SENTINEL : '') };
        if (mode === 'ordinary-failure' || mode === 'untrusted-failure') return { agent: request.agent, status: 'failed', output: '', durationMs: 1,
          error: mode === 'ordinary-failure' ? 'Ordinary offline provider failure.' : 'Untrusted provider text quoting: ' + PROCESS_CLEANUP_ERROR };
        return { agent: request.agent, status: 'completed', output: 'Explicitly released offline result.', durationMs: 1 };
      } finally { calls.delete(call); request.signal?.removeEventListener('abort', abort); }
    },
  });
  process.on('message', message => {
    if (!message || typeof message !== 'object' || typeof message.id !== 'number') return;
    void (async () => {
      let result;
      switch (message.command) {
        case 'start':
          behavior = message.behavior ?? 'success'; privateTurn = 0;
          result = service.start({ mode: 'delegate', agents: ['codex'], permission: 'read', task: message.task ?? 'Owned offline quarantine qualification.',
            ...(behavior === 'private-cleanup' ? { privateChats: { codex: { peers: ['claude'], maxRequests: 1 } } } : {}) });
          break;
        case 'completion': result = await service.completion(message.runId); break;
        case 'cancel': result = service.requestCancellation(message.runId); break;
        case 'release': {
          const release = calls.get(message.call); if (!release) throw new Error('Fake call is not waiting.');
          release(); result = { released: true }; break;
        }
        case 'health': result = { ...service.health(), fixtureCalls: sequence, fixtureWaiting: calls.size }; break;
        case 'quarantine': result = service.store.dispatchQuarantinePage(message.input ?? {}); break;
        case 'run': result = service.getRun(message.runId); break;
        case 'events': result = service.store.events(message.runId); break;
        case 'private-history': {
          const actor = 'run:' + message.runId + ':codex', chats = service.privateChats.list(actor);
          result = chats.flatMap(chat => service.privateChats.page({ chatId: chat.id, actor }).messages); break;
        }
        case 'close':
          closing = true; for (const release of calls.values()) release();
          await service.close(); await send({ id: message.id, result: { closed: true } });
          if (process.connected) process.disconnect(); return;
        default: throw new Error('Unknown fake-host command.');
      }
      await send({ id: message.id, result });
    })().catch(error => { emit({ id: message.id, error: failure(error) }); });
  });
  process.on('disconnect', () => {
    if (closing) return;
    closing = true; for (const release of calls.values()) release();
    void service.close().catch(() => { process.exitCode = 1; });
  });
  await send({ event: 'ready', pid: process.pid });
} catch (error) { await send({ event: 'fatal', error: failure(error) }).catch(() => {}); process.exitCode = 1; if (process.connected) process.disconnect(); }
