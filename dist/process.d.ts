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
export declare const PROCESS_KILL_TIMEOUT_MS = 1000;
export declare function killProcessTree(pid: number): Promise<void>;
/** The separate IPC supervisor survives parent death long enough to stop billing work. */
export declare class ManagedProcess extends EventEmitter {
    readonly stdin: Writable;
    readonly stdout: Readable;
    readonly stderr: Readable;
    readonly pid: number | undefined;
    private readonly supervisor;
    private vendorPid?;
    private outcome?;
    private stopping;
    private closed;
    private forcePromise?;
    constructor(options: ManagedProcessOptions);
    kill(_signal?: NodeJS.Signals): boolean;
    forceKill(): Promise<void>;
}
/** Resolve executable paths and recognized npm bins without ever evaluating shell text. */
export declare function resolveLaunch(command: string, args: string[], cwd: string, environment?: NodeJS.ProcessEnv, signal?: AbortSignal): Promise<ProcessLaunch>;
