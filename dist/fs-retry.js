import { performance } from 'node:perf_hooks';
import { setTimeout as delay } from 'node:timers/promises';
const sharingErrors = new Set(['EPERM', 'EBUSY', 'EACCES']);
const retryWindowMs = 1000;
/** Retry atomic rename/owned staging cleanup, never data writes; await issued I/O without destructive fallback. */
export async function retryWindowsSharing(operation, signal) {
    const deadline = performance.now() + retryWindowMs;
    let backoffMs = 10;
    while (true) {
        signal?.throwIfAborted();
        try {
            const result = await operation();
            signal?.throwIfAborted();
            return result;
        }
        catch (error) {
            signal?.throwIfAborted();
            const code = error && typeof error === 'object' && 'code' in error ? error.code : undefined;
            const remaining = deadline - performance.now();
            if (process.platform !== 'win32' || typeof code !== 'string' || !sharingErrors.has(code) || remaining <= 0)
                throw error;
            await delay(Math.min(backoffMs, remaining), undefined, { signal });
            signal?.throwIfAborted();
            if (performance.now() >= deadline)
                throw error;
            backoffMs = Math.min(100, backoffMs * 2);
        }
    }
}
//# sourceMappingURL=fs-retry.js.map