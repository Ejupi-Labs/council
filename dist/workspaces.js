import { createHash, randomUUID } from 'node:crypto';
import { realpathSync } from 'node:fs';
import { link, lstat, mkdir, open, readFile, realpath, rename, unlink } from 'node:fs/promises';
import { basename, dirname, isAbsolute, join, relative, resolve, sep } from 'node:path';
import { ManagedProcess, ProcessCleanupError } from './process.js';
import { sanitizedGitEnvironment } from './git-env.js';
import { retryWindowsSharing } from './fs-retry.js';
function segment(value) {
    const label = value.replace(/[^a-zA-Z0-9_-]/g, '-').slice(0, 40) || 'workspace';
    return `${label}-${createHash('sha256').update(value).digest('hex').slice(0, 12)}`;
}
function inside(base, path) {
    const suffix = relative(base, path);
    return Boolean(suffix) && suffix !== '..' && !suffix.startsWith(`..${sep}`) && !isAbsolute(suffix);
}
function canonicalStatePath(path) {
    let ancestor = resolve(path);
    const missing = [];
    for (;;) {
        try {
            return resolve(realpathSync.native(ancestor), ...missing.reverse());
        }
        catch (error) {
            if (error.code !== 'ENOENT')
                throw error;
            const parent = dirname(ancestor);
            if (parent === ancestor)
                throw error;
            missing.push(basename(ancestor));
            ancestor = parent;
        }
    }
}
/** Retained detached worktrees with durable base metadata and explicit reviewed cleanup. */
export class GitWorkspaces {
    root;
    state;
    runBases = new Map();
    constructor(workspaceRoot, stateDir) {
        this.root = resolve(workspaceRoot);
        this.state = canonicalStatePath(stateDir);
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
    async atomicWrite(path, data, category, exclusive = false, signal) {
        signal?.throwIfAborted();
        await this.checked(path, category);
        signal?.throwIfAborted();
        await mkdir(dirname(path), { recursive: true });
        await this.checked(path, category);
        signal?.throwIfAborted();
        const temporary = `${path}.${randomUUID()}.tmp`;
        let handle;
        try {
            handle = await open(temporary, 'wx', 0o600);
            signal?.throwIfAborted();
            await handle.writeFile(data);
            signal?.throwIfAborted();
            await handle.sync();
            await handle.close();
            handle = undefined;
            await this.checked(path, category);
            signal?.throwIfAborted();
            if (exclusive)
                await link(temporary, path);
            else
                await retryWindowsSharing(() => rename(temporary, path), signal);
            signal?.throwIfAborted();
        }
        finally {
            await handle?.close();
            // Cleanup must finish even when the caller's publication signal is already aborted.
            await retryWindowsSharing(() => unlink(temporary)).catch(error => { if (error.code !== 'ENOENT')
                throw error; });
        }
    }
    git(cwd, args, input, signal) {
        if (signal?.aborted)
            return Promise.reject(new Error('Git workspace operation cancelled.'));
        return new Promise((accept, reject) => {
            const child = new ManagedProcess({ command: 'git', args: ['-c', 'core.quotePath=false', '-c', 'core.hooksPath=' + join(this.state, 'disabled-hooks'), '-C', cwd, ...args],
                cwd, environment: { ...sanitizedGitEnvironment(), GIT_TERMINAL_PROMPT: '0' } });
            const chunks = [];
            let bytes = 0;
            let diagnostics = '';
            let failure;
            let forceTimer;
            let forceDone;
            let stopping = false;
            const stop = (reason) => {
                const error = typeof reason === 'string' ? new Error(reason) : reason;
                if (error instanceof ProcessCleanupError)
                    failure = error;
                else
                    failure ??= error;
                if (stopping)
                    return;
                stopping = true;
                child.kill();
                forceTimer = setTimeout(() => {
                    forceDone = child.forceKill().catch(error => { failure = error instanceof ProcessCleanupError ? error : new ProcessCleanupError(); });
                }, 1500);
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
            child.on('error', error => { stop(error); });
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
            let metadata;
            try {
                metadata = (await this.git(this.root, ['rev-parse', '--show-toplevel', '--verify', 'HEAD^{commit}'], undefined, signal)).toString('utf8');
            }
            catch (error) {
                if (signal?.aborted || error instanceof ProcessCleanupError)
                    throw error;
                throw new Error('Write workflows require a Git repository with an existing HEAD commit. Commit the project before creating isolated worktrees.');
            }
            const [reportedRepo, head] = metadata.trim().split(/\r?\n/);
            if (!reportedRepo || !/^[a-f0-9]{40,64}$/.test(head ?? ''))
                throw new Error('Git did not return a repository and base commit.');
            // Git and Node may spell the same directory differently (8.3 paths,
            // junctions or /var aliases). Compare native canonical paths, then the
            // actual repository ancestor so case-sensitive NTFS siblings stay apart.
            const [repo, root] = await Promise.all([realpath(reportedRepo), realpath(this.root)]);
            signal.throwIfAborted();
            const suffix = relative(repo, root);
            if (suffix && !inside(repo, root))
                throw new Error('Workspace is outside its Git repository.');
            const ancestor = resolve(root, ...suffix.split(sep).filter(Boolean).map(() => '..'));
            const [repositoryIdentity, ancestorIdentity] = await Promise.all([lstat(repo, { bigint: true }), lstat(ancestor, { bigint: true })]);
            signal.throwIfAborted();
            if (repositoryIdentity.dev !== ancestorIdentity.dev || repositoryIdentity.ino !== ancestorIdentity.ino)
                throw new Error('Workspace is outside its Git repository.');
            if (suffix) {
                let kind;
                try {
                    kind = (await this.git(this.root, ['cat-file', '-t', `${head}:${suffix.split(sep).join('/')}`], undefined, signal)).toString('utf8').trim();
                }
                catch (error) {
                    if (signal.aborted || error instanceof ProcessCleanupError)
                        throw error;
                    throw new Error('The selected project directory is not present in the captured HEAD commit. Commit that directory or select a committed project before creating isolated worktrees.');
                }
                if (kind !== 'tree')
                    throw new Error('The selected project directory is not a committed directory in the captured HEAD.');
            }
            // Another worker may have captured this run's base while Git metadata was
            // being read. Select the existing base without another asynchronous gap.
            base = this.runBases.get(runId) ?? { repo, head: head, suffix };
            this.runBases.set(runId, base);
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
        await this.atomicWrite(this.manifestPath(path), JSON.stringify(manifest), 'workspace-manifests', true, signal);
        await this.git(this.root, ['worktree', 'add', '--detach', path, base.head], undefined, signal);
        const pointer = /^gitdir: (.+)\s*$/.exec((await readFile(join(path, '.git'), 'utf8')).trim());
        if (!pointer)
            throw new Error('Git did not create valid worktree metadata.');
        manifest.gitDir = await realpath(resolve(path, pointer[1].trim()));
        await this.atomicWrite(this.manifestPath(path), JSON.stringify(manifest), 'workspace-manifests', false, signal);
        await this.checked(manifest.cwd, 'worktrees');
        const directory = await lstat(manifest.cwd).catch(error => { if (error.code === 'ENOENT')
            return undefined; throw error; });
        if (!directory?.isDirectory())
            throw new Error('The captured project directory is unavailable in the isolated worktree. Inspect sparse-checkout settings and the retained preparation intent before retrying.');
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
            if (resolve(actualGitDir, reverse) !== resolve(pointer))
                throw new Error('Incomplete worktree metadata does not belong to this managed repository.');
            manifest.gitDir = actualGitDir;
            await this.atomicWrite(this.manifestPath(manifest.path), JSON.stringify(manifest), 'workspace-manifests', false, signal);
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
        await this.assertExportableSubmodules(manifest, signal);
        const untracked = await this.git(manifest.path, ['ls-files', '--others', '--exclude-standard', '-z'], undefined, signal);
        if (untracked.length > 0)
            await this.git(manifest.path, ['add', '--intent-to-add', '--pathspec-from-file=-', '--pathspec-file-nul'], untracked, signal);
        const patch = await this.patch(manifest, signal);
        signal.throwIfAborted();
        await this.atomicWrite(manifest.patchPath, patch, 'patches', false, signal);
        signal.throwIfAborted();
        return { agent: artifact.agent, path: manifest.path, patchPath: manifest.patchPath, changed: patch.length > 0, baseCommit: manifest.baseHead, exportStatus: 'ready' };
    }
    forgetRun(runId) { this.runBases.delete(runId); }
    patch(manifest, signal) {
        return this.git(manifest.path, ['diff', '--binary', '--full-index', '--no-color', '--no-ext-diff', '--no-textconv',
            '--no-relative', '--src-prefix=a/', '--dst-prefix=b/', '--ignore-submodules=none', '--submodule=short', manifest.baseHead, '--'], undefined, signal);
    }
    async assertExportableSubmodules(manifest, signal) {
        const status = (await this.git(manifest.path, ['status', '--porcelain=v2', '-z', '--untracked-files=no', '--ignore-submodules=none'], undefined, signal)).toString('utf8').split('\0');
        for (let index = 0; index < status.length; index++) {
            const entry = status[index];
            const submodule = /^[12] [^ ]+ (S[.C][.M][.U]) /.exec(entry)?.[1];
            if (submodule && (submodule[2] === 'M' || submodule[3] === 'U')) {
                throw new Error('Uncommitted submodule files cannot be preserved in a parent Git patch. Inspect the retained worktree, then commit or separately export and review nested changes before retrying patch repair or cleanup.');
            }
            if (entry.startsWith('2 '))
                index++; // A rename record has a second NUL-delimited original path.
        }
    }
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
                await this.atomicWrite(this.manifestPath(manifest.path), JSON.stringify(manifest), 'workspace-manifests', false, signal);
            }
            return;
        }
        if (!present)
            throw new Error('The managed worktree directory is missing but its Git registration remains. Inspect it before cleanup.');
        const patch = await lstat(manifest.patchPath).catch(() => undefined);
        if (!patch?.isFile() || patch.isSymbolicLink())
            throw new Error('Export and review a regular patch file before cleanup.');
        await this.verify(artifact, signal, manifest, registered);
        if (options.discardChanges !== true)
            await this.assertExportableSubmodules(manifest, signal);
        const current = await this.patch(manifest, signal);
        const saved = await readFile(manifest.patchPath);
        if (options.discardChanges !== true && !current.equals(saved))
            throw new Error('The worktree changed since patch export. Export and review it again before cleanup.');
        signal.throwIfAborted();
        await this.git(this.root, ['worktree', 'remove', ...(options.discardChanges === true ? ['--force'] : []), manifest.path], undefined, signal);
        manifest.removed = true;
        await this.atomicWrite(this.manifestPath(manifest.path), JSON.stringify(manifest), 'workspace-manifests', false, signal);
    }
}
//# sourceMappingURL=workspaces.js.map