import { existsSync, realpathSync, statSync, writeFileSync } from 'node:fs';
import { isAbsolute, relative, resolve, sep } from 'node:path';
import { AGENTS, MODES, REASONING_EFFORTS, type CouncilConfig, type WorkflowInput } from './types.js';
import { objectSchema, parseSchema, ValidationError, type JsonSchema } from './schema.js';
import { BUILT_IN_SKILL_IDS } from './skills.js';
import { readUtf8File } from './utf8-file.js';
import { assertUniqueObjectKeys } from './control-json.js';

const boundedString = (minLength: number, maxLength: number): JsonSchema => ({ type: 'string', minLength, maxLength });
const fullString = (minLength: number): JsonSchema => ({ type: 'string', minLength });
const integer = (minimum: number, maximum: number, value?: number): JsonSchema => ({ type: 'integer', minimum, maximum, ...(value === undefined ? {} : { default: value }) });
export const backendDefinition: JsonSchema = { type: 'string', enum: ['native', 'acpx', 'mock'], default: 'native' };
export const reasoningEffortDefinition: JsonSchema = { type: 'string', enum: REASONING_EFFORTS };
export const agentDefinition = objectSchema({
  enabled: { type: 'boolean', default: true },
  profile: { type: 'string', enum: AGENTS },
  model: boundedString(1, 200), effort: reasoningEffortDefinition, authority: { type: 'number', exclusiveMinimum: 0 },
  skills: { type: 'array', items: { type: 'string', enum: BUILT_IN_SKILL_IDS } },
  transport: { type: 'string', enum: ['native', 'acp'] },
  command: { ...boundedString(1, 4096), pattern: '^[^\\u0000]+$' },
  args: { type: 'array', items: { ...boundedString(0, 4096), pattern: '^[^\\u0000]*$' }, maxItems: 64 },
}, ['profile']);

export const configDefinition = objectSchema({
  version: { const: 1, default: 1 }, backend: backendDefinition,
  stateDir: { ...boundedString(1, 4096), default: '.council' },
  maxConcurrency: integer(1, 8, 3), timeoutMs: integer(100, 3_600_000, 180_000),
  maxOutputBytes: integer(1024, 2_097_152, 131_072), maxCalls: integer(1, Number.MAX_SAFE_INTEGER, 16),
  agents: {
    type: 'object', propertyNames: { pattern: '^(?!__proto__$|constructor$|prototype$)[a-z][a-z0-9-]{0,63}$' },
    additionalProperties: agentDefinition,
    default: Object.fromEntries(AGENTS.map(agent => [agent, { enabled: true, profile: agent }])),
  },
});

export const workflowDefinition = objectSchema({
  mode: { type: 'string', enum: MODES }, task: fullString(1),
  agents: { type: 'array', items: boundedString(1, 64), minItems: 1 },
  judge: boundedString(1, 64), rounds: integer(1, Number.MAX_SAFE_INTEGER),
  permission: { type: 'string', enum: ['read', 'write'], default: 'read' },
  context: fullString(0), maxCalls: integer(1, Number.MAX_SAFE_INTEGER),
  assignments: { type: 'object', additionalProperties: fullString(1) },
  agentSettings: { type: 'object', additionalProperties: objectSchema({ model: boundedString(1, 200), effort: reasoningEffortDefinition, skills: { type: 'array', items: { type: 'string', enum: BUILT_IN_SKILL_IDS } } }) },
  advisors: { type: 'object', additionalProperties: objectSchema({ agent: boundedString(1, 64), maxRequests: { type: 'integer', minimum: 1, default: 1 } }, ['agent']) },
  privateChats: { type: 'object', additionalProperties: objectSchema({ peers: { type: 'array', items: boundedString(1, 64), minItems: 1 }, maxRequests: { type: 'integer', minimum: 1, default: 1 } }, ['peers']) },
  contextLimit: { type: ['integer', 'null'], minimum: 1 },
}, ['mode', 'task', 'agents']);

