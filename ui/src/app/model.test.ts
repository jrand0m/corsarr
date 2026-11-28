import { describe, expect, it } from 'vitest';
import { actorLabel, availableAt, eventDetail, eventSubject, eventView, isAvailable, movieStatus, parseEnvExport, queuedMovieIds, settingsFor, toQueueCard } from './model';
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
  it('lets TV links win over a movie link in the same download', () => {
    const links = [{ item: { kind: 'movie' as const, id: 1 }, status: 'pending' }, { item: { kind: 'tv' as const, id: 9 }, status: 'pending' }];
    expect(toQueueCard(dl({ kind: '' as Download['kind'], links }), () => 'The Matrix', () => undefined))
      .toMatchObject({ title: '1 episode', kind: 'tv', href: undefined });
  });
  it('keeps the release as title and still links the movie when its title is unknown', () => {
    expect(toQueueCard(dl({ kind: '' as Download['kind'] }), () => undefined))
      .toMatchObject({ title: 'The.Matrix.1999.1080p', subtitle: undefined, kind: 'movie', href: '#/library/movies/1' });
  });
  it('treats an empty item or episode title like a missing one', () => {
    expect(toQueueCard(dl(), () => '')).toMatchObject({ title: 'The.Matrix.1999.1080p', subtitle: undefined });
    const links = [{ item: { kind: 'tv' as const, id: 9 }, status: 'pending' }];
    expect(toQueueCard(dl({ kind: 'tv', links }), () => undefined, () => '').title).toBe('1 episode');
  });
  it('hides progress of a queued download of unknown size and blanks empty numbers', () => {
    const c = toQueueCard(dl({ state: 'queued', size: 0, downloaded: 0, speed: 0, eta: -1, statusReason: '', held: true }), () => 'The Matrix');
    expect(c).toMatchObject({ state: 'queued', held: true, progress: undefined, size: undefined, downloaded: undefined, speed: undefined, eta: undefined, reason: undefined });
    expect(toQueueCard(dl({ state: 'queued', statusReason: 'waiting for a slot' }), () => 'The Matrix')).toMatchObject({ progress: 0.4, reason: 'waiting for a slot' });
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
  it('words plugin module changes', () => {
    const ev = (data: Record<string, unknown>) => ({ eventId: 'p', type: 'plugin.changed', occurredAt: '', actor: 'user', data });
    const up = ev({ action: 'updated', id: 'harbourdl', version: '1.3.0', previous: '1.2.0', sha256: 'ab', source: 'catalogue' });
    expect(eventView(up)).toEqual({ label: 'Plugin updated', family: 'check', value: 'pass' });
    expect(eventSubject(up)).toBe('harbourdl');
    expect(eventDetail(up)).toBe('1.3.0 (was 1.2.0) · from the catalogue');
    expect(eventView(ev({ action: 'installed', id: 'x', version: '1.0.0' })).label).toBe('Plugin installed');
    expect(eventView(ev({ action: 'rolledback', id: 'x', version: '1.0.0' })).label).toBe('Plugin rolled back');
    const gone = ev({ action: 'uninstalled', id: 'x', version: '1.0.0', source: 'file' });
    expect(eventView(gone)).toEqual({ label: 'Plugin uninstalled', family: 'download', value: 'removed' });
    expect(eventDetail(gone)).toBe('1.0.0 · from a file');
    expect(eventDetail(ev({ action: 'installed', id: 'x', version: '1.0.0', previous: '' }))).toBe('1.0.0');
    expect(eventDetail(ev({ action: 'installed', id: 'x', version: '1.0.0', previous: 1 }))).toBe('1.0.0');
  });
  it('treats an empty subject like a missing one', () => {
    const ev = (type: string, data: Record<string, unknown>) => ({ eventId: 'e', type, occurredAt: '', actor: 'system', data });
    expect(eventSubject(ev('plugin.changed', { action: 'installed', id: '', version: '1.0.0' }))).toBe('plugin.changed');
    expect(eventSubject(ev('grab.sent', { release: { title: '' }, title: 'Fallback' }))).toBe('Fallback');
    expect(eventSubject(ev('grab.sent', { release: { title: '' }, title: '' }))).toBe('grab.sent');
  });
  it('falls back for unknown and incomplete events', () => {
    const ev = (type: string, data?: Record<string, unknown>) => ({ eventId: 'u', type, occurredAt: '', actor: 'system', data });
    expect(eventView(ev('health.changed'))).toEqual({ label: 'Health: OK', family: 'health', value: 'ok' });
    expect(eventView(ev('health.changed', { to: 'sideways' }))).toEqual({ label: 'Health: sideways', family: 'health', value: 'sideways' });
    expect(eventView(ev('mediakind.mode_changed'))).toEqual({ label: 'Kind mode: active', family: 'kindMode', value: 'active' });
    expect(eventView(ev('plugin.changed', { action: 'teleported' }))).toEqual({ label: 'Plugin changed', family: 'check', value: 'pass' });
    expect(eventView(ev('plugin.changed'))).toEqual({ label: 'Plugin changed', family: 'check', value: 'pass' });
    expect(eventView(ev('something.new'))).toEqual({ label: 'something.new', family: 'check', value: 'pending' });
  });
  it('names the subject from the integration, release or event type', () => {
    const ev = (type: string, data?: Record<string, unknown>) => ({ eventId: 's', type, occurredAt: '', actor: 'system', data });
    expect(eventSubject(ev('config.drifted', { ref: { type: 'config', key: 'source' } }))).toBe('Configuration source');
    expect(eventSubject(ev('grab.failed', { ref: { type: 'indexer' }, release: { title: 'Alien.1979.1080p' }, title: 'other' }))).toBe('Alien.1979.1080p');
    expect(eventSubject(ev('payload.rejected', { title: 'Brazil.1985.720p' }))).toBe('Brazil.1985.720p');
    expect(eventSubject(ev('plugin.changed', { id: 42 }))).toBe('plugin.changed');
    expect(eventSubject(ev('config.applied'))).toBe('config.applied');
    expect(eventSubject({ ...ev('grab.sent'), items: [] })).toBe('grab.sent');
  });
  it('lists item titles unless every item is one of several episodes', () => {
    const base = { eventId: 'i', type: 'grab.sent', occurredAt: '', actor: 'automation' };
    const ep = (n: number) => ({ ref: { kind: 'tv' as const, id: n }, title: `Example Show S01E0${n}`, series: 'Example Show' });
    expect(eventSubject({ ...base, items: [ep(1)] })).toBe('Example Show S01E01');
    expect(eventSubject({ ...base, items: [ep(1), ep(2), { ref: { kind: 'movie' as const, id: 3 }, title: 'Alien' }] })).toBe('Example Show S01E01, Example Show S01E02, Alien');
    expect(eventSubject({ ...base, items: [ep(1), { ref: { kind: 'tv' as const, id: 7, variant: 'series' }, title: 'Example Show' }] })).toBe('Example Show S01E01, Example Show');
  });
  it('details an event from its message, reason, release and path', () => {
    const ev = (type: string, data?: Record<string, unknown>) => ({ eventId: 'd', type, occurredAt: '', actor: 'system', data });
    expect(eventDetail(ev('import.blocked', { message: 'no match', reason: 'sample', release: { title: 'Alien.1979' }, path: '/dl/alien' })))
      .toBe('no match · sample · Alien.1979 · /dl/alien');
    expect(eventDetail(ev('import.failed', { message: '', reason: 3, path: '/dl/x' }))).toBe('/dl/x');
    expect(eventDetail(ev('config.applied'))).toBeUndefined();
    expect(eventDetail(ev('plugin.changed', {}))).toBeUndefined();
    expect(eventDetail(ev('plugin.changed', { source: 'file' }))).toBe('from a file');
  });
  it('names who acted', () => {
    expect(actorLabel('user')).toBe('You');
    expect(actorLabel('automation')).toBe('Automation');
    expect(actorLabel('system')).toBe('corsarr');
    expect(actorLabel('plugin:harbourdl')).toBe('Plugin harbourdl');
    expect(actorLabel('import:radarr')).toBe('Imported from radarr');
    expect(actorLabel('rule:7')).toBe('Rule');
    expect(actorLabel('mcp')).toBe('mcp');
    expect(actorLabel('')).toBe('—');
  });
  it('summarises a season of episodes in one line', () => {
    const items = [1, 2, 3].map((n) => ({ ref: { kind: 'tv' as const, id: n }, title: `Example Show S01E0${n}`, series: 'Example Show' }));
    expect(eventSubject({ eventId: 'z', type: 'grab.sent', occurredAt: '', actor: 'automation', items })).toBe('Example Show · S01 · 3 episodes');
  });
});

