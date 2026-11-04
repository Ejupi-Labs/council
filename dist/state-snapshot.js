import { createHash } from 'node:crypto';
import { closeSync, createReadStream, fsyncSync, lstatSync, mkdirSync, openSync, readFileSync, realpathSync, rmdirSync, statSync, unlinkSync, writeFileSync } from 'node:fs';
import { basename, dirname, join, resolve } from 'node:path';
import { DatabaseSync } from 'node:sqlite';
import { backupDatabase } from './sqlite-backup.js';
import { requireSafeSqliteRuntime } from './sqlite-runtime.js';
import { VERSION } from './version.js';
export const STATE_DATABASES = ['state.db', 'rooms.sqlite', 'billing.sqlite', 'private-chats.sqlite', 'delegation.sqlite'];
function missing(error) { return error?.code === 'ENOENT'; }
function regularFile(path) {
    if (!lstatSync(path).isFile())
        throw new Error('Expected a regular database file without a symbolic link.');
}
function inspectDatabase(path) {
    regularFile(path);
    const db = new DatabaseSync(path, { readOnly: true });
    try {
        db.exec('PRAGMA busy_timeout = 250;');
        const schemaVersion = Number(db.prepare('PRAGMA user_version').get().user_version);
        const integrity = db.prepare('PRAGMA quick_check').all().map(row => String(row.quick_check));
        const journalMode = String(db.prepare('PRAGMA journal_mode').get().journal_mode);
        return { schemaVersion, integrity, journalMode };
    }
    finally {
        db.close();
    }
}
/** Seal the isolated copy so no unmanifested WAL can alter its visible contents. */
function sealDatabase(path) {
    const db = new DatabaseSync(path);
    try {
        db.exec('PRAGMA busy_timeout = 250; PRAGMA wal_checkpoint(TRUNCATE);');
        const mode = db.prepare('PRAGMA journal_mode = DELETE').get();
        if (mode.journal_mode !== 'delete')
            throw new Error('Could not seal snapshot database in DELETE journal mode.');
    }
    finally {
        db.close();
    }
    const descriptor = openSync(path, 'r+');
    try {
        fsyncSync(descriptor);
    }
    finally {
        closeSync(descriptor);
    }
}
function snapshotSidecars(directory, name) {
    return ['-wal', '-shm', '-journal'].flatMap(suffix => {
        const filename = `${name}${suffix}`;
        try {
            lstatSync(join(directory, filename));
            return [filename];
        }
        catch (error) {
            if (missing(error))
                return [];
            throw error;
        }
    });
}
/** Inspect known collaboration stores without creating optional feature databases. */
export function inspectStateFiles(stateDir, checkIntegrity = false) {
    if (checkIntegrity)
        requireSafeSqliteRuntime();
    const size = (path) => {
        try {
            return lstatSync(path).size;
        }
        catch (error) {
            if (missing(error))
                return 0;
            throw error;
        }
    };
    return STATE_DATABASES.map(name => {
        const path = join(stateDir, name);
        const entry = { name, present: false, databaseBytes: 0, walBytes: 0, shmBytes: 0 };
        try {
            try {
                regularFile(path);
            }
            catch (error) {
                if (missing(error))
                    return entry;
                throw error;
            }
            entry.present = true;
            entry.databaseBytes = size(path);
            entry.walBytes = size(`${path}-wal`);
            entry.shmBytes = size(`${path}-shm`);
            if (checkIntegrity) {
                Object.assign(entry, inspectDatabase(path));
                if (entry.integrity?.length !== 1 || entry.integrity[0] !== 'ok')
                    entry.error = 'SQLite integrity check did not return ok.';
            }
        }
        catch (error) {
            entry.error = error instanceof Error ? error.message : 'Database inspection failed.';
        }
        return entry;
    });
}
async function checksum(path, signal) {
    signal?.throwIfAborted();
    const hash = createHash('sha256');
    for await (const chunk of createReadStream(path, { signal }))
        hash.update(chunk);
    signal?.throwIfAborted();
    return hash.digest('hex');
}
/** Back up all present stores; a durable manifest is published only after every copy verifies. */
export async function createStateSnapshot(stateDir, output, signal) {
    signal?.throwIfAborted();
    requireSafeSqliteRuntime();
    if (typeof output !== 'string' || !output.trim() || output.includes('\0') || Buffer.byteLength(output) > 4096)
        throw new Error('Snapshot destination must be non-empty text of at most 4096 UTF-8 bytes.');
    const requested = resolve(output), parent = realpathSync(dirname(requested));
    const destination = join(parent, basename(requested));
    const source = realpathSync(stateDir);
    const inventory = inspectStateFiles(source);
    const problem = inventory.find(entry => entry.error);
    if (problem)
        throw new Error(`Cannot snapshot ${problem.name}: ${problem.error}`);
    if (!inventory.some(entry => entry.name === 'state.db' && entry.present))
        throw new Error('The workflow state database is missing.');
    // This reservation is exclusive; a partial directory never has a completion manifest.
    mkdirSync(destination, { mode: 0o700 });
    const published = [];
    const manifest = {
        format: 'council-state-snapshot', formatVersion: 1, councilVersion: VERSION,
        startedAt: new Date().toISOString(), completedAt: '', consistency: 'per-database', artifactsIncluded: false,
        databases: [], absentDatabases: inventory.filter(entry => !entry.present).map(entry => entry.name),
    };
    try {
        for (const entry of inventory.filter(entry => entry.present)) {
            signal?.throwIfAborted();
            const startedAt = new Date().toISOString(), target = join(destination, entry.name);
            regularFile(join(source, entry.name));
            await backupDatabase(join(source, entry.name), target, signal);
            published.push(target);
            sealDatabase(target);
            if (snapshotSidecars(destination, entry.name).length)
                throw new Error(`Snapshot contains unexpected journal files for ${entry.name}.`);
            const details = inspectDatabase(target);
            if (details.integrity.length !== 1 || details.integrity[0] !== 'ok')
                throw new Error(`Snapshot integrity check failed for ${entry.name}.`);
            manifest.databases.push({ name: entry.name, bytes: statSync(target).size, sha256: await checksum(target, signal), schemaVersion: details.schemaVersion, startedAt, completedAt: new Date().toISOString() });
        }
        signal?.throwIfAborted();
        manifest.completedAt = new Date().toISOString();
        const completion = join(destination, 'manifest.json');
        writeFileSync(completion, JSON.stringify(manifest, null, 2) + '\n', { flag: 'wx', mode: 0o600 });
        published.push(completion);
        const descriptor = openSync(completion, 'r+');
        try {
            fsyncSync(descriptor);
        }
        finally {
            closeSync(descriptor);
        }
        signal?.throwIfAborted();
        return { destination, manifest };
    }
    catch (error) {
        // Remove only files this operation published. Preserve any unexpected concurrent additions.
        for (const file of published.reverse()) {
            try {
                unlinkSync(file);
            }
            catch { }
        }
        try {
            rmdirSync(destination);
        }
        catch { }
        throw error;
    }
}
/** Check a completed snapshot before a manual, stopped-host restore; never opens live state. */
export async function verifyStateSnapshot(directory, signal) {
    signal?.throwIfAborted();
    requireSafeSqliteRuntime();
    const root = realpathSync(directory), manifestPath = join(root, 'manifest.json');
    regularFile(manifestPath);
    if (statSync(manifestPath).size > 64 * 1024)
        throw new Error('Snapshot manifest exceeds 64 KiB.');
    const manifest = JSON.parse(readFileSync(manifestPath, 'utf8'));
    if (!manifest || manifest.format !== 'council-state-snapshot' || manifest.formatVersion !== 1 || manifest.consistency !== 'per-database' || manifest.artifactsIncluded !== false ||
        !Array.isArray(manifest.databases) || !Array.isArray(manifest.absentDatabases) || manifest.databases.length > STATE_DATABASES.length ||
        typeof manifest.startedAt !== 'string' || !Number.isFinite(Date.parse(manifest.startedAt)) || typeof manifest.completedAt !== 'string' || !Number.isFinite(Date.parse(manifest.completedAt)))
        throw new Error('Invalid or unsupported Council snapshot manifest.');
    const names = new Set();
    for (const entry of manifest.databases) {
        if (!entry || !STATE_DATABASES.includes(entry.name) || names.has(entry.name) || !Number.isSafeInteger(entry.bytes) || entry.bytes < 0 ||
            typeof entry.sha256 !== 'string' || !/^[a-f0-9]{64}$/.test(entry.sha256) || !Number.isSafeInteger(entry.schemaVersion) || entry.schemaVersion < 0)
            throw new Error('Invalid or duplicated database entry in snapshot manifest.');
        names.add(entry.name);
    }
    if (!names.has('state.db'))
        throw new Error('Snapshot does not contain workflow state.');
    for (const name of manifest.absentDatabases) {
        if (!STATE_DATABASES.includes(name) || names.has(name))
            throw new Error('Invalid or duplicated absent database in snapshot manifest.');
        names.add(name);
    }
    if (names.size !== STATE_DATABASES.length)
        throw new Error('Snapshot manifest must account for every collaboration database.');
    const issues = [];
    for (const name of manifest.absentDatabases) {
        try {
            lstatSync(join(root, name));
            issues.push(`${name}: database is present but declared absent.`);
        }
        catch (error) {
            if (!missing(error))
                issues.push(`${name}: cannot verify its declared absence.`);
        }
        for (const sidecar of snapshotSidecars(root, name))
            issues.push(`${name}: unmanifested sidecar ${sidecar}.`);
    }
    for (const entry of manifest.databases) {
        signal?.throwIfAborted();
        try {
            const path = join(root, entry.name);
            regularFile(path);
            const sidecars = snapshotSidecars(root, entry.name);
            if (sidecars.length) {
                issues.push(`${entry.name}: unmanifested sidecars ${sidecars.join(', ')}.`);
                continue;
            }
            if (statSync(path).size !== entry.bytes)
                issues.push(`${entry.name}: file size differs from the manifest.`);
            if (await checksum(path, signal) !== entry.sha256)
                issues.push(`${entry.name}: SHA-256 differs from the manifest.`);
            const details = inspectDatabase(path);
            if (details.journalMode !== 'delete')
                issues.push(`${entry.name}: snapshot database is not sealed in DELETE journal mode.`);
            if (snapshotSidecars(root, entry.name).length)
                issues.push(`${entry.name}: journal files appeared during verification.`);
            if (await checksum(path, signal) !== entry.sha256)
                issues.push(`${entry.name}: database changed during verification.`);
            if (details.schemaVersion !== entry.schemaVersion)
                issues.push(`${entry.name}: schema version differs from the manifest.`);
            if (details.integrity.length !== 1 || details.integrity[0] !== 'ok')
                issues.push(`${entry.name}: integrity check did not return ok.`);
        }
        catch (error) {
            signal?.throwIfAborted();
            issues.push(`${entry.name}: ${error instanceof Error ? error.message : 'Verification failed.'}`);
        }
    }
    return { valid: issues.length === 0, directory: root, manifest, issues };
}
//# sourceMappingURL=state-snapshot.js.map