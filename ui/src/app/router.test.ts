import { describe, expect, it } from 'vitest';
import { isAccess, resolve } from './router';
import { areaLoaders } from './areas';

describe('resolve', () => {
  it('maps the MVP routes to areas and screens', () => {
    expect(resolve('/')).toEqual({ area: 'dashboard', screen: 'dashboard', params: {} });
    expect(resolve('/setup')).toEqual({ area: 'access', screen: 'setup', params: {} });
    expect(resolve('/login')).toEqual({ area: 'access', screen: 'login', params: {} });
    expect(resolve('/library/movies')).toMatchObject({ area: 'library', screen: 'movies' });
    expect(resolve('/library/movies/add')).toMatchObject({ area: 'library', screen: 'add' });
    expect(resolve('/library/tv')).toMatchObject({ area: 'library', screen: 'series' });
    expect(resolve('/library/tv/add')).toMatchObject({ area: 'library', screen: 'addSeries' });
    expect(resolve('/wanted/missing')).toMatchObject({ area: 'wanted', screen: 'missing' });
    expect(resolve('/wanted/missing/tv')).toMatchObject({ area: 'wanted', screen: 'missingTv' });
    expect(resolve('/wanted/missing/music')).toMatchObject({ area: 'wanted', screen: 'missingMusic' });
    expect(resolve('/activity/queue')).toMatchObject({ area: 'activity', screen: 'queue' });
    expect(resolve('/activity/history')).toMatchObject({ area: 'activity', screen: 'history' });
    expect(resolve('/activity/blocklist')).toMatchObject({ area: 'activity', screen: 'blocklist' });
    expect(resolve('/system/health')).toMatchObject({ area: 'system', screen: 'health' });
    expect(resolve('/system/tasks')).toMatchObject({ area: 'system', screen: 'tasks' });
    expect(resolve('/system/about')).toMatchObject({ area: 'system', screen: 'about' });
    expect(resolve('/settings/general')).toMatchObject({ area: 'settings', screen: 'general' });
    expect(resolve('/settings/plugins')).toMatchObject({ area: 'settings', screen: 'plugins' });
  });

  it('reads path params and tolerates a trailing slash', () => {
    expect(resolve('/library/movies/42')).toEqual({ area: 'library', screen: 'movie', params: { id: '42' } });
    expect(resolve('/library/movies/42/')).toEqual({ area: 'library', screen: 'movie', params: { id: '42' } });
    expect(resolve('/library/tv/7')).toEqual({ area: 'library', screen: 'show', params: { id: '7' } });
    expect(resolve('/library/music')).toMatchObject({ area: 'library', screen: 'artists' });
    expect(resolve('/library/music/add')).toMatchObject({ area: 'library', screen: 'addArtist' });
    expect(resolve('/library/music/3')).toEqual({ area: 'library', screen: 'artist', params: { id: '3' } });
    expect(resolve('/library/music/albums/9')).toEqual({ area: 'library', screen: 'album', params: { id: '9' } });
    expect(resolve('/settings/plugins/indexers.knaben')).toEqual({ area: 'settings', screen: 'plugins', params: { id: 'indexers.knaben' } });
  });

  it('redirects area roots to their first screen', () => {
    expect(resolve('/library')).toEqual({ redirect: '/library/movies' });
    expect(resolve('/settings')).toEqual({ redirect: '/settings/general' });
    expect(resolve('/system')).toEqual({ redirect: '/system/health' });
    expect(resolve('/wanted')).toEqual({ redirect: '/wanted/missing' });
    expect(resolve('/activity')).toEqual({ redirect: '/activity/queue' });
  });

  it('marks known later screens and unknown paths', () => {
    expect(resolve('/wanted/calendar')).toEqual({ later: '/wanted/calendar' });
    expect(resolve('/library/books')).toEqual({ later: '/library/books' });
    expect(resolve('/search')).toEqual({ later: '/search' });
    expect(resolve('/nowhere')).toBeNull();
  });

  it('knows the access screens', () => {
    expect(isAccess('/login')).toBe(true);
    expect(isAccess('/setup')).toBe(true);
    expect(isAccess('/')).toBe(false);
  });
});

describe('areaLoaders', () => {
  it('loads every area as a module with the screens the table names', async () => {
    const screens: Record<string, string[]> = {
      access: ['setup', 'login'], dashboard: ['dashboard'], library: ['movies', 'add', 'movie', 'series', 'addSeries', 'show'], wanted: ['missing', 'missingTv'],
      activity: ['queue', 'history', 'blocklist'], system: ['health', 'tasks', 'migration', 'about'], settings: ['general', 'plugins'],
    };
    for (const [area, names] of Object.entries(screens)) {
      const mod = await areaLoaders[area as keyof typeof areaLoaders]();
      for (const n of names) expect(mod.screens[n], `${area}/${n}`).toBeTypeOf('function');
    }
  }, 30_000); // cold-transforms every screen of the app; slow under a full parallel run
});
