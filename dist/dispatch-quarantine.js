/** Fixed metadata only: cleanup uncertainty never proves a surviving orphan. */
export const PROCESS_CLEANUP_STAGES = ['agent_call', 'git_create', 'git_export', 'maintenance_export', 'maintenance_cleanup'];
export const QUARANTINE_PERSISTENCE_WARNING = 'The cleanup incident could not be persisted. Other hosts may be unaware; a shared dispatch block is not guaranteed.';
export const QUARANTINE_INSPECTION_REQUIRED = 'Subprocess cleanup was not verified in this workspace. Inspect OS processes and retained worktrees before accepting changes or starting more work.';
/** Expected operational refusal; it does not imply unavailable or corrupt storage. */
export class DispatchQuarantinedError extends Error {
    status = 409;
    code = 'COUNCIL_DISPATCH_QUARANTINED';
    receipt;
    constructor(receipt) {
        if (typeof receipt?.currentRevision !== 'string' || !/^(?:0|[1-9][0-9]{0,18})$/.test(receipt.currentRevision) || BigInt(receipt.currentRevision) > 9223372036854775807n
            || !Number.isSafeInteger(receipt.openCount) || receipt.openCount < 1)
            throw new Error('Invalid dispatch quarantine receipt.');
        super(`Workflow dispatch is quarantined by ${receipt.openCount} unresolved cleanup incident(s) at revision ${receipt.currentRevision}. ${QUARANTINE_INSPECTION_REQUIRED}`);
        this.name = 'DispatchQuarantinedError';
        this.receipt = Object.freeze({ blocked: true, currentRevision: receipt.currentRevision, openCount: receipt.openCount });
    }
}
//# sourceMappingURL=dispatch-quarantine.js.map