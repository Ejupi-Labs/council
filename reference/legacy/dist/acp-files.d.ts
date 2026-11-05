export declare const MAX_ACP_FILE_BYTES: number;
export declare const MAX_ACP_FILE_RESPONSE_BYTES: number;
type ReadParams = {
    path: string;
    line?: number;
    limit?: number;
};
type WriteParams = {
    path: string;
    content: string;
};
/** Scoped ACP file callbacks. This is a project API boundary, not an OS sandbox. */
export declare class AcpFiles {
    private readonly permission;
    private readonly configured;
    private readonly root;
    constructor(cwd: string, permission: 'read' | 'write');
    private checked;
    contains(path: string): Promise<boolean>;
    read(params: ReadParams, signal?: AbortSignal): Promise<{
        content: string;
    }>;
    write(params: WriteParams, signal?: AbortSignal): Promise<null>;
}
export {};
