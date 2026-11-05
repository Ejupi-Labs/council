import assert from 'node:assert/strict';
import { execFile } from 'node:child_process';
import { existsSync, mkdirSync, mkdtempSync, readFileSync, readdirSync, realpathSync, rmSync, statSync, writeFileSync } from 'node:fs';
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

function npmProgress(cache) {
  const stages = new Set();
  const directory = join(cache, '_logs');
  if (!existsSync(directory)) return [];
  // Only npm-created files inside this test's cache are inspected. Never expose
  // raw logs, configuration, command arguments, paths or package/user text.
  for (const name of readdirSync(directory).filter(name => name.endsWith('-debug-0.log')).sort().slice(-3)) {
    const path = join(directory, name);
    if (statSync(path).size > 1024 * 1024) continue;
    const contents = readFileSync(path, 'utf8').slice(-32 * 1024);
    for (const line of contents.split(/\r?\n/)) {
      const match = /^\d+ (?:timing|silly|verbose) (idealTree|reify|tarball|extract|build|audit)(?:\b|:)/.exec(line);
      if (match) stages.add(match[1]);
    }
  }
  return [...stages];
}

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
  const withinTemp = relative(realpathSync.native(tmpdir()), target);
  assert.ok(withinTemp && !withinTemp.startsWith('..') && !isAbsolute(withinTemp), 'Release cleanup must stay inside the temporary directory.');
  rmSync(target, { recursive: true, force: true });
}

