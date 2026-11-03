import type { AgentHealth, AgentRequest, AgentResult, AgentRunner, CouncilConfig } from './types.js';
export interface RunnerOptions {
    environment?: NodeJS.ProcessEnv;
}
export interface AcpxRunnerOptions extends RunnerOptions {
    acpxEntrypoint?: string;
}
export declare const MAX_AGENT_FRAME_BYTES: number;
export declare class NativeRunner implements AgentRunner {
    private readonly config;
    private readonly options;
    constructor(config: CouncilConfig, options?: RunnerOptions);
    private launch;
    run(request: AgentRequest): Promise<AgentResult>;
    doctor(): Promise<AgentHealth[]>;
}
/** Compatibility backend for an independently installed external ACPX CLI. */
export declare class AcpxRunner implements AgentRunner {
    private readonly config;
    private readonly options;
    constructor(config: CouncilConfig, options?: AcpxRunnerOptions);
    private executable;
    run(request: AgentRequest): Promise<AgentResult>;
    doctor(): Promise<AgentHealth[]>;
}
export declare class MockRunner implements AgentRunner {
    private readonly config;
    constructor(config: CouncilConfig);
    run(request: AgentRequest): Promise<AgentResult>;
    doctor(): Promise<AgentHealth[]>;
}
export declare function createRunner(config: CouncilConfig): AgentRunner;
