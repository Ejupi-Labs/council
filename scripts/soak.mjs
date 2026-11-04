import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { appendFileSync, cpSync, mkdirSync, mkdtempSync, readFileSync, readdirSync, realpathSync, rmSync, writeFileSync } from 'node:fs';
import { basename, dirname, join, resolve } from 'node:path';
import { tmpdir } from 'node:os';
import { monitorEventLoopDelay, performance } from 'node:perf_hooks';
import { DatabaseSync } from 'node:sqlite';
import { parseArgs } from 'node:util';
import { setTimeout as delay } from 'node:timers/promises';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { Client } from '@modelcontextprotocol/sdk/client/index.js';
import { StdioClientTransport } from '@modelcontextprotocol/sdk/client/stdio.js';

// Development-only, bounded local workloads. No vendor runner or credentials are used.
const { values } = parseArgs({ options: {
  'duration-ms': { type: 'string', default: '60000' },
  'interval-ms': { type: 'string', default: '2000' },
  'rotation-ms': { type: 'string', default: '900000' },
  label: { type: 'string', default: 'local' },
} });
const durationMs = Number(values['duration-ms']), intervalMs = Number(values['interval-ms']), rotationMs = Number(values['rotation-ms']);
for (const [name, value, minimum] of [['duration', durationMs, 1000], ['interval', intervalMs, 10], ['rotation', rotationMs, 1000]]) {
  if (!Number.isSafeInteger(value) || value < minimum || value > 86_400_000) throw new Error(`Invalid ${name} milliseconds.`);
}
if (!/^[a-z0-9-]{1,40}$/.test(values.label)) throw new Error('Use a short lowercase label.');
const repository = fileURLToPath(new URL('../', import.meta.url));
const evidence = join(repository, '.council', 'refinement');
mkdirSync(evidence, { recursive: true });
const session = `${new Date().toISOString().replaceAll(/[:.]/g, '-')}-${values.label}`;
const log = join(evidence, `soak-${session}.jsonl`);
const temporaryBase = realpathSync.native(tmpdir());
const temporary = mkdtempSync(join(temporaryBase, 'council-soak-'));
const runtime = join(temporary, 'runtime');
mkdirSync(runtime);
cpSync(join(repository, 'dist'), join(runtime, 'dist'), { recursive: true });
cpSync(join(repository, 'package.json'), join(runtime, 'package.json'));
const digest = createHash('sha256');
for (const name of readdirSync(join(runtime, 'dist')).sort()) { digest.update(name); digest.update(readFileSync(join(runtime, 'dist', name))); }
const runtimeDigest = digest.digest('hex');
const { CouncilService, CouncilStore, MockRunner, loadConfig, VERSION } = await import(pathToFileURL(join(runtime, 'dist', 'index.js')).href);
const cli = join(runtime, 'dist', 'cli.js');
process.env.COUNCIL_DEPTH = '0';
const start = performance.now(), loop = monitorEventLoopDelay({ resolution: 20 });
loop.enable();
let stopped = false, ticks = 0, workflows = 0, cancelled = 0, calls = 0, backups = 0, rotations = 0, peakCalls = 0;
let services = [], clients = [], workspace, rotatedAt = 0, failure;
const stop = () => { stopped = true; };
process.once('SIGINT', stop); process.once('SIGTERM', stop);
function emit(event, data = {}) {
  const row = { event, at: new Date().toISOString(), elapsedMs: Math.round(performance.now() - start), ...data };
  const line = JSON.stringify(row) + '\n'; appendFileSync(log, line); process.stdout.write(line);
}
async function connect() {
  const transport = new StdioClientTransport({ command: process.execPath,
    args: [cli, 'mcp', '--workspace', workspace, '--backend', 'mock'], cwd: workspace,
    env: { ...process.env, COUNCIL_DEPTH: '0', COUNCIL_BACKEND: 'mock' }, stderr: 'pipe' });
  const client = new Client({ name: 'council-offline-soak', version: VERSION });
  let stderrBytes = 0;
  transport.stderr?.on('data', chunk => { stderrBytes += chunk.length; });
  const errors = [];
  client.onerror = error => { errors.push(error.message); };
  try { await client.connect(transport); }
  catch (error) { await client.close(); throw error; }
  return { client, errors, stderrBytes: () => stderrBytes,
    call: async (name, input = {}) => {
      const result = await client.callTool({ name, arguments: input }, undefined, { timeout: 10_000 });
      assert.notEqual(result.isError, true, `${name}: ${result.content?.find(item => item.type === 'text')?.text}`);
      return JSON.parse(result.content.find(item => item.type === 'text').text);
    } };
}
function removeManaged(path) {
  const target = resolve(path);
  assert.equal(dirname(target), resolve(temporary));
  assert.ok(basename(target).startsWith('workspace-'));
  rmSync(target, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 });
}
async function closeRotation() {
  for (const connection of clients.splice(0)) { assert.deepEqual(connection.errors, []); await connection.client.close(); }
  await Promise.all(services.splice(0).map(service => service.close()));
  if (workspace) {
    const store = new CouncilStore(join(workspace, '.council'));
    try {
      const health = store.health();
      assert.equal(health.counts.instances, 0); assert.equal(health.counts.slots, 0); assert.equal(health.counts.slotWaiters ?? 0, 0); assert.equal(health.counts.activeRuns, 0); assert.equal(health.counts.leases, 0);
      assert.deepEqual(store.checkIntegrity(), ['ok']);
      emit('rotation.closed', { rotation: rotations, state: health });
    } finally { store.close(); }
    if (!failure) removeManaged(workspace);
    else emit('fixture.preserved', { workspace });
    workspace = undefined;
  }
}
async function openRotation() {
  workspace = join(temporary, `workspace-${++rotations}`); mkdirSync(workspace);
  let active = 0;
  for (let index = 0; index < 2; index++) {
    const config = loadConfig(workspace, { backend: 'mock' });
    const mock = new MockRunner(config);
    const runner = { doctor: () => mock.doctor(), run: async request => {
      active++; peakCalls = Math.max(peakCalls, active); calls++;
      assert.ok(active <= config.maxConcurrency, 'Separate services must share the call limit.');
      try {
        await delay(12, undefined, { signal: request.signal });
        return await mock.run(request);
      } catch (error) {
        if (!request.signal?.aborted) throw error;
        return { agent: request.agent, status: 'cancelled', output: '', durationMs: 12 };
      } finally { active--; }
    } };
    services.push(new CouncilService(config, runner));
  }
  for (let index = 0; index < 2; index++) clients.push(await connect());
  rotatedAt = performance.now(); emit('rotation.started', { rotation: rotations });
}
const modes = ['delegate', 'review', 'debate', 'parallel', 'pipeline', 'council'];
async function tick() {
  const pending = [];
  for (let index = 0; index < 4; index++) {
    const mode = modes[(ticks * 4 + index) % modes.length];
    const service = services[index % services.length];
    const input = { mode, agents: mode === 'delegate' ? ['codex'] : ['codex', 'claude'], permission: 'read', rounds: 1,
      task: `Offline sustained test ${ticks}:${index}: preserve evidence, Unicode café 日本語, and bounded decisions.` };
    const plan = service.engine.plan(input);
    const queued = service.start(input); workflows++;
    const shouldCancel = index === 0 && ticks % 3 === 0;
    if (shouldCancel) assert.equal(services[1].requestCancellation(queued.id).requested, true);
    pending.push(service.completion(queued.id).then(async run => {
      if (shouldCancel) { assert.equal(run.status, 'cancelled'); cancelled++; }
      else { assert.equal(run.status, 'completed'); assert.equal(run.steps.length, plan.estimatedCalls); }
      const persisted = await clients[index % clients.length].call('council_status', { id: run.id });
      assert.equal(persisted.status, run.status);
      return run;
    }));
  }
  const results = await Promise.all(pending);
  const id = results.at(-1).id;
  if (ticks % 5 === 0) {
    const queued = await clients[0].call('council_run', { mode: 'delegate', agents: ['junie'], permission: 'read', task: 'Offline client roundtrip; no provider calls.' });
    const done = await clients[1].call('council_status', { id: queued.id, waitMs: 1000 });
    assert.equal(done.status, 'completed'); workflows++;
    const events = await clients[1].call('council_events', { id, after: 0, limit: 100 });
    assert.ok(events.some(event => event.type === 'run.completed'));
    assert.deepEqual(await clients[1].call('council_events', { id, after: events.at(-1).sequence }), []);
    const message = await clients[0].call('council_send', { from: 'codex', to: 'claude', subject: 'Offline soak', body: 'Bounded evidence.', runId: id });
    const inbox = await clients[1].call('council_inbox_page', { agent: 'claude', limit: 5 });
    assert.ok(inbox.messages.some(value => value.id === message.id));
    assert.equal((await clients[1].call('council_ack', { id: message.id, agent: 'claude' })).acknowledged, true);
    const owner = `soak-${ticks}`;
    await clients[0].call('council_reserve', { owner, paths: [`soak/${ticks}.txt`], runId: id });
    assert.equal((await clients[1].call('council_release', { owner, runId: id })).released, 1);
    await clients[0].call('council_context', { key: 'soak', value: `tick ${ticks}`, author: 'codex' });
    assert.equal((await clients[1].call('council_context', { key: 'soak' })).value, `tick ${ticks}`);
  }
  if (ticks % 20 === 0) {
    const page = services[0].runPage(7); assert.ok(page.runs.length <= 7);
    if (page.nextCursor) {
      const next = services[1].runPage(7, page.nextCursor);
      assert.ok(next.runs.every(run => !page.runs.some(previous => previous.id === run.id)));
    }
    const destination = join(workspace, `backup-${ticks}.db`);
    await services[0].backup(destination); backups++;
    const db = new DatabaseSync(destination, { readOnly: true });
    try { assert.equal(db.prepare('PRAGMA quick_check').get().quick_check, 'ok'); } finally { db.close(); }
    services[0].prune({ before: new Date().toISOString(), keepLatest: 100, dryRun: false });
    const remote = await clients[0].call('council_health');
    assert.equal(remote.status, 'ok'); assert.equal(remote.state.counts.slots, 0); assert.equal(remote.state.counts.slotWaiters ?? 0, 0); assert.equal(remote.state.counts.activeRuns, 0);
    global.gc?.();
    emit('checkpoint', { ticks, workflows, cancelled, calls, backups, peakCalls, memory: process.memoryUsage(),
      eventLoopMaxMs: Math.round(loop.max / 1e6), remoteMemory: remote.memory, databaseBytes: remote.state.databaseBytes,
      walBytes: remote.state.walBytes, retainedRuns: remote.state.counts.runs, stderrBytes: clients.reduce((sum, connection) => sum + connection.stderrBytes(), 0) });
    loop.reset();
  }
  if (ticks > 0 && ticks % 50 === 0) {
    const previous = clients.shift(); await previous.client.close(); assert.deepEqual(previous.errors, []); clients.push(await connect());
  }
  ticks++;
}
emit('started', { version: VERSION, durationMs, intervalMs, rotationMs, runtimeDigest, log, liveProviderCalls: 0 });
try {
  await openRotation();
  while (!stopped && performance.now() - start < durationMs) {
    const tickStarted = performance.now(); await tick();
    if (performance.now() - rotatedAt >= rotationMs) { await closeRotation(); await openRotation(); }
    await delay(Math.max(1, Math.min(intervalMs - (performance.now() - tickStarted), durationMs - (performance.now() - start))));
  }
} catch (error) {
  failure = error; emit('failed', { message: error.message, stack: error.stack, fixture: temporary }); process.exitCode = 1;
} finally {
  loop.disable();
  process.removeListener('SIGINT', stop); process.removeListener('SIGTERM', stop);
  try { await closeRotation(); }
  catch (error) { failure ??= error; emit('cleanup.failed', { message: error.message, fixture: temporary }); process.exitCode = 1; }
  const target = resolve(realpathSync.native(temporary));
  assert.equal(dirname(target), resolve(temporaryBase)); assert.ok(basename(target).startsWith('council-soak-'));
  if (!failure) rmSync(target, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 });
  const summary = { event: failure ? 'failed' : stopped ? 'stopped' : 'completed', at: new Date().toISOString(), elapsedMs: Math.round(performance.now() - start),
    version: VERSION, runtimeDigest, ticks, workflows, cancelled, calls, backups, rotations, peakCalls, liveProviderCalls: 0, log };
  writeFileSync(join(evidence, `soak-${session}.summary.json`), JSON.stringify(summary, null, 2) + '\n');
  emit(summary.event, summary);
}
