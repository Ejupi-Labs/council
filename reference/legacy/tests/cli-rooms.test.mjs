import test from 'node:test';
import assert from 'node:assert/strict';
import { execFile } from 'node:child_process';
import { existsSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { setTimeout as delay } from 'node:timers/promises';
import { fileURLToPath } from 'node:url';
import { promisify } from 'node:util';

const execute = promisify(execFile);
const cli = fileURLToPath(new URL('../dist/cli.js', import.meta.url));
const native = fileURLToPath(new URL('./fixtures/fake-room-native.mjs', import.meta.url));
function fixture(t) {
  const root = mkdtempSync(join(tmpdir(), 'council-cli-rooms-'));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  return root;
}
function call(root, args, backend = 'mock') {
  return execute(process.execPath, [cli, ...args, '--workspace', root], { windowsHide: true, timeout: 20000, maxBuffer: 8 * 1024 * 1024, env: { ...process.env, COUNCIL_DEPTH: '0', COUNCIL_BACKEND: backend } });
}
function input(root, filename, value) { writeFileSync(join(root, filename), JSON.stringify(value)); return filename; }
const plan = { title: 'Offline CLI project', brief: 'Exercise a real command workflow without providers.', members: [{ agent: 'codex', role: 'lead' }], autoExecute: false, tasks: [
  { title: 'Inspect', brief: 'Inspect the supplied task.', permission: 'read' },
  { title: 'Review', brief: 'Review the retained evidence.', permission: 'read', dependsOn: ['task-1'] },
] };
const json = response => JSON.parse(response.stdout);

test('CLI creates, discusses, executes and extends a room without replaying completed tasks', async t => {
  const root = fixture(t);
  const room = json(await call(root, ['room-create', '--input-file', input(root, 'room.json', plan)]));
  assert.equal(room.status, 'idle');
  const listed = json(await call(root, ['rooms', '--limit', '1']));
  assert.equal(listed[0].id, room.id); assert.equal(Object.hasOwn(listed[0], 'brief'), false);
  const discussed = await call(root, ['room-discuss', room.id]);
  assert.equal(json(discussed).status, 'ready');
  assert.match(discussed.stderr, /"event":"room.accepted"/);
  const completed = json(await call(root, ['room-execute', room.id]));
  assert.equal(completed.status, 'completed');
  assert.equal(completed.callsUsed, 3);
  const originalIds = completed.tasks.map(task => task.runId);
  assert(originalIds.every(Boolean));
  assert.equal(json(await call(root, ['room-show', room.id])).status, 'completed');
  const followup = { title: 'Follow up', brief: 'Review both retained results.', permission: 'read', dependsOn: ['task-2'] };
  await call(root, ['room-task', room.id, '--input-file', input(root, 'followup.json', followup)]);
  await call(root, ['room-discuss', room.id]);
  const extended = json(await call(root, ['room-execute', room.id]));
  assert.equal(extended.status, 'completed');
  assert.deepEqual(extended.tasks.slice(0, 2).map(task => task.runId), originalIds);
  assert.equal(extended.callsUsed, 5);
  assert.equal(json(await call(root, ['room-wait', room.id, '--wait-ms', '0'])).timedOut, false);
  assert.equal(json(await call(root, ['room-recover', room.id])).status, 'completed');
});

test('CLI room history returns complete messages and explicit cursors rather than a truncated preview', async t => {
  const root = fixture(t);
  const room = json(await call(root, ['room-create', '--input-file', input(root, 'room.json', { ...plan, tasks: [] })]));
  const text = '雪"\\'.repeat(140000);
  await call(root, ['room-message', room.id, '--input-file', input(root, 'message.json', { author: 'human', body: text })]);
  await call(root, ['room-message', room.id, '--input-file', input(root, 'second.json', { author: 'human', body: 'Complete second message.' })]);
  const first = json(await call(root, ['room-history', room.id, '--limit', '1']));
  assert.equal(first.messages.length, 1); assert.notEqual(first.nextCursor, null);
  const all = [...first.messages]; let cursor = first.nextCursor;
  while (cursor !== null) {
    const page = json(await call(root, ['room-history', room.id, '--limit', '1', '--cursor', String(cursor)]));
    all.push(...page.messages); cursor = page.nextCursor;
  }
  assert(all.some(message => message.body === text));
  assert.equal(all.at(-1).body, 'Complete second message.');
});

test('invalid room flags and incomplete inputs fail before opening project state', async t => {
  const root = fixture(t);
  for (const args of [
    ['room-create'], ['room-show'], ['room-show', 'not-an-id'], ['rooms', '--limit', '0'],
    ['room-history', '00000000-0000-4000-8000-000000000000', '--cursor', '-1'],
    ['room-wait', '00000000-0000-4000-8000-000000000000', '--wait-ms', 'Infinity'],
    ['room-execute', '00000000-0000-4000-8000-000000000000', '--write'],
  ]) await assert.rejects(call(root, args), error => error.code === 1);
  assert.equal(existsSync(join(root, '.council')), false);
});

test('CLI execution keeps failed native task evidence and exits nonzero instead of claiming completion', async t => {
  const root = fixture(t);
  writeFileSync(join(root, 'council.config.json'), JSON.stringify({ agents: { codex: { profile: 'codex', command: process.execPath, args: [native, '--fail-task'] } } }));
  const room = json(await call(root, ['room-create', '--input-file', input(root, 'room.json', { ...plan, tasks: plan.tasks.slice(0, 1) })], 'native'));
  await call(root, ['room-discuss', room.id], 'native');
  await assert.rejects(call(root, ['room-execute', room.id], 'native'), error => {
    const result = JSON.parse(error.stdout);
    assert.equal(result.status, 'failed');
    assert.equal(result.tasks[0].status, 'failed');
    assert(result.tasks[0].runId);
    assert.match(result.error, /Task task-1 failed:/);
    return error.code === 1;
  });
  const retained = json(await call(root, ['room-show', room.id], 'native'));
  assert.equal(retained.status, 'failed');
  assert.equal(json(await call(root, ['show', retained.tasks[0].runId], 'native')).status, 'failed');
});

test('a foreign room wait times out without cancelling the owning foreground CLI execution', async t => {
  const root = fixture(t), marker = join(root, 'started.json');
  writeFileSync(join(root, 'council.config.json'), JSON.stringify({ agents: { codex: { profile: 'codex', command: process.execPath, args: [native, '--delay-task', '--started-file', marker] } } }));
  const room = json(await call(root, ['room-create', '--input-file', input(root, 'room.json', { ...plan, tasks: plan.tasks.slice(0, 1) })], 'native'));
  await call(root, ['room-discuss', room.id], 'native');
  const completion = call(root, ['room-execute', room.id], 'native');
  completion.catch(() => {});
  t.after(async () => { await completion.catch(() => {}); });
  const until = performance.now() + 10000;
  while (!existsSync(marker)) { assert(performance.now() < until, 'Native fixture task did not start.'); await delay(5); }
  await assert.rejects(call(root, ['room-wait', room.id, '--wait-ms', '0'], 'native'), error => {
    const result = JSON.parse(error.stdout);
    assert.equal(result.timedOut, true); assert.equal(result.room.status, 'running');
    return error.code === 1;
  });
  assert.equal(json(await completion).status, 'completed');
});



test('paused foreground execution exits incomplete, then resume completes pending work without replay', async t => {
  const root = fixture(t), marker = join(root, 'pause-started.json');
  writeFileSync(join(root, 'council.config.json'), JSON.stringify({ agents: { codex: { profile: 'codex', command: process.execPath, args: [native, '--delay-task', '--task-delay-ms', '3000', '--started-file', marker] } } }));
  const room = json(await call(root, ['room-create', '--input-file', input(root, 'room.json', plan)], 'native'));
  await call(root, ['room-discuss', room.id], 'native');
  const executing = call(root, ['room-execute', room.id], 'native');
  const outcome = executing.then(value => ({ value }), error => ({ error }));
  t.after(async () => { await outcome; });
  const until = performance.now() + 10000;
  while (!existsSync(marker)) { assert(performance.now() < until, 'Task did not reach its admitted native turn.'); await delay(5); }
  const paused = json(await call(root, ['room-pause', room.id], 'native'));
  assert.equal(paused.status, 'paused');
  const result = await outcome;
  assert(result.error, 'Execution must exit nonzero while its room remains incomplete.');
  assert.equal(result.error.code, 1);
  const partial = JSON.parse(result.error.stdout);
  assert.equal(partial.status, 'paused');
  assert.equal(partial.tasks[0].status, 'completed');
  assert.equal(partial.tasks[1].status, 'assigned');
  assert.equal(partial.tasks[1].runId, undefined);
  const completedId = partial.tasks[0].runId;
  const resumed = json(await call(root, ['room-resume', room.id], 'native'));
  assert.equal(resumed.status, 'completed');
  assert.equal(resumed.tasks[0].runId, completedId);
  assert.equal(resumed.callsUsed, 3);
});
