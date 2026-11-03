import { isUtf8 } from 'node:buffer';
import { Transform, type TransformCallback } from 'node:stream';

export const MAX_MCP_FRAME_BYTES = 1024 * 1024;
export const MAX_MCP_NODES = 8192;
export const MAX_MCP_DEPTH = 64;

function whitespace(buffer: Buffer): boolean {
  return buffer.every(byte => byte === 0x20 || byte === 0x09 || byte === 0x0d);
}

/** Bound nesting before JSON.parse allocates a potentially deep object graph. */
function checkNesting(buffer: Buffer): void {
  let quoted = false;
  let escaped = false;
  let depth = 0;
  for (const byte of buffer) {
    if (quoted) {
      if (escaped) escaped = false;
      else if (byte === 0x5c) escaped = true;
      else if (byte === 0x22) quoted = false;
    } else if (byte === 0x22) quoted = true;
    else if (byte === 0x7b || byte === 0x5b) {
      // Root depth is zero. One extra container can be valid when empty.
      if (++depth > MAX_MCP_DEPTH + 1) throw new Error('Council MCP input exceeds the maximum JSON depth.');
    } else if (byte === 0x7d || byte === 0x5d) depth--;
  }
}

/** Iterative validation keeps its own stack within the node budget. */
function checkGraph(value: unknown): void {
  const stack: Array<{ value: unknown; depth: number }> = [{ value, depth: 0 }];
  let visited = 0;
  const add = (child: unknown, depth: number): void => {
    if (visited + stack.length >= MAX_MCP_NODES) throw new Error('Council MCP input exceeds the maximum JSON node count.');
    stack.push({ value: child, depth });
  };
  while (stack.length > 0) {
    const current = stack.pop()!;
    visited++;
    if (current.depth > MAX_MCP_DEPTH) throw new Error('Council MCP input exceeds the maximum JSON depth.');
    if (Array.isArray(current.value)) {
      for (const child of current.value) add(child, current.depth + 1);
    } else if (current.value !== null && typeof current.value === 'object') {
      for (const key in current.value) {
        if (Object.hasOwn(current.value, key)) add((current.value as Record<string, unknown>)[key], current.depth + 1);
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
  private readonly frame = Buffer.allocUnsafe(MAX_MCP_FRAME_BYTES);
  private length = 0;

  constructor() { super({ decodeStrings: true, autoDestroy: true }); }

  override _transform(chunk: Buffer, _encoding: BufferEncoding, callback: TransformCallback): void {
    try {
      let start = 0;
      while (start < chunk.length) {
        const newline = chunk.indexOf(0x0a, start);
        const end = newline < 0 ? chunk.length : newline;
        const bytes = end - start;
        if (this.length + bytes > MAX_MCP_FRAME_BYTES) throw new Error('Council MCP input frame exceeds 1048576 bytes.');
        chunk.copy(this.frame, this.length, start, end);
        this.length += bytes;
        if (newline < 0) break;
        this.forwardFrame();
        this.length = 0;
        start = newline + 1;
      }
      callback();
    } catch (error) {
      callback(error instanceof Error ? error : new Error('Council MCP input validation failed.'));
    }
  }

  private forwardFrame(): void {
    const frame = this.frame.subarray(0, this.length);
    if (whitespace(frame)) return;
    if (!isUtf8(frame)) throw new Error('Council MCP input must contain valid UTF-8.');
    checkNesting(frame);
    let value: unknown;
    try { value = JSON.parse(frame.toString('utf8')); }
    catch { throw new Error('Council MCP input contains invalid JSON.'); }
    checkGraph(value);
    // The fixed input buffer is reused; never expose a mutable view downstream.
    const output = Buffer.allocUnsafe(frame.length + 1);
    frame.copy(output);
    output[frame.length] = 0x0a;
    this.push(output);
  }

  override _flush(callback: TransformCallback): void {
    if (whitespace(this.frame.subarray(0, this.length))) callback();
    else callback(new Error('Council MCP input ended with an incomplete frame; a newline is required.'));
  }
}
