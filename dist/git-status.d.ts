/** Porcelain-v2 byte parsing. Pathnames are opaque bytes, not UTF-8 text. */
export declare const GIT_STATUS_COMPONENT_BYTES: number;
export declare const GIT_STATUS_OUTPUT_BYTES: number;
export declare const DIRTY_SUBMODULE_ERROR = "Uncommitted submodule files cannot be preserved in a parent Git patch. Inspect the retained worktree, then commit or separately export and review nested changes before retrying patch repair or cleanup.";
/** Validate/count first, then allocate and copy exactly once. Limits can be reduced for bounded callers/tests. */
export declare function gitStatusUntrackedPaths(output: Buffer, limits?: {
    trackedBytes?: number;
    untrackedBytes?: number;
}): Buffer;
