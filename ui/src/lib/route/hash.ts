// Hash routes: `#/library/movies?view=posters&status=missing`. The embedded
// UI uses the hash so the Go server serves one index.html under any base path.
// Pure helpers plus two thin window wrappers; templates sync view state through these.
import { filterFromParams, filterToParams, type FilterFacet, type FilterState } from '../filters/url';
import type { SortState } from '../components/organisms/DataTable/DataTable.svelte';

export interface Route { path: string; params: URLSearchParams }

export function parseHash(hash: string): Route {
  const raw = hash.replace(/^#/, '') || '/';
  const q = raw.indexOf('?');
  const path = (q < 0 ? raw : raw.slice(0, q)) || '/';
  return { path: path.startsWith('/') ? path : `/${path}`, params: new URLSearchParams(q < 0 ? '' : raw.slice(q + 1)) };
}

export function formatHash(path: string, params?: URLSearchParams): string {
  const qs = params?.toString();
  return `#${path}${qs ? `?${qs}` : ''}`;
}

/** Link builder for nav and status links. */
export const href = (path: string, params?: Record<string, string>): string =>
  formatHash(path, params ? new URLSearchParams(params) : undefined);

/** True when `path` is `area` or below it ("/library/movies/42" is in "/library/movies"). */
export const inArea = (path: string, area: string): boolean =>
  area === '/' ? path === '/' : path === area || path.startsWith(`${area}/`);

// List view state (filters + sort + view mode) in the query string.
export type ListView = 'table' | 'posters';
export interface ListState { filter: FilterState; sort?: SortState; view: ListView }

export function listStateToParams(state: ListState, base?: URLSearchParams): URLSearchParams {
  const p = filterToParams(state.filter, base);
  if (state.sort) p.set('sort', `${state.sort.column}${state.sort.direction === 'desc' ? ':desc' : ''}`); else p.delete('sort');
  if (state.view !== 'table') p.set('view', state.view); else p.delete('view');
  return p;
}

export function listStateFromParams(params: URLSearchParams, facets: FilterFacet[], fallback: Partial<ListState> = {}): ListState {
  const s = params.get('sort');
  const [column, dir] = (s ?? '').split(':');
  const view = params.get('view');
  return {
    filter: filterFromParams(params, facets),
    sort: column ? { column, direction: dir === 'desc' ? 'desc' : 'asc' } : fallback.sort,
    view: view === 'posters' || view === 'table' ? view : (fallback.view ?? 'table'),
  };
}

/** Current route from the window (SSR and tests without a window get "/"). */
export function currentRoute(): Route {
  return parseHash(typeof location === 'undefined' ? '' : location.hash);
}

/** Replaces the query of the current hash route without adding a history entry (filters, sort, view). */
export function replaceParams(params: URLSearchParams): void {
  if (typeof location === 'undefined' || typeof history === 'undefined') return;
  const next = formatHash(currentRoute().path, params);
  if (next !== location.hash) history.replaceState(history.state, '', next);
}
