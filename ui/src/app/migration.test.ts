import { describe, expect, it } from 'vitest';
import {
  applyCount, countTiles, defaultForm, filterEntries, outcomeView, planFacets, progressOf, requestBody, scopeProblems, sourceProblems, toggleExclude,
  writes, type PlanEntry, type StreamLine,
} from './migration';
import { vocabulary } from '../lib/status/vocabulary';

const entries: PlanEntry[] = [
  { id: 'sonarr:series:1', category: 'library', source: 'sonarr', name: 'Show (2020)', outcome: 'create', reason: 'new' },
  { id: 'sonarr:episodefile:2', category: 'files', source: 'sonarr', name: '/tv/a.mkv', outcome: 'fileMissing', reason: 'not found on disk' },
  { id: 'corsarr:rules:all', category: 'modes', source: '', name: 'Automation rules', outcome: 'create', reason: 'dry-run' },
];

describe('migration form', () => {
  it('builds the request from the chosen sources only', () => {
    const f = defaultForm();
    f.sources[0] = { app: 'sonarr', include: true, url: ' http://sonarr:8989 ', apiKey: 'k' };
    f.sources[1] = { app: 'radarr', include: false, url: 'http://radarr', apiKey: 'k' };
    f.fromPrefix = '/data/media';
    f.toPrefix = '/srv/media';
    expect(requestBody(f, 'abc')).toEqual({
      sources: [{ app: 'sonarr', url: 'http://sonarr:8989', apiKey: 'k' }], scope: f.scope, activate: false, exclude: [],
      pathRewrite: '/data/media:/srv/media', planHash: 'abc',
    });
    f.scope = [...f.scope, 'history'];
    expect(requestBody(f).historyDays).toBe(90);
  });

  it('checks sources and scope before moving on', () => {
    const f = defaultForm();
    expect(sourceProblems(f)).toEqual(['Choose at least one source.']);
    f.sources[2] = { app: 'prowlarr', include: true, url: 'prowlarr', apiKey: '' };
    expect(sourceProblems(f)).toHaveLength(2);
    f.sources[2] = { app: 'prowlarr', include: true, url: 'https://p', apiKey: 'k' };
    expect(sourceProblems(f)).toEqual([]);
    expect(scopeProblems(f)).toEqual([]);
    f.fromPrefix = 'data';
    expect(scopeProblems(f)).toHaveLength(2);
    f.scope = [];
    f.fromPrefix = '';
    expect(scopeProblems(f)).toEqual(['Choose at least one category.']);
  });
});

describe('migration report', () => {
  it('shows every outcome with a known badge', () => {
    for (const o of ['create', 'update', 'unchanged', 'skip', 'approximate', 'unsupported', 'conflict', 'secretNeeded', 'fileMissing',
      'managedByConfig', 'gone', 'failed'] as const) {
      const v = outcomeView(o);
      expect((vocabulary as Record<string, Record<string, unknown>>)[v.family][v.value], o).toBeDefined();
    }
  });

  it('orders the count tiles and drops empty ones', () => {
    expect(countTiles({ skip: 1, create: 3, update: 0 })).toEqual([{ outcome: 'create', label: 'Create', n: 3 }, { outcome: 'skip', label: 'Skip', n: 1 }]);
    expect(countTiles(null)).toEqual([]);
  });

  it('filters by facet and text', () => {
    const facets = planFacets(entries);
    expect(facets.map((f) => f.id)).toEqual(['outcome', 'category', 'source']);
    expect(facets[2].options.map((o) => o.value)).toEqual(['corsarr', 'sonarr']);
    expect(filterEntries(entries, { q: '', facets: { outcome: ['create'] } })).toHaveLength(2);
    expect(filterEntries(entries, { q: 'disk', facets: {} })).toEqual([entries[1]]);
    expect(filterEntries(entries, { q: '', facets: { source: ['corsarr'] } })).toEqual([entries[2]]);
  });

  it('tracks progress per source from the stream', () => {
    const running: StreamLine[] = [{ record: { progress: { source: 'sonarr', step: 'series', done: 0, total: 0 } } }];
    expect(progressOf(['sonarr', 'radarr'], running).map((p) => p.state)).toEqual(['running', 'pending']);
    const done: StreamLine[] = [...running, { summary: { done: true, report: { mode: 'plan', planHash: 'h', counts: {}, entries,
      sources: [{ app: 'sonarr', url: 'u', supported: '3.x' }, { app: 'radarr', url: 'u', supported: '3.x', error: 'unauthorised' }] } } }];
    const p = progressOf(['sonarr', 'radarr'], done);
    expect(p[0]).toMatchObject({ state: 'ok', count: 2 });
    expect(p[1]).toMatchObject({ state: 'failed', reason: 'unauthorised' });
  });

  it('leaves out only what an apply would write', () => {
    expect(entries.filter(writes).map((e) => e.id)).toEqual(['sonarr:series:1', 'corsarr:rules:all']);
    expect(toggleExclude(toggleExclude([], 'a'), 'a')).toEqual([]);
    const profile: PlanEntry = { id: 'sonarr:profile:2', category: 'profiles', source: 'sonarr', name: 'Top', outcome: 'approximate', reason: 'x' };
    expect(writes(profile)).toBe(false);
    expect(applyCount([...entries, profile], ['corsarr:rules:all'])).toBe(1);
    expect(applyCount(null, [])).toBe(0);
  });
});
