import test from 'node:test';
import assert from 'node:assert/strict';
import { createContext, runInContext } from 'node:vm';
import { dashboardHtml } from '../dist/dashboard-ui.js';
import { privateChatClientSource } from '../dist/dashboard-private-chats-ui.js';

function deferred() { let resolve, reject; const promise = new Promise((accept, fail) => { resolve = accept; reject = fail; }); return { promise, resolve, reject }; }
const turn = () => new Promise(resolve => setImmediate(resolve));
const first = 'task-instance-A', second = 'task-instance-B', third = 'task-instance-C';
const chat = (id = 'chat-a', roomId = 'room-a', peer = second) => ({ id, participants: [first, peer], roomId, messageCount: 0, lastSequence: 0, createdAt: '2026-11-04T12:00:00Z' });
const message = (body, sequence = 1, from = second) => ({ id: 'message-' + sequence, sequence, from, body, createdAt: '2026-11-04T12:00:00Z' });
const page = messages => ({ messages, nextCursor: null });
function harness(t, handler = () => undefined) {
  const nodes = new Map(), closes = [], calls = [];
  class Node {
    constructor(tag) { this.tag = tag; this.children = []; this.listeners = new Map(); this.textContent = ''; this.value = ''; this.hidden = false; this.open = false; this.disabled = false; this.checked = false; }
    append(...children) { this.children.push(...children); }
    replaceChildren(...children) { this.children = children; }
    setAttribute(key, value) { this[key] = value; }
    addEventListener(event, handler) { if (!this.listeners.has(event)) this.listeners.set(event, []); this.listeners.get(event).push(handler); }
    fire(event) { return Promise.all((this.listeners.get(event) ?? []).map(handler => handler({ preventDefault() {} }))); }
    showModal() { this.open = true; }
    close() { if (this.open) { this.open = false; closes.push(() => this.fire('close')); } }
  }
  const $ = id => { if (!nodes.has(id)) nodes.set(id, new Node(id)); return nodes.get(id); };
  $('privateChatActor').value = first; $('privateChatRoomOnly').checked = true;
  let key = 0;
  const context = createContext({ Promise, AbortController, encodeURIComponent, $, token: 'local-session',
    crypto: { randomUUID: () => 'private-retry-' + ++key }, agents: [{ agent: 'codex' }, { agent: 'claude' }],
    room: { id: 'room-a' }, roomLoading: false, selected: 'room-a',
    element: (tag, cls, content) => { const node = new Node(tag); node.className = cls ?? ''; if (content !== undefined) node.textContent = String(content); return node; },
    api: async (path, body, signal) => {
      calls.push({ path, body, signal }); const handled = handler(path, body, signal); if (handled !== undefined) return await handled;
      if (path === '/api/private-chats/watch') return await new Promise((_, reject) => {
        if (signal.aborted) reject(signal.reason); else signal.addEventListener('abort', () => reject(signal.reason), { once: true });
      });
      if (path.startsWith('/api/private-chats/actors/page?')) return { actors: [first, second, third], nextCursor: null };
      if (path.startsWith('/api/private-chats/page?')) return { chats: [chat()], nextCursor: null };
      if (path.includes('/messages?')) return page([]);
      if (path.endsWith('/messages') && body) return message(body.body);
      if (path === '/api/private-chats' && body) return chat();
      assert.fail('Unhandled private UI request: ' + path);
    },
  });
  runInContext(privateChatClientSource() + '\ninstallPrivateChatsUI();', context);
  t.after(() => runInContext('dismissPrivateChats()', context));
  return { $, calls, context, run: source => runInContext(source, context), open: () => runInContext('openPrivateChats()', context),
    async select(value = chat()) { context.selectedFixtureChat = value; await runInContext('privateChatSelect(selectedFixtureChat,privateChatActor)', context); },
    async edit(value) { $('privateChatBody').value = value; await $('privateChatBody').fire('input'); },
    async flushClose() { for (const close of closes.splice(0)) await close(); },
  };
}
function text(node) { return [node.textContent, ...node.children.map(text)].join('\n'); }

test('shipped private chat UI preserves full literal text without interpreting message markup', async t => {
  const literal = '<script>private literal only</script>\n' + 'Retain complete private evidence. '.repeat(2000);
  const wire = harness(t, path => path.includes('/messages?') ? page([message(literal)]) : undefined);
  await wire.open(); await wire.select();
  assert.ok(text(wire.$('privateChatMessages')).includes(literal));
  assert.equal(wire.$('privateChatMessages').children.some(node => node.tag === 'script'), false);
  const html = dashboardHtml('private-ui-test'); assert.ok(html.includes(privateChatClientSource()));
  assert.match(html, /One-to-one coordination/); assert.match(html, /not authenticated identities/); assert.equal(html.includes('.innerHTML'), false);
});

