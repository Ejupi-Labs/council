import { randomBytes, timingSafeEqual } from 'node:crypto';
import { isUtf8 } from 'node:buffer';
import { createServer, type IncomingMessage, type ServerResponse } from 'node:http';
import type { CouncilService } from './service.js';
import type { RoomManager } from './rooms.js';
import type { RoomInput, RoomTaskInput } from './rooms-types.js';
import { parseSchema } from './schema.js';
import { dashboardHtml } from './dashboard-ui.js';
import { BillingRevisionConflict } from './billing.js';
import { collaborationSkills } from './skills.js';

const BODY_LIMIT = 256 * 1024;
const SSE_QUEUE_LIMIT = 256 * 1024;
const SSE_CONNECTION_LIMIT = 16;

class HttpError extends Error {
  constructor(readonly status: number, message: string) { super(message); }
}

function readObject(request: IncomingMessage): Promise<Record<string, unknown>> {
  const length = request.headers['content-length'];
  if (length !== undefined && (!/^\d+$/.test(length) || Number(length) > BODY_LIMIT)) return Promise.reject(new HttpError(413, 'The request exceeds the 256 KiB transport limit. No text was truncated.'));
  return new Promise((resolveValue, reject) => {
    const chunks: Buffer[] = [];
    let size = 0, settled = false;
    const cleanup = (): void => {
      clearTimeout(timer);
      request.removeListener('data', data);
      request.removeListener('end', end);
      request.removeListener('error', error);
      request.removeListener('aborted', aborted);
    };
    const fail = (failure: Error): void => {
      if (settled) return;
      settled = true; cleanup(); request.resume(); reject(failure);
    };
    const data = (chunk: Buffer): void => {
      size += chunk.length;
      if (size > BODY_LIMIT) { fail(new HttpError(413, 'The request exceeds the 256 KiB transport limit. No text was truncated.')); return; }
      chunks.push(chunk);
    };
    const end = (): void => {
      if (settled) return;
      settled = true; cleanup();
      try {
        if (size === 0) { resolveValue({}); return; }
        if (!/^application\/json(?:\s*;|$)/i.test(request.headers['content-type'] ?? '')) throw new HttpError(415, 'Send an application/json object.');
        const buffer = Buffer.concat(chunks, size);
        if (!isUtf8(buffer)) throw new HttpError(400, 'JSON must contain valid UTF-8.');
        const value: unknown = JSON.parse(buffer.toString('utf8'));
        resolveValue(parseSchema({ type: 'object', additionalProperties: true }, value));
      } catch (failure) { reject(failure instanceof HttpError ? failure : new HttpError(400, failure instanceof SyntaxError ? 'The request does not contain valid JSON.' : failure instanceof Error ? failure.message : 'Invalid request.')); }
    };
    const error = (): void => fail(new HttpError(400, 'Request reading was interrupted.'));
    const aborted = (): void => fail(new HttpError(400, 'The request was interrupted.'));
    const timer = setTimeout(() => fail(new HttpError(408, 'The request read deadline expired.')), 10_000);
    request.on('data', data); request.once('end', end); request.once('error', error); request.once('aborted', aborted);
  });
}

