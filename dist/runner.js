import { createHash } from 'node:crypto';
import { basename, resolve } from 'node:path';
import { isUtf8 } from 'node:buffer';
import { AcpProtocol, acpErrorSummary, isObject, validAcpEnvelope } from './acp.js';
import { ManagedProcess, ProcessCleanupError, PROCESS_CLEANUP_ERROR, resolveLaunch } from './process.js';
import { sanitizedGitEnvironment } from './git-env.js';
import { checkAgentJsonStructure } from './agent-json.js';
export const MAX_AGENT_FRAME_BYTES = 4 * 1024 * 1024;
async function withBudget(request, task) {
    const started = performance.now();
    const controller = new AbortController();
    let timedOut = false;
    const expire = () => { timedOut = true; controller.abort(); };
    const timer = setTimeout(expire, Math.min(request.timeoutMs, 2_147_483_647));
    const cancel = () => controller.abort();
    request.signal?.addEventListener('abort', cancel, { once: true });
    if (request.signal?.aborted)
        cancel();
    const remaining = () => {
        const left = request.timeoutMs - (performance.now() - started);
        if (left <= 0)
            expire();
        controller.signal.throwIfAborted();
        return Math.max(1, Math.ceil(left));
    };
    let result;
    try {
        result = await task(controller.signal, remaining);
    }
    catch (error) {
        result = failed(request, error instanceof Error ? error.message : 'Unable to resolve the configured agent executable.');
    }
    finally {
        clearTimeout(timer);
        request.signal?.removeEventListener('abort', cancel);
    }
    const durationMs = Math.round(performance.now() - started);
    if (result.error === PROCESS_CLEANUP_ERROR)
        return { ...result, status: 'failed', durationMs };
    if (timedOut)
        return { ...result, status: 'timeout', durationMs, error: 'Agent exceeded its total time limit, including executable startup.' };
    if (controller.signal.aborted)
        return { ...result, status: 'cancelled', durationMs, error: 'Agent request cancelled.' };
    return { ...result, durationMs };
}
function childEnvironment(environment) {
    const depth = Number(environment.COUNCIL_DEPTH ?? 0);
    return { ...sanitizedGitEnvironment(environment), COUNCIL_DEPTH: String(Number.isSafeInteger(depth) && depth >= 0 ? depth + 1 : 1) };
}
function providerAnswerFailure(output) {
    const normalized = output.trim().replace(/^Warning: Model metadata for `[^`\r\n]{1,200}` not found\. Defaulting to fallback metadata; this can degrade performance and cause issues\.\s*/, '');
    let value;
    try {
        value = JSON.parse(normalized);
    }
    catch {
        return undefined;
    }
    if (!isObject(value) || value.type !== 'error' || typeof value.status !== 'number' || value.status < 400 || value.status > 599 || !isObject(value.error))
        return undefined;
    if (value.error.code === 'model_not_found')
        return 'Agent provider rejected the configured model. Select a supported model in Council configuration; Council does not substitute one.';
    if (value.error.code === 'invalid_api_key' || value.error.type === 'authentication_error')
        return 'Agent authentication is unavailable. Sign in through its CLI outside Council, then retry.';
    if (value.error.code === 'insufficient_quota' || value.error.code === 'rate_limit_exceeded')
        return 'Agent provider usage limit was reached. Retry when that account has capacity.';
    return undefined;
}
function failed(request, error) {
    return { agent: request.agent, status: 'failed', output: '', durationMs: 0, error };
}
function validateRequest(config, request) {
    if (request.signal?.aborted)
        return { agent: request.agent, status: 'cancelled', output: '', durationMs: 0, error: 'Agent request cancelled.' };
    if (request.permission !== 'read' && request.permission !== 'write')
        return failed(request, 'Agent permission must be read or write.');
    if (!config.agents[request.agent]?.enabled)
        return failed(request, 'Agent is not enabled in Council configuration.');
    if (!Number.isFinite(request.timeoutMs) || request.timeoutMs <= 0)
        return failed(request, 'Agent timeout must be positive.');
    if (!Number.isSafeInteger(config.maxOutputBytes) || config.maxOutputBytes < 1)
        return failed(request, 'Invalid agent output limit.');
    return undefined;
}
/** Shared framing and lifecycle checks for native CLI, direct ACP, and optional external ACPX. */
async function runProtocol(launch, protocol, request, config, environment) {
    const started = performance.now();
    const maxBytes = config.maxOutputBytes;
    const maxWireBytes = Math.min(64 * 1024 * 1024, Math.max(4 * 1024 * 1024, Buffer.byteLength(request.prompt, 'utf8') * 6 + maxBytes * 8));
    return await new Promise(resolveResult => {
        let child;
        try {
            child = new ManagedProcess({ ...launch, cwd: request.cwd, environment: childEnvironment(environment) });
        }
        catch {
            resolveResult(failed(request, 'Unable to start the agent supervisor.'));
            return;
        }
        const frame = Buffer.allocUnsafe(MAX_AGENT_FRAME_BYTES);
        let frameBytes = 0, output = '';
        let outputBytes = 0, wireBytes = 0, stderrBytes = 0;
        let stopReason, failure;
        let forcedStatus;
        let settled = false, closingAfterCompletion = false;
        let graceTimer, forceTimer;
        const finish = (status, error) => {
            if (settled)
                return;
            settled = true;
            clearTimeout(timer);
            if (graceTimer)
                clearTimeout(graceTimer);
            if (forceTimer)
                clearTimeout(forceTimer);
            request.signal?.removeEventListener('abort', onAbort);
            resolveResult({ agent: request.agent, status, output, durationMs: Math.round(performance.now() - started),
                ...(error ? { error } : {}), ...(stopReason ? { stopReason } : {}) });
        };
        const terminate = (status, error) => {
            if (forcedStatus || settled)
                return;
            forcedStatus = status;
            failure = error;
            try {
                protocol.cancel?.(control);
            }
            catch { /* Killing the tree is authoritative. */ }
            child.kill();
            forceTimer = setTimeout(() => {
                void child.forceKill().catch(() => { forcedStatus = 'failed'; failure = PROCESS_CLEANUP_ERROR; })
                    .then(() => protocol.settle?.()).then(() => finish(forcedStatus ?? status, failure ?? error), () => finish('failed', PROCESS_CLEANUP_ERROR));
            }, 3000);
        };
        const control = {
            send(message) {
                if (settled || (forcedStatus && message.method !== 'session/cancel') || child.stdin.destroyed || child.stdin.writableEnded)
                    return;
                const encoded = JSON.stringify(message) + '\n';
                if (Buffer.byteLength(encoded) > 8 * 1024 * 1024 || child.stdin.writableLength + Buffer.byteLength(encoded) > 16 * 1024 * 1024) {
                    terminate('failed', 'Agent stopped reading bounded client replies; its input queue limit was reached.');
                    return;
                }
                child.stdin.write(encoded);
            },
            append(text) {
                if (stopReason) {
                    terminate('failed', 'Agent returned assistant text after terminal completion.');
                    return;
                }
                outputBytes += Buffer.byteLength(text, 'utf8');
                if (outputBytes > maxBytes) {
                    terminate('failed', 'Agent answer exceeded its configured byte limit.');
                    return;
                }
                output += text;
            },
            replace(text) {
                if (stopReason) {
                    terminate('failed', 'Agent returned duplicate final output.');
                    return;
                }
                if (Buffer.byteLength(text, 'utf8') > maxBytes) {
                    terminate('failed', 'Agent answer exceeded its configured byte limit.');
                    return;
                }
                output = text;
                outputBytes = Buffer.byteLength(text, 'utf8');
            },
            complete(reason) {
                if (stopReason || !/^[a-z_]{1,80}$/.test(reason)) {
                    terminate('failed', 'Agent returned an invalid or duplicate completion.');
                    return;
                }
                stopReason = reason;
                if (protocol.closeAfterCompletion)
                    graceTimer = setTimeout(() => { closingAfterCompletion = true; child.kill(); }, 500);
            },
            fail(error) { terminate('failed', error); },
            endInput(text) { if (!child.stdin.destroyed && !child.stdin.writableEnded)
                child.stdin.end(text); },
        };
        const onAbort = () => terminate('cancelled', 'Agent request cancelled.');
        const timer = setTimeout(() => terminate('timeout', 'Agent exceeded its time limit.'), Math.min(request.timeoutMs, 2_147_483_647));
        request.signal?.addEventListener('abort', onAbort, { once: true });
        const line = (text) => {
            if (!text.trim() || forcedStatus || settled)
                return;
            let value;
            try {
                value = JSON.parse(text);
            }
            catch {
                terminate('failed', 'Agent returned malformed or partial JSON.');
                return;
            }
            if (!isObject(value)) {
                terminate('failed', 'Agent returned an invalid protocol event.');
                return;
            }
            try {
                protocol.event(value, control);
            }
            catch {
                terminate('failed', 'Agent returned an invalid protocol response.');
            }
        };
        const readFrame = () => {
            if (forcedStatus || settled || frameBytes === 0)
                return;
            const bytes = frame.subarray(0, frameBytes);
            if (!isUtf8(bytes)) {
                terminate('failed', 'Agent protocol frame contains invalid UTF-8.');
                return;
            }
            try {
                checkAgentJsonStructure(bytes);
            }
            catch (error) {
                terminate('failed', error instanceof Error ? error.message : 'Agent protocol frame exceeds its structural limit.');
                return;
            }
            line(bytes.toString('utf8'));
        };
        child.stdout.on('data', (chunk) => {
            wireBytes += chunk.length;
            if (wireBytes > maxWireBytes) {
                terminate('failed', 'Agent protocol traffic exceeded its bounded transport allowance.');
                return;
            }
            let offset = 0;
            while (offset < chunk.length && !forcedStatus && !settled) {
                const newline = chunk.indexOf(0x0a, offset);
                const end = newline < 0 ? chunk.length : newline;
                const count = end - offset;
                if (frameBytes + count > MAX_AGENT_FRAME_BYTES) {
                    terminate('failed', 'Agent protocol frame exceeded 4 MiB.');
                    return;
                }
                chunk.copy(frame, frameBytes, offset, end);
                frameBytes += count;
                if (newline < 0)
                    break;
                readFrame();
                frameBytes = 0;
                offset = newline + 1;
            }
        });
        child.stderr.on('data', (chunk) => { stderrBytes += chunk.length; if (stderrBytes > 64 * 1024)
            terminate('failed', 'Agent diagnostics exceeded their bounded transport allowance.'); });
        child.on('error', error => {
            if (error instanceof ProcessCleanupError) {
                forcedStatus = 'failed';
                failure = PROCESS_CLEANUP_ERROR;
                return;
            }
            terminate('failed', 'Unable to start the configured agent executable. Run council doctor.');
        });
        child.once('close', async (code, signal) => {
            await protocol.settle?.();
            if (settled)
                return;
            if (forcedStatus) {
                finish(forcedStatus, failure);
                return;
            }
            readFrame();
            frameBytes = 0;
            if (forcedStatus) {
                finish(forcedStatus, failure);
                return;
            }
            if (code !== 0 && !(closingAfterCompletion && code === null)) {
                finish('failed', signal ? 'Agent was terminated before successful completion.'
                    : code === 5 ? 'Agent permission request was denied or could not be answered; the task is incomplete.' : `Agent exited with code ${code ?? 'unknown'}. Provider diagnostics were withheld.`);
                return;
            }
            if (stopReason === 'cancelled') {
                finish('cancelled', 'Agent cancelled its turn.');
                return;
            }
            if (stopReason !== 'end_turn') {
                finish('failed', stopReason ? `Agent stopped with ${stopReason}; the task is incomplete.` : 'Agent exited without a successful terminal completion.');
                return;
            }
            if (!output.trim()) {
                finish('failed', 'Agent returned an empty answer.');
                return;
            }
            const providerFailure = providerAnswerFailure(output);
            if (providerFailure) {
                output = '';
                finish('failed', providerFailure);
                return;
            }
            finish('completed');
        });
        if (request.signal?.aborted)
            onAbort();
        else
            try {
                protocol.start(control);
            }
            catch {
                terminate('failed', 'Unable to initialize the agent protocol.');
            }
    });
}
class CodexProtocol {
    prompt;
    active = false;
    constructor(prompt) {
        this.prompt = prompt;
    }
    start(control) { control.endInput(this.prompt); }
    event(event, control) {
        if (typeof event.type !== 'string') {
            control.fail('Codex returned an invalid JSON event.');
            return;
        }
        if (event.type === 'error' || event.type === 'turn.failed') {
            control.fail(acpErrorSummary(isObject(event.error) ? event.error : { message: event.message }));
            return;
        }
        if (event.type === 'turn.started') {
            if (this.active)
                control.fail('Codex returned duplicate turn start.');
            this.active = true;
        }
        if (event.type === 'item.completed' && isObject(event.item) && event.item.type === 'agent_message') {
            if (!this.active || typeof event.item.text !== 'string') {
                control.fail('Codex returned assistant text outside the active turn.');
                return;
            }
            control.append(event.item.text);
        }
        if (event.type === 'turn.completed') {
            if (!this.active)
                control.fail('Codex returned completion without an active turn.');
            else
                control.complete('end_turn');
        }
    }
}
class ClaudeProtocol {
    request;
    constructor(request) {
        this.request = request;
    }
    start(control) { control.endInput(this.request.prompt); }
    event(event, control) {
        if (typeof event.type !== 'string') {
            control.fail('Claude returned an invalid JSON event.');
            return;
        }
        if (event.type === 'error') {
            control.fail(acpErrorSummary(isObject(event.error) ? event.error : { message: event.message }));
            return;
        }
        if (event.type === 'assistant' && !event.parent_tool_use_id && isObject(event.message) && Array.isArray(event.message.content)) {
            for (const block of event.message.content) {
                if (!isObject(block))
                    continue;
                if (block.type === 'text' && typeof block.text === 'string')
                    control.append(block.text);
                if (block.type === 'tool_use' && this.request.permission === 'read' && !['Read', 'Glob', 'Grep'].includes(String(block.name)))
                    control.fail('Claude requested a tool outside the read-mode allowlist.');
            }
        }
        if (event.type === 'result') {
            if (typeof event.is_error !== 'boolean' || (event.subtype === 'success' && typeof event.result !== 'string')) {
                control.fail('Claude returned an invalid terminal result.');
                return;
            }
            if (event.is_error === true || event.subtype !== 'success') {
                control.fail(acpErrorSummary({ message: typeof event.result === 'string' ? event.result : event.subtype }));
                return;
            }
            if (event.permission_denials !== undefined && !Array.isArray(event.permission_denials)) {
                control.fail('Claude returned invalid terminal permission denials.');
                return;
            }
            if (Array.isArray(event.permission_denials) && event.permission_denials.length > 0) {
                control.fail('Claude tool permission was denied; the task is incomplete.');
                return;
            }
            control.replace(event.result);
            control.complete('end_turn');
        }
    }
}
class ExternalAcpxProtocol {
    prompt;
    promptId;
    sessionId;
    constructor(prompt) {
        this.prompt = prompt;
    }
    start(control) { control.endInput(this.prompt); }
    event(event, control) {
        if (!validAcpEnvelope(event)) {
            control.fail('External ACPX returned an invalid JSON-RPC response envelope.');
            return;
        }
        if (isObject(event.error)) {
            control.fail(acpErrorSummary(event.error));
            return;
        }
        if (event.method === 'session/prompt' && isObject(event.params)) {
            if (this.promptId !== undefined || (typeof event.id !== 'string' && !(typeof event.id === 'number' && Number.isSafeInteger(event.id)))
                || typeof event.params.sessionId !== 'string' || !event.params.sessionId) {
                control.fail('External ACPX returned an invalid prompt sequence.');
                return;
            }
            this.promptId = event.id;
            this.sessionId = typeof event.params.sessionId === 'string' ? event.params.sessionId : undefined;
        }
        if (event.method === 'session/update' && isObject(event.params) && isObject(event.params.update)) {
            const update = event.params.update;
            if (update.sessionUpdate !== 'agent_message_chunk' || !isObject(update.content) || update.content.type !== 'text' || typeof update.content.text !== 'string')
                return;
            if (this.promptId === undefined || (this.sessionId !== undefined && event.params.sessionId !== this.sessionId)) {
                control.fail('External ACPX returned text outside the active prompt.');
                return;
            }
            control.append(update.content.text);
        }
        if (isObject(event.result) && typeof event.result.stopReason === 'string') {
            if (this.promptId === undefined || event.id !== this.promptId) {
                control.fail('External ACPX returned an invalid completion sequence.');
                return;
            }
            control.complete(event.result.stopReason);
        }
    }
}
function nativeArguments(agent, request) {
    const model = request.model ?? agent.model;
    if (agent.profile === 'codex')
        return ['--ask-for-approval', 'never', 'exec', '--json', '--color', 'never', '--skip-git-repo-check', '--ephemeral',
            '--sandbox', request.permission === 'write' ? 'workspace-write' : 'read-only', '--cd', resolve(request.cwd), ...(model ? ['--model', model] : []), '-'];
    if (agent.profile === 'claude') {
        const tools = request.permission === 'read' ? 'Read,Glob,Grep' : 'Read,Glob,Grep,Edit,Write,Bash';
        return ['--print', '--output-format', 'stream-json', '--verbose', '--no-session-persistence', '--permission-mode', 'dontAsk',
            '--tools', tools, '--allowedTools', tools, '--disallowedTools', 'mcp__*', '--strict-mcp-config', '--mcp-config', '{"mcpServers":{}}',
            '--settings', '{"disableAllHooks":true}', ...(model ? ['--model', model] : [])];
    }
    throw new Error('Native transport is supported only for Codex and Claude. Configure ACP transport for this profile.');
}
function defaultAcp(agent) {
    if (agent.profile === 'deepseek-harness')
        return { command: 'dsh', args: ['--profile', 'acp'] };
    if (agent.profile === 'junie')
        return { command: 'junie', args: ['--acp=true'] };
    if (agent.profile === 'antigravity')
        return { command: process.platform === 'win32' ? 'agy_acp_server.exe' : 'agy_acp_server.par', args: process.platform === 'linux' ? ['--uid='] : [] };
    throw new Error('ACP transport for Codex or Claude requires an explicitly configured ACP adapter command.');
}
export class NativeRunner {
    config;
    options;
    constructor(config, options = {}) {
        this.config = config;
        this.options = options;
    }
    async launch(agent, request, signal) {
        const transport = agent.transport ?? (['codex', 'claude'].includes(agent.profile) ? 'native' : 'acp');
        const environment = this.options.environment ?? process.env;
        if (transport === 'native')
            return { transport, launch: await resolveLaunch(agent.command ?? agent.profile, [...agent.args ?? [], ...nativeArguments(agent, request)], request.cwd, environment, signal) };
        const defaults = agent.command ? { command: agent.command, args: agent.args ?? [] } : defaultAcp(agent);
        return { transport, launch: await resolveLaunch(defaults.command, agent.args ?? defaults.args, request.cwd, environment, signal) };
    }
    async run(request) {
        const invalid = validateRequest(this.config, request);
        if (invalid)
            return invalid;
        const agent = this.config.agents[request.agent];
        return await withBudget(request, async (signal, remaining) => {
            const scoped = { ...request, signal };
            const { launch, transport } = await this.launch(agent, scoped, signal);
            scoped.timeoutMs = remaining();
            const protocol = transport === 'acp' ? new AcpProtocol(scoped, request.model ?? agent.model) : agent.profile === 'codex' ? new CodexProtocol(request.prompt) : new ClaudeProtocol(scoped);
            return await runProtocol(launch, protocol, scoped, this.config, this.options.environment ?? process.env);
        });
    }
    async doctor() {
        return await Promise.all(Object.entries(this.config.agents).map(async ([name, agent]) => {
            if (!agent.enabled)
                return { agent: name, enabled: false, available: false, detail: 'Disabled in Council configuration.' };
            const controller = new AbortController();
            const timer = setTimeout(() => controller.abort(), 5000);
            try {
                const { launch, transport } = await this.launch(agent, { agent: name, cwd: this.config.workspaceRoot, prompt: '', permission: 'read', timeoutMs: 1000 }, controller.signal);
                return { agent: name, enabled: true, available: true, detail: `Local ${transport} executable found (${basename(launch.command)}). Authentication, model access, and protocol readiness were not tested.` };
            }
            catch (error) {
                return { agent: name, enabled: true, available: false, detail: controller.signal.aborted ? 'Agent executable lookup exceeded its five-second limit.' : error instanceof Error ? error.message : 'Agent executable unavailable.' };
            }
            finally {
                clearTimeout(timer);
            }
        }));
    }
}
/** Compatibility backend for an independently installed external ACPX CLI. */
export class AcpxRunner {
    config;
    options;
    constructor(config, options = {}) {
        this.config = config;
        this.options = options;
    }
    async executable(cwd, signal) { return await resolveLaunch(this.options.acpxEntrypoint ?? 'acpx', [], cwd, this.options.environment ?? process.env, signal); }
    async run(request) {
        const invalid = validateRequest(this.config, request);
        if (invalid)
            return invalid;
        const agent = this.config.agents[request.agent];
        if (!/^[a-zA-Z0-9][a-zA-Z0-9_.-]*$/.test(agent.profile))
            return failed(request, 'Invalid external ACPX agent profile.');
        return await withBudget(request, async (signal, remaining) => {
            const scoped = { ...request, signal };
            const launch = await this.executable(request.cwd, signal);
            scoped.timeoutMs = remaining();
            const write = request.permission === 'write', model = request.model ?? agent.model;
            launch.args.push('--format', 'json', '--json-strict', '--cwd', resolve(request.cwd), write ? '--approve-all' : '--deny-all', '--permission-policy', JSON.stringify({ defaultAction: write ? 'approve' : 'deny' }), '--non-interactive-permissions', 'deny', '--timeout', String(scoped.timeoutMs / 1000), ...(model ? ['--model', model] : []), agent.profile, 'exec', '--file', '-');
            return await runProtocol(launch, new ExternalAcpxProtocol(request.prompt), scoped, this.config, this.options.environment ?? process.env);
        });
    }
    async doctor() {
        const controller = new AbortController();
        const timer = setTimeout(() => controller.abort(), 5000);
        let available = true;
        try {
            await this.executable(this.config.workspaceRoot, controller.signal);
        }
        catch {
            available = false;
        }
        finally {
            clearTimeout(timer);
        }
        return Object.entries(this.config.agents).map(([agent, value]) => ({ agent, enabled: value.enabled, available: value.enabled && available,
            detail: !value.enabled ? 'Disabled in Council configuration.' : available ? 'External ACPX executable found. Underlying adapters, authentication, and protocol readiness were not tested.'
                : 'External ACPX executable not found. Install it separately or use the native backend.' }));
    }
}
export class MockRunner {
    config;
    constructor(config) {
        this.config = config;
    }
    async run(request) {
        const invalid = validateRequest(this.config, request);
        if (invalid)
            return invalid;
        const digest = createHash('sha256').update(request.prompt).digest('hex').slice(0, 12);
        return { agent: request.agent, status: 'completed', durationMs: 0, stopReason: 'end_turn',
            output: `[MOCK ${request.agent}] Offline simulation ${digest}.\nReviewed the supplied task. Suggested action: keep changes scoped, verify acceptance criteria, and report unresolved assumptions. No provider was called and no files were changed.` };
    }
    async doctor() {
        return Object.entries(this.config.agents).map(([agent, value]) => ({ agent, enabled: value.enabled, available: value.enabled,
            detail: value.enabled ? 'Deterministic offline mock; no provider calls.' : 'Disabled in Council configuration.' }));
    }
}
export function createRunner(config) {
    return config.backend === 'mock' ? new MockRunner(config) : config.backend === 'acpx' ? new AcpxRunner(config) : new NativeRunner(config);
}
//# sourceMappingURL=runner.js.map