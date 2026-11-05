import { isUtf8 } from 'node:buffer';
import { randomUUID } from 'node:crypto';
import { constants } from 'node:fs';
import { lstat, mkdir, open, realpath, rename, unlink } from 'node:fs/promises';
import { dirname, isAbsolute, join, relative, resolve, sep } from 'node:path';
import { metadataLookup, metadataResult } from './metadata.js';
import { retryWindowsSharing } from './fs-retry.js';
export const MAX_ACP_FILE_BYTES = 1024 * 1024;
export const MAX_ACP_FILE_RESPONSE_BYTES = 2 * MAX_ACP_FILE_BYTES;
function within(root, path) {
    const suffix = relative(root, path);
    return !isAbsolute(suffix) && suffix !== '..' && !suffix.startsWith(`..${sep}`);
}
function metadata(part) {
    const normalized = part.toLowerCase().replace(/[ .]+$/g, '');
    return normalized === '.git' || normalized === '.council' || part.includes(':');
}
function positive(value, label) {
    if (value !== undefined && (!Number.isSafeInteger(value) || value < 1))
        throw new Error(`${label} must be a positive safe integer.`);
}
/** Scoped ACP file callbacks. This is a project API boundary, not an OS sandbox. */
export class AcpFiles {
    permission;
    configured;
    root;
    constructor(cwd, permission) {
        this.permission = permission;
        if (typeof cwd !== 'string' || !cwd || cwd.includes('\0'))
            throw new Error('A valid project directory is required.');
        if (permission !== 'read' && permission !== 'write')
            throw new Error('File permission must be read or write.');
        this.configured = resolve(cwd);
        this.root = metadataLookup(() => realpath(this.configured)).then(async (path) => {
            if (!(await metadataLookup(() => lstat(path))).isDirectory())
                throw new Error('The project root must be a directory.');
            return path;
        });
        // A failed root remains observable to every operation without an unhandled rejection.
        void this.root.catch(() => { });
    }
    async checked(path, signal) {
        signal?.throwIfAborted();
        if (typeof path !== 'string' || !path || path.length > 4096 || path.includes('\0') || !isAbsolute(path))
            throw new Error('ACP file paths must be absolute, nonempty paths of at most 4096 characters.');
        const root = await metadataResult(this.root, signal);
        const input = resolve(path);
        const target = within(root, input) ? input : within(this.configured, input) ? resolve(root, relative(this.configured, input)) : undefined;
        if (!target || !within(root, target))
            throw new Error('File path is outside the project root.');
        const parts = relative(root, target).split(sep).filter(Boolean);
        if (parts.some(metadata))
            throw new Error('Git control paths and Council state are not available through ACP file callbacks.');
        const rootInfo = await metadataLookup(() => lstat(root, { bigint: true }), signal);
        signal?.throwIfAborted();
        if (!rootInfo.isDirectory() || rootInfo.isSymbolicLink() || await metadataLookup(() => realpath(root), signal) !== root)
            throw new Error('The canonical project root has been redirected.');
        let claimedRoot = target;
        for (let index = 0; index < parts.length; index++)
            claimedRoot = dirname(claimedRoot);
        if (claimedRoot !== root) {
            // Windows path comparison folds case even when NTFS has case sensitivity
            // enabled. A differently spelled prefix must identify the selected directory.
            const prefix = claimedRoot;
            const claimedInfo = await metadataLookup(() => lstat(prefix, { bigint: true }), signal);
            if (!claimedInfo.isDirectory() || claimedInfo.isSymbolicLink() || rootInfo.ino === 0n || claimedInfo.ino === 0n
                || claimedInfo.dev !== rootInfo.dev || claimedInfo.ino !== rootInfo.ino)
                throw new Error('File path is outside the selected project root identity.');
        }
        let current = root;
        for (const [index, part] of parts.entries()) {
            current = join(current, part);
            try {
                signal?.throwIfAborted();
                const info = await metadataLookup(() => lstat(current), signal);
                if (info.isSymbolicLink())
                    throw new Error('Symlinks and junctions are not supported by ACP file callbacks.');
                if (index < parts.length - 1 && !info.isDirectory())
                    throw new Error('A file path ancestor is not a directory.');
                signal?.throwIfAborted();
                const checkedPath = current;
                const canonical = await metadataLookup(() => realpath(checkedPath), signal);
                if (!within(root, canonical))
                    throw new Error('File path resolves outside the project root.');
                // Windows 8.3 aliases can name .git/.council without containing their literal names.
                if (relative(root, canonical).split(sep).some(metadata))
                    throw new Error('Git control paths and Council state are not available through ACP file callbacks.');
            }
            catch (error) {
                if (signal?.aborted)
                    throw signal.reason;
                if (error.code === 'ENOENT')
                    break;
                throw error;
            }
        }
        signal?.throwIfAborted();
        return target;
    }
    async contains(path) {
        try {
            await this.checked(path);
            return true;
        }
        catch {
            return false;
        }
    }
    async read(params, signal) {
        signal?.throwIfAborted();
        if (!params || typeof params !== 'object' || Array.isArray(params))
            throw new Error('File read parameters must be an object.');
        positive(params.line, 'line');
        positive(params.limit, 'limit');
        const path = await this.checked(params.path, signal);
        signal?.throwIfAborted();
        const before = await metadataLookup(() => lstat(path, { bigint: true }), signal);
        if (!before.isFile() || before.isSymbolicLink())
            throw new Error('Only regular UTF-8 files can be read.');
        if (before.size > BigInt(MAX_ACP_FILE_BYTES))
            throw new Error('File exceeds the 1 MiB read limit.');
        signal?.throwIfAborted();
        const file = await open(path, constants.O_RDONLY | (constants.O_NOFOLLOW ?? 0));
        let bytes;
        try {
            signal?.throwIfAborted();
            const opened = await file.stat({ bigint: true });
            await this.checked(params.path, signal);
            if (!opened.isFile() || opened.dev !== before.dev || opened.ino !== before.ino)
                throw new Error('File changed while being opened.');
            const buffer = Buffer.allocUnsafe(MAX_ACP_FILE_BYTES + 1);
            let count = 0;
            while (count < buffer.length) {
                signal?.throwIfAborted();
                const part = await file.read(buffer, count, buffer.length - count, count);
                if (part.bytesRead === 0)
                    break;
                count += part.bytesRead;
            }
            if (count > MAX_ACP_FILE_BYTES)
                throw new Error('File exceeds the 1 MiB read limit.');
            bytes = buffer.subarray(0, count);
        }
        finally {
            await file.close();
        }
        signal?.throwIfAborted();
        if (!isUtf8(bytes))
            throw new Error('File content is not valid UTF-8.');
        let content = bytes.toString('utf8');
        if (params.line !== undefined || params.limit !== undefined) {
            const lines = content.match(/[^\r\n]*(?:\r\n|\n|\r|$)/g) ?? [];
            if (lines.at(-1) === '')
                lines.pop();
            const start = (params.line ?? 1) - 1;
            content = lines.slice(start, params.limit === undefined ? undefined : Math.min(lines.length, start + params.limit)).join('');
        }
        const result = { content };
        if (Buffer.byteLength(JSON.stringify(result), 'utf8') > MAX_ACP_FILE_RESPONSE_BYTES)
            throw new Error('File response exceeds 2 MiB after JSON encoding. Request fewer lines.');
        return result;
    }
    async write(params, signal) {
        signal?.throwIfAborted();
        if (this.permission !== 'write')
            throw new Error('File writes are not allowed by the read-only policy.');
        if (!params || typeof params !== 'object' || Array.isArray(params) || typeof params.content !== 'string')
            throw new Error('File write requires a path and text content.');
        if (params.content.length > MAX_ACP_FILE_BYTES)
            throw new Error('Content exceeds the 1 MiB write limit.');
        const data = Buffer.from(params.content, 'utf8');
        if (data.length > MAX_ACP_FILE_BYTES)
            throw new Error('Content exceeds the 1 MiB write limit.');
        if (data.toString('utf8') !== params.content)
            throw new Error('File content contains an invalid Unicode sequence.');
        const path = await this.checked(params.path, signal);
        const parent = dirname(path);
        const root = await metadataResult(this.root, signal);
        let current = root;
        for (const part of relative(root, parent).split(sep).filter(Boolean)) {
            signal?.throwIfAborted();
            current = join(current, part);
            await this.checked(current, signal);
            signal?.throwIfAborted();
            await mkdir(current).catch(error => { if (error.code !== 'EEXIST')
                throw error; });
            await this.checked(current, signal);
        }
        await this.checked(params.path, signal);
        let mode = 0o600;
        try {
            signal?.throwIfAborted();
            const previous = await metadataLookup(() => lstat(path), signal);
            if (!previous.isFile() || previous.isSymbolicLink())
                throw new Error('Only regular text files can be replaced.');
            mode = previous.mode & 0o777;
        }
        catch (error) {
            if (signal?.aborted)
                throw signal.reason;
            if (error.code !== 'ENOENT')
                throw error;
        }
        signal?.throwIfAborted();
        const temporary = join(parent, `.council-write-${randomUUID()}.tmp`);
        let file;
        try {
            file = await open(temporary, constants.O_WRONLY | constants.O_CREAT | constants.O_EXCL | (constants.O_NOFOLLOW ?? 0), mode);
            await this.checked(temporary, signal);
            signal?.throwIfAborted();
            await file.writeFile(data);
            signal?.throwIfAborted();
            await file.chmod(mode);
            signal?.throwIfAborted();
            await file.sync();
            await file.close();
            file = undefined;
            signal?.throwIfAborted();
            await this.checked(params.path, signal);
            await this.checked(temporary, signal);
            signal?.throwIfAborted();
            if (relative(await metadataLookup(() => realpath(parent), signal), parent) !== '')
                throw new Error('File parent changed before atomic replacement.');
            signal?.throwIfAborted();
            await retryWindowsSharing(() => rename(temporary, path), signal);
            return null;
        }
        finally {
            await file?.close();
            await unlink(temporary).catch(error => { if (error.code !== 'ENOENT')
                throw error; });
        }
    }
}
//# sourceMappingURL=acp-files.js.map