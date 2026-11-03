import { existsSync, readFileSync, realpathSync, statSync, writeFileSync } from 'node:fs';
import { isAbsolute, relative, resolve, sep } from 'node:path';
import { AGENTS, MODES, type CouncilConfig, type WorkflowInput } from './types.js';
import { objectSchema, parseSchema, ValidationError, type JsonSchema } from './schema.js';

const boundedString = (minLength: number, maxLength: number): JsonSchema => ({ type: 'string', minLength, maxLength });
const integer = (minimum: number, maximum: number, value?: number): JsonSchema => ({ type: 'integer', minimum, maximum, ...(value === undefined ? {} : { default: value }) });
export const backendDefinition: JsonSchema = { type: 'string', enum: ['native', 'acpx', 'mock'], default: 'native' };
export const agentDefinition = objectSchema({
  enabled: { type: 'boolean', default: true },
  profile: { type: 'string', enum: AGENTS },
  model: boundedString(1, 200), transport: { type: 'string', enum: ['native', 'acp'] },
  command: { ...boundedString(1, 4096), pattern: '^[^\\u0000]+$' },
  args: { type: 'array', items: { ...boundedString(0, 4096), pattern: '^[^\\u0000]*$' }, maxItems: 64 },
}, ['profile']);

export const configDefinition = objectSchema({
  version: { const: 1, default: 1 }, backend: backendDefinition,
  stateDir: { ...boundedString(1, 4096), default: '.council' },
  maxConcurrency: integer(1, 8, 3), timeoutMs: integer(100, 3_600_000, 180_000),
  maxOutputBytes: integer(1024, 2_097_152, 131_072), maxCalls: integer(1, 64, 16),
  agents: {
    type: 'object', propertyNames: { pattern: '^(?!__proto__$|constructor$|prototype$)[a-z][a-z0-9-]{0,63}$' },
    additionalProperties: agentDefinition,
    default: Object.fromEntries(AGENTS.map(agent => [agent, { enabled: true, profile: agent }])),
  },
});

export const workflowDefinition = objectSchema({
  mode: { type: 'string', enum: MODES }, task: boundedString(1, 32_000),
  agents: { type: 'array', items: boundedString(1, 64), minItems: 1, maxItems: 8 },
  judge: boundedString(1, 64), rounds: integer(1, 3),
  permission: { type: 'string', enum: ['read', 'write'], default: 'read' },
  context: boundedString(0, 64_000), maxCalls: integer(1, 64),
  assignments: { type: 'object', additionalProperties: boundedString(1, 32_000) },
}, ['mode', 'task', 'agents']);

export const configSchema = {
  parse(value: unknown): Omit<CouncilConfig, 'workspaceRoot'> {
    const config = parseSchema<Omit<CouncilConfig, 'workspaceRoot'>>(configDefinition, value, 'configuration');
    if (config.maxCalls * config.maxOutputBytes > 2_097_152) throw new ValidationError('maxCalls multiplied by maxOutputBytes must not exceed 2 MiB per run.');
    if (Object.keys(config.agents).length === 0 || Object.keys(config.agents).length > 32) throw new ValidationError('Configure between 1 and 32 named agent profiles.');
    return config;
  },
};
export const workflowSchema = {
  parse(value: unknown): WorkflowInput {
    const input = parseSchema<WorkflowInput>(workflowDefinition, value, 'workflow');
    if (!input.task.trim()) throw new ValidationError('Task must contain non-whitespace text.');
    if (input.assignments && (Object.keys(input.assignments).length > 8 || Object.values(input.assignments).reduce((size, text) => size + text.length, 0) > 64_000)) throw new ValidationError('Assignments must contain at most 8 tasks and 64,000 characters in total.');
    return input;
  },
};

export function parseBackend(value: unknown): CouncilConfig['backend'] { return parseSchema(backendDefinition, value, 'backend'); }
export function readJsonFile(filename: string): unknown {
  if (statSync(filename).size > 1_048_576) throw new ValidationError('Configuration/input file exceeds 1 MiB.');
  try { return JSON.parse(readFileSync(filename, 'utf8').replace(/^\uFEFF/, '')); }
  catch (error) { if (error instanceof SyntaxError) throw new ValidationError(`Invalid JSON in ${filename}.`); throw error; }
}
function escapes(path: string): boolean { return path === '..' || path.startsWith(`..${sep}`) || isAbsolute(path); }

export function loadConfig(workspace = process.cwd(), overrides: Partial<CouncilConfig> = {}): CouncilConfig {
  const workspaceRoot = realpathSync(resolve(workspace));
  if (!statSync(workspaceRoot).isDirectory()) throw new Error('Workspace must be a directory.');
  const filename = resolve(workspaceRoot, 'council.config.json');
  const raw = existsSync(filename) ? readJsonFile(filename) : {};
  if (!raw || typeof raw !== 'object' || Array.isArray(raw)) throw new ValidationError('Project configuration must be a JSON object.');
  const { workspaceRoot: _workspace, stateDir: overrideState, ...settings } = overrides;
  const parsed = configSchema.parse({ ...raw, ...settings, ...(overrideState ? { stateDir: overrideState } : {}) });
  const stateDir = resolve(workspaceRoot, parsed.stateDir);
  const pathInside = relative(workspaceRoot, stateDir);
  if (!pathInside || escapes(pathInside)) throw new Error('stateDir must be a directory inside the workspace.');
  let ancestor = stateDir;
  while (!existsSync(ancestor)) ancestor = resolve(ancestor, '..');
  if (escapes(relative(workspaceRoot, realpathSync(ancestor)))) throw new Error('stateDir resolves outside the workspace.');
  return { ...parsed, workspaceRoot, stateDir };
}

export function initConfig(workspace = process.cwd(), backend: CouncilConfig['backend'] = 'native'): string {
  const filename = resolve(workspace, 'council.config.json');
  writeFileSync(filename, `${JSON.stringify(configSchema.parse({ backend }), null, 2)}\n`, { flag: 'wx' });
  return filename;
}
export function parseWorkflow(value: unknown): WorkflowInput { return workflowSchema.parse(value); }
