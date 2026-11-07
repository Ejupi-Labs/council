import assert from 'node:assert/strict';
import { lstat, readFile, realpath, writeFile } from 'node:fs/promises';
import path from 'node:path';
import test from 'node:test';
import { fixture, fixtureRequestId, openWorkspace, ownerAPI, pollOwner, reportFixture, responseBarrier } from './helpers.mjs';

async function verifyNarrowKeyboardFocus(page, control, name) {
  await page.setViewportSize({ width: 360, height: 800 });
  await control.scrollIntoViewIfNeeded();
  await control.focus(); await page.keyboard.press('Tab'); await page.keyboard.press('Shift+Tab');
  const observed = await control.evaluate(button => {
    const style = getComputedStyle(button), rect = button.getBoundingClientRect();
    return { focused: document.activeElement === button, visibleFocus: style.outlineStyle !== 'none' && parseFloat(style.outlineWidth) > 0, width: rect.width, viewport: innerWidth, pageWidth: document.documentElement.scrollWidth };
  });
  assert.equal(observed.focused, true, name + ' must be reachable by keyboard');
  assert.equal(observed.visibleFocus, true, name + ' must have visible keyboard focus');
  assert.ok(observed.width <= observed.viewport && observed.pageWidth <= observed.viewport + 1, name + ' and page must fit the narrow viewport');
}

