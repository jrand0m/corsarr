import { describe, expect, it } from 'vitest';
import { albumItemStatus, albumOrder, albumTypes, albumYear, artistItemStatus, duration, shownAlbums, trackPosition } from './music';

describe('music helpers', () => {
  it('names album types and years', () => {
    expect(albumTypes({ primaryType: 'album', secondaryTypes: ['live', 'compilation'] })).toBe('Album · Live, Compilation');
    expect(albumTypes({ primaryType: 'ep', secondaryTypes: null })).toBe('EP');
    expect(albumYear({ releaseDate: '2020-03-01T00:00:00Z' })).toBe(2020);
    expect(albumYear({ releaseDate: null })).toBe(0);
  });

  it('maps album states onto the shared item status', () => {
    const base = { cutoffMet: true, inProfile: true, monitored: true };
    expect(albumItemStatus({ ...base, status: 'downloaded' })).toBe('cutoff-met');
    expect(albumItemStatus({ ...base, status: 'downloaded', cutoffMet: false })).toBe('cutoff-unmet');
    expect(albumItemStatus({ ...base, status: 'partial' })).toBe('missing');
    expect(albumItemStatus({ ...base, status: 'notReleased' })).toBe('not-available');
    expect(albumItemStatus({ ...base, status: 'unmonitored', monitored: false })).toBe('unmonitored');
    expect(albumItemStatus({ ...base, status: 'unmonitored', inProfile: false })).toBe('outside-profile');
  });

  it('summarises an artist', () => {
    expect(artistItemStatus({ monitoredAlbums: 2, albumsWithFiles: 2 })).toBe('cutoff-met');
    expect(artistItemStatus({ monitoredAlbums: 2, albumsWithFiles: 1 })).toBe('missing');
    expect(artistItemStatus({ monitoredAlbums: 0, albumsWithFiles: 0 })).toBe('unmonitored');
    expect(artistItemStatus({ monitoredAlbums: 0, albumsWithFiles: 1 })).toBe('cutoff-met');
    expect(artistItemStatus(undefined)).toBe('unmonitored');
  });

  it('hides albums outside the profile unless kept or asked for', () => {
    const albums = [
      { id: 1, inProfile: true, monitored: false, fileCount: 0 },
      { id: 2, inProfile: false, monitored: true, fileCount: 0 },
      { id: 3, inProfile: false, monitored: false, fileCount: 2 },
      { id: 4, inProfile: false, monitored: false, fileCount: 0 },
    ];
    expect(shownAlbums(albums, false).map((a) => a.id)).toEqual([1, 2, 3]);
    expect(shownAlbums(albums, true)).toHaveLength(4);
  });

  it('orders albums by type, newest first, undated last', () => {
    const list = [
      { primaryType: 'single' as const, releaseDate: '2024-01-01' },
      { primaryType: 'album' as const, releaseDate: null },
      { primaryType: 'album' as const, releaseDate: '2019-01-01' },
      { primaryType: 'album' as const, releaseDate: '2022-01-01' },
    ];
    expect(list.slice().sort(albumOrder).map((a) => `${a.primaryType}:${a.releaseDate}`)).toEqual([
      'album:2022-01-01', 'album:2019-01-01', 'album:null', 'single:2024-01-01',
    ]);
    expect(albumOrder({ primaryType: 'ep', releaseDate: null }, { primaryType: 'ep', releaseDate: null })).toBe(0);
  });

  it('formats positions and durations', () => {
    expect(trackPosition({ medium: 2, number: 3 }, 2)).toBe('2-03');
    expect(trackPosition({ medium: 1, number: 12 }, 1)).toBe('12');
    expect(duration(201)).toBe('3:21');
    expect(duration(3725)).toBe('1:02:05');
    expect(duration(0)).toBe('—');
  });
});
