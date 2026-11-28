import { describe, expect, it } from 'vitest';
import { applyList } from './list';

interface Row { id: number; title: string; tags: string[]; size: number; release: string }
const rows: Row[] = [
  { id: 1, title: 'Alien', tags: ['scifi', 'horror'], size: 30, release: 'v10' },
  { id: 2, title: 'brazil', tags: ['comedy'], size: 4, release: 'v9' },
  { id: 3, title: 'Arrival', tags: ['scifi'], size: 200, release: 'v100' },
];
const spec = {
  text: (r: Row) => r.title,
  facets: { tag: (r: Row) => r.tags },
  sortKeys: { title: (r: Row) => r.title, size: (r: Row) => r.size, release: (r: Row) => r.release },
};
const ids = (out: Row[]) => out.map((r) => r.id);

describe('applyList', () => {
  it('searches text case-insensitively, ignoring surrounding spaces', () => {
    expect(ids(applyList(rows, { q: '  ARR ', facets: {} }, undefined, spec))).toEqual([3]);
  });

  it('keeps rows having any wanted value of a multi-valued facet', () => {
    expect(ids(applyList(rows, { q: '', facets: { tag: ['horror', 'comedy'] } }, undefined, spec))).toEqual([1, 2]);
  });

  it('ignores empty and unknown facets', () => {
    expect(ids(applyList(rows, { q: '', facets: { tag: [], colour: ['red'] } }, undefined, spec))).toEqual([1, 2, 3]);
    expect(ids(applyList(rows, { q: '', facets: { tag: ['scifi'] } }, undefined, { text: spec.text })))
      .toEqual([1, 2, 3]);
  });

  it('sorts numbers numerically and text naturally without case', () => {
    expect(ids(applyList(rows, { q: '', facets: {} }, { column: 'size', direction: 'asc' }, spec))).toEqual([2, 1, 3]);
    expect(ids(applyList(rows, { q: '', facets: {} }, { column: 'release', direction: 'asc' }, spec))).toEqual([2, 1, 3]);
    expect(ids(applyList(rows, { q: '', facets: {} }, { column: 'title', direction: 'desc' }, spec))).toEqual([2, 3, 1]);
  });

  it('leaves the order alone for a column without a sort key, and does not touch the input', () => {
    const input = rows.slice();
    expect(ids(applyList(input, { q: '', facets: {} }, { column: 'tags', direction: 'asc' }, spec))).toEqual([1, 2, 3]);
    applyList(input, { q: '', facets: {} }, { column: 'size', direction: 'desc' }, spec);
    expect(ids(input)).toEqual([1, 2, 3]);
  });
});
