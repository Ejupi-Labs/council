#!/usr/bin/env node
import { existsSync, mkdtempSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { parseArgs } from 'node:util';
import { fileURLToPath } from 'node:url';
import { initConfig, loadConfig, parseBackend, parseWorkflow, readJsonFile } from './config.js';
import { serveMcp } from './mcp.js';
import { CouncilService } from './service.js';
import { CouncilStore } from './store.js';
import { renderRun } from './report.js';
import { VERSION } from './version.js';
import { createRunner } from './runner.js';
import { runtimeReadiness } from './sqlite-runtime.js';
import type { RunRecord } from './types.js';
import { collaborationSkills } from './skills.js';
import { modelMapView } from './model-map.js';
import type { ModelMapQuery } from './model-map-types.js';
import type { RoutingInput } from './routing-types.js';
import { parseRoutingInput } from './routing-schema.js';
import { billingInputDefinition, billingImportDefinition } from './billing-schema.js';
import { roomDefinition, roomMessageDefinition, roomTaskDefinition } from './rooms-schema.js';
import { parseSchema } from './schema.js';
import { readUtf8File } from './utf8-file.js';
import { verifyStateSnapshot } from './state-snapshot.js';
import { ROOM_COMMANDS, ROOM_ID_COMMANDS, roomCommandOptions, runRoomCommand, validateRoomCommand } from './cli-rooms.js';

const HELP = `Council — collaboration for coding agents

Usage: council <command> [options]

Commands:
  init           Create project settings without overwriting existing settings
  doctor         Inspect local runtimes; no model calls or login
  health         Inspect state sizes, limits and runtime health; optional --integrity
  quarantine     Inspect open cleanup incidents with optional --limit and --cursor
  quarantine-ack Clear the reviewed dispatch block offline with --revision and --reviewed
  backup         Back up workflow state only with --output <new-file>
  snapshot       Back up all collaboration databases with --output <new-directory>
  verify-snapshot <directory>  Check a completed all-store snapshot
  rooms          List recent project rooms; --limit <count>
  room-create    Create a room from --input-file <room.json>, without model calls
  room-show <id> Read a room and its current full task evidence
  room-history <id> Read complete messages with --limit and --cursor
  room-message <id> Add guidance from --input-file <message.json>
  room-task <id> Add an explicit task from --input-file <task.json>
  room-discuss <id> Discuss the plan; wait for stable state before exiting
  room-execute <id> Execute an approved plan and wait for its outcome
  room-pause <id> Pause admission and wait for admitted work to drain
  room-resume <id> Resume a saved checkpoint and wait for stable state
  room-cancel <id> Request durable cancellation without replay
  room-wait <id> Wait for stable state; optional --wait-ms <budget>
  room-recover <id> Settle proven-dead ownership without replaying tasks
  prune          Preview old run retention with --before <timestamp>; --execute deletes
  connection     Print absolute MCP connection settings for this project
  plan           Preview workflow stages and call bounds without model calls
  run            Execute a workflow; print its ID to stderr and JSON result to stdout
  continue <id>  Start a new task with complete evidence from a previous terminal run
  runs           List recent runs
  show <id>      Read a saved run
  events <id>    Read ordered events; --after <cursor> and --limit <count>
  cancel <id>    Request cancellation across Council processes
  recover        Recover work owned by proven-dead processes; never replay tasks
  export <id>    Export a Markdown report; optional --output <path> (no overwrite)
  patches <id>   Refresh patch artifacts for terminal work without calling agents
  cleanup <id>   Remove managed worktrees after --reviewed; --discard permits dirty work
  mcp            Serve MCP over stdio
  ui             Serve the local project room dashboard; prints its private loopback URL
  wait <id>      Block inside Council until a run settles; no model-driven polling
  advise         Ask a configured advisor with --advisor and --question
  skills         List assignable collaboration skills; no model calls
  model-map      Read model evidence and task priorities; optional --task/--model/--role/--harness
  billing        Read local billing and limits; optionally --agent <alias>
  billing-set    Save manual metadata with --agent and --input-file
  billing-import Import a reported observation with --agent and --input-file
  route-plan     Preview a right-sized team with --task and --task-class
  route          Start a routed workflow and wait for its result
  demo           Run all six modes with local mock agents; zero model calls

Options:
  --workspace <path>     Target project (default: current directory)
  --backend <name>       native | acpx | mock
  --mode <name>          delegate | review | debate | parallel | pipeline | council
  --agents <names>       Comma-separated configured names
  --task <text>          Task brief, or exact task category for model-map
  --task-file <path>     Read a workflow task brief from a file
  --context-file <path>  Additional project context
  --assignments-file <p> JSON map of independent parallel tasks
  --judge <name>        Synthesis agent
  --model <id>           Explicit participant model, or exact model-map filter; no saved changes
  --role <name>          Model-map role: lead | worker | reviewer | advisor | economy
  --harness <name>       Exact model-map harness filter
  --full-catalog         Include the full evidence catalog despite model-map filters
  --effort <level>       Explicit reasoning effort for one participant (harness support varies)
  --advisor <name>       Advisor alias for a single worker, or the advise command
  --question <text>      Question for the advise command
  --requester <name>     Requester label for advise (default: host)
  --wait-ms <n>          Budget for wait, 0 to 600000 ms (default: 600000)
  --port <n>             Local UI port (default: an available loopback port)
  --rounds <n>           Debate rounds within your configured call budget
  --max-calls <n>        Per-run bound within project limits
  --agent <alias>       Agent for billing inspection or updates
  --input-file <path>    Room, message, task, billing or routing JSON
  --revision <n>         Expected billing revision or exact quarantine generation
  --task-class <name>   routine | standard | complex (routing)
  --strategy <name>     economy | balanced | quality (routing default: economy)
  --collaboration <name> direct | review | parallel | council (default: direct)
  --requires <names>    Comma-separated required configured capabilities
  --participants <n>    Team size for routed collaboration
  --allow-escalation    Explicitly allow a larger model tier when necessary
  --skills <ids>        Comma-separated bundled collaboration skill IDs
  --write               Enable isolated worktrees for requested edits
  --json                JSON output (default; export emits Markdown)
  --summary             Use lightweight summaries for runs/events
  --limit <n>            Maximum results; valid range depends on the command
  --cursor <token>       Continue a runs --summary, room-history or quarantine page
  --after <n>            Read events after this sequence number (default: 0)
  --output <path>        New report file, backup file or snapshot directory
  --integrity            Check every present collaboration database with health
  --before <timestamp>   Prune cutoff with an explicit timezone
  --keep-latest <n>      Preserve newest runs during pruning (default: 100)
  --execute              Execute the reviewed prune preview
  --reviewed             Confirm artifact review or offline quarantine inspection
  --discard              Permit cleanup to discard reviewed dirty worktree changes
  --version             Print version
  --help                Show help

Read policy is not an OS sandbox. Native agents retain their own accounts and settings.
`;

function print(value: unknown) { process.stdout.write(`${JSON.stringify(value, null, 2)}\n`); }
function textFile(filename: string): string {
  return readUtf8File(filename);
}

async function main(): Promise<void> {
  const { values, positionals } = parseArgs({ allowPositionals: true, options: {
    workspace: { type: 'string' }, backend: { type: 'string' }, mode: { type: 'string' },
    agents: { type: 'string' }, task: { type: 'string' }, 'task-file': { type: 'string' },
    'context-file': { type: 'string' }, judge: { type: 'string' }, rounds: { type: 'string' },
    'assignments-file': { type: 'string' }, model: { type: 'string' }, role: { type: 'string' }, harness: { type: 'string' }, 'full-catalog': { type: 'boolean' }, effort: { type: 'string' }, advisor: { type: 'string' },
    question: { type: 'string' }, requester: { type: 'string' }, 'wait-ms': { type: 'string' }, port: { type: 'string' },
    'max-calls': { type: 'string' }, write: { type: 'boolean' }, json: { type: 'boolean' },
    agent: { type: 'string' }, 'input-file': { type: 'string' }, revision: { type: 'string' },
    'task-class': { type: 'string' }, strategy: { type: 'string' }, collaboration: { type: 'string' },
    requires: { type: 'string' }, participants: { type: 'string' }, 'allow-escalation': { type: 'boolean' }, skills: { type: 'string' },
    after: { type: 'string' }, limit: { type: 'string' }, output: { type: 'string' },
    reviewed: { type: 'boolean' }, discard: { type: 'boolean' },
    integrity: { type: 'boolean' }, summary: { type: 'boolean' }, cursor: { type: 'string' },
    before: { type: 'string' }, 'keep-latest': { type: 'string' }, execute: { type: 'boolean' },
    help: { type: 'boolean' }, version: { type: 'boolean' },
  } });
  if (values.version) { print({ name: 'council', version: VERSION }); return; }
  const command = positionals[0] ?? 'help';
  if (values.help || command === 'help') { process.stdout.write(HELP); return; }
  const commands = ['init', 'doctor', 'health', 'quarantine', 'quarantine-ack', 'backup', 'snapshot', 'verify-snapshot', 'prune', 'connection', 'plan', 'run', 'continue', 'runs', 'show', 'events', 'cancel', 'recover', 'export', 'patches', 'cleanup', 'mcp', 'demo', 'ui', 'wait', 'advise', 'skills', 'model-map', 'billing', 'billing-set', 'billing-import', 'route-plan', 'route', ...ROOM_COMMANDS];
  if (!commands.includes(command)) throw new Error(`Unknown command: ${command}`);
  const roomCommand = (ROOM_COMMANDS as readonly string[]).includes(command);
  const takesId = ['continue', 'show', 'events', 'cancel', 'export', 'patches', 'cleanup', 'wait', ...ROOM_ID_COMMANDS].includes(command);
  const takesDirectory = command === 'verify-snapshot';
  if (positionals.length !== (takesId || takesDirectory ? 2 : 1)) throw new Error(takesId ? `${command} requires exactly one ${roomCommand ? 'room' : 'run'} ID.` : takesDirectory ? 'verify-snapshot requires exactly one snapshot directory.' : `${command} accepts no positional arguments.`);
  const id = positionals[1];
  if (takesId && !/^[a-f0-9-]{36}$/i.test(id!)) throw new Error(`Supply a valid Council ${roomCommand ? 'room' : 'run'} ID.`);
  if (values.task !== undefined && values['task-file'] !== undefined) throw new Error('Choose --task or --task-file, not both.');
  if (values.agent !== undefined && !values.agent.trim()) throw new Error('--agent must contain a configured alias.');
  for (const option of ['task-file', 'context-file', 'assignments-file', 'input-file'] as const) {
    if (values[option] === '') throw new Error(`--${option} must contain a filename.`);
  }
  const common = ['workspace', 'backend', 'json', 'help', 'version'];
  const workflowOptions = ['mode', 'agents', 'task', 'task-file', 'context-file', 'assignments-file', 'judge', 'rounds', 'max-calls', 'write', 'model', 'effort', 'advisor', 'skills'];
  const permitted = new Set([...common, ...(['plan', 'run', 'continue'].includes(command) ? workflowOptions : []), ...(command === 'events' ? ['after', 'limit', 'summary'] : []), ...(command === 'runs' ? ['limit', 'summary', 'cursor'] : []), ...(command === 'cancel' ? ['summary'] : []), ...(command === 'health' ? ['integrity'] : []), ...(command === 'prune' ? ['before', 'keep-latest', 'execute'] : []), ...(['export', 'backup', 'snapshot'].includes(command) ? ['output'] : []), ...(command === 'cleanup' ? ['reviewed', 'discard'] : [])]);
  for (const option of command === 'ui' ? ['port'] : command === 'wait' ? ['wait-ms'] : command === 'advise' ? ['advisor', 'requester', 'question', 'model', 'effort'] : []) permitted.add(option);
  if (command === 'quarantine') for (const option of ['limit', 'cursor']) permitted.add(option);
  if (command === 'quarantine-ack') for (const option of ['revision', 'reviewed']) permitted.add(option);
  if (command === 'billing') permitted.add('agent');
  if (command === 'model-map') for (const option of ['task', 'model', 'role', 'harness', 'full-catalog']) permitted.add(option);
  if (['billing-set', 'billing-import'].includes(command)) for (const option of ['agent', 'input-file', 'revision']) permitted.add(option);
  const routeOptions = ['input-file', 'task', 'task-file', 'context-file', 'agents', 'model', 'effort', 'skills', 'task-class', 'strategy', 'collaboration', 'requires', 'participants', 'allow-escalation', 'rounds', 'max-calls', 'write'];
  if (['route', 'route-plan'].includes(command)) for (const option of routeOptions) permitted.add(option);
  if (roomCommand) for (const option of roomCommandOptions(command)) permitted.add(option);
  for (const option of Object.keys(values)) if (!permitted.has(option)) throw new Error(`--${option} is not supported by ${command}.`);
  const generation = (value: string | undefined, label: string): string => {
    if (value === undefined || !/^(?:0|[1-9][0-9]{0,18})$/.test(value) || BigInt(value) > 9223372036854775807n) throw new Error(`${label} must be a canonical decimal generation from 0 to 9223372036854775807.`);
    return value;
  };
  if (command === 'quarantine' && values.cursor !== undefined) generation(values.cursor, '--cursor');
  if (command === 'quarantine' && values.limit !== undefined && (!/^[1-9][0-9]*$/.test(values.limit) || !Number.isSafeInteger(Number(values.limit)) || Number(values.limit) > 100)) throw new Error('--limit must be an integer from 1 to 100.');
  if (command === 'quarantine-ack') {
    if (values.reviewed !== true) throw new Error('Stop every Council host, inspect OS processes and retained worktrees, then provide --reviewed to acknowledge the dispatch block.');
    generation(values.revision, '--revision');
  }
  if (command === 'backup' && !values.output) throw new Error('Backup requires --output with a new destination filename.');
  if (command === 'snapshot' && !values.output) throw new Error('Snapshot requires --output with a new destination directory.');
  if (roomCommand) validateRoomCommand(command, values);
  if (command === 'prune' && !values.before) throw new Error('Prune requires --before with an explicit cutoff timestamp.');
  if (command === 'runs' && values.cursor && !values.summary) throw new Error('--cursor requires runs --summary.');
  const backend = values.backend === undefined ? undefined : parseBackend(values.backend);
  const workspace = resolve(values.workspace ?? process.env.COUNCIL_WORKSPACE ?? process.cwd());
  if (command === 'verify-snapshot') {
    const result = await verifyStateSnapshot(resolve(workspace, positionals[1]!));
    print(result); if (!result.valid) process.exitCode = 1; return;
  }
  if (command === 'skills') { print({ skills: collaborationSkills() }); return; }
  if (command === 'init') { print({ created: initConfig(workspace, backend) }); return; }
  if (command === 'connection') {
    loadConfig(workspace);
    print({ mcpServers: { council: { command: process.execPath, args: [fileURLToPath(import.meta.url), 'mcp', '--workspace', workspace], ...(backend ? { env: { COUNCIL_BACKEND: backend } } : {}) } } });
    return;
  }
  const config = command === 'demo'
    ? loadConfig(mkdtempSync(join(tmpdir(), 'council-demo-')), { backend: 'mock' })
    : loadConfig(workspace, { ...(backend ? { backend } : {}), ...(process.env.COUNCIL_BACKEND && !backend ? { backend: parseBackend(process.env.COUNCIL_BACKEND) } : {}) });
  if (command === 'doctor') {
    const runtime = runtimeReadiness();
    print({ version: VERSION, node: process.version, runtime, backend: config.backend, workspace: config.workspaceRoot,
      concurrencyScope: 'workspace', agents: await createRunner(config).doctor(), recoveredRuns: [],
      note: 'Offline inspection does not open state or recover runs. Prerequisite availability does not prove authentication, sandbox command startup, or model access.' });
    if (!runtime.supported) process.exitCode = 1;
    return;
  }
  if (command === 'model-map') {
    print(modelMapView(config, { ...(values.task !== undefined ? { task: values.task } : {}),
      ...(values.model !== undefined ? { model: values.model } : {}), ...(values.role !== undefined ? { role: values.role as ModelMapQuery['role'] } : {}),
      ...(values.harness !== undefined ? { harness: values.harness } : {}), ...(values['full-catalog'] !== undefined ? { includeFullCatalog: values['full-catalog'] } : {}) }));
    return;
  }
  if (command === 'quarantine' || command === 'quarantine-ack') {
    if (!existsSync(join(config.stateDir, 'state.db'))) {
      if (command === 'quarantine-ack') throw new Error('There is no Council state database to acknowledge. Inspect with quarantine first.');
      print({ blocked: false, currentRevision: '0', openCount: 0, incidents: [], nextCursor: null }); return;
    }
    // Offline inspection/acknowledgement never registers a live Council host.
    const store = new CouncilStore(config.stateDir);
    try {
      print(command === 'quarantine' ? store.dispatchQuarantinePage({ ...(values.limit !== undefined ? { limit: Number(values.limit) } : {}), ...(values.cursor !== undefined ? { cursor: values.cursor } : {}) })
        : store.acknowledgeDispatchQuarantine({ expectedRevision: values.revision!, reviewed: true }));
    } finally { store.close(); }
    return;
  }
  if ((command === 'billing-set' || command === 'billing-import') && (!values.agent?.trim() || !values['input-file'])) throw new Error(`${command} requires --agent and --input-file.`);
  const billingRevision = (command === 'billing-set' || command === 'billing-import') && values.revision !== undefined ? Number(values.revision) : undefined;
  if (billingRevision !== undefined && (!Number.isSafeInteger(billingRevision) || billingRevision < 0)) throw new Error('Expected billing revision must be a nonnegative safe integer.');
  if ((command === 'route' || command === 'route-plan') && values['input-file'] && routeOptions.some(option => option !== 'input-file' && Object.hasOwn(values, option))) throw new Error('Choose a routing --input-file or routing flags, not both.');
  // Read supplied files once before opening state or registering a Council host.
  const fileInputs = new Map<string, unknown>();
  for (const option of ['task-file', 'context-file', 'assignments-file', 'input-file'] as const) {
    const filename = values[option];
    if (filename !== undefined) fileInputs.set(option, option === 'task-file' || option === 'context-file'
      ? textFile(resolve(workspace, filename)) : readJsonFile(resolve(workspace, filename), Infinity));
  }
  const task = values.task ?? (fileInputs.get('task-file') as string | undefined) ?? '';
  const context = fileInputs.get('context-file') as string | undefined;
  const requestInput = fileInputs.get('input-file');
  if (command === 'billing-set' || command === 'billing-import') {
    parseSchema(command === 'billing-set' ? billingInputDefinition : billingImportDefinition, requestInput);
  }
  if (command === 'room-create') parseSchema(roomDefinition, requestInput);
  if (command === 'room-message') parseSchema(roomMessageDefinition, requestInput, 'message');
  if (command === 'room-task') parseSchema(roomTaskDefinition, requestInput);
  let routingInput: RoutingInput | undefined;
  if (command === 'route' || command === 'route-plan') {
    if (values['input-file']) {
      routingInput = parseRoutingInput(requestInput);
    } else {
      const agents = values.agents?.split(',').map(alias => alias.trim());
      if ((values.model !== undefined || values.effort !== undefined) && (!agents || agents.length !== 1)) throw new Error('Explicit routing model/effort requires one alias with --agents.');
      routingInput = parseRoutingInput({ task,
        taskClass: values['task-class'], strategy: values.strategy, collaboration: values.collaboration, agents, context,
        permission: values.write ? 'write' : 'read', requiredCapabilities: values.requires?.split(',').map(value => value.trim()),
        participants: values.participants === undefined ? undefined : Number(values.participants),
        rounds: values.rounds === undefined ? undefined : Number(values.rounds), maxCalls: values['max-calls'] === undefined ? undefined : Number(values['max-calls']),
        allowEscalation: values['allow-escalation'], skills: values.skills?.split(',').map(value => value.trim()),
        ...((values.model !== undefined || values.effort !== undefined) ? { modelSettings: { [agents![0]!]: {
          ...(values.model !== undefined ? { model: values.model } : {}), ...(values.effort !== undefined ? { effort: values.effort } : {}),
        } } } : {}),
      });
    }
  }
  const workflowInput = (previous?: RunRecord) => {
    const mode = values.mode ?? previous?.input.mode ?? 'delegate';
    const input = parseWorkflow({ mode, task,
      agents: values.agents !== undefined ? values.agents.split(',').map(agent => agent.trim()) : previous?.input.agents ?? (mode === 'delegate' ? ['codex'] : ['codex', 'claude']),
      judge: values.judge ?? previous?.input.judge,
      rounds: values.rounds === undefined ? previous?.input.rounds : Number(values.rounds),
      permission: values.write ? 'write' : 'read', context,
      maxCalls: values['max-calls'] === undefined ? undefined : Number(values['max-calls']),
      assignments: fileInputs.get('assignments-file'),
    });
    if (values.model !== undefined || values.effort !== undefined) {
      if (input.agents.length !== 1 || (input.judge && input.judge !== input.agents[0])) throw new Error('--model requires one participant; configure per-agent models in project settings for a team.');
      const agent = input.agents[0]!;
      if (!config.agents[agent]) throw new Error('The selected agent is not configured.');
      if (values.model !== undefined && (values.model.length > 200 || !values.model.trim())) throw new Error('Model must contain from 1 to 200 characters.');
      input.agentSettings = { [agent]: { ...(values.model !== undefined ? { model: values.model } : {}), ...(values.effort !== undefined ? { effort: values.effort as any } : {}) } };
    }
    if (values.advisor !== undefined) {
      if (input.agents.length !== 1 || !values.advisor.trim()) throw new Error('--advisor requires one participant and a configured advisor alias.');
      input.advisors = { [input.agents[0]!]: { agent: values.advisor, maxRequests: 1 } };
    }
    if (values.skills !== undefined) for (const alias of new Set([...input.agents, ...(input.judge ? [input.judge] : [])])) {
      input.agentSettings = { ...input.agentSettings, [alias]: { ...input.agentSettings?.[alias], skills: values.skills.split(',').map(value => value.trim()) } };
    }
    return parseWorkflow(input);
  };
  const preparedWorkflow = command === 'plan' || command === 'run' ? workflowInput() : undefined;
  const service = await CouncilService.open(config);
  if (command === 'ui') {
    const port = values.port === undefined ? 0 : Number(values.port);
    if (!Number.isInteger(port) || port < 0 || port > 65535) { await service.close(); throw new Error('UI port must be from 0 to 65535.'); }
    try {
      const dashboard = await service.openDashboard(port);
      print({ ...dashboard, workspace: config.workspaceRoot, backend: config.backend });
      const stop = () => { void service.close().catch(fail); };
      process.once('SIGINT', stop); process.once('SIGTERM', stop);
    } catch (error) { await service.close(); throw error; }
    return;
  }
  if (command === 'mcp') {
    const server = await serveMcp(service);
    for (const signal of ['SIGINT', 'SIGTERM'] as const) process.once(signal, () => { process.exitCode = signal === 'SIGINT' ? 130 : 143; void server.close().catch(fail); });
    return;
  }
  const stopOperation = (signal: 'SIGINT' | 'SIGTERM') => { process.exitCode = signal === 'SIGINT' ? 130 : 143; void service.close().catch(fail); };
  const operationInt = () => stopOperation('SIGINT'), operationTerm = () => stopOperation('SIGTERM');
  if (!['run', 'continue'].includes(command)) { process.once('SIGINT', operationInt); process.once('SIGTERM', operationTerm); }
  try {
    if (roomCommand) {
      const outcome = await runRoomCommand(service, command, id, values, values['input-file'] === undefined ? undefined : { value: requestInput });
      print(outcome.result); if (outcome.exitCode && !process.exitCode) process.exitCode = outcome.exitCode;
    }
    else if (command === 'wait') {
      const result = await service.awaitResults([id!], Number(values['wait-ms'] ?? 600_000));
      print(result);
      if (result.timedOut || result.runs.some(run => run.status !== 'completed')) process.exitCode = 1;
    }
    else if (command === 'advise') {
      const run = service.advise({ requester: values.requester ?? 'host', advisor: values.advisor ?? '', question: values.question ?? '',
        ...(values.model !== undefined ? { model: values.model } : {}), ...(values.effort !== undefined ? { effort: values.effort as any } : {}) });
      process.stderr.write(`${JSON.stringify({ event: 'run.started', id: run.id, mode: 'advisor' })}\n`);
      const outcome = await service.completion(run.id);
      print(outcome);
      if (outcome.status !== 'completed') process.exitCode = 1;
    }
    else if (command === 'billing') {
      if (!values.agent) print(service.billingView());
      else {
        const record = service.billing.get(values.agent), now = Date.now();
        print({ ...record, availability: service.billing.availabilityFor(record, now),
          subscriptionProtection: service.billing.subscriptionProtectionFor(record, now) });
      }
    }
    else if (command === 'billing-set' || command === 'billing-import') {
      const revision = billingRevision ?? service.billing.get(values.agent!).revision;
      print(command === 'billing-set' ? service.billing.update(values.agent!, requestInput as any, revision) : service.billing.importSnapshot(values.agent!, requestInput as any, revision));
    }
    else if (command === 'route' || command === 'route-plan') {
      const input = routingInput!;
      if (command === 'route-plan') print(service.routePlan(input));
      else {
        const { run, selection } = service.routeRun(input);
        process.stderr.write(`${JSON.stringify({ event: 'run.started', id: run.id, mode: run.input.mode })}\n`);
        const { workflow: _workflow, ...receipt } = selection;
        const completed = await service.completion(run.id);
        print({ run: completed, selection: receipt });
        if (completed.status !== 'completed' && !process.exitCode) process.exitCode = 1;
      }
    }
    else if (command === 'health') { const health = service.health(values.integrity); print(health); if (health.status === 'degraded') process.exitCode = 1; }
    else if (command === 'backup') print({ destination: await service.backup(values.output!) });
    else if (command === 'snapshot') print(await service.snapshot(values.output!));
    else if (command === 'prune') print({ dryRun: !values.execute, ...service.prune({ before: values.before!, keepLatest: Number(values['keep-latest'] ?? 100), dryRun: !values.execute }) });
    else if (command === 'runs') {
      const limit = Number(values.limit ?? 20);
      if (!Number.isInteger(limit) || limit < 1 || limit > 100) throw new Error('Run limit must be from 1 to 100.');
      print(values.summary ? service.runPage(limit, values.cursor) : service.store.listRuns(limit));
    }
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
      print(values.summary ? service.store.compactEvents(id!, after, limit) : service.store.events(id!, after, limit));
    } else if (command === 'recover') print({ recovered: [...service.recoveredRuns, ...service.store.recoverOrphans()].map(run => ({ id: run.id, status: run.status, error: run.error })) });
    else if (command === 'cancel') {
      const request = service.requestCancellation(id!);
      print({ cancellationRequested: request.requested, run: values.summary ? request.run : service.getRun(id!) });
    } else if (command === 'patches') print(await service.exportArtifacts(id!));
    else if (command === 'cleanup') {
      if (!values.reviewed) throw new Error('Review the worktrees and patches, then provide --reviewed to request cleanup.');
      print(await service.cleanup(id!, { reviewed: true, discardChanges: values.discard === true }));
    } else if (command === 'demo') await demo(service);
    else {
      const previous = command === 'continue' ? service.store.getRun(id!) : undefined;
      if (command === 'continue' && !previous) throw new Error('Previous run not found.');
      const input = preparedWorkflow ?? workflowInput(previous);
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
  } finally { process.removeListener('SIGINT', operationInt); process.removeListener('SIGTERM', operationTerm); await service.close(); }
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

function fail(error: unknown): void { process.stderr.write(`Council: ${error instanceof Error ? error.message : 'Unexpected failure.'}\n`); if (!process.exitCode) process.exitCode = 1; }
main().catch(fail);
