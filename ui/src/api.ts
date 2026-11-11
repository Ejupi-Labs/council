import type { Meta, Success } from './types.js';

export const ACCESS_KEY = 'council.accessToken';
const CACHE_PREFIX = 'council.foundation.';
let accessToken = '';
let verified = false;

export function readSession<T>(key: string): T | null {
  try {
    const value = sessionStorage.getItem(CACHE_PREFIX + key);
    return value ? JSON.parse(value) as T : null;
  } catch { return null; }
}

export function writeSession(key: string, value: unknown): void {
  try {
    if (value === null) sessionStorage.removeItem(CACHE_PREFIX + key);
    else sessionStorage.setItem(CACHE_PREFIX + key, JSON.stringify(value));
  } catch { /* A denied/full cache never changes a durable save acknowledgement. */ }
}

export function consumeAccessToken(): boolean {
  const fragment = location.hash.slice(1);
  // Only the launcher's "#token=" fragment carries credentials. Any other fragment is in-page
  // navigation (#tasks, #agents, ...): it must neither replace the stored token nor be consumed.
  if (fragment.startsWith('token=')) {
    history.replaceState(null, '', location.pathname + location.search);
    try {
      accessToken = new URLSearchParams(fragment).get('token') ?? '';
    } catch { accessToken = ''; }
    try {
      if (accessToken) sessionStorage.setItem(ACCESS_KEY, accessToken);
      else sessionStorage.removeItem(ACCESS_KEY);
    } catch { /* Access remains usable in this page when storage is unavailable. */ }
  } else {
    try { accessToken = sessionStorage.getItem(ACCESS_KEY) ?? ''; } catch { accessToken = ''; }
  }
  return accessToken.length > 0;
}

export function requestId(): string {
  const bytes = crypto.getRandomValues(new Uint8Array(16));
  return Array.from(bytes, value => value.toString(16).padStart(2, '0')).join('');
}

export class ApiError extends Error {
  constructor(
    readonly code: string,
    message: string,
    readonly requestId = '',
    readonly currentRevision?: string,
    readonly uncertain = false,
  ) { super(message); }
}

export async function api<T>(path: string, method = 'GET', body?: object): Promise<Success<T>> {
  if (method !== 'GET' && !verified) {
    throw new ApiError('owner_unverified', 'Reconnect to this local owner before saving.');
  }
  let response: Response;
  try {
    response = await fetch('/api/v1' + path, {
      method,
      headers: {
        Authorization: 'Bearer ' + accessToken,
        ...(body ? { 'Content-Type': 'application/json; charset=utf-8' } : {}),
      },
      ...(body ? { body: JSON.stringify(body) } : {}),
      credentials: 'omit',
      redirect: 'error',
      cache: 'no-store',
      referrerPolicy: 'no-referrer',
    });
  } catch {
    throw new ApiError('connection_failed', 'The local owner could not be reached. Reconnect, then retry this same request.', '', undefined, method !== 'GET');
  }
  let envelope: Success<T> & { error?: { code: string; message: string; currentRevision?: string } };
  try { envelope = await response.json() as typeof envelope; }
  catch {
    throw new ApiError('invalid_response', 'The local owner returned an unreadable response. Reconnect and retry this same request.', '', undefined, method !== 'GET');
  }
  if (envelope.apiVersion !== 1 || typeof envelope.requestId !== 'string') {
    throw new ApiError('incompatible_owner', 'This owner uses an incompatible API. Open Council again.', '', undefined, method !== 'GET');
  }
  if (!response.ok || envelope.error) {
    const error = envelope.error;
    const code = error?.code ?? 'invalid_response';
    if (response.status === 401 || response.status === 403) verified = false;
    throw new ApiError(code, error?.message ?? 'The local operation failed. Reconnect and retry.', envelope.requestId,
      error?.currentRevision, method !== 'GET' && (response.status >= 500 || code === 'write_uncertain'));
  }
  if (method !== 'GET' && (!envelope.receipt || envelope.receipt.requestId !== (body as { requestId?: string })?.requestId)) {
    throw new ApiError('missing_receipt', 'No saved acknowledgement was received. Retry this same request to reconcile its outcome.', envelope.requestId, undefined, true);
  }
  return envelope;
}

