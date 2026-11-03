import test from 'node:test';
import assert from 'node:assert/strict';
import { execFile, spawn } from 'node:child_process';
import { existsSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { promisify } from 'node:util';
import { CouncilService } from '../dist/service.js';
import { loadConfig } from '../dist/config.js';
import { killProcessTree } from '../dist/process.js';

const cli = fileURLToPath(new URL('../dist/cli.js', import.meta.url));
const execute = promisify(execFile);
const pause = ms => new Promise(resolve => setTimeout(resolve, ms));
const alive = pid => { try { process.kill(pid, 0); return true; } catch { return false; } };
function workspace(t) {
  const root = mkdtempSync(join(tmpdir(), 'council-cli-'));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  return root;
}
function call(root, args, options = {}) {
  return execute(process.execPath, [cli, ...args, '--workspace', root], {
    windowsHide: true, timeout: 15_000, maxBuffer: 2 * 1024 * 1024,
    env: { ...process.env, COUNCIL_DEPTH: '0', COUNCIL_BACKEND: 'mock' }, ...options,
  });
}
function events(stderr) { return stderr.split('\n').filter(line => line.startsWith('{')).map(line => JSON.parse(line)); }

test('CLI version is 1.0.0 and conflicting or inapplicable arguments fail before starting work', async t => {
  const root = workspace(t);
  assert.deepEqual(JSON.parse((await call(root, ['--version'])).stdout), { name: 'council', version: '1.0.0' });
  writeFileSync(join(root, 'task.txt'), 'task from a file');
  for (const args of [
    ['plan', '--task', 'inline task', '--task-file', 'task.txt'],
    ['doctor', '--write'], ['run', 'unexpected', '--task', 'task'],
    ['events', '00000000-0000-4000-8000-000000000000', '--limit', '0'],
    ['plan', '--mode', 'parallel', '--agents', 'codex,claude', '--model', 'explicit-model', '--task', 'compare'],
    ['run', '--task', 'task', '--output', 'result.json'],
  ]) await assert.rejects(call(root, args), error => { assert.notEqual(error.code, 0); assert.equal(error.stdout, ''); assert.ok(error.stderr.includes('Council:')); return true; });
  assert.deepEqual(JSON.parse((await call(root, ['runs'])).stdout), []);
});

test('CLI run emits its ID on stderr and one JSON result on stdout', async t => {
  const root = workspace(t);
  const response = await call(root, ['run', '--backend', 'mock', '--task', 'Inspect one bounded task.']);
  const run = JSON.parse(response.stdout);
  assert.equal(run.status, 'completed'); assert.equal(run.input.permission, 'read');
  const started = events(response.stderr).filter(event => event.event === 'run.started');
  assert.equal(started.length, 1); assert.equal(started[0].id, run.id);
  assert.equal(JSON.parse((await call(root, ['show', run.id])).stdout).id, run.id);
});

test('CLI events, report export and explicit continuation preserve evidence without overwriting files', async t => {
  const root = workspace(t);
  const run = JSON.parse((await call(root, ['run', '--task', 'Compare search choices using bounded evidence.'])).stdout);
  const log = JSON.parse((await call(root, ['events', run.id, '--limit', '2'])).stdout);
  assert.equal(log.length, 2); assert.equal(log[0].type, 'queued');
  const nextEvents = JSON.parse((await call(root, ['events', run.id, '--after', String(log.at(-1).sequence)])).stdout);
  assert.ok(nextEvents.every(event => event.sequence > log.at(-1).sequence));
  assert.ok(nextEvents.some(event => event.type === 'run.completed'));
  const report = (await call(root, ['export', run.id])).stdout;
  assert.match(report, /^# Council run report/); assert.ok(report.includes(run.id)); assert.ok(report.includes(run.input.task));
  const exported = JSON.parse((await call(root, ['export', run.id, '--output', 'report.md'])).stdout);
  assert.equal(exported.exported, join(root, 'report.md')); assert.equal(readFileSync(exported.exported, 'utf8'), report);
  await assert.rejects(call(root, ['export', run.id, '--output', 'report.md']), error => { assert.match(error.stderr, /exist|EEXIST/i); return true; });
  assert.equal(readFileSync(exported.exported, 'utf8'), report);
  await assert.rejects(call(root, ['continue', run.id]), error => { assert.match(error.stderr, /Task|task/); return true; });
  const response = await call(root, ['continue', run.id, '--mode', 'delegate', '--agents', 'junie', '--task', 'Check one remaining integration question.']);
  const continued = JSON.parse(response.stdout);
  assert.notEqual(continued.id, run.id); assert.equal(continued.status, 'completed');
  assert.deepEqual(continued.input.agents, ['junie']); assert.equal(continued.input.permission, 'read');
  assert.equal(continued.input.task, 'Check one remaining integration question.');
  assert.ok(continued.input.context.includes(run.id)); assert.equal(events(response.stderr)[0].id, continued.id);
  assert.deepEqual(JSON.parse((await call(root, ['recover'])).stdout).recovered, []);
});

function nativeFixture(root) {
  const filename = join(root, 'offline-native.mjs');
  const pids = join(root, 'owned-pids.json');
  writeFileSync(filename, `
import {spawn} from 'node:child_process';
import {writeFileSync} from 'node:fs';
let prompt='';for await(const chunk of process.stdin)prompt+=chunk.toString();
const send=value=>process.stdout.write(JSON.stringify(value)+'\\n');
send({type:'thread.started',thread_id:'offline-cli'});send({type:'turn.started'});
if(prompt.includes('CLI_TREE_HANG')){
 const child=spawn(process.execPath,['-e','setInterval(()=>{},1000)'],{stdio:'ignore',windowsHide:true});
 writeFileSync(${JSON.stringify(pids)},JSON.stringify({vendor:process.pid,grandchild:child.pid}));
 send({type:'item.completed',item:{type:'agent_message',text:'partial before interruption'}});
 setInterval(()=>{},1000);
}else{
 const text=prompt.includes('CLI_PRINT_ARGV')?JSON.stringify(process.argv.slice(2)):'offline native CLI result';
 send({type:'item.completed',item:{type:'agent_message',text}});send({type:'turn.completed',usage:{input_tokens:1,output_tokens:1}});
}
`);
  writeFileSync(join(root, 'council.config.json'), JSON.stringify({ backend: 'native', timeoutMs: 30_000,
    agents: { codex: { enabled: true, profile: 'codex', command: process.execPath, args: [filename] } } }));
  return { filename, pids };
}

test('CLI native custom commands preserve an explicitly requested model without global setup', async t => {
  const root = workspace(t); nativeFixture(root);
  const response = await call(root, ['run', '--backend', 'native', '--model', 'user-selected-model', '--task', 'CLI_PRINT_ARGV']);
  const run = JSON.parse(response.stdout);
  assert.equal(run.status, 'completed');
  const args = JSON.parse(run.output);
  assert.equal(args[args.indexOf('--model') + 1], 'user-selected-model');
  assert.equal(events(response.stderr)[0].id, run.id);
});

test('CLI interruption stops the native fake agent and its descendant, preserving an honest terminal state', { timeout: 25_000 }, async t => {
  const root = mkdtempSync(join(tmpdir(), 'council-cli-interrupt-'));
  const { pids: filename } = nativeFixture(root);
  const child = spawn(process.execPath, [cli, 'run', '--workspace', root, '--backend', 'native', '--task', 'CLI_TREE_HANG'], {
    shell: false, windowsHide: true, stdio: ['ignore', 'pipe', 'pipe'], env: { ...process.env, COUNCIL_DEPTH: '0', COUNCIL_BACKEND: 'native' },
  });
  let stdout = '', stderr = '', owned;
  child.stdout.on('data', chunk => { stdout += chunk.toString(); }); child.stderr.on('data', chunk => { stderr += chunk.toString(); });
  const closed = new Promise(resolve => child.once('close', (code, signal) => resolve({ code, signal })));
  try {
    const readyUntil = Date.now() + 10_000;
    while (Date.now() < readyUntil) {
      try { owned = JSON.parse(readFileSync(filename, 'utf8')); break; } catch { await pause(25); }
    }
    assert.ok(owned?.vendor && owned?.grandchild, 'offline native subprocess tree must be running');
    const started = events(stderr).find(event => event.event === 'run.started'); assert.ok(started?.id);
    child.kill('SIGTERM');
    const outcome = await closed; assert.notEqual(outcome.code, 0);
    const until = Date.now() + 7000;
    while (Date.now() < until && (alive(owned.vendor) || alive(owned.grandchild))) await pause(25);
    assert.equal(alive(owned.vendor), false); assert.equal(alive(owned.grandchild), false);
    const service = new CouncilService(loadConfig(root, { backend: 'mock' }));
    try {
      const run = service.store.getRun(started.id);
      assert.ok(['cancelled', 'failed'].includes(run.status));
      if (process.platform !== 'win32') { assert.equal(outcome.code, 143); assert.equal(JSON.parse(stdout).status, 'cancelled'); }
      else if (stdout.trim()) assert.equal(JSON.parse(stdout).status, 'cancelled');
    } finally { await service.close(); }
  } finally {
    if (child.pid && alive(child.pid)) await killProcessTree(child.pid);
    if (owned?.vendor && alive(owned.vendor)) await killProcessTree(owned.vendor);
    if (owned?.grandchild && alive(owned.grandchild)) await killProcessTree(owned.grandchild);
    await closed;
    rmSync(root, { recursive: true, force: true });
  }
});
