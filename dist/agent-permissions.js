/** The native Claude declaration, prompt and protocol guard share this allowlist. */
export const CLAUDE_READ_TOOLS = Object.freeze(['Read', 'Glob', 'Grep']);
export function agentTransport(agent) {
    return agent.transport ?? (['codex', 'claude'].includes(agent.profile) ? 'native' : 'acp');
}
export function taskPermissionGuidance(config, alias, permission) {
    if (permission === 'write')
        return 'Make changes only in the provided isolated worktree.';
    const prefix = 'Read-only task: do not modify files.';
    const agent = config.agents[alias];
    if (config.backend !== 'native' || !agent)
        return prefix;
    const transport = agentTransport(agent);
    if (transport === 'native' && agent.profile === 'claude') {
        return `${prefix} Available tools for this native Claude turn: ${CLAUDE_READ_TOOLS.join(', ')}. `
            + 'Do not call Bash, Terminal, Task, TodoWrite, MCP tools or other tools outside that list, even for read-only checks. '
            + 'If the task needs a command or test, identify the required host-executed check and report it unverified; '
            + 'do not request broader permissions or claim that it ran.';
    }
    if (transport === 'acp') {
        return `${prefix} The Council ACP client exposes scoped text-file reads and no terminal service for this turn. `
            + 'Do not request terminal commands, writes or new tool approvals. '
            + 'Identify any required command or test as an unverified host-executed check.';
    }
    return prefix;
}
//# sourceMappingURL=agent-permissions.js.map