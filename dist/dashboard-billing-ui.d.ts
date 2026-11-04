/** Browser-side accounting validation; unknown values remain null and timestamps are never refreshed implicitly. */
export declare function billingNumber(value: string, label: string): number | null;
export declare function billingTimestamp(value: string, label: string): string | null;
export declare function billingStable(value: unknown): string;
export declare function billingPatch(base: Record<string, any>, next: Record<string, any>): Record<string, unknown>;
export declare function billingInput(raw: Record<string, any>): Record<string, unknown>;
export declare function billingRaw(record: Record<string, any>): Record<string, any>;
export declare const BILLING_STYLES: string;
export declare const BILLING_DIALOG: string;
export declare const ROUTING_HELPER: string;
export declare function billingClientSource(): string;
