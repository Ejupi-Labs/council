export declare class MetadataLimitError extends Error {
    constructor();
}
/** OS lookups remain counted after logical cancellation until their actual promise settles. */
export declare function metadataPressure(): {
    pending: number;
    limit: number;
};
/** Only side-effect-free metadata or bounded reads that own their internal handles may use this boundary. */
export declare function metadataLookup<T>(lookup: () => Promise<T>, signal?: AbortSignal): Promise<T>;
/** Cancel a cached lookup wait without issuing or counting the same OS operation twice. */
export declare function metadataResult<T>(work: Promise<T>, signal?: AbortSignal): Promise<T>;
