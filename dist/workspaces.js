import { spawn } from 'node:child_process';
import { createHash, randomUUID } from 'node:crypto';
import { link, lstat, mkdir, open, readFile, realpath, rename, unlink } from 'node:fs/promises';
import { dirname, isAbsolute, join, relative, resolve, sep } from 'node:path';
function segment(value) {
    const label = value.replace(/[^a-zA-Z0-9_-]/g, '-').slice(0, 40) || 'workspace';
    return `${label}-${createHash('sha256').update(value).digest('hex').slice(0, 12)}`;
}
function inside(base, path) {
    const suffix = relative(base, path);
    return Boolean(suffix) && suffix !== '..' && !suffix.startsWith(`..${sep}`) && !isAbsolute(suffix);
}
/** Retained detached worktrees with durable base metadata and explicit reviewed cleanup. */
export class GitWorkspaces {
    root;
    state;
    runBases = new Map();
    constructor(workspaceRoot, stateDir) {
        this.root = resolve(workspaceRoot);
        this.state = resolve(stateDir);
    }
    /** Check every existing managed ancestor; junctions and symlinks cannot redirect artifacts. */
    async checked(path, category) {
        const base = join(this.state, category);
        const target = resolve(path);
        if (!inside(base, target))
            throw new Error('Workspace artifact path is outside its managed directory.');
        const components = relative(this.state, target).split(sep);
        let current = this.state;
        for (const component of ['', ...components]) {
            if (component)
                current = join(current, component);
            try {
                if ((await lstat(current)).isSymbolicLink())
                    throw new Error('Managed workspace paths cannot contain symlinks or junctions.');
            }
            catch (error) {
                if (error.code !== 'ENOENT')
                    throw error;
            }
        }
        return target;
    }
    manifestPath(path) {
        const parts = relative(join(this.state, 'worktrees'), resolve(path)).split(sep);
        if (parts.length !== 2 || parts.some(part => !/^[a-zA-Z0-9_-]+$/.test(part)))
            throw new Error('Not a managed Council worktree path.');
        return join(this.state, 'workspace-manifests', parts[0], `${parts[1]}.json`);
    }
    async atomicWrite(path, data, category, exclusive = false) {
        await this.checked(path, category);
        await mkdir(dirname(path), { recursive: true });
        await this.checked(path, category);
        const temporary = `${path}.${randomUUID()}.tmp`;
        let handle;
        try {
            handle = await open(temporary, 'wx', 0o600);
            await handle.writeFile(data);
            await handle.sync();
            await handle.close();
            handle = undefined;
            await this.checked(path, category);
            if (exclusive)
                await link(temporary, path);
            else
                await rename(temporary, path);
        }
        finally {
            await handle?.close();
            await unlink(temporary).catch(error => { if (error.code !== 'ENOENT')
                throw error; });
        }
    }
    git(cwd, args, input, signal) {
        if (signal?.aborted)
            return Promise.reject(new Error('Git workspace operation cancelled.'));
        return new Promise((accept, reject) => {
            const child = spawn('git', ['-c', 'core.quotePath=false', '-c', 'core.hooksPath=' + join(this.state, 'disabled-hooks'), '-C', cwd, ...args], {
                shell: false, windowsHide: true, detached: process.platform !== 'win32',
                stdio: ['pipe', 'pipe', 'pipe'], env: { ...process.env, GIT_TERMINAL_PROMPT: '0' },
            });
            const chunks = [];
            let bytes = 0;
            let diagnostics = '';
            let failure;
            let killerDone;
            const stop = (message) => {
                if (failure)
                    return;
                failure = new Error(message);
                if (!child.pid)
                    return;
                if (process.platform === 'win32') {
                    const killer = spawn('taskkill', ['/pid', String(child.pid), '/t', '/f'], { shell: false, windowsHide: true, stdio: 'ignore', timeout: 5000 });
                    killerDone = new Promise(done => {
                        killer.once('error', () => { child.kill(); done(); });
                        killer.once('close', code => { if (code !== 0)
                            child.kill(); done(); });
                    });
                }
                else {
                    try {
                        process.kill(-child.pid, 'SIGKILL');
                    }
                    catch {
                        child.kill('SIGKILL');
                    }
                }
            };
            const abort = () => { stop('Git workspace operation cancelled.'); };
            const timer = setTimeout(() => stop('Git workspace operation timed out.'), 60_000);
            signal?.addEventListener('abort', abort, { once: true });
            child.stdout.on('data', (chunk) => {
                bytes += chunk.length;
                if (bytes > 64 * 1024 * 1024)
                    stop('Git workspace output exceeds 64 MiB.');
                else
                    chunks.push(chunk);
            });
            child.stderr.on('data', (chunk) => { diagnostics = (diagnostics + chunk.toString('utf8')).slice(0, 4096); });
            child.stdin.on('error', () => { });
            child.on('error', error => { failure ??= error; });
            child.once('close', async (code) => {
                clearTimeout(timer);
                signal?.removeEventListener('abort', abort);
                await killerDone;
                if (failure || code !== 0)
                    reject(failure ?? new Error(`Git workspace operation failed: ${diagnostics.trim() || `exit ${code}`}`));
                else
                    accept(Buffer.concat(chunks));
            });
            child.stdin.end(input);
        });
    }
    async create(runId, agent, slot = agent, signal) {
        let base = this.runBases.get(runId);
        if (!base) {
            try {
                const repo = (await this.git(this.root, ['rev-parse', '--show-toplevel'], undefined, signal)).toString('utf8').trim();
                const head = (await this.git(this.root, ['rev-parse', '--verify', 'HEAD^{commit}'], undefined, signal)).toString('utf8').trim();
                const suffix = relative(resolve(repo), this.root);
                if (suffix && !inside(resolve(repo), this.root))
                    throw new Error('Workspace is outside its Git repository.');
                base = { repo, head, suffix };
                this.runBases.set(runId, base);
            }
            catch (error) {
                if (signal?.aborted)
                    throw error;
                throw new Error('Write workflows require a Git repository with an existing HEAD commit. Commit the project before creating isolated worktrees.');
            }
        }
        const path = join(this.state, 'worktrees', segment(runId), segment(slot));
        const patchPath = join(this.state, 'patches', segment(runId), `${segment(slot)}.patch`);
        await this.checked(path, 'worktrees');
        await this.checked(patchPath, 'patches');
        await mkdir(dirname(path), { recursive: true });
        const manifest = { agent, path, patchPath, cwd: join(path, base.suffix), baseHead: base.head };
        await this.atomicWrite(this.manifestPath(path), JSON.stringify(manifest), 'workspace-manifests', true);
        await this.git(this.root, ['worktree', 'add', '--detach', path, base.head], undefined, signal);
        manifest.gitDir = (await this.git(path, ['rev-parse', '--absolute-git-dir'])).toString('utf8').trim();
        await this.atomicWrite(this.manifestPath(path), JSON.stringify(manifest), 'workspace-manifests');
        return { agent, path, patchPath, changed: false, cwd: manifest.cwd, baseHead: base.head, baseCommit: base.head };
    }
    async verify(artifact) {
        await this.checked(artifact.path, 'worktrees');
        const manifestFile = await this.checked(this.manifestPath(artifact.path), 'workspace-manifests');
        const manifest = JSON.parse(await readFile(manifestFile, 'utf8'));
        const expectedPatch = join(this.state, 'patches', ...relative(join(this.state, 'worktrees'), artifact.path).split(sep)) + '.patch';
        if (manifest.removed || manifest.path !== resolve(artifact.path) || manifest.patchPath !== expectedPatch || artifact.patchPath !== expectedPatch || !/^[a-f0-9]{40,64}$/.test(manifest.baseHead))
            throw new Error('Workspace artifact does not match its managed manifest.');
        if (artifact.baseCommit && artifact.baseCommit !== manifest.baseHead)
            throw new Error('Workspace base commit does not match its manifest.');
        await this.checked(expectedPatch, 'patches');
        const pointer = join(manifest.path, '.git');
        const metadata = await lstat(pointer);
        if (metadata.isSymbolicLink() || !metadata.isFile())
            throw new Error('Managed worktree Git metadata must be a regular file.');
        const match = /^gitdir: (.+)\s*$/.exec((await readFile(pointer, 'utf8')).trim());
        if (!match || !manifest.gitDir || await realpath(resolve(manifest.path, match[1].trim())) !== await realpath(manifest.gitDir))
            throw new Error('Managed worktree Git metadata has been redirected.');
        const registered = (await this.git(this.root, ['worktree', 'list', '--porcelain', '-z'])).toString('utf8').split('\0').filter(item => item.startsWith('worktree ')).map(item => resolve(item.slice(9)));
        if (!registered.includes(resolve(manifest.path)))
            throw new Error('Worktree is no longer registered with the original repository.');
        return manifest;
    }
    async export(artifact) {
        const manifest = await this.verify(artifact);
        const untracked = await this.git(manifest.path, ['ls-files', '--others', '--exclude-standard', '-z']);
        if (untracked.length > 0)
            await this.git(manifest.path, ['add', '--intent-to-add', '--pathspec-from-file=-', '--pathspec-file-nul'], untracked);
        const patch = await this.git(manifest.path, ['diff', '--binary', '--no-ext-diff', '--no-textconv', manifest.baseHead, '--']);
        await this.atomicWrite(manifest.patchPath, patch, 'patches');
        return { agent: artifact.agent, path: manifest.path, patchPath: manifest.patchPath, changed: patch.length > 0, baseCommit: manifest.baseHead };
    }
    forgetRun(runId) { this.runBases.delete(runId); }
    async cleanup(artifact, options) {
        if (options?.reviewed !== true)
            throw new Error('Explicit review is required before worktree cleanup.');
        const manifest = await this.verify(artifact);
        const patch = await lstat(manifest.patchPath).catch(() => undefined);
        if (!patch?.isFile() || patch.isSymbolicLink())
            throw new Error('Export and review a regular patch file before cleanup.');
        const current = await this.git(manifest.path, ['diff', '--binary', '--no-ext-diff', '--no-textconv', manifest.baseHead, '--']);
        const saved = await readFile(manifest.patchPath);
        if (options.discardChanges !== true && !current.equals(saved))
            throw new Error('The worktree changed since patch export. Export and review it again before cleanup.');
        await this.git(this.root, ['worktree', 'remove', ...(options.discardChanges === true ? ['--force'] : []), manifest.path]);
        manifest.removed = true;
        await this.atomicWrite(this.manifestPath(manifest.path), JSON.stringify(manifest), 'workspace-manifests');
    }
}
//# sourceMappingURL=workspaces.js.map