import { fork, spawn } from 'node:child_process';
import { EventEmitter } from 'node:events';
import { access, open, readFile, stat } from 'node:fs/promises';
import { constants } from 'node:fs';
import { delimiter, dirname, extname, isAbsolute, join, resolve, sep } from 'node:path';
import { fileURLToPath } from 'node:url';
export async function killProcessTree(pid) {
    if (!Number.isSafeInteger(pid) || pid < 1 || pid === process.pid)
        return;
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
    await new Promise(resolveDone => {
        const killer = spawn(join(process.env.SystemRoot ?? 'C:\\Windows', 'System32', 'taskkill.exe'), ['/PID', String(pid), '/T', '/F'], { shell: false, windowsHide: true, stdio: 'ignore' });
        const fallback = () => { try {
            process.kill(pid, 'SIGKILL');
        }
        catch { /* Already gone. */ } resolveDone(); };
        killer.once('error', fallback);
        killer.once('close', code => { if (code !== 0)
            fallback();
        else
            resolveDone(); });
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
                this.outcome = { code: typeof value.code === 'number' ? value.code : null, signal: typeof value.signal === 'string' ? value.signal : null };
            if (value.type === 'launch_error')
                this.emit('error', new Error('Unable to start the configured agent executable.'));
        });
        this.supervisor.once('error', () => this.emit('error', new Error('Unable to start the agent supervisor.')));
        this.supervisor.once('close', () => {
            this.emit('close', this.outcome?.code ?? null, this.outcome?.signal ?? null);
        });
        this.supervisor.send({ type: 'start', command: options.command, args: options.args, cwd: options.cwd }, error => {
            if (error && !this.stopping)
                this.emit('error', new Error('Unable to initialize the agent supervisor.'));
        });
        this.stdin.on('error', () => { });
    }
    kill(_signal) {
        if (this.stopping)
            return true;
        this.stopping = true;
        if (this.supervisor.connected)
            this.supervisor.send({ type: 'stop' }, error => { if (error)
                void this.forceKill(); });
        else
            void this.forceKill();
        return true;
    }
    async forceKill() {
        if (this.vendorPid)
            await killProcessTree(this.vendorPid);
        if (this.pid)
            await killProcessTree(this.pid);
    }
}
const PACKAGE_BINS = {
    codex: ['@openai/codex'], claude: ['@anthropic-ai/claude-code'],
    dsh: ['@deepseek-ai/cli', '@deepseek-ai/dsh', 'deepseek-harness'],
    acpx: ['acpx'],
};
async function isExecutable(path) {
    try {
        return (await stat(path)).isFile() && (await access(path, /\.[cm]?js$/i.test(path) ? constants.R_OK : process.platform === 'win32' ? constants.F_OK : constants.X_OK), true);
    }
    catch {
        return false;
    }
}
async function unwrapNpmShim(path, name) {
    for (const packageName of PACKAGE_BINS[name] ?? []) {
        const root = join(dirname(path), 'node_modules', packageName);
        try {
            const manifest = JSON.parse(await readFile(join(root, 'package.json'), 'utf8'));
            if (!manifest || typeof manifest !== 'object')
                continue;
            const value = manifest;
            if (value.name !== packageName)
                continue;
            const bin = typeof value.bin === 'string' ? value.bin : value.bin && typeof value.bin === 'object' ? value.bin[name] : undefined;
            if (typeof bin !== 'string')
                continue;
            const target = resolve(root, bin);
            if (!target.startsWith(resolve(root) + sep) || !await isExecutable(target))
                continue;
            if (/\.[cm]?js$/i.test(target))
                return { command: process.execPath, args: [target] };
            if (/\.(exe|com)$/i.test(target))
                return { command: target, args: [] };
        }
        catch { /* This PATH entry is not a recognized npm installation. */ }
    }
    return undefined;
}
async function isWindowsNativeBinary(path) {
    let file;
    try {
        file = await open(path, 'r');
        const header = Buffer.alloc(2);
        return (await file.read(header, 0, 2, 0)).bytesRead === 2 && header[0] === 0x4d && header[1] === 0x5a;
    }
    catch {
        return false;
    }
    finally {
        await file?.close();
    }
}
/** Resolve executable paths and recognized npm bins without ever evaluating shell text. */
export async function resolveLaunch(command, args, cwd, environment = process.env) {
    if (!command || command.includes('\0') || args.some(arg => typeof arg !== 'string' || arg.includes('\0')))
        throw new Error('Agent command and arguments must be nonempty executable/argument strings.');
    const explicit = isAbsolute(command) || command.includes('/') || command.includes('\\');
    const directories = explicit ? [''] : (environment.PATH ?? environment.Path ?? '').split(delimiter).filter(Boolean);
    const names = process.platform === 'win32' && !extname(command) ? [command + '.exe', command + '.com', command, command + '.cmd', command + '.ps1'] : [command];
    for (const directory of directories)
        for (const name of names) {
            const candidate = explicit ? resolve(cwd, name) : resolve(cwd, directory.replace(/^"|"$/g, ''), name);
            if (!await isExecutable(candidate))
                continue;
            if (process.platform === 'win32' && !extname(candidate)) {
                // npm creates a POSIX shell script alongside .cmd/.ps1 on Windows. File
                // existence is not native executability; resolve recognized bins directly.
                const unwrapped = await unwrapNpmShim(candidate, command.split(/[\\/]/).pop());
                if (unwrapped)
                    return { command: unwrapped.command, args: [...unwrapped.args, ...args] };
                if (await isWindowsNativeBinary(candidate))
                    return { command: candidate, args: [...args] };
                if (explicit)
                    throw new Error('Windows shell entrypoints are not executed. Configure an executable or an explicit interpreter with an argument array.');
                continue;
            }
            if (/\.(cmd|bat|ps1)$/i.test(candidate)) {
                const unwrapped = await unwrapNpmShim(candidate, command.split(/[\\/]/).pop().replace(/\.(cmd|bat|ps1)$/i, ''));
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