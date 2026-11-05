/** Inspect the actual process runtime without opening any collaboration database. */
export declare function runtimeReadiness(): {
    supported: boolean;
    node: {
        version: string;
        executable: string;
        minimum: string;
        supported: boolean;
    };
    sqlite: {
        version: string | null;
        minimum: string;
        supported: boolean;
    };
};
/** Reject unpatched SQLite runtimes before opening or creating Council state. */
export declare function requireSafeSqliteRuntime(): string;
