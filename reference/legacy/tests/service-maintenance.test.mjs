import test from 'node:test';
import assert from 'node:assert/strict';
import { execFileSync, spawnSync } from 'node:child_process';
import { existsSync, mkdirSync, mkdtempSync, readFileSync, realpathSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { basename, dirname, join } from 'node:path';
import { CouncilService } from '../dist/service.js';
import { CouncilStore } from '../dist/store.js';
import { loadConfig } from '../dist/config.js';
import { sanitizedGitEnvironment } from '../dist/git-env.js';

for (const operation of ['cleanup', 'patches']) test(`a dead ${operation} owner releases only its maintenance lease and permits explicit retry`, { timeout: 90_000 }, async t => {
  const temporaryBase = realpathSync(tmpdir());
  const temporary = mkdtempSync(join(temporaryBase, 'council-maintenance-crash-'));
  const root = join(temporary, 'repo'); mkdirSync(root);
  let service;
  t.after(async () => {
    await service?.close();
    const target = realpathSync(temporary);
    assert.equal(dirname(target), temporaryBase); assert.ok(basename(target).startsWith('council-maintenance-crash-'));
    rmSync(target, { recursive: true, force: true });
  });
  const environment = { ...sanitizedGitEnvironment(), COUNCIL_DEPTH: '0' };
  const git = (...args) => execFileSync('git', ['-C', root, ...args], { encoding: 'utf8', windowsHide: true, env: environment });
  git('init', '--quiet'); git('config', 'user.name', 'Offline Maintenance Test'); git('config', 'user.email', 'offline@example.invalid');
  writeFileSync(join(root, '.gitignore'), '.council/\n'); writeFileSync(join(root, 'base.txt'), 'original\n');
  git('add', '.'); git('commit', '--quiet', '-m', 'base');
  const state = new CouncilStore(join(root, '.council'));
  try { state.reserve('user-kept', ['notes/user-file.txt'], 3_600_000); } finally { state.close(); }
  const marker = join(temporary, 'run.json');
  const source = `
import {writeFileSync} from 'node:fs';
import {CouncilService} from ${JSON.stringify(new URL('../dist/service.js', import.meta.url).href)};
import {loadConfig} from ${JSON.stringify(new URL('../dist/config.js', import.meta.url).href)};
import {GitWorkspaces} from ${JSON.stringify(new URL('../dist/workspaces.js', import.meta.url).href)};
const service=new CouncilService(loadConfig(${JSON.stringify(root)},{backend:'mock'}));
const run=await service.run({mode:'delegate',task:'Retain an offline worktree for maintenance recovery.',agents:['codex'],permission:'write'});
if(run.status!=='completed')throw new Error(run.error);
writeFileSync(${JSON.stringify(marker)},JSON.stringify(run));
GitWorkspaces.prototype.${operation === 'cleanup' ? 'cleanup' : 'export'}=async()=>{process.exit(86);};
await service.${operation === 'cleanup' ? 'cleanup(run.id,{reviewed:true})' : 'exportArtifacts(run.id)'};
`;
  const child = spawnSync(process.execPath, ['--input-type=module', '-e', source], { encoding: 'utf8', windowsHide: true, timeout: 60_000, env: environment });
  assert.equal(child.status, 86, child.stderr || child.error?.message);
  const previous = JSON.parse(readFileSync(marker, 'utf8'));
  let calls = 0;
  service = new CouncilService(loadConfig(root, { backend: 'mock' }), { doctor: async () => [], run: async () => { calls++; throw new Error('Maintenance must never call an agent.'); } });
  assert.deepEqual(service.store.leases().map(lease => lease.owner), ['user-kept']);
  if (operation === 'cleanup') {
    const result = await service.cleanup(previous.id, { reviewed: true });
    assert.deepEqual(result.removed, [previous.workspaces[0].path]); assert.equal(existsSync(previous.workspaces[0].path), false);
    assert.ok(existsSync(previous.workspaces[0].patchPath));
  } else {
    const result = await service.exportArtifacts(previous.id);
    assert.equal(result.artifacts[0].exportStatus, 'ready'); assert.ok(existsSync(result.artifacts[0].path));
  }
  assert.equal(service.store.getRun(previous.id).status, 'completed'); assert.equal(calls, 0);
  assert.deepEqual(service.store.leases().map(lease => lease.owner), ['user-kept']);
});
