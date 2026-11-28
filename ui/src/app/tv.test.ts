import { describe, expect, it } from 'vitest';
import {
  count, episodeCode, episodeRange, episodeState, parseEpisodeTitle, queuedEpisodes, seasonName, seasonOrder, seriesItemStatus, summariseEpisodes,
} from './tv';
import type { Download, Episode } from './api/types';

const ep = (o: Partial<Episode> = {}): Episode => ({
  id: 1, seriesId: 7, season: 1, number: 1, title: 'Pilot', airAt: '2026-01-01T00:00:00Z', monitored: true, inMetadata: true, status: 'missing', ...o,
});
const dl = (o: Partial<Download>): Download => ({
  id: 1, downloader: 'q', downloadId: 'x', protocol: 'torrent', kind: 'tv', grabId: 1, title: 'Example.Show.S01', state: 'downloading',
  links: [], progress: 0.4, size: 1, downloaded: 0, speed: 0, eta: 0, seeders: 0, peers: 0, category: 'tv', held: false, addedAt: '', ...o,
});

describe('labels', () => {
  it('formats codes, season names and counts', () => {
    expect(episodeCode(1, 3)).toBe('S01E03');
    expect(episodeCode(12, 104)).toBe('S12E104');
    expect(seasonName(0)).toBe('Specials');
    expect(seasonName(2)).toBe('Season 2');
    expect(count(1, 'episode')).toBe('1 episode');
    expect(count(0, 'episode')).toBe('0 episodes');
    expect(count(3, 'series', 'series')).toBe('3 series');
  });

  it('orders seasons newest first with specials last', () => {
    expect([{ number: 0 }, { number: 1 }, { number: 3 }, { number: 2 }].sort(seasonOrder).map((s) => s.number)).toEqual([3, 2, 1, 0]);
  });
});

describe('episodeState', () => {
  const none = new Map<number, Download>();
  it('prefers the file, then the queue, then air date and monitoring', () => {
    expect(episodeState(ep({ status: 'downloaded', fileId: 3, file: { path: '/a', size: 1, quality: 'WEB-1080p', cutoffMet: true } }), none)).toBe('have');
    expect(episodeState(ep({ fileId: 3, file: { path: '/a', size: 1, quality: 'HDTV-720p', cutoffMet: false } }), none)).toBe('upgrade');
    expect(episodeState(ep(), new Map([[1, dl({ state: 'downloading' })]]))).toBe('downloading');
    expect(episodeState(ep(), new Map([[1, dl({ state: 'queued' })]]))).toBe('queued');
    expect(episodeState(ep({ status: 'notAired' }), none)).toBe('unaired');
    expect(episodeState(ep({ monitored: false, status: 'unmonitored' }), none)).toBe('unmonitored');
    expect(episodeState(ep(), none)).toBe('missing');
  });

  it('counts a file id or a downloaded status as on disk, and an unmonitored episode as unmonitored whatever its status', () => {
    expect(episodeState(ep({ fileId: 3 }), none)).toBe('have');
    expect(episodeState(ep({ status: 'downloaded' }), none)).toBe('have');
    expect(episodeState(ep({ monitored: false }), none)).toBe('unmonitored');
  });

  it('collects the episodes live downloads cover', () => {
    const q = queuedEpisodes([
      dl({ id: 1, links: [{ item: { kind: 'tv', id: 10 }, status: 'pending' }, { item: { kind: 'tv', id: 11 }, status: 'pending' }] }),
      dl({ id: 2, state: 'imported', links: [{ item: { kind: 'tv', id: 12 }, status: 'imported' }] }),
      dl({ id: 3, kind: 'movie', links: [{ item: { kind: 'movie', id: 13 }, status: 'pending' }] }),
    ]);
    expect([...q.keys()]).toEqual([10, 11]);
    expect(q.get(10)?.id).toBe(1);
  });
});

describe('seriesItemStatus', () => {
  const s = (episodeCount: number, episodeFileCount: number) => ({ stats: { seasonCount: 1, episodeCount, episodeFileCount, totalEpisodeCount: 10, sizeOnDisk: 0 } });
  it('reads complete, missing and nothing aired', () => {
    expect(seriesItemStatus(s(10, 10))).toBe('cutoff-met');
    expect(seriesItemStatus(s(10, 8))).toBe('missing');
    expect(seriesItemStatus(s(0, 0))).toBe('not-available');
  });
});

describe('episode ranges', () => {
  it('names one episode, a season pack and a multi-season set', () => {
    expect(episodeRange('Example Show', [{ season: 1, number: 3 }])).toBe('Example Show S01E03');
    expect(episodeRange('Example Show', Array.from({ length: 10 }, (_, i) => ({ season: 1, number: i + 1 })))).toBe('Example Show · S01 · 10 episodes');
    expect(episodeRange('Example Show', [{ season: 1, number: 1 }, { season: 2, number: 1 }])).toBe('Example Show · 2 episodes in 2 seasons');
    expect(episodeRange('', [{ season: 2, number: 1 }, { season: 2, number: 2 }])).toBe('S02 · 2 episodes');
  });

  it('parses episode titles', () => {
    expect(parseEpisodeTitle('Example Show S01E03')).toEqual({ series: 'Example Show', ep: { season: 1, number: 3 } });
    expect(parseEpisodeTitle('Example Show')).toEqual({ series: 'Example Show', ep: undefined });
    expect(parseEpisodeTitle('Ex S02E01', 'Example Show').series).toBe('Example Show');
  });

  it('summarises event items per series', () => {
    const items = Array.from({ length: 10 }, (_, i) => ({ title: `Example Show S01E${String(i + 1).padStart(2, '0')}`, series: 'Example Show' }));
    expect(summariseEpisodes(items)).toBe('Example Show · S01 · 10 episodes');
    expect(summariseEpisodes([items[0]])).toBe('Example Show S01E01');
    expect(summariseEpisodes([])).toBe('');
    expect(summariseEpisodes([{ title: 'A S01E01' }, { title: 'B S03E02' }, { title: 'Loose' }])).toBe('A S01E01, B S03E02, Loose');
  });
});
