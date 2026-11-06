export const ACCESS_KEY = 'council.accessToken';
const CACHE_PREFIX = 'council.foundation.';
let accessToken = '';
let verified = false;
export function readSession(key) {
    try {
        const value = sessionStorage.getItem(CACHE_PREFIX + key);
        return value ? JSON.parse(value) : null;
    }
    catch {
        return null;
    }
}
export function writeSession(key, value) {
    try {
        if (value === null)
            sessionStorage.removeItem(CACHE_PREFIX + key);
        else
            sessionStorage.setItem(CACHE_PREFIX + key, JSON.stringify(value));
    }
    catch { /* A denied/full cache never changes a durable save acknowledgement. */ }
}
export function consumeAccessToken() {
    const fragment = location.hash.slice(1);
    if (fragment) {
        history.replaceState(null, '', location.pathname + location.search);
        try {
            accessToken = fragment.startsWith('token=')
                ? new URLSearchParams(fragment).get('token') ?? ''
                : decodeURIComponent(fragment);
        }
        catch {
            accessToken = '';
        }
        try {
            if (accessToken)
                sessionStorage.setItem(ACCESS_KEY, accessToken);
            else
                sessionStorage.removeItem(ACCESS_KEY);
        }
        catch { /* Access remains usable in this page when storage is unavailable. */ }
    }
    else {
        try {
            accessToken = sessionStorage.getItem(ACCESS_KEY) ?? '';
        }
        catch {
            accessToken = '';
        }
    }
    return accessToken.length > 0;
}
export function requestId() {
    const bytes = crypto.getRandomValues(new Uint8Array(16));
    return Array.from(bytes, value => value.toString(16).padStart(2, '0')).join('');
}
export class ApiError extends Error {
    code;
    requestId;
    currentRevision;
    uncertain;
    constructor(code, message, requestId = '', currentRevision, uncertain = false) {
        super(message);
        this.code = code;
        this.requestId = requestId;
        this.currentRevision = currentRevision;
        this.uncertain = uncertain;
    }
}
export async function api(path, method = 'GET', body) {
    if (method !== 'GET' && !verified) {
        throw new ApiError('owner_unverified', 'Reconnect to this local owner before saving.');
    }
    let response;
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
    }
    catch {
        throw new ApiError('connection_failed', 'The local owner could not be reached. Reconnect, then retry this same request.', '', undefined, method !== 'GET');
    }
    let envelope;
    try {
        envelope = await response.json();
    }
    catch {
        throw new ApiError('invalid_response', 'The local owner returned an unreadable response. Reconnect and retry this same request.', '', undefined, method !== 'GET');
    }
    if (envelope.apiVersion !== 1 || typeof envelope.requestId !== 'string') {
        throw new ApiError('incompatible_owner', 'This owner uses an incompatible API. Open Council again.', '', undefined, method !== 'GET');
    }
    if (!response.ok || envelope.error) {
        const error = envelope.error;
        const code = error?.code ?? 'invalid_response';
        if (response.status === 401 || response.status === 403)
            verified = false;
        throw new ApiError(code, error?.message ?? 'The local operation failed. Reconnect and retry.', envelope.requestId, error?.currentRevision, method !== 'GET' && (response.status >= 500 || code === 'write_uncertain'));
    }
    if (method !== 'GET' && (!envelope.receipt || envelope.receipt.requestId !== body?.requestId)) {
        throw new ApiError('missing_receipt', 'No saved acknowledgement was received. Retry this same request to reconcile its outcome.', envelope.requestId, undefined, true);
    }
    return envelope;
}
export function supportsManagedExecution(meta) {
    return meta.apiVersion === 1 && meta.appVersion === '2.0.0-dev.4' && /^[a-f0-9]{32}$/.test(meta.ownerInstanceId)
        && meta.scope === 'managed_execution' && meta.managedExecution === 'native_codex_claude'
        && meta.executionContract === '004-v1' && meta.maintenanceContract === 'owner-stop/1'
        && Array.isArray(meta.capabilities) && meta.capabilities.includes('workspace_foundation') && meta.capabilities.includes('managed_execution') && meta.capabilities.includes('economical_routing')
        && !!meta.admission && ['open', 'closing', 'fenced'].includes(meta.admission.state)
        && meta.capacity?.resourceId === 'managed-native' && meta.capacity.limit === 1
        && (meta.capacity.activeAttemptId === null || /^[a-f0-9]{32}$/.test(meta.capacity.activeAttemptId));
}
export async function verifyOwner() {
    verified = false;
    const { data } = await api('/meta');
    const foundation = data.scope === 'workspace_foundation' && data.managedExecution === 'not_implemented';
    const execution = supportsManagedExecution(data);
    if (data.apiVersion !== 1 || (!foundation && !execution)) {
        throw new ApiError('incompatible_owner', 'Open this workspace with a compatible Council owner.');
    }
    verified = true;
    return data;
}
export function errorMessage(error, pendingId) {
    if (error instanceof ApiError) {
        const id = pendingId ?? error.requestId;
        return error.message + (id ? ' Request ' + id + '.' : '');
    }
    return 'The local operation failed. Keep your input and try again.';
}
export function element(id) {
    const found = document.getElementById(id);
    if (!found)
        throw new Error('Missing workspace control: ' + id);
    return found;
}
export function feedback(id, message, tone = 'neutral') {
    const target = element(id);
    target.textContent = message;
    target.dataset['tone'] = tone;
}
export function policyName(mode) {
    return mode === 'autonomous' ? 'Autonomous full access' : 'Supervised';
}
export function timeLabel(value) {
    const date = new Date(value);
    return Number.isNaN(date.valueOf()) ? value : date.toLocaleString();
}
export function addFact(list, name, value) {
    const pair = document.createElement('div');
    const term = document.createElement('dt');
    const description = document.createElement('dd');
    term.textContent = name;
    description.textContent = value;
    pair.append(term, description);
    list.append(pair);
}
