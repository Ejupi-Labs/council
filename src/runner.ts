import { spawn, type ChildProcessWithoutNullStreams } from 'node:child_process';
import { access } from 'node:fs/promises';
import { constants } from 'node:fs';
import { createRequire } from 'node:module';
import { delimiter, isAbsolute, join, resolve } from 'node:path';
import { StringDecoder } from 'node:string_decoder';
import { createHash } from 'node:crypto';
import type { AgentHealth, AgentRequest, AgentResult, AgentRunner, CouncilConfig, ResultStatus } from './types.js';

/** Optional process configuration for embedding and local subprocess tests. */
export interface AcpxRunnerOptions {
  acpxEntrypoint?: string;
  environment?: NodeJS.ProcessEnv;
}

type JsonObject = Record<string, unknown>;
const object = (value: unknown): value is JsonObject => typeof value === 'object' && value !== null && !Array.isArray(value);

function resolveAcpx(): string {
  return createRequire(import.meta.url).resolve('acpx');
}

function acpErrorSummary(error: JsonObject): string {
  const message = typeof error.message === 'string' ? error.message : '';
  const detail = object(error.data) && typeof error.data.detailCode === 'string' ? error.data.detailCode : '';
  // Classify known failure shapes without returning provider text, which may contain secrets.
  if (/ETARGET|No matching version found/i.test(message)) return 'Agent adapter dependency is unavailable from npm. Run npm ci to restore bundled adapters and check any custom ACPX profile overrides.';
  if (/Cannot apply --model|did not advertise that model|model_not_found/i.test(message)) return 'Agent adapter rejected the configured model. Select a model advertised by that adapter in Council configuration.';
  if (/ENOENT|executable not found|command not found/i.test(message)) return 'Agent adapter launch executable is missing. Run council doctor and check the ACPX profile launch configuration.';
  if (/auth(?:entication)?[_ ](?:required|failed)|not logged in|login required|unauthorized/i.test(message)) return 'Agent authentication is unavailable. Sign in through the agent CLI outside Council, then retry.';
  if (detail === 'AGENT_STARTUP_FAILED') return 'Agent adapter failed before ACP initialization. Run council doctor and verify the configured adapter can start.';
  const code = typeof error.code === 'number' ? ` (${error.code})` : '';
  return `Agent returned an ACP error${code}.`;
}