test('an obsolete thread button cannot select a previous room scope while its replacement list loads', async t => {
  const newer = deferred(); let lists = 0;
  const wire = harness(t, path => path.startsWith('/api/private-chats/page?') ? ++lists === 1 ? { chats: [chat()], nextCursor: null } : newer.promise : undefined);
  await wire.open(); const oldButton = wire.$('privateChatThreads').children[0];
  wire.$('privateChatRoomOnly').checked = false; const loading = wire.$('privateChatRoomOnly').fire('change');
  assert.equal(wire.$('privateChatThreads').children.length, 0, 'Old room-scoped choices disappear immediately.');
  await oldButton.fire('click'); await turn(); assert.equal(wire.run('privateChatSelected'), null);
  newer.resolve({ chats: [chat('workspace-chat', undefined, third)], nextCursor: null }); await loading;
  assert.equal(wire.run('privateChatSelected'), null);
});

test('late actor-list and private-message reads cannot replace a newer actor or selected pair', async t => {
  const oldList = deferred(), currentList = deferred(), oldMessages = deferred(), newMessages = deferred();
  const wire = harness(t, path => {
    if (path.startsWith('/api/private-chats/page?limit=50&actor=' + first)) return oldList.promise;
    if (path.startsWith('/api/private-chats/page?limit=50&actor=' + second)) return currentList.promise;
    if (path.includes('/chat-a/messages?')) return oldMessages.promise;
    if (path.includes('/chat-b/messages?')) return newMessages.promise;
  });
  const opening = wire.open(); wire.$('privateChatActor').value = second; await wire.$('privateChatActor').fire('input');
  const current = wire.run('privateChatLoadList()'); currentList.resolve({ chats: [chat('chat-b')], nextCursor: null }); await current;
  oldList.resolve({ chats: [chat('chat-a')], nextCursor: null }); await opening;
  assert.equal(wire.run('privateChatActor'), second); assert.equal(wire.$('privateChatThreads').children.length, 1);
  const firstRead = wire.select(chat('chat-a')), nextRead = wire.select(chat('chat-b'));
  newMessages.resolve(page([message('Current pair evidence.')])); await nextRead;
  oldMessages.resolve(page([message('Obsolete private evidence.')])); await firstRead;
  assert.equal(wire.run('privateChatSelected.id'), 'chat-b'); assert.ok(text(wire.$('privateChatMessages')).includes('Current pair evidence.'));
  assert.equal(text(wire.$('privateChatMessages')).includes('Obsolete private evidence.'), false);
});

test('closing and reopening a pair ignores earlier reads and queued native close events', async t => {
  const old = deferred(), current = deferred(); let reads = 0;
  const wire = harness(t, path => path.includes('/messages?') ? ++reads === 1 ? old.promise : current.promise : undefined);
  await wire.open(); const firstRead = wire.select(); wire.run('dismissPrivateChats()');
  await wire.open(); const nextRead = wire.select(); const epoch = wire.run('privateChatEpoch');
  await wire.flushClose(); assert.equal(wire.run('privateChatEpoch'), epoch); assert.equal(wire.run('privateChatController.signal.aborted'), false);
  current.resolve(page([message('Reopened current evidence.')])); await nextRead;
  old.reject(new Error('Obsolete private read failed.')); await firstRead;
  assert.equal(wire.$('privateChatError').hidden, true); assert.ok(text(wire.$('privateChatMessages')).includes('Reopened current evidence.'));
});

test('a successful send to an earlier pair cannot clear a newer pair draft', async t => {
  const accepted = deferred();
  const wire = harness(t, (path, body) => path.endsWith('/messages') && body ? accepted.promise : undefined);
  await wire.open(); await wire.select(chat('chat-a')); await wire.edit('Message for pair A.');
  const sending = wire.$('privateChatCompose').fire('submit');
  await wire.select(chat('chat-b', 'room-a', third)); await wire.edit('New draft for pair B.');
  accepted.resolve(message('Message for pair A.')); await sending;
  const sent = wire.calls.find(call => call.body?.body === 'Message for pair A.');
  assert.ok(sent.path.includes('/chat-a/messages')); assert.equal(sent.body.from, first);
  assert.equal(wire.$('privateChatBody').value, 'New draft for pair B.'); assert.equal(wire.run('privateChatSelected.id'), 'chat-b');
});

