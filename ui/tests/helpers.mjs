import { spawn, spawnSync } from 'node:child_process';
import { mkdtemp, mkdir, writeFile, readFile } from 'node:fs/promises';
import path from 'node:path';
import os from 'node:os';
import { once } from 'node:events';
import { chromium } from '@playwright/test';
import { binary, root } from '../../scripts/tools.mjs';

export async function fixture() {
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
    return { ...launch, async stop() { if (process.exitCode !== null) return; process.kill(); await once(process, 'exit'); } };
  }
  const browser = await chromium.launch({ headless: true });
  return { home, project, env, cli, start, browser, indexBefore, async close() { await browser.close(); } };
}
