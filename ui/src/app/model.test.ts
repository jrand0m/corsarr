import { describe, expect, it } from 'vitest';
import { availableAt, eventSubject, eventView, isAvailable, movieStatus, parseEnvExport, queuedMovieIds, toQueueCard } from './model';
import { applyList, facetCounts } from './list';
import type { Download, Movie } from './api/types';
import { instanceView, toSchemaField } from './screens/settings/plugin-form';

const movie = (o: Partial<Movie> = {}): Movie => ({
  id: 1, tmdbId: 603, title: 'The Matrix', year: 1999, monitored: true, minAvailability: 'released', profileKey: 'hd1080p',
  rootFolder: '/m', path: '/m/The Matrix (1999)', addedAt: '2026-11-01T00:00:00Z', ...o,
});
const NOW = Date.parse('2026-11-22T12:00:00Z');

describe('availability', () => {
  it('follows the minimum availability rules', () => {
    expect(availableAt(movie({ minAvailability: 'announced' }))).toBe(0);
    expect(availableAt(movie({ minAvailability: 'inCinemas', inCinemas: '2026-12-01T00:00:00Z' }))).toBe(Date.parse('2026-12-01T00:00:00Z'));
    expect(availableAt(movie({ digital: '2026-12-10T00:00:00Z', physical: '2026-12-05T00:00:00Z' }))).toBe(Date.parse('2026-12-05T00:00:00Z'));
    expect(availableAt(movie({ inCinemas: '2026-01-01T00:00:00Z' }))).toBe(Date.parse('2026-01-01T00:00:00Z') + 90 * 86_400_000);
    expect(availableAt(movie({ year: 2026 }))).toBe(Date.UTC(2027, 0, 1));
    expect(availableAt(movie({ year: 0 }))).toBeUndefined();
    expect(isAvailable(movie(), NOW)).toBe(true);
    expect(isAvailable(movie({ year: 2026 }), NOW)).toBe(false);
  });
});

describe('movieStatus', () => {
  it('reads on disk, queued, missing and not available', () => {
    const q = new Set([2]);
    expect(movieStatus(movie({ fileId: 9 }), NOW, q)).toBe('cutoff-met');
    expect(movieStatus(movie({ id: 2 }), NOW, q)).toBe('queued');
    expect(movieStatus(movie(), NOW, q)).toBe('missing');
    expect(movieStatus(movie({ year: 2027 }), NOW, q)).toBe('not-available');
  });
});

const dl = (o: Partial<Download> = {}): Download => ({
  id: 5, downloader: 'qbit', downloadId: 'h', protocol: 'torrent', kind: 'movie', grabId: 1, title: 'The.Matrix.1999.1080p', state: 'downloading',
  links: [{ item: { kind: 'movie', id: 1 }, status: 'pending' }], progress: 0.4, size: 1e9, downloaded: 4e8, speed: 2e6, eta: 300, seeders: 3, peers: 4,
  category: 'movies', held: false, addedAt: '2026-11-22T10:00:00Z', ...o,
});

describe('queue mapping', () => {
  it('collects movie ids of live downloads', () => {
    expect([...queuedMovieIds([dl(), dl({ id: 6, state: 'imported', links: [{ item: { kind: 'movie', id: 2 }, status: 'imported' }] })])]).toEqual([1]);
  });
  it('builds a queue card titled by the item with the release below', () => {
    const c = toQueueCard(dl(), (id) => (id === 1 ? 'The Matrix' : undefined));
    expect(c).toMatchObject({ id: '5', title: 'The Matrix', subtitle: 'The.Matrix.1999.1080p', state: 'downloading', progress: 0.4, eta: 300, href: '#/library/movies/1' });
    expect(toQueueCard(dl({ links: null, eta: 0 }), () => undefined)).toMatchObject({ title: 'The.Matrix.1999.1080p', subtitle: undefined, eta: undefined });
  });
  it('names a TV download by its episodes, never by a movie with the same id', () => {
    const links = Array.from({ length: 10 }, (_, i) => ({ item: { kind: 'tv' as const, id: i + 1 }, status: 'pending' }));
    const d = dl({ kind: 'tv', title: 'Example.Show.S01.1080p', links });
    expect(toQueueCard(d, () => 'The Matrix')).toMatchObject({ title: '10 episodes', subtitle: 'Example.Show.S01.1080p', kind: 'tv', href: undefined });
    expect(toQueueCard(d, () => 'The Matrix', (ids) => `Example Show · S01 · ${ids.length} episodes`).title).toBe('Example Show · S01 · 10 episodes');
  });
});

describe('events', () => {
  it('labels events and their subject', () => {
    const ev = { eventId: 'x', type: 'import.completed', occurredAt: '', actor: 'automation', items: [{ ref: { kind: 'movie' as const, id: 1 }, title: 'The Matrix' }] };
    expect(eventView(ev)).toEqual({ label: 'Imported', family: 'importOutcome', value: 'imported' });
    expect(eventSubject(ev)).toBe('The Matrix');
    const h = { eventId: 'y', type: 'health.changed', occurredAt: '', actor: 'system', data: { to: 'action_needed', ref: { type: 'metadata', key: 'tmdb' } } };
    expect(eventView(h)).toMatchObject({ family: 'health', value: 'action_needed', label: 'Health: action needed' });
    expect(eventView({ ...h, type: 'mediakind.mode_changed', data: { kind: 'movie', from: 'active', to: 'shadow' } }))
      .toEqual({ label: 'movie mode: shadow', family: 'kindMode', value: 'shadow' });
    expect(eventView({ ...h, type: 'decision.shadowed', data: {} })).toMatchObject({ family: 'kindMode', value: 'shadow' });
    expect(eventSubject(h)).toBe('tmdb');
  });
  it('summarises a season of episodes in one line', () => {
    const items = [1, 2, 3].map((n) => ({ ref: { kind: 'tv' as const, id: n }, title: `Example Show S01E0${n}`, series: 'Example Show' }));
    expect(eventSubject({ eventId: 'z', type: 'grab.sent', occurredAt: '', actor: 'automation', items })).toBe('Example Show · S01 · 3 episodes');
  });
});

