import { spawn, spawnSync } from 'node:child_process';
import { mkdtemp, mkdir, writeFile, readFile } from 'node:fs/promises';
import path from 'node:path';
import os from 'node:os';
import { once } from 'node:events';
import { randomBytes } from 'node:crypto';
import { chromium } from '@playwright/test';
import { binary, root } from '../../scripts/tools.mjs';

// These helpers only address the disposable Council owner; no vendor executable is used.
export const fixtureRequestId = () => randomBytes(16).toString('hex');

export function responseBarrier() {
  let arrive; let release;
  const arrived = new Promise(resolve => { arrive = resolve; });
  const released = new Promise(resolve => { release = resolve; });
  return { arrived, release, async hold() { arrive(); await released; } };
}

export async function ownerAPI(owner, route, method = 'GET', body) {
  const address = new URL(owner.url);
  const token = new URLSearchParams(address.hash.slice(1)).get('token') ?? decodeURIComponent(address.hash.slice(1));
  const response = await fetch(address.origin + '/api/v1' + route, {
    method, redirect: 'error', signal: AbortSignal.timeout(20000),
    headers: { Authorization: 'Bearer ' + token, Origin: address.origin, ...(body ? { 'Content-Type': 'application/json' } : {}) },
    ...(body ? { body: JSON.stringify(body) } : {}),
  });
  return { status: response.status, envelope: await response.json() };
}

export async function openWorkspace(f, owner, { fake = true } = {}) {
  const page = await f.browser.newPage(); await page.goto(owner.url);
  await page.locator('#project-path').fill(f.project); await page.locator('#open-project').click();
  await page.waitForFunction(() => !document.querySelector('#start-task')?.disabled);
  if (fake) {
    await page.locator('#execution-options > summary').click();
    await page.locator('#task-fake').check(); await page.locator('#task-deliverable').selectOption('response_only');
  }
  return page;
}

export async function reportFixture(owner, contextId, dimension, state, extra = {}) {
  const current = await ownerAPI(owner, '/routing/capacity?contextId=' + contextId);
  if (current.status !== 200) throw new Error('Fixture capacity read failed: ' + current.envelope.error?.code);
  return ownerAPI(owner, '/routing/reports', 'POST', {
    requestId: fixtureRequestId(), subject: { kind: 'context', id: contextId },
    expectedRevision: current.envelope.data.revision, dimension, state, validForSeconds: 900,
    note: 'CONTROLLED LOCAL ROUTING TEST DATA; no provider access or quota was observed.', ...extra,
  });
}

export async function seedSchema3(f) {
  const output = path.join(f.home, 'controlled-schema3-identities.json');
  const env = { ...process.env, COUNCIL_ROUTING_SCHEMA3_SEED_PATH: path.join(f.home, '.council', 'v2', 'workspace.sqlite3'), COUNCIL_ROUTING_SCHEMA3_PROJECT: f.project, COUNCIL_ROUTING_SCHEMA3_SEED_OUTPUT: output };
  const executable = process.env.COUNCIL_ROUTING_STORE_TEST_BINARY || 'go';
  const args = process.env.COUNCIL_ROUTING_STORE_TEST_BINARY ? ['-test.run=^TestRoutingSchema3SeedChild$', '-test.count=1'] : ['test', './internal/store', '-run', '^TestRoutingSchema3SeedChild$', '-count=1'];
  const result = spawnSync(executable, args, { cwd: root, env, encoding: 'utf8', shell: false, timeout: 120000 });
  if (result.status !== 0) throw new Error('Controlled accepted-schema3 seed failed (exit ' + result.status + '): ' + result.stderr);
  return JSON.parse(await readFile(output, 'utf8'));
}

export async function fixture(t) {
  const owners = new Set(); let closed = false;
  const home = await mkdtemp(path.join(os.tmpdir(), 'council-foundation-'));
  const project = path.join(home, 'Project café');
  await mkdir(project);
  const env = { ...process.env, USERPROFILE: home, HOME: home };
  function git(args) { const result = spawnSync('git', args, { cwd: project, env, encoding: 'utf8', shell: false }); if (result.status !== 0) throw new Error('Fixture Git failed'); return result.stdout; }
  git(['init', '-q']);
  await writeFile(path.join(project, 'hello.txt'), 'original\n');
  git(['add', 'hello.txt']);
  git(['-c', 'user.name=Council Test', '-c', 'user.email=council@example.invalid', 'commit', '-qm', 'fixture']);
  await writeFile(path.join(project, 'hello.txt'), 'staged\n'); git(['add', 'hello.txt']);
  await writeFile(path.join(project, 'hello.txt'), 'unstaged\n');
  const indexBefore = await readFile(path.join(project, '.git/index'));
  const cli = (args) => {
    const result = spawnSync(binary, [...args, '--json'], { cwd: project, env, encoding: 'utf8', shell: false, timeout: 20000 });
    let data;
    try { data = JSON.parse(result.stdout); } catch { throw new Error(`CLI returned no JSON (exit ${result.status})`); }
    return { status: result.status, envelope: data };
  };
  async function start() {
    const process = spawn(binary, ['ui', '--no-open', '--json'], { cwd: root, env, shell: false, stdio: ['ignore', 'pipe', 'pipe'] });
    let output = ''; let errors = '';
    process.stderr.on('data', chunk => { errors += chunk.toString(); });
    const launch = await new Promise((resolve, reject) => {
      const timer = setTimeout(() => { process.kill(); reject(new Error('Owner startup timed out')); }, 20000);
      process.once('exit', code => { clearTimeout(timer); reject(new Error(`Owner startup failed (${code}): ${errors.replace(/token=[^\s]+/g, 'token=[redacted]')}`)); });
      process.stdout.on('data', chunk => {
        output += chunk.toString();
        if (!output.includes('\n')) return;
        clearTimeout(timer);
        try { const envelope = JSON.parse(output.split('\n')[0]); if (envelope.error) reject(new Error(envelope.error.code)); else resolve(envelope.data); } catch { reject(new Error('Invalid startup envelope')); }
      });
    });
    let stopping;
    const owner = { ...launch, async stop() {
      if (stopping) return stopping;
      stopping = (async () => {
        if (process.exitCode !== null) return;
        const exited = once(process, 'exit');
        try {
          const meta = await ownerAPI(owner, '/meta');
          await ownerAPI(owner, '/service/stop', 'POST', { requestId: fixtureRequestId(), ownerInstanceId: meta.envelope.data.ownerInstanceId });
          await Promise.race([exited, new Promise((_, reject) => { const timer = setTimeout(() => reject(new Error('Fixture cleanup timed out')), 20000); timer.unref(); })]);
        } finally {
          // Emergency cleanup is restricted to this exact spawned fixture child.
          if (process.exitCode === null) { process.kill(); await exited; }
          owners.delete(owner);
        }
      })();
      return stopping;
    } };
    owners.add(owner);
    return owner;
  }
  const browser = await chromium.launch({ headless: true });
  const f = { home, project, env, cli, start, browser, indexBefore, async close() {
    if (closed) return; closed = true;
    try { for (const owner of owners) await owner.stop(); } finally { await browser.close(); }
  } };
  t?.after(() => f.close()); // Registered before the caller's first assertion.
  return f;
}
