// System › Migration: the wire shapes of a migration run and the pure helpers the screen uses
// (request body, outcome badges, progress per source, filtering the plan).
import type { FilterFacet, FilterState } from '../lib/filters/url';
import type { SourceProgress } from '../lib/components/molecules/SourceProgressStrip/SourceProgressStrip.svelte';

export type SourceApp = 'sonarr' | 'radarr' | 'lidarr' | 'prowlarr';
export const sourceApps: { app: SourceApp; label: string; note: string }[] = [
  { app: 'sonarr', label: 'Sonarr', note: 'Series, episodes, files, queue, blocklist and history' },
  { app: 'radarr', label: 'Radarr', note: 'Movies, files, queue, blocklist and history' },
  { app: 'lidarr', label: 'Lidarr', note: 'Artists, albums, track files, queue, blocklist and history' },
  { app: 'prowlarr', label: 'Prowlarr', note: 'Indexers' },
];

export type Outcome = 'create' | 'update' | 'unchanged' | 'skip' | 'approximate' | 'unsupported' | 'conflict' | 'secretNeeded'
  | 'fileMissing' | 'managedByConfig' | 'gone' | 'failed';

export interface PlanEntry {
  id: string;
  category: string;
  source: string;
  name: string;
  outcome: Outcome;
  reason: string;
  target?: string;
}

export interface SourceInfo { app: SourceApp; url: string; name?: string; version?: string; supported: string; error?: string }

export interface MigrationReport {
  runId?: number;
  mode: 'plan' | 'apply';
  sources: SourceInfo[] | null;
  entries: PlanEntry[] | null;
  counts: Partial<Record<Outcome, number>> | null;
  config?: string;
  env?: string[];
  planHash: string;
}

export interface MigrationRun {
  id: number;
  mode: 'plan' | 'apply';
  sources: string[];
  status: 'running' | 'done' | 'failed';
  counts: Partial<Record<Outcome, number>>;
  error?: string;
  startedAt: string;
  finishedAt?: string;
}

export interface KindState { kind: string; mode: 'disabled' | 'shadow' | 'active'; managedByConfig: boolean; snippet?: string }

export interface SourceForm { app: SourceApp; include: boolean; url: string; apiKey: string }

export interface RunForm {
  sources: SourceForm[];
  scope: string[];
  fromPrefix: string;
  toPrefix: string;
  historyDays: number | null;
  activate: boolean;
  exclude: string[];
}

export const categories: { value: string; label: string; description: string }[] = [
  { value: 'library', label: 'Library', description: 'Series, episodes, movies, artists and albums with monitoring; files adopted where they are' },
  { value: 'profiles', label: 'Profiles', description: 'Quality and metadata profiles, mapped onto the built-in ones' },
  { value: 'rootFolders', label: 'Root folders', description: 'Written to the config snippet' },
  { value: 'indexers', label: 'Indexers', description: 'Written to the config snippet' },
  { value: 'clients', label: 'Download clients', description: 'Written to the config snippet' },
  { value: 'naming', label: 'Naming', description: 'Reported only' },
  { value: 'downloads', label: 'In-flight downloads', description: 'Tracked by hash; the client is not touched' },
  { value: 'blocklist', label: 'Blocklist', description: 'Kept from grabbing again' },
  { value: 'history', label: 'History', description: 'Recent rows, marked as imported' },
];

export const defaultForm = (): RunForm => ({
  sources: sourceApps.map((s) => ({ app: s.app, include: false, url: '', apiKey: '' })),
  scope: categories.filter((c) => c.value !== 'history').map((c) => c.value),
  fromPrefix: '', toPrefix: '', historyDays: 90, activate: false, exclude: [],
});

/** Display name of a plan category (scope categories plus files and modes). */
export function categoryLabel(c: string): string {
  return categories.find((x) => x.value === c)?.label ?? ({ files: 'Files', modes: 'Modes' } as Record<string, string>)[c] ?? c;
}

/** The plan/apply request; sources left out or without an address are dropped. */
export function requestBody(f: RunForm, planHash?: string): Record<string, unknown> {
  const body: Record<string, unknown> = {
    sources: f.sources.filter((s) => s.include && s.url.trim()).map((s) => ({ app: s.app, url: s.url.trim(), apiKey: s.apiKey })),
    scope: f.scope,
    activate: f.activate,
    exclude: f.exclude,
  };
  if (f.fromPrefix.trim() && f.toPrefix.trim()) body.pathRewrite = `${f.fromPrefix.trim()}:${f.toPrefix.trim()}`;
  if (f.scope.includes('history') && f.historyDays) body.historyDays = f.historyDays;
  if (planHash) body.planHash = planHash;
  return body;
}

/** Problems that keep the sources step from moving on. */
export function sourceProblems(f: RunForm): string[] {
  const chosen = f.sources.filter((s) => s.include);
  if (!chosen.length) return ['Choose at least one source.'];
  const out: string[] = [];
  for (const s of chosen) {
    const label = sourceApps.find((a) => a.app === s.app)?.label ?? s.app;
    if (!/^https?:\/\/[^/]+/.test(s.url.trim())) out.push(`${label}: enter its address, starting with http:// or https://.`);
    if (!s.apiKey.trim()) out.push(`${label}: enter its API key.`);
  }
  return out;
}

