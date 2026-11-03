#!/usr/bin/env node
import { mkdtempSync, readFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { parseArgs } from 'node:util';
import { fileURLToPath } from 'node:url';
import { configSchema, initConfig, loadConfig, parseWorkflow } from './config.js';
import { serveMcp } from './mcp.js';
import { CouncilService } from './service.js';
import type { CouncilConfig } from './types.js';

const HELP = `Council — collaboration for coding agents

Usage: council <command> [options]

Commands:
  init         Create council.config.json without overwriting an existing file
  doctor       Inspect local agent prerequisites; makes no model calls
  connection   Print portable MCP connection settings for this workspace
  plan         Preview a workflow without invoking agents
  run          Execute a workflow and print its complete result
  runs         List recent runs
  show <id>    Read a saved run
  cancel <id>  Request cancellation, including across server processes
  mcp          Run the stdio MCP server
  demo         Execute all six modes with deterministic local mock agents

Options:
  --workspace <path>   Project directory (default: current directory)
  --backend <name>     acpx or mock (default: project configuration)
  --mode <name>        delegate | review | debate | parallel | pipeline | council
  --agents <names>     Comma-separated configured profiles
  --task <text>        Task brief; alternatively --task-file <path>
  --context-file <p>   Additional context to hand to participants
  --assignments-file  JSON map of agent names to independent parallel subtasks
  --judge <name>      Synthesis agent
  --rounds <number>    Debate rounds, from 1 to 3
  --max-calls <n>      Per-run bound, within the project limit
  --write             Allow edits in isolated Git worktrees; never auto-merge
  --json              JSON output (also the default for every command)
  --help              Show this help

Read policy denies tool approval requests; it is not an operating-system sandbox.
Real agent calls use each agent's own configured account and limits.
`;

function print(value: unknown) { process.stdout.write(`${JSON.stringify(value, null, 2)}\n`); }

async function main(): Promise<void> {
  const { values, positionals } = parseArgs({ allowPositionals: true, options: {
    workspace: { type: 'string' }, backend: { type: 'string' }, mode: { type: 'string' },
    agents: { type: 'string' }, task: { type: 'string' }, 'task-file': { type: 'string' },
    'context-file': { type: 'string' }, judge: { type: 'string' }, rounds: { type: 'string' },
    'assignments-file': { type: 'string' },
    'max-calls': { type: 'string' }, write: { type: 'boolean' }, json: { type: 'boolean' },
    help: { type: 'boolean' }, version: { type: 'boolean' },
  } });
  if (values.version) { print({ name: 'council', version: '0.1.0' }); return; }
  const command = positionals[0] ?? 'help';
  if (values.help || command === 'help') { process.stdout.write(HELP); return; }
  if (!['init', 'doctor', 'connection', 'plan', 'run', 'runs', 'show', 'cancel', 'mcp', 'demo'].includes(command)) throw new Error(`Unknown command: ${command}`);
  const backend = values.backend === undefined ? undefined : configSchema.shape.backend.parse(values.backend);
  const workspace = resolve(values.workspace ?? process.env.COUNCIL_WORKSPACE ?? process.cwd());
  if (command === 'init') { print({ created: initConfig(workspace, backend) }); return; }
  if (command === 'connection') {
    print({ mcpServers: { council: { command: process.execPath, args: [fileURLToPath(import.meta.url), 'mcp', '--workspace', workspace], ...(backend ? { env: { COUNCIL_BACKEND: backend } } : {}) } } });
    return;
  }
  const config = command === 'demo'
    ? loadConfig(mkdtempSync(join(tmpdir(), 'council-demo-')), { backend: 'mock' })
    : loadConfig(workspace, { ...(backend ? { backend } : {}), ...(process.env.COUNCIL_BACKEND && !backend ? { backend: configSchema.shape.backend.parse(process.env.COUNCIL_BACKEND) } : {}) });
  const service = new CouncilService(config);
  if (command === 'mcp') {
    const server = await serveMcp(service);
    const stop = async () => { await service.close(); await server.close(); };
    process.once('SIGINT', () => { void stop(); });
    process.once('SIGTERM', () => { void stop(); });
    return;
  }
  try {
    if (command === 'doctor') {
      print({ node: process.version, backend: config.backend, workspace: config.workspaceRoot, agents: await service.runner.doctor(), note: 'Availability checks do not prove authentication or make a model call.' });
    } else if (command === 'runs') print(service.store.listRuns());
    else if (command === 'show') {
      const run = service.store.getRun(positionals[1] ?? '');
      if (!run) throw new Error('Run not found. Supply the ID printed by Council.');
      print(run);
    } else if (command === 'cancel') print({ cancellationRequested: true, run: service.cancel(positionals[1] ?? '') });
    else if (command === 'demo') await demo(service);
    else {
      const input = parseWorkflow({
        mode: values.mode ?? 'delegate',
        task: values.task ?? (values['task-file'] ? readFileSync(resolve(workspace, values['task-file']), 'utf8') : ''),
        agents: (values.agents ?? (values.mode && values.mode !== 'delegate' ? 'codex,claude' : 'codex')).split(',').map(agent => agent.trim()),
        judge: values.judge, rounds: values.rounds === undefined ? undefined : Number(values.rounds),
        permission: values.write ? 'write' : 'read',
        context: values['context-file'] ? readFileSync(resolve(workspace, values['context-file']), 'utf8') : undefined,
        maxCalls: values['max-calls'] === undefined ? undefined : Number(values['max-calls']),
        assignments: values['assignments-file'] ? JSON.parse(readFileSync(resolve(workspace, values['assignments-file']), 'utf8')) : undefined,
      });
      if (command === 'plan') print(service.engine.plan(input));
      else {
        const interrupt = () => { void service.close(); };
        process.once('SIGINT', interrupt);
        try {
          const run = await service.run(input);
          print(run);
          if (run.status !== 'completed') process.exitCode = 1;
        } finally { process.removeListener('SIGINT', interrupt); }
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
  print({ backend: 'mock', liveAgentCalls: 0, stateDir: service.config.stateDir, results });
  if (results.some(run => run.status !== 'completed')) process.exitCode = 1;
}

main().catch(error => {
  process.stderr.write(`Council: ${error instanceof Error ? error.message : 'Unexpected failure.'}\n`);
  process.exitCode = 1;
});
