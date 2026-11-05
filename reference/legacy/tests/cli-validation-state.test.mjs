import test from 'node:test';
import assert from 'node:assert/strict';
import { execFile } from 'node:child_process';
import { existsSync, mkdtempSync, readFileSync, realpathSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { basename, dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { promisify } from 'node:util';
const execute = promisify(execFile), cli = fileURLToPath(new URL('../dist/cli.js', import.meta.url));
const preload = new URL('./fixtures/file-read-once.mjs', import.meta.url).href;
const absent = '11111111-1111-4111-8111-111111111111';
function fixture(t) {
  const parent = realpathSync.native(tmpdir()), root = mkdtempSync(join(parent, 'council-cli-state-')), pending = new Set();
  t.after(async () => {
    let timer;
    try { await Promise.race([Promise.allSettled([...pending]), new Promise((_, reject) => { timer = setTimeout(() => reject(new Error('Owned CLI children did not drain.')), 5000); })]); }
    finally { clearTimeout(timer); }
    assert.equal(pending.size, 0); assert.equal(dirname(realpathSync.native(root)), parent); assert.ok(basename(root).startsWith('council-cli-state-'));
    rmSync(root, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 });
  });
  return { root, async call(args, files = []) {
    const trace = join(root, 'read-trace.json');
    const operation = execute(process.execPath, [...(files.length ? ['--import', preload] : []), cli, ...args, '--workspace', root, '--backend', 'mock'], {
      windowsHide: true, timeout: 10000, signal: t.signal, maxBuffer: 2 * 1024 * 1024,
      env: { ...process.env, COUNCIL_BACKEND: 'mock', COUNCIL_DEPTH: '0', ...(files.length ? { COUNCIL_TEST_READ_FILES: JSON.stringify(files), COUNCIL_TEST_READ_TRACE: trace } : {}) },
    });
    pending.add(operation); let result;
    try { result = await operation; } finally { pending.delete(operation); }
    if (files.length) assert.deepEqual(JSON.parse(readFileSync(trace, 'utf8')), Object.fromEntries(files.map(path => [path, 1])));
    return JSON.parse(result.stdout);
  } };
}
async function rejectBeforeState(wire, args, message) {
  await assert.rejects(wire.call(args), error => { assert.equal(error.code, 1); assert.match(error.stderr, message); return true; });
  assert.equal(existsSync(join(wire.root, '.council')), false);
}
test('CLI effort and skills added after base workflow parsing are rejected before opening state', { timeout: 20000 }, async t => {
  const wire = fixture(t);
  for (const command of ['plan', 'run']) {
    await rejectBeforeState(wire, [command, '--task', 'Offline validation.', '--effort', 'invalid-effort'], /agentSettings.codex.effort/);
    await rejectBeforeState(wire, [command, '--task', 'Offline validation.', '--skills', 'invalid-skill'], /agentSettings.codex.skills/);
  }
});
test('room-message structural schema rejects types, unknown fields and NUL before state', { timeout: 20000 }, async t => {
  const wire = fixture(t), filename = join(wire.root, 'input.json');
  for (const value of [{ author: 42, body: 'Offline.' }, { author: 'human', body: 'Offline.', surprise: true }, { author: 'human', body: 'NUL\0body' }]) {
    writeFileSync(filename, JSON.stringify(value)); await rejectBeforeState(wire, ['room-message', absent, '--input-file', 'input.json'], /message(?:\.(?:author|body)|: unknown property surprise)/);
  }
});
test('unsafe explicit billing revisions fail before opening workflow or billing state', { timeout: 20000 }, async t => {
  const wire = fixture(t); writeFileSync(join(wire.root, 'billing.json'), '{}');
  for (const revision of ['not-a-number', '-1', '1.5', '9007199254740992']) await rejectBeforeState(wire, ['billing-set', '--agent', 'codex', '--input-file', 'billing.json', '--revision=' + revision], /nonnegative safe integer/);
});
test('BOM Unicode workflow files are read once and retained after their paths change', { timeout: 20000 }, async t => {
  const wire = fixture(t), task = 'Inspect café 東京 🐜 and intentional �.', context = 'Società 🧪 and interior \uFEFF.', assignment = 'Retain naïve Unicode 🐜.';
  const files = ['task.txt', 'context.txt', 'assignments.json'].map(name => join(wire.root, name));
  writeFileSync(files[0], '\uFEFF' + task); writeFileSync(files[1], '\uFEFF' + context); writeFileSync(files[2], '\uFEFF' + JSON.stringify({ codex: assignment }));
  const run = await wire.call(['run', '--mode', 'parallel', '--agents', 'codex', '--task-file', 'task.txt', '--context-file', 'context.txt', '--assignments-file', 'assignments.json'], files);
  assert.equal(run.status, 'completed'); assert.equal(run.steps.length, 1); assert.equal(run.input.task, task); assert.equal(run.input.context, context); assert.deepEqual(run.input.assignments, { codex: assignment });
});
test('valid explicit effort and skills survive complete workflow validation', { timeout: 20000 }, async t => {
  const wire = fixture(t), run = await wire.call(['run', '--task', 'Offline validation.', '--effort', 'high', '--skills', 'council-clear-explanations']);
  assert.equal(run.status, 'completed'); assert.equal(run.steps.length, 1); assert.equal(run.input.agentSettings.codex.effort, 'high'); assert.deepEqual(run.input.agentSettings.codex.skills, ['council-clear-explanations']);
});
test('room-message uses its original complete Unicode input once and preserves its receipt', { timeout: 20000 }, async t => {
  const wire = fixture(t), create = join(wire.root, 'room.json'), message = join(wire.root, 'message.json');
  writeFileSync(create, '\uFEFF' + JSON.stringify({ title: 'Offline café 東京', brief: 'No providers.', members: [{ agent: 'codex', role: 'lead' }], autoExecute: false }));
  const room = await wire.call(['room-create', '--input-file', 'room.json'], [create]), body = 'Complete naïve guidance 🐜 with interior \uFEFF.';
  writeFileSync(message, '\uFEFF' + JSON.stringify({ author: 'human', body })); const changed = await wire.call(['room-message', room.id, '--input-file', 'message.json'], [message]);
  assert.equal(changed.status, 'idle'); assert.equal(changed.instructionRevision, (room.instructionRevision ?? 0) + 1); assert.equal(changed.messages.filter(value => value.body === body).length, 1);
});
test('explicit billing revision zero keeps the committed observation receipt', { timeout: 20000 }, async t => {
  const wire = fixture(t), filename = join(wire.root, 'billing.json'), plan = 'Offline café 東京';
  writeFileSync(filename, '\uFEFF' + JSON.stringify({ mode: 'api', plan, source: { kind: 'manual', label: 'Offline' } }));
  const updated = await wire.call(['billing-set', '--agent', 'codex', '--input-file', 'billing.json', '--revision', '0'], [filename]);
  const observed = await wire.call(['billing', '--agent', 'codex']); assert.equal(updated.revision, 1); assert.equal(observed.revision, 1); assert.equal(observed.plan, plan); assert.equal(observed.subscriptionProtection.status, 'disabled');
});
