import type { AgentRequest } from './types.js';
export type JsonObject = Record<string, unknown>;
export declare const isObject: (value: unknown) => value is JsonObject;
/** A response cannot carry both success and failure, including malformed error values. */
export declare function validAcpEnvelope(event: JsonObject): boolean;
export interface ProtocolControl {
    send(message: JsonObject): void;
    append(text: string): void;
    replace(text: string): void;
    complete(stopReason: string): void;
    fail(error: string): void;
    endInput(text?: string): void;
}
export interface AgentProtocol {
    start(control: ProtocolControl): void;
    event(event: JsonObject, control: ProtocolControl): void;
    cancel?(control: ProtocolControl): void;
    settle?(): Promise<void>;
    readonly closeAfterCompletion?: boolean;
}
export declare function acpErrorSummary(error: JsonObject): string;
/** Narrow ACP v1 client with scoped text files, no credentials, and no terminal service. */
export declare class AcpProtocol implements AgentProtocol {
    private readonly request;
    private readonly model?;
    readonly closeAfterCompletion = true;
    private stage;
    private sessionId?;
    private session;
    private readonly files;
    private readonly fileAbort;
    private readonly pendingFiles;
    constructor(request: AgentRequest, model?: string | undefined);
    start(control: ProtocolControl): void;
    cancel(control: ProtocolControl): void;
    settle(): Promise<void>;
    private fileRequest;
    private prompt;
    private selectModel;
    event(event: JsonObject, control: ProtocolControl): void;
}
