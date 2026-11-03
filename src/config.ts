import { existsSync, readFileSync, realpathSync, statSync, writeFileSync } from 'node:fs';
import { isAbsolute, relative, resolve } from 'node:path';
import { z } from 'zod';
import { AGENTS, MODES, type CouncilConfig, type WorkflowInput } from './types.js';

export const configSchema = z.object({
  version: z.literal(1).default(1),
  backend: z.enum(['acpx', 'mock']).default('acpx'),
  stateDir: z.string().min(1).default('.council'),
  maxConcurrency: z.number().int().min(1).max(8).default(3),
  timeoutMs: z.number().int().min(100).max(3_600_000).default(180_000),
  maxOutputBytes: z.number().int().min(1024).max(2_097_152).default(131_072),
  maxCalls: z.number().int().min(1).max(64).default(16),
  agents: z.record(z.string().regex(/^[a-z][a-z0-9-]{0,63}$/), z.object({
    enabled: z.boolean().default(true),
    profile: z.enum(AGENTS),
    model: z.string().min(1).max(200).optional(),
  }).strict()).default(Object.fromEntries(AGENTS.map(agent => [agent, { enabled: true, profile: agent }]))),
}).strict().refine(config => config.maxCalls * config.maxOutputBytes <= 2_097_152, { message: 'maxCalls multiplied by maxOutputBytes must not exceed 2 MiB per run.' });

export const workflowSchema = z.object({
  mode: z.enum(MODES), task: z.string().min(1).max(32_000),
  agents: z.array(z.string().min(1).max(64)).min(1).max(8),
  judge: z.string().min(1).max(64).optional(),
  rounds: z.number().int().min(1).max(3).optional(),
  permission: z.enum(['read', 'write']).default('read'),
  context: z.string().max(64_000).optional(),
  maxCalls: z.number().int().min(1).max(64).optional(),
  assignments: z.record(z.string().min(1).max(64), z.string().min(1).max(32_000)).refine(value => Object.keys(value).length <= 8 && Object.values(value).reduce((size, task) => size + task.length, 0) <= 64_000, { message: 'Assignments must contain at most 8 tasks and 64,000 characters in total.' }).optional(),
}).strict();

export function loadConfig(workspace = process.cwd(), overrides: Partial<CouncilConfig> = {}): CouncilConfig {
  const workspaceRoot = realpathSync(resolve(workspace));
  if (!statSync(workspaceRoot).isDirectory()) throw new Error('Workspace must be a directory.');
  const filename = resolve(workspaceRoot, 'council.config.json');
  const raw = existsSync(filename) ? JSON.parse(readFileSync(filename, 'utf8')) : {};
  const { workspaceRoot: _workspace, stateDir: overrideState, ...settings } = overrides;
  const parsed = configSchema.parse({ ...raw, ...settings, ...(overrideState ? { stateDir: overrideState } : {}) });
  const stateDir = resolve(workspaceRoot, parsed.stateDir);
  const pathInside = relative(workspaceRoot, stateDir);
  if (!pathInside || pathInside.startsWith('..') || isAbsolute(pathInside)) {
    throw new Error('stateDir must be a directory inside the workspace.');
  }
  // Resolve existing ancestors so a symlink cannot redirect collaboration state outside the workspace.
  let ancestor = stateDir;
  while (!existsSync(ancestor)) ancestor = resolve(ancestor, '..');
  const realAncestor = relative(workspaceRoot, realpathSync(ancestor));
  if (realAncestor.startsWith('..') || isAbsolute(realAncestor)) throw new Error('stateDir resolves outside the workspace.');
  return { ...parsed, workspaceRoot, stateDir };
}

export function initConfig(workspace = process.cwd(), backend: 'acpx' | 'mock' = 'acpx'): string {
  const filename = resolve(workspace, 'council.config.json');
  const config = configSchema.parse({ backend });
  writeFileSync(filename, `${JSON.stringify(config, null, 2)}\n`, { flag: 'wx' });
  return filename;
}

export function parseWorkflow(input: unknown): WorkflowInput { return workflowSchema.parse(input); }
