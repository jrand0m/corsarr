// Deterministic synthetic library rows for DataTable and PosterGrid stories and tests.
import type { MediaKind } from '../tokens';

export interface LibraryRow {
  id: string;
  title: string;
  kind: MediaKind;
  year: number;
  status: 'cutoff-met' | 'cutoff-unmet' | 'missing' | 'queued' | 'not-available';
  profile: string;
  size: number;
  have: number;
  total: number;
  added: number;
}

const first = ['The Long', 'Quay', 'Night', 'Northern', 'Salt', 'Iron', 'Harbour', 'Slow', 'Pilot', 'Low', 'Grey', 'Red'];
const second = ['Haul', 'Lights', 'Shift', 'Crossing', 'Road', 'Tide', 'Watch', 'Current', 'Signal', 'Water', 'Line', 'Manifest'];
const kinds: MediaKind[] = ['tv', 'movie', 'music', 'book'];
const statuses: LibraryRow['status'][] = ['cutoff-met', 'cutoff-met', 'cutoff-met', 'cutoff-unmet', 'missing', 'queued', 'not-available'];
const profiles = ['HD', 'UHD', 'Any', 'Lossless'];
export const libraryNow = Date.parse('2026-11-22T14:05:00Z');

export function libraryRows(n: number, offset = 0): LibraryRow[] {
  return Array.from({ length: n }, (_, k) => {
    const i = k + offset;
    const total = 6 + ((i * 7) % 18);
    const status = statuses[(i * 5) % statuses.length];
    return {
      id: `item-${i}`,
      title: `${first[i % first.length]} ${second[(i * 7 + Math.floor(i / 12)) % second.length]}${i >= 144 ? ` ${Math.floor(i / 144) + 1}` : ''}`,
      kind: kinds[(i * 3) % 4],
      year: 1994 + ((i * 11) % 33),
      status,
      profile: profiles[i % profiles.length],
      size: status === 'missing' || status === 'not-available' ? 0 : 1_200_000_000 + ((i * 7_919_000_000) % 96_000_000_000),
      have: status === 'missing' ? 0 : status === 'cutoff-met' ? total : Math.max(0, total - 1 - (i % 3)),
      total,
      added: libraryNow - ((i * 3_600_000 * 7) % (86_400_000 * 400)),
    };
  });
}
