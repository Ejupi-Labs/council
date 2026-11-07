import { ApiError } from './api.js';
const known = new Map();
const id = (value) => typeof value === 'string' && /^[a-f0-9]{32}$/.test(value);
function valid(taskId, ref) {
    return !!ref && id(ref.graphId) && id(ref.rootTaskId)
        && (ref.kind === 'root' ? ref.nodeId === null && taskId === ref.rootTaskId : ref.kind === 'node' && id(ref.nodeId) && taskId !== ref.rootTaskId);
}
function same(a, b) {
    return a.graphId === b.graphId && a.rootTaskId === b.rootTaskId && a.nodeId === b.nodeId && a.kind === b.kind;
}
export function rememberGraphTask(taskId, ref) {
    const prior = known.get(taskId);
    if (!valid(taskId, ref) || prior && !same(prior, ref))
        throw new ApiError('invalid_response', 'The task-group membership does not identify this task. Refresh its group; standalone controls remain unavailable.');
    known.set(taskId, { ...ref });
}
export function checkGraphTask(task) {
    const expected = known.get(task.id), actual = task.graphRef;
    if (expected && (!actual || !same(expected, actual)))
        throw new ApiError('invalid_response', 'The owner omitted or changed known task-group membership. Refresh its group; this task cannot fall back to standalone controls.');
    if (actual)
        rememberGraphTask(task.id, actual);
}
