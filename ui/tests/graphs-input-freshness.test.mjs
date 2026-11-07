import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import path from 'node:path';
import test from 'node:test';
import { fixture, fixtureRequestId, openWorkspace, ownerAPI, pollOwner, reportFixture } from './helpers.mjs';

async function read(owner, route) {
  const result = await ownerAPI(owner, route);
  assert.equal(result.status, 200, JSON.stringify(result.envelope.error));
  return result.envelope.data;
}

function attemptHistorySnapshot(page) {
  // Store.observe derives only these two top-level fields at read time, even
  // for settled attempts. Retain the full page, cursor and every other field.
  return { ...page, items: page.items.map(attempt => {
    const retained = { ...attempt };
    delete retained.observedAt;
    delete retained.remainingSeconds;
    return retained;
  }) };
}

test('an open exact input inspector stops claiming current composition after same-graph guidance makes it historical', { timeout: 90000 }, async t => {
  const f = await fixture(t), owner = await f.start();
  const opened = await ownerAPI(owner, '/projects/open', 'POST', {
    requestId: fixtureRequestId(), selectedPath: f.project, basePath: f.project,
  });
  assert.equal(opened.status, 201);
  // Fixed local reports prevent native admission; this case exercises real
  // retained metadata and browser reads, without a provider or worker model.
  for (const context of ['codex-native', 'claude-native']) {
    assert.equal((await reportFixture(owner, context, 'occupancy', 'busy')).status, 201);
  }
  const original = '\ufeffComplete original stays historical 東京🙂\r\nKeep the original working bytes and index intact.';
  const created = await ownerAPI(owner, '/graphs', 'POST', {
    requestId: fixtureRequestId(), projectId: opened.envelope.data.project.id, executionMode: 'fake',
    nodes: [{ key: 'receiver', originalText: original, fakeScenario: 'graph-response', acceptance: { deliverable: 'response_only' } }],
  });
  assert.equal(created.status, 201, JSON.stringify(created.envelope.error));
  const graphId = created.envelope.data.graph.id;
  const prepared = await pollOwner(owner, '/graphs/' + graphId,
    data => data.nodes[0].inputId !== null && data.activeClaims.length === 0);
  const node = prepared.nodes[0], inputId = node.inputId;
  const oldInput = await pollOwner(owner, '/graphs/' + graphId + '/inputs/' + inputId,
    data => data.summary.state === 'ready' && data.summary.current === true);
  assert.equal(prepared.budget.budget.consumedAdmissions, '0');
  assert.deepEqual((await read(owner, '/tasks/' + node.taskId + '/attempts')).items, []);

  const page = await openWorkspace(f, owner, { fake: false, workflow: 'auto' });
  await page.locator('#open-graph-list').click({ timeout: 5000 });
  await page.locator('#graph-list button').filter({ hasText: graphId }).click({ timeout: 5000 });
  const writes = [];
  page.on('request', request => {
    if (!['GET', 'HEAD'].includes(request.method())) writes.push({ method: request.method(), path: new URL(request.url()).pathname });
  });
  await page.getByRole('button', { name: 'Inputs and context receiver', exact: true }).click({ timeout: 5000 });
  await page.waitForFunction(id => document.getElementById('graph-input-facts')?.textContent.includes(id), inputId, { timeout: 5000 });
  assert.match(await page.locator('#graph-input-facts').textContent(), /Current retained input/);
  await page.getByRole('button', { name: 'Read complete receiver original', exact: true }).click({ timeout: 5000 });
  await page.waitForFunction(text => document.getElementById('graph-input-artifact')?.textContent === text, original, { timeout: 5000 });
  assert.equal(writes.length, 0, 'initial complete input inspection must remain read-only');

  const guidancePath = '/api/v1/graphs/' + graphId + '/nodes/receiver/guidance';
  const reply = page.waitForResponse(response => response.request().method() === 'POST' && new URL(response.url()).pathname === guidancePath);
  await page.locator('#graph-guidance-node').selectOption(node.id);
  await page.locator('#graph-guidance-text').fill('Complete V2 guidance π\r\nKeep old evidence selected until an explicit new selection.');
  await page.locator('#graph-save-guidance').click({ timeout: 5000 });
  const response = await reply, acknowledgement = await response.json();
  assert.equal(response.status(), 200, JSON.stringify(acknowledgement.error));
  assert.equal(acknowledgement.data.node.taskVersion, '2');
  const guided = await read(owner, '/graphs/' + graphId);
  assert.equal(guided.nodes[0].taskVersion, '2');
  assert.equal(guided.nodes[0].inputId, null);
  const retained = await read(owner, '/graphs/' + graphId + '/inputs/' + inputId);
  assert.equal(retained.summary.current, false, 'the actual owner, rather than a mock response, marks this exact old input historical');
  assert.deepEqual(retained.inputBinding, oldInput.inputBinding);

  // Read the real current graph while keeping the old input and byte selection.
  // A refresh must neither silently retarget evidence nor claim this old input
  // is still the current composition beside the now-visible task version 2.
  await page.locator('#graph-refresh').click({ timeout: 5000 });
  await page.waitForFunction(() => document.querySelector('#graph-node-list [data-node-key="receiver"]')?.textContent.includes('Current version 2'), undefined, { timeout: 5000 });
  assert.equal(await page.locator('#graph-input-selector').inputValue(), inputId);
  assert.equal(await page.locator('#graph-input-artifact').textContent(), original);
  assert.deepEqual((await read(owner, '/graphs/' + graphId)).budget, prepared.budget, 'guidance and inspection cannot grant, debit or renew shared authority');
  assert.deepEqual((await read(owner, '/tasks/' + node.taskId + '/attempts')).items, [], 'passive reads and guidance cannot launch a native stage');
  assert.deepEqual(writes, [{ method: 'POST', path: guidancePath }], 'the explicit guidance is the only write; inspection and graph refresh stay read-only');
  assert.equal(await readFile(path.join(f.project, 'hello.txt'), 'utf8'), 'unstaged\n');
  assert.deepEqual(await readFile(path.join(f.project, '.git/index')), f.indexBefore);

  const visible = await page.locator('#graph-input-facts').evaluate(facts => {
    const scope = [...facts.querySelectorAll('dt')].find(term => term.textContent === 'Scope')?.nextElementSibling?.textContent ?? '';
    return { scope, status: document.getElementById('graph-input-status')?.textContent ?? '', selected: document.getElementById('graph-input-selector')?.selectedOptions[0]?.textContent ?? '' };
  });
  // A clearly dated read snapshot is legitimate. An undated bare "current"
  // claim after the current graph disproves it is the regression under test.
  const datedSnapshot = /(?:as of|observed at|read at)\s+\d{4}-\d{2}-\d{2}/i.test(visible.scope + ' ' + visible.status);
  assert.ok(datedSnapshot || visible.scope !== 'Current retained input',
    'the actual historical input is still presented as an undated current input: ' + JSON.stringify(visible));
  assert.ok(datedSnapshot || !visible.status.includes('Current composition.'),
    'the retained historical evidence is still labelled as the current composition: ' + JSON.stringify(visible));
  assert.match(visible.scope + ' ' + visible.status, /historical|stale|superseded|previously observed|as of|observed at|read at/i,
    'the retained input must be distinguished from the current composition without losing its exact evidence');
  assert.ok(datedSnapshot || !visible.selected.includes(' · current · '),
    'the selected historical input option still makes an undated current claim: ' + JSON.stringify(visible));
});