test('a delayed pair creation cannot steal a newer selected conversation or its draft', async t => {
  const created = deferred();
  const wire = harness(t, (path, body) => path === '/api/private-chats' && body ? created.promise : undefined);
  await wire.open(); wire.$('privateChatPeer').value = third;
  const creating = wire.$('privateChatCreate').fire('submit');
  await wire.select(chat('chat-b', 'room-a', 'task-instance-D')); await wire.edit('Keep my newer conversation selected.');
  created.resolve(chat('created-chat', 'room-a', third)); await creating;
  assert.equal(wire.run('privateChatSelected.id'), 'chat-b'); assert.equal(wire.$('privateChatBody').value, 'Keep my newer conversation selected.');
});

test('an earlier pending create cannot reenable or replace a creation in a reopened private-chat dialog', async t => {
  const old = deferred(), current = deferred(); let creates = 0;
  const wire = harness(t, (path, body) => path === '/api/private-chats' && body ? ++creates === 1 ? old.promise : current.promise : undefined);
  await wire.open(); wire.$('privateChatPeer').value = second;
  const firstCreate = wire.$('privateChatCreate').fire('submit'); wire.run('dismissPrivateChats()');
  await wire.open(); wire.$('privateChatPeer').value = third;
  const nextCreate = wire.$('privateChatCreate').fire('submit');
  await wire.$('privateChatCreate').fire('submit'); assert.equal(creates, 2, 'Repeated submits within the same session share one admission.');
  old.resolve(chat('old-created-chat')); await firstCreate;
  assert.equal(wire.$('privateChatCreateButton').disabled, true); assert.equal(wire.run('privateChatSelected'), null);
  current.resolve(chat('current-created-chat', 'room-a', third)); await nextCreate;
  assert.equal(wire.$('privateChatCreateButton').disabled, false); assert.equal(wire.run('privateChatSelected.id'), 'current-created-chat');
});

test('reopening while the new room is loading clears the prior private transcript before reporting an invalid scope', async t => {
  const wire = harness(t, path => path.includes('/messages?') ? page([message('Prior room private evidence.')]) : undefined);
  await wire.open(); await wire.select(); wire.run('dismissPrivateChats();roomLoading=true;selected="room-b";');
  await wire.open();
  assert.equal(wire.run('privateChatSelected'), null); assert.equal(wire.$('privateChatMessages').children.length, 0);
  assert.equal(wire.$('privateChatPair').textContent, 'Select a private thread'); assert.equal(wire.$('privateChatBody').disabled, true);
  assert.match(wire.$('privateChatError').textContent, /Select a loaded room/);
});

test('an invalid replacement scope invalidates an older pending list instead of publishing its old choices', async t => {
  const old = deferred(); let lists = 0;
  const wire = harness(t, path => path.startsWith('/api/private-chats/page?') ? ++lists === 1 ? { chats: [chat()], nextCursor: null } : old.promise : undefined);
  await wire.open(); const olderLoad = wire.run('privateChatLoadList()');
  wire.run('roomLoading=true;selected="room-b";'); await wire.$('privateChatRoomOnly').fire('change');
  old.resolve({ chats: [chat('obsolete-chat')], nextCursor: null }); await olderLoad;
  assert.equal(wire.$('privateChatThreads').children.length, 0); assert.equal(wire.run('privateChatSelected'), null);
  assert.match(wire.$('privateChatError').textContent, /Select a loaded room/);
});

test('a send accepted after the same pair reopens clears its unchanged visible draft and prevents duplicate resubmission', async t => {
  const accepted = deferred(); let sends = 0;
  const wire = harness(t, (path, body) => { if (path.endsWith('/messages') && body) { sends++; return accepted.promise; } });
  await wire.open(); await wire.select(); await wire.edit('Retain one successful private message.');
  const sending = wire.$('privateChatCompose').fire('submit'); wire.run('dismissPrivateChats()');
  await wire.open(); await wire.select(); assert.equal(wire.$('privateChatBody').value, 'Retain one successful private message.');
  assert.equal(wire.$('privateChatSend').disabled, true, 'The original submission is still pending.');
  accepted.resolve(message('Retain one successful private message.')); await sending;
  assert.equal(wire.$('privateChatBody').value, '', 'The completed unchanged draft is cleared in the reopened pair.');
  await wire.$('privateChatCompose').fire('submit'); assert.equal(sends, 1);
});

