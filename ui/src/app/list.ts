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
  const out = rows.filter((r) => (!q || spec.text(r).toLowerCase().includes(q)) && matchesFacets(r, filter.facets, spec));
  return sortRows(out, sort, spec);
}

/** A row passes when, for every facet with wanted values, one of its values is wanted. */
function matchesFacets<T>(row: T, facets: FilterState['facets'], spec: ListSpec<T>): boolean {
  for (const [id, wanted] of Object.entries(facets)) {
    const get = spec.facets?.[id];
    if (!wanted.length || !get) continue;
    const v = get(row);
    const values = Array.isArray(v) ? v : [v];
    if (!values.some((x) => wanted.includes(x))) return false;
  }
  return true;
}

/** A sorted copy by the column's key; rows stay as they are when the column has none. */
function sortRows<T>(rows: T[], sort: SortState | undefined, spec: ListSpec<T>): T[] {
  const key = sort && spec.sortKeys?.[sort.column];
  if (!sort || !key) return rows;
  const dir = sort.direction === 'desc' ? -1 : 1;
  return rows.slice().sort((a, b) => compareKeys(key(a), key(b)) * dir);
}

/** Numbers by value; anything else as text, numbers inside it in order, ignoring case. */
const compareKeys = (x: string | number, y: string | number) =>
  typeof x === 'number' && typeof y === 'number' ? x - y : String(x).localeCompare(String(y), undefined, { numeric: true, sensitivity: 'base' });

/** Counts per facet option over all rows (shown next to each option). */
export function facetCounts<T>(rows: T[], get: (row: T) => string): Record<string, number> {
  const out: Record<string, number> = {};
  for (const r of rows) {
    const v = get(r);
    out[v] = (out[v] ?? 0) + 1;
  }
  return out;
}
