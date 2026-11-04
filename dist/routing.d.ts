import type { RoutingContext, RoutingInput, RoutingPlan } from './routing-types.js';
/** Deterministic dry routing: no agent dispatch, task inference, prices, or vendor defaults. */
export declare function planRouting(value: RoutingInput, context: RoutingContext): RoutingPlan;
