// Deterministic music fixtures for the Pages/* music stories. Same fixed "now" as fixtures.ts
// (22 Nov 2026, 14:05 UTC). Not a design-system component.
import type { Decision } from '../components/organisms/DecisionPanel/DecisionPanel.svelte';
import { NOW } from './fixtures';

const day = 86_400_000;

export interface ArtistRow {
  id: string; name: string; disambiguation?: string; profile: string; metadata: string;
  withFiles: number; monitored: number; size: number; nextRelease?: number; watched: boolean;
}

const names = ['Harbour Lights', 'The Gantry Choir', 'Low Tide Orchestra', 'Salt & Signal', 'Berth Seven', 'Fog Horn Society',
  'Northern Quay', 'Tidewater Trio', 'Crane Hands', 'Breakwater'];

export const artistRows: ArtistRow[] = names.map((name, i) => {
  const monitored = 2 + ((i * 3) % 9);
  const withFiles = i % 3 === 0 ? monitored : Math.max(0, monitored - ((i * 2) % 4));
  return {
    id: `a${i}`, name, disambiguation: i === 4 ? 'the dub band' : undefined, profile: i % 3 === 0 ? 'Lossless' : 'Standard',
    metadata: i % 4 === 0 ? 'Standard + Singles' : 'Standard', withFiles, monitored, size: withFiles * 420_000_000,
    nextRelease: i % 5 === 1 ? NOW + (i + 3) * day : undefined, watched: i % 7 !== 6,
  };
});

export interface AlbumRow {
  id: string; title: string; year: number; type: string; tracks: number; files: number; status: string; quality?: string;
  size: number; monitored: boolean; inProfile: boolean;
}

export const lightsAlbums: AlbumRow[] = [
  { id: 'b1', title: 'Low Tide', year: 2025, type: 'Album', tracks: 11, files: 11, status: 'cutoff-met', quality: 'FLAC', size: 312e6, monitored: true, inProfile: true },
  { id: 'b2', title: 'Signal Fires', year: 2022, type: 'Album', tracks: 12, files: 7, status: 'missing', quality: 'MP3-320', size: 98e6, monitored: true, inProfile: true },
  { id: 'b3', title: 'Breakwater Sessions', year: 2019, type: 'Album', tracks: 9, files: 9, status: 'cutoff-unmet', quality: 'MP3-320', size: 120e6, monitored: true, inProfile: true },
  { id: 'b4', title: 'Harbour Lights', year: 2016, type: 'Album', tracks: 10, files: 0, status: 'missing', size: 0, monitored: true, inProfile: true },
  { id: 'b5', title: 'Live at Berth Seven', year: 2021, type: 'Album · Live', tracks: 14, files: 14, status: 'cutoff-met', quality: 'FLAC', size: 410e6, monitored: true, inProfile: false },
  { id: 'b6', title: 'Night Ferry', year: 2026, type: 'Album', tracks: 0, files: 0, status: 'not-available', size: 0, monitored: true, inProfile: true },
  { id: 'b7', title: 'Fog Horn (Remixes)', year: 2023, type: 'EP · Remix', tracks: 4, files: 0, status: 'outside-profile', size: 0, monitored: false, inProfile: false },
];

export interface TrackRow { id: string; pos: string; title: string; length: string; file?: string }

export const lowTideTracks: TrackRow[] = [
  ['Ebb', '3:41'], ['Mooring Lines', '4:12'], ['Slack Water', '2:58'], ['The Lighthouse Keeper', '5:30'], ['Shingle', '3:05'],
  ['Neap', '4:44'], ['Brine', '3:19'], ['Leeward', '4:01'], ['Spring Tide', '6:12'], ['Tidal Bore', '3:33'], ['Flood', '7:02'],
].map(([title, length], i) => ({ id: `t${i}`, pos: String(i + 1).padStart(2, '0'), title, length, file: i < 9 ? 'FLAC' : undefined }));

const release = (title: string, indexer: string, protocol: Decision['release']['protocol'], size: number, quality: string, seeders?: number) =>
  ({ title, indexer, protocol, size, publishedAt: NOW - 3 * day, seeders, quality, runtimeSeconds: 2640 });

export const lowTideDecisions: Decision[] = [
  { release: release('Harbour Lights - Low Tide (2025) [FLAC]', 'TrubadurCamp', 'source', 312e6, 'FLAC'), verdict: 'accept', rank: 1, candidates: 2,
    score: { total: 0, formats: [] }, reasons: [] },
  { release: release('Harbour_Lights-Low_Tide-WEB-2025-QUAY', 'Harbour Tracker', 'torrent', 305e6, 'FLAC', 42), verdict: 'accept', rank: 2, candidates: 2,
    score: { total: 0, formats: [] }, reasons: [] },
  { release: release('Harbour Lights - Low Tide (2025) [MP3 320kbps]', 'Harbour Tracker', 'torrent', 101e6, 'MP3-320', 120), verdict: 'reject',
    score: { total: 0, formats: [] }, reasons: [{ rule: 'quality.not_allowed', category: 'permanent', message: 'quality MP3-320 is not allowed by profile "Lossless"' }] },
  { release: release('Harbour Lights - Discography 2016-2025 [FLAC]', 'Harbour Tracker', 'torrent', 2.1e9, 'FLAC', 8), verdict: 'reject',
    score: { total: 0, formats: [] }, reasons: [{ rule: 'music.discography', category: 'preference', message: 'a discography covers more than this album' }] },
];
