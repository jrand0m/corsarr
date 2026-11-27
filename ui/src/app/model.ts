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
  const links = d.links ?? [];
  const tv = links.filter((l) => l.item.kind === 'tv').map((l) => l.item.id);
  const link = links.find((l) => l.item.kind === 'movie');
  const item = tv.length ? (episodes?.(tv) ?? count(tv.length, 'episode')) : link ? titleOf(link.item.id) : undefined;
  return {
    id: String(d.id),
    title: item ?? d.title,
    subtitle: item ? d.title : undefined,
    kind: d.kind || (tv.length ? 'tv' : 'movie'),
    state: d.state,
    reason: d.statusReason || undefined,
    held: d.held,
    progress: d.state === 'queued' && !d.size ? undefined : d.progress,
    size: d.size || undefined,
    downloaded: d.downloaded || undefined,
    speed: d.speed || undefined,
    eta: d.eta > 0 ? d.eta : undefined,
    href: link && !tv.length ? `#/library/movies/${link.item.id}` : undefined,
  };
}

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
};

/** Label and badge for a History event; health changes take the new health state. */
export function eventView(ev: LiveEvent): EventView {
  if (ev.type === 'health.changed') {
    const to = String(ev.data?.to ?? 'ok') as HealthStatus;
    return { label: `Health: ${healthLabel[to] ?? to}`, family: 'health', value: to };
  }
  if (ev.type === 'mediakind.mode_changed') {
    const to = String(ev.data?.to ?? 'active');
    return { label: `${String(ev.data?.kind ?? 'Kind')} mode: ${to}`, family: 'kindMode', value: to };
  }
  return eventViews[ev.type] ?? { label: ev.type, family: 'check', value: 'pending' };
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
  if (ev.items?.length) {
    const episodes = ev.items.filter((i) => i.ref.kind === 'tv' && i.ref.variant !== 'series');
    if (episodes.length > 1 && episodes.length === ev.items.length) return summariseEpisodes(episodes);
    return ev.items.map((i) => i.title).join(', ');
  }
  const d = ev.data ?? {};
  const ref = d.ref as { type?: string; key?: string } | undefined;
  if (ref?.key) return integrationName(ref as IntegrationHealth['ref']);
  const rel = (d.release as { title?: string } | undefined)?.title ?? (d.title as string | undefined);
  return rel ?? ev.type;
}

/** One line on what happened, from the event data. */
export function eventDetail(ev: LiveEvent): string | undefined {
  const d = ev.data ?? {};
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
  const view: ExportView = { kinds: {}, rootFolders: [], instances: [], server: {} };
  const byKey = new Map<string, ConfiguredInstance>();
  for (const raw of text.split('\n')) {
    const line = raw.trim();
    if (!line || line.startsWith('#')) continue;
    const eq = line.indexOf('=');
    if (eq < 0 || !line.startsWith('CORSARR_')) continue;
    const name = line.slice(8, eq);
    const value = line.slice(eq + 1);
    const parts = name.split('_');
    const [head, a, b, ...rest] = parts;
    if (head === 'MEDIAKINDS' && b === 'MODE') view.kinds[a.toLowerCase() as MediaKind] = value;
    else if (head === 'ROOTFOLDERS') {
      const i = Number(a);
      view.rootFolders[i] ??= { path: '', kind: '' };
      if (b === 'PATH') view.rootFolders[i].path = value;
      if (b === 'KIND') view.rootFolders[i].kind = value;
    } else if (instanceSections[head] && a && b) {
      const id = `${head}.${a}`;
      let inst = byKey.get(id);
      if (!inst) {
        inst = { section: instanceSections[head], key: a.toLowerCase(), plugin: '', values: {}, secrets: {} };
        byKey.set(id, inst);
        view.instances.push(inst);
      }
      const field = b.toLowerCase();
      if (field === 'type') inst.plugin = value;
      else if (field === 'name') inst.name = value;
      else if (rest.length && /^\d+$/.test(rest[0])) {
        const list = (inst.values[field] as string[] | undefined) ?? [];
        list[Number(rest[0])] = value;
        inst.values[field] = list;
      } else if (rest.length) {
        const map = (inst.values[field] as Record<string, string> | undefined) ?? {};
        map[rest.join('_').toLowerCase()] = value;
        inst.values[field] = map;
      } else if (value === '') inst.secrets[field] = `CORSARR_SECRET_${name}`;
      else inst.values[field] = value;
    } else if (head === 'SERVER' || head === 'DATA' || head === 'LOG' || head === 'AUTH') {
      view.server[parts.join('.').toLowerCase()] = value;
    }
  }
  view.rootFolders = view.rootFolders.filter(Boolean);
  return view;
}