function validMetadataId(value: unknown): value is string {
  return typeof value === 'string' && /^[a-f0-9]{32}$/.test(value);
}

function validMetadataRevision(value: unknown): boolean {
  return typeof value === 'string' && /^[1-9][0-9]{0,18}$/.test(value) && BigInt(value) <= 9223372036854775807n;
}

export function supportsGraphCapacity(capacity: Meta['capacity']): boolean {
  if (!capacity || capacity.resourceId !== 'managed-native' || capacity.limit !== 2
    || Object.keys(capacity).length !== 5
    || !['resourceId', 'limit', 'activeAttemptId', 'occupiedSlots', 'activeClaims'].every(key => Object.hasOwn(capacity, key))
    || !Array.isArray(capacity.activeClaims) || capacity.activeClaims.length > 2
    || capacity.occupiedSlots !== String(capacity.activeClaims.length)) return false;
  const seen = new Set<string>(); let previousSlot = '';
  for (const claim of capacity.activeClaims) {
    if (!claim || typeof claim !== 'object' || Array.isArray(claim) || Object.keys(claim).length !== 9
      || !['slot', 'attemptId', 'graphId', 'nodeId', 'executorKind', 'contextId', 'capturedDeclaredAccountId', 'capturedLinkRevision', 'legacyBarrier'].every(key => Object.hasOwn(claim, key))
      || !['1', '2'].includes(claim.slot) || claim.slot <= previousSlot || !validMetadataId(claim.attemptId) || seen.has(claim.attemptId)
      || typeof claim.legacyBarrier !== 'boolean') return false;
    previousSlot = claim.slot; seen.add(claim.attemptId);
    if (![claim.graphId, claim.nodeId, claim.capturedDeclaredAccountId].every(id => id === null || validMetadataId(id))
      || claim.nodeId !== null && claim.graphId === null
      || claim.contextId !== null && !['codex-native', 'claude-native'].includes(claim.contextId)
      || !['native', 'local_check', 'local_application', 'local_external', 'local_graph'].includes(claim.executorKind)
      || claim.capturedLinkRevision !== null && !validMetadataRevision(claim.capturedLinkRevision)
      || (claim.capturedDeclaredAccountId === null) !== (claim.capturedLinkRevision === null)
      || claim.legacyBarrier && (capacity.activeClaims.length !== 1 || claim.slot !== '1')) return false;
    if (claim.executorKind === 'native') {
      if (!claim.legacyBarrier && claim.contextId === null) return false;
    } else if (claim.contextId !== null || claim.capturedDeclaredAccountId !== null || claim.capturedLinkRevision !== null) return false;
    if (claim.executorKind === 'local_graph') {
      if (claim.graphId === null || claim.nodeId !== null) return false;
    } else if (claim.graphId !== null && claim.nodeId === null) return false;
  }
  return capacity.activeClaims.length === 1
    ? capacity.activeAttemptId === capacity.activeClaims[0]!.attemptId
    : capacity.activeAttemptId === null;
}

export function supportsManagedExecution(meta: Meta): boolean {
  return meta.apiVersion === 1 && meta.appVersion === '2.0.0-dev.6' && validMetadataId(meta.ownerInstanceId)
    && meta.scope === 'managed_execution' && meta.managedExecution === 'native_codex_claude'
    && meta.executionContract === '006-v1' && meta.workflowContract === '006-v1' && meta.graphContract === '006-v1'
    && meta.maintenanceContract === 'owner-stop/1'
    && Array.isArray(meta.capabilities) && ['workspace_foundation', 'managed_execution', 'economical_routing', 'verified_delivery', 'task_graphs'].every(capability => meta.capabilities!.includes(capability))
    && !!meta.admission && ['open', 'closing', 'fenced'].includes(meta.admission.state)
    && (meta.admission.incidentId === null || validMetadataId(meta.admission.incidentId))
    && supportsGraphCapacity(meta.capacity);
}

