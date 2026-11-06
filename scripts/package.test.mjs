import assert from 'node:assert/strict';
import { execFileSync, spawn } from 'node:child_process';
import { once } from 'node:events';
import { chmod, mkdir, mkdtemp, readFile, realpath, lstat, rm, writeFile } from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import { createInterface } from 'node:readline';
import { setTimeout as delay } from 'node:timers/promises';
import test from 'node:test';
import { fileURLToPath } from 'node:url';
import { inflateRawSync } from 'node:zlib';
import { packagePreview, sha256, version } from './package.mjs';
import { qualifyExtractedWorkflow } from './package-workflow.mjs';

const root = fileURLToPath(new URL('../', import.meta.url));
const executable = process.platform === 'win32' ? 'council.exe' : 'council';
const expected = [executable, 'README.md', 'LICENSE', 'third-party/brand-assets.md',
  'third-party/ejupilabs-icons/LICENSE', 'third-party/go-NOTICES.txt', 'SHA256SUMS.txt'].sort();

// Read the portable ZIP central directory independently of the Go writer. A generic
// archive extractor is deliberately not a release user's runtime dependency.
function readZip(data) {
  const end = data.length - 22;
  assert.equal(data.readUInt32LE(end), 0x06054b50, 'archive must have an uncommented ZIP end record');
  const count = data.readUInt16LE(end + 10);
  let cursor = data.readUInt32LE(end + 16);
  const entries = new Map();
  for (let i = 0; i < count; i++) {
    assert.equal(data.readUInt32LE(cursor), 0x02014b50);
    const method = data.readUInt16LE(cursor + 10);
    const compressedSize = data.readUInt32LE(cursor + 20);
    const size = data.readUInt32LE(cursor + 24);
    const nameLength = data.readUInt16LE(cursor + 28);
    const extraLength = data.readUInt16LE(cursor + 30);
    const commentLength = data.readUInt16LE(cursor + 32);
    const mode = data.readUInt32LE(cursor + 38) >>> 16;
    const offset = data.readUInt32LE(cursor + 42);
    const name = data.subarray(cursor + 46, cursor + 46 + nameLength).toString('utf8');
    assert.ok(expected.includes(name), 'archive contains only allowlisted runtime and notice files');
    assert.ok(!entries.has(name), 'archive must not contain duplicate filenames');
    assert.equal(data.readUInt32LE(offset), 0x04034b50);
    assert.equal(data.readUInt16LE(offset + 8), method);
    const start = offset + 30 + data.readUInt16LE(offset + 26) + data.readUInt16LE(offset + 28);
    const compressed = data.subarray(start, start + compressedSize);
    assert.equal(method, 8, 'archive uses portable deflate compression');
    const contents = inflateRawSync(compressed);
    assert.equal(contents.length, size);
    entries.set(name, { contents, mode });
    cursor += 46 + nameLength + extraLength + commentLength;
  }
  assert.deepEqual([...entries.keys()].sort(), expected);
  return entries;
}

function privateEnvironment(home, emptyPath) {
  const environment = { ...process.env };
  // Windows environment names are case-insensitive, unlike JavaScript object keys.
  for (const key of Object.keys(environment)) {
    if (['path', 'home', 'userprofile', 'homedrive', 'homepath', 'node_options'].includes(key.toLowerCase()) || key.toLowerCase().startsWith('git_')) delete environment[key];
  }
  return { ...environment, PATH: emptyPath, HOME: home, USERPROFILE: home, GOPROXY: 'off', GOSUMDB: 'off', GIT_CONFIG_GLOBAL: '/dev/null', GIT_CONFIG_NOSYSTEM: '1', GIT_TERMINAL_PROMPT: '0' };
}

async function stop(child) {
  if (child.exitCode !== null || child.signalCode !== null) return;
  const exited = once(child, 'exit');
  child.kill('SIGTERM');
  await exited;
}

