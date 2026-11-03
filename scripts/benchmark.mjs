import assert from 'node:assert/strict';
import { basename, dirname, join, resolve } from 'node:path';
import { mkdtempSync, realpathSync, rmSync } from 'node:fs';
import { arch, cpus, platform, tmpdir } from 'node:os';
import { randomUUID } from 'node:crypto';
import { performance } from 'node:perf_hooks';
import { CouncilStore } from '../dist/store.js';
import { CouncilService } from '../dist/service.js';
import { loadConfig } from '../dist/config.js';
import { summarizeRun } from '../dist/summary.js';
import { VERSION } from '../dist/version.js';

const RECORDS = 20;
const ITERATIONS = 10;
const WARMUP = 2;
const MAX_FIXTURE_BYTES = 25 * 1024 * 1024;
const temporaryBase = realpathSync(tmpdir());
const temporary = mkdtempSync(join(temporaryBase, 'council-benchmark-'));
let store;
let service;

function bytes(value) { return Buffer.byteLength(JSON.stringify(value), 'utf8'); }
function rounded(value) { return Math.round(value * 1000) / 1000; }
function usage(startCpu, startWall) {
  const cpu = process.cpuUsage(startCpu);
  return { wallMs: rounded(performance.now() - startWall), cpuMs: rounded((cpu.user + cpu.system) / 1000) };
}
function measure(action) {
  for (let i = 0; i < WARMUP; i++) action();
  const cpu = process.cpuUsage(), wall = performance.now();
  let value;
  for (let i = 0; i < ITERATIONS; i++) value = action();
  return { ...usage(cpu, wall), iterations: ITERATIONS, resultBytes: bytes(value) };
}
function sharedFields(summaries) {
  return summaries.map(({ revision: _revision, cancellationRequested: _cancel, ...summary }) => summary);
}

try {
  store = new CouncilStore(join(temporary, '.council'));
  const ids = [];
  const recordSizes = new Map();
  let fixtureBytes = 0, eventCount = 0;
  const now = Date.now();
  const append = (id, type, data) => {
    fixtureBytes += bytes(data);
    if (fixtureBytes > MAX_FIXTURE_BYTES) throw new Error('Synthetic fixture payload exceeds its 25 MiB budget.');
    store.appendEvent(id, type, data); eventCount++;
  };
  for (let index = 0; index < RECORDS; index++) {
    const id = randomUUID(); ids.push(id);
    const timestamp = new Date(now + index).toISOString();
    const run = {
      id, input: { mode: 'council', agents: ['codex', 'claude', 'junie'], judge: 'deepseek-harness', task: 'Synthetic offline benchmark fixture.', permission: 'read' },
      status: index === 0 ? 'running' : 'completed', createdAt: timestamp, updatedAt: timestamp,
      steps: Array.from({ length: 10 }, (_, step) => ({ id: `${step}-offline`, agent: 'codex', role: 'benchmark-evidence', result: { agent: 'codex', status: 'completed', output: 'e'.repeat(128_000), durationMs: 1 } })),
      output: index === 0 ? 'r'.repeat(131_072) : 'Synthetic selected result. No provider was called.',
    };
    const size = bytes(run); recordSizes.set(id, size); fixtureBytes += size;
    if (fixtureBytes > MAX_FIXTURE_BYTES) throw new Error('Synthetic fixture payload exceeds its 25 MiB budget.');
    store.saveRun(run);
    append(id, 'run.started', { mode: run.input.mode, fixture: true });
    append(id, 'step.finished', { agent: 'codex', status: 'completed', durationMs: 1 });
  }
  // One bounded transcript history makes cancellation discovery a realistic payload comparison.
  const target = ids[0];
  for (let i = 0; i < 32; i++) append(target, 'step.finished', { agent: 'codex', status: 'completed', output: 'p'.repeat(8192), fixture: true });

  const legacyList = () => store.listRuns(RECORDS).map(run => summarizeRun(run));
  const metadataList = () => store.listRunSummaries(RECORDS);
  assert.ok(JSON.stringify(sharedFields(legacyList())) === JSON.stringify(sharedFields(metadataList())), 'Compact summaries must preserve the shared public JSON fields.');
  const list = { legacySnapshots: measure(legacyList), metadataSummaries: measure(metadataList), sharedFieldsEqual: true,
    legacySnapshotBytesPerIteration: [...recordSizes.values()].reduce((sum, size) => sum + size, 0),
    metadataSummaryBytesPerIteration: bytes(metadataList()),
    additionalMetadataFields: ['revision', 'cancellationRequested'] };

  append(target, 'cancel_requested', { fixture: true });
  const legacyCancellation = () => store.events(target).some(event => event.type === 'cancel_requested');
  const metadataCancellation = () => store.isCancellationRequested(target);
  assert.equal(legacyCancellation(), true); assert.equal(metadataCancellation(), true);
  const cancellation = { legacyEvents: measure(legacyCancellation), metadataFlag: measure(metadataCancellation), equivalent: true,
    legacyEventPayloadBytesPerRead: bytes(store.events(target)) };

  service = new CouncilService(loadConfig(temporary, { backend: 'mock' }));
  const summaryBytes = bytes(service.store.getRunSummary(target));
  const full = service.store.getRun.bind(service.store), compact = service.store.getRunSummary.bind(service.store);
  let fullReads = 0, summaryReads = 0;
  service.store.getRun = id => { fullReads++; return full(id); };
  service.store.getRunSummary = id => { summaryReads++; return compact(id); };
  const cpu = process.cpuUsage(), wall = performance.now();
  await Promise.all(Array.from({ length: 20 }, () => service.waitSummary(target, 500)));
  const foreignWaiters = { ...usage(cpu, wall), waiters: 20, requestedWaitMs: 500,
    fullDocumentReads: fullReads, summaryReads, fullDocumentBytes: recordSizes.get(target), summaryBytes,
    decodedPayloadBytes: fullReads * recordSizes.get(target) + summaryReads * summaryBytes };

  process.stdout.write(`${JSON.stringify({
    benchmark: 'Council offline metadata efficiency', version: VERSION, measuredAt: new Date().toISOString(),
    machine: { platform: platform(), arch: arch(), node: process.version, cpu: cpus()[0]?.model ?? 'unknown', logicalCpus: cpus().length },
    fixture: { records: RECORDS, events: eventCount, payloadBytes: fixtureBytes, payloadBudgetBytes: MAX_FIXTURE_BYTES,
      meanRecordBytes: Math.round([...recordSizes.values()].reduce((sum, size) => sum + size, 0) / RECORDS), liveProviderCalls: 0 },
    warmupIterations: WARMUP, list, cancellation, foreignWaiters,
    interpretation: 'Timings depend on this machine and its current load. Semantic equivalence is checked; timing ratios are observations, not pass/fail assertions. Fixtures are temporary; no existing Council database is read.',
  }, null, 2)}\n`);
} finally {
  try { await service?.close(); }
  finally {
    store?.close();
    const target = resolve(realpathSync(temporary));
    if (dirname(target) !== resolve(temporaryBase) || !basename(target).startsWith('council-benchmark-')) throw new Error('Refusing to remove an unexpected benchmark directory.');
    rmSync(target, { recursive: true, force: true });
  }
}
