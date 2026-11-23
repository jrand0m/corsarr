import { describe, it, expect } from 'vitest';
import { parseHash, formatHash, href, inArea, listStateToParams, listStateFromParams } from './hash';

const facets = [{ id: 'status', label: 'Status', options: [{ value: 'missing', label: 'Missing' }] }];

describe('hash routes', () => {
  it('parses and formats paths with query state', () => {
    const r = parseHash('#/library/movies?view=posters&status=missing');
    expect(r.path).toBe('/library/movies');
    expect(r.params.get('view')).toBe('posters');
    expect(parseHash('').path).toBe('/');
    expect(parseHash('#settings').path).toBe('/settings');
    expect(formatHash('/activity/queue', new URLSearchParams())).toBe('#/activity/queue');
    expect(href('/search', { q: 'haul' })).toBe('#/search?q=haul');
  });

  it('matches areas without prefix collisions', () => {
    expect(inArea('/library/movies/42', '/library/movies')).toBe(true);
    expect(inArea('/library/movies-old', '/library/movies')).toBe(false);
    expect(inArea('/library', '/')).toBe(false);
    expect(inArea('/', '/')).toBe(true);
  });

  it('round-trips list state and keeps defaults out of the URL', () => {
    const p = listStateToParams({ filter: { q: 'salt', facets: { status: ['missing'] } }, sort: { column: 'size', direction: 'desc' }, view: 'posters' });
    expect(p.toString()).toBe('q=salt&status=missing&sort=size%3Adesc&view=posters');
    expect(listStateFromParams(p, facets)).toEqual({ filter: { q: 'salt', facets: { status: ['missing'] } }, sort: { column: 'size', direction: 'desc' }, view: 'posters' });
    expect(listStateToParams({ filter: { q: '', facets: {} }, view: 'table' }).toString()).toBe('');
    expect(listStateFromParams(new URLSearchParams('view=bogus'), facets, { sort: { column: 'title', direction: 'asc' } }))
      .toEqual({ filter: { q: '', facets: {} }, sort: { column: 'title', direction: 'asc' }, view: 'table' });
  });
});