test('an open descendant input inspector becomes historical when ancestor guidance stales the same receiver version and input', { timeout: 90000 }, async t => {
  const f = await fixture(t), owner = await f.start();
  const opened = await ownerAPI(owner, '/projects/open', 'POST', {
    requestId: fixtureRequestId(), selectedPath: f.project, basePath: f.project,
  });
  assert.equal(opened.status, 201);
  const sourceOriginal = 'Complete source original π\r\nAuthor the fixed branch A file, inspect the whole candidate independently, and preserve user files and staged intent.';
  const receiverOriginal = '\ufeffComplete receiver original 東京🙂\r\nRead the accepted source context, author the fixed branch B file, and preserve user files and staged intent.';
  const created = await ownerAPI(owner, '/graphs', 'POST', {
    requestId: fixtureRequestId(), projectId: opened.envelope.data.project.id, executionMode: 'fake',
    nodes: [
      { key: 'source', originalText: sourceOriginal, fakeScenario: 'graph-branch-a', pins: { harnessId: 'codex' },
        acceptance: { deliverable: 'candidate' }, workflow: { review: 'required' } },
      { key: 'receiver', originalText: receiverOriginal, fakeScenario: 'graph-branch-b', pins: { harnessId: 'claude-code' },
        acceptance: { deliverable: 'candidate' }, workflow: { review: 'required' } },
    ],
    dependencies: [{ from: 'source', to: 'receiver', kind: 'context' }],
    routing: { budget: { admissionLimit: '4', windowSeconds: 900 } },
  });
  assert.equal(created.status, 201, JSON.stringify(created.envelope.error));
  const graphId = created.envelope.data.graph.id;
  // Candidate review cannot be disabled by a fictional "none" mode. Both
  // nodes complete their real author/review workflow before changing guidance.
  const completed = await pollOwner(owner, '/graphs/' + graphId,
    data => data.graph.state === 'completed' && data.activeClaims.length === 0, { timeoutMs: 60000 });
  assert.equal(completed.nodes.length, 2);
  assert.ok(completed.nodes.every(node => node.state === 'completed' && node.acceptedResult?.acceptanceKind === 'workflow' && node.acceptedResult.candidateId));
  assert.equal(completed.budget.budget.consumedAdmissions, '4');
  const source = completed.nodes.find(node => node.key === 'source'), receiver = completed.nodes.find(node => node.key === 'receiver');
  const receiverTuple = node => ({ taskVersion: node.taskVersion, inputId: node.inputId, nodeAdmissionId: node.nodeAdmissionId, latestAttemptId: node.latestAttemptId });
  const originalTuple = receiverTuple(receiver), inputId = receiver.inputId;
  assert.equal(originalTuple.taskVersion, '1');
  assert.ok(inputId && originalTuple.nodeAdmissionId && originalTuple.latestAttemptId);
  const oldInput = await read(owner, '/graphs/' + graphId + '/inputs/' + inputId);
  assert.equal(oldInput.summary.current, true);
  assert.equal(oldInput.inputBinding.contributions.length, 1);
  assert.equal(oldInput.inputBinding.contributions[0].fromNodeId, source.id);
  assert.equal(oldInput.inputBinding.contributions[0].edgeKind, 'context');
  const beforeAttempts = new Map();
  for (const node of completed.nodes) {
    const attempts = await read(owner, '/tasks/' + node.taskId + '/attempts');
    assert.equal(attempts.items.length, 2);
    assert.ok(attempts.items.every(attempt => attempt.phase === 'settled' && attempt.outcome === 'succeeded'
      && attempt.executorKind === 'native' && attempt.cleanup.state === 'verified'));
    assert.deepEqual(attempts.items.map(attempt => attempt.role).sort(), ['author', 'review']);
    const workflow = await read(owner, '/tasks/' + node.taskId + '/workflow');
    assert.equal(workflow.workflow.state, 'completed');
    assert.equal(workflow.acceptance.deliveryResult, 'review_only');
    assert.equal(workflow.acceptance.deliveryOperationId, null);
    beforeAttempts.set(node.taskId, attemptHistorySnapshot(attempts));
  }

  const page = await openWorkspace(f, owner, { fake: false, workflow: 'auto' });
  await page.locator('#open-graph-list').click({ timeout: 5000 });
  await page.locator('#graph-list button').filter({ hasText: graphId }).click({ timeout: 5000 });
  const writes = [];
  page.on('request', request => {
    if (!['GET', 'HEAD'].includes(request.method())) writes.push({ method: request.method(), path: new URL(request.url()).pathname });
  });
  await page.getByRole('button', { name: 'Inputs and context receiver', exact: true }).click({ timeout: 5000 });
  await page.waitForFunction(id => document.getElementById('graph-input-facts')?.textContent.includes(id), inputId, { timeout: 5000 });
  assert.match(await page.locator('#graph-input-facts').textContent(), /Current retained input/);
  await page.getByRole('button', { name: 'Read complete receiver original', exact: true }).click({ timeout: 5000 });
  await page.waitForFunction(text => document.getElementById('graph-input-artifact')?.textContent === text, receiverOriginal, { timeout: 5000 });
  assert.equal(writes.length, 0);

  const guidancePath = '/api/v1/graphs/' + graphId + '/nodes/source/guidance';
  const reply = page.waitForResponse(response => response.request().method() === 'POST' && new URL(response.url()).pathname === guidancePath);
  await page.locator('#graph-guidance-node').selectOption(source.id);
  await page.locator('#graph-guidance-text').fill('Complete changed ancestor guidance 東京\r\nKeep the old result as history; no Advance or automatic replacement is requested.');
  await page.locator('#graph-save-guidance').click({ timeout: 5000 });
  const response = await reply, acknowledgement = await response.json();
  assert.equal(response.status(), 200, JSON.stringify(acknowledgement.error));
  assert.equal(acknowledgement.data.node.taskVersion, '2');
  const guided = await read(owner, '/graphs/' + graphId), sameReceiver = guided.nodes.find(node => node.id === receiver.id);
  assert.equal(guided.nodes.find(node => node.id === source.id).taskVersion, '2');
  assert.deepEqual(receiverTuple(sameReceiver), originalTuple, 'ancestor guidance must stale the unchanged receiver input/version rather than replace its identity');
  const stale = guided.blockers.find(blocker => blocker.nodeId === receiver.id);
  assert.equal(stale?.code, 'graph_input_stale');
  assert.equal(stale.inputId, inputId);
  const retained = await read(owner, '/graphs/' + graphId + '/inputs/' + inputId);
  assert.equal(retained.summary.current, false);
  assert.deepEqual(retained.inputBinding, oldInput.inputBinding);

  await page.locator('#graph-refresh').click({ timeout: 5000 });
  await page.waitForFunction(() => document.querySelector('#graph-node-list [data-node-key="source"]')?.textContent.includes('Current version 2'), undefined, { timeout: 5000 });
  assert.equal(await page.locator('#graph-input-selector').inputValue(), inputId);
  assert.equal(await page.locator('#graph-input-artifact').textContent(), receiverOriginal);
  assert.deepEqual((await read(owner, '/graphs/' + graphId)).budget, completed.budget, 'ancestor guidance and inspection cannot debit, grant or renew shared authority');
  for (const node of completed.nodes) assert.deepEqual(attemptHistorySnapshot(await read(owner, '/tasks/' + node.taskId + '/attempts')), beforeAttempts.get(node.taskId),
    'ancestor guidance and observation must preserve both real completed author/review histories');
  assert.deepEqual(writes, [{ method: 'POST', path: guidancePath }], 'guidance is the only write; no Advance, execution or inspection mutation is permitted');
  assert.equal(await readFile(path.join(f.project, 'hello.txt'), 'utf8'), 'unstaged\n');
  assert.deepEqual(await readFile(path.join(f.project, '.git/index')), f.indexBefore);

  const visible = await page.locator('#graph-input-facts').evaluate(facts => {
    const scope = [...facts.querySelectorAll('dt')].find(term => term.textContent === 'Scope')?.nextElementSibling?.textContent ?? '';
    return { scope, status: document.getElementById('graph-input-status')?.textContent ?? '', selected: document.getElementById('graph-input-selector')?.selectedOptions[0]?.textContent ?? '' };
  });
  const datedSnapshot = /(?:as of|observed at|read at)\s+\d{4}-\d{2}-\d{2}/i.test(visible.scope + ' ' + visible.status);
  assert.ok(datedSnapshot || visible.scope !== 'Current retained input',
    'an unchanged descendant tuple is still falsely presented as current despite its exact stale-input blocker: ' + JSON.stringify(visible));
  assert.ok(datedSnapshot || !visible.status.includes('Current composition.'), 'the ancestor-stale input still claims current composition: ' + JSON.stringify(visible));
  assert.match(visible.scope + ' ' + visible.status, /historical|stale|superseded|previously observed|as of|observed at|read at/i);
  assert.ok(datedSnapshot || !visible.selected.includes(' · current · '), 'the ancestor-stale selected input option still makes an undated current claim: ' + JSON.stringify(visible));
});
