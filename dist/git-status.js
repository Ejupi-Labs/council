/** Porcelain-v2 byte parsing. Pathnames are opaque bytes, not UTF-8 text. */
export const GIT_STATUS_COMPONENT_BYTES = 64 * 1024 * 1024;
export const GIT_STATUS_OUTPUT_BYTES = 3 * GIT_STATUS_COMPONENT_BYTES;
export const DIRTY_SUBMODULE_ERROR = 'Uncommitted submodule files cannot be preserved in a parent Git patch. Inspect the retained worktree, then commit or separately export and review nested changes before retrying patch repair or cleanup.';
function malformed() { throw new Error('Malformed or incomplete Git porcelain-v2 status output.'); }
function tokenEnd(output, start, end) {
    const next = output.indexOf(32, start);
    if (next < start || next >= end)
        malformed();
    return next;
}
function mode(output, start, end) {
    if (end - start !== 6)
        malformed();
    for (let index = start; index < end; index++)
        if (output[index] < 48 || output[index] > 55)
            malformed();
}
function hash(output, start, end) {
    if (end - start !== 40 && end - start !== 64)
        malformed();
    for (let index = start; index < end; index++) {
        const byte = output[index];
        if (!(byte >= 48 && byte <= 57 || byte >= 97 && byte <= 102))
            malformed();
    }
}
function trackedEntry(output, start, end, kind) {
    let position = start + 2, next = tokenEnd(output, position, end);
    if (next - position !== 2)
        malformed();
    for (let index = position; index < next; index++) {
        const byte = output[index];
        if (byte !== 46 && byte !== 77 && byte !== 65 && byte !== 68 && byte !== 82 && byte !== 67 && byte !== 85 && byte !== 84)
            malformed();
    }
    position = next + 1;
    next = tokenEnd(output, position, end);
    const submodule = output[position] === 83;
    if (next - position !== 4 || !(submodule
        ? (output[position + 1] === 46 || output[position + 1] === 67) && (output[position + 2] === 46 || output[position + 2] === 77) && (output[position + 3] === 46 || output[position + 3] === 85)
        : output[position] === 78 && output[position + 1] === 46 && output[position + 2] === 46 && output[position + 3] === 46))
        malformed();
    if (kind !== 117 && submodule && (output[position + 2] === 77 || output[position + 3] === 85))
        throw new Error(DIRTY_SUBMODULE_ERROR);
    position = next + 1;
    for (let index = 0; index < (kind === 117 ? 4 : 3); index++) {
        next = tokenEnd(output, position, end);
        mode(output, position, next);
        position = next + 1;
    }
    for (let index = 0; index < (kind === 117 ? 3 : 2); index++) {
        next = tokenEnd(output, position, end);
        hash(output, position, next);
        position = next + 1;
    }
    if (kind === 50) {
        next = tokenEnd(output, position, end);
        if (next - position < 2 || next - position > 4 || output[position] !== 82 && output[position] !== 67)
            malformed();
        let score = 0;
        for (let index = position + 1; index < next; index++) {
            const byte = output[index];
            if (byte < 48 || byte > 57)
                malformed();
            score = score * 10 + byte - 48;
        }
        if (score > 100)
            malformed();
        position = next + 1;
    }
    if (position >= end)
        malformed();
}
function limit(value) {
    const result = value ?? GIT_STATUS_COMPONENT_BYTES;
    if (!Number.isSafeInteger(result) || result < 0 || result > GIT_STATUS_COMPONENT_BYTES)
        throw new Error('Git status component limits must be integers from 0 to 64 MiB.');
    return result;
}
function overBudget(kind, maximum) {
    throw new Error(`Git status ${kind} exceeds ${maximum === GIT_STATUS_COMPONENT_BYTES ? '64 MiB' : `${maximum} bytes`}.`);
}
/** Validate/count first, then allocate and copy exactly once. Limits can be reduced for bounded callers/tests. */
export function gitStatusUntrackedPaths(output, limits = {}) {
    const maxTracked = limit(limits.trackedBytes), maxUntracked = limit(limits.untrackedBytes);
    let tracked = 0, untracked = 0, position = 0;
    while (position < output.length) {
        const start = position, end = output.indexOf(0, start);
        if (end < 0 || end - start < 3 || output[start + 1] !== 32)
            malformed();
        const kind = output[start];
        position = end + 1;
        if (kind === 63) {
            untracked += end - start - 1; // Exclude '? ', include the pathname's NUL.
            if (untracked > maxUntracked)
                overBudget('untracked paths', maxUntracked);
            continue;
        }
        if (kind === 49 || kind === 50 || kind === 117) {
            trackedEntry(output, start, end, kind);
            if (kind === 50) {
                const originalEnd = output.indexOf(0, position);
                if (originalEnd <= position)
                    malformed();
                position = originalEnd + 1;
            }
        }
        else if (kind !== 35 && kind !== 33)
            malformed(); // Extensible headers and ignored paths are not staged.
        tracked += position - start;
        if (tracked > maxTracked)
            overBudget('tracked metadata', maxTracked);
    }
    const result = Buffer.allocUnsafe(untracked);
    let written = 0;
    position = 0;
    while (position < output.length) {
        const start = position, end = output.indexOf(0, start), kind = output[start];
        position = end + 1;
        if (kind === 63)
            written += output.copy(result, written, start + 2, end + 1);
        else if (kind === 50)
            position = output.indexOf(0, position) + 1;
    }
    if (written !== untracked)
        malformed();
    return result;
}
//# sourceMappingURL=git-status.js.map