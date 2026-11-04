import { spawn } from 'node:child_process';
import { killProcessTree } from './process.js';
let vendor;
let finished = false;
let stopping = false;
let cleanup;
function cleanupVendor() {
    return cleanup ??= vendor?.pid ? killProcessTree(vendor.pid) : Promise.resolve();
}
function send(message) {
    if (process.connected)
        process.send?.(message, () => { });
}
async function finish(code, signal) {
    if (finished)
        return;
    finished = true;
    let cleanupFailed = false;
    try {
        await cleanupVendor();
    }
    catch {
        cleanupFailed = true;
    }
    send({ type: 'closed', code, signal, cleanupFailed });
    process.stdin.destroy();
    process.stdout.end();
    process.stderr.end();
    if (process.connected)
        process.disconnect();
}
function stop() {
    if (stopping || finished)
        return;
    stopping = true;
    void finish(null, 'SIGKILL');
}
process.on('disconnect', stop);
process.on('SIGTERM', stop);
process.on('SIGINT', stop);
process.stdout.on('error', stop);
process.stderr.on('error', stop);
process.on('message', (message) => {
    if (!message || typeof message !== 'object')
        return;
    const value = message;
    if (value.type === 'stop') {
        stop();
        return;
    }
    if (value.type !== 'start' || vendor || stopping || finished)
        return;
    if (typeof value.command !== 'string' || typeof value.cwd !== 'string' || !Array.isArray(value.args) || value.args.some(arg => typeof arg !== 'string')) {
        send({ type: 'launch_error' });
        void finish(1, null);
        return;
    }
    try {
        vendor = spawn(value.command, value.args, {
            cwd: value.cwd, env: process.env, shell: false, windowsHide: true,
            detached: process.platform !== 'win32', stdio: ['pipe', 'pipe', 'pipe'],
        });
        vendor.stdin.on('error', () => { });
        vendor.stdout.pipe(process.stdout, { end: false });
        vendor.stderr.pipe(process.stderr, { end: false });
        process.stdin.pipe(vendor.stdin);
        vendor.once('spawn', () => send({ type: 'started', pid: vendor.pid }));
        vendor.once('error', () => { send({ type: 'launch_error' }); void finish(1, null); });
        vendor.once('close', (code, signal) => { void finish(code, signal); });
    }
    catch {
        send({ type: 'launch_error' });
        void finish(1, null);
    }
});
//# sourceMappingURL=supervisor.js.map