import type { CouncilService } from './service.js';
export declare const ROOM_COMMANDS: readonly ['rooms', 'room-create', 'room-show', 'room-history', 'room-message', 'room-task', 'room-discuss', 'room-execute', 'room-pause', 'room-resume', 'room-cancel', 'room-wait', 'room-recover'];
export declare const ROOM_ID_COMMANDS: readonly string[];
interface RoomCliOptions {
    'input-file'?: string;
    limit?: string;
    cursor?: string;
    'wait-ms'?: string;
}
export declare function roomCommandOptions(command: string): readonly string[];
export declare function validateRoomCommand(command: string, options: RoomCliOptions): void;
/** Lifecycle commands keep their owning process alive until the room reaches a stable state. */
export declare function runRoomCommand(service: CouncilService, command: string, id: string | undefined, options: RoomCliOptions): Promise<{
    result: unknown;
    exitCode?: number;
}>;
export {};
