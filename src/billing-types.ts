export type BillingMode = 'unknown' | 'api' | 'subscription' | 'local';
export interface BillingSource { kind: 'manual' | 'reported'; label: string }
export interface BillingWindow {
  id: string; label: string; unit: string;
  scope: 'subscription' | 'credits' | 'other';
  limit: number | null; remaining: number | null; used: number | null; resetAt: string | null;
}
export type BillingWindowInput = Pick<BillingWindow, 'id' | 'label' | 'unit'> & Partial<Omit<BillingWindow, 'id' | 'label' | 'unit'>>;
export interface BillingPrices { currency: string | null; inputPerMillion: number | null; outputPerMillion: number | null }
export interface BillingRouting {
  tier: 'small' | 'standard' | 'large'; capabilities: string[];
  costRank: number | null; qualityRank: number | null;
}
export interface BillingInput {
  mode?: BillingMode; accountKey?: string | null; plan?: string | null;
  source?: BillingSource | null; observedAt?: string | null; validUntil?: string | null;
  windows?: BillingWindowInput[]; prices?: Partial<BillingPrices> | null;
  routing?: (Pick<BillingRouting, 'tier'> & Partial<Omit<BillingRouting, 'tier'>>) | null;
  /** null inherits the configured agent skills; [] explicitly disables them. */
  skills?: string[] | null;
  stopAtSubscriptionLimit?: boolean;
  /** null or [] selects every subscription-scoped window, never paid credits. */
  subscriptionWindowIds?: string[] | null;
}
export interface BillingRecord {
  alias: string; enabled: boolean; profile: string; model: string | null; revision: number;
  mode: BillingMode; accountKey: string | null; plan: string | null;
  source: BillingSource | null; observedAt: string | null; validUntil: string | null; stale: boolean;
  windows: BillingWindow[]; prices: BillingPrices | null; routing: BillingRouting | null; skills: string[] | null;
  stopAtSubscriptionLimit: boolean; subscriptionWindowIds: string[] | null;
  /** Exhaustion observations retained until a newer fresh positive allowance is observed. */
  subscriptionExhausted: Record<string, string>;
}
export interface SubscriptionProtection {
  enabled: boolean; allowed: boolean; status: 'disabled' | 'available' | 'exhausted' | 'unknown';
  alias: string; accountKey: string | null; evaluatedAt: string; observedAt: string | null;
  windowIds: string[]; reasons: string[];
}
export type BillingAvailabilityStatus = 'available' | 'exhausted' | 'unknown';
export interface BillingAvailabilityWindow extends BillingWindow {
  state: BillingAvailabilityStatus; effectiveRemaining: number | null; reasons: string[];
}
export interface BillingAvailability {
  status: BillingAvailabilityStatus; alias: string; accountKey: string | null;
  observedAt: string | null; evaluatedAt: string; reasons: string[]; windows: BillingAvailabilityWindow[];
}
