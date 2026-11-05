import type { CouncilService } from './service.js';
import type { RoomManager } from './rooms.js';
/** An authenticated loopback interface. Closing it never closes the owning service or rooms. */
export declare function serveDashboard(service: CouncilService, rooms: RoomManager, options?: {
    port?: number;
}): Promise<{
    url: string;
    close(): Promise<void>;
}>;
