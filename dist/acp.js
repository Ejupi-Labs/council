import { AcpFiles } from './acp-files.js';
import { VERSION } from './version.js';
export const isObject = (value) => typeof value === 'object' && value !== null && !Array.isArray(value);
export function acpErrorSummary(error) {
    const message = typeof error.message === 'string' ? error.message : '';
    const detail = isObject(error.data) && typeof error.data.detailCode === 'string' ? error.data.detailCode : '';
    if (/ETARGET|No matching version found/i.test(message))
        return 'Agent adapter dependency is unavailable from npm. Install a compatible external adapter and check the configured executable.';
    if (/Cannot apply --model|did not advertise that model|model_not_found/i.test(message))
        return 'Agent adapter rejected the configured model. Select a model advertised by that adapter in Council configuration.';
    if (/ENOENT|executable not found|command not found/i.test(message))
        return 'Agent launch executable is missing. Run council doctor and check command and args.';
    if (/auth(?:entication)?[_ ](?:required|failed)|not logged in|login required|unauthorized|auth_required/i.test(message))
        return 'Agent authentication is unavailable. Sign in through the agent CLI outside Council, then retry.';
    if (detail === 'AGENT_STARTUP_FAILED')
        return 'Agent adapter failed before ACP initialization. Run council doctor and verify the configured adapter can start.';
    const code = typeof error.code === 'number' ? ` (${error.code})` : '';
    return `Agent returned a protocol error${code}. Provider diagnostics were withheld.`;
}
/** Narrow ACP v1 client with scoped text files, no credentials, and no terminal service. */
export class AcpProtocol {
    request;
    model;
    closeAfterCompletion = true;
    stage = 'initialize';
    sessionId;
    session = {};
    files;
    fileAbort = new AbortController();
    pendingFiles = new Map();
    constructor(request, model) {
        this.request = request;
        this.model = model;
        this.files = new AcpFiles(request.cwd, request.permission);
    }
    start(control) {
        control.send({ jsonrpc: '2.0', id: 'council-initialize', method: 'initialize', params: {
                protocolVersion: 1, clientInfo: { name: 'council', version: VERSION },
                clientCapabilities: { fs: { readTextFile: true, writeTextFile: this.request.permission === 'write' }, terminal: false },
            } });
    }
    cancel(control) {
        this.fileAbort.abort();
        if (this.sessionId)
            control.send({ jsonrpc: '2.0', method: 'session/cancel', params: { sessionId: this.sessionId } });
    }
    async settle() { await Promise.allSettled(this.pendingFiles.values()); }
    fileRequest(event, control) {
        const id = event.id;
        if ((typeof id !== 'string' && !(typeof id === 'number' && Number.isSafeInteger(id))) || this.pendingFiles.has(id)) {
            control.fail('ACP agent returned an invalid or duplicate client request identity.');
            return;
        }
        const params = isObject(event.params) ? event.params : {};
        if (this.stage !== 'prompt' || this.fileAbort.signal.aborted || params.sessionId !== this.sessionId) {
            control.send({ jsonrpc: '2.0', id, error: { code: -32602, message: 'File request is outside the active session prompt.' } });
            control.fail('ACP file request arrived outside the active session prompt.');
            return;
        }
        if (this.pendingFiles.size >= 8) {
            control.fail('ACP agent exceeded the eight outstanding client file request limit.');
            return;
        }
        const operation = (async () => {
            try {
                let result;
                if (event.method === 'fs/read_text_file')
                    result = await this.files.read({ path: params.path,
                        ...(params.line == null ? {} : { line: params.line }), ...(params.limit == null ? {} : { limit: params.limit }) }, this.fileAbort.signal);
                else {
                    await this.files.write({ path: params.path, content: params.content }, this.fileAbort.signal);
                    result = {};
                }
                if (!this.fileAbort.signal.aborted)
                    control.send({ jsonrpc: '2.0', id, result });
            }
            catch {
                if (this.fileAbort.signal.aborted)
                    return;
                control.send({ jsonrpc: '2.0', id, error: { code: -32000, message: 'Council file request was rejected by the project file policy.' } });
                control.fail('ACP file request was rejected by the project file policy; the task is incomplete.');
            }
        })().finally(() => { this.pendingFiles.delete(id); });
        this.pendingFiles.set(id, operation);
    }
    prompt(control) {
        this.stage = 'prompt';
        control.send({ jsonrpc: '2.0', id: 'council-prompt', method: 'session/prompt', params: {
                sessionId: this.sessionId, prompt: [{ type: 'text', text: this.request.prompt }],
            } });
    }
    selectModel(control) {
        if (!this.model) {
            this.prompt(control);
            return;
        }
        const options = Array.isArray(this.session.configOptions) ? this.session.configOptions.filter(isObject) : [];
        const modelOption = options.find(option => option.category === 'model' && typeof option.id === 'string');
        if (modelOption) {
            const pending = [{ value: modelOption.options, depth: 0 }];
            let advertised = false;
            let inspected = 0;
            while (pending.length && inspected++ < 10_000) {
                const item = pending.pop();
                if (item.depth > 4 || !Array.isArray(item.value))
                    continue;
                for (const option of item.value)
                    if (isObject(option)) {
                        if (option.value === this.model)
                            advertised = true;
                        if (Array.isArray(option.options))
                            pending.push({ value: option.options, depth: item.depth + 1 });
                    }
            }
            if (!advertised) {
                control.fail('Configured model was not advertised by the ACP agent. Select an advertised model; Council does not substitute one.');
                return;
            }
            this.stage = 'model';
            control.send({ jsonrpc: '2.0', id: 'council-model', method: 'session/set_config_option', params: { sessionId: this.sessionId, configId: modelOption.id, value: this.model } });
            return;
        }
        const models = isObject(this.session.models) ? this.session.models : undefined;
        if (!models || !Array.isArray(models.availableModels) || !models.availableModels.some(item => isObject(item) && item.modelId === this.model)) {
            control.fail('Configured model was not advertised by the ACP agent. Select an advertised model; Council does not substitute one.');
            return;
        }
        this.stage = 'model';
        control.send({ jsonrpc: '2.0', id: 'council-model', method: 'session/set_model', params: { sessionId: this.sessionId, modelId: this.model } });
    }
    event(event, control) {
        if (event.jsonrpc !== '2.0') {
            control.fail('Agent returned invalid ACP JSON-RPC.');
            return;
        }
        if (typeof event.method === 'string' && event.id !== undefined) {
            if (typeof event.id !== 'string' && !(typeof event.id === 'number' && Number.isSafeInteger(event.id))) {
                control.fail('ACP agent returned an invalid client request identity.');
                return;
            }
            if (event.method === 'fs/read_text_file' || event.method === 'fs/write_text_file') {
                this.fileRequest(event, control);
                return;
            }
            if (event.method !== 'session/request_permission') {
                control.send({ jsonrpc: '2.0', id: event.id, error: { code: -32601, message: 'Council does not provide this client capability.' } });
                control.fail('ACP agent requested an unsupported client capability. Terminal and interactive authentication services are disabled.');
                return;
            }
            const params = isObject(event.params) ? event.params : {};
            if (this.stage !== 'prompt' || params.sessionId !== this.sessionId) {
                control.send({ jsonrpc: '2.0', id: event.id, result: { outcome: { outcome: 'cancelled' } } });
                control.fail('ACP permission request arrived outside the active session prompt.');
                return;
            }
            const options = Array.isArray(params.options) ? params.options.filter(isObject) : [];
            const tool = isObject(params.toolCall) ? params.toolCall : {};
            const question = options.some(option => typeof option.optionId === 'string' && option.optionId.startsWith('interaction_'))
                || /AskUserQuestion|interaction_/i.test(typeof tool.title === 'string' ? tool.title : '')
                || (isObject(tool.rawInput) && /AskUserQuestion|interaction_/i.test(String(tool.rawInput.name ?? tool.rawInput.tool ?? '')));
            const allowed = this.request.permission === 'write' && !question;
            const selected = options.find(option => option.kind === (allowed ? 'allow_once' : 'reject_once'))
                ?? (!allowed ? options.find(option => option.kind === 'reject_always') : undefined);
            control.send({ jsonrpc: '2.0', id: event.id, result: { outcome: selected && typeof selected.optionId === 'string'
                        ? { outcome: 'selected', optionId: selected.optionId } : { outcome: 'cancelled' } } });
            if (!allowed || !selected)
                control.fail(question ? 'ACP agent requires a human interaction that unattended Council cannot answer.'
                    : this.request.permission === 'read' ? 'ACP tool permission was denied by read mode; the task is incomplete.' : 'ACP agent did not offer a one-time permission; the task is incomplete.');
            return;
        }
        if (isObject(event.error)) {
            control.fail(acpErrorSummary(event.error));
            return;
        }
        if (event.method === 'session/update' && isObject(event.params) && isObject(event.params.update)) {
            const update = event.params.update;
            if (update.sessionUpdate !== 'agent_message_chunk' || !isObject(update.content) || update.content.type !== 'text')
                return;
            if (this.stage !== 'prompt' || event.params.sessionId !== this.sessionId || typeof update.content.text !== 'string') {
                control.fail('ACP assistant text arrived outside the active session prompt.');
                return;
            }
            control.append(update.content.text);
            return;
        }
        if (event.id === undefined || typeof event.method === 'string')
            return;
        const expected = `council-${this.stage === 'complete' ? 'prompt' : this.stage}`;
        if (event.id !== expected || !isObject(event.result)) {
            control.fail('ACP agent returned an unexpected response or request ID.');
            return;
        }
        if (this.stage === 'initialize') {
            if (event.result.protocolVersion !== 1) {
                control.fail('ACP agent did not negotiate supported protocol version 1.');
                return;
            }
            this.stage = 'new';
            control.send({ jsonrpc: '2.0', id: 'council-new', method: 'session/new', params: { cwd: this.request.cwd, mcpServers: [] } });
        }
        else if (this.stage === 'new') {
            if (typeof event.result.sessionId !== 'string' || !event.result.sessionId) {
                control.fail('ACP agent returned no session identity.');
                return;
            }
            this.sessionId = event.result.sessionId;
            this.session = event.result;
            const modes = isObject(this.session.modes) ? this.session.modes : {};
            const available = Array.isArray(modes.availableModes) ? modes.availableModes.filter(isObject) : [];
            const plan = available.find(mode => mode.id === 'read-only' || mode.id === 'plan');
            if (this.request.permission === 'read' && plan) {
                this.stage = 'mode';
                control.send({ jsonrpc: '2.0', id: 'council-mode', method: 'session/set_mode', params: { sessionId: this.sessionId, modeId: plan.id } });
            }
            else
                this.selectModel(control);
        }
        else if (this.stage === 'mode')
            this.selectModel(control);
        else if (this.stage === 'model')
            this.prompt(control);
        else if (this.stage === 'prompt') {
            if (this.pendingFiles.size > 0) {
                control.fail('ACP agent completed while client file requests were still outstanding.');
                return;
            }
            if (typeof event.result.stopReason !== 'string' || !/^[a-z_]{1,80}$/.test(event.result.stopReason)) {
                control.fail('ACP agent returned no valid terminal stop reason.');
                return;
            }
            this.stage = 'complete';
            control.complete(event.result.stopReason);
            control.endInput();
        }
        else
            control.fail('ACP agent returned a duplicate terminal response.');
    }
}
//# sourceMappingURL=acp.js.map