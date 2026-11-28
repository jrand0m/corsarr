// Pure mapping from API records to what the design-system components take: item status, queue cards,
// history wording, health names and the read-only view of the config export. No fetch, no DOM.
import type { QueueCardData } from '../lib/components/molecules/QueueCard/QueueCard.svelte';
import type { StatusFamily } from '../lib/status/vocabulary';
import type { MediaKind } from '../lib/tokens';
import type { Download, HealthStatus, IntegrationHealth, LiveEvent, Movie } from './api/types';
import { count, summariseEpisodes } from './tv';

const DAY = 86_400_000;
/** With only a cinema date, a movie counts as released 90 days later. */
const RELEASED_DELAY = 90 * DAY;

const ms = (s?: string) => (s ? Date.parse(s) : undefined);

/** When the movie meets its minimum availability (mirrors movies.AvailableAt); undefined = unknown. */
export function availableAt(m: Movie): number | undefined {
  if (m.minAvailability === 'announced') return 0;
  if (m.minAvailability === 'inCinemas' && m.inCinemas) return ms(m.inCinemas);
  const dates = [ms(m.digital), ms(m.physical)].filter((d): d is number => d !== undefined);
  if (dates.length) return Math.min(...dates);
  if (m.inCinemas) return ms(m.inCinemas)! + RELEASED_DELAY;
  if (m.year > 0) return Date.UTC(m.year + 1, 0, 1);
  return undefined;
}

export const isAvailable = (m: Movie, now: number): boolean => {
  const at = availableAt(m);
  return at !== undefined && at <= now;
};

export type ItemStatus = 'missing' | 'queued' | 'cutoff-met' | 'not-available';

/** The shared item status of a movie. On disk reads as "cutoff met": the list API has no cutoff state yet. */
export function movieStatus(m: Movie, now: number, queued: ReadonlySet<number>): ItemStatus {
  if (m.fileId) return 'cutoff-met';
  if (queued.has(m.id)) return 'queued';
  return isAvailable(m, now) ? 'missing' : 'not-available';
}

/** Movie ids a live download covers. */
export function queuedMovieIds(downloads: Download[]): Set<number> {
  const out = new Set<number>();
  for (const d of downloads) {
    if (['imported', 'removed', 'failed'].includes(d.state)) continue;
    for (const l of d.links ?? []) if (l.item.kind === 'movie') out.add(l.item.id);
  }
  return out;
}

export const isActiveDownload = (d: Download) => !['imported', 'removed'].includes(d.state);

/**
 * Queue card for a download. Movie links take the movie title; TV links point at episodes, so the
 * caller may name them (`episodes`, e.g. "Example Show · S01 · 10 episodes"), else the count shows.
 */
export function toQueueCard(d: Download, titleOf: (id: number) => string | undefined, episodes?: (ids: number[]) => string | undefined): QueueCardData {
  const item = queueItem(d, titleOf, episodes);
  return {
    id: String(d.id),
    title: item.title ?? d.title,
    subtitle: item.title ? d.title : undefined,
    kind: d.kind || (item.tv ? 'tv' : 'movie'),
    state: d.state,
    reason: orNone(d.statusReason),
    held: d.held,
    progress: queueProgress(d),
    size: orNone(d.size),
    downloaded: orNone(d.downloaded),
    speed: orNone(d.speed),
    eta: d.eta > 0 ? d.eta : undefined,
    href: item.movieId === undefined ? undefined : `#/library/movies/${item.movieId}`,
  };
}

interface QueueItem { title?: string; tv: boolean; movieId?: number }

/** What a download fetches: its episodes when it has TV links, else the linked movie. */
function queueItem(d: Download, titleOf: (id: number) => string | undefined, episodes?: (ids: number[]) => string | undefined): QueueItem {
  const links = d.links ?? [];
  const tv = links.filter((l) => l.item.kind === 'tv').map((l) => l.item.id);
  if (tv.length) return { title: episodes?.(tv) ?? count(tv.length, 'episode'), tv: true };
  const movieId = links.find((l) => l.item.kind === 'movie')?.item.id;
  return { title: movieId === undefined ? undefined : titleOf(movieId), tv: false, movieId };
}

/** A queued download of unknown size shows no progress yet. */
const queueProgress = (d: Download) => (d.state === 'queued' && !d.size ? undefined : d.progress);

/** Empty strings and zeros read as "not known". */
const orNone = <T>(v: T): T | undefined => v || undefined;

export interface EventView { label: string; family: StatusFamily; value: string }

