const LIMIT = 128;
let pending = 0;
export class MetadataLimitError extends Error {
    constructor() { super('Filesystem metadata lookup limit reached; wait for pending OS lookups before retrying.'); this.name = 'MetadataLimitError'; }
}
/** OS lookups remain counted after logical cancellation until their actual promise settles. */
export function metadataPressure() { return { pending, limit: LIMIT }; }
/** Only side-effect-free metadata or bounded reads that own their internal handles may use this boundary. */
export function metadataLookup(lookup, signal) {
    if (signal?.aborted)
        return Promise.reject(signal.reason);
    if (pending >= LIMIT)
        return Promise.reject(new MetadataLimitError());
    pending++;
    let work;
    try {
        work = Promise.resolve(lookup());
    }
    catch (error) {
        pending--;
        return Promise.reject(error);
    }
    const tracked = work.finally(() => { pending--; });
    return metadataResult(tracked, signal);
}
/** Cancel a cached lookup wait without issuing or counting the same OS operation twice. */
export function metadataResult(work, signal) {
    if (!signal)
        return work;
    if (signal.aborted) {
        void work.catch(() => { });
        return Promise.reject(signal.reason);
    }
    return new Promise((resolveValue, reject) => {
        const abort = () => { reject(signal.reason); };
        signal.addEventListener('abort', abort, { once: true });
        void work.then(value => {
            signal.removeEventListener('abort', abort);
            if (signal.aborted)
                reject(signal.reason);
            else
                resolveValue(value);
        }, error => { signal.removeEventListener('abort', abort); reject(error); });
    });
}
//# sourceMappingURL=metadata.js.map