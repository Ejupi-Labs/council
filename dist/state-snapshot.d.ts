export declare const STATE_DATABASES: readonly ['state.db', 'rooms.sqlite', 'billing.sqlite', 'private-chats.sqlite', 'delegation.sqlite'];
type StateDatabaseName = typeof STATE_DATABASES[number];
export interface StateFileHealth {
    name: StateDatabaseName;
    present: boolean;
    databaseBytes: number;
    walBytes: number;
    shmBytes: number;
    schemaVersion?: number;
    integrity?: string[];
    error?: string;
}
export interface StateSnapshotEntry {
    name: StateDatabaseName;
    bytes: number;
    sha256: string;
    schemaVersion: number;
    startedAt: string;
    completedAt: string;
}
export interface StateSnapshotManifest {
    format: 'council-state-snapshot';
    formatVersion: 1;
    councilVersion: string;
    startedAt: string;
    completedAt: string;
    consistency: 'per-database';
    artifactsIncluded: false;
    databases: StateSnapshotEntry[];
    absentDatabases: StateDatabaseName[];
}
/** Inspect known collaboration stores without creating optional feature databases. */
export declare function inspectStateFiles(stateDir: string, checkIntegrity?: boolean): StateFileHealth[];
/** Back up all present stores; a durable manifest is published only after every copy verifies. */
export declare function createStateSnapshot(stateDir: string, output: string, signal?: AbortSignal): Promise<{
    destination: string;
    manifest: StateSnapshotManifest;
}>;
/** Check a completed snapshot before a manual, stopped-host restore; never opens live state. */
export declare function verifyStateSnapshot(directory: string, signal?: AbortSignal): Promise<{
    valid: boolean;
    directory: string;
    manifest: StateSnapshotManifest;
    issues: string[];
}>;
export {};
