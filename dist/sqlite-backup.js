import { chmodSync, closeSync, fsyncSync, linkSync, lstatSync, mkdtempSync, openSync, realpathSync, rmSync } from 'node:fs';
import { basename, dirname, join, resolve } from 'node:path';
import { backup, DatabaseSync } from 'node:sqlite';
import { requireSafeSqliteRuntime } from './sqlite-runtime.js';
/** Copy a pinned read snapshot and publish a new file exclusively. */
export async function backupDatabase(sourcePath, path, signal) {
    signal?.throwIfAborted();
    requireSafeSqliteRuntime();
    if (typeof path !== 'string' || !path.trim() || Buffer.byteLength(path, 'utf8') > 4096 || path.includes('\0')) {
        throw new Error('Backup destination must be non-empty text of at most 4096 UTF-8 bytes');
    }
    const requested = resolve(path);
    const parent = realpathSync(dirname(requested));
    const destination = join(parent, basename(requested));
    const source = realpathSync(sourcePath);
    const key = (value) => process.platform === 'win32' ? value.toLowerCase() : value;
    if ([source, `${source}-wal`, `${source}-shm`, `${source}-journal`].map(key).includes(key(destination)))
        throw new Error('A backup cannot replace the active Council database or its journal files.');
    try {
        lstatSync(destination);
        throw new Error('Backup destination already exists; choose a new filename.');
    }
    catch (error) {
        if (error.code !== 'ENOENT')
            throw error;
    }
    const temporary = mkdtempSync(join(parent, '.council-backup-'));
    const snapshot = join(temporary, 'snapshot.db');
    let reader;
    let failed = false;
    try {
        const deadline = performance.now() + 60_000;
        // A separate pinned WAL reader prevents foreign writes from restarting every batch.
        reader = new DatabaseSync(source, { readOnly: true });
        reader.exec('PRAGMA busy_timeout = 250; BEGIN;');
        reader.prepare('SELECT name FROM sqlite_schema LIMIT 1').get();
        await backup(reader, snapshot, { rate: 128, progress: () => {
                signal?.throwIfAborted();
                if (performance.now() > deadline)
                    throw new Error('Council backup exceeded its 60-second deadline.');
            } });
        signal?.throwIfAborted();
        if (performance.now() > deadline)
            throw new Error('Council backup exceeded its 60-second deadline.');
        if (process.platform !== 'win32')
            chmodSync(snapshot, 0o600);
        const descriptor = openSync(snapshot, 'r+');
        try {
            fsyncSync(descriptor);
        }
        finally {
            closeSync(descriptor);
        }
        // Same-directory hard links are exclusive, including a racing destination creator.
        linkSync(snapshot, destination);
        return destination;
    }
    catch (error) {
        failed = true;
        throw error;
    }
    finally {
        let cleanupError;
        try {
            if (reader?.isTransaction)
                reader.exec('ROLLBACK');
        }
        catch (error) {
            cleanupError = error;
        }
        try {
            reader?.close();
        }
        catch (error) {
            cleanupError ??= error;
        }
        try {
            if (dirname(temporary) !== parent || !basename(temporary).startsWith('.council-backup-'))
                throw new Error('Refusing to remove an unexpected backup staging directory.');
            rmSync(temporary, { recursive: true, force: true });
        }
        catch (error) {
            cleanupError ??= error;
        }
        if (!failed && cleanupError)
            throw cleanupError;
    }
}
//# sourceMappingURL=sqlite-backup.js.map