// Client-side filter and sort for list screens whose endpoint has neither (the list APIs page by
// cursor only). Pure; the page passes how to read text, facet values and sort keys from a row.
import type { FilterState } from '../lib/filters/url';
import type { SortState } from '../lib/components/organisms/DataTable/DataTable.svelte';

export interface ListSpec<T> {
  /** Text the free-text filter searches. */
  text: (row: T) => string;
  /** Facet id → the row's value(s) for it. */
  facets?: Record<string, (row: T) => string | string[]>;
  /** Column id → sort key. */
  sortKeys?: Record<string, (row: T) => string | number>;
}

export function applyList<T>(rows: T[], filter: FilterState, sort: SortState | undefined, spec: ListSpec<T>): T[] {
  const q = filter.q.trim().toLowerCase();
  let out = rows.filter((r) => {
    if (q && !spec.text(r).toLowerCase().includes(q)) return false;
    for (const [id, wanted] of Object.entries(filter.facets)) {
      if (!wanted.length) continue;
      const get = spec.facets?.[id];
      if (!get) continue;
      const v = get(r);
      const values = Array.isArray(v) ? v : [v];
      if (!values.some((x) => wanted.includes(x))) return false;
    }
    return true;
  });
  const key = sort && spec.sortKeys?.[sort.column];
  if (sort && key) {
    const dir = sort.direction === 'desc' ? -1 : 1;
    out = out.slice().sort((a, b) => {
      const x = key(a);
      const y = key(b);
      return (typeof x === 'number' && typeof y === 'number' ? x - y : String(x).localeCompare(String(y), undefined, { numeric: true, sensitivity: 'base' })) * dir;
    });
  }
  return out;
}

/** Counts per facet option over all rows (shown next to each option). */
export function facetCounts<T>(rows: T[], get: (row: T) => string): Record<string, number> {
  const out: Record<string, number> = {};
  for (const r of rows) {
    const v = get(r);
    out[v] = (out[v] ?? 0) + 1;
  }
  return out;
}