test('newer edits survive pending sends and failed-response retries reuse the original idempotency key', async t => {
  let sends = 0; const pending = deferred();
  const wire = harness(t, (path, body) => {
    if (path.endsWith('/messages') && body) {
      sends++; if (sends === 1) return Promise.reject(new Error('The response was lost after acceptance.'));
      return pending.promise;
    }
  });
  await wire.open(); await wire.select(); const literal = '<b>Private literal</b>\nComplete unchanged message.'; await wire.edit(literal);
  await wire.$('privateChatCompose').fire('submit'); assert.equal(wire.$('privateChatBody').value, literal);
  const retry = wire.$('privateChatCompose').fire('submit');
  const posted = wire.calls.filter(call => call.body?.body === literal);
  assert.equal(posted.length, 2); assert.equal(posted[0].body.idempotencyKey, posted[1].body.idempotencyKey);
  await wire.edit('New private draft while the retry is pending.'); pending.resolve(message(literal)); await retry;
  assert.equal(wire.$('privateChatBody').value, 'New private draft while the retry is pending.');
});

test('new-message notices keep pair history explicit and old watches cannot signal a different pair', async t => {
  const oldWatch = deferred(), currentWatch = deferred(); let watches = 0;
  const wire = harness(t, path => path === '/api/private-chats/watch' ? ++watches === 1 ? oldWatch.promise : currentWatch.promise : undefined);
  await wire.open(); await wire.select(chat('chat-a')); await wire.select(chat('chat-b', 'room-a', third));
  oldWatch.resolve({ reason: 'changed' }); await turn(); assert.equal(wire.$('privateChatNewer').hidden, true);
  currentWatch.resolve({ reason: 'changed' }); await turn(); assert.equal(wire.$('privateChatNewer').hidden, false);
  assert.match(wire.$('privateChatStatus').textContent, /New private messages/);
  assert.equal(wire.calls.filter(call => call.path.includes('/messages?')).length, 2, 'Notices do not silently download another transcript.');
});

test('successful private-history retries clear obsolete failures and changing actors stops the waiting status', async t => {
  let reads = 0;
  const wire = harness(t, path => path.includes('/messages?') ? ++reads === 1 ? Promise.reject(new Error('Temporary private history failure.')) : page([message('Retried complete private history.')]) : undefined);
  await wire.open(); await wire.select(); assert.equal(wire.$('privateChatError').hidden, false);
  await wire.$('privateChatReloadMessages').fire('click');
  assert.equal(wire.$('privateChatError').hidden, true); assert.ok(text(wire.$('privateChatMessages')).includes('Retried complete private history.'));
  wire.$('privateChatActor').value = third; await wire.$('privateChatActor').fire('input');
  assert.equal(wire.$('privateChatStatus').textContent, ''); assert.equal(wire.run('privateChatWatchController'), undefined);
});

test('private discovery loads only requested metadata pages and appending preserves the selected pair and draft', async t => {
  const wire = harness(t, path => {
    if (path.startsWith('/api/private-chats/page?')) return path.includes('&cursor=threads-1')
      ? { chats: [chat(), chat('chat-b', 'room-a', third)], nextCursor: null }
      : { chats: [chat()], nextCursor: 'threads-1' };
    if (path.startsWith('/api/private-chats/actors/page?')) return path.includes('&cursor=actors-1')
      ? { actors: [second, third], nextCursor: null } : { actors: [first, second], nextCursor: 'actors-1' };
  });
  await wire.open(); await turn();
  assert.equal(wire.calls.filter(call => call.path.startsWith('/api/private-chats/page?')).length, 1);
  assert.equal(wire.calls.filter(call => call.path.startsWith('/api/private-chats/actors/page?')).length, 1);
  assert.equal(wire.calls.some(call => call.path.includes('/messages?')), false, 'Discovery never fetches transcript bodies.');
  assert.equal(wire.$('privateChatThreadsMore').hidden, false); assert.equal(wire.$('privateChatActorsMore').hidden, false);
  const original = wire.$('privateChatThreads').children[0]; await original.fire('click'); await wire.edit('Keep this selected-pair draft.');
  await wire.$('privateChatThreadsMore').fire('click'); await wire.$('privateChatActorsMore').fire('click');
  assert.equal(wire.$('privateChatThreads').children.length, 2); assert.equal(wire.$('privateChatThreads').children[0], original);
  assert.equal(wire.$('privateChatActors').children.filter(node => node.value === second).length, 1);
  assert.equal(wire.run('privateChatSelected.id'), 'chat-a'); assert.equal(wire.$('privateChatBody').value, 'Keep this selected-pair draft.');
  assert.equal(wire.$('privateChatThreadsMore').hidden, true); assert.equal(wire.$('privateChatActorsMore').hidden, true);
  await original.fire('click'); assert.equal(wire.run('privateChatSelected.id'), 'chat-a', 'Earlier page buttons remain valid after appending.');
});

