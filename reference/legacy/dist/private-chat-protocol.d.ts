export interface PrivateChatRequest {
    peer: string;
    message: string;
    context: string;
}
/** Only an entire exact envelope yields control; examples and surrounding prose remain ordinary output. */
export declare function privateChatRequest(output: string): PrivateChatRequest | undefined;
/** Detect a candidate even when its grammar is invalid, so private request bodies are not saved as public failed output. */
export declare function privateChatCandidate(output: string): boolean;
