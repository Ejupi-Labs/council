/** Reject unpatched SQLite runtimes before opening or creating Council state. */
export function requireSafeSqliteRuntime(): string {
  const version = process.versions.sqlite;
  const match = typeof version === 'string' ? /^(\d+)\.(\d+)\.(\d+)(?:\.\d+)?$/.exec(version) : null;
  const major = Number(match?.[1]);
  const minor = Number(match?.[2]);
  const patch = Number(match?.[3]);
  // https://sqlite.org/wal.html#walresetbug documents the multi-writer WAL reset fix.
  if (!match || !(major > 3 || (major === 3 && (minor > 51 || (minor === 51 && patch >= 3))))) {
    throw new Error(`Council requires SQLite 3.51.3 or newer for safe multi-process WAL writes; this runtime reports ${version ?? 'unknown'}. Upgrade Node.js to 24.18.1 or newer with a supported SQLite build before opening Council state.`);
  }
  return version!;
}

