#!/usr/bin/env node
import { mkdtempSync, readFileSync, statSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { parseArgs } from 'node:util';
import { fileURLToPath } from 'node:url';
import { initConfig, loadConfig, parseBackend, parseWorkflow, readJsonFile } from './config.js';
import { serveMcp } from './mcp.js';
import { CouncilService } from './service.js';
import { renderRun } from './report.js';
import { VERSION } from './version.js';
import type { RunRecord } from './types.js';

const HELP = `Council — collaboration for coding agents

Usage: council <command> [options]

Commands:
  init           Create project settings without overwriting existing settings
  doctor         Inspect local runtimes; no model calls or login
  connection     Print absolute MCP connection settings for this project
  plan           Preview workflow stages and call bounds without model calls
  run            Execute a workflow; print its ID to stderr and JSON result to stdout
  continue <id>  Start a new task with a previous terminal run as bounded context
  runs           List recent runs
  show <id>      Read a saved run
  events <id>    Read ordered events; --after <cursor> and --limit <count>
  cancel <id>    Request cancellation across Council processes
  recover        Recover work owned by proven-dead processes; never replay tasks
  export <id>    Export a Markdown report; optional --output <path> (no overwrite)
  cleanup <id>   Remove managed worktrees after --reviewed; --discard permits dirty work
  mcp            Serve MCP over stdio
  demo           Run all six modes with local mock agents; zero model calls

Options:
  --workspace <path>     Target project (default: current directory)
  --backend <name>       native | acpx | mock
  --mode <name>          delegate | review | debate | parallel | pipeline | council
  --agents <names>       Comma-separated configured names
  --task <text>          Task brief, or --task-file <path>
  --context-file <path>  Additional project context
  --assignments-file <p> JSON map of independent parallel tasks
  --judge <name>        Synthesis agent
  --model <id>           Explicit model for a single participant, without saving settings
  --rounds <n>           Debate rounds, 1 to 3
  --max-calls <n>        Per-run bound within project limits
  --write               Enable isolated worktrees for requested edits
  --json                JSON output (default; export emits Markdown)
  --version             Print version
  --help                Show help

Read policy is not an OS sandbox. Native agents retain their own accounts and settings.
`;

function print(value: unknown) { process.stdout.write(`${JSON.stringify(value, null, 2)}\n`); }
function textFile(filename: string): string {
  if (statSync(filename).size > 1_048_576) throw new Error('Input file exceeds 1 MiB.');
  return readFileSync(filename, 'utf8').replace(/^\uFEFF/, '');
}

async function main(): Promise<void> {
  const { values, positionals } = parseArgs({ allowPositionals: true, options: {
    workspace: { type: 'string' }, backend: { type: 'string' }, mode: { type: 'string' },
    agents: { type: 'string' }, task: { type: 'string' }, 'task-file': { type: 'string' },
    'context-file': { type: 'string' }, judge: { type: 'string' }, rounds: { type: 'string' },
    'assignments-file': { type: 'string' }, model: { type: 'string' },
    'max-calls': { type: 'string' }, write: { type: 'boolean' }, json: { type: 'boolean' },
    after: { type: 'string' }, limit: { type: 'string' }, output: { type: 'string' },
    reviewed: { type: 'boolean' }, discard: { type: 'boolean' },
    help: { type: 'boolean' }, version: { type: 'boolean' },
  } });
  if (values.version) { print({ name: 'council', version: VERSION }); return; }
  const command = positionals[0] ?? 'help';
  if (values.help || command === 'help') { process.stdout.write(HELP); return; }
  const commands = ['init', 'doctor', 'connection', 'plan', 'run', 'continue', 'runs', 'show', 'events', 'cancel', 'recover', 'export', 'cleanup', 'mcp', 'demo'];
  if (!commands.includes(command)) throw new Error(`Unknown command: ${command}`);
  const takesId = ['continue', 'show', 'events', 'cancel', 'export', 'cleanup'].includes(command);
  if (positionals.length !== (takesId ? 2 : 1)) throw new Error(takesId ? `${command} requires exactly one run ID.` : `${command} accepts no positional arguments.`);
  const id = positionals[1];
  if (takesId && !/^[a-f0-9-]{36}$/i.test(id!)) throw new Error('Supply a valid Council run ID.');
  if (values.task !== undefined && values['task-file'] !== undefined) throw new Error('Choose --task or --task-file, not both.');
  const common = ['workspace', 'backend', 'json', 'help', 'version'];
  const workflowOptions = ['mode', 'agents', 'task', 'task-file', 'context-file', 'assignments-file', 'judge', 'rounds', 'max-calls', 'write', 'model'];
  const permitted = new Set([...common, ...(['plan', 'run', 'continue'].includes(command) ? workflowOptions : []), ...(command === 'events' ? ['after', 'limit'] : []), ...(command === 'export' ? ['output'] : []), ...(command === 'cleanup' ? ['reviewed', 'discard'] : [])]);
  for (const option of Object.keys(values)) if (!permitted.has(option)) throw new Error(`--${option} is not supported by ${command}.`);
  const backend = values.backend === undefined ? undefined : parseBackend(values.backend);
  const workspace = resolve(values.workspace ?? process.env.COUNCIL_WORKSPACE ?? process.cwd());
  if (command === 'init') { print({ created: initConfig(workspace, backend) }); return; }
  if (command === 'connection') {
    loadConfig(workspace);
    print({ mcpServers: { council: { command: process.execPath, args: [fileURLToPath(import.meta.url), 'mcp', '--workspace', workspace], ...(backend ? { env: { COUNCIL_BACKEND: backend } } : {}) } } });
    return;
  }
  const config = command === 'demo'
    ? loadConfig(mkdtempSync(join(tmpdir(), 'council-demo-')), { backend: 'mock' })
    : loadConfig(workspace, { ...(backend ? { backend } : {}), ...(process.env.COUNCIL_BACKEND && !backend ? { backend: parseBackend(process.env.COUNCIL_BACKEND) } : {}) });
  const service = new CouncilService(config);
  if (command === 'mcp') {
    const server = await serveMcp(service);
    for (const signal of ['SIGINT', 'SIGTERM'] as const) process.once(signal, () => { process.exitCode = signal === 'SIGINT' ? 130 : 143; void server.close().catch(fail); });
    return;
  }
  try {
    if (command === 'doctor') print({ version: VERSION, node: process.version, backend: config.backend, workspace: config.workspaceRoot, concurrencyScope: 'workspace', agents: await service.runner.doctor(), recoveredRuns: service.recoveredRuns.map(run => run.id), note: 'Prerequisite availability does not prove authentication or model access.' });
    else if (command === 'runs') print(service.store.listRuns());
    else if (command === 'show' || command === 'export') {
      const run = service.store.getRun(id!);
      if (!run) throw new Error('Run not found.');
      if (command === 'show') print(run);
      else {
        const report = renderRun(run);
        if (values.output) { const path = resolve(workspace, values.output); writeFileSync(path, report, { flag: 'wx' }); print({ exported: path }); }
        else process.stdout.write(report);
      }
    } else if (command === 'events') {
      const after = Number(values.after ?? 0), limit = Number(values.limit ?? 100);
      if (!Number.isSafeInteger(after) || after < 0 || !Number.isInteger(limit) || limit < 1 || limit > 1000) throw new Error('Event cursor must be non-negative and limit must be from 1 to 1000.');
      print(service.store.events(id!, after).slice(0, limit));
    } else if (command === 'recover') print({ recovered: [...service.recoveredRuns, ...service.store.recoverOrphans()].map(run => ({ id: run.id, status: run.status, error: run.error })) });
    else if (command === 'cancel') {
      const previous = service.store.getRun(id!);
      const run = service.cancel(id!);
      print({ cancellationRequested: !!previous && ['queued', 'running'].includes(previous.status) && ['queued', 'running'].includes(run.status), run });
    } else if (command === 'cleanup') {
      if (!values.reviewed) throw new Error('Review the worktrees and patches, then provide --reviewed to request cleanup.');
      print(await service.cleanup(id!, { reviewed: true, discardChanges: values.discard === true }));
    } else if (command === 'demo') await demo(service);
    else {
      const previous = command === 'continue' ? service.store.getRun(id!) : undefined;
      if (command === 'continue' && !previous) throw new Error('Previous run not found.');
      const mode = values.mode ?? previous?.input.mode ?? 'delegate';
      const input = parseWorkflow({
        mode,
        task: values.task ?? (values['task-file'] ? textFile(resolve(workspace, values['task-file'])) : ''),
        agents: values.agents ? values.agents.split(',').map(agent => agent.trim()) : previous?.input.agents ?? (mode === 'delegate' ? ['codex'] : ['codex', 'claude']),
        judge: values.judge ?? previous?.input.judge,
        rounds: values.rounds === undefined ? previous?.input.rounds : Number(values.rounds),
        permission: values.write ? 'write' : 'read',
        context: values['context-file'] ? textFile(resolve(workspace, values['context-file'])) : undefined,
        maxCalls: values['max-calls'] === undefined ? undefined : Number(values['max-calls']),
        assignments: values['assignments-file'] ? readJsonFile(resolve(workspace, values['assignments-file'])) : undefined,
      });
      if (values.model) {
        if (input.agents.length !== 1 || (input.judge && input.judge !== input.agents[0])) throw new Error('--model requires one participant; configure per-agent models in project settings for a team.');
        const agent = input.agents[0]!;
        if (!config.agents[agent]) throw new Error('The selected agent is not configured.');
        if (values.model.length > 200 || !values.model.trim()) throw new Error('Model must contain from 1 to 200 characters.');
        config.agents[agent] = { ...config.agents[agent]!, model: values.model };
      }
      if (command === 'plan') print(service.engine.plan(input));
      else {
        const interrupt = (signal: 'SIGINT' | 'SIGTERM') => { process.exitCode = signal === 'SIGINT' ? 130 : 143; void service.close().catch(fail); };
        const sigint = () => interrupt('SIGINT'), sigterm = () => interrupt('SIGTERM');
        process.once('SIGINT', sigint); process.once('SIGTERM', sigterm);
        try {
          const queued = command === 'continue' ? service.continue(id!, input) : service.start(input);
          process.stderr.write(`${JSON.stringify({ event: 'run.started', id: queued.id, mode: input.mode })}\n`);
          const run = await service.completion(queued.id);
          print(run);
          if (run.status !== 'completed' && !process.exitCode) process.exitCode = 1;
        } finally { process.removeListener('SIGINT', sigint); process.removeListener('SIGTERM', sigterm); }
      }
    }
  } finally { await service.close(); }
}

async function demo(service: CouncilService): Promise<void> {
  const results = [];
  for (const mode of ['delegate', 'review', 'debate', 'parallel', 'pipeline', 'council'] as const) {
    const agents = mode === 'delegate' ? ['codex'] : ['codex', 'claude', 'junie'];
    const run = await service.run({ mode, agents, task: 'Design a small searchable project notes feature. State evidence, risks and an implementation plan.', permission: 'read', judge: mode === 'delegate' || mode === 'pipeline' || mode === 'review' ? undefined : 'deepseek-harness', rounds: 1 });
    results.push({ mode, id: run.id, status: run.status, calls: run.steps.length, output: run.output });
  }
  print({ version: VERSION, backend: 'mock', liveAgentCalls: 0, stateDir: service.config.stateDir, results });
  if (results.some(run => run.status !== 'completed')) process.exitCode = 1;
}

function fail(error: unknown): void { process.stderr.write(`Council: ${error instanceof Error ? error.message : 'Unexpected failure.'}\n`); process.exitCode = 1; }
main().catch(fail);
