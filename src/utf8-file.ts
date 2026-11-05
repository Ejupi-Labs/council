import { readFileSync } from 'node:fs';
import { isUtf8 } from 'node:buffer';
import { ValidationError } from './schema.js';

/** Preserve supplied evidence exactly; invalid bytes must not become replacement characters. */
export function readUtf8File(filename: string): string {
  const bytes = readFileSync(filename);
  if (!isUtf8(bytes)) throw new ValidationError('Input file must contain valid UTF-8. No content was replaced or truncated.');
  return bytes.toString('utf8').replace(/^\uFEFF/, '');
}