export function supportsOwnerStop(meta: Meta): boolean {
  if (supportsManagedExecution(meta)) return true;
  if (meta.apiVersion !== 1 || !validMetadataId(meta.ownerInstanceId) || meta.scope !== 'managed_execution'
    || !Array.isArray(meta.capabilities) || !['workspace_foundation', 'managed_execution'].every(capability => meta.capabilities!.includes(capability))
    || !meta.admission || !['open', 'closing', 'fenced'].includes(meta.admission.state)
    || meta.admission.incidentId !== null && !validMetadataId(meta.admission.incidentId)
    || !meta.capacity || meta.capacity.limit !== 1 || meta.capacity.activeAttemptId !== null && !validMetadataId(meta.capacity.activeAttemptId)
    || !!meta.graphContract || meta.capacity.occupiedSlots != null || meta.capacity.activeClaims != null) return false;
  if (meta.appVersion === '2.0.0-dev.2') return meta.executionContract === '002-v1' && meta.managedExecution === 'native_codex'
    && !meta.maintenanceContract && meta.capacity.connectionId === 'codex-native' && !meta.capacity.resourceId;
  if (meta.appVersion === '2.0.0-dev.5') return meta.executionContract === '005-v1' && meta.workflowContract === '005-v1'
    && ['economical_routing', 'verified_delivery'].every(capability => meta.capabilities!.includes(capability))
    && meta.managedExecution === 'native_codex_claude' && meta.maintenanceContract === 'owner-stop/1'
    && meta.capacity.resourceId === 'managed-native' && !meta.capacity.connectionId;
  return (meta.appVersion === '2.0.0-dev.3' && meta.executionContract === '003-v1'
    || meta.appVersion === '2.0.0-dev.4' && meta.executionContract === '004-v1' && meta.capabilities.includes('economical_routing'))
    && meta.managedExecution === 'native_codex_claude' && meta.maintenanceContract === 'owner-stop/1'
    && meta.capacity.resourceId === 'managed-native' && !meta.capacity.connectionId;
}

export async function verifyOwner(): Promise<Meta> {
  verified = false;
  const { data } = await api<Meta>('/meta');
  if (!supportsManagedExecution(data)) {
    throw new ApiError('incompatible_owner', 'Open this workspace with the current Council 006 owner. Older owners require their exact maintenance Stop command before reopening.');
  }
  verified = true;
  return data;
}

export function errorMessage(error: unknown, pendingId?: string): string {
  if (error instanceof ApiError) {
    const id = pendingId ?? error.requestId;
    return error.message + (id ? ' Request ' + id + '.' : '');
  }
  return 'The local operation failed. Keep your input and try again.';
}

export function element<T extends HTMLElement = HTMLElement>(id: string): T {
  const found = document.getElementById(id);
  if (!found) throw new Error('Missing workspace control: ' + id);
  return found as T;
}

export function feedback(id: string, message: string, tone: 'neutral' | 'success' | 'error' = 'neutral'): void {
  const target = element(id);
  target.textContent = message;
  target.dataset['tone'] = tone;
}

export function policyName(mode: string): string {
  return mode === 'autonomous' ? 'Autonomous full access' : 'Supervised';
}

export function timeLabel(value: string): string {
  const date = new Date(value);
  return Number.isNaN(date.valueOf()) ? value : date.toLocaleString();
}

export function addFact(list: HTMLElement, name: string, value: string): void {
  const pair = document.createElement('div');
  const term = document.createElement('dt');
  const description = document.createElement('dd');
  term.textContent = name;
  description.textContent = value;
  pair.append(term, description);
  list.append(pair);
}
