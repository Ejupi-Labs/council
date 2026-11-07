import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { createHash, randomBytes } from 'node:crypto';
import { lstat, open, readFile, realpath, rename, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { setTimeout as delay } from 'node:timers/promises';

const fileA = 'council-graph-a.txt', fileB = 'council-graph-b.txt';
const contentA = 'Council controlled graph branch A: café, 東京.\r\n';
const contentB = 'Council controlled graph branch B: π, 🙂.\r\n';
const readyFile = 'council-graph-ready.marker', releaseFile = 'council-graph-release.marker';
const sha256 = bytes => createHash('sha256').update(bytes).digest('hex');

// Node is the qualification runner, never an extracted runtime dependency.
// Every author/reviewer below is an actual fixed worker of the extracted binary.
export async function qualifyExtractedGraphs({ binary, runtimeOptions, repository, temporary, project, home, ownerURL, git, runBinary, busyReportId }) {
  const run = args => runBinary(binary, args, runtimeOptions);
  const owner = new URL(ownerURL);
  assert.equal(owner.hostname, '127.0.0.1');
  const token = new URLSearchParams(owner.hash.slice(1)).get('token');
  assert.ok(token, 'the isolated owner supplied its local authenticated request capability');
  const get = async route => {
    const response = await fetch(owner.origin + '/api/v1' + route, {
      headers: { Authorization: 'Bearer ' + token }, signal: AbortSignal.timeout(5000),
    });
    const envelope = await response.json();
    assert.equal(response.status, 200, route + ': ' + JSON.stringify(envelope.error));
    assert.ok(!envelope.error, route + ': complete acknowledged read');
    return envelope.data;
  };
  const poll = async (route, predicate, label, timeout = 40_000) => {
    const end = Date.now() + timeout;
    let observed;
    while (Date.now() < end) {
      observed = await get(route);
      if (predicate(observed)) return observed;
      await delay(25);
    }
    throw new Error(label + ' did not reach its bounded actual state: ' + JSON.stringify(observed));
  };
  const readRef = ref => {
    const metadata = run(['artifacts', 'get', ref.artifactId]).data;
    assert.equal(metadata.id, ref.artifactId);
    assert.equal(metadata.attemptId, ref.producerAttemptId);
    assert.equal(metadata.sha256, ref.sha256);
    assert.equal(metadata.byteLength, ref.byteLength);
    assert.equal(metadata.closed, true);
    assert.equal(metadata.completeness, 'complete');
    const chunks = [];
    let offset = '0';
    for (;;) {
      const chunk = run(['artifacts', 'read', ref.artifactId, '--offset', offset, '--through', ref.byteLength, '--limit', '65536']).data;
      chunks.push(Buffer.from(chunk.bytesBase64, 'base64'));
      if (chunk.eof) break;
      assert.ok(BigInt(chunk.nextOffset) > BigInt(offset), 'full artifact reads must advance without clipping');
      offset = chunk.nextOffset;
    }
    const bytes = Buffer.concat(chunks);
    assert.equal(BigInt(bytes.length), BigInt(ref.byteLength));
    assert.equal(sha256(bytes), ref.sha256);
    return bytes;
  };
  const jsonRef = ref => JSON.parse(readRef(ref).toString('utf8'));
  const start = async (name, declaration) => {
    const filename = path.join(temporary, 'extracted-' + name + '-graph.json');
    const body = { requestId: randomBytes(16).toString('hex'), projectId: project.id, executionMode: 'fake', ...declaration };
    await writeFile(filename, JSON.stringify(body), { flag: 'wx' });
    const args = ['graphs', 'start', '--graph-file', filename];
    const envelope = run(args);
    assert.equal(envelope.data.graph.maxParallelism, '2', 'omitted parallelism retains the two-slot graph default');
    return { envelope, args, graph: envelope.data.graph };
  };
  const completed = id => poll('/graphs/' + id, data => data.graph.state === 'completed', 'Extracted graph completion');
  const snapshotDirectory = async snapshotId => {
    assert.match(snapshotId, /^[a-f0-9]{32}$/);
    const parent = await realpath(path.join(home, '.council', 'v2', 'workspaces'));
    const directory = path.join(parent, snapshotId);
    const physical = await realpath(directory);
    assert.equal(path.dirname(physical), parent, 'only this isolated native managed workspace is inspected');
    assert.equal((await lstat(directory)).isSymbolicLink(), false);
    return physical;
  };
  const release = async directory => {
    const target = path.join(directory, releaseFile);
    const temporaryFile = path.join(directory, '.council-package-release-' + randomBytes(16).toString('hex') + '.tmp');
    const file = await open(temporaryFile, 'wx', 0o600);
    try { await file.writeFile('release\n'); await file.sync(); }
    finally { await file.close(); }
    await rename(temporaryFile, target); // Closed, complete bytes become visible in one owned-directory step.
  };
  const successfulNative = async attemptId => {
    const attempt = await get('/attempts/' + attemptId);
    assert.equal(attempt.phase, 'settled');
    assert.equal(attempt.outcome, 'succeeded');
    assert.equal(attempt.executionMode, 'fake');
    assert.equal(attempt.executorKind, 'native');
    assert.equal(attempt.cleanup.state, 'verified');
    assert.equal(attempt.evidenceStatus, 'complete');
    assert.ok(attempt.startedAt && attempt.launchIntent, 'the fixed native child actually passed GO');
    return attempt;
  };

  const workingBefore = await readFile(path.join(repository, 'user.txt'));
  const indexBefore = await readFile(path.join(repository, '.git', 'index'));
  const stagedBefore = Buffer.from(execFileSync(git, ['--no-optional-locks', '-c', 'core.fsmonitor=false', '-C', repository, 'show', ':user.txt'], runtimeOptions));
  assert.notDeepEqual(workingBefore, stagedBefore, 'the actual source has distinct working and staged bytes');
  const unchangedUser = async () => {
    assert.deepEqual(await readFile(path.join(repository, 'user.txt')), workingBefore);
    assert.deepEqual(await readFile(path.join(repository, '.git', 'index')), indexBefore);
  };

  // Recover only the exact earlier fixture denial. Reports remain immutable
  // user declarations; this never measures provider/account capacity.
  assert.match(busyReportId, /^[a-f0-9]{32}$/);
  const capacity = run(['routing', 'capacity', '--context', 'codex-native']).data;
  assert.equal(capacity.dimensions.occupancy.effectiveState, 'busy');
  assert.deepEqual(capacity.dimensions.occupancy.blockingReports.map(report => report.reportId), [busyReportId],
    'only the earlier exact fixture occupancy report may be recovered');
  const available = run(['routing', 'report', '--context', 'codex-native', '--dimension', 'occupancy', '--state', 'available',
    '--expected-revision', capacity.revision, '--valid-for-seconds', '900',
    '--correction-of', busyReportId, '--correction-action', 'recover']).data;
  assert.equal(available.report.source, 'user_report');
  assert.equal(available.report.correctionOf, busyReportId);
  assert.equal(available.report.correctionAction, 'recover');
  assert.equal(available.capacity.dimensions.occupancy.effectiveState, 'available');
  assert.deepEqual(available.capacity.dimensions.occupancy.blockingReports, []);
  assert.equal(available.capacity.reportEligibility.permitted, true);

  const parallel = await start('parallel', {
    nodes: ['left', 'right'].map(key => ({ key, originalText: 'Inspect the complete fixed independent file result for ' + key + '.',
      fakeScenario: 'graph-wait', workflow: { review: 'required' }, acceptance: { deliverable: 'candidate' } })),
    dependencies: [], routing: { suitability: 'routine', budget: { admissionLimit: '4', windowSeconds: 900 } },
  });
  const overlapEnd = Date.now() + 25_000;
  let held;
  while (Date.now() < overlapEnd) {
    const detail = await get('/graphs/' + parallel.graph.id);
    const observed = [];
    for (const node of detail.nodes) {
      if (!node.latestAttemptId) continue;
      const attempt = await get('/attempts/' + node.latestAttemptId);
      if (attempt.phase === 'settled') throw new Error('An extracted overlap worker ended before both fixed barriers: ' + JSON.stringify(attempt));
      if (attempt.role !== 'author' || attempt.phase !== 'running' || !attempt.sourceSnapshotId) continue;
      const directory = await snapshotDirectory(attempt.sourceSnapshotId);
      try {
        const marker = await lstat(path.join(directory, readyFile));
        if (marker.isFile() && !marker.isSymbolicLink() && await readFile(path.join(directory, readyFile), 'utf8') === 'ready\n') observed.push({ attempt, directory });
      } catch (error) { if (error.code !== 'ENOENT') throw error; }
    }
    if (observed.length === 2) { held = observed; break; }
    await delay(15);
  }
  assert.equal(held?.length, 2, 'two actual fixed native children must hold ready markers simultaneously within their unchanged eight-second barriers');
  assert.equal(new Set(held.map(item => item.attempt.resolvedTarget.contextId)).size, 2);
  assert.equal(new Set(held.map(item => item.directory)).size, 2);
  for (const item of held) {
    assert.equal(item.attempt.executionMode, 'fake');
    assert.ok(item.attempt.startedAt && item.attempt.launchIntent);
    assert.equal(await readFile(path.join(item.directory, readyFile), 'utf8'), 'ready\n');
  }
  await Promise.all(held.map(item => release(item.directory)));
  const parallelDone = await completed(parallel.graph.id);
  assert.equal(parallelDone.graph.counts.completed, '2');
  assert.equal(parallelDone.budget.budget.consumedAdmissions, '4');
  const authors = await Promise.all(held.map(item => successfulNative(item.attempt.id)));
  assert.ok(Date.parse(authors[0].startedAt) < Date.parse(authors[1].settledAt) && Date.parse(authors[1].startedAt) < Date.parse(authors[0].settledAt),
    'actual native author lifetimes overlap, independently of declared capacity');
  for (const node of parallelDone.nodes) {
    assert.equal(node.state, 'completed');
    assert.equal(node.acceptedResult.acceptanceKind, 'workflow');
    const workflow = run(['tasks', 'workflow', node.taskId]).data;
    assert.equal(workflow.acceptance.deliveryResult, 'review_only');
    const steps = run(['tasks', 'steps', node.taskId]).data;
    assert.equal(steps.nextCursor, null);
    assert.deepEqual(steps.items.map(item => item.role), ['author', 'review']);
  }
  assert.deepEqual(run(parallel.args), parallel.envelope, 'known graph Start retry never creates another group or model debit');
  await unchangedUser();
  for (const filename of [fileA, fileB]) {
    await assert.rejects(lstat(path.join(repository, filename)), error => error.code === 'ENOENT', 'default separate graph outputs never apply to the user checkout');
  }

  const delivered = await start('cumulative-delivery', {
    nodes: [
      { key: 'a', fakeScenario: 'graph-branch-a', pins: { harnessId: 'codex' } },
      { key: 'b', fakeScenario: 'graph-branch-b', pins: { harnessId: 'claude-code' } },
      { key: 'final', fakeScenario: 'graph-join-no-edit', pins: { harnessId: 'codex' } },
    ].map(node => ({ ...node, originalText: 'Inspect the complete controlled file result for ' + node.key + ' while preserving original working bytes and staged index.',
      workflow: { review: 'required' }, acceptance: { deliverable: 'candidate' } })),
    dependencies: [{ from: 'a', to: 'final', kind: 'files' }, { from: 'b', to: 'final', kind: 'files' }], finalNodeKey: 'final',
    delivery: { localMode: 'apply_verified_local' }, routing: { budget: { admissionLimit: '6', windowSeconds: 900 } },
  });
  const done = await completed(delivered.graph.id);
  assert.equal(done.graph.counts.completed, '3');
  assert.equal(done.graph.counts.required, '3');
  assert.equal(done.budget.budget.consumedAdmissions, '6');
  assert.equal(done.budget.budget.remainingAdmissions, '0');
  await unchangedUser();
  assert.equal(await readFile(path.join(repository, fileA), 'utf8'), contentA);
  assert.equal(await readFile(path.join(repository, fileB), 'utf8'), contentB);
  const final = done.nodes.find(node => node.key === 'final');
  assert.ok(final?.inputId && final.acceptedResult?.candidateId);
  const finalWorkflow = run(['tasks', 'workflow', final.taskId]).data;
  assert.equal(finalWorkflow.workflow.state, 'completed');
  assert.equal(finalWorkflow.acceptance.deliveryResult, 'applied');
  const candidates = run(['tasks', 'candidates', final.taskId]).data.items;
  const candidate = candidates.find(item => item.id === final.acceptedResult.candidateId);
  assert.ok(candidate, 'the final acceptance selects an actual retained node candidate');
  assert.equal(candidate.hasDelta, false);
  assert.equal(candidate.hasIndexDelta, false);
  assert.equal(candidate.rootSourceSnapshotId, candidate.sourceSnapshotId);
  const dispatches = run(['graphs', 'dispatches', done.graph.id, '--node', 'final']).data;
  assert.equal(dispatches.nextCursor, null);
  const author = dispatches.items.find(item => item.dispatch.role === 'author');
  const review = dispatches.items.find(item => item.dispatch.role === 'review');
  const application = dispatches.items.find(item => item.dispatch.role === 'local_delivery');
  assert.ok(author && review && application);
  assert.equal(author.outputObservation.candidate.candidateId, candidate.id);
  assert.equal(author.outputObservation.candidate.state, 'complete');
  const finalRef = author.outputObservation.candidate.finalVerificationRef;
  assert.deepEqual(review.dispatch.sourceBasis.finalVerificationRef, finalRef);
  assert.deepEqual(application.dispatch.sourceBasis.finalVerificationRef, finalRef);
  const basis = jsonRef(finalRef);
  assert.equal(basis.finalCandidateBinding.candidateId, candidate.id);
  assert.equal(basis.hasCumulativeWorkingDelta, true);
  assert.equal(basis.hasCumulativeIndexDelta, false);
  assert.notEqual(candidate.rootSourceSnapshotId, basis.rootBinding.sourceSnapshotId, 'graph root never replaces the node candidate root');
  assert.equal(Object.hasOwn(basis.rootBinding, 'projectIdentity'), false);
  assert.equal(Object.hasOwn(basis.rootBinding, 'repositoryIdentity'), false);

  const input = run(['graphs', 'input', done.graph.id, '--input', final.inputId]).data;
  assert.equal(input.summary.state, 'ready');
  assert.equal(input.inputBinding.receivingNodeId, final.id);
  assert.equal(input.inputBinding.contributions.length, 2);
  assert.equal(input.inputBinding.contributorVector.length, 2);
  const receipt = author.materialization;
  assert.equal(receipt.state, 'complete');
  assert.equal(receipt.effectsKnown, true);
  assert.equal(receipt.evidenceStatus, 'complete');
  assert.equal(receipt.receivingAttemptId, author.dispatch.attemptId);
  assert.equal(receipt.sourceSnapshotId, candidate.sourceSnapshotId);
  assert.deepEqual(receipt.sourceBasis.expectedManifest, input.inputBinding.manifest);
  assert.deepEqual(receipt.sourceBasis.expectedIndex, input.inputBinding.index);
  assert.equal(receipt.observedWorkingEntriesSha256, receipt.sourceBasis.expectedWorkingEntriesSha256);
  assert.equal(receipt.observedIndexEntriesSha256, receipt.sourceBasis.expectedIndexEntriesSha256);
  const manifest = jsonRef(input.inputBinding.manifest);
  const userIndex = manifest.index.find(entry => entry.path === 'user.txt');
  assert.ok(userIndex, 'the retained input preserves the actual staged user entry');
  const staged = jsonRef(input.inputBinding.stagedBlobs).entries.find(entry => entry.objectId === userIndex.object);
  assert.ok(staged, 'the staged object has retained complete bytes independent of working files');
  assert.deepEqual(readRef(staged.artifactRef), stagedBefore);
  const receivingDirectory = await snapshotDirectory(candidate.sourceSnapshotId);
  assert.deepEqual(await readFile(path.join(receivingDirectory, 'user.txt')), workingBefore);
  assert.deepEqual(Buffer.from(execFileSync(git, ['--no-optional-locks', '-c', 'core.fsmonitor=false', '-C', receivingDirectory, 'show', ':user.txt'], runtimeOptions)), stagedBefore);
  const members = run(['graphs', 'input-members', done.graph.id, '--input', final.inputId, '--limit', '100']).data;
  assert.equal(members.nextCursor, null);
  for (const [filename, text, key] of [[fileA, contentA, 'a'], [fileB, contentB, 'b']]) {
    const predecessor = done.nodes.find(node => node.key === key);
    const member = members.items.find(item => item.dimension === 'working' && item.path === filename);
    assert.ok(member?.artifactRef && predecessor?.acceptedResult);
    assert.equal(member.origins.length, 1);
    assert.equal(member.origins[0].nodeId, predecessor.id);
    assert.equal(member.origins[0].acceptanceId, predecessor.acceptedResult.acceptanceId);
    assert.equal(member.origins[0].producerAttemptId, predecessor.acceptedResult.producerAttemptId);
    assert.equal(readRef(member.artifactRef).toString('utf8'), text);
    assert.equal(await readFile(path.join(receivingDirectory, filename), 'utf8'), text, 'the actual receiving workspace contains the complete accepted branch');
    const producer = run(['graphs', 'dispatches', done.graph.id, '--node', key]).data.items.find(item => item.dispatch.attemptId === predecessor.acceptedResult.producerAttemptId);
    const closure = jsonRef(producer.outputObservation.candidate.closureRef);
    assert.equal(member.origins[0].stateSha256, closure.closureSha256, 'member provenance binds the source-head seal, not an envelope/path hash');
  }
  const deliveries = run(['tasks', 'deliveries', final.taskId]).data.items;
  const operations = deliveries.filter(item => item.kind === 'local_application');
  assert.equal(operations.length, 1);
  const operation = operations[0].operation;
  assert.equal(operation.status, 'applied');
  assert.equal(operation.attemptId, application.dispatch.attemptId);
  assert.equal(operation.lastEffectSequence, '4');
  assert.ok(operation.beforeDestination && operation.afterDestination && operation.graphDeliveryRef);
  const binding = jsonRef(operation.graphDeliveryRef);
  assert.equal(binding.finalCandidateBinding.candidateId, candidate.id);
  assert.deepEqual(binding.finalVerificationRef, finalRef);
  assert.equal(binding.otherNodeAcceptances.length, 2);
  assert.ok(binding.preDeliveryEvidenceUses.some(use => use.kind === 'review'));
  assert.equal(Object.hasOwn(binding.rootBinding, 'projectIdentity'), false);
  assert.equal(Object.hasOwn(binding.rootBinding, 'repositoryIdentity'), false);
  assert.deepEqual(run(delivered.args), delivered.envelope, 'known successful graph Start replay retains the same captured request/result');
  assert.equal(run(['graphs', 'budget', done.graph.id]).data.budget.consumedAdmissions, '6');
}