/** Problems on the scope step. */
export function scopeProblems(f: RunForm): string[] {
  const out: string[] = [];
  if (!f.scope.length) out.push('Choose at least one category.');
  if (!!f.fromPrefix.trim() !== !!f.toPrefix.trim()) out.push('Give both path prefixes, or neither.');
  for (const p of [f.fromPrefix, f.toPrefix]) if (p.trim() && !p.trim().startsWith('/')) out.push(`${p} must be an absolute path.`);
  return out;
}

/** Badge (status family and value) and label of an outcome. */
export function outcomeView(o: Outcome): { family: 'migration' | 'check'; value: string; label: string } {
  const map: Record<Outcome, [ 'migration' | 'check', string, string ]> = {
    create: ['migration', 'create', 'Create'], update: ['migration', 'update', 'Update'], unchanged: ['migration', 'unchanged', 'Unchanged'],
    skip: ['migration', 'skip', 'Skip'], approximate: ['migration', 'approximate', 'Approximate'],
    unsupported: ['migration', 'unsupported', 'Unsupported'], conflict: ['migration', 'conflict', 'Conflict'],
    secretNeeded: ['migration', 'secret-needed', 'Secret needed'], fileMissing: ['migration', 'file-missing', 'File missing'],
    managedByConfig: ['migration', 'managed', 'Managed by config'], gone: ['migration', 'skip', 'Gone'], failed: ['check', 'fail', 'Failed'],
  };
  const [family, value, label] = map[o] ?? ['migration', 'skip', o];
  return { family, value, label };
}

/** Outcomes in report order, with their counts. */
export function countTiles(counts: MigrationReport['counts']): { outcome: Outcome; label: string; n: number }[] {
  const order: Outcome[] = ['create', 'update', 'approximate', 'unchanged', 'conflict', 'secretNeeded', 'fileMissing', 'managedByConfig',
    'unsupported', 'skip', 'gone', 'failed'];
  return order.filter((o) => (counts?.[o] ?? 0) > 0).map((o) => ({ outcome: o, label: outcomeView(o).label, n: counts?.[o] ?? 0 }));
}

/** Facets of the plan table: outcome, category and source, with counts. */
export function planFacets(entries: PlanEntry[]): FilterFacet[] {
  const facet = (id: string, label: string, key: (e: PlanEntry) => string, name: (v: string) => string): FilterFacet => {
    const n = new Map<string, number>();
    for (const e of entries) n.set(key(e), (n.get(key(e)) ?? 0) + 1);
    return { id, label, options: [...n].sort((a, b) => a[0].localeCompare(b[0])).map(([value, count]) => ({ value, label: name(value), count })) };
  };
  return [
    facet('outcome', 'Outcome', (e) => e.outcome, (v) => outcomeView(v as Outcome).label),
    facet('category', 'Category', (e) => e.category, categoryLabel),
    facet('source', 'Source', (e) => e.source || 'corsarr', (v) => sourceApps.find((s) => s.app === v)?.label ?? v),
  ];
}

/** The entries the filter keeps. */
export function filterEntries(entries: PlanEntry[], f: FilterState): PlanEntry[] {
  const q = f.q.trim().toLowerCase();
  const keep = (id: string, v: string) => !f.facets[id]?.length || f.facets[id].includes(v);
  return entries.filter((e) => keep('outcome', e.outcome) && keep('category', e.category) && keep('source', e.source || 'corsarr')
    && (!q || `${e.name} ${e.reason} ${e.id}`.toLowerCase().includes(q)));
}

/** One line of the plan/apply stream. */
export type StreamLine =
  | { record: { progress: { source: string; step: string; done: number; total: number } } }
  | { error: { source: string; message: string } }
  | { summary: { done: boolean; report: MigrationReport; planChanged?: boolean } };

/** Progress per chosen source as the stream goes: running until the summary names its outcome. */
export function progressOf(apps: SourceApp[], lines: StreamLine[]): SourceProgress[] {
  const steps = new Map<string, number>();
  let report: MigrationReport | undefined;
  for (const l of lines) {
    if ('record' in l) steps.set(l.record.progress.source, (steps.get(l.record.progress.source) ?? 0) + 1);
    if ('summary' in l) report = l.summary.report;
  }
  return apps.map((app) => {
    const name = sourceApps.find((s) => s.app === app)?.label ?? app;
    const info = report?.sources?.find((s) => s.app === app);
    if (info?.error) return { id: app, name, state: 'failed', reason: info.error };
    if (report) return { id: app, name, state: 'ok', count: report.entries?.filter((e) => e.source === app).length ?? 0 };
    return { id: app, name, state: steps.has(app) ? 'running' : 'pending' };
  });
}

/** Leave an entry out of the apply, or take it back in. */
export function toggleExclude(list: string[], id: string): string[] {
  return list.includes(id) ? list.filter((x) => x !== id) : [...list, id];
}

/** Categories an apply writes to the database (the others are reported or go to the config snippet). */
const written = new Set(['library', 'files', 'downloads', 'blocklist', 'history', 'modes']);

/** Entries that an apply would write: the ones the owner may leave out. */
export const writes = (e: PlanEntry): boolean =>
  written.has(e.category) && (e.outcome === 'create' || e.outcome === 'update' || e.outcome === 'approximate');

/** How many entries an apply writes, after the ones left out. */
export const applyCount = (entries: PlanEntry[] | null | undefined, exclude: string[]): number =>
  (entries ?? []).filter((e) => writes(e) && !exclude.includes(e.id)).length;
