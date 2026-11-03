import type { WorkspaceArtifact } from './types.js';
export interface WorkspaceHandle extends WorkspaceArtifact {
    cwd: string;
    baseHead: string;
}
export interface CleanupOptions {
    reviewed: true;
    discardChanges?: boolean;
}
/** Retained detached worktrees with durable base metadata and explicit reviewed cleanup. */
export declare class GitWorkspaces {
    private readonly root;
    private readonly state;
    private readonly runBases;
    constructor(workspaceRoot: string, stateDir: string);
    /** Check every existing managed ancestor; junctions and symlinks cannot redirect artifacts. */
    private checked;
    private manifestPath;
    private atomicWrite;
    private git;
    create(runId: string, agent: string, slot?: string, signal?: AbortSignal): Promise<WorkspaceHandle>;
    private verify;
    export(artifact: WorkspaceArtifact): Promise<WorkspaceArtifact>;
    forgetRun(runId: string): void;
    cleanup(artifact: WorkspaceArtifact, options: CleanupOptions): Promise<void>;
}
