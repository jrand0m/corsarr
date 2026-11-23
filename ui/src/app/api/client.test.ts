import { describe, expect, it, vi } from 'vitest';
import { ApiError, buildUrl, Client, toApiError } from './client';

const fail = (p: Promise<unknown>): Promise<ApiError> => p.then(() => { throw new Error('expected a failure'); }, (e: unknown) => e as ApiError);

function json(status: number, body: unknown): Response {
  return new Response(body === undefined ? null : JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
}

describe('buildUrl', () => {
  it('joins base and path and drops empty query values', () => {
    expect(buildUrl('api/v1/', '/movies', { cursor: undefined, limit: 50, q: '' })).toBe('api/v1/movies?limit=50');
    expect(buildUrl('api/v1/', 'movies/lookup', { term: 'a b' })).toBe('api/v1/movies/lookup?term=a+b');
  });
});

describe('Client', () => {
  it('sends JSON and parses the answer', async () => {
    const fetch = vi.fn(async () => json(201, { id: 7 }));
    const api = new Client({ fetch });
    expect(await api.post<{ id: number }>('movies', { tmdbId: 1 })).toEqual({ id: 7 });
    const [url, init] = fetch.mock.calls[0] as unknown as [string, RequestInit];
    expect(url).toBe('api/v1/movies');
    expect(init.method).toBe('POST');
    expect(init.body).toBe('{"tmdbId":1}');
    expect((init.headers as Record<string, string>)['Content-Type']).toBe('application/json');
  });

  it('returns undefined for 204', async () => {
    const api = new Client({ fetch: async () => new Response(null, { status: 204 }) });
    expect(await api.del('blocklist/3')).toBeUndefined();
  });

  it('turns the error shape into an ApiError with code, field and body', async () => {
    const api = new Client({ fetch: async () => json(409, { error: { code: 'confirm_required', message: 'confirm to grab anyway', field: 'releaseId' }, decision: { override: 'confirm' } }) });
    const err = await fail(api.post('grabs', {}));
    expect(err).toBeInstanceOf(ApiError);
    expect(err.status).toBe(409);
    expect(err.code).toBe('confirm_required');
    expect(err.field).toBe('releaseId');
    expect(err.message).toBe('confirm to grab anyway');
    expect(err.body.decision).toEqual({ override: 'confirm' });
    expect(err.label).toBe('confirm_required · 409');
  });

  it('falls back to the status when the body is not the error shape', async () => {
    const api = new Client({ fetch: async () => new Response('oops', { status: 502, statusText: 'Bad Gateway' }) });
    const err = await fail(api.get('queue'));
    expect(err.code).toBe('http_502');
    expect(err.message).toBe('Bad Gateway');
  });

  it('reports 401 from protected endpoints, never from the auth ones', async () => {
    const onunauthorized = vi.fn();
    const api = new Client({ fetch: async () => json(401, { error: { code: 'unauthorized', message: 'sign in' } }), onunauthorized });
    await api.get('movies').catch(() => {});
    expect(onunauthorized).toHaveBeenCalledWith('unauthorized');
    await api.post('auth/login', {}).catch(() => {});
    await api.get('auth/me').catch(() => {});
    await api.get('firstrun').catch(() => {});
    expect(onunauthorized).toHaveBeenCalledTimes(1);
  });

  it('maps network failures and aborts', async () => {
    const api = new Client({ fetch: async () => { throw new TypeError('Failed to fetch'); } });
    const err = await fail(api.get('movies'));
    expect(err.code).toBe('network');
    expect(err.status).toBe(0);
    expect(toApiError(new DOMException('x', 'AbortError')).code).toBe('aborted');
  });

  it('reads a null list as empty and passes the cursor', async () => {
    const fetch = vi.fn(async () => json(200, { items: null }));
    const api = new Client({ fetch });
    expect(await api.list('queue')).toEqual([]);
    expect(await api.page('history', '42', 10)).toEqual({ items: [], nextCursor: undefined });
    expect((fetch.mock.calls[1] as unknown as [string])[0]).toBe('api/v1/history?cursor=42&limit=10');
  });

  it('follows cursors through every page', async () => {
    const pages: Record<string, unknown> = {
      '': { items: [1, 2], nextCursor: '2' },
      '2': { items: [3, 4], nextCursor: '4' },
      '4': { items: [5] },
    };
    const api = new Client({ fetch: async (u) => json(200, pages[new URL(String(u), 'http://x/').searchParams.get('cursor') ?? '']) });
    const seen: number[] = [];
    expect(await api.all<number>('movies', { limit: 2, onpage: (r) => seen.push(r.length) })).toEqual([1, 2, 3, 4, 5]);
    expect(seen).toEqual([2, 4, 5]);
  });

  it('streams NDJSON records as they arrive', async () => {
    const body = '{"record":{"n":1}}\n{"error":{"source":"slow","message":"timeout"}}\n{"summary":{"done":true}}\n';
    const api = new Client({ fetch: async () => new Response(body, { status: 200, headers: { 'Content-Type': 'application/x-ndjson' } }) });
    const lines: unknown[] = [];
    for await (const l of api.stream('movies/1/releases')) lines.push(l);
    expect(lines).toEqual([{ record: { n: 1 } }, { error: { source: 'slow', message: 'timeout' } }, { summary: { done: true } }]);
  });

  it('reads plain text', async () => {
    const api = new Client({ fetch: async () => new Response('version: 1\n', { status: 200 }) });
    expect(await api.text('config/export', { format: 'yaml' })).toBe('version: 1\n');
  });
});