test('failed discovery pages retain loaded choices and retry the same independent cursor without automatic requests', async t => {
  let threads = 0, actors = 0;
  const wire = harness(t, path => {
    if (path.startsWith('/api/private-chats/page?')) {
      threads++; if (threads === 1) return { chats: [chat()], nextCursor: 'thread-retry' };
      return threads === 2 ? Promise.reject(new Error('Transient thread page failure.')) : { chats: [chat('chat-b')], nextCursor: null };
    }
    if (path.startsWith('/api/private-chats/actors/page?')) {
      actors++; if (actors === 1) return Promise.reject(new Error('Transient participant page failure.'));
      return { actors: [first, second], nextCursor: null };
    }
  });
  await wire.open(); await turn(); assert.equal(threads, 1); assert.equal(actors, 1);
  assert.equal(wire.$('privateChatActorsMore').textContent, 'Retry participant page');
  await wire.$('privateChatThreadsMore').fire('click'); await turn();
  assert.equal(threads, 2); assert.equal(wire.$('privateChatThreads').children.length, 1);
  assert.equal(wire.$('privateChatThreadsMore').disabled, false);
  await wire.$('privateChatActorsMore').fire('click'); await wire.$('privateChatThreadsMore').fire('click');
  assert.equal(wire.$('privateChatThreads').children.length, 2);
  const attempted = wire.calls.filter(call => call.path.includes('cursor=thread-retry')); assert.equal(attempted.length, 2);
  assert.equal(attempted[0].path, attempted[1].path); assert.equal(actors, 2);
  assert.equal(wire.$('privateChatError').hidden, true); assert.equal(wire.$('privateChatListStatus').textContent.includes('failed'), false);
  assert.equal(wire.$('privateChatActorsStatus').textContent.includes('failed'), false);
});

test('late discovery pages cannot restore metadata from a replaced actor or a reopened dialog', async t => {
  const oldThreads = deferred(), oldActors = deferred(); let threads = 0, actors = 0;
  const wire = harness(t, path => {
    if (path.startsWith('/api/private-chats/page?')) {
      threads++; if (threads === 1) return { chats: [chat()], nextCursor: 'old-threads' };
      if (threads === 2) return oldThreads.promise;
      return { chats: [chat('new-chat')], nextCursor: null };
    }
    if (path.startsWith('/api/private-chats/actors/page?')) {
      actors++; if (actors === 1) return { actors: [first], nextCursor: 'old-actors' };
      if (actors === 2) return oldActors.promise;
      return { actors: [third], nextCursor: null };
    }
  });
  await wire.open(); const pendingThreads = wire.$('privateChatThreadsMore').fire('click'), pendingActors = wire.$('privateChatActorsMore').fire('click');
  wire.run('dismissPrivateChats()'); wire.$('privateChatActor').value = third; await wire.open();
  oldThreads.resolve({ chats: [chat('obsolete-chat')], nextCursor: 'obsolete-next' });
  oldActors.resolve({ actors: ['obsolete actor'], nextCursor: 'obsolete-next' }); await Promise.all([pendingThreads, pendingActors]);
  assert.equal(wire.$('privateChatThreads').children.length, 1);
  assert.equal(wire.$('privateChatActors').children.some(node => node.value === 'obsolete actor'), false);
  assert.equal(wire.$('privateChatThreadsMore').hidden, true); assert.equal(wire.$('privateChatActorsMore').hidden, true);
});

test('private discovery and message submission preserve exact participant labels including surrounding spaces', async t => {
  const actor = '  task instance A  ', peer = ' task instance B ';
  const exact = { ...chat(), participants: [actor, peer] };
  const wire = harness(t, (path, body) => {
    if (path.startsWith('/api/private-chats/page?')) return { chats: [exact], nextCursor: null };
    if (path === '/api/private-chats' && body) return exact;
  });
  wire.$('privateChatActor').value = actor; await wire.open();
  assert.ok(wire.calls.some(call => call.path.includes('&actor=' + encodeURIComponent(actor))));
  await wire.$('privateChatThreads').children[0].fire('click'); assert.equal(wire.$('privateChatBody').disabled, false);
  await wire.edit('Literal actor labels are preserved.'); await wire.$('privateChatCompose').fire('submit');
  assert.equal(wire.calls.find(call => call.body?.body === 'Literal actor labels are preserved.').body.from, actor);
  wire.$('privateChatPeer').value = peer; await wire.$('privateChatCreate').fire('submit');
  const created = wire.calls.find(call => call.path === '/api/private-chats' && call.body);
  assert.deepEqual(Array.from(created.body.participants), [actor, peer]);
});
