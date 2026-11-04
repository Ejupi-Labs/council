import { fork, spawn } from 'node:child_process';
import { EventEmitter } from 'node:events';
import { access, open, readFile, stat } from 'node:fs/promises';
import { constants } from 'node:fs';
import { delimiter, dirname, extname, isAbsolute, join, resolve, sep } from 'node:path';
import { fileURLToPath } from 'node:url';
import { metadataLookup, MetadataLimitError } from './metadata.js';
export const PROCESS_KILL_TIMEOUT_MS = 5000;
export const PROCESS_STOP_GRACE_MS = PROCESS_KILL_TIMEOUT_MS + 1000;
export const PROCESS_CLEANUP_ERROR = 'Owned agent process-tree cleanup could not be verified; a descendant may still be running.';
export class ProcessCleanupError extends Error {
    constructor() { super(PROCESS_CLEANUP_ERROR); this.name = 'ProcessCleanupError'; }
}
const activeKills = new Map();
export function killProcessTree(pid) {
    if (!Number.isSafeInteger(pid) || pid < 1 || pid === process.pid)
        return Promise.resolve();
    const existing = activeKills.get(pid);
    if (existing)
        return existing;
    const work = terminateProcessTree(pid);
    activeKills.set(pid, work);
    void work.finally(() => { activeKills.delete(pid); }).catch(() => { });
    return work;
}
async function terminateProcessTree(pid) {
    if (process.platform !== 'win32') {
        try {
            process.kill(-pid, 'SIGKILL');
        }
        catch {
            try {
                process.kill(pid, 'SIGKILL');
            }
            catch { /* Already gone. */ }
        }
        return;
    }
    await new Promise((resolveDone, reject) => {
        let killer;
        try {
            killer = spawn(join(process.env.SystemRoot ?? 'C:\\Windows', 'System32', 'taskkill.exe'), ['/PID', String(pid), '/T', '/F'], { shell: false, windowsHide: true, stdio: 'ignore' });
        }
        catch {
            try {
                process.kill(pid, 'SIGKILL');
            }
            catch { /* Already gone or denied. */ }
            reject(new ProcessCleanupError());
            return;
        }
        let finished = false;
        const finish = (fallback) => {
            if (finished)
                return;
            finished = true;
            clearTimeout(timer);
            if (fallback) {
                try {
                    process.kill(pid, 'SIGKILL');
                }
                catch { /* Already gone or denied by the OS. */ }
                try {
                    killer.kill('SIGKILL');
                }
                catch { /* Helper already exited. */ }
            }
            if (fallback)
                reject(new ProcessCleanupError());
            else
                resolveDone();
        };
        const timer = setTimeout(() => finish(true), PROCESS_KILL_TIMEOUT_MS);
        killer.on('error', () => finish(true));
        killer.once('close', code => {
            if (code === 0) {
                finish(false);
                return;
            }
            try {
                process.kill(pid, 0);
                finish(true);
            }
            catch (error) {
                finish(error.code !== 'ESRCH');
            }
        });
    });
}
/** The separate IPC supervisor survives parent death long enough to stop billing work. */
export class ManagedProcess extends EventEmitter {
    stdin;
    stdout;
    stderr;
    pid;
    supervisor;
    vendorPid;
    outcome;
    stopping = false;
    stoppingAt = 0;
    closed = false;
    forcePromise;
    constructor(options) {
        super();
        this.supervisor = fork(fileURLToPath(new URL('./supervisor.js', import.meta.url)), [], {
            execPath: process.execPath, execArgv: [], cwd: options.cwd,
            env: options.environment ?? process.env, detached: process.platform !== 'win32', windowsHide: true,
            stdio: ['pipe', 'pipe', 'pipe', 'ipc'], serialization: 'json',
        });
        this.stdin = this.supervisor.stdin;
        this.stdout = this.supervisor.stdout;
        this.stderr = this.supervisor.stderr;
        this.pid = this.supervisor.pid;
        this.supervisor.on('message', (message) => {
            if (!message || typeof message !== 'object')
                return;
            const value = message;
            if (value.type === 'started' && typeof value.pid === 'number') {
                this.vendorPid = value.pid;
                this.emit('spawn');
            }
            if (value.type === 'closed')
                this.outcome = { code: typeof value.code === 'number' ? value.code : null, signal: typeof value.signal === 'string' ? value.signal : null, cleanupFailed: value.cleanupFailed === true };
            if (value.type === 'launch_error')
                this.emit('error', new Error('Unable to start the configured agent executable.'));
        });
        this.supervisor.once('error', () => this.emit('error', new Error('Unable to start the agent supervisor.')));
        this.supervisor.once('close', (code, signal) => {
            this.closed = true;
            if (this.outcome && code === 0 && signal === null) {
                if (this.outcome.cleanupFailed)
                    this.emit('error', new ProcessCleanupError());
                this.emit('close', this.outcome.cleanupFailed ? 1 : this.outcome.code, this.outcome.signal);
                return;
            }
            // A supervisor's own successful exit is not evidence of vendor completion.
            void this.forceKill().then(() => {
                this.emit('error', new Error('Agent supervisor exited without a verified vendor outcome.'));
                this.emit('close', code && code !== 0 ? code : 1, signal);
            }, () => {
                this.emit('error', new ProcessCleanupError());
                this.emit('close', 1, signal);
            });
        });
        this.supervisor.send({ type: 'start', command: options.command, args: options.args, cwd: options.cwd }, error => {
            if (error && !this.stopping)
                this.emit('error', new Error('Unable to initialize the agent supervisor.'));
        });
        this.stdin.on('error', () => { });
    }
    kill(_signal) {
        if (this.closed)
            return false;
        if (this.stopping)
            return true;
        this.stopping = true;
        this.stoppingAt = performance.now();
        const escalate = () => { void this.forceKill().catch(() => { this.emit('error', new ProcessCleanupError()); }); };
        try {
            if (this.supervisor.connected)
                this.supervisor.send({ type: 'stop' }, error => { if (error)
                    escalate(); });
            else
                escalate();
        }
        catch {
            escalate();
        }
        return true;
    }
    forceKill() {
        if (this.forcePromise)
            return this.forcePromise;
        this.forcePromise = (async () => {
            // Let an owned native tree walk finish before killing its supervisor or
            // creating overlapping taskkill helpers that can sever ancestor links.
            const remaining = process.platform === 'win32' && this.stopping ? PROCESS_STOP_GRACE_MS - (performance.now() - this.stoppingAt) : 0;
            if (!this.closed && remaining > 0)
                await new Promise(resolveDone => {
                    const done = () => { clearTimeout(timer); this.supervisor.removeListener('close', done); resolveDone(); };
                    const timer = setTimeout(done, remaining);
                    this.supervisor.once('close', done);
                });
            let failure;
            try {
                if (this.closed && this.outcome) {
                    if (this.outcome.cleanupFailed)
                        throw new ProcessCleanupError();
                    return;
                }
                if (this.vendorPid)
                    await killProcessTree(this.vendorPid).catch(error => { failure = error; });
                if (!this.closed && this.pid)
                    await killProcessTree(this.pid).catch(error => { failure = error; });
                if (failure)
                    throw failure;
            }
            finally {
                this.stdin.destroy();
                this.stdout.destroy();
                this.stderr.destroy();
            }
        })();
        return this.forcePromise;
    }
}
const PACKAGE_BINS = {
    codex: ['@openai/codex'], claude: ['@anthropic-ai/claude-code'],
    dsh: ['@deepseek-ai/cli', '@deepseek-ai/dsh', 'deepseek-harness'],
    acpx: ['acpx'],
};
async function isExecutable(path, signal) {
    signal?.throwIfAborted();
    try {
        return (await metadataLookup(() => stat(path), signal)).isFile() && (await metadataLookup(() => access(path, /\.[cm]?js$/i.test(path) ? constants.R_OK : process.platform === 'win32' ? constants.F_OK : constants.X_OK), signal), true);
    }
    catch (error) {
        if (signal?.aborted || error instanceof MetadataLimitError)
            throw error;
        return false;
    }
}
async function unwrapNpmShim(path, name, signal) {
    for (const packageName of PACKAGE_BINS[name] ?? []) {
        signal?.throwIfAborted();
        const root = join(dirname(path), 'node_modules', packageName);
        try {
            const filename = join(root, 'package.json');
            if ((await metadataLookup(() => stat(filename), signal)).size > 64 * 1024)
                continue;
            const manifest = JSON.parse(await metadataLookup(() => readFile(filename, { encoding: 'utf8', signal }), signal));
            if (!manifest || typeof manifest !== 'object')
                continue;
            const value = manifest;
            if (value.name !== packageName)
                continue;
            const bin = typeof value.bin === 'string' ? value.bin : value.bin && typeof value.bin === 'object' ? value.bin[name] : undefined;
            if (typeof bin !== 'string')
                continue;
            const target = resolve(root, bin);
            if (!target.startsWith(resolve(root) + sep) || !await isExecutable(target, signal))
                continue;
            if (/\.[cm]?js$/i.test(target))
                return { command: process.execPath, args: [target] };
            if (/\.(exe|com)$/i.test(target))
                return { command: target, args: [] };
        }
        catch (error) {
            if (signal?.aborted || error instanceof MetadataLimitError)
                throw error; /* Not a recognized npm installation. */
        }
    }
    return undefined;
}
async function isWindowsNativeBinary(path, signal) {
    let file;
    try {
        signal?.throwIfAborted();
        file = await open(path, 'r');
        signal?.throwIfAborted();
        const header = Buffer.alloc(2);
        const result = await file.read(header, 0, 2, 0);
        signal?.throwIfAborted();
        return result.bytesRead === 2 && header[0] === 0x4d && header[1] === 0x5a;
    }
    catch (error) {
        if (signal?.aborted)
            throw error;
        return false;
    }
    finally {
        await file?.close();
    }
}
/** Resolve executable paths and recognized npm bins without ever evaluating shell text. */
export async function resolveLaunch(command, args, cwd, environment = process.env, signal) {
    signal?.throwIfAborted();
    if (!command || command.includes('\0') || args.some(arg => typeof arg !== 'string' || arg.includes('\0')))
        throw new Error('Agent command and arguments must be nonempty executable/argument strings.');
    const explicit = isAbsolute(command) || command.includes('/') || command.includes('\\');
    const directories = explicit ? [''] : (environment.PATH ?? environment.Path ?? '').split(delimiter).filter(Boolean);
    const names = process.platform === 'win32' && !extname(command) ? [command + '.exe', command + '.com', command, command + '.cmd', command + '.ps1'] : [command];
    for (const directory of directories)
        for (const name of names) {
            signal?.throwIfAborted();
            const candidate = explicit ? resolve(cwd, name) : resolve(cwd, directory.replace(/^"|"$/g, ''), name);
            if (!await isExecutable(candidate, signal))
                continue;
            if (process.platform === 'win32' && !extname(candidate)) {
                // npm creates a POSIX shell script alongside .cmd/.ps1 on Windows. File
                // existence is not native executability; resolve recognized bins directly.
                const unwrapped = await unwrapNpmShim(candidate, command.split(/[\\/]/).pop(), signal);
                if (unwrapped)
                    return { command: unwrapped.command, args: [...unwrapped.args, ...args] };
                // Drain an opened header handle before returning, even when cancellation arrives.
                if (await metadataLookup(() => isWindowsNativeBinary(candidate, signal)))
                    return { command: candidate, args: [...args] };
                if (explicit)
                    throw new Error('Windows shell entrypoints are not executed. Configure an executable or an explicit interpreter with an argument array.');
                continue;
            }
            if (/\.(cmd|bat|ps1)$/i.test(candidate)) {
                const unwrapped = await unwrapNpmShim(candidate, command.split(/[\\/]/).pop().replace(/\.(cmd|bat|ps1)$/i, ''), signal);
                if (unwrapped)
                    return { command: unwrapped.command, args: [...unwrapped.args, ...args] };
                throw new Error('Windows shell wrappers are not executed. Configure an executable or an explicit interpreter with an argument array.');
            }
            if (/\.[cm]?js$/i.test(candidate))
                return { command: process.execPath, args: [candidate, ...args] };
            return { command: candidate, args: [...args] };
        }
    throw new Error('Agent executable was not found. Install its CLI or configure command and args; run council doctor.');
}
//# sourceMappingURL=process.js.map