const eventViews: Record<string, EventView> = {
  'item.added': { label: 'Added', family: 'item', value: 'queued' },
  'item.deleted': { label: 'Deleted', family: 'download', value: 'removed' },
  'grab.sent': { label: 'Grabbed', family: 'download', value: 'queued' },
  'grab.failed': { label: 'Grab failed', family: 'download', value: 'failed' },
  'decision.shadowed': { label: 'Would grab (shadow)', family: 'kindMode', value: 'shadow' },
  'downloader.shadowed': { label: 'Would change the client (shadow)', family: 'kindMode', value: 'shadow' },
  'download.failed': { label: 'Download failed', family: 'download', value: 'failed' },
  'download.removed': { label: 'Removed', family: 'download', value: 'removed' },
  'import.completed': { label: 'Imported', family: 'importOutcome', value: 'imported' },
  'import.blocked': { label: 'Import blocked', family: 'importOutcome', value: 'blocked' },
  'import.failed': { label: 'Import failed', family: 'importOutcome', value: 'failed' },
  'payload.rejected': { label: 'Payload rejected', family: 'verdict', value: 'reject' },
  'file.renamed': { label: 'Renamed', family: 'importOutcome', value: 'imported' },
  'file.deleted': { label: 'File deleted', family: 'download', value: 'removed' },
  'blocklist.added': { label: 'Blocklisted', family: 'link', value: 'blocked' },
  'blocklist.removed': { label: 'Unblocklisted', family: 'link', value: 'skipped' },
  'rule.acted': { label: 'Rule acted', family: 'classification', value: 'acted' },
  'rule.undone': { label: 'Rule undone', family: 'classification', value: 'held' },
  'config.applied': { label: 'Config applied', family: 'check', value: 'pass' },
  'config.apply_failed': { label: 'Config rejected', family: 'check', value: 'fail' },
  'config.drifted': { label: 'Config drifted', family: 'check', value: 'warn' },
  'mediakind.mode_changed': { label: 'Kind mode changed', family: 'kindMode', value: 'active' },
  'refresh.abandoned': { label: 'Library refresh gave up', family: 'health', value: 'failing' },
  'migration.run_completed': { label: 'Migration applied', family: 'check', value: 'pass' },
  'mcp.called': { label: 'Assistant call', family: 'check', value: 'pass' },
};

/** Label and badge for a History event; health changes take the new health state. */
export function eventView(ev: LiveEvent): EventView {
  const fromData = dataViews[ev.type];
  if (fromData) return fromData(ev.data ?? {});
  return eventViews[ev.type] ?? { label: ev.type, family: 'check', value: 'pending' };
}

/** Events whose label and badge depend on their data. */
const dataViews: Record<string, (d: Record<string, unknown>) => EventView> = {
  'health.changed': (d) => {
    const to = String(d.to ?? 'ok');
    return { label: `Health: ${healthLabel[to] ?? to}`, family: 'health', value: to };
  },
  'mediakind.mode_changed': (d) => {
    const to = String(d.to ?? 'active');
    return { label: `${String(d.kind ?? 'Kind')} mode: ${to}`, family: 'kindMode', value: to };
  },
  'plugin.changed': (d) => pluginChangeViews[String(d.action)] ?? { label: 'Plugin changed', family: 'check', value: 'pass' },
};

const pluginChangeViews: Record<string, EventView> = {
  installed: { label: 'Plugin installed', family: 'check', value: 'pass' },
  updated: { label: 'Plugin updated', family: 'check', value: 'pass' },
  rolledback: { label: 'Plugin rolled back', family: 'check', value: 'warn' },
  uninstalled: { label: 'Plugin uninstalled', family: 'download', value: 'removed' },
};

/** "1.3.0 (was 1.2.0) · from the catalogue" for a plugin module change. */
function pluginChangeDetail(d: Record<string, unknown>): string | undefined {
  const version = typeof d.version === 'string' ? d.version : '';
  const was = typeof d.previous === 'string' && d.previous ? ` (was ${d.previous})` : '';
  const from = d.source === 'catalogue' ? 'from the catalogue' : d.source === 'file' ? 'from a file' : '';
  const parts = [version + was, from].filter(Boolean);
  return parts.length ? parts.join(' · ') : undefined;
}

const healthLabel: Record<string, string> = {
  ok: 'OK', degraded: 'degraded', backed_off: 'backed off', failing: 'failing', action_needed: 'action needed', disabled: 'disabled',
};

