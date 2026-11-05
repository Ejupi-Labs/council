function safeSqliteVersion(version: string | undefined): boolean {
  const match = typeof version === 'string' ? /^(\d+)\.(\d+)\.(\d+)(?:\.\d+)?$/.exec(version) : null;
  const major = Number(match?.[1]);
  const minor = Number(match?.[2]);
  const patch = Number(match?.[3]);
  return Boolean(match && (major > 3 || (major === 3 && (minor > 51 || (minor === 51 && patch >= 3)))));
}

/** Inspect the actual process runtime without opening any collaboration database. */
export function runtimeReadiness() {
  const node = /^(\d+)\.(\d+)\.(\d+)$/.exec(process.versions.node);
  const major = Number(node?.[1]), minor = Number(node?.[2]), patch = Number(node?.[3]);
  const nodeSupported = Boolean(node && (major > 24 || (major === 24 && (minor > 18 || (minor === 18 && patch >= 1)))));
  const sqliteSupported = safeSqliteVersion(process.versions.sqlite);
  return { supported: nodeSupported && sqliteSupported,
    node: { version: process.versions.node, executable: process.execPath, minimum: '24.18.1', supported: nodeSupported },
    sqlite: { version: process.versions.sqlite ?? null, minimum: '3.51.3', supported: sqliteSupported } };
}

/** Reject unpatched SQLite runtimes before opening or creating Council state. */
export function requireSafeSqliteRuntime(): string {
  const version = process.versions.sqlite;
  // https://sqlite.org/wal.html#walresetbug documents the multi-writer WAL reset fix.
  if (!safeSqliteVersion(version)) {
    throw new Error(`Council requires SQLite 3.51.3 or newer for safe multi-process WAL writes; this runtime reports ${version ?? 'unknown'}. Upgrade Node.js to 24.18.1 or newer with a supported SQLite build before opening Council state.`);
  }
  return version!;
}

