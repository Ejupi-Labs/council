import { EventEmitter } from 'node:events';
import type { Readable, Writable } from 'node:stream';
export interface ProcessLaunch {
    command: string;
    args: string[];
}
export interface ManagedProcessOptions extends ProcessLaunch {
    cwd: string;
    environment?: NodeJS.ProcessEnv;
}
export declare const PROCESS_KILL_TIMEOUT_MS = 5000;
export declare const PROCESS_STOP_GRACE_MS: number;
export declare const PROCESS_CLEANUP_ERROR = "Owned agent process-tree cleanup could not be verified; a descendant may still be running.";
export declare class ProcessCleanupError extends Error {
    constructor();
}
export declare function killProcessTree(pid: number): Promise<void>;
/** The separate IPC supervisor survives parent death long enough to stop billing work. */
export declare class ManagedProcess extends EventEmitter {
    readonly stdin: Writable;
    readonly stdout: Readable;
    readonly stderr: Readable;
    readonly pid: number | undefined;
    private readonly supervisor;
    private vendorPid?;
    private launchFailed;
    private outcome?;
    private stopping;
    private stoppingAt;
    private closed;
    private forcePromise?;
    constructor(options: ManagedProcessOptions);
    kill(_signal?: NodeJS.Signals): boolean;
    forceKill(): Promise<void>;
}
/** Resolve executables and recognized installation shims without evaluating shell text. */
export declare function resolveLaunch(command: string, args: string[], cwd: string, environment?: NodeJS.ProcessEnv, signal?: AbortSignal): Promise<ProcessLaunch>;
