import { Worker } from 'node:worker_threads';
const parentPid = Number(process.env.COUNCIL_OUTPUT_PARENT_PID);
if (!process.send || !Number.isSafeInteger(parentPid) || parentPid < 1)
    throw new Error('Council output pump requires its owning IPC parent.');
// Windows stdout can block this thread. An independent watchdog still reaps this
// output-only process after abrupt parent death, even while WriteFile is blocked.
const watchdog = new Worker(`
  const { workerData } = require('node:worker_threads');
  setInterval(() => {
    try { process.kill(workerData.parentPid, 0); }
    catch (error) { if (error.code === 'ESRCH') process.kill(process.pid, 'SIGKILL'); }
  }, 250);
`, { eval: true, workerData: { parentPid } });
process.on('disconnect', () => { void watchdog.terminate().finally(() => process.exit(0)); });
process.stdout.on('error', () => { process.exit(1); });
process.on('message', (message) => {
    if (!message || typeof message !== 'object')
        return;
    const frame = message;
    if (!Number.isSafeInteger(frame.id) || typeof frame.data !== 'string' || Buffer.byteLength(frame.data) > 32 * 1024 * 1024) {
        process.exit(1);
        return;
    }
    process.stdout.write(frame.data, error => {
        if (error) {
            process.exit(1);
            return;
        }
        if (process.connected)
            process.send?.({ id: frame.id }, () => { });
    });
});
//# sourceMappingURL=output-pump.js.map