import { Transform, type TransformCallback } from 'node:stream';
export declare const MAX_MCP_FRAME_BYTES: number;
export declare const MAX_MCP_NODES = 8192;
export declare const MAX_MCP_DEPTH = 64;
/**
 * Validate newline-delimited MCP input before the SDK's schema parser sees it.
 * Limits count UTF-8 bytes before the newline, JSON values (including containers),
 * and graph depth with the root at zero. No partial frame is ever forwarded.
 */
export declare class BoundedMcpInput extends Transform {
    private readonly frame;
    private length;
    constructor();
    _transform(chunk: Buffer, _encoding: BufferEncoding, callback: TransformCallback): void;
    private forwardFrame;
    _flush(callback: TransformCallback): void;
}
