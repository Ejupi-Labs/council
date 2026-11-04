import test from 'node:test';
import assert from 'node:assert/strict';
import { createContext, runInContext } from 'node:vm';
import { serveDashboard } from '../dist/dashboard.js';
import { dashboardHtml } from '../dist/dashboard-ui.js';
import { runClientSource } from '../dist/dashboard-runs-ui.js';

function record(id = 'run-a', status = 'completed') {
  return { id, status, createdAt: '2026-11-04T12:00:00Z', updatedAt: '2026-11-04T12:01:00Z',
    input: { mode: 'delegate', task: 'Keep the complete task.', context: 'Complete context.', agents: ['worker'], permission: 'write',
      agentSettings: { worker: { model: 'pinned-model', effort: 'low', skills: ['council-advisor'] } } },
    output: '<script>literal output</script>\n' + 'Complete evidence. '.repeat(2000),
    steps: Array.from({ length: 70 }, (_, index) => ({ id: 'step-' + index, agent: 'worker', role: 'worker',
      result: { agent: 'worker', status: 'completed', output: 'Full step ' + index, durationMs: index + 1 } })),
    workspaces: [{ agent: 'worker', path: 'C:\\project\\.council\\worktrees\\run-a', patchPath: 'C:\\project\\.council\\patches\\run-a.patch',
      changed: true, baseCommit: 'abc123', exportStatus: 'failed', exportError: 'Patch export needs repair.' }] };
}

async function fixture(t) {
  const stored = record();
  let reads = 0;
  const service = { config: { backend: 'mock' }, getRun: id => { reads++; return id === stored.id ? structuredClone(stored) : undefined; },
    start() { assert.fail('Inspection must not dispatch agents.'); } };
  const dashboard = await serveDashboard(service, { list: () => [] });
  t.after(() => dashboard.close());
  const url = new URL(dashboard.url), token = new URLSearchParams(url.hash.slice(1)).get('token');
  return { stored, token, reads: () => reads, get: (path, options = {}) => fetch(url.origin + path, {
    ...options, headers: { Authorization: 'Bearer ' + token, ...options.headers },
  }) };
}

test('workflow inspection authenticates before reading complete stored evidence', async t => {
  const wire = await fixture(t);
  assert.equal((await wire.get('/api/runs/run-a', { headers: { Authorization: '' } })).status, 401);
  assert.equal((await wire.get('/api/runs/run-a', { headers: { Origin: 'https://example.com' } })).status, 403);
  assert.equal(wire.reads(), 0);
  const response = await wire.get('/api/runs/run-a');
  assert.equal(response.status, 200); assert.equal(response.headers.get('cache-control'), 'no-store');
  const full = await response.json();
  assert.deepEqual(full, wire.stored); assert.equal(full.steps.length, 70);
  assert.equal(full.output.length > 16000, true); assert.equal(JSON.stringify(full).includes(wire.token), false);
});

test('workflow inspection reports unavailable evidence and refuses mutation', async t => {
  const wire = await fixture(t);
  assert.equal((await wire.get('/api/runs/run-a', { method: 'POST' })).status, 405);
  assert.equal(wire.reads(), 0);
  const missing = await wire.get('/api/runs/missing'); assert.equal(missing.status, 404);
  assert.match((await missing.json()).error, /pruned/);
});

class Node {
  constructor(tag = '') { this.tag = tag; this.children = []; this.listeners = new Map(); this.textContent = ''; this.open = false; this.hidden = false; }
  append(...children) { this.children.push(...children); }
  replaceChildren(...children) { this.children = children; }
  setAttribute(key, value) { this[key] = value; }
  addEventListener(key, listener) { this.listeners.set(key, listener); }
  showModal() { this.open = true; }
  close() { this.open = false; }
  click() { this.listeners.get('click')?.(); }
  remove() { this.removed = true; }
}

