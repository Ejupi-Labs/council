import assert from 'node:assert/strict';
import { execFile } from 'node:child_process';
import { existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { basename, dirname, isAbsolute, join, relative, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { promisify } from 'node:util';
import test from 'node:test';
import { Client } from '@modelcontextprotocol/sdk/client/index.js';
import { StdioClientTransport } from '@modelcontextprotocol/sdk/client/stdio.js';

const execute = promisify(execFile);
const repository = fileURLToPath(new URL('../', import.meta.url));
const releaseVersion = JSON.parse(readFileSync(new URL('../package.json', import.meta.url), 'utf8')).version;

function npmEntrypoint() {
  const candidates = [
    process.env.npm_execpath,
    join(dirname(process.execPath), 'node_modules', 'npm', 'bin', 'npm-cli.js'),
    resolve(dirname(process.execPath), '..', 'lib', 'node_modules', 'npm', 'bin', 'npm-cli.js'),
  ];
  const entrypoint = candidates.find(candidate => candidate && existsSync(candidate));
  assert.ok(entrypoint, 'Run release acceptance through npm test, or install npm beside Node.');
  return entrypoint;
}

function removeTemporary(directory) {
  const target = resolve(directory);
  const withinTemp = relative(resolve(tmpdir()), target);
  assert.ok(withinTemp && !withinTemp.startsWith('..') && !isAbsolute(withinTemp), 'Release cleanup must stay inside the temporary directory.');
  rmSync(target, { recursive: true, force: true });
}

test('the release tarball installs fully offline and runs its CLI and MCP server without production dependencies', { timeout: 60_000 }, async t => {
  const root = mkdtempSync(join(tmpdir(), 'council-release-'));
  const consumer = join(root, 'consumer café 日本語 with spaces');
  const workspace = join(root, 'project café 日本語 with spaces');
  mkdirSync(consumer);
  mkdirSync(workspace);
  writeFileSync(join(consumer, 'package.json'), JSON.stringify({ name: 'council-release-consumer', version: '1.0.0', private: true }));
  const closeClients = [];
  const demoDirectories = [];
  t.after(async () => {
    for (const close of closeClients.toReversed()) await close();
    for (const directory of demoDirectories) removeTemporary(directory);
    removeTemporary(root);
  });
  const npm = npmEntrypoint();
  const environment = { ...process.env, npm_config_cache: join(root, 'empty npm cache'), COUNCIL_DEPTH: '0', COUNCIL_BACKEND: 'mock' };
  const runNode = async (args, cwd = consumer) => await execute(process.execPath, args, {
    cwd, env: environment, windowsHide: true, timeout: 30_000, maxBuffer: 8 * 1024 * 1024,
  });
  const packed = JSON.parse((await runNode([npm, 'pack', '--ignore-scripts', '--json', '--pack-destination', root], repository)).stdout);
  assert.equal(packed.length, 1);
  assert.equal(packed[0].version, releaseVersion);
  const packagedPaths = packed[0].files.map(file => file.path.replaceAll('\\', '/'));
  for (const file of ['dist/cli.js', 'plugin.json', 'mcp.json', '.codex-plugin/plugin.json', '.claude-plugin/plugin.json', 'skills/council/SKILL.md',
    'skills/dont-kill-an-ant-with-a-bazooka/SKILL.md', 'skills/council-implementation-review/SKILL.md',
    'skills/council-independent-perspectives/SKILL.md', 'skills/council-advisor/SKILL.md', 'schemas/billing.schema.json', 'schemas/routing.schema.json']) {
    assert.ok(packagedPaths.includes(file), `The release must contain ${file}.`);
  }
  assert.ok(packagedPaths.every(file => !/(^|\/)(?:\.council|\.serena|node_modules|tests)(?:\/|$)/.test(file)), 'Local state, dependencies and test fixtures must not enter the release.');
  const tarball = join(root, packed[0].filename);
  // An empty cache plus --offline proves installation cannot depend on a registry fetch.
  await runNode([npm, 'install', '--prefix', consumer, '--global=false', '--ignore-scripts', '--omit=dev', '--offline', '--no-audit', '--no-fund', tarball]);
  const installedRoot = join(consumer, 'node_modules', 'council-ai-plugin');
  const metadata = JSON.parse(readFileSync(join(installedRoot, 'package.json'), 'utf8'));
  assert.equal(metadata.version, releaseVersion);
  assert.equal(Object.keys(metadata.dependencies ?? {}).length, 0);
  assert.equal(Object.keys(metadata.optionalDependencies ?? {}).length, 0);
  for (const manifest of ['plugin.json', '.codex-plugin/plugin.json', '.claude-plugin/plugin.json']) {
    assert.equal(JSON.parse(readFileSync(join(installedRoot, manifest), 'utf8')).version, metadata.version, `${manifest} must match the package release version.`);
  }
  for (const manifest of ['mcp.json', '.mcp.json']) {
    const launch = JSON.parse(readFileSync(join(installedRoot, manifest), 'utf8')).mcpServers.council;
    const entrypoint = launch.args[0].replaceAll('${PLUGIN_ROOT}', installedRoot).replaceAll('${CLAUDE_PLUGIN_ROOT}', installedRoot);
    assert.ok(existsSync(entrypoint), `${manifest} must point to a packaged executable entrypoint.`);
  }
  const dependencyTree = JSON.parse((await runNode([npm, 'ls', '--prefix', consumer, '--all', '--omit=dev', '--json', '--offline'])).stdout);
  assert.deepEqual(Object.keys(dependencyTree.dependencies ?? {}), ['council-ai-plugin']);
  assert.deepEqual(Object.keys(dependencyTree.dependencies['council-ai-plugin'].dependencies ?? {}), []);

  const cli = join(installedRoot, 'dist', 'cli.js');
  const version = JSON.parse((await runNode([cli, '--version'])).stdout);
  assert.equal(version.version, releaseVersion);
  const demo = JSON.parse((await runNode([cli, 'demo'])).stdout);
  assert.equal(demo.backend, 'mock');
  assert.equal(demo.liveAgentCalls, 0);
  assert.equal(demo.results.length, 6);
  assert.ok(demo.results.every(result => result.status === 'completed'));
  if (typeof demo.stateDir === 'string') {
    const demoRoot = dirname(resolve(demo.stateDir));
    if (basename(demoRoot).startsWith('council-demo-')) demoDirectories.push(demoRoot);
  }
  assert.equal(existsSync(join(consumer, '.council')), false, 'The offline demo must keep state outside the consumer project.');

  const connection = JSON.parse((await runNode([cli, 'connection', '--workspace', workspace, '--backend', 'mock'])).stdout).mcpServers.council;
  assert.equal(connection.command, process.execPath);
  assert.ok(connection.args.includes(cli));
  assert.ok(connection.args.includes(workspace));
  const transport = new StdioClientTransport({ command: connection.command, args: connection.args,
    cwd: consumer, env: { ...environment, ...(connection.env ?? {}) }, stderr: 'pipe' });
  transport.stderr?.on('data', () => {});
  const client = new Client({ name: 'council-installed-release-acceptance', version: '1.0.0' });
  let closed = false;
  const close = async () => { if (!closed) { closed = true; await client.close(); } };
  closeClients.push(close);
  await client.connect(transport);
  assert.equal(client.getServerVersion().version, releaseVersion);
  const available = (await client.listTools()).tools.map(tool => tool.name);
  for (const name of ['council_plan', 'council_run', 'council_status', 'council_cancel', 'council_health', 'council_run_page', 'council_backup', 'council_prune', 'council_patches',
    'council_billing', 'council_billing_update', 'council_billing_import', 'council_route_plan', 'council_route_run', 'council_skills']) assert.ok(available.includes(name));
  const call = async (name, input) => {
    const response = await client.callTool({ name, arguments: input });
    assert.notEqual(response.isError, true, `Installed MCP tool ${name} failed: ${response.content?.map(item => item.text ?? '').join(' ')}`);
    return JSON.parse(response.content.find(item => item.type === 'text').text);
  };
  const input = { mode: 'delegate', task: 'Verify the self-contained installed package offline.', agents: ['codex'], permission: 'read',
    agentSettings: { codex: { skills: ['dont-kill-an-ant-with-a-bazooka'] } } };
  assert.equal((await call('council_plan', input)).estimatedCalls, 1);
  const queued = await call('council_run', input);
  const completed = await call('council_status', { id: queued.id, waitMs: 3000, includeResults: true });
  assert.equal(completed.status, 'completed');
  assert.ok(completed.steps[0].result.output.includes('[MOCK'));
  assert.deepEqual(completed.input.agentSettings.codex.skills, ['dont-kill-an-ant-with-a-bazooka']);
  const modes = await client.readResource({ uri: 'council://modes' });
  assert.equal(JSON.parse(modes.contents[0].text).modes.length, 6);
  await close();
  const persisted = JSON.parse((await runNode([cli, 'show', queued.id, '--workspace', workspace, '--backend', 'mock'])).stdout);
  assert.equal(persisted.status, 'completed');
});