test('group pause retries the exact saved request after selection changes and resumes a verified checkpoint', { timeout: 120000 }, async t => {
  const barrier = responseBarrier(), f = await fixture();
  t.after(async () => { barrier.release(); await f.close(); });
  const owner = await f.start();
  const opened = await ownerAPI(owner, '/projects/open', 'POST', { requestId: fixtureRequestId(), selectedPath: f.project, basePath: f.project });
  assert.equal(opened.status, 201);
  const projectId = opened.envelope.data.project.id;
  for (const context of ['codex-native', 'claude-native']) assert.equal((await reportFixture(owner, context, 'occupancy', 'busy')).status, 201);
  const groups = [];
  for (const title of ['Recovery A', 'Recovery B']) {
    const started = await ownerAPI(owner, '/graphs', 'POST', { requestId: fixtureRequestId(), projectId, title, executionMode: 'fake', nodes: [{ key: 'one', originalText: 'A controlled task.', fakeScenario: 'graph-response' }, { key: 'two', originalText: 'Another controlled task.', fakeScenario: 'graph-response' }] });
    assert.equal(started.status, 201, JSON.stringify(started.envelope.error));
    const graphId = started.envelope.data.graph.id;
    await pollOwner(owner, '/graphs/' + graphId + '/inputs', data => data.items.length === 3 && data.items.every(item => item.state === 'ready'), { timeoutMs: 30000 });
    groups.push((await ownerAPI(owner, '/graphs/' + graphId)).envelope.data);
  }
  const [a, b] = groups, page = await openWorkspace(f, owner, { fake: false, workflow: 'auto' });
  await page.locator('#open-graph-list').click();
  const select = async graph => {
    await page.locator('#graph-list button').filter({ hasText: graph.graph.id }).click();
    await page.waitForFunction(id => document.getElementById('graph-facts')?.textContent.includes(id), graph.graph.id);
  };
  await select(a);
  const pauseButton = page.getByRole('button', { name: 'Pause group', exact: true });
  await verifyNarrowKeyboardFocus(page, pauseButton, 'Pause group');
  const pausePath = '/api/v1/graphs/' + a.graph.id + '/pause', sent = []; let first = true, saved;
  await page.route('**' + pausePath, async route => {
    sent.push(route.request().postDataJSON());
    const response = await route.fetch(), envelope = await response.json();
    if (first) { first = false; saved = structuredClone(envelope); await barrier.hold(); await route.abort('failed'); }
    else {
      // The saved receipt is real. Reorder only object properties on its actual
      // replay to exercise semantically identical JSON produced by map encoding.
      envelope.data.checkpoint.scope.nodeAuthorities = envelope.data.checkpoint.scope.nodeAuthorities.map(item => ({ nodeControlRevision: item.nodeControlRevision, taskVersion: item.taskVersion, nodeId: item.nodeId }));
      await route.fulfill({ response, json: envelope });
    }
  });
  await page.getByRole('button', { name: 'Pause group', exact: true }).click();
  let timer;
  try { await Promise.race([barrier.arrived, new Promise((_, reject) => { timer = setTimeout(() => reject(new Error('Pause acknowledgement was not reached')), 10000); })]); }
  finally { clearTimeout(timer); }
  assert.equal(saved.data.checkpoint.scope.graphId, a.graph.id);
  for (const field of ['graphVersion', 'expectedGraphControlRevision', 'expectedBudgetAuthorityEpoch', 'expectedWindowNumber']) assert.equal(saved.data.checkpoint.scope[field], sent[0][field], 'saved checkpoint must retain captured ' + field);
  assert.equal(JSON.stringify(saved.data.checkpoint.scope.nodeAuthorities), JSON.stringify(sent[0].nodeAuthorities), 'saved checkpoint must retain exact node authority values');
  await select(b);
  const statusB = await page.locator('#group-recovery [role=status]').textContent();
  barrier.release();
  const pending = page.locator('#group-recovery li').filter({ hasText: sent[0].requestId });
  await pending.getByRole('button', { name: 'Retry exact graphs.pause request' }).waitFor();
  assert.equal(await page.locator('#group-recovery [role=status]').textContent(), statusB, 'late A result must not replace B status');
  const replay = page.waitForResponse(response => response.request().method() === 'POST' && new URL(response.url()).pathname === pausePath && response.request().postDataJSON().requestId === sent[0].requestId);
  await pending.getByRole('button', { name: 'Retry exact graphs.pause request' }).click();
  const replayed = await (await replay).json();
  assert.deepEqual(replayed, saved);
  assert.notEqual(JSON.stringify(replayed.data.checkpoint.scope.nodeAuthorities), JSON.stringify(saved.data.checkpoint.scope.nodeAuthorities), 'the replay must exercise equivalent object properties in a different order');
  assert.deepEqual(sent[1], sent[0], 'retry must keep the complete authority vector');
  try { await pending.waitFor({ state: 'detached' }); }
  catch (error) {
    const diagnostic = await page.evaluate(() => {
      let pending = [];
      try { pending = JSON.parse(sessionStorage.getItem('council.foundation.recoveryPendingControls') || '[]').map(({ requestId, op, target, unresolved, body }) => ({ requestId, op, target, unresolved, graphVersion: body?.graphVersion, graphControlRevision: body?.expectedGraphControlRevision, budgetEpoch: body?.expectedBudgetAuthorityEpoch, windowNumber: body?.expectedWindowNumber, nodeAuthorities: body?.nodeAuthorities })); } catch { /* Report an unreadable session as an empty diagnostic list. */ }
      return { status: document.querySelector('#group-recovery [role=status]')?.textContent, pending };
    });
    throw new Error('The exact saved pause acknowledgement did not clear its pending row: ' + JSON.stringify(diagnostic), { cause: error });
  }
  await select(a);
  const paused = await pollOwner(owner, '/checkpoints/' + saved.data.checkpoint.id, data => data.state === 'paused');
  const beforeCLI = (await ownerAPI(owner, '/graphs/' + a.graph.id)).envelope.data;
  const authoritiesFile = path.join(f.home, 'controlled-pause-authorities.json');
  await writeFile(authoritiesFile, JSON.stringify(sent[0].nodeAuthorities));
  const exactCLI = f.cli(['graphs', 'pause', a.graph.id, '--graph-version', sent[0].graphVersion, '--graph-control-revision', sent[0].expectedGraphControlRevision,
    '--budget-authority-epoch', sent[0].expectedBudgetAuthorityEpoch, '--window-number', sent[0].expectedWindowNumber,
    '--node-authorities-file', authoritiesFile, '--request-id', sent[0].requestId]);
  assert.equal(exactCLI.status, 0, JSON.stringify(exactCLI.envelope.error));
  assert.deepEqual(exactCLI.envelope, saved, 'CLI must replay the same UI pause receipt and captured checkpoint');
  const cliCheckpoint = f.cli(['checkpoints', 'get', paused.id]);
  assert.equal(cliCheckpoint.status, 0, JSON.stringify(cliCheckpoint.envelope.error));
  assert.equal(cliCheckpoint.envelope.data.id, paused.id);
  assert.equal(cliCheckpoint.envelope.data.revision, paused.revision);
  const afterCLI = (await ownerAPI(owner, '/graphs/' + a.graph.id)).envelope.data;
  assert.equal(afterCLI.budget.budget.consumedAdmissions, beforeCLI.budget.budget.consumedAdmissions, 'CLI replay and observation must not admit model work');
  assert.deepEqual(afterCLI.nodes.map(node => node.latestAttemptId), beforeCLI.nodes.map(node => node.latestAttemptId));
  await page.locator('#graph-refresh').click();
  const resume = page.getByRole('button', { name: 'Resume checkpoint ' + paused.id.slice(0, 8), exact: true });
  await resume.waitFor();
  await verifyNarrowKeyboardFocus(page, resume, 'Resume checkpoint');
  const response = page.waitForResponse(r => r.request().method() === 'POST' && new URL(r.url()).pathname === '/api/v1/checkpoints/' + paused.id + '/resume');
  await resume.click();
  const resumed = await response;
  assert.equal(resumed.status(), 200, JSON.stringify((await resumed.json()).error));
  assert.equal((await resumed.json()).data.checkpoint.id, paused.id);
  assert.equal((await resumed.json()).data.checkpoint.state, 'resumed');
  await page.locator('#graph-refresh').click();
  const memberPause = page.waitForResponse(r => r.request().method() === 'POST' && new URL(r.url()).pathname === '/api/v1/tasks/' + a.nodes[0].taskId + '/pause');
  await page.getByRole('button', { name: 'Pause member one', exact: true }).click();
  const memberResponse = await memberPause, member = await memberResponse.json();
  assert.equal(memberResponse.status(), 202, JSON.stringify(member.error));
  assert.equal(member.data.checkpoint.scope.kind, 'member');
  assert.equal(member.data.checkpoint.scope.nodeId, a.nodes[0].id);
  assert.equal(member.data.checkpoint.scope.graphId, a.graph.id);
});

