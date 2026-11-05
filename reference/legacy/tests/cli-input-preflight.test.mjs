import test from 'node:test';
import assert from 'node:assert/strict';
import { execFile } from 'node:child_process';
import { existsSync, mkdtempSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { promisify } from 'node:util';

const cli = fileURLToPath(new URL('../dist/cli.js', import.meta.url));
const execute = promisify(execFile);
const absentId = '00000000-0000-4000-8000-000000000000';
function workspace(t) {
  const root = mkdtempSync(join(tmpdir(), 'council-cli-input-'));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  return root;
}
function call(root, args) {
  return execute(process.execPath, [cli, ...args, '--workspace', root], {
    windowsHide: true, timeout: 15_000, maxBuffer: 2 * 1024 * 1024,
    env: { ...process.env, COUNCIL_DEPTH: '0', COUNCIL_BACKEND: 'mock' },
  });
}
async function rejectsBeforeState(root, args, message) {
  await assert.rejects(call(root, args), error => {
    assert.notEqual(error.code, 0);
    assert.equal(error.stdout, '');
    assert.match(error.stderr, message);
    assert.equal(error.stderr.includes('run.started'), false);
    assert.equal(error.stderr.includes('room.accepted'), false);
    return true;
  });
  assert.equal(existsSync(join(root, '.council')), false, 'invalid input must not open state or register a host');
}

test('missing supplied workflow and request files fail before creating Council state', async t => {
  const root = workspace(t);
  for (const args of [
    ['run', '--task-file', 'missing.txt'],
    ['plan', '--task', 'Inspect evidence.', '--context-file', 'missing.txt'],
    ['continue', absentId, '--task', 'Inspect evidence.', '--assignments-file', 'missing.json'],
    ['route-plan', '--task-file', 'missing.txt', '--task-class', 'coding'],
    ['billing-set', '--agent', 'codex', '--input-file', 'missing.json'],
    ['room-create', '--input-file', 'missing.json'],
  ]) await rejectsBeforeState(root, args, /ENOENT/);
});

test('usage errors retain priority over reading conflicting or incomplete file arguments', async t => {
  const root = workspace(t);
  await rejectsBeforeState(root, ['route-plan', '--input-file', 'missing.json', '--task', 'Conflicting flags.'], /Choose a routing --input-file or routing flags/);
  await rejectsBeforeState(root, ['billing-set', '--input-file', 'missing.json'], /requires --agent and --input-file/);
  await rejectsBeforeState(root, ['doctor', '--task-file', 'missing.txt'], /--task-file is not supported by doctor/);
});

test('invalid UTF-8 in every supplied file kind is rejected without leaking or replacing content', async t => {
  const root = workspace(t);
  writeFileSync(join(root, 'invalid.txt'), Buffer.concat([Buffer.from('private-evidence-'), Buffer.from([0xff])]));
  writeFileSync(join(root, 'invalid.json'), Buffer.concat([Buffer.from('{"task":"private-evidence-'), Buffer.from([0xff]), Buffer.from('"}')]));
  for (const args of [
    ['run', '--task-file', 'invalid.txt'],
    ['run', '--task', 'Inspect evidence.', '--context-file', 'invalid.txt'],
    ['run', '--task', 'Inspect evidence.', '--assignments-file', 'invalid.json'],
    ['route-plan', '--input-file', 'invalid.json'],
    ['billing-import', '--agent', 'codex', '--input-file', 'invalid.json'],
    ['room-task', absentId, '--input-file', 'invalid.json'],
    ['room-message', absentId, '--input-file', 'invalid.json'],
  ]) {
    await rejectsBeforeState(root, args, /valid UTF-8/);
  }
  await assert.rejects(call(root, ['run', '--task-file', 'invalid.txt']), error => {
    assert.equal(error.stderr.includes('private-evidence'), false);
    return true;
  });
});

test('malformed, ambiguous and schema-invalid request files are rejected before state opens', async t => {
  const root = workspace(t);
  const cases = [
    ['{', ['run', '--task', 'Inspect evidence.', '--assignments-file', 'input.json'], /Invalid JSON/],
    ['{"codex":42}', ['run', '--task', 'Inspect evidence.', '--assignments-file', 'input.json'], /assignments.codex/],
    ['{"task":"first","task":"second","taskClass":"coding"}', ['route-plan', '--input-file', 'input.json'], /duplicate JSON object keys/],
    ['{"task":"Inspect evidence.","taskClass":"coding","permission":"read","permission":"write"}', ['route-plan', '--input-file', 'input.json'], /duplicate JSON object keys/],
    ['{"task":"Inspect evidence.","taskClass":"unknown-category"}', ['route-plan', '--input-file', 'input.json'], /taskClass/],
    ['{"title":42,"brief":"Inspect evidence.","members":[{"agent":"codex","role":"worker"}]}', ['room-create', '--input-file', 'input.json'], /title/],
    ['{"title":"Inspect evidence."}', ['room-task', absentId, '--input-file', 'input.json'], /brief/],
    ['{"mode":"invalid-mode"}', ['billing-set', '--agent', 'codex', '--input-file', 'input.json'], /mode/],
    ['{"source":{"kind":"reported","label":"Manual observation"},"observedAt":42}', ['billing-import', '--agent', 'codex', '--input-file', 'input.json'], /observedAt/],
  ];
  for (const [document, args, message] of cases) {
    writeFileSync(join(root, 'input.json'), document);
    await rejectsBeforeState(root, args, message);
  }
});

test('valid BOM-prefixed task, context and assignments retain full Unicode through dispatch', async t => {
  const root = workspace(t);
  const task = 'Inspect café 東京 🐜 and intentional \uFFFD.';
  const context = 'Evidence: società 🧪. Interior \uFEFF is preserved.';
  const assignment = 'Preserve naïve Unicode 🐜 and this \uFFFD symbol.';
  writeFileSync(join(root, 'task.txt'), '\uFEFF' + task);
  writeFileSync(join(root, 'context.txt'), '\uFEFF' + context);
  writeFileSync(join(root, 'assignments.json'), '\uFEFF' + JSON.stringify({ codex: assignment }));
  const run = JSON.parse((await call(root, ['run', '--mode', 'parallel', '--agents', 'codex', '--task-file', 'task.txt', '--context-file', 'context.txt', '--assignments-file', 'assignments.json'])).stdout);
  assert.equal(run.status, 'completed');
  assert.equal(run.input.task, task);
  assert.equal(run.input.context, context);
  assert.deepEqual(run.input.assignments, { codex: assignment });
});
