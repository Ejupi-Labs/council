import { spawnSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { mkdirSync, writeFileSync } from 'node:fs';
import { dirname, join, relative, resolve } from 'node:path';
import type { WorkspaceArtifact } from './types.js';

export interface WorkspaceHandle extends WorkspaceArtifact { cwd: string; baseHead: string }

function segment(value: string): string {
  const label = value.replace(/[^a-zA-Z0-9_-]/g, '-').slice(0, 40) || 'workspace';
  return `${label}-${createHash('sha256').update(value).digest('hex').slice(0, 12)}`;
}

/** Retained detached worktrees: this class never changes the user's checkout. */
export class GitWorkspaces {
  private readonly root: string;
  private readonly state: string;

  constructor(workspaceRoot: string, stateDir: string) {
    this.root = resolve(workspaceRoot);
    this.state = resolve(stateDir);
  }

  private git(cwd: string, args: string[], input?: Buffer): Buffer {
    const result = spawnSync('git', ['-c', 'core.quotePath=false', '-C', cwd, ...args], {
      shell: false, windowsHide: true, timeout: 60_000, maxBuffer: 64 * 1024 * 1024,
      input, env: { ...process.env, GIT_TERMINAL_PROMPT: '0' },
    });
    if (result.error || result.status !== 0) {
      const detail = result.error?.message ?? result.stderr?.toString('utf8').trim() ?? 'unknown Git failure';
      throw new Error(`Git workspace operation failed: ${detail}`);
    }
    return result.stdout;
  }

  create(runId: string, agent: string, slot = agent): WorkspaceHandle {
    let repo: string;
    let head: string;
    try {
      repo = this.git(this.root, ['rev-parse', '--show-toplevel']).toString('utf8').trim();
      head = this.git(this.root, ['rev-parse', '--verify', 'HEAD^{commit}']).toString('utf8').trim();
    } catch {
      throw new Error('Write workflows require a Git repository with an existing HEAD commit. Commit the project before creating isolated worktrees.');
    }
    const suffix = relative(resolve(repo), this.root);
    if (suffix.startsWith('..') || resolve(repo, suffix) !== this.root) {
      throw new Error('The configured workspace must be inside its Git repository.');
    }
    const path = join(this.state, 'worktrees', segment(runId), segment(slot));
    const patchPath = join(this.state, 'patches', segment(runId), `${segment(slot)}.patch`);
    mkdirSync(dirname(path), { recursive: true });
    this.git(this.root, ['worktree', 'add', '--detach', path, head]);
    return { agent, path, patchPath, changed: false, cwd: join(path, suffix), baseHead: head };
  }

  export(handle: WorkspaceHandle): WorkspaceArtifact {
    // Intent-to-add makes new files appear in a normal binary-capable Git patch.
    // Passing NUL-separated paths on stdin avoids shell parsing and argv limits.
    const untracked = this.git(handle.path, ['ls-files', '--others', '--exclude-standard', '-z']);
    if (untracked.length > 0) {
      this.git(handle.path, ['add', '--intent-to-add', '--pathspec-from-file=-', '--pathspec-file-nul'], untracked);
    }
    const patch = this.git(handle.path, ['diff', '--binary', '--no-ext-diff', '--no-textconv', handle.baseHead, '--']);
    mkdirSync(dirname(handle.patchPath), { recursive: true });
    writeFileSync(handle.patchPath, patch);
    handle.changed = patch.length > 0;
    return { agent: handle.agent, path: handle.path, patchPath: handle.patchPath, changed: handle.changed };
  }
}
