// The one HTTP client of the web UI: JSON in and out, the `{error:{code,message,field?}}`
// shape as a typed ApiError, cursor paging, NDJSON streams and one place that turns 401 into
// "go to sign-in" (or first run). Paths are relative ("movies"), so the UI works under any base path.
import { readNdjson } from './ndjson';

export interface ErrorDetail { code: string; message: string; field?: string }

export class ApiError extends Error {
  readonly status: number;
  readonly code: string;
  readonly field?: string;
  /** The whole response body (409/403 grabs carry the decision next to the error). */
  readonly body: Record<string, unknown>;

  constructor(status: number, detail: ErrorDetail, body: Record<string, unknown> = {}) {
    super(detail.message);
    this.name = 'ApiError';
    this.status = status;
    this.code = detail.code;
    this.field = detail.field;
    this.body = body;
  }

  /** "upstream · 502": the code line ErrorState shows. */
  get label(): string {
    return this.status ? `${this.code} · ${this.status}` : this.code;
  }
}

export interface Page<T> { items: T[]; nextCursor?: string }

export type Query = Record<string, string | number | boolean | undefined>;

export interface RequestOptions { query?: Query; body?: unknown; signal?: AbortSignal; accept?: string }

export interface ClientOptions {
  /** Prefix of every path; default "api/v1/" (relative to the page, so base paths just work). */
  base?: string;
  fetch?: typeof fetch;
  /** Called on 401 from a protected endpoint: `setup_required` or `unauthorized`. */
  onunauthorized?: (code: string) => void;
}

/** Normalises any thrown value into an ApiError (network failures get status 0). */
export function toApiError(e: unknown): ApiError {
  if (e instanceof ApiError) return e;
  if (e instanceof DOMException && e.name === 'AbortError') return new ApiError(0, { code: 'aborted', message: 'The request was cancelled.' });
  return new ApiError(0, { code: 'network', message: 'corsarr is not answering. Check that the server is running and reachable.' });
}

export function buildUrl(base: string, path: string, query?: Query): string {
  const qs = new URLSearchParams();
  for (const [k, v] of Object.entries(query ?? {})) if (v !== undefined && v !== '') qs.set(k, String(v));
  const s = qs.toString();
  return `${base}${path.replace(/^\//, '')}${s ? `?${s}` : ''}`;
}

const isAuthPath = (path: string) => /^\/?(auth\/(login|setup|me|logout)|firstrun)\b/.test(path);

export class Client {
  readonly base: string;
  readonly #fetch: typeof fetch;
  readonly #onunauthorized?: (code: string) => void;

  constructor(opts: ClientOptions = {}) {
    this.base = opts.base ?? 'api/v1/';
    this.#fetch = opts.fetch ?? ((...a) => fetch(...a));
    this.#onunauthorized = opts.onunauthorized;
  }

  url(path: string, query?: Query): string {
    return buildUrl(this.base, path, query);
  }

  /** Sends the request; resolves the raw Response when it is 2xx, throws ApiError otherwise. */
  async raw(method: string, path: string, o: RequestOptions = {}): Promise<Response> {
    const headers: Record<string, string> = { Accept: o.accept ?? 'application/json' };
    let body: string | undefined;
    if (o.body !== undefined) {
      headers['Content-Type'] = 'application/json';
      body = JSON.stringify(o.body);
    }
    let res: Response;
    try {
      res = await this.#fetch(this.url(path, o.query), { method, headers, body, signal: o.signal, credentials: 'same-origin' });
    } catch (e) {
      throw toApiError(e);
    }
    if (res.ok) return res;
    const parsed = await res.json().catch(() => ({})) as Record<string, unknown>;
    const detail = (parsed.error as ErrorDetail | undefined) ?? { code: `http_${res.status}`, message: res.statusText || `HTTP ${res.status}` };
    const err = new ApiError(res.status, detail, parsed);
    if (res.status === 401 && !isAuthPath(path)) this.#onunauthorized?.(err.code);
    throw err;
  }

  async request<T>(method: string, path: string, o: RequestOptions = {}): Promise<T> {
    const res = await this.raw(method, path, o);
    if (res.status === 204) return undefined as T;
    const text = await res.text();
    return (text ? JSON.parse(text) : undefined) as T;
  }

  get<T>(path: string, query?: Query, signal?: AbortSignal): Promise<T> {
    return this.request<T>('GET', path, { query, signal });
  }
  post<T>(path: string, body?: unknown): Promise<T> {
    return this.request<T>('POST', path, { body });
  }
  put<T>(path: string, body?: unknown): Promise<T> {
    return this.request<T>('PUT', path, { body });
  }
  del<T = void>(path: string, query?: Query): Promise<T> {
    return this.request<T>('DELETE', path, { query });
  }

  /** Plain text (config export). */
  async text(path: string, query?: Query): Promise<string> {
    return (await this.raw('GET', path, { query, accept: 'text/plain, application/yaml, */*' })).text();
  }

  /** `{items}` lists; a null list (Go nil slice) reads as empty. */
  async list<T>(path: string, query?: Query, signal?: AbortSignal): Promise<T[]> {
    return (await this.get<{ items: T[] | null }>(path, query, signal)).items ?? [];
  }

  /** One cursor page: `?cursor=&limit=` → `{items, nextCursor?}`. */
  async page<T>(path: string, cursor?: string, limit = 50, query?: Query, signal?: AbortSignal): Promise<Page<T>> {
    const p = await this.get<{ items: T[] | null; nextCursor?: string }>(path, { ...query, cursor, limit }, signal);
    return { items: p.items ?? [], nextCursor: p.nextCursor || undefined };
  }

  /** Every page in order, calling `onpage` with the rows so far (first rows show at once). */
  async all<T>(path: string, o: { limit?: number; max?: number; signal?: AbortSignal; onpage?: (rows: T[]) => void } = {}): Promise<T[]> {
    const rows: T[] = [];
    let cursor: string | undefined;
    do {
      const p = await this.page<T>(path, cursor, o.limit ?? 200, undefined, o.signal);
      rows.push(...p.items);
      o.onpage?.(rows.slice());
      cursor = p.nextCursor;
    } while (cursor && rows.length < (o.max ?? Infinity));
    return rows;
  }

  /** NDJSON lines as they arrive. */
  async *stream<T>(path: string, signal?: AbortSignal): AsyncGenerator<T> {
    const res = await this.raw('GET', path, { signal, accept: 'application/x-ndjson' });
    if (!res.body) return;
    yield* readNdjson<T>(res.body);
  }

  /** NDJSON lines of a POST (a long run that reports progress) as they arrive. */
  async *postStream<T>(path: string, body: unknown, signal?: AbortSignal): AsyncGenerator<T> {
    const res = await this.raw('POST', path, { body, signal, accept: 'application/x-ndjson' });
    if (!res.body) return;
    yield* readNdjson<T>(res.body);
  }
}