test('native preview has exact contents, complete notices and an independently verifiable checksum', { timeout: 300_000 }, async t => {
  const temporary = await mkdtemp(path.join(os.tmpdir(), 'council-package-'));
  let child;
  let ownerOptions;
  let ownerInstanceId;
  const runBinary = (binary, args, options) => {
    const envelope = JSON.parse(execFileSync(binary, [...args, '--json'], { ...options, timeout: 30_000 }));
    assert.ok(!envelope.error, 'extracted operation has an acknowledged result');
    return envelope;
  };
  const stopOwnedService = async binary => {
    const current = child;
    const exited = current && current.exitCode === null && current.signalCode === null ? once(current, 'exit') : Promise.resolve([current?.exitCode, current?.signalCode]);
    const acknowledgement = runBinary(binary, ['service', 'stop', '--owner-instance', ownerInstanceId], ownerOptions);
    assert.equal(acknowledgement.data.shutdownAccepted, true);
    const [code, signal] = await Promise.race([exited, delay(20_000, undefined, { ref: false }).then(() => { throw new Error('Extracted owner did not finish clean service shutdown.'); })]);
    assert.equal(code, 0, 'explicit service shutdown exits the actual foreground owner cleanly');
    assert.equal(signal, null);
    child = undefined;
    ownerInstanceId = undefined;
  };
  t.after(async () => {
    if (child) {
      try { if (ownerOptions && ownerInstanceId) await stopOwnedService(path.join(temporary, 'extracted', executable)); else await stop(child); }
      catch { await stop(child); }
    }
    const resolved = await realpath(temporary);
    const temporaryRoot = await realpath(os.tmpdir());
    if (path.relative(temporaryRoot, path.dirname(resolved)) !== '' || !path.basename(resolved).startsWith('council-package-') || (await lstat(temporary)).isSymbolicLink()) {
      throw new Error('Refusing cleanup outside the created package fixture.');
    }
    await rm(resolved, { recursive: true, force: true, maxRetries: 20, retryDelay: 100 });
  });
  execFileSync('go', ['test', './scripts/packagearchive'], { cwd: root, shell: false, stdio: 'pipe' });
  const release = await packagePreview({ outputDirectory: path.join(temporary, 'releases') });
  const data = await readFile(release.archive);
  assert.equal(await readFile(release.checksum, 'utf8'), `${sha256(data)}  ${release.archiveName}\n`);
  assert.match(release.archiveName, /^council-2\.0\.0-dev\.5-(windows|darwin|linux)-(amd64|arm64)\.zip$/);
  const entries = readZip(data);
  const manifest = entries.get('SHA256SUMS.txt').contents.toString('utf8');
  const records = manifest.trimEnd().split('\n').map(line => line.match(/^([a-f0-9]{64})  (.+)$/));
  assert.ok(records.every(Boolean), 'each checksum record has one exact relative filename');
  assert.deepEqual(records.map(record => record[2]).sort(), expected.filter(name => name !== 'SHA256SUMS.txt'));
  for (const [, digest, name] of records) assert.equal(sha256(entries.get(name).contents), digest, `checksum matches ${name}`);
  assert.equal(entries.get(executable).mode & 0o777, 0o755, 'Unix extraction preserves executable permissions');
  assert.ok(entries.get(executable).contents.equals(await readFile(path.join(root, 'build', executable))), 'package contains the freshly built native binary');
  const notices = entries.get('third-party/go-NOTICES.txt').contents.toString('utf8');
  const goModule = await readFile(path.join(root, 'go.mod'), 'utf8');
  for (const [, module, pinned] of goModule.matchAll(/^\s+([^\s]+) (v[^\s]+)(?: \/\/ indirect)?$/gm)) {
    assert.ok(notices.includes(`===== ${module}@${pinned}/LICENSE =====`), `license inventory covers ${module}@${pinned}`);
  }
  for (const notice of ['Go 1.27.3/LICENSE', 'Go 1.27.3/PATENTS', 'LICENSE-3RD-PARTY.md', 'LICENSE-SQLITE', 'LICENSE-MMAP-GO']) {
    assert.ok(notices.includes(notice), `retained upstream notice ${notice}`);
  }
  assert.match(entries.get('README.md').contents.toString('utf8'), /verified workflow preview/);
  assert.match(entries.get('README.md').contents.toString('utf8'), /live\s+provider compatibility remains (?:separately )?unqualified/i);
  assert.match(entries.get('README.md').contents.toString('utf8'), /fixed fake mode uses only local processes/);
  for (const { contents } of entries.values()) {
    for (const forbidden of [root.replaceAll('\\', '/').replace(/\/$/, ''), root.replaceAll('/', '\\').replace(/\\$/, '')]) {
      assert.equal(contents.includes(Buffer.from(forbidden)), false, 'release contains no developer workspace path');
    }
  }
  const repeat = await packagePreview({ outputDirectory: path.join(temporary, 'repeat'), build: false });
  assert.equal(sha256(await readFile(repeat.archive)), sha256(data), 'fixed input produces reproducible ZIP bytes');

  const extracted = path.join(temporary, 'extracted');
  for (const [name, { contents, mode }] of entries) {
    const filename = path.join(extracted, name);
    await mkdir(path.dirname(filename), { recursive: true });
    await writeFile(filename, contents);
    await chmod(filename, mode & 0o777);
  }
  const elsewhere = path.join(temporary, 'unrelated-working-directory');
  const home = path.join(temporary, 'isolated-home');
  const emptyPath = path.join(temporary, 'empty-path');
  for (const directory of [elsewhere, home, emptyPath]) await mkdir(directory);
  const environment = privateEnvironment(home, emptyPath);
  const binary = path.join(extracted, executable);
  const options = { cwd: elsewhere, env: environment, shell: false, encoding: 'utf8', timeout: 10_000 };
  const metadata = JSON.parse(execFileSync(binary, ['--version', '--json'], options));
  assert.equal(metadata.data.appVersion, version);
  assert.equal(metadata.data.apiVersion, 1);
  const policy = JSON.parse(execFileSync(binary, ['policy', 'get', '--json'], options));
  assert.ok(policy.data && !policy.error, 'extracted binary can initialize private storage without Node, Go or Git on PATH');

  const launchOwner = async runtimeOptions => {
    ownerOptions = runtimeOptions;
    child = spawn(binary, ['ui', '--no-open', '--json'], { cwd: runtimeOptions.cwd, env: runtimeOptions.env, shell: false, stdio: ['ignore', 'pipe', 'pipe'] });
    const lines = createInterface({ input: child.stdout });
    try {
      const launch = await Promise.race([
        once(lines, 'line').then(([line]) => JSON.parse(line)),
        once(child, 'exit').then(() => { throw new Error('Extracted preview stopped before reporting UI startup.'); }),
        delay(15_000, undefined, { ref: false }).then(() => { throw new Error('Extracted preview did not report bounded UI readiness.'); }),
      ]);
      ownerInstanceId = launch.data?.ownerInstanceId;
      return launch;
    } finally { lines.close(); }
  };
  const launch = await launchOwner(options);
  assert.ok(launch.data?.url && !launch.error, 'extracted binary reports native UI startup');
  const url = new URL(launch.data.url);
  assert.equal(url.hostname, '127.0.0.1');
  const accessToken = new URLSearchParams(url.hash.slice(1)).get('token');
  const ownerMetadata = await (await fetch(url.origin + '/api/v1/meta', { headers: { Authorization: 'Bearer ' + accessToken }, signal: AbortSignal.timeout(10_000) })).json();
  assert.equal(ownerMetadata.data.appVersion, '2.0.0-dev.5');
  assert.equal(ownerMetadata.data.executionContract, '005-v1');
  assert.equal(ownerMetadata.data.maintenanceContract, 'owner-stop/1');
  assert.equal(ownerMetadata.data.managedExecution, 'native_codex_claude');
  assert.ok(ownerMetadata.data.capabilities.includes('economical_routing'));
  assert.equal(ownerMetadata.data.capacity.resourceId, 'managed-native');
  const declared = await (await fetch(url.origin + '/api/v1/execution/capabilities', { headers: { Authorization: 'Bearer ' + accessToken }, signal: AbortSignal.timeout(10_000) })).json();
  assert.deepEqual(declared.data.connections.map(connection => connection.connectionId), ['codex-native', 'claude-native']);
  assert.equal(declared.data.connections[1].liveQualification, 'unqualified');
  const response = await fetch(url.origin, { signal: AbortSignal.timeout(10_000) });
  assert.equal(response.status, 200);
  const html = await response.text();
  assert.match(html, /Ejupi Labs/);
  const resources = [...html.matchAll(/(?:src|href)="([^"]+)"/g)].map(match => match[1]).filter(value => !value.startsWith('#'));
  assert.ok(resources.length > 0, 'embedded page identifies its embedded public resources');
  for (const resource of resources) {
    const asset = new URL(resource, url.origin);
    assert.equal(asset.origin, url.origin, 'runtime resources remain local and embedded');
    assert.equal((await fetch(asset, { signal: AbortSignal.timeout(10_000) })).status, 200, 'extracted preview serves its embedded assets');
  }
  await stopOwnedService(binary);

  // Node and Go are qualification tools above; the extracted application needs neither.
  // This stage's PATH contains the resolved Git installation directory only. That
  // directory may also contain Git helpers or system tools; it is not advertised as
  // a directory containing exclusively one executable.
  const hostPath = Object.entries(process.env).find(([key]) => key.toLowerCase() === 'path')?.[1] ?? '';
  let git;
  for (const directory of hostPath.split(path.delimiter).filter(Boolean)) {
    const candidate = path.join(directory.replace(/^"(.*)"$/, '$1'), process.platform === 'win32' ? 'git.exe' : 'git');
    try { const physical = await realpath(candidate); if ((await lstat(physical)).isFile()) { git = physical; break; } } catch {}
  }
  assert.ok(git, 'native Git is a declared prerequisite for extracted source capture');
  const gitDirectory = path.dirname(git);
  const colocatedDevelopmentTools = [];
  for (const name of process.platform === 'win32' ? ['node.exe', 'go.exe', 'npm.cmd'] : ['node', 'go', 'npm']) {
    try { await lstat(path.join(gitDirectory, name)); colocatedDevelopmentTools.push(name); } catch {}
  }
  t.diagnostic(colocatedDevelopmentTools.length ? `Git's directory also contains ${colocatedDevelopmentTools.join(', ')}; empty-PATH startup above is independently verified.` : 'Execution PATH excludes Node, npm and Go; only the Git installation directory is supplied.');
  const gitEnvironment = privateEnvironment(home, gitDirectory);
  const runtimeOptions = { cwd: elsewhere, env: gitEnvironment, shell: false, encoding: 'utf8', timeout: 30_000 };
  const repository = path.join(temporary, 'runtime-project');
  await mkdir(repository);
  for (const args of [['init', '-b', 'fixture'], ['-c', 'user.name=Fixture', '-c', 'user.email=fixture@example.invalid', 'commit', '--allow-empty', '-m', 'fixture']]) {
    execFileSync(git, ['--no-optional-locks', '-c', 'core.fsmonitor=false', '-C', repository, ...args], runtimeOptions);
  }
  await launchOwner(runtimeOptions);
  const project = runBinary(binary, ['projects', 'open', repository], runtimeOptions).data.project;
  const admitted = runBinary(binary, ['tasks', 'start', '--project', project.id, '--task', 'Extracted offline fake response fixture.', '--deliverable', 'response_only', '--workflow', 'author_only', '--fake', '--timeout-seconds', '60'], runtimeOptions).data;
  assert.equal(admitted.attempt.executionMode, 'fake');
  const settled = async attemptId => {
    const deadline = Date.now() + 25_000;
    while (Date.now() < deadline) {
      const attempt = runBinary(binary, ['attempts', 'get', attemptId], runtimeOptions).data;
      if (attempt.phase === 'settled') return attempt;
      await delay(40);
    }
    throw new Error('Extracted fake attempt did not settle within its qualification bound.');
  };
  const result = await settled(admitted.attempt.id);
  assert.equal(result.outcome, 'succeeded', 'fake authoring succeeds without implying task acceptance');
  assert.equal(result.cleanup.state, 'verified');
  assert.ok(result.responseArtifactId);
  const metadataArtifact = runBinary(binary, ['artifacts', 'get', result.responseArtifactId], runtimeOptions).data;
  const responseChunk = runBinary(binary, ['artifacts', 'read', result.responseArtifactId], runtimeOptions).data;
  const responseBytes = Buffer.from(responseChunk.bytesBase64, 'base64');
  assert.equal(responseChunk.eof, true);
  assert.equal(responseBytes.toString('utf8'), 'Council deterministic fake response (002-v1).\nComplete fixture evidence: café, 東京, π.\nThis is a local fixture, not a provider result.\n');
  assert.equal(sha256(responseBytes), metadataArtifact.sha256);
  assert.equal(BigInt(metadataArtifact.byteLength), BigInt(responseBytes.length));
  assert.equal(metadataArtifact.closed, true);

  const claude = runBinary(binary, ['tasks', 'start', '--project', project.id, '--task', 'Extracted offline Claude fixture.', '--harness', 'claude-code', '--deliverable', 'response_only', '--workflow', 'author_only', '--fake', '--timeout-seconds', '60'], runtimeOptions).data;
  assert.equal(claude.attempt.connectionId, 'claude-native');
  assert.equal(claude.version.pins.harnessId, 'claude-code');
  const claudeResult = await settled(claude.attempt.id);
  assert.equal(claudeResult.outcome, 'succeeded');
  assert.equal(claudeResult.cleanup.state, 'verified');
  assert.equal(claudeResult.targetObservations.provider.status, 'unknown');
  assert.equal(claudeResult.targetObservations.usage.inputTokens, null);
  const claudeMetadata = runBinary(binary, ['artifacts', 'get', claudeResult.responseArtifactId], runtimeOptions).data;
  const claudeChunk = runBinary(binary, ['artifacts', 'read', claudeResult.responseArtifactId], runtimeOptions).data;
  const claudeBytes = Buffer.from(claudeChunk.bytesBase64, 'base64');
  assert.equal(claudeChunk.eof, true);
  assert.equal(sha256(claudeBytes), claudeMetadata.sha256);
  assert.equal(BigInt(claudeMetadata.byteLength), BigInt(claudeBytes.length));
  assert.match(claudeBytes.toString('utf8'), /^\ufeff/);
  assert.match(claudeBytes.toString('utf8'), /café, 東京, π/);
  assert.match(claudeBytes.toString('utf8'), /\r\n/);

  // Reports are user declarations; a fake worker never measures provider capacity.
  const capacityBefore = runBinary(binary, ['routing', 'capacity', '--context', 'codex-native'], runtimeOptions).data;
  const busy = runBinary(binary, ['routing', 'report', '--context', 'codex-native',
    '--dimension', 'occupancy', '--state', 'busy', '--expected-revision', capacityBefore.revision,
    '--valid-for-seconds', '900'], runtimeOptions).data;
  assert.equal(busy.report.source, 'user_report');
  assert.equal(busy.report.subject.kind, 'context');
  assert.equal(busy.report.subject.id, 'codex-native');
  assert.equal(busy.capacity.dimensions.occupancy.effectiveState, 'busy');
  assert.equal(busy.capacity.reportEligibility.permitted, false);

  const autoText = '\ufeffExtracted suitable Auto fixture.\r\nKeep original selector intent empty.';
  const autoArgs = ['tasks', 'start', '--project', project.id, '--task', autoText,
    '--suitability', 'routine', '--budget-admissions', '2', '--deliverable', 'response_only', '--workflow', 'author_only', '--fake', '--timeout-seconds', '60'];
  const autoEnvelope = runBinary(binary, autoArgs, runtimeOptions);
  const auto = autoEnvelope.data;
  assert.equal(auto.attempt.connectionId, 'claude-native');
  assert.equal(auto.attempt.resolvedTarget.contextId, 'claude-native');
  assert.equal(auto.attempt.resolvedTarget.modelSelector, 'haiku');
  assert.equal(auto.attempt.resolvedTarget.profileId, 'claude-haiku-economy');
  assert.equal(auto.version.pins.harnessId ?? '', '');
  assert.equal(auto.version.pins.model ?? '', '');
  assert.equal(auto.routingDecision.selectedTarget.profileId, 'claude-haiku-economy');
  assert.equal(auto.routingDecision.budgetEffect.admissionUnits, '1');
  assert.equal(auto.budget.grantedAdmissions, '2');
  assert.equal(auto.budget.consumedAdmissions, '1');
  assert.equal(auto.budget.remainingAdmissions, '1');
  assert.equal(auto.budget.currentWindow.windowSeconds, 1800);
  assert.ok(auto.budget.currentWindow.firstAdmissionAt && auto.budget.currentWindow.deadlineAt);
  const replayedAuto = runBinary(binary, [...autoArgs, '--request-id', autoEnvelope.requestId], runtimeOptions);
  assert.deepEqual(replayedAuto, autoEnvelope, 'exact extracted Auto replay preserves its original admission and one budget debit');
  const autoResult = await settled(auto.attempt.id);
  assert.equal(autoResult.outcome, 'succeeded');
  assert.equal(autoResult.cleanup.state, 'verified');
  assert.equal(autoResult.targetObservations.provider.status, 'unknown');
  assert.equal(autoResult.targetObservations.account.status, 'unknown');
  assert.equal(autoResult.targetObservations.model.status, 'unknown');
  const autoTask = runBinary(binary, ['tasks', 'get', auto.task.id], runtimeOptions).data;
  assert.equal(autoTask.task.originalText, autoText);
  const autoRouting = runBinary(binary, ['tasks', 'routing', auto.task.id], runtimeOptions).data;
  assert.equal(autoRouting.adoptionRequired, false);
  assert.equal(autoRouting.authority.suitabilityAssessment.source, 'user_declared');
  assert.equal(autoRouting.latestDecision.attemptId, auto.attempt.id);
  assert.equal(autoRouting.latestDecision.selectedTarget.profileId, 'claude-haiku-economy');
  const autoBudget = runBinary(binary, ['tasks', 'budget', auto.task.id], runtimeOptions).data;
  assert.equal(autoBudget.budget.consumedAdmissions, '1');
  assert.equal(autoBudget.budget.remainingAdmissions, '1');
  assert.equal(autoBudget.budget.currentWindow.deadlineAt, auto.budget.currentWindow.deadlineAt);
  const ledger = runBinary(binary, ['tasks', 'budget-ledger', auto.task.id], runtimeOptions).data;
  assert.equal(ledger.nextCursor, null);
  assert.equal(ledger.items.filter(item => item.kind === 'admission').length, 1);
  assert.equal(ledger.items.filter(item => item.kind === 'window_open').length, 1);
  assert.equal(ledger.items.filter(item => item.kind === 'launch_intent').length, 1);
  assert.equal(ledger.items.filter(item => item.kind === 'launch_started').length, 1);
  assert.equal(ledger.items.reduce((sum, item) => sum + BigInt(item.consumedDelta), 0n), 1n);
  assert.equal(ledger.items.reduce((sum, item) => sum + BigInt(item.grantedDelta), 0n), 2n);
  const usage = runBinary(binary, ['tasks', 'usage', auto.task.id], runtimeOptions).data;
  assert.equal(usage.nextCursor, null);
  assert.equal(usage.items.length, 1);
  assert.equal(usage.items[0].attemptId, auto.attempt.id);
  assert.equal(usage.items[0].selectedUsageScope, 'fake_unmeasured');
  assert.deepEqual(usage.coverage.excludedFakeAttemptIds, [auto.attempt.id]);
  assert.equal(usage.knownSubtotal.scope, 'returned_attempts');
  assert.equal(usage.knownSubtotal.byUsageScope.top_level.inputTokens.knownValue, null);
  assert.equal(usage.knownSubtotal.byUsageScope.whole_tree.inputTokens.knownValue, null);
  assert.deepEqual(usage.estimates, []);
  assert.deepEqual(usage.charges, { status: 'unknown', items: [] });

  await qualifyExtractedWorkflow({ binary, runtimeOptions, repository, temporary, project, git, runBinary });

  // The same extracted binary can restart and read retained bytes without rerunning work.
  await stopOwnedService(binary);
  await launchOwner(runtimeOptions);
  const retained = runBinary(binary, ['artifacts', 'read', result.responseArtifactId, '--through', responseChunk.through], runtimeOptions).data;
  assert.equal(retained.bytesBase64, responseChunk.bytesBase64);
  const retainedAttempt = runBinary(binary, ['attempts', 'get', result.id], runtimeOptions).data;
  assert.equal(retainedAttempt.outcome, 'succeeded');
  const retainedClaude = runBinary(binary, ['artifacts', 'read', claudeResult.responseArtifactId, '--through', claudeChunk.through], runtimeOptions).data;
  assert.equal(retainedClaude.bytesBase64, claudeChunk.bytesBase64);

  const slow = runBinary(binary, ['tasks', 'start', '--project', project.id, '--task', 'Extracted cancellation fixture.', '--harness', 'claude-code', '--workflow', 'author_only', '--fake', '--fake-scenario', 'slow', '--timeout-seconds', '60'], runtimeOptions).data;
  const runningDeadline = Date.now() + 15_000;
  let running = false;
  while (Date.now() < runningDeadline) {
    const observed = runBinary(binary, ['attempts', 'get', slow.attempt.id], runtimeOptions).data;
    if (observed.phase === 'running') { running = true; break; }
    assert.notEqual(observed.phase, 'settled');
    await delay(40);
  }
  assert.equal(running, true, 'extracted fixed slow fixture reaches actual native launch');
  const stopped = runBinary(binary, ['attempts', 'stop', slow.attempt.id], runtimeOptions);
  assert.equal(stopped.data.stopAccepted, true);
  const cancelled = await settled(slow.attempt.id);
  assert.equal(cancelled.outcome, 'cancelled');
  assert.equal(cancelled.cleanup.state, 'verified');
  await stopOwnedService(binary);
});
