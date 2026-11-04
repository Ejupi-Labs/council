/** Retry atomic rename/owned staging cleanup, never data writes; await issued I/O without destructive fallback. */
export declare function retryWindowsSharing<T>(operation: () => Promise<T>, signal?: AbortSignal): Promise<T>;
