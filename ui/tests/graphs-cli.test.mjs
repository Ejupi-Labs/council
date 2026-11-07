import assert from 'node:assert/strict';
import { readFile, writeFile } from 'node:fs/promises';
import path from 'node:path';
import test from 'node:test';
import { fixture, fixtureRequestId, ownerAPI, pollOwner } from './helpers.mjs';

test('the actual CLI retains graph guidance intent across owner recovery and never refreshes a retry into new work', async t => {
  const f = await fixture(t);
  let owner = await f.start();
  const run = args => {
    const result = f.cli(args);
    assert.equal(result.status, 0, result.envelope.error?.message);
    assert.equal(result.envelope.error, undefined);
    return result.envelope;
  };
  const project = run(['projects', 'open', f.project]).data.project;
  const originals = ['\ufeffComplete CLI original A 東京🙂\r\nReturn only a response.', 'Complete CLI original B π\nPreserve the staged source.'];
  const declaration = { executionMode: 'fake', maxParallelism: '2', sharedInstructions: 'Complete shared CLI instructions. Preserve every source byte.',
    routing: { budget: { admissionLimit: '2' } }, nodes: originals.map((originalText, i) => ({ key: i ? 'b' : 'a', originalText,
      fakeScenario: 'graph-response', pins: { harnessId: i ? 'claude-code' : 'codex' }, acceptance: { deliverable: 'response_only' } })) };
  const file = path.join(f.home, 'Complete graph 東京.json');
  await writeFile(file, JSON.stringify(declaration));
  const startArgs = ['graphs', 'start', '--project', project.id, '--graph-file', file, '--request-id', fixtureRequestId()];
  const created = run(startArgs);
  const id = created.data.graph.id;
  const settled = await pollOwner(owner, '/graphs/' + id, value => value.nodes.every(node => node.latestAttemptId) && value.budget.budget.consumedAdmissions === '2');
  for (const node of settled.nodes) {
    await pollOwner(owner, '/attempts/' + node.latestAttemptId, attempt => attempt.phase === 'settled' && attempt.outcome === 'succeeded' && attempt.cleanup.state === 'verified');
    assert.equal(run(['tasks', 'get', node.taskId]).data.task.originalText, originals[node.key === 'a' ? 0 : 1]);
  }
  const before = run(['graphs', 'get', id]).data;
  const a = before.nodes.find(node => node.key === 'a');
  const b = before.nodes.find(node => node.key === 'b');
  const oldInput = a.inputId;
  const oldAuthor = run(['attempts', 'get', a.latestAttemptId]).data;
  const guidance = '\ufeffComplete replacement guidance 東京🙂\r\nKeep the original request and do not refill allowance.';
  const guidanceFile = path.join(f.home, 'Full guidance café.txt');
  await writeFile(guidanceFile, guidance);
  const guidanceId = fixtureRequestId();
  const guideArgs = ['graphs', 'guide', id, '--node', 'a', '--guidance-file', guidanceFile, '--request-id', guidanceId];
  const guided = run(guideArgs);
  assert.equal(guided.data.node.taskVersion, '2');
  assert.equal(guided.data.node.inputId, null);
  assert.equal((await ownerAPI(owner, '/tasks/' + a.taskId + '/versions/2')).envelope.data.version.guidanceText, guidance);

  // Restart the real owner before replay. The private CLI cache keeps the exact
  // old proof; recovering transport must not become fresh execution intent.
  await owner.stop();
  owner = await f.start();
  assert.deepEqual(run(guideArgs), guided);
  const guideB = run(['graphs', 'guide', id, '--node', 'b', '--guidance', 'Separate current B guidance.', '--request-id', fixtureRequestId()]);
  assert.equal(guideB.data.node.taskVersion, '2');
  assert.deepEqual(run(guideArgs), guided);
  await writeFile(guidanceFile, guidance + '\nChanged after capture.');
  const conflict = f.cli(guideArgs);
  assert.notEqual(conflict.status, 0);
  assert.equal(conflict.envelope.error?.code, 'request_conflict');

  const ready = run(['graphs', 'get', id]).data;
  assert.equal(ready.nodes.find(node => node.key === 'a').taskVersion, '2');
  assert.equal(ready.budget.budget.consumedAdmissions, '2');
  assert.equal(run(['graphs', 'dispatches', id]).data.items.length, 2);
  const advanceId = fixtureRequestId();
  const advanceArgs = ['graphs', 'advance', id, '--request-id', advanceId];
  const advanced = run(advanceArgs);
  assert.equal(advanced.data.preparationAttemptIds.length, 2);
  assert.deepEqual(advanced.data.dispatches, []);
  assert.deepEqual(advanced.data.reboundNodes, []);
  const prepared = await pollOwner(owner, '/graphs/' + id, value => value.nodes.every(node => node.taskVersion === '2' && node.inputId));
  assert.notEqual(prepared.nodes.find(node => node.key === 'a').inputId, oldInput);
  assert.equal(prepared.budget.budget.consumedAdmissions, '2');
  const nativeBefore = run(['graphs', 'dispatches', id]).data;
  assert.equal(nativeBefore.items.length, 2);
  assert.deepEqual(run(advanceArgs), advanced);
  for (const node of prepared.nodes) {
    await pollOwner(owner, '/graphs/' + id + '/inputs/' + node.inputId, value => value.summary.state === 'ready' && value.inputBinding !== null);
    const input = run(['graphs', 'input', id, '--input', node.inputId]).data;
    assert.equal(input.inputBinding.receivingTaskVersion, '2');
    const members = run(['graphs', 'input-members', id, '--input', node.inputId, '--limit', '1']).data;
    assert.equal(members.items.length, 1);
  }
  const oldAfter = run(['attempts', 'get', a.latestAttemptId]).data;
  for (const key of ['id', 'revision', 'phase', 'outcome', 'settledAt', 'sourceSnapshotId', 'candidateId', 'responseArtifactId']) assert.deepEqual(oldAfter[key], oldAuthor[key]);
  assert.equal(run(['graphs', 'dispatches', id]).data.items.length, 2);
  assert.deepEqual(await readFile(path.join(f.project, '.git', 'index')), f.indexBefore);
  assert.equal(await readFile(path.join(f.project, 'hello.txt'), 'utf8'), 'unstaged\n');
});
