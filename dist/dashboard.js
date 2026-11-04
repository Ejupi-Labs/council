import { randomBytes, timingSafeEqual } from 'node:crypto';
import { isUtf8 } from 'node:buffer';
import { createServer } from 'node:http';
import { parseSchema } from './schema.js';
import { dashboardHtml } from './dashboard-ui.js';
import { BillingRevisionConflict } from './billing.js';
import { collaborationSkills } from './skills.js';
import { RunEventsUnavailableError } from './store.js';
import { DelegationRevisionConflict } from './delegation.js';
import { PrivateChatConflict } from './private-chats.js';
const BODY_LIMIT = 256 * 1024;
const SSE_QUEUE_LIMIT = 256 * 1024;
const SSE_CONNECTION_LIMIT = 16;
class HttpError extends Error {
    status;
    constructor(status, message) {
        super(message);
        this.status = status;
    }
}
function readObject(request) {
    const length = request.headers['content-length'];
    if (length !== undefined && (!/^\d+$/.test(length) || Number(length) > BODY_LIMIT))
        return Promise.reject(new HttpError(413, 'The request exceeds the 256 KiB transport limit. No text was truncated.'));
    return new Promise((resolveValue, reject) => {
        const chunks = [];
        let size = 0, settled = false;
        const cleanup = () => {
            clearTimeout(timer);
            request.removeListener('data', data);
            request.removeListener('end', end);
            request.removeListener('error', error);
            request.removeListener('aborted', aborted);
        };
        const fail = (failure) => {
            if (settled)
                return;
            settled = true;
            cleanup();
            request.resume();
            reject(failure);
        };
        const data = (chunk) => {
            size += chunk.length;
            if (size > BODY_LIMIT) {
                fail(new HttpError(413, 'The request exceeds the 256 KiB transport limit. No text was truncated.'));
                return;
            }
            chunks.push(chunk);
        };
        const end = () => {
            if (settled)
                return;
            settled = true;
            cleanup();
            try {
                if (size === 0) {
                    resolveValue({});
                    return;
                }
                if (!/^application\/json(?:\s*;|$)/i.test(request.headers['content-type'] ?? ''))
                    throw new HttpError(415, 'Send an application/json object.');
                const buffer = Buffer.concat(chunks, size);
                if (!isUtf8(buffer))
                    throw new HttpError(400, 'JSON must contain valid UTF-8.');
                const value = JSON.parse(buffer.toString('utf8'));
                resolveValue(parseSchema({ type: 'object', additionalProperties: true }, value));
            }
            catch (failure) {
                reject(failure instanceof HttpError ? failure : new HttpError(400, failure instanceof SyntaxError ? 'The request does not contain valid JSON.' : failure instanceof Error ? failure.message : 'Invalid request.'));
            }
        };
        const error = () => fail(new HttpError(400, 'Request reading was interrupted.'));
        const aborted = () => fail(new HttpError(400, 'The request was interrupted.'));
        const timer = setTimeout(() => fail(new HttpError(408, 'The request read deadline expired.')), 10_000);
        request.on('data', data);
        request.once('end', end);
        request.once('error', error);
        request.once('aborted', aborted);
    });
}
/** An authenticated loopback interface. Closing it never closes the owning service or rooms. */
export async function serveDashboard(service, rooms, options = {}) {
    const port = options.port ?? 0;
    if (!Number.isInteger(port) || port < 0 || port > 65535)
        throw new Error('Dashboard port must be an integer from 0 to 65535.');
    const token = randomBytes(32).toString('base64url');
    const tokenBytes = Buffer.from(`Bearer ${token}`);
    const clients = new Set();
    let host = '', origin = '', closed = false, polling = false;
    let closePromise;
    let snapshot = JSON.stringify(await rooms.list());
    let delegateSnapshot = '';
    const json = (response, status, value) => {
        response.writeHead(status, { 'Content-Type': 'application/json; charset=utf-8' });
        response.end(JSON.stringify(value));
    };
    const writeEvent = (response, frame) => {
        if (response.destroyed || response.writableEnded) {
            clients.delete(response);
            return;
        }
        if (response.writableLength + Buffer.byteLength(frame) > SSE_QUEUE_LIMIT) {
            clients.delete(response);
            response.destroy();
            return;
        }
        response.write(frame);
    };
    const server = createServer((request, response) => {
        response.setHeader('Cache-Control', 'no-store');
        response.setHeader('Referrer-Policy', 'no-referrer');
        response.setHeader('X-Content-Type-Options', 'nosniff');
        response.setHeader('Cross-Origin-Resource-Policy', 'same-origin');
        response.setHeader('Content-Security-Policy', "default-src 'none'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'");
        void (async () => {
            if (closed)
                throw new HttpError(503, 'The dashboard is shutting down.');
            if (request.headers.host !== host)
                throw new HttpError(403, 'Invalid loopback Host header.');
            if (request.headers.origin !== undefined && request.headers.origin !== origin)
                throw new HttpError(403, 'Origin is not allowed.');
            const url = new URL(request.url ?? '/', origin);
            if (url.origin !== origin)
                throw new HttpError(403, 'Invalid loopback destination.');
            const pathname = url.pathname;
            if (pathname === '/') {
                if (request.method !== 'GET')
                    throw new HttpError(405, 'Method not allowed.');
                const nonce = randomBytes(18).toString('base64');
                response.setHeader('Content-Security-Policy', `default-src 'none'; script-src 'nonce-${nonce}'; style-src 'nonce-${nonce}'; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'`);
                response.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8' });
                response.end(dashboardHtml(nonce));
                return;
            }
            if (pathname !== '/api' && !pathname.startsWith('/api/'))
                throw new HttpError(404, 'Page not found.');
            const authorization = Buffer.from(request.headers.authorization ?? '');
            if (authorization.length !== tokenBytes.length || !timingSafeEqual(authorization, tokenBytes))
                throw new HttpError(401, 'Open the complete dashboard link to authorize this session.');
            if (request.method !== 'GET' && request.method !== 'POST')
                throw new HttpError(405, 'Method not allowed.');
            if (pathname === '/api/private-chats/actors' || pathname === '/api/private-chats') {
                if (pathname.endsWith('/actors') && request.method !== 'GET')
                    throw new HttpError(405, 'Method not allowed.');
                if (request.method === 'POST')
                    json(response, 201, service.privateChats.create(await readObject(request)));
                else {
                    const actors = pathname.endsWith('/actors');
                    for (const key of url.searchParams.keys())
                        if (!(actors ? ['roomId'] : ['actor', 'roomId']).includes(key) || url.searchParams.getAll(key).length !== 1)
                            throw new HttpError(400, 'Use an exact actor and optional room ID.');
                    json(response, 200, actors ? service.privateChats.actors(url.searchParams.get('roomId') ?? undefined)
                        : service.privateChats.list(url.searchParams.get('actor') ?? '', url.searchParams.get('roomId') ?? undefined));
                }
                return;
            }
            if (pathname === '/api/private-chats/watch') {
                if (request.method !== 'POST')
                    throw new HttpError(405, 'Method not allowed.');
                const input = await readObject(request), controller = new AbortController();
                const abort = () => controller.abort(new DOMException('Private chat watch disconnected.', 'AbortError'));
                response.once('close', abort);
                request.once('aborted', abort);
                try {
                    json(response, 200, await service.privateChats.watch(input, controller.signal));
                }
                finally {
                    response.removeListener('close', abort);
                    request.removeListener('aborted', abort);
                }
                return;
            }
            const chatMatch = /^\/api\/private-chats\/([a-zA-Z0-9_-]{1,100})\/messages$/.exec(pathname);
            if (chatMatch) {
                if (request.method === 'POST')
                    json(response, 201, service.privateChats.send({ ...await readObject(request), chatId: chatMatch[1] }));
                else {
                    for (const key of url.searchParams.keys())
                        if (!['actor', 'after', 'limit'].includes(key) || url.searchParams.getAll(key).length !== 1)
                            throw new HttpError(400, 'Use one actor, after cursor and page limit.');
                    const bound = (key, fallback) => {
                        const value = url.searchParams.get(key);
                        if (value === null)
                            return fallback;
                        if (!/^(?:0|[1-9]\d*)$/.test(value) || !Number.isSafeInteger(Number(value)))
                            throw new HttpError(400, 'Chat page bounds must be non-negative safe integers.');
                        return Number(value);
                    };
                    json(response, 200, service.privateChats.page({ chatId: chatMatch[1], actor: url.searchParams.get('actor') ?? '', after: bound('after', 0), limit: bound('limit', 100) }));
                }
                return;
            }
            if (pathname === '/api/delegation') {
                if (request.method === 'GET')
                    json(response, 200, service.delegation.get());
                else
                    json(response, 200, service.delegation.configure(await readObject(request)));
                return;
            }
            if (pathname === '/api/delegation/reports') {
                if (request.method === 'POST')
                    json(response, 201, service.delegation.report(await readObject(request)));
                else {
                    for (const key of url.searchParams.keys())
                        if (!['limit', 'cursor', 'openOnly'].includes(key) || url.searchParams.getAll(key).length !== 1)
                            throw new HttpError(400, 'Use one report page limit, cursor and openOnly filter.');
                    const bound = (key, fallback) => {
                        const value = url.searchParams.get(key);
                        if (value === null)
                            return fallback;
                        if (!/^(?:0|[1-9]\d*)$/.test(value) || !Number.isSafeInteger(Number(value)))
                            throw new HttpError(400, 'Report page bounds must be non-negative safe integers.');
                        return Number(value);
                    };
                    const openOnly = url.searchParams.get('openOnly');
                    if (openOnly !== null && openOnly !== 'true' && openOnly !== 'false')
                        throw new HttpError(400, 'openOnly must be true or false.');
                    json(response, 200, service.delegation.reportsPage({ limit: bound('limit', 20), cursor: bound('cursor', 0), openOnly: openOnly === 'true' }));
                }
                return;
            }
            if (pathname === '/api/delegation/changes') {
                if (request.method !== 'GET')
                    throw new HttpError(405, 'Method not allowed.');
                for (const key of url.searchParams.keys())
                    if (!['limit', 'after'].includes(key) || url.searchParams.getAll(key).length !== 1)
                        throw new HttpError(400, 'Use one change page limit and after revision.');
                const bound = (key, fallback) => {
                    const value = url.searchParams.get(key);
                    if (value === null)
                        return fallback;
                    if (!/^(?:0|[1-9]\d*)$/.test(value) || !Number.isSafeInteger(Number(value)))
                        throw new HttpError(400, 'Change page bounds must be non-negative safe integers.');
                    return Number(value);
                };
                json(response, 200, service.delegation.reportChangesPage({ after: bound('after', 0), limit: bound('limit', 20) }));
                return;
            }
            const delegateReply = /^\/api\/delegation\/reports\/([a-zA-Z0-9_-]{1,100})\/resolve$/.exec(pathname);
            if (delegateReply) {
                if (request.method !== 'POST')
                    throw new HttpError(405, 'Method not allowed.');
                json(response, 200, service.delegation.resolve(delegateReply[1], await readObject(request)));
                return;
            }
            if (pathname === '/api/skills' && request.method === 'GET') {
                json(response, 200, { skills: collaborationSkills() });
                return;
            }
            if (pathname === '/api/billing' && request.method === 'GET') {
                json(response, 200, service.billingView());
                return;
            }
            const billingMatch = /^\/api\/billing\/([a-z][a-z0-9-]{0,63})$/.exec(pathname);
            if (billingMatch) {
                if (request.method !== 'POST')
                    throw new HttpError(405, 'Method not allowed.');
                const { expectedRevision, ...settings } = await readObject(request);
                if (!Number.isSafeInteger(expectedRevision) || expectedRevision < 0)
                    throw new HttpError(400, 'Supply the billing revision shown by the current agent view.');
                const saved = service.billing.update(billingMatch[1], settings, expectedRevision);
                const now = Date.now();
                json(response, 200, { ...saved, effectiveSkills: saved.skills ?? service.config.agents[saved.alias]?.skills ?? [], availability: service.billing.availabilityFor(saved, now), subscriptionProtection: service.billing.subscriptionProtectionFor(saved, now) });
                return;
            }
            if (pathname === '/api/routing/plan') {
                if (request.method !== 'POST')
                    throw new HttpError(405, 'Method not allowed.');
                json(response, 200, service.routePlan(await readObject(request)));
                return;
            }
            const runMatch = /^\/api\/runs\/([a-zA-Z0-9_-]{1,256})(?:\/(events))?$/.exec(pathname);
            if (runMatch) {
                if (request.method !== 'GET')
                    throw new HttpError(405, 'Method not allowed.');
                if (runMatch[2] === 'events') {
                    for (const key of url.searchParams.keys())
                        if (!['after', 'limit', 'type'].includes(key) || url.searchParams.getAll(key).length !== 1)
                            throw new HttpError(400, 'Use one after cursor, page limit and optional exact event type.');
                    const bound = (key, fallback) => {
                        const value = url.searchParams.get(key);
                        if (value === null)
                            return fallback;
                        if (!/^(?:0|[1-9]\d*)$/.test(value) || !Number.isSafeInteger(Number(value)))
                            throw new HttpError(400, 'Event page bounds must be non-negative safe integers in decimal notation.');
                        return Number(value);
                    };
                    json(response, 200, service.eventPage(runMatch[1], bound('after', 0), bound('limit', 100), url.searchParams.has('type') ? url.searchParams.get('type') : undefined));
                    return;
                }
                const run = service.getRun(runMatch[1]);
                if (!run)
                    throw new HttpError(404, 'Workflow not found. Its stored result may have been pruned.');
                json(response, 200, run);
                return;
            }
            if (pathname === '/api/bootstrap' && request.method === 'GET') {
                const agents = Object.entries(service.config.agents).filter(([, settings]) => settings.enabled).map(([agent, settings]) => ({ agent, profile: settings.profile, model: settings.model ?? null, effort: settings.effort ?? null, authority: settings.authority ?? 1 }));
                json(response, 200, { backend: service.config.backend, agents, rooms: await rooms.list(), defaults: { maxCalls: service.config.maxCalls, contextLimit: null }, limits: { requestBytes: BODY_LIMIT } });
                return;
            }
            if (pathname === '/api/events' && request.method === 'GET') {
                if (clients.size >= SSE_CONNECTION_LIMIT)
                    throw new HttpError(429, 'There are already 16 open update connections.');
                response.writeHead(200, { 'Content-Type': 'text/event-stream; charset=utf-8', Connection: 'keep-alive' });
                response.flushHeaders();
                clients.add(response);
                response.once('close', () => clients.delete(response));
                writeEvent(response, ': connected\n\n');
                if ('delegation' in service)
                    writeEvent(response, `event: delegation\ndata: ${JSON.stringify(service.delegation.overview())}\n\n`);
                return;
            }
            if (pathname === '/api/rooms') {
                if (request.method === 'GET')
                    json(response, 200, await rooms.list());
                else
                    json(response, 201, await rooms.create(await readObject(request)));
                return;
            }
            const match = /^\/api\/rooms\/([a-zA-Z0-9_-]{1,100})(?:\/(messages|tasks|discuss|execute|pause|resume|cancel|history))?$/.exec(pathname);
            if (!match)
                throw new HttpError(404, 'API endpoint not found.');
            const id = match[1], action = match[2];
            let current;
            try {
                current = await rooms.get(id);
            }
            catch (failure) {
                if (failure instanceof Error && /not found/i.test(failure.message))
                    throw new HttpError(404, 'Room not found.');
                throw failure;
            }
            if (!current)
                throw new HttpError(404, 'Room not found.');
            if (!action && request.method === 'GET') {
                json(response, 200, current);
                return;
            }
            if (action === 'history') {
                if (request.method !== 'GET')
                    throw new HttpError(405, 'Method not allowed.');
                const limit = Number(url.searchParams.get('limit') ?? 100);
                const cursor = url.searchParams.has('cursor') ? Number(url.searchParams.get('cursor')) : undefined;
                if (!Number.isInteger(limit) || limit < 1 || limit > 100 || (cursor !== undefined && (!Number.isSafeInteger(cursor) || cursor < 0)))
                    throw new HttpError(400, 'Invalid history page bounds.');
                json(response, 200, await rooms.messagePage(id, limit, cursor));
                return;
            }
            if (!action || request.method !== 'POST')
                throw new HttpError(405, 'Method not allowed.');
            const body = await readObject(request);
            let result;
            if (action === 'messages')
                result = await rooms.postMessage(id, body);
            else if (action === 'tasks')
                result = await rooms.addTask(id, body);
            else if (action === 'discuss')
                result = await rooms.startDiscussion(id);
            else if (action === 'execute')
                result = await rooms.execute(id);
            else if (action === 'pause')
                result = await rooms.pause(id);
            else if (action === 'resume')
                result = await rooms.resume(id);
            else
                result = await rooms.cancel(id);
            json(response, 200, result);
        })().catch(failure => {
            if (response.destroyed || response.writableEnded)
                return;
            if (response.headersSent) {
                response.destroy();
                return;
            }
            if (failure instanceof HttpError && [408, 413].includes(failure.status))
                response.setHeader('Connection', 'close');
            json(response, failure instanceof HttpError ? failure.status : failure instanceof RunEventsUnavailableError ? 404 : failure instanceof BillingRevisionConflict || failure instanceof DelegationRevisionConflict || failure instanceof PrivateChatConflict ? 409 : 400, { error: failure instanceof Error ? failure.message : 'The operation failed.',
                ...(failure && typeof failure === 'object' && 'code' in failure ? { code: failure.code } : {}) });
        });
    });
    server.requestTimeout = 15_000;
    server.headersTimeout = 10_000;
    server.keepAliveTimeout = 5000;
    server.maxHeadersCount = 64;
    server.maxRequestsPerSocket = 1000;
    await new Promise((resolveReady, reject) => {
        server.once('error', reject);
        server.listen(port, '127.0.0.1', () => { server.removeListener('error', reject); resolveReady(); });
    });
    const address = server.address();
    if (!address || typeof address === 'string') {
        server.close();
        throw new Error('Could not determine the local dashboard address.');
    }
    host = `127.0.0.1:${address.port}`;
    origin = `http://${host}`;
    const poll = setInterval(() => {
        if (closed || polling || clients.size === 0)
            return;
        polling = true;
        void Promise.resolve().then(() => rooms.list()).then(value => {
            const next = JSON.stringify(value);
            if (closed)
                return;
            if (next !== snapshot) {
                snapshot = next;
                const frame = `event: rooms\ndata: ${next}\n\n`;
                for (const client of clients)
                    writeEvent(client, frame);
            }
            if ('delegation' in service) {
                const nextDelegate = JSON.stringify(service.delegation.overview());
                if (nextDelegate !== delegateSnapshot) {
                    delegateSnapshot = nextDelegate;
                    for (const client of clients)
                        writeEvent(client, `event: delegation\ndata: ${nextDelegate}\n\n`);
                }
            }
        }).catch(() => {
            for (const client of clients) {
                client.end();
                clients.delete(client);
            }
        }).finally(() => { polling = false; });
    }, 750);
    const heartbeat = setInterval(() => { for (const client of clients)
        writeEvent(client, ': keep-alive\n\n'); }, 15_000);
    poll.unref();
    heartbeat.unref();
    return {
        url: `${origin}/#token=${token}`,
        close() {
            if (closePromise)
                return closePromise;
            closed = true;
            clearInterval(poll);
            clearInterval(heartbeat);
            for (const client of clients)
                client.destroy();
            clients.clear();
            closePromise = new Promise((resolveClosed, reject) => {
                server.close(error => { if (error)
                    reject(error);
                else
                    resolveClosed(); });
                server.closeAllConnections();
            });
            return closePromise;
        },
    };
}
//# sourceMappingURL=dashboard.js.map