function providerAnswerFailure(output: string): string | undefined {
  // Some ACP adapters relay a provider failure as the entire assistant answer,
  // followed by end_turn. Match only a complete, known error envelope, never
  // prose, quoted examples, code fences, or arbitrary references to an API error.
  const normalized = output.trim().replace(/^Warning: Model metadata for `[^`\r\n]{1,200}` not found\. Defaulting to fallback metadata; this can degrade performance and cause issues\.\s*/, '');
  let value: unknown;
  try { value = JSON.parse(normalized); } catch { return undefined; }
  if (!object(value) || value.type !== 'error' || typeof value.status !== 'number' || value.status < 400 || value.status > 599 || !object(value.error)) return undefined;
  if (value.error.code === 'model_not_found') return 'Agent provider rejected the configured model. Select a model supported by that agent and account in Council configuration.';
  if (value.error.code === 'invalid_api_key' || value.error.type === 'authentication_error') return 'Agent authentication is unavailable. Sign in through the agent CLI outside Council, then retry.';
  if (value.error.code === 'insufficient_quota' || value.error.code === 'rate_limit_exceeded') return 'Agent provider usage limit was reached. Retry when that account has capacity.';
  return undefined;
}

async function executableOnPath(command: string, environment: NodeJS.ProcessEnv): Promise<boolean> {
  const extensions = process.platform === 'win32' ? (environment.PATHEXT ?? '.COM;.EXE;.BAT;.CMD').split(';') : [''];
  const directories = isAbsolute(command) ? [''] : (environment.PATH ?? environment.Path ?? '').split(delimiter).filter(Boolean);
  for (const directory of directories) {
    const base = directory ? join(directory.replace(/^"|"$/g, ''), command) : command;
    const candidates = process.platform === 'win32' && /\.(exe|com|cmd|bat)$/i.test(command) ? [base] : extensions.map(extension => base + extension);
    for (const candidate of candidates) {
      try {
        await access(candidate, process.platform === 'win32' ? constants.F_OK : constants.X_OK);
        return true;
      } catch { /* Try the next PATH entry. */ }
    }
  }
  return false;
}

/** Kill the process group/tree, including adapters launched underneath ACPX. */
function killTree(child: ChildProcessWithoutNullStreams): void {
  if (child.pid === undefined) return;
  if (process.platform === 'win32') {
    const taskkill = join(process.env.SystemRoot ?? 'C:\\Windows', 'System32', 'taskkill.exe');
    const killer = spawn(taskkill, ['/PID', String(child.pid), '/T', '/F'], { shell: false, windowsHide: true, stdio: 'ignore' });
    killer.once('error', () => { child.kill('SIGKILL'); });
    killer.once('exit', code => { if (code !== 0) child.kill('SIGKILL'); });
  } else {
    try { process.kill(-child.pid, 'SIGKILL'); } catch { child.kill('SIGKILL'); }
  }
}

/** ACPX 0.21.1 emits raw ACP JSON-RPC NDJSON, not provider-specific CLI JSON. */
export class AcpxRunner implements AgentRunner {
  constructor(private readonly config: CouncilConfig, private readonly options: AcpxRunnerOptions = {}) {}

  async run(request: AgentRequest): Promise<AgentResult> {
    const started = performance.now();
    const result = (status: ResultStatus, output: string, error?: string, stopReason?: string): AgentResult => ({
      agent: request.agent, status, output, durationMs: Math.round(performance.now() - started),
      ...(error ? { error } : {}), ...(stopReason ? { stopReason } : {}),
    });
    if (request.signal?.aborted) return result('cancelled', '', 'Agent request cancelled.');
    const agent = this.config.agents[request.agent];
    if (!agent?.enabled) return result('failed', '', 'Agent is not enabled in Council configuration.');
    if (!/^[a-zA-Z0-9][a-zA-Z0-9_.-]*$/.test(agent.profile)) return result('failed', '', 'Invalid ACPX agent profile.');
    if (!Number.isFinite(request.timeoutMs) || request.timeoutMs <= 0) return result('failed', '', 'Agent timeout must be positive.');
    const maxBytes = this.config.maxOutputBytes;
    if (!Number.isSafeInteger(maxBytes) || maxBytes < 1) return result('failed', '', 'Invalid agent output limit.');
    // ACPX echoes the submitted prompt and tool traffic. Bound that transport separately
    // so a large input does not consume the final assistant-answer allowance.
    const maxWireBytes = Math.min(64 * 1024 * 1024, Math.max(4 * 1024 * 1024, Buffer.byteLength(request.prompt, 'utf8') * 6 + maxBytes * 8));
    const maxStderrBytes = 64 * 1024;
    let entrypoint: string;
    try { entrypoint = this.options.acpxEntrypoint ?? resolveAcpx(); }
    catch { return result('failed', '', 'ACPX is unavailable. Install Council dependencies and run council doctor.'); }
    const write = request.permission === 'write';
    const args = [entrypoint, '--format', 'json', '--json-strict', '--cwd', resolve(request.cwd),
      write ? '--approve-all' : '--deny-all', '--permission-policy', JSON.stringify({ defaultAction: write ? 'approve' : 'deny' }),
      '--non-interactive-permissions', 'deny', '--timeout', String(request.timeoutMs / 1000)];
    const model = request.model ?? agent.model;
    if (model) args.push('--model', model);
    args.push(agent.profile, 'exec', '--file', '-');
    const inheritedEnvironment = this.options.environment ?? process.env;
    const inheritedDepth = Number(inheritedEnvironment.COUNCIL_DEPTH ?? 0);
    const environment = { ...inheritedEnvironment,
      COUNCIL_DEPTH: String(Number.isSafeInteger(inheritedDepth) && inheritedDepth >= 0 ? inheritedDepth + 1 : 1) };

    return await new Promise<AgentResult>(resolveResult => {
      let child: ChildProcessWithoutNullStreams;
      try {
        child = spawn(process.execPath, args, {
          cwd: request.cwd, shell: false, windowsHide: true, detached: process.platform !== 'win32',
          stdio: ['pipe', 'pipe', 'pipe'], env: environment,
        });
      } catch { resolveResult(result('failed', '', 'Unable to start ACPX.')); return; }
      const decoder = new StringDecoder('utf8');
      let pending = '';
      let output = '';
      let receivedBytes = 0;
      let stderrBytes = 0;
      let outputBytes = 0;
      let promptId: string | number | undefined;
      let sessionId: string | undefined;
      let stopReason: string | undefined;
      let failure: string | undefined;
      let forcedStatus: ResultStatus | undefined;
      let settled = false;
      let settleTimer: ReturnType<typeof setTimeout> | undefined;
      const finish = (status: ResultStatus, error?: string): void => {
        if (settled) return;
        settled = true;
        clearTimeout(timer);
        if (settleTimer) clearTimeout(settleTimer);
        request.signal?.removeEventListener('abort', onAbort);
        resolveResult(result(status, output, error, stopReason));
      };
      const terminate = (status: ResultStatus, error: string): void => {
        if (forcedStatus || settled) return;
        forcedStatus = status;
        failure = error;
        child.stdin.destroy();
        killTree(child);
        settleTimer = setTimeout(() => {
          child.stdout.destroy();
          child.stderr.destroy();
          child.kill('SIGKILL');
          finish(status, error);
        }, 3000);
      };
      const onAbort = (): void => terminate('cancelled', 'Agent request cancelled.');
      const timer = setTimeout(() => terminate('timeout', 'Agent exceeded its time limit.'), Math.min(request.timeoutMs, 2_147_483_647));
      request.signal?.addEventListener('abort', onAbort, { once: true });
      if (request.signal?.aborted) onAbort();

      const parseLine = (line: string): void => {
        if (!line.trim() || forcedStatus) return;
        let event: unknown;
        try { event = JSON.parse(line); } catch { terminate('failed', 'ACPX returned malformed or partial JSON.'); return; }
        if (!object(event) || event.jsonrpc !== '2.0') { terminate('failed', 'ACPX returned an invalid JSON-RPC event.'); return; }
        if (object(event.error)) {
          terminate('failed', acpErrorSummary(event.error));
          return;
        }
        if (event.method === 'session/prompt' && object(event.params)) {
          if (promptId !== undefined || (typeof event.id !== 'string' && typeof event.id !== 'number')) {
            terminate('failed', 'ACPX returned an invalid prompt sequence.'); return;
          }
          promptId = event.id;
          sessionId = typeof event.params.sessionId === 'string' ? event.params.sessionId : undefined;
        }
        if (event.method === 'session/update' && object(event.params) && object(event.params.update)) {
          const update = event.params.update;
          if (update.sessionUpdate !== 'agent_message_chunk' || !object(update.content) || update.content.type !== 'text' || typeof update.content.text !== 'string') return;
          if (stopReason || promptId === undefined || (sessionId !== undefined && event.params.sessionId !== sessionId)) {
            terminate('failed', 'ACPX returned assistant text outside the active prompt.'); return;
          }
          outputBytes += Buffer.byteLength(update.content.text, 'utf8');
          if (outputBytes > maxBytes) { terminate('failed', 'Agent answer exceeded its configured byte limit.'); return; }
          output += update.content.text;
        }
        if (object(event.result) && typeof event.result.stopReason === 'string') {
          if (promptId === undefined || event.id !== promptId || stopReason !== undefined) {
            terminate('failed', 'ACPX returned an invalid completion sequence.'); return;
          }
          const reason = event.result.stopReason;
          if (!/^[a-z_]{1,80}$/.test(reason)) { terminate('failed', 'ACPX returned an invalid completion reason.'); return; }
          stopReason = reason;
        }
      };
      child.stdout.on('data', (chunk: Buffer) => {
        receivedBytes += chunk.length;
        if (receivedBytes > maxWireBytes) { terminate('failed', 'Agent protocol traffic exceeded its bounded transport allowance.'); return; }
        pending += decoder.write(chunk);
        let newline: number;
        while ((newline = pending.indexOf('\n')) !== -1 && !forcedStatus) {
          const line = pending.slice(0, newline);
          pending = pending.slice(newline + 1);
          parseLine(line);
        }
      });
      child.stderr.on('data', (chunk: Buffer) => {
        // Bound diagnostics separately without retaining or exposing secret-bearing text.
        stderrBytes += chunk.length;
        if (stderrBytes > maxStderrBytes) terminate('failed', 'Agent diagnostics exceeded their bounded transport allowance.');
      });
      child.once('error', () => finish('failed', 'Unable to start ACPX.'));
      child.stdin.on('error', () => { /* Exit and protocol validation determine the result; avoid EPIPE crashes. */ });
      child.once('close', (code, signal) => {
        if (settled) return;
        if (forcedStatus) { finish(forcedStatus, failure); return; }
        pending += decoder.end();
        if (pending.trim()) parseLine(pending);
        if (forcedStatus) { finish(forcedStatus, failure); return; }
        if (code !== 0) { finish('failed', signal ? 'ACPX was terminated before successful completion.' : code === 5
          ? 'Agent permission request was denied or could not be answered; the task is incomplete.'
          : `ACPX exited with code ${code ?? 'unknown'}. Provider diagnostics were withheld.`); return; }
        if (stopReason === 'cancelled') { finish('cancelled', 'Agent cancelled its turn.'); return; }
        if (stopReason !== 'end_turn') { finish('failed', stopReason ? `Agent stopped with ${stopReason}; the task is incomplete.` : 'Agent exited without a successful terminal completion.'); return; }
        if (!output.trim()) { finish('failed', 'Agent returned an empty answer.'); return; }
        const providerFailure = providerAnswerFailure(output);
        if (providerFailure) { output = ''; finish('failed', providerFailure); return; }
        finish('completed');
      });
      child.stdin.end(request.prompt, 'utf8');
    });
  }

  async doctor(): Promise<AgentHealth[]> {
    let runtimeAvailable = true;
    try { await access(this.options.acpxEntrypoint ?? resolveAcpx(), constants.R_OK); } catch { runtimeAvailable = false; }
    const environment = this.options.environment ?? process.env;
    let registry: typeof import('acpx/agent-registry') | undefined;
    try { registry = await import('acpx/agent-registry'); } catch { /* Missing dependencies are reported below. */ }
    const commands: Record<string, string[]> = {
      codex: ['codex', 'npx'], claude: ['claude', 'npx'], 'deepseek-harness': ['dsh'],
      junie: ['junie'], antigravity: ['agy_acp_server.par', 'agy_acp_server'],
    };
    return await Promise.all(Object.entries(this.config.agents).map(async ([name, agent]) => {
      if (!agent.enabled) return { agent: name, enabled: false, available: false, detail: 'Disabled in Council configuration.' };
      if (!runtimeAvailable) return { agent: name, enabled: true, available: false, detail: 'ACPX runtime missing; install Council dependencies.' };
      const builtInCommand = registry?.AGENT_REGISTRY[agent.profile];
      const bundled = builtInCommand ? registry?.resolveInstalledBuiltInAgentLaunch(builtInCommand) : undefined;
      if (bundled) return { agent: name, enabled: true, available: true,
        detail: `Local ACPX runtime and bundled ${bundled.packageName}@${bundled.packageVersion ?? 'unknown'} adapter found; no npx download needed. Authentication, provider access, and ACP readiness were not tested.` };
      const required = commands[agent.profile] ?? [agent.profile];
      const checks = await Promise.all(required.map(command => executableOnPath(command, environment)));
      const available = checks.every(Boolean);
      return { agent: name, enabled: true, available, detail: available
        ? `Local runtime and launch prerequisites found (${required.join(', ')}). Authentication, adapter download, and ACP readiness were not tested.`
        : `Missing local launch prerequisite: ${required.filter((_, index) => !checks[index]).join(', ')}. Custom ACPX profiles may need separate configuration; authentication was not tested.` };
    }));
  }
}

/** Deterministic, offline runner for demos and tests. Never invokes a provider. */
export class MockRunner implements AgentRunner {
  constructor(private readonly config: CouncilConfig) {}

  async run(request: AgentRequest): Promise<AgentResult> {
    if (request.signal?.aborted) return { agent: request.agent, status: 'cancelled', output: '', durationMs: 0, error: 'Agent request cancelled.' };
    if (!this.config.agents[request.agent]?.enabled) return { agent: request.agent, status: 'failed', output: '', durationMs: 0, error: 'Agent is not enabled in Council configuration.' };
    const digest = createHash('sha256').update(request.prompt).digest('hex').slice(0, 12);
    return { agent: request.agent, status: 'completed', durationMs: 0, stopReason: 'end_turn',
      output: `[MOCK ${request.agent}] Offline simulation ${digest}.\nReviewed the supplied task. Suggested action: keep changes scoped, verify acceptance criteria, and report unresolved assumptions. No provider was called and no files were changed.` };
  }

  async doctor(): Promise<AgentHealth[]> {
    return Object.entries(this.config.agents).map(([agent, value]) => ({ agent, enabled: value.enabled, available: value.enabled,
      detail: value.enabled ? 'Deterministic offline mock; no provider calls.' : 'Disabled in Council configuration.' }));
  }
}

export function createRunner(config: CouncilConfig): AgentRunner {
  return config.backend === 'mock' ? new MockRunner(config) : new AcpxRunner(config);
}
