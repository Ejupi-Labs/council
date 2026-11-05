/** Keep potentially blocking platform stdout writes out of the MCP event loop. */
export declare class OutputPump {
    private readonly child;
    private readonly pending;
    private sequence;
    private closed;
    private closePromise?;
    constructor();
    private fail;
    write(data: string, signal?: AbortSignal): Promise<void>;
    close(): Promise<void>;
}