/** An authenticated loopback interface. Closing it never closes the owning service or rooms. */
export async function serveDashboard(service: CouncilService, rooms: RoomManager, options: { port?: number } = {}): Promise<{ url: string; close(): Promise<void> }> {
  const port = options.port ?? 0;
  if (!Number.isInteger(port) || port < 0 || port > 65535) throw new Error('Dashboard port must be an integer from 0 to 65535.');
  const token = randomBytes(32).toString('base64url');
  const tokenBytes = Buffer.from(`Bearer ${token}`);
  const clients = new Set<ServerResponse>();
  let host = '', origin = '', closed = false, polling = false;
  let closePromise: Promise<void> | undefined;
  let snapshot = JSON.stringify(await rooms.list());
  const json = (response: ServerResponse, status: number, value: unknown): void => {
    response.writeHead(status, { 'Content-Type': 'application/json; charset=utf-8' });
    response.end(JSON.stringify(value));
  };
  const writeEvent = (response: ServerResponse, frame: string): void => {
    if (response.destroyed || response.writableEnded) { clients.delete(response); return; }
    if (response.writableLength + Buffer.byteLength(frame) > SSE_QUEUE_LIMIT) { clients.delete(response); response.destroy(); return; }
    response.write(frame);
  };
  const server = createServer((request, response) => {
    response.setHeader('Cache-Control', 'no-store');
    response.setHeader('Referrer-Policy', 'no-referrer');
    response.setHeader('X-Content-Type-Options', 'nosniff');
    response.setHeader('Cross-Origin-Resource-Policy', 'same-origin');
    response.setHeader('Content-Security-Policy', "default-src 'none'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'");
    void (async () => {
      if (closed) throw new HttpError(503, 'The dashboard is shutting down.');
      if (request.headers.host !== host) throw new HttpError(403, 'Invalid loopback Host header.');
      if (request.headers.origin !== undefined && request.headers.origin !== origin) throw new HttpError(403, 'Origin is not allowed.');
      const url = new URL(request.url ?? '/', origin);
      if (url.origin !== origin) throw new HttpError(403, 'Invalid loopback destination.');
      const pathname = url.pathname;
      if (pathname === '/') {
        if (request.method !== 'GET') throw new HttpError(405, 'Method not allowed.');
        const nonce = randomBytes(18).toString('base64');
        response.setHeader('Content-Security-Policy', `default-src 'none'; script-src 'nonce-${nonce}'; style-src 'nonce-${nonce}'; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'`);
        response.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8' });
        response.end(dashboardHtml(nonce)); return;
      }
      if (pathname !== '/api' && !pathname.startsWith('/api/')) throw new HttpError(404, 'Page not found.');
      const authorization = Buffer.from(request.headers.authorization ?? '');
      if (authorization.length !== tokenBytes.length || !timingSafeEqual(authorization, tokenBytes)) throw new HttpError(401, 'Open the complete dashboard link to authorize this session.');
      if (request.method !== 'GET' && request.method !== 'POST') throw new HttpError(405, 'Method not allowed.');
      if (pathname === '/api/skills' && request.method === 'GET') { json(response, 200, { skills: collaborationSkills() }); return; }
      if (pathname === '/api/billing' && request.method === 'GET') { json(response, 200, service.billingView()); return; }
      const billingMatch = /^\/api\/billing\/([a-z][a-z0-9-]{0,63})$/.exec(pathname);
      if (billingMatch) {
        if (request.method !== 'POST') throw new HttpError(405, 'Method not allowed.');
        const { expectedRevision, ...settings } = await readObject(request);
        if (!Number.isSafeInteger(expectedRevision) || (expectedRevision as number) < 0) throw new HttpError(400, 'Supply the billing revision shown by the current agent view.');
        const saved = service.billing.update(billingMatch[1]!, settings, expectedRevision as number);
        json(response, 200, { ...saved, effectiveSkills: saved.skills ?? service.config.agents[saved.alias]?.skills ?? [], availability: service.billing.availability(saved.alias) }); return;
      }
      if (pathname === '/api/routing/plan') {
        if (request.method !== 'POST') throw new HttpError(405, 'Method not allowed.');
        json(response, 200, service.routePlan(await readObject(request) as any)); return;
      }
      if (pathname === '/api/bootstrap' && request.method === 'GET') {
        const agents = Object.entries(service.config.agents).filter(([, settings]) => settings.enabled).map(([agent, settings]) => ({ agent, profile: settings.profile, model: settings.model ?? null, effort: settings.effort ?? null, authority: settings.authority ?? 1 }));
        json(response, 200, { backend: service.config.backend, agents, rooms: await rooms.list(), defaults: { maxCalls: service.config.maxCalls, contextLimit: null }, limits: { requestBytes: BODY_LIMIT } }); return;
      }
      if (pathname === '/api/events' && request.method === 'GET') {
        if (clients.size >= SSE_CONNECTION_LIMIT) throw new HttpError(429, 'There are already 16 open update connections.');
        response.writeHead(200, { 'Content-Type': 'text/event-stream; charset=utf-8', Connection: 'keep-alive' });
        response.flushHeaders(); clients.add(response);
        response.once('close', () => clients.delete(response));
        writeEvent(response, ': connected\n\n'); return;
      }
      if (pathname === '/api/rooms') {
        if (request.method === 'GET') json(response, 200, await rooms.list());
        else json(response, 201, await rooms.create(await readObject(request) as unknown as RoomInput));
        return;
      }
      const match = /^\/api\/rooms\/([a-zA-Z0-9_-]{1,100})(?:\/(messages|tasks|discuss|execute|cancel|history))?$/.exec(pathname);
      if (!match) throw new HttpError(404, 'API endpoint not found.');
      const id = match[1]!, action = match[2];
      let current;
      try { current = await rooms.get(id); }
      catch (failure) { if (failure instanceof Error && /not found/i.test(failure.message)) throw new HttpError(404, 'Room not found.'); throw failure; }
      if (!current) throw new HttpError(404, 'Room not found.');
      if (!action && request.method === 'GET') { json(response, 200, current); return; }
      if (action === 'history') {
        if (request.method !== 'GET') throw new HttpError(405, 'Method not allowed.');
        const limit = Number(url.searchParams.get('limit') ?? 100);
        const cursor = url.searchParams.has('cursor') ? Number(url.searchParams.get('cursor')) : undefined;
        if (!Number.isInteger(limit) || limit < 1 || limit > 100 || (cursor !== undefined && (!Number.isSafeInteger(cursor) || cursor < 0))) throw new HttpError(400, 'Invalid history page bounds.');
        json(response, 200, await rooms.messagePage(id, limit, cursor)); return;
      }
      if (!action || request.method !== 'POST') throw new HttpError(405, 'Method not allowed.');
      const body = await readObject(request);
      let result: unknown;
      if (action === 'messages') result = await rooms.postMessage(id, body as unknown as { author: string; body: string });
      else if (action === 'tasks') result = await rooms.addTask(id, body as unknown as RoomTaskInput);
      else if (action === 'discuss') result = await rooms.startDiscussion(id);
      else if (action === 'execute') result = await rooms.execute(id);
      else result = await rooms.cancel(id);
      json(response, 200, result);
    })().catch(failure => {
      if (response.destroyed || response.writableEnded) return;
      if (response.headersSent) { response.destroy(); return; }
      if (failure instanceof HttpError && [408, 413].includes(failure.status)) response.setHeader('Connection', 'close');
      json(response, failure instanceof HttpError ? failure.status : failure instanceof BillingRevisionConflict ? 409 : 400,
        { error: failure instanceof Error ? failure.message : 'The operation failed.',
          ...(failure && typeof failure === 'object' && 'code' in failure ? { code: failure.code } : {}) });
    });
  });
  server.requestTimeout = 15_000; server.headersTimeout = 10_000; server.keepAliveTimeout = 5000;
  server.maxHeadersCount = 64; server.maxRequestsPerSocket = 1000;
  await new Promise<void>((resolveReady, reject) => {
    server.once('error', reject);
    server.listen(port, '127.0.0.1', () => { server.removeListener('error', reject); resolveReady(); });
  });
  const address = server.address();
  if (!address || typeof address === 'string') { server.close(); throw new Error('Could not determine the local dashboard address.'); }
  host = `127.0.0.1:${address.port}`; origin = `http://${host}`;
  const poll = setInterval(() => {
    if (closed || polling || clients.size === 0) return;
    polling = true;
    void Promise.resolve().then(() => rooms.list()).then(value => {
      const next = JSON.stringify(value);
      if (closed || next === snapshot) return;
      snapshot = next;
      const frame = `event: rooms\ndata: ${next}\n\n`;
      for (const client of clients) writeEvent(client, frame);
    }).catch(() => {
      for (const client of clients) { client.end(); clients.delete(client); }
    }).finally(() => { polling = false; });
  }, 750);
  const heartbeat = setInterval(() => { for (const client of clients) writeEvent(client, ': keep-alive\n\n'); }, 15_000);
  poll.unref(); heartbeat.unref();
  return {
    url: `${origin}/#token=${token}`,
    close(): Promise<void> {
      if (closePromise) return closePromise;
      closed = true; clearInterval(poll); clearInterval(heartbeat);
      for (const client of clients) client.destroy();
      clients.clear();
      closePromise = new Promise<void>((resolveClosed, reject) => {
        server.close(error => { if (error) reject(error); else resolveClosed(); });
        server.closeAllConnections();
      });
      return closePromise;
    },
  };
}