test('task recovery preserves the current draft and posts its captured version', { timeout: 90000 }, async t => {
  const f = await fixture(); t.after(() => f.close());
  const owner = await f.start(), page = await openWorkspace(f, owner);
  await page.locator('#task-text').fill('Controlled recovery task.');
  await page.locator('#start-task').click();
  await page.locator('#task-detail').waitFor({ state: 'visible' });
  await page.locator('#task-text').fill('A newer independent draft.');
  const pause = page.waitForResponse(r => r.request().method() === 'POST' && /\/api\/v1\/tasks\/[a-f0-9]{32}\/pause$/.test(new URL(r.url()).pathname));
  await page.getByRole('button', { name: 'Pause task', exact: true }).click();
  const response = await pause, result = await response.json();
  assert.equal(response.status(), 202, JSON.stringify(result.error));
  assert.equal(result.data.checkpoint.scope.taskId, new URL(response.url()).pathname.split('/')[4]);
  assert.equal(response.request().postDataJSON().taskVersion, result.data.checkpoint.scope.taskVersion);
  assert.equal(await page.locator('#task-text').inputValue(), 'A newer independent draft.');
});

test('a real retained cleanup incident is inspected and reconciled only with owner evidence', { timeout: 90000 }, async t => {
  const f = await fixture(); t.after(() => f.close());
  const owner = await f.start(), page = await openWorkspace(f, owner);
  await page.locator('#task-fake-scenario').selectOption('cleanup_unknown');
  await page.locator('#task-text').fill('Inspect a controlled cleanup incident.');
  await page.locator('#start-task').click();
  await page.locator('#attempt-facts').filter({ hasText: 'Retained incident' }).waitFor({ state: 'attached' });
  const inspect = page.getByRole('button', { name: 'Inspect recovery incident', exact: true });
  await inspect.waitFor();
  const incidentResponse = page.waitForResponse(r => r.request().method() === 'GET' && /\/api\/v1\/incidents\/[a-f0-9]{32}$/.test(new URL(r.url()).pathname));
  await inspect.click();
  const incident = await (await incidentResponse).json();
  assert.ok(incident.data.revision, 'the owner must supply the incident revision');
  const reconcile = page.getByRole('button', { name: 'Reconcile incident ' + incident.data.incident.id.slice(0, 8), exact: true });
  await reconcile.waitFor();
  await verifyNarrowKeyboardFocus(page, reconcile, 'Reconcile incident');
  const resultResponse = page.waitForResponse(r => r.request().method() === 'POST' && new URL(r.url()).pathname === '/api/v1/incidents/' + incident.data.incident.id + '/reconciliations');
  await reconcile.click();
  const response = await resultResponse, result = await response.json();
  assert.equal(response.request().postDataJSON().expectedIncidentRevision, incident.data.revision);
  assert.equal(response.request().postDataJSON().action, 'verify_and_reconcile');
  assert.equal(response.status(), 200, JSON.stringify(result.error));
  assert.equal(result.data.incident.incident.id, incident.data.incident.id);
  assert.ok(['unresolved', 'reconciled'].includes(result.data.decision.outcome));
  await page.locator('#task-recovery li[data-recovery-request-id="' + response.request().postDataJSON().requestId + '"]').waitFor({ state: 'detached' });

  // Inject only the read projection to exercise the reconciled-but-fenced UI.
  // Physical reconciliation is proved separately by the engine tests; this
  // browser case verifies the visible action and the real request it sends.
  const incidentPath = '/api/v1/incidents/' + incident.data.incident.id;
  await page.route('**' + incidentPath, async route => {
    const actual = await route.fetch(), projection = await actual.json();
    projection.data.state = 'reconciled';
    projection.data.blockers = ['Controlled retained shared fence.'];
    projection.data.nextAction = 'Resolve remaining blockers, then explicitly recheck this incident.';
    await route.fulfill({ response: actual, json: projection });
  });
  const projectionResponse = page.waitForResponse(r => r.request().method() === 'GET' && new URL(r.url()).pathname === incidentPath);
  await page.getByRole('button', { name: 'Inspect recovery incident', exact: true }).click();
  const projection = await (await projectionResponse).json();
  const recheck = page.getByRole('button', { name: 'Recheck remaining fence', exact: true });
  await recheck.waitFor();
  const recheckResponse = page.waitForResponse(r => r.request().method() === 'POST' && new URL(r.url()).pathname === incidentPath + '/reconciliations');
  await recheck.click();
  const rechecked = await recheckResponse;
  assert.equal(rechecked.request().postDataJSON().expectedIncidentRevision, projection.data.revision);
  assert.notEqual(rechecked.request().postDataJSON().requestId, response.request().postDataJSON().requestId, 'recheck is a new explicit operation');
});

