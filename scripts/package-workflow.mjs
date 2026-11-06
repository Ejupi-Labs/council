import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { mkdir, readFile, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { setTimeout as delay } from 'node:timers/promises';

// The Node caller is qualification tooling. Every worker/check/publisher below
// is a child of the extracted Council binary and uses the supplied Git-only PATH.
export async function qualifyExtractedWorkflow({ binary, runtimeOptions, repository, temporary, project, git, runBinary }) {
  const run = args => runBinary(binary, args, runtimeOptions);
  const current = id => run(['tasks', 'workflow', id]).data;
  const completed = async id => {
    const deadline = Date.now() + 40_000;
    while (Date.now() < deadline) {
      const view = current(id);
      if (view.workflow?.state === 'completed') return view;
      if (['failed', 'cancelled', 'uncertain', 'blocked'].includes(view.workflow?.state) && view.workflow?.nextOwner === 'user') {
        throw new Error('Extracted workflow did not qualify: ' + JSON.stringify(view));
      }
      await delay(50);
    }
    throw new Error('Extracted workflow did not reach its bounded completion: ' + JSON.stringify(current(id)));
  };
  const steps = id => run(['tasks', 'steps', id]).data.items;
  const budget = id => run(['tasks', 'budget', id]).data.budget;
  const original = '\ufeffExtracted immutable document\r\nold\r\n東京\r\n';
  await mkdir(path.join(repository, 'docs'));
  await writeFile(path.join(repository, 'docs/note.md'), original);
  await writeFile(path.join(repository, 'user.txt'), 'staged user text\n');
  execFileSync(git, ['--no-optional-locks', '-c', 'core.fsmonitor=false', '-C', repository, 'add', '--', 'user.txt'], runtimeOptions);
  await writeFile(path.join(repository, 'user.txt'), 'unstaged user text\n');
  const indexBefore = await readFile(path.join(repository, '.git/index'));

  const destination = path.join(temporary, 'explicit-publication-marker.txt');
  const declaration = {
    localMode: 'review_only',
    externalAction: {
      action: 'record controlled local publication marker', destination, acceptanceBasis: 'command_completion',
      command: { executable: binary, arguments: ['__fixture', 'publish', '--case', 'workflow-publication', '--destination', destination] },
    },
  };
  const deliveryFile = path.join(temporary, 'extracted-delivery.json');
  await writeFile(deliveryFile, JSON.stringify(declaration));
  const directArgs = ['tasks', 'start', '--project', project.id, '--task', 'Replace "old" with "new" in docs/note.md only', '--fake', '--fake-scenario', 'workflow-document', '--delivery-file', deliveryFile];
  const directEnvelope = run(directArgs);
  const direct = directEnvelope.data;
  const directResult = await completed(direct.task.id);
  assert.equal(direct.workflow.authority.delivery.localMode, 'review_only');
  assert.equal(directResult.acceptance.deliveryResult, 'review_only');
  assert.equal(budget(direct.task.id).grantedAdmissions, '4');
  assert.equal(budget(direct.task.id).consumedAdmissions, '1');
  assert.equal(steps(direct.task.id).filter(s => s.executorKind === 'native').length, 1);
  assert.equal(steps(direct.task.id).filter(s => s.executorKind === 'local_check').length, 1);
  assert.equal(await readFile(path.join(repository, 'docs/note.md'), 'utf8'), original);
  const marker = await readFile(destination);
  assert.equal(marker.toString('utf8'), 'Council controlled local publication marker (005-v1).\n');
  const deliveries = run(['tasks', 'deliveries', direct.task.id]).data.items;
  const external = deliveries.filter(d => d.kind === 'external_command');
  assert.equal(external.length, 1);
  assert.equal(external[0].external.outcome, 'command_succeeded');
  assert.equal(external[0].external.action.acceptanceBasis, 'command_completion');
  assert.deepEqual(external[0].external.command.arguments, declaration.externalAction.command.arguments);
  assert.equal(deliveries.some(d => d.kind === 'local_application'), false);
  assert.deepEqual(run([...directArgs, '--request-id', directEnvelope.requestId]), directEnvelope);
  assert.deepEqual(await readFile(destination), marker, 'known Start replay never repeats the exclusive fixed publisher');
  assert.equal(steps(direct.task.id).filter(s => s.executorKind === 'local_external').length, 1);

  const checksFile = path.join(temporary, 'extracted-checks.json');
  await writeFile(checksFile, JSON.stringify([{ key: 'controlled-validation', executable: binary, arguments: ['__fixture', 'check', '--case', 'workflow-validation'] }]));
  const review = run(['tasks', 'start', '--project', project.id, '--task', 'Make council-workflow.txt contain exactly Council controlled workflow validation: VALID followed by CRLF. Inspect and correct the actual bytes.', '--fake', '--fake-scenario', 'workflow-defect', '--review', 'required', '--checks-file', checksFile]).data;
  await completed(review.task.id);
  assert.deepEqual(steps(review.task.id).filter(s => s.executorKind === 'native').map(s => s.role), ['author', 'review', 'correction', 'review']);
  const reviews = run(['tasks', 'reviews', review.task.id]).data.items;
  assert.deepEqual(reviews.map(r => r.outcome), ['changes_required', 'approved']);
  assert.notEqual(reviews[0].candidateBinding.candidateId, reviews[1].candidateBinding.candidateId);
  assert.ok(reviews[0].findings.some(f => f.finding.severity === 'blocking'));
  const checks = run(['tasks', 'checks', review.task.id]).data.items;
  assert.deepEqual(checks.filter(c => c.observedExecutable).map(c => c.outcome), ['failed', 'passed']);
  assert.equal(checks[0].exitCode, 1);
  assert.equal(checks[0].cleanup.state, 'verified');
  assert.equal(checks[0].rootTerminal, true);
  assert.equal(checks[0].inputComplete, true);
  assert.equal(checks[0].outputComplete, true);
  assert.equal(budget(review.task.id).consumedAdmissions, '4');
  assert.equal(budget(review.task.id).remainingAdmissions, '0');

  const preference = run(['workflow', 'preferences', 'get']).data;
  run(['workflow', 'preferences', 'update', '--delivery', 'apply_verified_local', '--expected-revision', preference.revision]);
  const apply = run(['tasks', 'start', '--project', project.id, '--task', 'Replace "old" with "new" in docs/note.md only', '--fake', '--fake-scenario', 'workflow-document']).data;
  const applied = await completed(apply.task.id);
  assert.equal(applied.acceptance.deliveryResult, 'applied');
  assert.equal(await readFile(path.join(repository, 'docs/note.md'), 'utf8'), original.replace('old', 'new'));
  assert.equal(await readFile(path.join(repository, 'user.txt'), 'utf8'), 'unstaged user text\n');
  assert.deepEqual(await readFile(path.join(repository, '.git/index')), indexBefore);
  const operations = run(['tasks', 'deliveries', apply.task.id]).data.items.filter(d => d.kind === 'local_application');
  assert.equal(operations.length, 1);
  assert.equal(operations[0].operation.status, 'applied');
  assert.ok(operations[0].operation.planArtifactId);
  assert.equal(steps(apply.task.id).filter(s => s.executorKind === 'local_application').length, 1);
  assert.equal(budget(apply.task.id).consumedAdmissions, '1');
}
