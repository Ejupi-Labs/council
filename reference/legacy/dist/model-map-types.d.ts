import type { CouncilConfig } from './types.js';
export type ModelMapRole = 'lead' | 'worker' | 'reviewer' | 'advisor' | 'economy';
export interface ModelMapSource {
    id: string;
    title: string;
    url: string;
    retrievedAt: string;
    publisher: string;
    type: 'benchmark-owner' | 'vendor' | 'methodology';
    publishedAt?: string;
}
export interface ModelMapObservation {
    id: string;
    sourceId: string;
    modelId: string;
    benchmark: string;
    version: string;
    metric: string;
    score: number;
    unit: string;
    direction: 'higher' | 'lower';
    harness: string;
    effort: string | null;
    cohort: string;
    notes?: string;
    sampleSize?: number;
    uncertainty?: string;
}
export interface ModelMapRecommendation {
    task: string;
    /** Positive ordinal preference within this task and role. Lower numbers are preferred; never an IQ score. */
    priority: number;
    role: ModelMapRole;
    basis: 'benchmark' | 'vendor-guidance' | 'policy-inference';
    confidence: 'high' | 'medium' | 'low';
    reason: string;
    sourceIds: string[];
}
export interface ModelProfile {
    id: string;
    name: string;
    provider: string;
    aliases?: string[];
    status: 'verified' | 'unverified' | 'legacy';
    strengths: string[];
    limitations: string[];
    evidenceIds: string[];
    recommendations: ModelMapRecommendation[];
    effortNotes?: string;
    referencePricing?: {
        inputPerMillion: number;
        outputPerMillion: number;
        currency: string;
        sourceId: string;
    };
    harnesses?: string[];
}
export interface ModelMapTask {
    id: string;
    label: string;
    description: string;
}
export interface ModelMapCatalog {
    asOf: string;
    revision: number;
    sources: ModelMapSource[];
    observations: ModelMapObservation[];
    models: ModelProfile[];
    tasks: ModelMapTask[];
}
/** Arrays replace complete records with matching IDs, or append new records. Omitted records are retained. */
export interface ModelMapOverlay {
    sources?: ModelMapSource[];
    observations?: ModelMapObservation[];
    models?: ModelProfile[];
    tasks?: ModelMapTask[];
}
export interface ModelMapQuery {
    task?: string;
    model?: string;
    role?: ModelMapRole;
    harness?: string;
    includeFullCatalog?: boolean;
}
export type ModelMapConfig = Pick<CouncilConfig, 'workspaceRoot' | 'stateDir' | 'agents'>;
export interface ModelMapBinding {
    alias: string;
    profile: string;
    model: string | null;
    match: 'exact' | 'unresolved' | 'unknown';
    catalogModelId?: string;
    enabled: boolean;
}
export interface ModelMapSuggestion extends ModelMapRecommendation {
    modelId: string;
    modelName: string;
    status: ModelProfile['status'];
    tied: boolean;
}
export interface ModelMapRanking {
    cohort: string;
    benchmark: string;
    version: string;
    metric: string;
    unit: string;
    direction: 'higher' | 'lower';
    entries: {
        rank: number;
        observationId: string;
        modelId: string;
        score: number;
        harness: string;
        effort: string | null;
    }[];
}
export interface ModelMapView {
    /** Complete evidence for matching profiles; the full reference catalog is an explicit opt-in. */
    catalog: ModelMapCatalog;
    catalogScope: 'matching-models' | 'full';
    totalModels: number;
    query: ModelMapQuery;
    models: ModelProfile[];
    hierarchy: ModelMapSuggestion[];
    rankings: ModelMapRanking[];
    bindings: ModelMapBinding[];
    warnings: string[];
    overrideApplied: boolean;
    overridePath: string;
}
