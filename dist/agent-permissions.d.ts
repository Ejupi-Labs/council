import type { AgentConfig, CouncilConfig, Permission } from './types.js';
/** The native Claude declaration, prompt and protocol guard share this allowlist. */
export declare const CLAUDE_READ_TOOLS: readonly string[];
export declare function agentTransport(agent: AgentConfig): 'native' | 'acp';
export declare function taskPermissionGuidance(config: Pick<CouncilConfig, 'backend' | 'agents'>, alias: string, permission: Permission): string;