test('a global owner incident remains inspectable without selecting a task after reload', { timeout: 90000 }, async t => {
  const f = await fixture(); t.after(() => f.close());
  const owner = await f.start(), page = await openWorkspace(f, owner);
  await page.locator('#task-fake-scenario').selectOption('cleanup_unknown');
  await page.locator('#task-text').fill('Retain an owner-wide cleanup incident.');
  await page.locator('#start-task').click();
  await page.locator('#attempt-facts').filter({ hasText: 'Retained incident' }).waitFor({ state: 'attached' });
  await page.reload();
  const global = page.locator('#global-recovery');
  const inspect = global.getByRole('button', { name: 'Inspect global recovery incident', exact: true });
  await inspect.waitFor();
  assert.equal(await page.locator('#task-detail').isVisible(), false, 'global inspection must not depend on a selected task');
  const get = page.waitForResponse(r => r.request().method() === 'GET' && /\/api\/v1\/incidents\/[a-f0-9]{32}$/.test(new URL(r.url()).pathname));
  await inspect.click();
  const observed = await (await get).json();
  assert.ok(observed.data.revision);
  const reconcile = global.getByRole('button', { name: 'Reconcile incident ' + observed.data.incident.id.slice(0, 8), exact: true });
  await reconcile.waitFor();
  const posted = page.waitForResponse(r => r.request().method() === 'POST' && new URL(r.url()).pathname === '/api/v1/incidents/' + observed.data.incident.id + '/reconciliations');
  await reconcile.click();
  const result = await posted, body = await result.json();
  assert.equal(result.status(), 200, JSON.stringify(body.error));
  assert.equal(result.request().postDataJSON().expectedIncidentRevision, observed.data.revision);
  assert.equal(body.data.incident.incident.id, observed.data.incident.id);
});

