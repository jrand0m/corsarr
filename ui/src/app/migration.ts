// System › Migration: the wire shapes of a migration run and the pure helpers the screen uses
// (request body, outcome badges, progress per source, filtering the plan).
import type { FilterFacet, FilterState } from '../lib/filters/url';
import type { SourceProgress } from '../lib/components/molecules/SourceProgressStrip/SourceProgressStrip.svelte';
import type { ManagedSource } from '../lib/components/atoms/ManagedValue/ManagedValue.svelte';

export type SourceApp = 'sonarr' | 'radarr' | 'lidarr' | 'prowlarr';
export const sourceApps: { app: SourceApp; label: string; note: string }[] = [
  { app: 'sonarr', label: 'Sonarr', note: 'Series, episodes, files, queue, blocklist and history' },
  { app: 'radarr', label: 'Radarr', note: 'Movies, files, queue, blocklist and history' },
  { app: 'lidarr', label: 'Lidarr', note: 'Artists, albums, track files, queue, blocklist and history' },
  { app: 'prowlarr', label: 'Prowlarr', note: 'Indexers' },
];

/** The display name of a source app; an app not listed keeps its id. */
const appLabel = (app: string): string => sourceApps.find((s) => s.app === app)?.label ?? app;

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

export interface SourceInfo {
  app: SourceApp; url: string; name?: string; version?: string; supported: string; error?: string;
  /** How long reading this source took. */
  readMs?: number;
}

/** How one source naming token maps onto corsarr's templates. */
export interface TokenMapping { source: SourceApp; field: string; token: string; target?: string; status: 'exact' | 'approximate' | 'none'; note?: string }

export interface MigrationReport {
  runId?: number;
  mode: 'plan' | 'apply';
  sources: SourceInfo[] | null;
  entries: PlanEntry[] | null;
  counts: Partial<Record<Outcome, number>> | null;
  config?: string;
  env?: string[];
  naming?: TokenMapping[];
  planHash: string;
  startedAt?: string;
  finishedAt?: string;
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

export interface KindState {
  kind: string; mode: 'disabled' | 'shadow' | 'active'; managedByConfig: boolean; snippet?: string;
  /** The config key and source that set the mode. */
  managedBy?: ManagedSource;
}

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
  { value: 'naming', label: 'Naming', description: 'Written to the config snippet, with a token report' },
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
    const label = appLabel(s.app);
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
    facet('source', 'Source', (e) => e.source || 'corsarr', appLabel),
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
  const { steps, report } = streamState(lines);
  return apps.map((app) => sourceProgress(app, steps, report));
}

/** Progress lines seen per source, and the report once the summary arrived. */
function streamState(lines: StreamLine[]): { steps: Map<string, number>; report?: MigrationReport } {
  const steps = new Map<string, number>();
  let report: MigrationReport | undefined;
  for (const l of lines) {
    if ('record' in l) steps.set(l.record.progress.source, (steps.get(l.record.progress.source) ?? 0) + 1);
    if ('summary' in l) report = l.summary.report;
  }
  return { steps, report };
}

