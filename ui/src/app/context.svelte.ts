// App-wide state handed to screens through Svelte context: the API client, the SSE store, the toast
// queue, navigation and the shared movie index (titles for queue, history and palette rows).
import { getContext, setContext } from 'svelte';
import type { Client } from './api/client';
import type { Movie, QualityProfile, SystemStatus } from './api/types';
import type { LiveEvents } from './live.svelte';
import { Resource } from './resource.svelte';
import { ToastQueue } from '../lib/toast/toasts.svelte';
import { formatHash } from '../lib/route/hash';

/** Every movie, loaded page by page (first rows paint at once) and kept fresh by item.* events. */
export class MovieIndex {
  rows = $state<Movie[]>([]);
  loaded = $state(false);
  readonly res: Resource<Movie[]>;
  #byId = $derived(new Map(this.rows.map((m) => [m.id, m])));

  constructor(api: Client) {
    this.res = new Resource((signal) => api.all<Movie>('movies', { signal, onpage: (rows) => { if (!signal.aborted) this.rows = rows; } }));
  }

  async load(quiet = false): Promise<void> {
    await this.res.load(quiet);
    if (this.res.data) this.rows = this.res.data;
    this.loaded = !this.res.error;
  }

  /** Loads once; later calls reuse the index. */
  ensure(): void {
    if (this.#started) return;
    this.#started = true;
    void this.load();
  }
  #started = false;

  get(id: number): Movie | undefined {
    return this.#byId.get(id);
  }

  title = (id: number): string | undefined => this.#byId.get(id)?.title;

  upsert(m: Movie): void {
    const i = this.rows.findIndex((r) => r.id === m.id);
    if (i >= 0) this.rows[i] = m;
    else this.rows = [...this.rows, m];
  }

  remove(id: number): void {
    this.rows = this.rows.filter((r) => r.id !== id);
  }
}

export interface AppContext {
  api: Client;
  live: LiveEvents;
  toasts: ToastQueue;
  movies: MovieIndex;
  profiles: Resource<QualityProfile[]>;
  status: Resource<SystemStatus>;
  navigate: (path: string, params?: URLSearchParams) => void;
}

const KEY = Symbol('corsarr-app');

export const setApp = (ctx: AppContext) => setContext(KEY, ctx);
export const app = (): AppContext => getContext<AppContext>(KEY);

export function navigateTo(path: string, params?: URLSearchParams): void {
  if (typeof location !== 'undefined') location.hash = formatHash(path, params);
}

/** Profile key → name ("hd1080p" → "HD-1080p"). */
export function profileName(profiles: QualityProfile[] | undefined, key: string): string {
  return profiles?.find((p) => p.key === key)?.name ?? key;
}