test('a paused member resumes while an unpaused sibling continues without replay', { timeout: 120000 }, async t => {
  const f = await fixture(); t.after(() => f.close());
  const owner = await f.start();
  const project = await ownerAPI(owner, '/projects/open', 'POST', { requestId: fixtureRequestId(), selectedPath: f.project, basePath: f.project });
  assert.equal(project.status, 201);
  let codexBusy;
  for (const context of ['codex-native', 'claude-native']) {
    const report = await reportFixture(owner, context, 'occupancy', 'busy');
    assert.equal(report.status, 201);
    if (context === 'codex-native') codexBusy = report.envelope.data.report.id;
  }
  const created = await ownerAPI(owner, '/graphs', 'POST', { requestId: fixtureRequestId(), projectId: project.envelope.data.project.id, executionMode: 'fake', nodes: [
    { key: 'paused', originalText: 'Keep this member paused.', fakeScenario: 'graph-response' },
    { key: 'sibling', originalText: 'Run independently and wait for the controlled release.', fakeScenario: 'graph-wait' },
  ] });
  assert.equal(created.status, 201, JSON.stringify(created.envelope.error));
  const graphId = created.envelope.data.graph.id;
  await pollOwner(owner, '/graphs/' + graphId + '/inputs', data => data.items.length === 3 && data.items.every(item => item.state === 'ready'));
  const original = (await ownerAPI(owner, '/graphs/' + graphId)).envelope.data;
  const member = original.nodes.find(node => node.key === 'paused'), sibling = original.nodes.find(node => node.key === 'sibling');
  const page = await openWorkspace(f, owner, { fake: false, workflow: 'auto' });
  await page.locator('#open-graph-list').click();
  await page.locator('#graph-list button').filter({ hasText: graphId }).click();
  await page.waitForFunction(id => document.getElementById('graph-facts')?.textContent.includes(id), graphId);
  const pauseResponse = page.waitForResponse(r => r.request().method() === 'POST' && new URL(r.url()).pathname === '/api/v1/tasks/' + member.taskId + '/pause');
  await page.getByRole('button', { name: 'Pause member paused', exact: true }).click();
  const pause = await pauseResponse, pauseBody = await pause.json();
  assert.equal(pause.status(), 202, JSON.stringify(pauseBody.error));
  assert.equal(pauseBody.data.checkpoint.scope.nodeId, member.id);
  const checkpoint = await pollOwner(owner, '/checkpoints/' + pauseBody.data.checkpoint.id, data => data.state === 'paused');
  assert.equal((await reportFixture(owner, 'codex-native', 'occupancy', 'available', { correctionOf: codexBusy, correctionAction: 'recover' })).status, 201);
  const capacity = await ownerAPI(owner, '/routing/capacity?contextId=codex-native');
  assert.equal(capacity.status, 200);
  assert.equal(capacity.envelope.data.reportEligibility.permitted, true, 'the original busy report must be explicitly recovered before sibling admission');
  await page.locator('#graph-refresh').click();
  const advanceResponse = page.waitForResponse(r => r.request().method() === 'POST' && new URL(r.url()).pathname === '/api/v1/graphs/' + graphId + '/advance');
  await page.locator('#graph-advance').click();
  const advance = await advanceResponse, advanceBody = await advance.json();
  assert.equal(advance.status(), 200, JSON.stringify(advanceBody.error));
  assert.ok(advanceBody.data.dispatches.some(dispatch => dispatch.dispatch.nodeId === sibling.id), 'Advance must actually dispatch the unpaused sibling');
  const withSibling = await pollOwner(owner, '/graphs/' + graphId, data => !!data.nodes.find(node => node.id === sibling.id)?.latestAttemptId);
  const siblingAttempt = withSibling.nodes.find(node => node.id === sibling.id).latestAttemptId;
  assert.equal(withSibling.nodes.find(node => node.id === member.id).latestAttemptId, null, 'paused member cannot be newly admitted');
  const held = await pollOwner(owner, '/attempts/' + siblingAttempt, data => data.phase === 'running' && !!data.sourceSnapshotId, { timeoutMs: 10000 });
  assert.equal(held.executionMode, 'fake');
  const workspaceRoot = await realpath(path.join(f.home, '.council', 'v2', 'workspaces'));
  const workspacePath = path.join(workspaceRoot, held.sourceSnapshotId), workspace = await realpath(workspacePath);
  assert.equal(path.dirname(workspace), workspaceRoot);
  assert.equal((await lstat(workspacePath)).isSymbolicLink(), false);
  const markerPath = path.join(workspace, 'council-graph-ready.marker'), markerDeadline = Date.now() + 5000;
  let ready = false;
  while (Date.now() < markerDeadline) {
    try { if (await readFile(markerPath, 'utf8') === 'ready\n') { ready = true; break; } } catch (error) { if (error.code !== 'ENOENT') throw error; }
    await new Promise(resolve => setTimeout(resolve, 20));
  }
  assert.equal(ready, true, 'the controlled sibling must hold its real worker before resume');
  await page.locator('#graph-refresh').click();
  const resume = page.getByRole('button', { name: 'Resume checkpoint ' + checkpoint.id.slice(0, 8), exact: true });
  await resume.waitFor();
  const resumedResponse = page.waitForResponse(r => r.request().method() === 'POST' && new URL(r.url()).pathname === '/api/v1/checkpoints/' + checkpoint.id + '/resume');
  await resume.click();
  const resumed = await resumedResponse, resumedBody = await resumed.json();
  assert.equal(resumed.status(), 200, JSON.stringify(resumedBody.error));
  assert.ok(resumedBody.data.blockers.includes('capacity_wait'), 'the still-held sibling must produce an explicit capacity wait');
  assert.equal(resumedBody.data.admittedAttemptIds.length, 0, 'the paused member has not been admitted while the sibling holds capacity');
  await writeFile(path.join(workspace, 'council-graph-release.marker'), 'release\n', { flag: 'wx', mode: 0o600 });
  await pollOwner(owner, '/attempts/' + siblingAttempt, data => data.phase === 'settled');
  const after = await pollOwner(owner, '/graphs/' + graphId, data => !!data.nodes.find(node => node.id === member.id)?.latestAttemptId);
  assert.ok(after.nodes.find(node => node.id === member.id).latestAttemptId);
  const dispatches = await ownerAPI(owner, '/graphs/' + graphId + '/dispatches');
  assert.equal(dispatches.status, 200);
  assert.equal(dispatches.envelope.data.items.filter(item => item.dispatch.nodeId === sibling.id && item.dispatch.role === 'author').length, 1, 'resuming a different member must not replay the sibling author');
});

