import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { appendFileSync, cpSync, existsSync, mkdirSync, mkdtempSync, readFileSync, readdirSync, realpathSync, rmSync, statSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { basename, dirname, join, relative, resolve } from 'node:path';
import { performance } from 'node:perf_hooks';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { setTimeout as delay } from 'node:timers/promises';

const repository = fileURLToPath(new URL('../', import.meta.url));
const options = { durationMs: 60_000, cycles: 1000 };
const argumentsList = process.argv.slice(2);
for (let index = 0; index < argumentsList.length; index++) {
  const flag = argumentsList[index], value = Number(argumentsList[++index]);
  if (!Number.isInteger(value) || value < 1) throw new Error('Options require positive integer values.');
  if (flag === '--duration-ms' && value <= 3_600_000) options.durationMs = value;
  else if (flag === '--cycles' && value <= 1000) options.cycles = value;
  else throw new Error('Use --duration-ms 1..3600000 and optional --cycles 1..1000.');
}

const startedAt = new Date().toISOString(), started = performance.now();
const temporaryBase = realpathSync(tmpdir());
const temporary = mkdtempSync(join(temporaryBase, 'council-artifact-soak-'));
const runtime = join(temporary, 'runtime');
const workspace = join(temporary, 'project café 日本語');
const evidenceDirectory = join(repository, '.council', 'refinement');
mkdirSync(evidenceDirectory, { recursive: true });
const evidence = join(evidenceDirectory, `artifact-soak-${startedAt.replaceAll(':', '-')}.jsonl`);
const summaryFile = evidence.replace(/\.jsonl$/, '.summary.json');
const control = new AbortController();
let service, snapshotDigest, version, failure, interrupted = false, completedCycles = 0;
let workloadStarted;
let cancelledRuns = 0, patches = 0, patchBytes = 0, peakFixtureBytes = 0, peakHeap = 0, peakRss = 0;
const ownedGit = new Set();
const record = value => appendFileSync(evidence, JSON.stringify({ at: new Date().toISOString(), ...value }) + '\n');
const stop = () => { interrupted = true; control.abort(); void service?.close().catch(() => {}); };
process.once('SIGINT', stop); process.once('SIGTERM', stop);

function files(directory) {
  return readdirSync(directory, { withFileTypes: true }).flatMap(entry => entry.isDirectory()
    ? files(join(directory, entry.name)) : [join(directory, entry.name)]).sort();
}
function digest(directory) {
  const hash = createHash('sha256');
  for (const path of files(directory).filter(path => path.endsWith('.js'))) {
    hash.update(relative(directory, path).replaceAll('\\', '/')); hash.update('\0'); hash.update(readFileSync(path)); hash.update('\0');
  }
  return hash.digest('hex');
}
function checkedTemporary(path) {
  const target = realpathSync(path);
  if (dirname(target) !== temporaryBase || !basename(target).startsWith('council-artifact-soak-')) throw new Error('Refusing to remove an unexpected fixture path.');
  return target;
}
function fixtureSize() {
  const paths = files(temporary);
  if (paths.length > 20_000) throw new Error('Artifact fixture exceeds its 20,000-file budget.');
  const bytes = paths.reduce((total, path) => total + statSync(path).size, 0);
  peakFixtureBytes = Math.max(peakFixtureBytes, bytes);
  if (bytes > 128 * 1024 * 1024) throw new Error('Artifact fixture exceeds its 128 MiB logical-data budget.');
  return bytes;
}

try {
  mkdirSync(runtime);
  const source = join(repository, 'dist'), destination = join(runtime, 'dist');
  for (let attempt = 0; attempt < 3; attempt++) {
    const before = digest(source);
    cpSync(source, destination, { recursive: true });
    const copied = digest(destination), after = digest(source);
    if (before === copied && copied === after) { snapshotDigest = copied; break; }
    assert.equal(dirname(destination), runtime);
    rmSync(destination, { recursive: true, force: true });
  }
  if (!snapshotDigest) throw new Error('Compiled runtime changed while being captured. Retry after the build settles.');
  writeFileSync(join(runtime, 'package.json'), JSON.stringify({ private: true, type: 'module' }));
  const load = filename => import(pathToFileURL(join(destination, filename)).href);
  const [{ CouncilService }, { loadConfig }, { ManagedProcess }, { sanitizedGitEnvironment }, release] = await Promise.all([
    load('service.js'), load('config.js'), load('process.js'), load('git-env.js'), load('version.js'),
  ]);
  version = release.VERSION;
  mkdirSync(workspace);
  const hooks = join(temporary, 'disabled-hooks'), attributes = join(temporary, 'empty-attributes');
  writeFileSync(attributes, '');
  const environment = { ...sanitizedGitEnvironment(), GIT_TERMINAL_PROMPT: '0' };
  async function git(cwd, args, signal = control.signal) {
    signal.throwIfAborted();
    return await new Promise((accept, reject) => {
      const child = new ManagedProcess({ command: 'git', args: ['-c', `core.hooksPath=${hooks}`, '-c', `core.attributesFile=${attributes}`, '-C', cwd, ...args], cwd, environment });
      ownedGit.add(child);
      const chunks = []; let bytes = 0, diagnostics = '', error, stopping = false, forceTimer, forceDone;
      const halt = reason => {
        error ??= reason;
        if (stopping) return;
        stopping = true; child.kill(); forceTimer = setTimeout(() => { forceDone = child.forceKill().catch(() => {}); }, 1500);
      };
      const abort = () => halt(signal.reason instanceof Error ? signal.reason : new Error('Artifact soak cancelled.'));
      const timer = setTimeout(() => halt(new Error('Fixture Git command exceeded 30 seconds.')), 30_000);
      signal.addEventListener('abort', abort, { once: true });
      child.stdout.on('data', chunk => { bytes += chunk.length; if (bytes > 4 * 1024 * 1024) halt(new Error('Fixture Git output exceeds 4 MiB.')); else chunks.push(chunk); });
      child.stderr.on('data', chunk => { diagnostics = (diagnostics + chunk.toString('utf8')).slice(0, 4000); });
      child.stdin.on('error', () => {}); child.on('error', cause => halt(cause));
      child.once('close', async code => {
        clearTimeout(timer); clearTimeout(forceTimer); signal.removeEventListener('abort', abort); await forceDone; ownedGit.delete(child);
        if (error || code !== 0) reject(error ?? new Error(`Fixture Git failed (${code}): ${diagnostics}`));
        else accept(Buffer.concat(chunks));
      });
      child.stdin.end();
    });
  }
  await git(workspace, ['init', '--quiet']);
  assert.equal(realpathSync((await git(workspace, ['rev-parse', '--show-toplevel'])).toString('utf8').trim()), realpathSync(workspace));
  for (const [key, value] of [['user.name', 'Offline Artifact Soak'], ['user.email', 'offline@example.invalid'], ['commit.gpgsign', 'false'], ['core.fsmonitor', 'false'], ['gc.auto', '0'], ['maintenance.auto', 'false'], ['core.attributesFile', attributes]]) await git(workspace, ['config', key, value]);
  writeFileSync(join(workspace, '.gitignore'), '.council/\n'); writeFileSync(join(workspace, 'base.txt'), 'original checkout remains unchanged\n');
  await git(workspace, ['add', '.']); await git(workspace, ['commit', '--quiet', '-m', 'offline fixture base']);
  const originalHead = (await git(workspace, ['rev-parse', 'HEAD'])).toString('utf8');
  const originalIndex = readFileSync(join(workspace, '.git', 'index'));
  const originalText = readFileSync(join(workspace, 'base.txt'), 'utf8');
  const initialProcessResources = process.getActiveResourcesInfo().filter(name => /Process/i.test(name)).length;
  let current;
  const runner = {
    doctor: async () => [],
    run: async request => {
      const timeout = new AbortController(), timer = setTimeout(() => timeout.abort(new Error('Fixture actor exceeded its call deadline.')), request.timeoutMs);
      const signal = request.signal ? AbortSignal.any([request.signal, timeout.signal]) : timeout.signal;
      try {
        signal.throwIfAborted();
        const role = /You are ([^ ]+) in/.exec(request.prompt)?.[1] ?? 'unknown';
        if (request.permission === 'read') {
          const text = readFileSync(join(request.cwd, 'base.txt'), 'utf8');
          if (role === 'reviewer') assert.ok(text.includes(current.marker));
          return { agent: request.agent, status: 'completed', output: `Read verification by ${request.agent}: ${role}.`, durationMs: 1 };
        }
        const marker = `${current.marker}_${request.agent}_${role}`;
        appendFileSync(join(request.cwd, 'base.txt'), marker + '\n');
        writeFileSync(join(request.cwd, `notes café 日本語-${request.agent}-${role}.txt`), marker + '\n');
        const binary = Buffer.from(Array.from({ length: 256 }, (_, index) => (index + current.cycle) % 256));
        writeFileSync(join(request.cwd, `asset-${request.agent}.bin`), binary);
        if (current.cancel && !current.cancelRequested) {
          current.cancelRequested = true; service.requestCancellation(current.id); signal.throwIfAborted();
        }
        if (current.commit) { await git(request.cwd, ['add', '--all'], signal); await git(request.cwd, ['commit', '--quiet', '-m', marker], signal); }
        return { agent: request.agent, status: 'completed', output: `Retained ${marker}; commit=${current.commit}.`, durationMs: 1 };
      } catch (error) {
        return { agent: request.agent, status: signal.aborted ? timeout.signal.aborted ? 'timeout' : 'cancelled' : 'failed', output: `Partial fixture evidence for ${request.agent}.`, error: error instanceof Error ? error.message : 'Fixture actor failed.', durationMs: 1 };
      } finally { clearTimeout(timer); }
    },
  };
  service = new CouncilService(loadConfig(workspace, { backend: 'mock', maxConcurrency: 2, maxCalls: 16, maxOutputBytes: 4096, timeoutMs: 30_000 }), runner);
  workloadStarted = performance.now();
  record({ event: 'started', version, node: process.version, sqlite: process.versions.sqlite, platform: process.platform, architecture: process.arch, snapshotDigest, options, setupMs: Math.round(workloadStarted - started), fixture: temporary });
  for (let cycle = 0; cycle < options.cycles && performance.now() - workloadStarted < options.durationMs && !control.signal.aborted; cycle++) {
    const mode = ['parallel', 'review', 'pipeline'][cycle % 3], cycleStarted = performance.now();
    current = { cycle, marker: `ARTIFACT_SOAK_${cycle}_${mode}`, commit: cycle % 2 === 0, cancel: cycle % 5 === 4, cancelRequested: false };
    const queued = service.start({ mode, task: current.marker, agents: ['codex', 'claude'], permission: 'write', ...(mode === 'parallel' ? { judge: 'junie' } : {}) });
    current.id = queued.id;
    let run = await service.completion(queued.id);
    if (control.signal.aborted) break;
    assert.equal(run.status, current.cancel ? 'cancelled' : 'completed', run.error);
    if (current.cancel) cancelledRuns++;
    if (run.workspaces.some(artifact => artifact.exportStatus !== 'ready')) { await service.exportArtifacts(run.id, control.signal); run = service.getRun(run.id); }
    assert.equal(run.workspaces.length, mode === 'parallel' ? 2 : 1);
    for (const artifact of run.workspaces) {
      assert.equal(artifact.exportStatus, 'ready');
      const patch = readFileSync(artifact.patchPath);
      if (patch.length) {
        await git(workspace, ['apply', '--check', artifact.patchPath]);
        assert.ok(patch.includes(Buffer.from(current.marker))); assert.ok(patch.includes(Buffer.from('GIT binary patch')));
        patches++; patchBytes += patch.length;
      }
    }
    await service.cleanup(run.id, { reviewed: true, discardChanges: !current.commit || current.cancel }, control.signal);
    assert.ok(run.workspaces.every(artifact => !existsSync(artifact.path) && existsSync(artifact.patchPath)));
    assert.deepEqual(await service.cleanup(run.id, { reviewed: true, discardChanges: true }, control.signal), { removed: [], alreadyRemoved: run.workspaces.map(artifact => artifact.path) });
    assert.equal((await git(workspace, ['rev-parse', 'HEAD'])).toString('utf8'), originalHead);
    assert.deepEqual(readFileSync(join(workspace, '.git', 'index')), originalIndex);
    assert.equal(readFileSync(join(workspace, 'base.txt'), 'utf8'), originalText);
    const registrations = (await git(workspace, ['worktree', 'list', '--porcelain', '-z'])).toString('utf8').split('\0').filter(row => row.startsWith('worktree '));
    assert.equal(registrations.length, 1); assert.equal(ownedGit.size, 0);
    const health = service.store.health();
    assert.equal(health.counts.slots, 0); assert.equal(health.counts.slotWaiters, 0); assert.equal(health.counts.leases, 0);
    assert.ok(files(join(workspace, '.council')).every(path => !path.endsWith('.tmp')));
    await delay(25);
    const processResources = process.getActiveResourcesInfo().filter(name => /Process/i.test(name)).length;
    assert.ok(processResources <= initialProcessResources, 'Owned subprocess resources remained after lifecycle completion.');
    if (global.gc && cycle % 10 === 0) global.gc();
    const memory = process.memoryUsage(); peakHeap = Math.max(peakHeap, memory.heapUsed); peakRss = Math.max(peakRss, memory.rss);
    const bytes = fixtureSize(); completedCycles++;
    record({ event: 'cycle', cycle, mode, status: run.status, committed: current.commit, workspaces: run.workspaces.length, durationMs: Math.round(performance.now() - cycleStarted), fixtureBytes: bytes, heapUsed: memory.heapUsed, rss: memory.rss, processResources });
  }
} catch (error) {
  failure = error instanceof Error ? error.stack?.slice(0, 8000) ?? error.message : String(error).slice(0, 8000);
  record({ event: 'failed', error: failure }); process.exitCode = 1;
} finally {
  try { await service?.close(); } catch (error) { failure ??= error instanceof Error ? error.message : String(error); process.exitCode = 1; }
  await Promise.allSettled([...ownedGit].map(child => child.forceKill()));
  process.removeListener('SIGINT', stop); process.removeListener('SIGTERM', stop);
  const summary = { startedAt, finishedAt: new Date().toISOString(), elapsedMs: Math.round(performance.now() - started), setupMs: workloadStarted === undefined ? undefined : Math.round(workloadStarted - started), workloadMs: workloadStarted === undefined ? undefined : Math.round(performance.now() - workloadStarted), success: !failure && !interrupted, interrupted, version, snapshotDigest, completedCycles, cancelledRuns, patches, patchBytes, peakFixtureBytes, peakHeap, peakRss, ...(failure ? { error: failure, preservedFixture: temporary } : {}) };
  if (failure) writeFileSync(join(temporary, 'FAILED.json'), JSON.stringify(summary, null, 2) + '\n');
  else rmSync(checkedTemporary(temporary), { recursive: true, force: true });
  writeFileSync(summaryFile, JSON.stringify(summary, null, 2) + '\n'); record({ event: 'finished', ...summary });
  process.stdout.write(JSON.stringify({ ...summary, evidence, summaryFile }, null, 2) + '\n');
}
