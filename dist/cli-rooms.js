import { resolve } from 'node:path';
import { readJsonFile } from './config.js';
export const ROOM_COMMANDS = ['rooms', 'room-create', 'room-show', 'room-history', 'room-message', 'room-task', 'room-discuss', 'room-execute', 'room-pause', 'room-resume', 'room-cancel', 'room-wait', 'room-recover'];
export const ROOM_ID_COMMANDS = ROOM_COMMANDS.filter(command => command !== 'rooms' && command !== 'room-create');
export function roomCommandOptions(command) {
    if (command === 'rooms')
        return ['limit'];
    if (['room-create', 'room-task', 'room-message'].includes(command))
        return ['input-file'];
    if (command === 'room-history')
        return ['limit', 'cursor'];
    if (command === 'room-wait')
        return ['wait-ms'];
    return [];
}
function integer(value, fallback, minimum, maximum, name) {
    if (value === undefined)
        return fallback;
    if (!/^(?:0|[1-9]\d*)$/.test(value) || !Number.isSafeInteger(Number(value)) || Number(value) < minimum || Number(value) > maximum)
        throw new Error(`${name} must be an integer from ${minimum} to ${maximum}.`);
    return Number(value);
}
export function validateRoomCommand(command, options) {
    if (['room-create', 'room-task', 'room-message'].includes(command) && !options['input-file']?.trim())
        throw new Error(`${command} requires --input-file with complete JSON input.`);
    if (command === 'rooms' || command === 'room-history')
        integer(options.limit, command === 'rooms' ? 20 : 100, 1, 100, '--limit');
    if (command === 'room-history')
        integer(options.cursor, 0, 0, Number.MAX_SAFE_INTEGER, '--cursor');
    if (command === 'room-wait')
        integer(options['wait-ms'], 600000, 0, 600000, '--wait-ms');
}
const active = (room) => room.status === 'discussing' || room.status === 'running';
const incomplete = (room) => ['failed', 'cancelled', 'needs_attention'].includes(room.status);
/** Lifecycle commands keep their owning process alive until the room reaches a stable state. */
export async function runRoomCommand(service, command, id, options) {
    validateRoomCommand(command, options);
    if (command === 'rooms')
        return { result: service.rooms.list(integer(options.limit, 20, 1, 100, '--limit')) };
    const input = () => readJsonFile(resolve(service.config.workspaceRoot, options['input-file']), Infinity);
    if (command === 'room-create')
        return { result: service.rooms.create(input()) };
    if (!id)
        throw new Error(`${command} requires a room ID.`);
    if (command === 'room-show')
        return { result: service.rooms.get(id) };
    if (command === 'room-history')
        return { result: service.rooms.messagePage(id, integer(options.limit, 100, 1, 100, '--limit'), options.cursor === undefined ? undefined : integer(options.cursor, 0, 0, Number.MAX_SAFE_INTEGER, '--cursor')) };
    if (command === 'room-message')
        return { result: service.rooms.postMessage(id, input()) };
    if (command === 'room-task')
        return { result: service.rooms.addTask(id, input()) };
    if (command === 'room-recover') {
        service.rooms.recover(id);
        return { result: service.rooms.get(id) };
    }
    if (command === 'room-cancel')
        return { result: service.rooms.cancel(id) };
    if (command === 'room-wait') {
        const room = await service.rooms.wait(id, integer(options['wait-ms'], 600000, 0, 600000, '--wait-ms'));
        const timedOut = active(room);
        return { result: { room, timedOut }, ...(timedOut || incomplete(room) ? { exitCode: 1 } : {}) };
    }
    let room;
    if (command === 'room-discuss')
        room = service.rooms.startDiscussion(id);
    else if (command === 'room-execute')
        room = service.rooms.execute(id);
    else if (command === 'room-pause')
        room = service.rooms.pause(id);
    else if (command === 'room-resume')
        room = service.rooms.resume(id);
    else
        throw new Error(`Unknown room command: ${command}`);
    process.stderr.write(`${JSON.stringify({ event: 'room.accepted', id: room.id, action: command, status: room.status })}\n`);
    while (active(room))
        room = await service.rooms.wait(id, 600000);
    const expected = command === 'room-execute' ? room.status === 'completed'
        : command === 'room-pause' ? room.status === 'paused'
            : room.status === 'ready' || room.status === 'completed';
    return { result: room, ...(!expected ? { exitCode: 1 } : {}) };
}
//# sourceMappingURL=cli-rooms.js.map