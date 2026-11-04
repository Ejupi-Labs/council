/** Porcelain-v2 byte parsing. Pathnames are opaque bytes, not UTF-8 text. */
export declare const GIT_STATUS_COMPONENT_BYTES: number;
export declare const GIT_STATUS_OUTPUT_BYTES: number;
export declare const DIRTY_SUBMODULE_ERROR = "Uncommitted submodule files cannot be preserved in a parent Git patch. Inspect the retained worktree, then commit or separately export and review nested changes before retrying patch repair or cleanup.";
export interface GitStatusDiscovery {
    untracked: Buffer;
    gitlinks: Buffer;
}
/** Two scans, one exact allocation. Candidate paths are already bounded by tracked metadata. */
export declare function gitStatusDiscovery(output: Buffer, limits?: {
    trackedBytes?: number;
    untrackedBytes?: number;
}): GitStatusDiscovery;
export declare function gitStatusUntrackedPaths(output: Buffer, limits?: {
    trackedBytes?: number;
    untrackedBytes?: number;
}): Buffer;
