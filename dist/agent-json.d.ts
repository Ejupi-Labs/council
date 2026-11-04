export declare const MAX_AGENT_JSON_DEPTH = 64;
export declare const MAX_AGENT_JSON_TOKENS = 65536;
/** Bound graph allocation before JSON.parse; quoted tool output remains one token. */
export declare function checkAgentJsonStructure(frame: Buffer): void;