describe('parseEnvExport', () => {
  it('reads list and map entries after a plain base value of the same field', () => {
    const v = parseEnvExport([
      'CORSARR_NOTIFICATIONS_NTFY_TYPE=ntfy', 'CORSARR_NOTIFICATIONS_NTFY_URLS=x', 'CORSARR_NOTIFICATIONS_NTFY_URLS_0=ntfy://a',
      'CORSARR_DOWNLOADCLIENTS_QBIT_TYPE=qbittorrent', 'CORSARR_DOWNLOADCLIENTS_QBIT_CATEGORIES=x', 'CORSARR_DOWNLOADCLIENTS_QBIT_CATEGORIES_MOVIE=movies',
    ].join('\n'));
    expect(v.instances.find((i) => i.key === 'ntfy')!.values.urls).toEqual(['ntfy://a']);
    expect(v.instances.find((i) => i.key === 'qbit')!.values.categories).toEqual({ movie: 'movies' });
  });
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
    expect(v.mcp).toEqual({ enabled: false, scope: 'read', tools: [] });
  });

  it('reads the assistant endpoint settings', () => {
    const v = parseEnvExport(['CORSARR_MCP_ENABLED=true', 'CORSARR_MCP_SCOPE=write', 'CORSARR_MCP_RATELIMIT=30',
      'CORSARR_MCP_TOOLS_0=find_media', 'CORSARR_MCP_TOOLS_1=request_status', 'CORSARR_MCP_TOOLS=', 'CORSARR_MCP_ALLOWEDORIGINS_0=http://x'].join('\n'));
    expect(v.mcp).toEqual({ enabled: true, scope: 'write', rateLimit: 30, tools: ['find_media', 'request_status'] });
  });

  it('skips lines it cannot place', () => {
    const v = parseEnvExport([
      '', '   ', 'CORSARR_SERVER_PORT', 'OTHER_SERVER_ADDRESS=x', 'CORSARR_MEDIAKINDS_MOVIE_ENABLED=true', 'CORSARR_UNKNOWN_A_B=1',
      'CORSARR_INDEXERS_KNABEN=1', 'CORSARR_ROOTFOLDERS_1_PATH=/data/tv', 'CORSARR_ROOTFOLDERS_1_FREE=9', 'CORSARR_MCP_TOOLS_0=',
      'CORSARR_MCP_OTHER=1', '  CORSARR_DATA_DIR=/var/lib/corsarr  ', 'CORSARR_LOG_LEVEL=debug', 'CORSARR_AUTH_MODE=basic',
    ].join('\n'));
    expect(v.kinds).toEqual({});
    expect(v.instances).toEqual([]);
    expect(v.rootFolders).toEqual([{ path: '/data/tv', kind: '' }]);
    expect(v.mcp).toEqual({ enabled: false, scope: 'read', tools: [] });
    expect(v.server).toEqual({ 'data.dir': '/var/lib/corsarr', 'log.level': 'debug', 'auth.mode': 'basic' });
  });

  it('keeps instances of each section apart and groups their fields', () => {
    const v = parseEnvExport([
      'CORSARR_SOURCES_A_TYPE=s', 'CORSARR_MEDIASERVERS_A_TYPE=jellyfin', 'CORSARR_MEDIASERVERS_A_LIBRARY_IDS_0=x',
      'CORSARR_METADATA_TMDB_LANGUAGE=en', 'CORSARR_CHATBOTS_TG_TYPE=telegram',
    ].join('\n'));
    expect(v.instances.map((i) => `${i.section}.${i.key}:${i.plugin}`)).toEqual(['sources.a:s', 'mediaServers.a:jellyfin', 'metadata.tmdb:', 'chatbots.tg:telegram']);
    expect(v.instances[1].values).toEqual({ library: { ids_0: 'x' } });
    expect(v.instances[2].values).toEqual({ language: 'en' });
  });
});

describe('settingsFor', () => {
  it('opens general settings for folders, disks and config, plugins for the rest', () => {
    expect(settingsFor('rootFolder')).toBe('#/settings/general');
    expect(settingsFor('disk')).toBe('#/settings/general');
    expect(settingsFor('config')).toBe('#/settings/general');
    expect(settingsFor('indexer')).toBe('#/settings/plugins');
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
