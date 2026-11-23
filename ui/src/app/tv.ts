// Pure TV helpers for the series screens: episode codes, season names, the per-episode state
// the bay grid and badges show, episode-range summaries for queue and history rows, and the
// monitoring mode wording. No fetch, no DOM.
import type { BayState } from '../lib/components/atoms/BayGrid/BayGrid.svelte';
import type { Download, Episode, MonitorMode, SeriesStatus, SeriesSummary } from './api/types';

const pad = (n: number) => String(n).padStart(2, '0');

/** "S01E03". */
export const episodeCode = (season: number, number: number) => `S${pad(season)}E${pad(number)}`;

/** "Season 2", or "Specials" for season 0. */
export const seasonName = (n: number) => (n === 0 ? 'Specials' : `Season ${n}`);

/** "1 episode", "10 episodes". */
export const count = (n: number, one: string, many = `${one}s`) => `${n} ${n === 1 ? one : many}`;

export const seriesStatusLabel: Record<SeriesStatus, string> = { continuing: 'Continuing', ended: 'Ended', upcoming: 'Upcoming', '': 'Unknown' };

/** Episodes a live download covers, with that download (tv links point at episodes). */
export function queuedEpisodes(downloads: Download[]): Map<number, Download> {
  const out = new Map<number, Download>();
  for (const d of downloads) {
    if (['imported', 'removed', 'failed'].includes(d.state)) continue;
    for (const l of d.links ?? []) if (l.item.kind === 'tv' && !out.has(l.item.id)) out.set(l.item.id, d);
  }
  return out;
}

/** The bay-grid state of an episode: the file wins, then a live download, then air date and monitoring. */
export function episodeState(ep: Episode, queued: ReadonlyMap<number, Pick<Download, 'state'>>): BayState {
  if (ep.file || ep.fileId || ep.status === 'downloaded') return ep.file && !ep.file.cutoffMet ? 'upgrade' : 'have';
  const d = queued.get(ep.id);
  if (d) return d.state === 'downloading' ? 'downloading' : 'queued';
  if (ep.status === 'notAired') return 'unaired';
  if (!ep.monitored || ep.status === 'unmonitored') return 'unmonitored';
  return 'missing';
}

/** The shared item status badge value for a bay state. */
export const itemStatusOf: Record<BayState, string> = {
  have: 'cutoff-met', upgrade: 'cutoff-unmet', downloading: 'queued', queued: 'queued', missing: 'missing', blocked: 'missing',
  unaired: 'not-available', unmonitored: 'unmonitored',
};

/** The library status of a whole series: all monitored aired episodes on disk, some missing, or nothing aired yet. */
export function seriesItemStatus(s: Pick<SeriesSummary, 'stats'>): 'cutoff-met' | 'missing' | 'not-available' {
  const st = s.stats;
  if (!st || !st.episodeCount) return st?.episodeFileCount ? 'cutoff-met' : 'not-available';
  return st.episodeFileCount >= st.episodeCount ? 'cutoff-met' : 'missing';
}

export interface EpisodeRef { season: number; number: number }

/**
 * One line for a set of episodes of one series: "Example Show S01E03" for one,
 * "Example Show · S01 · 10 episodes" within a season, "Example Show · 12 episodes in 2 seasons" across.
 */
export function episodeRange(series: string, eps: EpisodeRef[]): string {
  if (!eps.length) return series;
  if (eps.length === 1) return `${series} ${episodeCode(eps[0].season, eps[0].number)}`.trim();
  const seasons = [...new Set(eps.map((e) => e.season))].sort((a, b) => a - b);
  const head = series ? `${series} · ` : '';
  if (seasons.length === 1) return `${head}S${pad(seasons[0])} · ${count(eps.length, 'episode')}`;
  return `${head}${count(eps.length, 'episode')} in ${count(seasons.length, 'season')}`;
}

const codeRe = /\bS(\d{1,3})E(\d{1,4})\b/i;

/** Splits "Example Show S01E03" into the series name and the episode (when the title carries a code). */
export function parseEpisodeTitle(title: string, series?: string): { series: string; ep?: EpisodeRef } {
  const m = codeRe.exec(title);
  const name = series ?? (m ? title.slice(0, m.index) : title).replace(/[\s·-]+$/, '').trim();
  return { series: name, ep: m ? { season: Number(m[1]), number: Number(m[2]) } : undefined };
}

/** Summary of event items that are episodes: grouped into one line per series. */
export function summariseEpisodes(items: { title: string; series?: string }[]): string {
  if (items.length <= 1) return items[0]?.title ?? '';
  const bySeries = new Map<string, EpisodeRef[]>();
  const loose: string[] = [];
  for (const i of items) {
    const p = parseEpisodeTitle(i.title, i.series);
    if (!p.ep) { loose.push(i.title); continue; }
    bySeries.set(p.series, [...(bySeries.get(p.series) ?? []), p.ep]);
  }
  return [...[...bySeries].map(([s, eps]) => episodeRange(s, eps)), ...loose].join(', ');
}

export interface ModeOption { value: MonitorMode; label: string; description: string }

/** Monitoring modes, applied once to the episodes that exist now. */
export const monitorModes: ModeOption[] = [
  { value: 'all', label: 'All episodes', description: 'Every episode except specials.' },
  { value: 'future', label: 'Future episodes', description: 'Only episodes that have not aired yet.' },
  { value: 'missing', label: 'Missing episodes', description: 'Episodes without a file, plus future ones.' },
  { value: 'existing', label: 'Existing episodes', description: 'Only episodes that already have a file.' },
  { value: 'firstSeason', label: 'First season', description: 'Every episode of season 1.' },
  { value: 'latestSeason', label: 'Latest season', description: 'Every episode of the newest season.' },
  { value: 'pilot', label: 'Pilot only', description: 'Just the first episode.' },
  { value: 'none', label: 'Nothing', description: 'Add the series without monitoring any episode.' },
];

/** Seasons newest first, specials last. */
export const seasonOrder = (a: { number: number }, b: { number: number }) =>
  a.number === 0 ? 1 : b.number === 0 ? -1 : b.number - a.number;
