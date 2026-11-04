import type { CouncilService } from './service.js';
import type { DelegationChangesPage, DelegationConfiguration, DelegationInput, DelegationReport, DelegationReportInput, DelegationReportsPage, DelegationResolutionInput, DelegationWatchInput, DelegationWatchResult } from './delegation-types.js';
export declare class DelegationRevisionConflict extends Error {
    readonly currentRevision: number;
    readonly code = "COUNCIL_DELEGATION_REVISION_CONFLICT";
    readonly status = 409;
    constructor(currentRevision: number);
}
/** Durable executive coordination. No providers, credentials, task permissions or worker prompts are changed. */
export declare class DelegationManager {
    private readonly service;
    private readonly db;
    private readonly shutdown;
    private readonly listeners;
    private readonly roomRecoveryAt;
    private closed;
    constructor(service: CouncilService);
    private open;
    private transaction;
    private notify;
    private state;
    private rooms;
    private bumpReports;
    get(): DelegationConfiguration;
    overview(): {
        mode: DelegationConfiguration['mode'];
        delegate: string;
        configurationRevision: number;
        reportRevision: number;
        openQuestions: number;
    };
    configure(value: DelegationInput): DelegationConfiguration;
    report(value: DelegationReportInput): DelegationReport;
    resolve(id: string, value: DelegationResolutionInput): DelegationReport;
    reportsPage(options?: {
        limit?: number;
        cursor?: number;
        openOnly?: boolean;
    }): DelegationReportsPage;
    reportChangesPage(options?: {
        after?: number;
        limit?: number;
    }): DelegationChangesPage;
    private snapshot;
    watch(value?: DelegationWatchInput, signal?: AbortSignal): Promise<DelegationWatchResult>;
    close(): void;
}