/** One source's progress: failed or done once the report names it, else running or still waiting. */
function sourceProgress(app: SourceApp, steps: Map<string, number>, report: MigrationReport | undefined): SourceProgress {
  const name = appLabel(app);
  const info = report?.sources?.find((s) => s.app === app);
  if (info?.error) return { id: app, name, state: 'failed', reason: info.error };
  if (report) return { id: app, name, state: 'ok', count: (report.entries ?? []).filter((e) => e.source === app).length };
  return { id: app, name, state: steps.has(app) ? 'running' : 'pending' };
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

// ── Instant checks ──────────────────────────────────────────────────────────

/** What an instant check shows: nothing yet, in flight, an answer, or a failure to ask. */
export interface CheckView<R> { status: 'idle' | 'checking' | 'done' | 'error'; result?: R; error?: string }

export interface InstantCheckOptions<I, R> {
  /** Runs one check; it must stop when the signal aborts. */
  run: (input: I, signal: AbortSignal) => Promise<R>;
  /** Identity of an input: equal signatures are the same check. */
  sig: (input: I) => string;
  /** Whether the input is complete and well-formed enough to check. */
  ready: (input: I) => boolean;
  onview: (v: CheckView<R>) => void;
  /** Pause after the last keystroke before checking, in ms. */
  delay?: number;
}

/**
 * Checks an input as it is entered: at once after a paste, `delay` ms after typing stops, on blur,
 * and on demand. A changed input cancels the pending and the in-flight check, so only the answer
 * for the current input shows; the same input is never checked twice at the same time.
 */
export class InstantCheck<I, R> {
  readonly #o: Required<InstantCheckOptions<I, R>>;
  #timer: ReturnType<typeof setTimeout> | undefined;
  #ctrl: AbortController | undefined;
  #inflight: string | undefined;
  #settled: string | undefined;
  #shown: string | undefined;
  #pasted = false;

  constructor(o: InstantCheckOptions<I, R>) {
    this.#o = { delay: 600, ...o };
  }

  /** The next input event comes from a paste: check it without waiting. */
  paste(): void { this.#pasted = true; }

  /** The input changed (typing or paste). */
  input(i: I): void {
    this.#clearTimer();
    const sig = this.#o.sig(i);
    if (sig !== this.#inflight) this.#abort();
    if (sig !== this.#shown) this.#view(undefined, { status: 'idle' });
    const pasted = this.#pasted;
    this.#pasted = false;
    if (!this.#o.ready(i)) return;
    if (pasted) { this.#start(i, false); return; }
    this.#timer = setTimeout(() => { this.#timer = undefined; this.#start(i, false); }, this.#o.delay);
  }

  /** The field lost focus: check now unless this input was already checked. */
  blur(i: I): void {
    this.#clearTimer();
    if (this.#o.ready(i)) this.#start(i, false);
  }

  /** The Test button: check again even if this input was checked before. Returns false when not ready. */
  test(i: I): boolean {
    this.#clearTimer();
    if (!this.#o.ready(i)) return false;
    this.#start(i, true);
    return true;
  }

  /** Stop everything (the field went away). */
  cancel(): void {
    this.#clearTimer();
    this.#abort();
  }

  #start(i: I, force: boolean): void {
    const sig = this.#o.sig(i);
    if (sig === this.#inflight) return;
    if (!force && sig === this.#settled) return;
    this.#abort();
    const ctrl = new AbortController();
    this.#ctrl = ctrl;
    this.#inflight = sig;
    this.#view(sig, { status: 'checking' });
    this.#o.run(i, ctrl.signal).then(
      (result) => this.#finish(ctrl, sig, { status: 'done', result }),
      (e: unknown) => this.#finish(ctrl, sig, { status: 'error', error: e instanceof Error ? e.message : String(e) }),
    );
  }

  #finish(ctrl: AbortController, sig: string, v: CheckView<R>): void {
    if (ctrl.signal.aborted || this.#ctrl !== ctrl) return;
    this.#ctrl = this.#inflight = undefined;
    this.#settled = sig;
    this.#view(sig, v);
  }

  #view(sig: string | undefined, v: CheckView<R>): void {
    this.#shown = sig;
    this.#o.onview(v);
  }

  #abort(): void {
    this.#ctrl?.abort();
    this.#ctrl = this.#inflight = undefined;
  }

  #clearTimer(): void {
    if (this.#timer !== undefined) clearTimeout(this.#timer);
    this.#timer = undefined;
  }
}

/** A source address corsarr can try: http(s) with a host. */
export const addressReady = (url: string): boolean => {
  try {
    const u = new URL(url.trim());
    return (u.protocol === 'http:' || u.protocol === 'https:') && !!u.hostname;
  } catch {
    return false;
  }
};

/** An API key worth sending: present, no spaces inside. */
export const keyReady = (key: string): boolean => /^\S+$/.test(key.trim());

export type SourceCheckState = 'reachable' | 'keyNeeded' | 'unauthorised' | 'wrongApp' | 'unsupported' | 'unreachable';
export interface SourceCheck { app: SourceApp; state: SourceCheckState; name?: string; version?: string; supported: string; reason?: string }

/** One source check: the full check with the key, or the address alone. */
export interface SourceCheckInput { app: SourceApp; url: string; apiKey: string; withKey: boolean }

export const sourceCheckReady = (i: SourceCheckInput): boolean => addressReady(i.url) && (!i.withKey || keyReady(i.apiKey));

/**
 * Signature of a source check. The key itself is not kept: only its length and a short
 * non-reversible digest, enough to tell one typed key from another.
 */
export function sourceCheckSig(i: SourceCheckInput): string {
  let h = 0;
  const key = i.withKey ? i.apiKey.trim() : '';
  for (let n = 0; n < key.length; n++) h = (Math.imul(h, 31) + key.charCodeAt(n)) | 0;
  return `${i.app}|${i.url.trim()}|${i.withKey ? `${key.length}:${(h >>> 0).toString(36)}` : '-'}`;
}

/** The request body of a source check (the key only when the check uses it). */
export const sourceCheckBody = (i: SourceCheckInput): Record<string, string> =>
  ({ app: i.app, url: i.url.trim(), ...(i.withKey ? { apiKey: i.apiKey.trim() } : {}) });

/** Badge value and detail line of a source check. */
export function sourceCheckView(v: CheckView<SourceCheck>): { value: string; detail?: string } | undefined {
  if (v.status === 'idle') return undefined;
  if (v.status === 'checking') return { value: 'checking' };
  if (v.status === 'error' || !v.result) return { value: 'unreachable', detail: `corsarr could not run the check: ${v.error ?? 'no answer'}` };
  const r = v.result;
  const value = ({ keyNeeded: 'key-needed', wrongApp: 'wrong-app' } as Record<string, string>)[r.state] ?? r.state;
  return { value, detail: r.state === 'reachable' ? reachableDetail(r) : r.reason };
}

/** "Sonarr 4.0.20 · supported 3.x to 4.x": the name the app reports (else its own), version and supported range. */
function reachableDetail(r: SourceCheck): string {
  const name = r.name || appLabel(r.app);
  return `${[name, r.version].filter(Boolean).join(' ')} · supported ${r.supported}`;
}

// ── Path rewrite checks ─────────────────────────────────────────────────────

export interface Access { exists: boolean; readable: boolean; writable: boolean }
export interface RootCheck extends Access { source: SourceApp; path: string; matched: boolean; rewritten: string }
export interface PathCheck { roots: RootCheck[]; target?: Access & { path: string }; sources: { app: SourceApp; reason: string }[] }

/** The path check input: the chosen sources and both prefixes. */
export interface PathCheckInput { sources: SourceForm[]; fromPrefix: string; toPrefix: string }

const prefixOk = (p: string) => !p.trim() || p.trim().startsWith('/');

/** Chosen sources that hold root folders and are complete enough to read. */
export const readableSources = (sources: SourceForm[]): SourceForm[] =>
  sources.filter((s) => s.include && s.app !== 'prowlarr' && addressReady(s.url) && keyReady(s.apiKey));

export const pathCheckReady = (i: PathCheckInput): boolean => prefixOk(i.fromPrefix) && prefixOk(i.toPrefix) && readableSources(i.sources).length > 0;

export const pathCheckSig = (i: PathCheckInput): string =>
  [i.fromPrefix.trim(), i.toPrefix.trim(), ...readableSources(i.sources).map((s) => sourceCheckSig({ ...s, withKey: true }))].join('\n');

export const pathCheckBody = (i: PathCheckInput): Record<string, unknown> => ({
  sources: readableSources(i.sources).map((s) => ({ app: s.app, url: s.url.trim(), apiKey: s.apiKey.trim() })),
  fromPrefix: i.fromPrefix.trim(),
  toPrefix: i.toPrefix.trim(),
});

/** One check line: a `check` badge value, a sentence and optional detail rows. */
export interface CheckLine { value: 'pass' | 'warn' | 'fail'; detail: string; items?: string[] }

/** What the source prefix field shows: which roots it matches and which stay unmapped. */
export function fromPrefixView(c: PathCheck, fromPrefix: string): CheckLine {
  if (!fromPrefix.trim()) return { value: 'pass', detail: 'No rewrite: corsarr uses the paths exactly as the sources report them.' };
  const matched = c.roots.filter((r) => r.matched);
  const unmapped = c.roots.filter((r) => !r.matched);
  if (!c.roots.length) return { value: 'warn', detail: 'The sources report no root folders to compare with.' };
  const items = [...matched.map((r) => `Matches ${r.path}`), ...unmapped.map((r) => `Unmapped ${r.path}`)];
  if (!matched.length) return { value: 'fail', detail: 'No source root folder starts with this prefix.', items };
  return {
    value: unmapped.length ? 'warn' : 'pass',
    detail: unmapped.length ? `Matches ${matched.length} of ${c.roots.length} root folders.` : `Matches every root folder (${matched.length}).`,
    items,
  };
}

/** What the corsarr prefix field shows: exists and readable decide; writable is information. */
export function toPrefixView(c: PathCheck, toPrefix: string): CheckLine {
  if (!toPrefix.trim() || !c.target) return { value: 'pass', detail: 'No rewrite.' };
  const t = c.target;
  if (!t.exists) return { value: 'fail', detail: `${t.path} does not exist for corsarr.` };
  if (!t.readable) return { value: 'fail', detail: `corsarr cannot read ${t.path}.` };
  return { value: 'pass', detail: t.writable ? 'Exists, readable and writable.' : 'Exists and readable. Read-only, which is fine for shadow mode.' };
}

/** The sample: each source root, the path it becomes and whether that exists for corsarr. */
export function sampleView(c: PathCheck): CheckLine {
  const items = [...c.roots.map(rootLine), ...c.sources.map((s) => `${appLabel(s.app)}: ${s.reason}`)];
  if (!c.roots.length) return { value: 'warn', detail: 'No source root folders to check.', items };
  const missing = c.roots.filter((r) => !r.exists || !r.readable).length;
  let value: CheckLine['value'] = 'warn';
  if (missing === c.roots.length) value = 'fail';
  else if (missing === 0 && !c.sources.length) value = 'pass';
  return {
    value,
    detail: missing ? `${missing} of ${c.roots.length} root folders are missing for corsarr.` : `All ${c.roots.length} root folders exist for corsarr.`,
    items,
  };
}

/** "/data/tv → /media/tv: found": a root, the path it becomes when rewritten, and whether corsarr finds and reads it. */
const rootLine = (r: RootCheck): string =>
  `${r.path}${r.rewritten !== r.path ? ` → ${r.rewritten}` : ''}: ${!r.exists ? 'missing' : r.readable ? 'found' : 'found, not readable'}`;

// ── Config-managed notice ───────────────────────────────────────────────────

/** Categories that only produce a config snippet or a report: nothing reaches the database. */
const configOnly = new Set(['rootFolders', 'indexers', 'clients', 'naming']);
const appKind: Partial<Record<SourceApp, string>> = { sonarr: 'tv', radarr: 'movie', lidarr: 'music' };
const kindLabel: Record<string, string> = { tv: 'TV', movie: 'Movies', music: 'Music', book: 'Books' };

export interface ConfigNotice {
  /** Media kinds whose mode the config file sets to another mode, with the snippet to put there instead. */
  kinds: { kind: string; label: string; snippet: string }[];
  /** Chosen categories that only become a config snippet or a report. */
  snippetOnly: string[];
  /** Chosen categories that import into the database as usual. */
  importing: string[];
  /** Every chosen category is config-only: the run writes nothing to the database. */
  nothingWritten: boolean;
}

/** Which chosen categories and kinds the config file controls, and what still imports. */
export function configNotice(f: RunForm, modes: KindState[]): ConfigNotice {
  const mode = f.activate ? 'active' : 'shadow';
  const chosenKinds = new Set(f.sources.filter((s) => s.include).map((s) => appKind[s.app]));
  const kinds = f.scope.includes('library')
    ? modes.filter((k) => k.managedByConfig && k.mode !== mode && chosenKinds.has(k.kind))
      .map((k) => ({ kind: k.kind, label: kindLabel[k.kind] ?? k.kind, snippet: `mediaKinds:\n  ${k.kind}:\n    mode: ${mode}\n` }))
    : [];
  const snippetOnly = f.scope.filter((c) => configOnly.has(c)).map(categoryLabel);
  const importing = f.scope.filter((c) => !configOnly.has(c)).map(categoryLabel);
  return { kinds, snippetOnly, importing, nothingWritten: f.scope.length > 0 && importing.length === 0 };
}

/** Whether the notice has anything to say. */
export const hasConfigNotice = (n: ConfigNotice): boolean => n.kinds.length > 0 || n.snippetOnly.length > 0;

/** The shown plan rows as tab-separated text with a header line, for pasting into a sheet or a chat. */
export function planAsText(entries: PlanEntry[]): string {
  const clean = (v: string | undefined) => (v ?? '').replace(/[\t\r\n]+/g, ' ').trim();
  const rows = entries.map((e) => [outcomeView(e.outcome).label, categoryLabel(e.category), e.source, e.name, e.reason, e.target].map(clean).join('\t'));
  return ['Outcome\tCategory\tSource\tObject\tReason\tTarget', ...rows].join('\n') + '\n';
}

/** The shown plan rows as indented JSON. */
export function planAsJSON(entries: PlanEntry[]): string {
  return JSON.stringify(entries, null, 2) + '\n';
}
