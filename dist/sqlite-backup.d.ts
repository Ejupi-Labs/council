/** Copy a pinned read snapshot and publish a new file exclusively. */
export declare function backupDatabase(sourcePath: string, path: string, signal?: AbortSignal): Promise<string>;
