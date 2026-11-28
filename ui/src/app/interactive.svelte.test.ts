import { describe, expect, it } from 'vitest';
import { ApiError, type Client } from './api/client';
import type { Decided, ReleaseLine } from './api/types';
import { grabFailure, InteractiveSearch, nothingAcceptedNote, outcomeFor, rank, readSort, sortReleases, toPanel, writeSort, type SortKey } from './interactive.svelte';

const decided = (id: string, accepted: boolean, rankKey: string, protocol: 'torrent' | 'source' = 'torrent'): Decided => ({
  release: { id, title: id, protocol, size: 1, publishedAt: '2026-01-01T00:00:00Z', indexers: [{ key: 'ix', name: 'Indexer' }], seeders: 9, peers: 0 },
  decision: { releaseId: id, accepted, reasons: accepted ? [] : [{ ruleId: 'blocklist', category: 'permanent', message: 'blocked' }], quality: 'FLAC',
    resolution: 0, formatScore: 0, rankKey, override: accepted ? 'allowed' : 'confirm' },
});

function fakeClient(lines: ReleaseLine[], fail?: Error): Client {
  return {
    async *stream<T>() {
      for (const l of lines) yield l as T;
      if (fail) throw fail;
    },
  } as unknown as Client;
}

describe('interactive search', () => {
  it('ranks accepted releases first, best rank key first', () => {
    const out = rank([decided('a', false, '9'), decided('b', true, '1'), decided('c', true, '5')]);
    expect(out.map((d) => d.release.id)).toEqual(['c', 'b', 'a']);
  });

  it('turns decisions into panel decisions', () => {
    const p = toPanel(decided('x', false, '1', 'source'), 0, 2, 600);
    expect(p.verdict).toBe('reject');
    expect(p.rank).toBeUndefined();
    expect(p.release.protocol).toBe('source');
    expect(p.release.seeders).toBeUndefined();
    expect(p.reasons[0].blocklist).toBe(true);
    expect(toPanel(decided('y', true, '1'), 1, 2).rank).toBe(2);
  });

  it('fills gaps in a release with plain fallbacks', () => {
    const bare = decided('z', true, '1');
    bare.release.indexers = null;
    bare.decision.quality = '';
    bare.decision.reasons = null;
    const p = toPanel(bare, 0, 3);
    expect(p.release).toMatchObject({ indexer: 'unknown indexer', quality: 'unknown', seeders: 9, runtimeSeconds: undefined, swarm: undefined, availability: undefined });
    expect(p).toMatchObject({ verdict: 'accept', rank: 1, candidates: 3, reasons: [], automation: { outcome: 'unavailable', lostTo: undefined } });
  });

  it('names indexers by name or key and passes swarm, availability and desirability through', () => {
    const d = decided('w', false, '1');
    d.release.indexers = [{ key: 'one', name: '' }, { key: 'two', name: 'Two' }];
    d.swarm = { seeders: 4, reported: true, health: 'good' };
    d.availability = { status: 'complete' };
    d.decision.desirability = { score: 1 } as unknown as NonNullable<typeof d.decision.desirability>;
    const p = toPanel(d, 0, 1, { streaming: true, runtimeSeconds: 1200 });
    expect(p.release).toMatchObject({ indexer: 'one, Two', runtimeSeconds: 1200, swarm: d.swarm, availability: d.availability });
    expect(p).toMatchObject({ verdict: 'reject', rank: undefined, candidates: undefined, automation: { outcome: 'declined' } });
    expect(p.desirability).toBe(d.decision.desirability);
  });

  it('streams records, errors and the summary', async () => {
    const s = new InteractiveSearch(fakeClient([
      { record: decided('a', true, '2') }, { record: decided('b', true, '3') },
      { error: { source: 'down', message: 'timeout' } },
      { summary: { done: true, count: 2, errors: 1, indexers: 2, failed: 1, releases: 2, accepted: 2 } },
    ]));
    await s.start('albums/1/releases');
    expect(s.ranked[0].release.id).toBe('b');
    expect(s.accepted).toBe(2);
    expect(s.sources).toEqual([{ id: 'ix', name: 'Indexer', state: 'ok', count: 2 }, { id: 'down', name: 'down', state: 'failed', reason: 'timeout', count: undefined }]);
    expect(s.summary?.failed).toBe(1);
    expect(s.streaming).toBe(false);
  });

  it('keeps the stream error', async () => {
    const s = new InteractiveSearch(fakeClient([], new ApiError(503, { code: 'kind_mode', message: 'off' })));
    await s.start('albums/1/releases');
    expect(s.error?.code).toBe('kind_mode');
    s.stop();
  });

  it('reads grab failures', () => {
    const confirm = grabFailure(new ApiError(409, { code: 'confirm_required', message: 'confirm' }, { decision: { reasons: [{ message: 'blocked' }] } }));
    expect(confirm).toEqual({ confirm: true, refused: false, message: 'confirm', reasons: ['blocked'] });
    expect(grabFailure(new ApiError(403, { code: 'refused', message: 'no' })).refused).toBe(true);
    expect(grabFailure(new ApiError(404, { code: 'not_found', message: 'gone' })).message).toContain('expired');
    expect(grabFailure(new ApiError(409, { code: 'shadowed', message: 'shadow' })).confirm).toBe(false);
  });
});