export const configSchema = {
  parse(value: unknown): Omit<CouncilConfig, 'workspaceRoot'> {
    const config = parseSchema<Omit<CouncilConfig, 'workspaceRoot'>>(configDefinition, value, 'configuration');
    if (Object.keys(config.agents).length === 0) throw new ValidationError('Configure at least one named agent profile.');
    return config;
  },
};
export const workflowSchema = {
  parse(value: unknown): WorkflowInput {
    const input = parseSchema<WorkflowInput>(workflowDefinition, value, 'workflow');
    if (!input.task.trim()) throw new ValidationError('Task must contain non-whitespace text.');
    return input;
  },
};

export function parseBackend(value: unknown): CouncilConfig['backend'] { return parseSchema(backendDefinition, value, 'backend'); }
export function readJsonFile(filename: string, maxBytes = 1_048_576): unknown {
  if (maxBytes !== Infinity && (!Number.isSafeInteger(maxBytes) || maxBytes < 1)) throw new ValidationError('JSON file byte limit must be a positive integer or Infinity.');
  if (statSync(filename).size > maxBytes) throw new ValidationError(maxBytes === 1_048_576 ? 'Configuration/input file exceeds 1 MiB.' : 'JSON input exceeds its configured file byte limit.');
  try {
    const text = readUtf8File(filename), value: unknown = JSON.parse(text);
    assertUniqueObjectKeys(text, 'JSON file');
    return value;
  }
  catch (error) { if (error instanceof SyntaxError) throw new ValidationError(`Invalid JSON in ${filename}.`); throw error; }
}
function escapes(path: string): boolean { return path === '..' || path.startsWith(`..${sep}`) || isAbsolute(path); }

export function loadConfig(workspace = process.cwd(), overrides: Partial<CouncilConfig> = {}): CouncilConfig {
  // The legacy sync implementation preserves Windows 8.3 spellings. Use the
  // same native canonicalization as async filesystem operations and Git.
  const workspaceRoot = realpathSync.native(resolve(workspace));
  if (!statSync(workspaceRoot).isDirectory()) throw new Error('Workspace must be a directory.');
  const filename = resolve(workspaceRoot, 'council.config.json');
  const raw = existsSync(filename) ? readJsonFile(filename) : {};
  if (!raw || typeof raw !== 'object' || Array.isArray(raw)) throw new ValidationError('Project configuration must be a JSON object.');
  const { workspaceRoot: _workspace, stateDir: overrideState, ...settings } = overrides;
  const parsed = configSchema.parse({ ...raw, ...settings, ...(overrideState ? { stateDir: overrideState } : {}) });
  const requestedState = resolve(workspaceRoot, parsed.stateDir);
  const pathInside = relative(workspaceRoot, requestedState);
  if (!pathInside || escapes(pathInside)) throw new Error('stateDir must be a directory inside the workspace.');
  let ancestor = requestedState;
  while (!existsSync(ancestor)) ancestor = resolve(ancestor, '..');
  const canonicalAncestor = realpathSync.native(ancestor), ancestorSuffix = relative(workspaceRoot, canonicalAncestor);
  if (escapes(ancestorSuffix)) throw new Error('stateDir resolves outside the workspace.');
  // path.relative folds Windows casing even inside a case-sensitive directory.
  // Verify the physical root selected by that lexical prefix before accepting it.
  const selectedAncestor = resolve(canonicalAncestor, ...ancestorSuffix.split(sep).filter(Boolean).map(() => '..'));
  const rootIdentity = statSync(workspaceRoot, { bigint: true }), ancestorIdentity = statSync(selectedAncestor, { bigint: true });
  if (rootIdentity.dev !== ancestorIdentity.dev || rootIdentity.ino !== ancestorIdentity.ino) throw new Error('stateDir resolves outside the workspace.');
  const stateDir = resolve(canonicalAncestor, relative(ancestor, requestedState));
  if (!relative(workspaceRoot, stateDir)) throw new Error('stateDir must be a directory inside the workspace.');
  return { ...parsed, workspaceRoot, stateDir };
}

export function initConfig(workspace = process.cwd(), backend: CouncilConfig['backend'] = 'native'): string {
  const filename = resolve(workspace, 'council.config.json');
  writeFileSync(filename, `${JSON.stringify(configSchema.parse({ backend }), null, 2)}\n`, { flag: 'wx' });
  return filename;
}
export function parseWorkflow(value: unknown): WorkflowInput { return workflowSchema.parse(value); }
