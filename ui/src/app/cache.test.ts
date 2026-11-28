import { describe, expect, it } from 'vitest';
import { cacheProperties, runText, usageText } from './cache';
import type { CacheStatus } from './api/types';

const status = (o: Partial<CacheStatus> = {}): CacheStatus => ({
  enabled: true, dir: '/data/cache', maxSize: 1024 ** 3, usage: 512 * 1024 ** 2, imageBytes: 500 * 1024 ** 2, metadataBytes: 12 * 1024 ** 2,
  images: 812, variants: 1, misses: 0, requests: 200, hits: 150, hitRate: 0.75, highWater: 0.9, lowWater: 0.8, ...o,
});

describe('cache health', () => {
  it('states usage against the cap', () => {
    expect(usageText(status())).toBe('537 MB of 1.1 GB');
  });
  it('says what a shrink did', () => {
    expect(runText({ at: '', reencoded: 120, resized: 1, evicted: 0, metadataEvicted: 2, bytesFreed: 4_200_000 }))
      .toMatch(/^Lowered the quality of 120 images, made 1 image smaller, removed 2 stored lookups; freed/);
    expect(runText({ at: '', reencoded: 0, resized: 0, evicted: 0, metadataEvicted: 0, bytesFreed: 0 })).toMatch(/^Changed nothing/);
  });
  it('lists entries, hit rate and the last run', () => {
    const rows = Object.fromEntries(cacheProperties(status()).map((p) => [p.key, p.value]));
    expect(rows.images).toBe('812 images + 1 smaller copy');
    expect(rows.hit).toBe('75% of 200 views since start');
    expect(rows.shrink).toBe('90% full, down to 80%');
    expect(rows.last).toBe('Not needed yet');
    const idle = Object.fromEntries(cacheProperties(status({ requests: 0, variants: 0, images: 1, lastRun: { at: '2026-01-01T00:00:00Z', reencoded: 1, resized: 0, evicted: 0, metadataEvicted: 0, bytesFreed: 10 } })).map((p) => [p.key, p.value]));
    expect(idle.hit).toBe('No views since start');
    expect(idle.images).toBe('1 image');
    expect(String(idle.last)).toMatch(/Lowered the quality of 1 image/);
  });
});
