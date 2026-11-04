import { acpErrorSummary, isObject } from './acp.js';
function identity(value) {
    return typeof value === 'string' && value.length > 0 && value.length <= 256 && !/[\x00-\x20\x7f]/.test(value);
}
/** One stdin turn using the documented Antigravity CLI stream-json protocol. */
export class AntigravityProtocol {
    prompt;
    stage = 'initialize';
    started = false;
    conversationId;
    output = '';
    assistantSteps = 0;
    steps = new Map();
    constructor(prompt) {
        this.prompt = prompt;
    }
    start(control) {
        if (this.started) {
            this.fail(control, 'Antigravity protocol was started more than once.');
            return;
        }
        this.started = true;
        control.endInput(JSON.stringify({ event: 'user', message: { content: this.prompt } }) + '\n');
    }
    cancel() { if (this.stage !== 'complete' && this.stage !== 'failed')
        this.stage = 'cancelled'; }
    diagnostic(text, control) {
        if (text.trim())
            this.fail(control, 'Antigravity emitted diagnostics; successful completion could not be verified. Provider diagnostics were withheld.');
    }
    fail(control, message) { this.stage = 'failed'; control.fail(message); }
    event(event, control) {
        if (!this.started || this.stage === 'complete' || this.stage === 'failed' || this.stage === 'cancelled') {
            this.fail(control, 'Antigravity returned an event outside the active turn.');
            return;
        }
        if (typeof event.event !== 'string' || !['init', 'step_update', 'result'].includes(event.event)
            || ['init', 'step_update', 'result'].some(key => key !== event.event && Object.hasOwn(event, key))) {
            this.fail(control, 'Antigravity returned an invalid or unsupported protocol event.');
            return;
        }
        if (Object.hasOwn(event, 'error')) {
            this.fail(control, 'Antigravity returned a protocol error. Provider diagnostics were withheld.');
            return;
        }
        if (event.event === 'init') {
            if (this.stage !== 'initialize' || !identity(event.conversation_id) || !isObject(event.init)
                || (Object.hasOwn(event.init, 'conversation_id') && event.init.conversation_id !== event.conversation_id)) {
                this.fail(control, 'Antigravity returned an invalid or duplicate conversation initialization.');
                return;
            }
            this.conversationId = event.conversation_id;
            this.stage = 'active';
            return;
        }
        const payload = event[event.event];
        if (this.stage !== 'active' || !isObject(payload) || payload.conversation_id !== this.conversationId
            || (Object.hasOwn(event, 'conversation_id') && event.conversation_id !== this.conversationId)) {
            this.fail(control, 'Antigravity returned an event outside the selected conversation.');
            return;
        }
        if (event.event === 'step_update') {
            if (!Number.isSafeInteger(payload.step_index) || payload.step_index < 0
                || !['ACTIVE', 'DONE'].includes(payload.state)
                || typeof payload.step_type !== 'string' || !payload.step_type
                || (Object.hasOwn(payload, 'text_delta') && typeof payload.text_delta !== 'string')) {
                this.fail(control, 'Antigravity returned an invalid step update.');
                return;
            }
            if (Object.hasOwn(payload, 'error') || (Object.hasOwn(payload, 'tool_info') && !isObject(payload.tool_info))) {
                this.fail(control, 'Antigravity returned an invalid or failed tool step; the task is incomplete.');
                return;
            }
            const tool = isObject(payload.tool_info) ? payload.tool_info : {};
            if (Object.hasOwn(tool, 'error')) {
                this.fail(control, 'Antigravity tool execution failed or was denied; the task is incomplete.');
                return;
            }
            if ([payload.tool_name, tool.name].some(name => typeof name === 'string'
                && /^(?:ask_permission|ask_custom_permission|ask_question|request_permission|request_approval|ask_user(?:_question)?|AskUserQuestion)$/i.test(name))) {
                this.fail(control, 'Antigravity requires a tool approval or human interaction that unattended Council cannot answer.');
                return;
            }
            const index = payload.step_index;
            const previous = this.steps.get(index);
            if (previous?.done || (previous && previous.type !== payload.step_type)) {
                this.fail(control, 'Antigravity returned a duplicate step or changed its identity.');
                return;
            }
            if (!previous && payload.step_type === 'agent_response')
                this.assistantSteps++;
            this.steps.set(index, { type: payload.step_type, done: payload.state === 'DONE' });
            if (payload.step_type === 'agent_response' && typeof payload.text_delta === 'string') {
                this.output += payload.text_delta;
                control.append(payload.text_delta);
            }
            return;
        }
        if (typeof payload.status !== 'string' || typeof payload.response !== 'string') {
            this.fail(control, 'Antigravity returned an invalid terminal result.');
            return;
        }
        if (payload.status === 'CANCELED' || payload.status === 'CANCELLED') {
            if (Object.hasOwn(payload, 'error') && typeof payload.error !== 'string') {
                this.fail(control, 'Antigravity returned an invalid cancellation result.');
                return;
            }
            this.stage = 'complete';
            control.complete('cancelled');
            return;
        }
        if (payload.status !== 'SUCCESS') {
            this.fail(control, payload.status === 'ERROR' ? acpErrorSummary({ message: payload.error })
                : 'Antigravity did not reach a successful terminal state; the task is incomplete.');
            return;
        }
        if (Object.hasOwn(payload, 'error') || (Object.hasOwn(payload, 'finished_reason') && payload.finished_reason !== '')) {
            this.fail(control, 'Antigravity returned a partial or contradictory terminal result; the task is incomplete.');
            return;
        }
        if (this.assistantSteps === 0 || [...this.steps.values()].some(step => !step.done)
            || !payload.response.trim() || payload.response !== this.output) {
            this.fail(control, 'Antigravity terminal response does not match its completed assistant stream.');
            return;
        }
        this.stage = 'complete';
        control.complete('end_turn');
    }
}
//# sourceMappingURL=antigravity.js.map