/** "You", "Automation", "Rule", "corsarr": who did it. */
export function actorLabel(actor: string): string {
  if (actor === 'user') return 'You';
  if (actor === 'automation') return 'Automation';
  if (actor === 'system') return 'corsarr';
  if (actor.startsWith('plugin:')) return `Plugin ${actor.slice(7)}`;
  if (actor.startsWith('import:')) return `Imported from ${actor.slice(7)}`;
  if (actor.startsWith('rule')) return 'Rule';
  return actor || '—';
}

/** The subject of an event: item titles, else the integration or release it names. */
export function eventSubject(ev: LiveEvent): string {
  if (ev.items?.length) return itemsSubject(ev.items);
  return dataSubject(ev) ?? ev.type;
}

/** Item titles, or one line for several episodes when that is all the event names. */
function itemsSubject(items: NonNullable<LiveEvent['items']>): string {
  const episodes = items.filter((i) => i.ref.kind === 'tv' && i.ref.variant !== 'series');
  if (episodes.length > 1 && episodes.length === items.length) return summariseEpisodes(episodes);
  return items.map((i) => i.title).join(', ');
}

/** The plugin, integration or release an event's data names. */
function dataSubject(ev: LiveEvent): string | undefined {
  const d = ev.data ?? {};
  if (ev.type === 'plugin.changed' && typeof d.id === 'string') return d.id;
  const ref = d.ref as { type?: string; key?: string } | undefined;
  if (ref?.key) return integrationName(ref as IntegrationHealth['ref']);
  return (d.release as { title?: string } | undefined)?.title ?? (d.title as string | undefined);
}

/** One line on what happened, from the event data. */
export function eventDetail(ev: LiveEvent): string | undefined {
  const d = ev.data ?? {};
  if (ev.type === 'plugin.changed') return pluginChangeDetail(d);
  const parts = [d.message, d.reason, (d.release as { title?: string } | undefined)?.title, d.path].filter((x) => typeof x === 'string' && x);
  return parts.length ? parts.join(' · ') : undefined;
}

const typeNames: Record<string, string> = {
  indexer: 'Indexer', downloader: 'Download client', metadata: 'Metadata', mediaserver: 'Media server', notifier: 'Notifications',
  rootFolder: 'Root folder', disk: 'Disk', config: 'Configuration', plugin: 'Plugin', scheduler: 'Scheduler',
};
export const integrationType = (t: string) => typeNames[t] ?? t;
export const integrationName = (ref: { type: string; key: string }) =>
  ref.type === 'config' && ref.key === 'source' ? 'Configuration source' : `${ref.key}`;

/** Health problems first (action needed → degraded), then OK, then disabled. */
export const healthRank: Record<HealthStatus, number> = { action_needed: 0, failing: 1, backed_off: 2, degraded: 3, ok: 4, disabled: 5 };
export const isProblem = (h: IntegrationHealth) => healthRank[h.status] < 4;

/** Settings › section that configures an integration type (for "Open settings"). */
export const settingsFor = (type: string) => (type === 'rootFolder' || type === 'disk' || type === 'config' ? '#/settings/general' : '#/settings/plugins');

// ── config export (read-only views) ─────────────────────────────────────────

export interface ConfiguredInstance {
  section: string;
  key: string;
  plugin: string;
  name?: string;
  /** Field values by lower-case field name (env keys lose camelCase). */
  values: Record<string, unknown>;
  /** Fields that are secrets with the env var holding them. */
  secrets: Record<string, string>;
}

export interface ExportView {
  kinds: Partial<Record<MediaKind, string>>;
  rootFolders: { path: string; kind: string }[];
  instances: ConfiguredInstance[];
  server: Record<string, string>;
  /** The assistant (MCP) endpoint settings. */
  mcp: McpView;
}

export interface McpView {
  enabled: boolean;
  scope: string;
  /** Tools exposed by name; empty exposes every tool the scope allows. */
  tools: string[];
  rateLimit?: number;
}

function readMcp(view: ExportView, field: string, index: string | undefined, value: string) {
  if (field === 'ENABLED') view.mcp.enabled = value === 'true';
  else if (field === 'SCOPE') view.mcp.scope = value;
  else if (field === 'RATELIMIT') view.mcp.rateLimit = Number(value);
  else if (field === 'TOOLS' && index !== undefined && value) view.mcp.tools[Number(index)] = value;
}

/** Config sections as the env export spells them → their YAML names. */
export const instanceSections: Record<string, string> = {
  INDEXERS: 'indexers', SOURCES: 'sources', DOWNLOADCLIENTS: 'downloadClients', MEDIASERVERS: 'mediaServers',
  NOTIFICATIONS: 'notifications', METADATA: 'metadata', CHATBOTS: 'chatbots',
};

