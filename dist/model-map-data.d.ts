import type { ModelMapCatalog, ModelMapObservation } from './model-map-types.js';
type Row = [model: string, score: number, effort: string | null, uncertainty?: string];
/** Internal curation helper. Observation identity survives row insertion, reordering and score corrections. */
export declare function createModelMapCohort(id: string, sourceId: string, benchmark: string, version: string, metric: string, unit: string, harness: string, rows: Row[], notes: string, sampleSize?: number): ModelMapObservation[];
export declare const MODEL_MAP_CATALOG: ModelMapCatalog;
export {};
