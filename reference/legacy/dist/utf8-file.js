import { closeSync, fstatSync, openSync, readFileSync, readSync } from 'node:fs';
import { isUtf8 } from 'node:buffer';
import { ValidationError } from './schema.js';
/** Preserve supplied evidence exactly; invalid bytes must not become replacement characters. */
export function readUtf8File(filename, maxBytes = Infinity) {
    if (maxBytes !== Infinity && (!Number.isSafeInteger(maxBytes) || maxBytes < 1))
        throw new ValidationError('UTF-8 file byte limit must be a positive integer or Infinity.');
    let bytes;
    if (maxBytes === Infinity)
        bytes = readFileSync(filename);
    else {
        const descriptor = openSync(filename, 'r');
        try {
            if (fstatSync(descriptor).size > maxBytes)
                throw new ValidationError('Input file exceeds its configured file byte limit.');
            const chunks = [];
            let total = 0, filled = 0;
            let chunk;
            for (;;) {
                const remaining = maxBytes - total;
                if (!chunk) {
                    chunk = Buffer.allocUnsafe(remaining >= 65536 ? 65536 : remaining + 1);
                    filled = 0;
                }
                const count = readSync(descriptor, chunk, filled, chunk.length - filled, null);
                if (!count) {
                    if (filled)
                        chunks.push(chunk.subarray(0, filled));
                    break;
                }
                if (count > remaining)
                    throw new ValidationError('Input file exceeds its configured file byte limit.');
                total += count;
                filled += count;
                if (filled === chunk.length) {
                    chunks.push(chunk);
                    chunk = undefined;
                }
            }
            bytes = Buffer.concat(chunks, total);
        }
        finally {
            closeSync(descriptor);
        }
    }
    if (!isUtf8(bytes))
        throw new ValidationError('Input file must contain valid UTF-8. No content was replaced or truncated.');
    return bytes.toString('utf8').replace(/^\uFEFF/, '');
}
//# sourceMappingURL=utf8-file.js.map