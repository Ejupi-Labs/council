export const MAX_AGENT_JSON_DEPTH = 64;
export const MAX_AGENT_JSON_TOKENS = 65_536;

/** Bound graph allocation before JSON.parse; quoted tool output remains one token. */
export function checkAgentJsonStructure(frame: Buffer): void {
  if (frame.length >= 16 * 1024) { checkLargeFrame(frame); return; }
  let quoted = false, escaped = false, primitive = false;
  let depth = 0, tokens = 0;
  const token = (): void => {
    if (depth > MAX_AGENT_JSON_DEPTH) throw new Error('Agent protocol frame exceeded JSON depth 64.');
    if (++tokens > MAX_AGENT_JSON_TOKENS) throw new Error('Agent protocol frame exceeded 65536 structural tokens.');
  };
  for (const byte of frame) {
    if (quoted) {
      if (escaped) escaped = false;
      else if (byte === 0x5c) escaped = true;
      else if (byte === 0x22) quoted = false;
      continue;
    }
    if (byte === 0x22) { token(); quoted = true; primitive = false; }
    else if (byte === 0x7b || byte === 0x5b) { token(); depth++; primitive = false; }
    else if (byte === 0x7d || byte === 0x5d) { depth--; primitive = false; }
    else if (byte === 0x3a || byte === 0x2c || byte === 0x20 || byte === 0x09 || byte === 0x0d || byte === 0x0a) primitive = false;
    else if (!primitive) { token(); primitive = true; }
  }
}

/** Skip long quoted payloads natively; scan escape-heavy tails once without repeated searches. */
function checkLargeFrame(frame: Buffer): void {
  let depth = 0, tokens = 0, primitive = false;
  const token = (): void => {
    if (depth > MAX_AGENT_JSON_DEPTH) throw new Error('Agent protocol frame exceeded JSON depth 64.');
    if (++tokens > MAX_AGENT_JSON_TOKENS) throw new Error('Agent protocol frame exceeded 65536 structural tokens.');
  };
  for (let index = 0; index < frame.length; index++) {
    const byte = frame[index];
    if (byte === 0x22) {
      token(); primitive = false;
      let end = frame.indexOf(0x22, index + 1);
      if (end < 0) return; // JSON.parse reports an unterminated string.
      let before = end - 1, slashes = 0;
      while (before > index && frame[before--] === 0x5c) slashes++;
      if (slashes & 1) {
        let closed = false;
        for (let position = end + 1; position < frame.length; position++) {
          if (frame[position] === 0x5c) position++;
          else if (frame[position] === 0x22) { end = position; closed = true; break; }
        }
        if (!closed) return;
      }
      index = end;
    } else if (byte === 0x7b || byte === 0x5b) { token(); depth++; primitive = false; }
    else if (byte === 0x7d || byte === 0x5d) { depth--; primitive = false; }
    else if (byte === 0x3a || byte === 0x2c || byte === 0x20 || byte === 0x09 || byte === 0x0d || byte === 0x0a) primitive = false;
    else if (!primitive) { token(); primitive = true; }
  }
}
