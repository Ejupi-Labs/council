import { CouncilService } from './service.js';
import { CouncilMcpServer } from './rpc.js';
export declare function createMcpServer(service: CouncilService): CouncilMcpServer;
export declare function serveMcp(service: CouncilService): Promise<CouncilMcpServer>;
