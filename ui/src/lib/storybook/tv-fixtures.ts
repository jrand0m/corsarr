// Deterministic TV fixtures for the Pages/* series stories (S04–S06). Same fixed "now" as
// fixtures.ts (22 Nov 2026, 14:05 UTC). Not a design-system component.
import type { BayCell, BayState } from '../components/atoms/BayGrid/BayGrid.svelte';
import { NOW } from './fixtures';

const day = 86_400_000;

export interface SeriesRow {
  id: string; title: string; year: number; network: string; status: 'continuing' | 'ended' | 'upcoming';
  profile: string; files: number; episodes: number; size: number; nextAiring?: number; monitored: boolean;
}

const titles = [
  'The Long Haul', 'Harbour Lights', 'Night Shift at Berth Seven', 'Salt Road', 'Pilot House', 'Tidewater', 'Customs and Excise',
  'Under the Gantry', 'Low Water Mark', 'Northern Signal', 'Fog Bank', 'The Tender Years', 'Breakwater', 'Crane Hands',
];
const networks = ['Harbourline', 'Quay One', 'Channel Nine Docks', 'Pier Streaming'];
const statuses: SeriesRow['status'][] = ['continuing', 'ended', 'continuing', 'ended', 'upcoming'];

export const seriesRows: SeriesRow[] = titles.map((title, i) => {
  const status = statuses[i % statuses.length];
  const episodes = status === 'upcoming' ? 0 : 8 + ((i * 7) % 40);
  const files = status === 'upcoming' ? 0 : i % 3 === 0 ? episodes : Math.max(0, episodes - ((i * 3) % 9));
  return {
    id: `s${i}`, title, year: 2008 + ((i * 5) % 18), network: networks[i % networks.length], status,
    profile: i % 4 === 0 ? 'UHD' : 'HD', files, episodes, size: files * 1_350_000_000,
    nextAiring: status === 'ended' ? undefined : NOW + ((i * 2 + 1) * day), monitored: i % 6 !== 5,
  };
});

export interface EpisodeRow {
  id: string; season: number; number: number; title: string; airAt?: number; monitored: boolean; state: BayState; quality?: string; progress?: number;
}

export interface SeasonFixture { number: number; monitored: boolean; files: number; episodes: number; size: number; rows: EpisodeRow[] }

const epTitles = [
  'Slack Water', 'The Manifest', 'Ballast', 'Night Crossing', 'Bill of Lading', 'Dry Dock', 'Pilot Ladder', 'Bunkering', 'The Gantry Crane', 'High Tide',
];

function season(number: number, states: BayState[], monitored = true): SeasonFixture {
  const rows = states.map<EpisodeRow>((state, i) => ({
    id: `e${number}-${i + 1}`, season: number, number: i + 1, title: state === 'unaired' && i > 7 ? '' : epTitles[i % epTitles.length],
    airAt: state === 'unaired' ? NOW + (i - 5) * 7 * day : NOW - ((states.length - i) * 7 + number * 120) * day,
    monitored: monitored && state !== 'unmonitored', state,
    quality: state === 'have' ? 'WEB-DL 1080p' : state === 'upgrade' ? 'HDTV 720p' : undefined,
    progress: state === 'downloading' ? 0.46 : undefined,
  }));
  const files = rows.filter((r) => r.state === 'have' || r.state === 'upgrade').length;
  return { number, monitored, files, episodes: rows.filter((r) => r.monitored && r.state !== 'unaired').length, size: files * 1_350_000_000, rows };
}

/** The Long Haul: season 3 airing (two downloading, rest not aired), 2 complete with one upgrade, 1 complete, specials unmonitored. */
export const longHaulSeasons: SeasonFixture[] = [
  season(3, ['have', 'have', 'have', 'downloading', 'queued', 'unaired', 'unaired', 'unaired', 'unaired', 'unaired']),
  season(2, ['have', 'have', 'upgrade', 'have', 'have', 'missing', 'have', 'have', 'have', 'have']),
  season(1, ['have', 'have', 'have', 'have', 'have', 'have', 'have', 'have']),
  season(0, ['unmonitored', 'unmonitored', 'unmonitored'], false),
];

export const bayCells = (s: SeasonFixture): BayCell[] =>
  s.rows.map((r) => ({ state: r.state, label: `S${String(r.season).padStart(2, '0')}E${String(r.number).padStart(2, '0')}${r.title ? ` ${r.title}` : ''}` }));
