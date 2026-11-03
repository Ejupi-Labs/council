import type { WorkspaceArtifact } from './types.js';
export interface WorkspaceHandle extends WorkspaceArtifact {
    cwd: string;
    baseHead: string;
}
export interface CleanupOptions {
    reviewed: true;
    discardChanges?: boolean;
    signal?: AbortSignal;
}
/** Retained detached worktrees with durable base metadata and explicit reviewed cleanup. */
export declare class GitWorkspaces {
    private readonly root;
    private readonly state;
    private readonly runBases;
    constructor(workspaceRoot: string, stateDir: string);
    private operation;
    /** Check every existing managed ancestor; junctions and symlinks cannot redirect artifacts. */
    private checked;
    private manifestPath;
    private atomicWrite;
    private git;
    create(runId: string, agent: string, slot?: string, signal?: AbortSignal, onPrepared?: (artifact: WorkspaceArtifact) => void): Promise<WorkspaceHandle>;
    private createWorktree;
    private manifest;
    private registrations;
    private verify;
    export(artifact: WorkspaceArtifact, signal?: AbortSignal): Promise<WorkspaceArtifact>;
    private exportWorktree;
    forgetRun(runId: string): void;
    cleanup(artifact: WorkspaceArtifact, options: CleanupOptions): Promise<void>;
    private cleanupWorktree;
}
