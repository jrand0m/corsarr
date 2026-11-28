import { describe, expect, it } from 'vitest';
import type { QualityProfile } from './api/types';
import { defaultProfile, profileName, profilesFor } from './context.svelte';

const profile = (o: Partial<QualityProfile>): QualityProfile => ({ key: 'any', name: 'Any', cutoff: 'hd', upgrades: true, ...o });
const profiles = [
  profile({ key: 'any', name: 'Any' }),
  profile({ key: 'hd1080p', name: 'HD-1080p', kinds: ['movie', 'tv'], default: ['movie'] }),
  profile({ key: 'lossless', name: 'Lossless', kinds: ['music'] }),
  profile({ key: 'webdl', name: 'WEB-DL', kinds: ['tv'], isDefault: true }),
];

describe('quality profiles', () => {
  it('names a profile by key, falling back to the key', () => {
    expect(profileName(profiles, 'hd1080p')).toBe('HD-1080p');
    expect(profileName(profiles, 'gone')).toBe('gone');
    expect(profileName(undefined, 'hd1080p')).toBe('hd1080p');
  });

  it('lists the profiles a kind may use in order', () => {
    expect(profilesFor(profiles, 'movie').map((p) => p.key)).toEqual(['any', 'hd1080p']);
    expect(profilesFor(profiles, 'music').map((p) => p.key)).toEqual(['any', 'lossless']);
    expect(profilesFor(undefined, 'movie')).toEqual([]);
  });

  it("picks the kind's default, else the overall default, else the first", () => {
    expect(defaultProfile(profiles, 'movie')?.key).toBe('hd1080p');
    expect(defaultProfile(profiles, 'tv')?.key).toBe('webdl');
    expect(defaultProfile(profiles, 'music')?.key).toBe('any');
    expect(defaultProfile(undefined, 'movie')).toBeUndefined();
  });
});
