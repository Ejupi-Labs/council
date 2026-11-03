export const MAX_RUN_SUMMARY_BYTES = 64 * 1024;
export const MAX_COMPACT_EVENT_DATA_BYTES = 8 * 1024;
export function summarizeRun(run, revision = 1) {
    let truncated = run.steps.length > 64 || (run.error?.length ?? 0) > 2048 || run.steps.some(step => (step.result.error?.length ?? 0) > 1024);
    const workspaces = run.workspaces?.map(artifact => {
        const copy = { ...artifact };
        if (typeof copy.exportError === 'string' && copy.exportError.length > 1024) {
            copy.exportError = copy.exportError.slice(0, 1024);
            truncated = true;
        }
        return copy;
    });
    const summary = {
        id: run.id, status: run.status, mode: run.input.mode, agents: [...run.input.agents],
        createdAt: run.createdAt, updatedAt: run.updatedAt, revision,
        steps: run.steps.slice(0, 64).map(step => ({ id: step.id, agent: step.agent, role: step.role, status: step.result.status, durationMs: step.result.durationMs, error: step.result.error?.slice(0, 1024) })),
        output: run.output?.slice(0, 16_000), outputTruncated: (run.output?.length ?? 0) > 16_000,
        error: run.error?.slice(0, 2048), workspaces, fullResultResource: `council://runs/${run.id}`,
    };
    if (truncated)
        summary.summaryTruncated = true;
    const bytes = (value) => Buffer.byteLength(JSON.stringify(value), 'utf8');
    if (bytes(summary) <= MAX_RUN_SUMMARY_BYTES)
        return summary;
    summary.summaryTruncated = true;
    summary.output = summary.output?.slice(0, 1000);
    summary.outputTruncated = (run.output?.length ?? 0) > (summary.output?.length ?? 0);
    summary.error = summary.error?.slice(0, 256);
    for (const step of summary.steps)
        delete step.error;
    while (bytes(summary) > MAX_RUN_SUMMARY_BYTES) {
        const artifact = summary.workspaces?.at(-1);
        const step = summary.steps.at(-1);
        // Omit complete entries; paths and IDs on retained entries remain exact.
        if (artifact && (!step || bytes(artifact) >= bytes(step))) {
            summary.workspaces.pop();
            summary.workspacesTruncated = true;
        }
        else if (step)
            summary.steps.pop();
        else {
            // Public storage IDs are bounded to 256 UTF-8 bytes; this fallback bounds ancillary fields too.
            summary.agents = [];
            summary.createdAt = summary.createdAt.slice(0, 64);
            summary.updatedAt = summary.updatedAt.slice(0, 64);
            delete summary.output;
            delete summary.error;
            summary.outputTruncated = Boolean(run.output?.length);
            break;
        }
    }
    return summary;
}
/** Event discovery never needs complete peer responses or repeated task/context briefs. */
export function summarizeEvent(event) {
    let source = event.data;
    if (source && typeof source === 'object' && !Array.isArray(source)) {
        const data = { ...source };
        if (typeof data.output === 'string') {
            data.outputPreview = data.output.slice(0, 1000);
            data.outputTruncated = data.output.length > 1000;
            delete data.output;
        }
        if (data.result && typeof data.result === 'object' && !Array.isArray(data.result)) {
            const result = data.result;
            data.result = { agent: result.agent, status: result.status, durationMs: result.durationMs, error: result.error };
        }
        if (data.input && typeof data.input === 'object' && !Array.isArray(data.input)) {
            const input = data.input;
            data.input = { mode: input.mode, agents: input.agents, judge: input.judge, permission: input.permission, rounds: input.rounds, maxCalls: input.maxCalls };
        }
        source = data;
    }
    let nodes = 0;
    let remainingCharacters = 6000;
    const compact = (value, depth) => {
        if (++nodes > 256 || depth > 5)
            return '[summary truncated]';
        if (typeof value === 'string') {
            const result = value.slice(0, Math.max(0, Math.min(1000, remainingCharacters)));
            remainingCharacters -= result.length;
            return result;
        }
        if (Array.isArray(value))
            return value.slice(0, 16).map(item => compact(item, depth + 1));
        if (value !== null && typeof value === 'object') {
            return Object.fromEntries(Object.entries(value).slice(0, 16).map(([key, item]) => [key.slice(0, 128), compact(item, depth + 1)]));
        }
        return value;
    };
    let data = compact(source, 0);
    if (Buffer.byteLength(JSON.stringify(data) ?? 'null') > MAX_COMPACT_EVENT_DATA_BYTES)
        data = { dataTruncated: true, note: 'Read the full event or run resource for this payload.' };
    return { ...event, data };
}
//# sourceMappingURL=summary.js.map