/**
 * Reads `GET /config/export?format=env&full=true` (CORSARR_SECTION_KEY_FIELD[_N]=value) into kinds,
 * root folders and plugin instances. Secrets export empty; their reference is CORSARR_SECRET_<rest>.
 */
export function parseEnvExport(text: string): ExportView {
  const view: ExportView = { kinds: {}, rootFolders: [], instances: [], server: {}, mcp: { enabled: false, scope: 'read', tools: [] } };
  const byKey = new Map<string, ConfiguredInstance>();
  for (const raw of text.split('\n')) {
    const line = readEnvLine(raw);
    if (line) applyEnvLine(view, byKey, line);
  }
  view.rootFolders = view.rootFolders.filter(Boolean);
  view.mcp.tools = view.mcp.tools.filter(Boolean);
  return view;
}

/** One `CORSARR_NAME=value` line: the name after the prefix, split on `_`, and its value. */
interface EnvLine { name: string; parts: string[]; value: string }

/** Reads an export line; blank lines, comments and other variables give undefined. */
function readEnvLine(raw: string): EnvLine | undefined {
  const line = raw.trim();
  const eq = line.indexOf('=');
  if (eq < 0 || !line.startsWith('CORSARR_')) return undefined;
  const name = line.slice(8, eq);
  return { name, parts: name.split('_'), value: line.slice(eq + 1) };
}

/** Server-level settings sections, kept as dotted names. */
const serverSections = new Set(['SERVER', 'DATA', 'LOG', 'AUTH']);

/** Files one export line into the view by its section; unknown sections are skipped. */
function applyEnvLine(view: ExportView, byKey: Map<string, ConfiguredInstance>, line: EnvLine) {
  const [head, a, b] = line.parts;
  if (head === 'MEDIAKINDS') readKind(view, a, b, line.value);
  else if (head === 'MCP') readMcp(view, a, b, line.value);
  else if (head === 'ROOTFOLDERS') readRootFolder(view, a, b, line.value);
  else if (instanceSections[head]) readInstance(view, byKey, line);
  else if (serverSections.has(head)) view.server[line.parts.join('.').toLowerCase()] = line.value;
}

function readKind(view: ExportView, kind: string, field: string | undefined, value: string) {
  if (field === 'MODE') view.kinds[kind.toLowerCase() as MediaKind] = value;
}

function readRootFolder(view: ExportView, index: string, field: string | undefined, value: string) {
  const folder = (view.rootFolders[Number(index)] ??= { path: '', kind: '' });
  if (field === 'PATH') folder.path = value;
  if (field === 'KIND') folder.kind = value;
}

/** SECTION_KEY_FIELD[_SUB]=value: plugin type, name, list or map entries, secrets and plain values. */
function readInstance(view: ExportView, byKey: Map<string, ConfiguredInstance>, line: EnvLine) {
  const [head, key, rawField, ...rest] = line.parts;
  if (!key || !rawField) return;
  const inst = instanceFor(view, byKey, head, key);
  const field = rawField.toLowerCase();
  if (field === 'type') inst.plugin = line.value;
  else if (field === 'name') inst.name = line.value;
  else if (rest.length) readNested(inst, field, rest, line.value);
  else if (line.value === '') inst.secrets[field] = `CORSARR_SECRET_${line.name}`;
  else inst.values[field] = line.value;
}

/** The instance a section and key name, added to the view on first sight. */
function instanceFor(view: ExportView, byKey: Map<string, ConfiguredInstance>, head: string, key: string): ConfiguredInstance {
  const id = `${head}.${key}`;
  let inst = byKey.get(id);
  if (!inst) {
    inst = { section: instanceSections[head], key: key.toLowerCase(), plugin: '', values: {}, secrets: {} };
    byKey.set(id, inst);
    view.instances.push(inst);
  }
  return inst;
}

/** FIELD_N is a list entry, FIELD_SUB a map entry. */
function readNested(inst: ConfiguredInstance, field: string, rest: string[], value: string) {
  if (/^\d+$/.test(rest[0])) {
    const list = (inst.values[field] as string[] | undefined) ?? [];
    list[Number(rest[0])] = value;
    inst.values[field] = list;
  } else {
    const map = (inst.values[field] as Record<string, string> | undefined) ?? {};
    map[rest.join('_').toLowerCase()] = value;
    inst.values[field] = map;
  }
}
