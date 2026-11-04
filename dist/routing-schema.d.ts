import { type JsonSchema } from './schema.js';
import type { RoutingInput, RoutingProfile } from './routing-types.js';
export declare const routingProfileDefinition: JsonSchema;
export declare const routingDefinition: JsonSchema;
export declare function parseRoutingInput(value: unknown): RoutingInput;
export declare function parseRoutingProfile(value: unknown): RoutingProfile;
