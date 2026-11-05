import { isUtf8 } from 'node:buffer';
import { Transform } from 'node:stream';
export const MAX_MCP_FRAME_BYTES = 1024 * 1024;
export const MAX_MCP_NODES = 8192;
export const MAX_MCP_DEPTH = 64;
function whitespace(buffer) {
    return buffer.every(byte => byte === 0x20 || byte === 0x09 || byte === 0x0d);
}
/** Bound nesting before JSON.parse allocates a potentially deep object graph. */
function checkNesting(buffer) {
    let quoted = false;
    let escaped = false;
    let depth = 0;
    for (const byte of buffer) {
        if (quoted) {
            if (escaped)
                escaped = false;
            else if (byte === 0x5c)
                escaped = true;
            else if (byte === 0x22)
                quoted = false;
        }
        else if (byte === 0x22)
            quoted = true;
        else if (byte === 0x7b || byte === 0x5b) {
            // Root depth is zero. One extra container can be valid when empty.
            if (++depth > MAX_MCP_DEPTH + 1)
                throw new Error('Council MCP input exceeds the maximum JSON depth.');
        }
        else if (byte === 0x7d || byte === 0x5d)
            depth--;
    }
}
/** Iterative validation keeps its own stack within the node budget. */
function checkGraph(value) {
    const stack = [{ value, depth: 0 }];
    let visited = 0;
    const add = (child, depth) => {
        if (visited + stack.length >= MAX_MCP_NODES)
            throw new Error('Council MCP input exceeds the maximum JSON node count.');
        stack.push({ value: child, depth });
    };
    while (stack.length > 0) {
        const current = stack.pop();
        visited++;
        if (current.depth > MAX_MCP_DEPTH)
            throw new Error('Council MCP input exceeds the maximum JSON depth.');
        if (Array.isArray(current.value)) {
            for (const child of current.value)
                add(child, current.depth + 1);
        }
        else if (current.value !== null && typeof current.value === 'object') {
            for (const key in current.value) {
                if (Object.hasOwn(current.value, key))
                    add(current.value[key], current.depth + 1);
            }
        }
    }
}
/**
 * Validate newline-delimited MCP input before the SDK's schema parser sees it.
 * Limits count UTF-8 bytes before the newline, JSON values (including containers),
 * and graph depth with the root at zero. No partial frame is ever forwarded.
 */
export class BoundedMcpInput extends Transform {
    frame = Buffer.allocUnsafe(MAX_MCP_FRAME_BYTES);
    length = 0;
    active;
    processing = false;
    scheduled;
    constructor() { super({ decodeStrings: true, autoDestroy: true }); }
    _transform(chunk, _encoding, callback) {
        this.active = { chunk, offset: 0, callback };
        this.processChunk();
    }
    _read(size) {
        super._read(size);
        this.processChunk();
    }
    processChunk() {
        if (this.processing || this.scheduled || this.destroyed || !this.active)
            return;
        const work = this.active;
        const startOffset = work.offset;
        let frames = 0;
        this.processing = true;
        try {
            while (work.offset < work.chunk.length) {
                const newline = work.chunk.indexOf(0x0a, work.offset);
                const end = newline < 0 ? work.chunk.length : newline;
                const bytes = end - work.offset;
                if (this.length + bytes > MAX_MCP_FRAME_BYTES)
                    throw new Error('Council MCP input frame exceeds 1048576 bytes.');
                work.chunk.copy(this.frame, this.length, work.offset, end);
                this.length += bytes;
                work.offset = newline < 0 ? end : newline + 1;
                if (newline < 0)
                    break;
                const ready = this.forwardFrame();
                this.length = 0;
                if (this.destroyed || this.active !== work)
                    return;
                if (!ready)
                    return; // Resume only when the readable consumer requests more data.
                if (work.offset < work.chunk.length && (++frames >= 64 || work.offset - startOffset >= 64 * 1024)) {
                    this.scheduled = setImmediate(() => { this.scheduled = undefined; this.processChunk(); });
                    return;
                }
            }
            this.active = undefined;
            this.processing = false;
            work.callback();
        }
        catch (error) {
            this.active = undefined;
            this.processing = false;
            work.callback(error instanceof Error ? error : new Error('Council MCP input validation failed.'));
        }
        finally {
            this.processing = false;
        }
    }
    forwardFrame() {
        const frame = this.frame.subarray(0, this.length);
        if (whitespace(frame))
            return true;
        if (!isUtf8(frame))
            throw new Error('Council MCP input must contain valid UTF-8.');
        checkNesting(frame);
        let value;
        try {
            value = JSON.parse(frame.toString('utf8'));
        }
        catch {
            throw new Error('Council MCP input contains invalid JSON.');
        }
        checkGraph(value);
        // The fixed input buffer is reused; never expose a mutable view downstream.
        const output = Buffer.allocUnsafe(frame.length + 1);
        frame.copy(output);
        output[frame.length] = 0x0a;
        return this.push(output);
    }
    _destroy(error, callback) {
        if (this.scheduled)
            clearImmediate(this.scheduled);
        this.scheduled = undefined;
        const work = this.active;
        this.active = undefined;
        work?.callback(error ?? undefined);
        callback(error);
    }
    _flush(callback) {
        if (whitespace(this.frame.subarray(0, this.length)))
            callback();
        else
            callback(new Error('Council MCP input ended with an incomplete frame; a newline is required.'));
    }
}
//# sourceMappingURL=transport.js.map