import { type CouncilConfig, type WorkflowInput } from './types.js';
import { type JsonSchema } from './schema.js';
export declare const backendDefinition: JsonSchema;
export declare const reasoningEffortDefinition: JsonSchema;
export declare const agentDefinition: JsonSchema;
export declare const configDefinition: JsonSchema;
export declare const workflowDefinition: JsonSchema;
export declare const configSchema: {
    parse(value: unknown): Omit<CouncilConfig, 'workspaceRoot'>;
};
export declare const workflowSchema: {
    parse(value: unknown): WorkflowInput;
};
export declare function parseBackend(value: unknown): CouncilConfig['backend'];
export declare function readJsonFile(filename: string, maxBytes?: number): unknown;
export declare function loadConfig(workspace?: string, overrides?: Partial<CouncilConfig>): CouncilConfig;
export declare function initConfig(workspace?: string, backend?: CouncilConfig['backend']): string;
export declare function parseWorkflow(value: unknown): WorkflowInput;
