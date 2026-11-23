// Filter state for list screens and its URL form (filters live in the query
// string so back/forward restore them). Pure: no window access.

export interface FilterOption { value: string; label: string; count?: number }
export interface FilterFacet {
  id: string;
  label: string;
  options: FilterOption[];
  /** Several values at once (default) or exactly one. */
  multiple?: boolean;
}
export interface FilterState { q: string; facets: Record<string, string[]> }

export const emptyFilter = (): FilterState => ({ q: '', facets: {} });

export function activeCount(state: FilterState): number {
  return Object.values(state.facets).reduce((n, v) => n + v.length, 0) + (state.q.trim() ? 1 : 0);
}

/** Writes `q` and one comma-separated param per facet onto `base` (other params kept). */
export function filterToParams(state: FilterState, base: URLSearchParams = new URLSearchParams()): URLSearchParams {
  const out = new URLSearchParams(base);
  if (state.q.trim()) out.set('q', state.q.trim()); else out.delete('q');
  for (const [id, values] of Object.entries(state.facets)) {
    if (values.length) out.set(id, values.join(',')); else out.delete(id);
  }
  return out;
}

/** Reads the state back, keeping only known facets and option values (stale links stay safe). */
export function filterFromParams(params: URLSearchParams, facets: FilterFacet[]): FilterState {
  const state: FilterState = { q: params.get('q') ?? '', facets: {} };
  for (const f of facets) {
    const raw = params.get(f.id);
    if (!raw) continue;
    const known = new Set(f.options.map((o) => o.value));
    let values = raw.split(',').filter((v) => known.has(v));
    if (f.multiple === false) values = values.slice(0, 1);
    if (values.length) state.facets[f.id] = values;
  }
  return state;
}