test('the release tarball installs fully offline and runs its CLI and MCP server without production dependencies', { timeout: 60_000 }, async t => {
  const root = mkdtempSync(join(realpathSync.native(tmpdir()), 'council-release-'));
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
  const packCache = join(root, 'pack npm cache'), installCache = join(root, 'fresh install npm cache');
  const userConfig = join(root, 'empty-user.npmrc'), globalConfig = join(root, 'empty-global.npmrc');
  writeFileSync(userConfig, ''); writeFileSync(globalConfig, '');
  const environment = { ...Object.fromEntries(Object.entries(process.env).filter(([name]) => !/^npm_config_/i.test(name))), COUNCIL_DEPTH: '0', COUNCIL_BACKEND: 'mock' };
  const runNode = async (phase, args, cwd = consumer, cache = installCache) => {
    const start = performance.now();
    const isNpm = args[0] === npm;
    const isolated = isNpm ? [...args, '--cache', cache, '--userconfig', userConfig, '--globalconfig', globalConfig, '--timing', '--offline'] : args;
    try {
      const result = await execute(process.execPath, isolated, {
        cwd, env: environment, windowsHide: true, timeout: 30_000, maxBuffer: 8 * 1024 * 1024, signal: t.signal,
      });
      t.diagnostic(JSON.stringify({ phase, elapsedMs: Math.round(performance.now() - start), status: 'completed' }));
      return result;
    } catch (error) {
      let progress;
      try { progress = isNpm ? npmProgress(cache) : undefined; } catch { progress = ['unavailable']; }
      t.diagnostic(JSON.stringify({ phase, elapsedMs: Math.round(performance.now() - start), timeoutMs: 30_000,
        exitCode: typeof error.code === 'number' ? error.code : null, code: typeof error.code === 'string' && /^[A-Z0-9_]{1,64}$/.test(error.code) ? error.code : undefined,
        killed: error.killed === true, signal: error.signal ?? null, stdoutBytes: Buffer.byteLength(error.stdout ?? ''), stderrBytes: Buffer.byteLength(error.stderr ?? ''), progress }));
      throw new Error(`Release acceptance phase ${phase} failed; see bounded process diagnostics.`);
    }
  };
  const packed = JSON.parse((await runNode('pack', [npm, 'pack', '--ignore-scripts', '--json', '--pack-destination', root], repository, packCache)).stdout);
  assert.equal(packed.length, 1);
  assert.equal(packed[0].version, releaseVersion);
  const packagedPaths = packed[0].files.map(file => file.path.replaceAll('\\', '/'));
  for (const file of ['dist/cli.js', 'plugin.json', 'mcp.json', '.codex-plugin/plugin.json', '.claude-plugin/plugin.json', 'skills/council/SKILL.md',
    'skills/dont-kill-an-ant-with-a-bazooka/SKILL.md', 'skills/council-implementation-review/SKILL.md',
    'skills/council-independent-perspectives/SKILL.md', 'skills/council-advisor/SKILL.md', 'schemas/billing.schema.json', 'schemas/routing.schema.json',
    'skills/council-human-delegate/SKILL.md', 'docs/delegation.md', 'schemas/delegation.schema.json', 'dist/delegation.js', 'dist/dashboard-delegate-ui.js',
    'schemas/private-chat.schema.json', 'dist/private-chats.js', 'dist/private-chat-protocol.js', 'dist/dashboard-private-chats-ui.js', 'docs/private-chats.md',
    'schemas/state-snapshot.schema.json', 'dist/state-snapshot.js', 'dist/sqlite-backup.js', 'dist/cli-rooms.js', 'docs/rooms.md', 'examples/room-fix.json', 'dist/dispatch-quarantine.js', 'dist/dashboard-quarantine-ui.js', 'docs/cleanup-quarantine.md']) {
    assert.ok(packagedPaths.includes(file), `The release must contain ${file}.`);
  }
  assert.ok(packagedPaths.every(file => !/(^|\/)(?:\.council|\.serena|node_modules|tests)(?:\/|$)/.test(file)), 'Local state, dependencies and test fixtures must not enter the release.');
  const tarball = join(root, packed[0].filename);
  // An empty cache plus --offline proves installation cannot depend on a registry fetch.
  assert.equal(existsSync(installCache), false, 'Installation must start with its own genuinely empty cache.');
  await runNode('offline-install', [npm, 'install', '--prefix', consumer, '--global=false', '--ignore-scripts', '--omit=dev', '--offline', '--no-audit', '--no-fund', tarball]);
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
  const dependencyTree = JSON.parse((await runNode('dependencies', [npm, 'ls', '--prefix', consumer, '--all', '--omit=dev', '--json', '--offline'])).stdout);
  assert.deepEqual(Object.keys(dependencyTree.dependencies ?? {}), ['council-ai-plugin']);
  assert.deepEqual(Object.keys(dependencyTree.dependencies['council-ai-plugin'].dependencies ?? {}), []);

  const cli = join(installedRoot, 'dist', 'cli.js');
  const version = JSON.parse((await runNode('version', [cli, '--version'])).stdout);
  assert.equal(version.version, releaseVersion);
  const demo = JSON.parse((await runNode('mock-demo', [cli, 'demo'])).stdout);
  assert.equal(demo.backend, 'mock');
  assert.equal(demo.liveAgentCalls, 0);
  assert.equal(demo.results.length, 6);
  assert.ok(demo.results.every(result => result.status === 'completed'));
  if (typeof demo.stateDir === 'string') {
    const demoRoot = dirname(resolve(demo.stateDir));
    if (basename(demoRoot).startsWith('council-demo-')) demoDirectories.push(demoRoot);
  }
  assert.equal(existsSync(join(consumer, '.council')), false, 'The offline demo must keep state outside the consumer project.');

  const connection = JSON.parse((await runNode('connection', [cli, 'connection', '--workspace', workspace, '--backend', 'mock'])).stdout).mcpServers.council;
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
    'council_billing', 'council_billing_update', 'council_billing_import', 'council_route_plan', 'council_route_run', 'council_skills',
    'council_delegate_status', 'council_delegate_configure', 'council_delegate_report', 'council_delegate_resolve', 'council_delegate_reports', 'council_delegate_changes', 'council_delegate_watch',
    'council_chat_create', 'council_chat_send', 'council_chat_list', 'council_chat_actors', 'council_chat_history', 'council_chat_watch']) assert.ok(available.includes(name));
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
  const persisted = JSON.parse((await runNode('persisted-run', [cli, 'show', queued.id, '--workspace', workspace, '--backend', 'mock'])).stdout);
  assert.equal(persisted.status, 'completed');
});
