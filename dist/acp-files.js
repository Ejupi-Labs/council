import { isUtf8 } from 'node:buffer';
import { randomUUID } from 'node:crypto';
import { constants } from 'node:fs';
import { lstat, mkdir, open, realpath, rename, unlink } from 'node:fs/promises';
import { dirname, isAbsolute, join, relative, resolve, sep } from 'node:path';
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
        this.root = realpath(this.configured).then(async (path) => {
            if (!(await lstat(path)).isDirectory())
                throw new Error('The project root must be a directory.');
            return path;
        });
        // A failed root remains observable to every operation without an unhandled rejection.
        void this.root.catch(() => { });
    }
    async checked(path) {
        if (typeof path !== 'string' || !path || path.length > 4096 || path.includes('\0') || !isAbsolute(path))
            throw new Error('ACP file paths must be absolute, nonempty paths of at most 4096 characters.');
        const root = await this.root;
        const input = resolve(path);
        const target = within(root, input) ? input : within(this.configured, input) ? resolve(root, relative(this.configured, input)) : undefined;
        if (!target || !within(root, target))
            throw new Error('File path is outside the project root.');
        const parts = relative(root, target).split(sep).filter(Boolean);
        if (parts.some(metadata))
            throw new Error('Git control paths and Council state are not available through ACP file callbacks.');
        const rootInfo = await lstat(root);
        if (!rootInfo.isDirectory() || rootInfo.isSymbolicLink() || await realpath(root) !== root)
            throw new Error('The canonical project root has been redirected.');
        let current = root;
        for (const [index, part] of parts.entries()) {
            current = join(current, part);
            try {
                const info = await lstat(current);
                if (info.isSymbolicLink())
                    throw new Error('Symlinks and junctions are not supported by ACP file callbacks.');
                if (index < parts.length - 1 && !info.isDirectory())
                    throw new Error('A file path ancestor is not a directory.');
                if (!within(root, await realpath(current)))
                    throw new Error('File path resolves outside the project root.');
            }
            catch (error) {
                if (error.code === 'ENOENT')
                    break;
                throw error;
            }
        }
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
        const path = await this.checked(params.path);
        const before = await lstat(path, { bigint: true });
        if (!before.isFile() || before.isSymbolicLink())
            throw new Error('Only regular UTF-8 files can be read.');
        if (before.size > BigInt(MAX_ACP_FILE_BYTES))
            throw new Error('File exceeds the 1 MiB read limit.');
        const file = await open(path, constants.O_RDONLY | (constants.O_NOFOLLOW ?? 0));
        let bytes;
        try {
            const opened = await file.stat({ bigint: true });
            await this.checked(params.path);
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
        const path = await this.checked(params.path);
        const parent = dirname(path);
        const root = await this.root;
        let current = root;
        for (const part of relative(root, parent).split(sep).filter(Boolean)) {
            signal?.throwIfAborted();
            current = join(current, part);
            await this.checked(current);
            await mkdir(current).catch(error => { if (error.code !== 'EEXIST')
                throw error; });
            await this.checked(current);
        }
        await this.checked(params.path);
        let mode = 0o600;
        try {
            const previous = await lstat(path);
            if (!previous.isFile() || previous.isSymbolicLink())
                throw new Error('Only regular text files can be replaced.');
            mode = previous.mode & 0o777;
        }
        catch (error) {
            if (error.code !== 'ENOENT')
                throw error;
        }
        signal?.throwIfAborted();
        const temporary = join(parent, `.council-write-${randomUUID()}.tmp`);
        let file;
        try {
            file = await open(temporary, constants.O_WRONLY | constants.O_CREAT | constants.O_EXCL | (constants.O_NOFOLLOW ?? 0), mode);
            await this.checked(temporary);
            await file.writeFile(data);
            await file.chmod(mode);
            await file.sync();
            await file.close();
            file = undefined;
            signal?.throwIfAborted();
            await this.checked(params.path);
            await this.checked(temporary);
            if (await realpath(parent) !== parent)
                throw new Error('File parent changed before atomic replacement.');
            signal?.throwIfAborted();
            await rename(temporary, path);
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