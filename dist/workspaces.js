import { createHash, randomUUID } from 'node:crypto';
import { link, lstat, mkdir, open, readFile, realpath, rename, unlink } from 'node:fs/promises';
import { dirname, isAbsolute, join, relative, resolve, sep } from 'node:path';
import { ManagedProcess } from './process.js';
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
    async operation(action, parent) {
        parent?.throwIfAborted();
        const controller = new AbortController();
        const abort = () => { controller.abort(parent?.reason); };
        parent?.addEventListener('abort', abort, { once: true });
        const timer = setTimeout(() => controller.abort(new Error('Git workspace operation exceeded its 60 second deadline.')), 60_000);
        try {
            return await action(controller.signal);
        }
        finally {
            clearTimeout(timer);
            parent?.removeEventListener('abort', abort);
        }
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
            const child = new ManagedProcess({ command: 'git', args: ['-c', 'core.quotePath=false', '-c', 'core.hooksPath=' + join(this.state, 'disabled-hooks'), '-C', cwd, ...args],
                cwd, environment: { ...process.env, GIT_TERMINAL_PROMPT: '0' } });
            const chunks = [];
            let bytes = 0;
            let diagnostics = '';
            let failure;
            let forceTimer;
            let forceDone;
            let stopping = false;
            const stop = (message) => {
                failure ??= new Error(message);
                if (stopping)
                    return;
                stopping = true;
                child.kill();
                forceTimer = setTimeout(() => { forceDone = child.forceKill().catch(() => { }); }, 1500);
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
            child.on('error', error => { stop(error.message); });
            child.once('close', async (code) => {
                clearTimeout(timer);
                clearTimeout(forceTimer);
                signal?.removeEventListener('abort', abort);
                await forceDone;
                if (failure || code !== 0)
                    reject(failure ?? new Error(`Git workspace operation failed: ${diagnostics.trim() || `exit ${code}`}`));
                else
                    accept(Buffer.concat(chunks));
            });
            child.stdin.end(input);
        });
    }
    create(runId, agent, slot = agent, signal, onPrepared) {
        return this.operation(active => this.createWorktree(runId, agent, slot, active, onPrepared), signal);
    }
    async createWorktree(runId, agent, slot, signal, onPrepared) {
        let base = this.runBases.get(runId);
        if (!base) {
            try {
                const [repo, head] = (await this.git(this.root, ['rev-parse', '--show-toplevel', '--verify', 'HEAD^{commit}'], undefined, signal)).toString('utf8').trim().split(/\r?\n/);
                if (!repo || !head)
                    throw new Error('Git did not return a repository and base commit.');
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
        const manifest = { agent, path, patchPath, cwd: join(path, base.suffix), baseHead: base.head };
        // Persist the database intent before the first managed filesystem side effect.
        onPrepared?.({ agent, path, patchPath, baseCommit: base.head, changed: false, exportStatus: 'pending' });
        signal.throwIfAborted();
        await mkdir(dirname(path), { recursive: true });
        await this.atomicWrite(this.manifestPath(path), JSON.stringify(manifest), 'workspace-manifests', true);
        await this.git(this.root, ['worktree', 'add', '--detach', path, base.head], undefined, signal);
        const pointer = /^gitdir: (.+)\s*$/.exec((await readFile(join(path, '.git'), 'utf8')).trim());
        if (!pointer)
            throw new Error('Git did not create valid worktree metadata.');
        manifest.gitDir = await realpath(resolve(path, pointer[1].trim()));
        await this.atomicWrite(this.manifestPath(path), JSON.stringify(manifest), 'workspace-manifests');
        return { agent, path, patchPath, changed: false, cwd: manifest.cwd, baseHead: base.head, baseCommit: base.head };
    }
    async manifest(artifact) {
        await this.checked(artifact.path, 'worktrees');
        const manifestFile = await this.checked(this.manifestPath(artifact.path), 'workspace-manifests');
        const manifest = JSON.parse(await readFile(manifestFile, 'utf8'));
        const expectedPatch = join(this.state, 'patches', ...relative(join(this.state, 'worktrees'), artifact.path).split(sep)) + '.patch';
        if (manifest.path !== resolve(artifact.path) || manifest.patchPath !== expectedPatch || artifact.patchPath !== expectedPatch || !/^[a-f0-9]{40,64}$/.test(manifest.baseHead))
            throw new Error('Workspace artifact does not match its managed manifest.');
        if (artifact.baseCommit && artifact.baseCommit !== manifest.baseHead)
            throw new Error('Workspace base commit does not match its manifest.');
        await this.checked(expectedPatch, 'patches');
        return manifest;
    }
    async registrations(signal) {
        return new Set((await this.git(this.root, ['worktree', 'list', '--porcelain', '-z'], undefined, signal)).toString('utf8').split('\0').filter(item => item.startsWith('worktree ')).map(item => resolve(item.slice(9))));
    }
    async verify(artifact, signal, loaded, registered) {
        const manifest = loaded ?? await this.manifest(artifact);
        if (manifest.removed)
            throw new Error('This managed worktree has already been removed.');
        const pointer = join(manifest.path, '.git');
        const metadata = await lstat(pointer);
        if (metadata.isSymbolicLink() || !metadata.isFile())
            throw new Error('Managed worktree Git metadata must be a regular file.');
        const match = /^gitdir: (.+)\s*$/.exec((await readFile(pointer, 'utf8')).trim());
        if (!match)
            throw new Error('Managed worktree Git metadata has been redirected.');
        const actualGitDir = await realpath(resolve(manifest.path, match[1].trim()));
        if (!manifest.gitDir) {
            const common = (await this.git(this.root, ['rev-parse', '--path-format=absolute', '--git-common-dir'], undefined, signal)).toString('utf8').trim();
            if (!inside(join(common, 'worktrees'), actualGitDir))
                throw new Error('Incomplete worktree metadata does not belong to this managed repository.');
            const reverse = (await readFile(join(actualGitDir, 'gitdir'), 'utf8')).trim();
            if (resolve(reverse) !== resolve(pointer))
                throw new Error('Incomplete worktree metadata does not belong to this managed repository.');
            manifest.gitDir = actualGitDir;
            await this.atomicWrite(this.manifestPath(manifest.path), JSON.stringify(manifest), 'workspace-manifests');
        }
        if (actualGitDir !== await realpath(manifest.gitDir))
            throw new Error('Managed worktree Git metadata has been redirected.');
        if (!(registered ?? await this.registrations(signal)).has(resolve(manifest.path)))
            throw new Error('Worktree is no longer registered with the original repository.');
        return manifest;
    }
    export(artifact, signal) {
        return this.operation(active => this.exportWorktree(artifact, active), signal);
    }
    async exportWorktree(artifact, signal) {
        const manifest = await this.verify(artifact, signal);
        const untracked = await this.git(manifest.path, ['ls-files', '--others', '--exclude-standard', '-z'], undefined, signal);
        if (untracked.length > 0)
            await this.git(manifest.path, ['add', '--intent-to-add', '--pathspec-from-file=-', '--pathspec-file-nul'], untracked, signal);
        const patch = await this.git(manifest.path, ['diff', '--binary', '--no-ext-diff', '--no-textconv', manifest.baseHead, '--'], undefined, signal);
        signal.throwIfAborted();
        await this.atomicWrite(manifest.patchPath, patch, 'patches');
        return { agent: artifact.agent, path: manifest.path, patchPath: manifest.patchPath, changed: patch.length > 0, baseCommit: manifest.baseHead, exportStatus: 'ready' };
    }
    forgetRun(runId) { this.runBases.delete(runId); }
    cleanup(artifact, options) {
        return this.operation(signal => this.cleanupWorktree(artifact, options, signal), options?.signal);
    }
    async cleanupWorktree(artifact, options, signal) {
        if (options?.reviewed !== true)
            throw new Error('Explicit review is required before worktree cleanup.');
        const manifest = await this.manifest(artifact);
        const registered = await this.registrations(signal);
        const present = await lstat(manifest.path).catch(error => { if (error.code === 'ENOENT')
            return undefined; throw error; });
        // Reconcile a crash after Git removal but before its manifest/event was committed.
        if (!present && !registered.has(resolve(manifest.path))) {
            if (!manifest.removed) {
                manifest.removed = true;
                await this.atomicWrite(this.manifestPath(manifest.path), JSON.stringify(manifest), 'workspace-manifests');
            }
            return;
        }
        if (!present)
            throw new Error('The managed worktree directory is missing but its Git registration remains. Inspect it before cleanup.');
        const patch = await lstat(manifest.patchPath).catch(() => undefined);
        if (!patch?.isFile() || patch.isSymbolicLink())
            throw new Error('Export and review a regular patch file before cleanup.');
        await this.verify(artifact, signal, manifest, registered);
        const current = await this.git(manifest.path, ['diff', '--binary', '--no-ext-diff', '--no-textconv', manifest.baseHead, '--'], undefined, signal);
        const saved = await readFile(manifest.patchPath);
        if (options.discardChanges !== true && !current.equals(saved))
            throw new Error('The worktree changed since patch export. Export and review it again before cleanup.');
        signal.throwIfAborted();
        await this.git(this.root, ['worktree', 'remove', ...(options.discardChanges === true ? ['--force'] : []), manifest.path], undefined, signal);
        manifest.removed = true;
        await this.atomicWrite(this.manifestPath(manifest.path), JSON.stringify(manifest), 'workspace-manifests');
    }
}
//# sourceMappingURL=workspaces.js.map