describe('sorting and automation outcome', () => {
  const des = (id: string, score: number | undefined, position: 'above' | 'on_target' | 'below', rankKey: string, extra: Partial<Decided['release']> = {}): Decided => {
    const d = decided(id, true, rankKey);
    return { ...d, release: { ...d.release, ...extra }, decision: { ...d.decision, desirability: score === undefined ? undefined : { score, position, qualitySteps: position === 'above' ? 1 : position === 'below' ? -1 : 0, cutoff: 'X' } } };
  };
  const list = [
    des('below75', 75, 'below', '5', { seeders: 3, size: 3, publishedAt: '2026-01-03T00:00:00Z' }),
    des('outside', undefined, 'below', '9', { seeders: 50, size: 9 }),
    des('above75', 75, 'above', '4', { seeders: 1, size: 5 }),
    des('on', 100, 'on_target', '1', { seeders: 7, size: 4, publishedAt: '2026-01-02T00:00:00Z' }),
  ];
  const ids = (key: SortKey) => sortReleases(list, key).map((d) => d.release.id);

  it('orders by every key, ties in automation order', () => {
    expect(ids('automation')).toEqual(['outside', 'below75', 'above75', 'on']);
    expect(ids('desirability')).toEqual(['on', 'above75', 'below75', 'outside']);
    expect(ids('seeders')).toEqual(['outside', 'on', 'below75', 'above75']);
    expect(ids('size')).toEqual(['outside', 'above75', 'on', 'below75']);
    expect(ids('newest')).toEqual(['below75', 'on', 'outside', 'above75']);
    expect(ids('quality')).toEqual(['above75', 'on', 'below75', 'outside']);
  });

  it('reads the sort from the URL, defaulting to automation', () => {
    expect(readSort(new URLSearchParams('sort=desirability'))).toBe('desirability');
    expect(readSort(new URLSearchParams('sort=bogus'))).toBe('automation');
    expect(readSort(new URLSearchParams(''))).toBe('automation');
  });

  it('keeps the sort in the URL hash, leaving out the default', () => {
    location.hash = '#/library/movies/1?tab=files';
    writeSort('seeders');
    expect(location.hash).toBe('#/library/movies/1?tab=files&sort=seeders');
    writeSort('automation');
    expect(location.hash).toBe('#/library/movies/1?tab=files');
  });

  it('names the automation outcome: pending, from the verdict, or unavailable', () => {
    const a = decided('a', true, '2');
    const r = decided('r', false, '1');
    const verdict = { mode: 'active' as const, pick: 'a', outcomes: { a: { status: 'grab' as const } } };
    expect(outcomeFor(a, undefined, true)).toBe('pending');
    expect(outcomeFor(r, undefined, true)).toBe('declined');
    expect(outcomeFor(a, verdict, false)).toBe('grab');
    expect(outcomeFor(a, undefined, false)).toBe('unavailable');
    const p = toPanel(decided('b', true, '1'), 1, 2, { verdict: { mode: 'active', pick: 'a', outcomes: { b: { status: 'outranked', lostTo: { criterion: 'quality', message: 'lower quality than the pick' } } } } });
    expect(p.automation).toEqual({ outcome: 'outranked', lostTo: 'lower quality than the pick' });
    expect(p.desirability).toBeNull();
  });

  it('explains when automation would grab nothing', () => {
    expect(nothingAcceptedNote(4, [{ message: 'too big', count: 3 }])).toContain('Automatic search would grab nothing: none of the 4 releases passed the profile. Most common reasons: too big (3).');
  });
});