function harness(handler) {
  const elements = new Map(), calls = [], downloads = [], revoked = [], timers = [];
  const element = (tag, cls, text) => { const node = new Node(tag); node.className = cls ?? ''; if (text !== undefined) node.textContent = String(text); return node; };
  const context = createContext({ Promise, AbortController, encodeURIComponent, Blob,
    $: id => { if (!elements.has(id)) elements.set(id, new Node(id)); return elements.get(id); }, element,
    labels: { completed: 'Completed', failed: 'Failed', cancelled: 'Cancelled' },
    api: (path, _body, signal) => { calls.push({ path, signal }); return handler(path, signal); },
    URL: { createObjectURL: blob => { downloads.push(blob); return 'blob:fixture'; }, revokeObjectURL: url => revoked.push(url) },
    document: { body: new Node('body') }, window: new Node('window'), setTimeout: callback => timers.push(callback),
  });
  runInContext(runClientSource(), context);
  return { context, elements, calls, downloads, revoked, timers, run: source => runInContext(source, context) };
}

function allText(node) { return [node.textContent, ...node.children.map(allText)].join('\n'); }
function deferred() { let resolve, reject; const promise = new Promise((a, b) => { resolve = a; reject = b; }); return { promise, resolve, reject }; }

test('workflow view keeps every step and literal output and shows failed artifact exports', async () => {
  const full = record('run-a', 'failed'); full.error = 'Verification failed.';
  const wire = harness(async () => full); await wire.run("openWorkflow('run-a')");
  const body = wire.elements.get('workflowBody'), text = allText(body);
  assert.ok(text.includes(full.output)); assert.ok(text.includes('Full step 69'));
  assert.ok(text.includes(full.workspaces[0].path)); assert.ok(text.includes(full.workspaces[0].patchPath));
  assert.ok(text.includes('Patch export needs repair.')); assert.ok(text.includes('not a successful result'));
  assert.ok(text.includes('pinned-model')); assert.ok(text.includes('Complete context.'));
  assert.equal(wire.elements.get('downloadWorkflow').disabled, false);
  assert.equal(body.children.some(node => node.tag === 'script'), false);
  const html = dashboardHtml('test-nonce'); assert.equal(html.includes('.innerHTML'), false);
  assert.ok(html.includes('View workflow')); assert.ok(html.includes('Workflow evidence'));
});

test('workflow selection aborts obsolete reads and ignores late results', async () => {
  const a = deferred(), b = deferred(), wire = harness(path => path.endsWith('run-a') ? a.promise : b.promise);
  const first = wire.run("openWorkflow('run-a')"), second = wire.run("openWorkflow('run-b')");
  assert.equal(wire.calls[0].signal.aborted, true);
  b.resolve(record('run-b')); await second;
  a.resolve(record('run-a')); await first;
  assert.ok(wire.elements.get('workflowMeta').textContent.startsWith('run-b'));
  assert.equal(wire.run('workflowRecord.id'), 'run-b');
});

test('closing and reopening the same workflow ignores an earlier failed response', async () => {
  const old = deferred(), next = deferred(); let calls = 0;
  const wire = harness(() => ++calls === 1 ? old.promise : next.promise);
  const first = wire.run("openWorkflow('run-a')"); wire.run('dismissWorkflow()');
  assert.equal(wire.calls[0].signal.aborted, true);
  const second = wire.run("openWorkflow('run-a')"); next.resolve(record()); await second;
  old.reject(new Error('Obsolete failure.')); await first;
  assert.equal(wire.elements.get('workflowError').hidden, true);
  assert.equal(wire.run('workflowRecord.id'), 'run-a');
});

test('unavailable reload clears obsolete evidence and prevents stale downloads', async () => {
  let calls = 0;
  const wire = harness(async () => { if (++calls === 1) return record(); throw Object.assign(new Error('Missing'), { status: 404 }); });
  await wire.run("openWorkflow('run-a')"); await wire.run("openWorkflow('run-a')");
  assert.equal(wire.elements.get('workflowBody').children.length, 0);
  assert.match(wire.elements.get('workflowError').textContent, /pruned/);
  assert.equal(wire.elements.get('downloadWorkflow').disabled, true);
  wire.run('downloadWorkflow()'); assert.equal(wire.downloads.length, 0);
});

test('workflow JSON export contains the exact snapshot and releases its download URL', async () => {
  const full = record(), wire = harness(async () => full);
  await wire.run("openWorkflow('run-a')"); wire.run('downloadWorkflow()');
  assert.deepEqual(JSON.parse(await wire.downloads[0].text()), full);
  assert.equal(wire.revoked.length, 0); wire.timers[0](); assert.deepEqual(wire.revoked, ['blob:fixture']);
});
