// Pure music helpers for the artist and album screens: album type names, the item status an
// album or artist shows, which albums a list shows by default (inside the metadata profile, or
// kept because they are monitored or have files), monitoring options and track positions.
// No fetch, no DOM.
import type { AlbumMonitor, AlbumType, AlbumView, ArtistStats, MonitorNewItems, Track } from './api/types';

export const albumTypeLabel: Record<AlbumType, string> = { album: 'Album', ep: 'EP', single: 'Single', broadcast: 'Broadcast', other: 'Other' };

const typeOrder: AlbumType[] = ['album', 'ep', 'single', 'broadcast', 'other'];

/** The year an album first came out, or 0 when unknown. */
export const albumYear = (a: Pick<AlbumView, 'releaseDate'>) => (a.releaseDate ? new Date(a.releaseDate).getUTCFullYear() : 0);

/** "Album · Live, Compilation": the primary type plus any secondary types. */
export function albumTypes(a: Pick<AlbumView, 'primaryType' | 'secondaryTypes'>): string {
  const sec = (a.secondaryTypes ?? []).map((s) => s.charAt(0).toUpperCase() + s.slice(1));
  return [albumTypeLabel[a.primaryType] ?? a.primaryType, sec.join(', ')].filter(Boolean).join(' · ');
}

/** The shared item status of an album. */
export function albumItemStatus(a: Pick<AlbumView, 'status' | 'cutoffMet' | 'inProfile' | 'monitored'>): string {
  switch (a.status) {
    case 'downloaded':
      return a.cutoffMet ? 'cutoff-met' : 'cutoff-unmet';
    case 'notReleased':
      return 'not-available';
    case 'unmonitored':
      return a.inProfile ? 'unmonitored' : 'outside-profile';
  }
  return 'missing'; // missing, or partly on disk
}

/** The library status of a whole artist: every monitored album has files, some miss, or nothing is monitored. */
export function artistItemStatus(st: Pick<ArtistStats, 'monitoredAlbums' | 'albumsWithFiles'> | undefined): 'cutoff-met' | 'missing' | 'unmonitored' {
  if (!st || !st.monitoredAlbums) return st?.albumsWithFiles ? 'cutoff-met' : 'unmonitored';
  return st.albumsWithFiles >= st.monitoredAlbums ? 'cutoff-met' : 'missing';
}

/** Albums a list shows: inside the profile or kept for another reason, unless everything is asked for. */
export const shownAlbums = <T extends Pick<AlbumView, 'inProfile' | 'monitored' | 'fileCount'>>(albums: T[], all: boolean): T[] =>
  all ? albums : albums.filter((a) => a.inProfile || a.monitored || a.fileCount > 0);

/** Albums ordered by type (albums first), then newest first, undated last. */
export function albumOrder(a: Pick<AlbumView, 'primaryType' | 'releaseDate'>, b: Pick<AlbumView, 'primaryType' | 'releaseDate'>): number {
  const t = typeOrder.indexOf(a.primaryType) - typeOrder.indexOf(b.primaryType);
  if (t) return t;
  if (!a.releaseDate || !b.releaseDate) return a.releaseDate ? -1 : b.releaseDate ? 1 : 0;
  return b.releaseDate.localeCompare(a.releaseDate);
}

/** "1-03" on a multi-disc edition, "03" otherwise. */
export function trackPosition(t: Pick<Track, 'medium' | 'number'>, mediums: number): string {
  const n = String(t.number).padStart(2, '0');
  return mediums > 1 ? `${t.medium}-${n}` : n;
}

/** "3:21", or "1:02:05" past an hour; "—" when unknown. */
export function duration(seconds: number): string {
  if (!seconds) return '—';
  const h = Math.floor(seconds / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  const s = String(seconds % 60).padStart(2, '0');
  return h ? `${h}:${String(m).padStart(2, '0')}:${s}` : `${m}:${s}`;
}

export interface Option<T extends string> { value: T; label: string; description: string }

/** Monitoring choices applied once when an artist is added. */
export const albumMonitorModes: Option<AlbumMonitor>[] = [
  { value: 'all', label: 'All albums', description: 'Every album inside the metadata profile.' },
  { value: 'future', label: 'Future albums', description: 'Only albums that have not come out yet.' },
  { value: 'latest', label: 'Latest album', description: 'The newest album that is out.' },
  { value: 'first', label: 'First album', description: 'The earliest album.' },
  { value: 'none', label: 'Nothing', description: 'Add the artist without monitoring any album.' },
];

/** What happens to albums that appear later. */
export const monitorNewOptions: Option<MonitorNewItems>[] = [
  { value: 'all', label: 'Monitor every new album', description: 'Albums found later are monitored when the metadata profile covers them.' },
  { value: 'new', label: 'Only albums released from now on', description: 'Older albums found later stay unmonitored.' },
  { value: 'none', label: 'Leave new albums unmonitored', description: 'Nothing found later is searched until you monitor it.' },
];
