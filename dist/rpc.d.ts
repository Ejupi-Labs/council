import type { Readable, Writable } from 'node:stream';
import { type JsonSchema } from './schema.js';
export interface ToolDefinition {
    name: string;
    description: string;
    inputSchema: JsonSchema;
    annotations: {
        readOnlyHint: boolean;
        destructiveHint: boolean;
        openWorldHint: boolean;
    };
}
type ToolHandler = (input: Record<string, any>, signal: AbortSignal) => unknown | Promise<unknown>;
/** Local stdio MCP tools/resources subset, verified against the official SDK client. */
export declare class CouncilMcpServer {
    private readonly shutdown;
    private readonly tools;
    private readonly pending;
    private initialized;
    private closed;
    private input?;
    private output?;
    private guard?;
    private writeQueue;
    private queuedBytes;
    private queuedFrames;
    private closePromise?;
    private pump?;
    private readonly outputAbort;
    private readonly inputError;
    private readonly outputError;
    private modeReader?;
    private runReader?;
    constructor(shutdown: () => Promise<void>);
    registerTool(definition: ToolDefinition, handler: ToolHandler): void;
    registerResources(modes: () => unknown, run: (id: string) => unknown): void;
    metrics(): {
        connected: boolean;
        pendingRequests: number;
        queuedResponseBytes: number;
        queuedResponseFrames: number;
        outputIsolated: boolean;
    };
    connect(input?: Readable, output?: Writable): Promise<void>;
    close(): Promise<void>;
    private send;
    private error;
    private receive;
    private dispatch;
}
export {};
