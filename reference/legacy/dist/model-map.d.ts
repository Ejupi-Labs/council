import type { ModelMapCatalog, ModelMapConfig, ModelMapQuery, ModelMapView } from './model-map-types.js';
/** Validate identities and evidence without translating model names or combining benchmark scores. */
export declare function validateModelMap(value: unknown): ModelMapCatalog;
/** Read-only planning reference. Does not choose a running model, update weights, or inspect account availability. */
export declare function modelMapView(config: ModelMapConfig, value?: ModelMapQuery, baseCatalog?: ModelMapCatalog): ModelMapView;