describe('parseEnvExport', () => {
  it('reads kinds, root folders, instances and secret references', () => {
    const v = parseEnvExport([
      '# comment', 'CORSARR_SERVER_ADDRESS=127.0.0.1:8090', 'CORSARR_MEDIAKINDS_MUSIC_MODE=shadow', 'CORSARR_MEDIAKINDS_BOOK_MODE=disabled',
      'CORSARR_ROOTFOLDERS_0_PATH=/data/movies', 'CORSARR_ROOTFOLDERS_0_KIND=movie',
      'CORSARR_INDEXERS_KNABEN_TYPE=torznab', 'CORSARR_INDEXERS_KNABEN_NAME=Knaben', 'CORSARR_INDEXERS_KNABEN_URL=http://x/api',
      'CORSARR_INDEXERS_KNABEN_APIKEY=', 'CORSARR_INDEXERS_KNABEN_PRIORITY=10',
      'CORSARR_NOTIFICATIONS_NTFY_URLS_0=ntfy://a', 'CORSARR_NOTIFICATIONS_NTFY_URLS_1=ntfy://b',
      'CORSARR_DOWNLOADCLIENTS_QBIT_CATEGORIES_MOVIE=movies',
    ].join('\n'));
    expect(v.kinds).toEqual({ music: 'shadow', book: 'disabled' });
    expect(v.rootFolders).toEqual([{ path: '/data/movies', kind: 'movie' }]);
    expect(v.server['server.address']).toBe('127.0.0.1:8090');
    const k = v.instances.find((i) => i.key === 'knaben')!;
    expect(k).toMatchObject({ section: 'indexers', plugin: 'torznab', name: 'Knaben', values: { url: 'http://x/api', priority: '10' } });
    expect(k.secrets).toEqual({ apikey: 'CORSARR_SECRET_INDEXERS_KNABEN_APIKEY' });
    expect(v.instances.find((i) => i.key === 'ntfy')!.values.urls).toEqual(['ntfy://a', 'ntfy://b']);
    expect(v.instances.find((i) => i.key === 'qbit')!.values.categories).toEqual({ movie: 'movies' });
  });
});

describe('plugin form', () => {
  it('maps manifest fields to schema fields and locks every value', () => {
    expect(toSchemaField({ key: 'token', type: 'secret', label: 'Token' })).toMatchObject({ type: 'text', secret: true });
    expect(toSchemaField({ key: 'mode', type: 'enum', label: 'Mode', options: ['a', 'b'], showIf: 'on=true' }))
      .toMatchObject({ type: 'select', options: [{ value: 'a', label: 'a' }, { value: 'b', label: 'b' }], visibleWhen: { field: 'on', equals: true } });
    const v = instanceView(
      { section: 'indexers', key: 'knaben', plugin: 'torznab', name: 'Knaben', values: { url: 'http://x', priority: '10' }, secrets: { apikey: 'CORSARR_SECRET_INDEXERS_KNABEN_APIKEY' } },
      { id: 'torznab', kind: 'indexer', version: '1', apiVersion: 1, name: 'Torznab', delivery: 'builtin', settings: [
        { key: 'url', type: 'url', label: 'URL' }, { key: 'apiKey', type: 'secret', label: 'API key' }, { key: 'priority', type: 'number', label: 'Priority' },
      ] },
    );
    expect(v.values).toEqual({ name: 'Knaben', url: 'http://x', priority: 10 });
    expect(v.meta.apiKey).toEqual({ managed: 'indexers.knaben.apiKey', isSet: true, ref: '${env:CORSARR_SECRET_INDEXERS_KNABEN_APIKEY}' });
    expect(v.meta.url).toEqual({ managed: 'indexers.knaben.url' });
  });
});

describe('applyList', () => {
  const rows = [movie({ id: 1, title: 'Alien', year: 1979 }), movie({ id: 2, title: 'Brazil', year: 1985, monitored: false }), movie({ id: 3, title: 'Arrival', year: 2016 })];
  const spec = { text: (m: Movie) => m.title, facets: { monitored: (m: Movie) => (m.monitored ? 'yes' : 'no') }, sortKeys: { title: (m: Movie) => m.title, year: (m: Movie) => m.year } };
  it('filters by text and facets and sorts both ways', () => {
    expect(applyList(rows, { q: 'a', facets: {} }, { column: 'title', direction: 'asc' }, spec).map((m) => m.id)).toEqual([1, 3, 2]);
    expect(applyList(rows, { q: '', facets: { monitored: ['yes'] } }, { column: 'year', direction: 'desc' }, spec).map((m) => m.id)).toEqual([3, 1]);
    expect(applyList(rows, { q: 'zzz', facets: {} }, undefined, spec)).toEqual([]);
  });
  it('counts facet values', () => {
    expect(facetCounts(rows, (m) => (m.monitored ? 'yes' : 'no'))).toEqual({ yes: 2, no: 1 });
  });
});