test('UI member pause continues through the actual CLI and is observed in the UI without a duplicate admission', { timeout: 120000 }, async t => {
  const f = await fixture(); t.after(() => f.close());
  const owner = await f.start();
  const opened = await ownerAPI(owner, '/projects/open', 'POST', { requestId: fixtureRequestId(), selectedPath: f.project, basePath: f.project });
  assert.equal(opened.status, 201);
  let codexBusy;
  for (const context of ['codex-native', 'claude-native']) {
    const report = await reportFixture(owner, context, 'occupancy', 'busy');
    assert.equal(report.status, 201);
    if (context === 'codex-native') codexBusy = report.envelope.data.report.id;
  }
  const created = await ownerAPI(owner, '/graphs', 'POST', { requestId: fixtureRequestId(), projectId: opened.envelope.data.project.id, executionMode: 'fake',
    nodes: [{ key: 'handoff', originalText: 'Return the controlled response.', fakeScenario: 'graph-response', acceptance: { deliverable: 'response_only' } }] });
  assert.equal(created.status, 201, JSON.stringify(created.envelope.error));
  const graphId = created.envelope.data.graph.id;
  await pollOwner(owner, '/graphs/' + graphId + '/inputs', data => data.items.length === 2 && data.items.every(item => item.state === 'ready'));
  const initial = (await ownerAPI(owner, '/graphs/' + graphId)).envelope.data;
  const member = initial.nodes[0];
  assert.equal(initial.budget.budget.consumedAdmissions, '0');
  const page = await openWorkspace(f, owner, { fake: false, workflow: 'auto' });
  await page.locator('#open-graph-list').click();
  await page.locator('#graph-list button').filter({ hasText: graphId }).click();
  await page.waitForFunction(id => document.getElementById('graph-facts')?.textContent.includes(id), graphId);
  const clicked = page.waitForResponse(r => r.request().method() === 'POST' && new URL(r.url()).pathname === '/api/v1/tasks/' + member.taskId + '/pause');
  await page.getByRole('button', { name: 'Pause member handoff', exact: true }).click();
  const pause = await clicked, pauseBody = await pause.json();
  assert.equal(pause.status(), 202, JSON.stringify(pauseBody.error));
  assert.equal(pauseBody.data.checkpoint.scope.nodeId, member.id);
  const checkpoint = await pollOwner(owner, '/checkpoints/' + pauseBody.data.checkpoint.id, data => data.state === 'paused');
  assert.equal((await reportFixture(owner, 'codex-native', 'occupancy', 'available', { correctionOf: codexBusy, correctionAction: 'recover' })).status, 201);
  const capacity = await ownerAPI(owner, '/routing/capacity?contextId=codex-native');
  assert.equal(capacity.envelope.data.reportEligibility.permitted, true);
  const current = (await ownerAPI(owner, '/graphs/' + graphId)).envelope.data;
  const node = current.nodes.find(item => item.id === member.id);
  const scope = { kind: 'member', taskId: node.taskId, taskVersion: node.taskVersion, expectedTaskRevision: node.taskRevision,
    graphId, graphVersion: current.graph.graphVersion, expectedGraphControlRevision: current.graph.controlRevision,
    nodeId: node.id, expectedNodeControlRevision: node.controlRevision, expectedBudgetAuthorityEpoch: current.graph.budgetAuthorityEpoch,
    expectedWindowNumber: current.budget.budget.currentWindow.number };
  const scopeFile = path.join(f.home, 'controlled-recovery-cli-scope.json');
  await writeFile(scopeFile, JSON.stringify(scope));
  const resumeId = fixtureRequestId();
  const args = ['checkpoints', 'resume', checkpoint.id, '--checkpoint-revision', checkpoint.revision, '--scope-file', scopeFile, '--request-id', resumeId];
  const resumed = f.cli(args);
  assert.equal(resumed.status, 0, JSON.stringify(resumed.envelope.error));
  assert.equal(resumed.envelope.receipt.requestId, resumeId);
  assert.equal(resumed.envelope.receipt.operation, 'checkpoints.resume');
  assert.equal(resumed.envelope.data.checkpoint.id, checkpoint.id);
  assert.equal(resumed.envelope.data.checkpoint.state, 'resumed');
  assert.equal(resumed.envelope.data.admittedAttemptIds.length, 1);
  const attemptId = resumed.envelope.data.admittedAttemptIds[0];
  const replay = f.cli(args);
  assert.equal(replay.status, 0, JSON.stringify(replay.envelope.error));
  assert.deepEqual(replay.envelope, resumed.envelope, 'the exact CLI retry returns the same checkpoint receipt');
  const finished = await pollOwner(owner, '/attempts/' + attemptId, data => data.phase === 'settled');
  assert.equal(finished.executionMode, 'fake');
  assert.equal(finished.outcome, 'succeeded');
  const after = await pollOwner(owner, '/graphs/' + graphId, data => data.nodes[0].latestAttemptId === attemptId);
  assert.equal(after.budget.budget.consumedAdmissions, '1', 'resume admits exactly one fake native stage');
  const dispatches = await ownerAPI(owner, '/graphs/' + graphId + '/dispatches');
  assert.equal(dispatches.status, 200);
  assert.equal(dispatches.envelope.data.items.filter(item => item.dispatch.nodeId === member.id && item.dispatch.role === 'author').length, 1);
  await page.locator('#graph-refresh').click();
  await page.getByRole('button', { name: 'Open task handoff', exact: true }).click();
  await page.locator('#attempt-facts').filter({ hasText: attemptId }).waitFor({ state: 'attached' });
  assert.match(await page.locator('#attempt-status').textContent(), /LOCAL FAKE.*succeeded/i, 'the UI observes the actual CLI-started fake outcome');
  const finalGraph = (await ownerAPI(owner, '/graphs/' + graphId)).envelope.data;
  assert.equal(finalGraph.budget.budget.consumedAdmissions, '1', 'observation and receipt replay do not add a model admission');
});
