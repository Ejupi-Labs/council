import { spawn, type ChildProcessWithoutNullStreams } from 'node:child_process';
import { killProcessTree } from './process.js';

let vendor: ChildProcessWithoutNullStreams | undefined;
let finished = false;
let stopping = false;
let cleanup: Promise<void> | undefined;

function cleanupVendor(): Promise<void> {
  return cleanup ??= vendor?.pid ? killProcessTree(vendor.pid) : Promise.resolve();
}

function send(message: unknown): void {
  if (process.connected) process.send?.(message, () => { /* The Council parent may have exited. */ });
}

async function finish(code: number | null, signal: NodeJS.Signals | null): Promise<void> {
  if (finished) return;
  finished = true;
  let cleanupFailed = false;
  try { await cleanupVendor(); } catch { cleanupFailed = true; }
  send({ type: 'closed', code, signal, cleanupFailed });
  process.stdin.destroy();
  process.stdout.end();
  process.stderr.end();
  if (process.connected) process.disconnect();
}

function stop(): void {
  if (stopping || finished) return;
  stopping = true;
  void finish(null, 'SIGKILL');
}

process.on('disconnect', stop);
process.on('SIGTERM', stop);
process.on('SIGINT', stop);
process.stdout.on('error', stop);
process.stderr.on('error', stop);
process.on('message', (message: unknown) => {
  if (!message || typeof message !== 'object') return;
  const value = message as Record<string, unknown>;
  if (value.type === 'stop') { stop(); return; }
  if (value.type !== 'start' || vendor || stopping || finished) return;
  if (typeof value.command !== 'string' || typeof value.cwd !== 'string' || !Array.isArray(value.args) || value.args.some(arg => typeof arg !== 'string')) {
    send({ type: 'launch_error' }); void finish(1, null); return;
  }
  try {
    vendor = spawn(value.command, value.args as string[], {
      cwd: value.cwd, env: process.env, shell: false, windowsHide: true,
      detached: process.platform !== 'win32', stdio: ['pipe', 'pipe', 'pipe'],
    });
    vendor.stdin.on('error', () => { /* Process outcome determines success. */ });
    vendor.stdout.pipe(process.stdout, { end: false });
    vendor.stderr.pipe(process.stderr, { end: false });
    process.stdin.pipe(vendor.stdin);
    vendor.once('spawn', () => send({ type: 'started', pid: vendor!.pid }));
    vendor.once('error', () => { send({ type: 'launch_error' }); void finish(1, null); });
    vendor.once('close', (code, signal) => { void finish(code, signal); });
  } catch { send({ type: 'launch_error' }); void finish(1, null); }
});
