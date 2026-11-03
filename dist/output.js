import { fork } from 'node:child_process';
import { fileURLToPath } from 'node:url';
/** Keep potentially blocking platform stdout writes out of the MCP event loop. */
export class OutputPump {
    child;
    pending = new Map();
    sequence = 0;
    closed = false;
    closePromise;
    constructor() {
        this.child = fork(fileURLToPath(new URL('./output-pump.js', import.meta.url)), [], {
            execPath: process.execPath, execArgv: [], serialization: 'advanced',
            windowsHide: true, stdio: ['ignore', 'inherit', 'ignore', 'ipc'],
            env: { SystemRoot: process.env.SystemRoot, WINDIR: process.env.WINDIR, COUNCIL_OUTPUT_PARENT_PID: String(process.pid) },
        });
        this.child.on('message', (message) => {
            if (!message || typeof message !== 'object')
                return;
            const id = message.id;
            const operation = id === undefined ? undefined : this.pending.get(id);
            if (operation) {
                this.pending.delete(id);
                operation.accept();
            }
        });
        this.child.once('error', () => this.fail());
        this.child.once('exit', () => this.fail());
    }
    fail() {
        this.closed = true;
        for (const operation of this.pending.values())
            operation.reject(new Error('Council output connection closed.'));
        this.pending.clear();
    }
    write(data, signal) {
        if (this.closed || !this.child.connected)
            return Promise.reject(new Error('Council output connection closed.'));
        if (signal?.aborted)
            return Promise.reject(new Error('Council output was cancelled.'));
        const id = ++this.sequence;
        return new Promise((accept, reject) => {
            const abort = () => { this.pending.delete(id); reject(new Error('Council output was cancelled.')); };
            const settled = (operation) => { signal?.removeEventListener('abort', abort); operation(); };
            this.pending.set(id, { accept: () => settled(accept), reject: error => settled(() => reject(error)) });
            signal?.addEventListener('abort', abort, { once: true });
            const failed = () => { this.pending.delete(id); settled(() => reject(new Error('Unable to write the Council output channel.'))); };
            try {
                this.child.send({ id, data }, error => { if (error)
                    failed(); });
            }
            catch {
                failed();
            }
        });
    }
    close() {
        if (this.closePromise)
            return this.closePromise;
        this.fail();
        this.closePromise = new Promise(resolve => {
            if (this.child.exitCode !== null || this.child.signalCode !== null) {
                resolve();
                return;
            }
            const timer = setTimeout(() => {
                this.child.unref();
                this.child.channel?.unref();
                resolve();
            }, 1000);
            this.child.once('exit', () => { clearTimeout(timer); resolve(); });
            try {
                this.child.kill('SIGKILL');
            }
            catch {
                this.child.unref();
                this.child.channel?.unref();
            }
        });
        return this.closePromise;
    }
}
//# sourceMappingURL=output.js.map