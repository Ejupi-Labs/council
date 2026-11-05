import { type AgentProtocol, type JsonObject, type ProtocolControl } from './acp.js';
/** One stdin turn using the documented Antigravity CLI stream-json protocol. */
export declare class AntigravityProtocol implements AgentProtocol {
    private readonly prompt;
    private stage;
    private started;
    private conversationId?;
    private output;
    private assistantSteps;
    private readonly steps;
    constructor(prompt: string);
    start(control: ProtocolControl): void;
    cancel(): void;
    diagnostic(text: string, control: ProtocolControl): void;
    private fail;
    event(event: JsonObject, control: ProtocolControl): void;
}
