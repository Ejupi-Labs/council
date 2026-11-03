import { fork, type ChildProcess, type ForkOptions } from 'node:child_process';
import { fileURLToPath } from 'node:url';

/** Keep potentially blocking platform stdout writes out of the MCP event loop. */
export class OutputPump {
  private readonly child: ChildProcess;
  private readonly pending = new Map<number, { accept: () => void; reject: (error: Error) => void }>();
  private sequence = 0;
  private closed = false;
  private closePromise?: Promise<void>;

  constructor() {
    this.child = fork(fileURLToPath(new URL('./output-pump.js', import.meta.url)), [], {
      execPath: process.execPath, execArgv: [], serialization: 'advanced',
      windowsHide: true, stdio: ['ignore', 'inherit', 'ignore', 'ipc'],
      env: { SystemRoot: process.env.SystemRoot, WINDIR: process.env.WINDIR, COUNCIL_OUTPUT_PARENT_PID: String(process.pid) },
    } as ForkOptions & { windowsHide: boolean });
    this.child.on('message', (message: unknown) => {
      if (!message || typeof message !== 'object') return;
      const id = (message as { id?: number }).id;
      const operation = id === undefined ? undefined : this.pending.get(id);
      if (operation) { this.pending.delete(id!); operation.accept(); }
    });
    this.child.once('error', () => this.fail());
    this.child.once('exit', () => this.fail());
  }

  private fail(): void {
    this.closed = true;
    for (const operation of this.pending.values()) operation.reject(new Error('Council output connection closed.'));
    this.pending.clear();
  }

  write(data: string, signal?: AbortSignal): Promise<void> {
    if (this.closed || !this.child.connected) return Promise.reject(new Error('Council output connection closed.'));
    if (signal?.aborted) return Promise.reject(new Error('Council output was cancelled.'));
    const id = ++this.sequence;
    return new Promise<void>((accept, reject) => {
      const abort = () => { this.pending.delete(id); reject(new Error('Council output was cancelled.')); };
      const settled = (operation: () => void) => { signal?.removeEventListener('abort', abort); operation(); };
      this.pending.set(id, { accept: () => settled(accept), reject: error => settled(() => reject(error)) });
      signal?.addEventListener('abort', abort, { once: true });
      const failed = () => { this.pending.delete(id); settled(() => reject(new Error('Unable to write the Council output channel.'))); };
      try { this.child.send({ id, data }, error => { if (error) failed(); }); }
      catch { failed(); }
    });
  }

  close(): Promise<void> {
    if (this.closePromise) return this.closePromise;
    this.fail();
    this.closePromise = new Promise<void>(resolve => {
      if (this.child.exitCode !== null || this.child.signalCode !== null) { resolve(); return; }
      const timer = setTimeout(() => {
        this.child.unref(); this.child.channel?.unref(); resolve();
      }, 1000);
      this.child.once('exit', () => { clearTimeout(timer); resolve(); });
      try { this.child.kill('SIGKILL'); } catch { this.child.unref(); this.child.channel?.unref(); }
    });
    return this.closePromise;
  }
}
