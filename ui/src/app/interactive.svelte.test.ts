import { describe, expect, it } from 'vitest';
import { ApiError, type Client } from './api/client';
import type { Decided, ReleaseLine } from './api/types';
import { grabFailure, InteractiveSearch, rank, toPanel } from './interactive.svelte';

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
