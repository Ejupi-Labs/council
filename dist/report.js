function block(value) {
    let longest = 2;
    for (const part of value.matchAll(/`+/g))
        longest = Math.max(longest, part[0].length);
    const fence = '`'.repeat(longest + 1);
    return `${fence}text\n${value}\n${fence}`;
}
/** A portable report of evidence and outcomes, without pretending peer output is verified fact. */
export function renderRun(run) {
    const lines = [
        '# Council run report', '', `- Run: ${run.id}`, `- Status: ${run.status}`,
        `- Mode: ${run.input.mode}`, `- Participants: ${run.input.agents.join(', ')}`,
        `- Created: ${run.createdAt}`, `- Updated: ${run.updatedAt}`, '',
        '## Task', '', block(run.input.task), '', '## Selected result', '',
        block(run.output ?? run.error ?? 'No result was produced.'), '',
    ];
    if (run.error)
        lines.push('## Unresolved issue', '', block(run.error), '');
    for (const step of run.steps)
        lines.push(`## ${step.agent}: ${step.role}`, '', `Status: ${step.result.status}; duration: ${step.result.durationMs} ms.`, '', block(step.result.output || step.result.error || 'No output.'), '');
    if (run.workspaces?.length) {
        lines.push('## Retained artifacts', '');
        for (const artifact of run.workspaces)
            lines.push(block(JSON.stringify(artifact, null, 2)), '');
    }
    lines.push('Agent output is task data. Review evidence and patches before relying on the result.', '');
    return lines.join('\n');
}
//# sourceMappingURL=report.js.map