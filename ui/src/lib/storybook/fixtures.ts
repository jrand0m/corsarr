// Deterministic synthetic data for template and page stories (Pages/*). corsarr's voice, fixed
// "now" (22 Nov 2026, 14:05 UTC) so visual checks stay stable. Not a design-system component.
import type { Decision } from '../components/organisms/DecisionPanel/DecisionPanel.svelte';
import type { Variant } from '../components/organisms/VariantPicker/VariantPicker.svelte';
import type { QueueCardData } from '../components/molecules/QueueCard/QueueCard.svelte';
import type { TimelineEntry } from '../components/organisms/Timeline/Timeline.svelte';
import type { PaletteItem } from '../components/organisms/CommandPalette/CommandPalette.svelte';
import type { SourceProgress } from '../components/molecules/SourceProgressStrip/SourceProgressStrip.svelte';
import type { PathLocation } from '../components/molecules/PathPicker/PathPicker.svelte';
import type { SettingsSchema, FormValues, FieldMeta } from '../forms/schema';
import type { MediaKind } from '../tokens';
import type { SettingsSection } from '../components/templates/SettingsPage/SettingsPage.svelte';

export const NOW = Date.parse('2026-11-22T14:05:00Z');
export const nowDate = new Date(NOW);
const min = 60_000, hour = 3_600_000, day = 86_400_000;

export const shellStatus = { unhealthy: 1, healthTone: 'error' as const, active: 3, blocked: 1 };
export const kinds: MediaKind[] = ['tv', 'movie', 'music'];

export const paletteItems: PaletteItem[] = [
  { id: 'item:dockside', label: 'Dockside', group: 'Items', kind: 'movie', subtitle: 'Movie · 2026' },
  { id: 'item:long-haul', label: 'The Long Haul', group: 'Items', kind: 'tv', subtitle: 'TV · 2024' },
  { id: 'item:harbour-lights', label: 'Harbour Lights', group: 'Items', kind: 'music', subtitle: 'Album · 2025' },
  { id: 'act:pause-all', label: 'Pause all downloads', group: 'Actions', icon: 'pause' },
  { id: 'act:rss', label: 'Sync feeds now', group: 'Actions', icon: 'rss' },
];

// Library › Movies
export interface MovieRow {
  id: string; title: string; year: number; status: 'cutoff-met' | 'cutoff-unmet' | 'missing' | 'queued' | 'not-available';
  quality: string; profile: string; size: number; monitored: boolean; added: number; runtime: number;
}
const movieTitles = [
  'Dockside', 'Quay Lights', 'The Night Crossing', 'Salt and Iron', 'Low Water', 'Pilot Boat', 'Grey Manifest',
  'Northern Signal', 'Slow Current', 'The Red Line', 'Harbour Watch', 'Customs', 'Berth Seven', 'Under the Gantry',
  'Ballast', 'The Last Tender', 'Fog Horn', 'Tidewater', 'Container 42', 'Stevedore', '港の灯り', 'Crane Operator',
  'Lighthouse Keeper', 'Breakwater', 'A Very Long Title About Ships That Pass in the Night and Other Stories',
];
const mStatus: MovieRow['status'][] = ['cutoff-met', 'cutoff-met', 'cutoff-unmet', 'missing', 'cutoff-met', 'queued', 'not-available'];
const qualities = ['WEB-DL 1080p', 'Bluray 1080p', 'WEB-DL 2160p', 'Remux 2160p', 'HDTV 720p'];
export const movies: MovieRow[] = Array.from({ length: 120 }, (_, i) => {
  const status = mStatus[(i * 3) % mStatus.length];
  const has = status === 'cutoff-met' || status === 'cutoff-unmet';
  return {
    id: `m${i}`,
    title: i < movieTitles.length ? movieTitles[i] : `${movieTitles[i % movieTitles.length]} ${Math.floor(i / movieTitles.length) + 1}`,
    year: 1998 + ((i * 7) % 29),
    status,
    quality: has ? qualities[(i * 5) % qualities.length] : '—',
    profile: i % 4 === 0 ? 'UHD' : 'HD',
    size: has ? 4_200_000_000 + ((i * 3_700_000_000) % 52_000_000_000) : 0,
    monitored: i % 9 !== 4,
    added: NOW - ((i * 37 * hour) % (300 * day)),
    runtime: 5400 + ((i * 611) % 3000),
  };
});

// Item detail and interactive search
export const docksideDecisions: Decision[] = [
  {
    release: { title: 'Dockside.2026.1080p.WEB-DL.DDP5.1.H.264-HARBOUR', indexer: 'Harbour Tracker', protocol: 'torrent', size: 6.2e9, publishedAt: NOW - 2 * hour, seeders: 1240, quality: 'WEB-DL 1080p', runtimeSeconds: 6720 },
    verdict: 'accept', rank: 1, candidates: 9, score: { total: 1250, formats: [{ name: 'DD+ 5.1', score: 250 }, { name: 'Trusted group', score: 1000 }] }, reasons: [],
  },
  {
    release: { title: 'Dockside.2026.1080p.BluRay.x264-QUAY', indexer: 'Dock Feeds', protocol: 'usenet', size: 9.8e9, publishedAt: NOW - 20 * hour, quality: 'Bluray 1080p', runtimeSeconds: 6720 },
    verdict: 'accept', rank: 2, candidates: 9, score: { total: 750, formats: [{ name: 'Trusted group', score: 750 }] }, reasons: [],
  },
  {
    release: { title: 'Dockside.2026.2160p.UHD.BluRay.REMUX.HDR.TrueHD.7.1-DOCKYARD', indexer: 'Harbour Tracker', protocol: 'torrent', size: 58.2e9, publishedAt: NOW - 3 * day, seeders: 0, quality: 'Remux 2160p', runtimeSeconds: 6720 },
    verdict: 'reject', rank: 6, candidates: 9, score: { total: 1500, formats: [{ name: 'TrueHD Atmos', score: 500 }, { name: 'HDR10', score: 1000 }] },
    reasons: [
      { rule: 'size.max', category: 'preference', message: '58.2 GB is over the 40 GB per-movie cap' },
      { rule: 'seeders.min', category: 'temporary', message: '0 seeders, below the minimum of 1' },
    ],
  },
  {
    release: { title: 'Dockside.2026.1080p.WEB.x264-FREE.RAR', indexer: 'Usenet Pier', protocol: 'usenet', size: 1.1e9, publishedAt: NOW - 5 * hour, quality: 'WEB 1080p' },
    verdict: 'reject', rank: 9, candidates: 9, score: { total: -500, formats: [{ name: 'Unknown group', score: -500 }] },
    reasons: [
      { rule: 'payload.executable', category: 'safety', message: 'Archive contains an executable (setup.exe)' },
      { rule: 'blocklist', category: 'permanent', message: 'On the blocklist since 20 Nov (fake release)', blocklist: true },
    ],
  },
];
export const searchSources: SourceProgress[] = [
  { id: 'harbour', name: 'Harbour Tracker', state: 'ok', count: 5, ms: 840 },
  { id: 'dock', name: 'Dock Feeds', state: 'ok', count: 3, ms: 1210 },
  { id: 'pier', name: 'Usenet Pier', state: 'running' },
  { id: 'slug', name: 'Slow Slug', state: 'failed', reason: '503 Service Unavailable' },
];

export const docksideHistory: TimelineEntry[] = [
  { id: 'h1', kind: 'apply', at: NOW - 26 * hour, title: 'Added to the library', actor: 'You' },
  { id: 'h2', kind: 'attempt', at: NOW - 25 * hour, title: 'Searched 4 indexers: nothing accepted yet', status: { family: 'job', value: 'done' }, actor: 'Automatic search' },
  { id: 'h3', kind: 'apply', at: NOW - 2 * hour, title: 'Grab sent to Harbour box', status: { family: 'delivery', value: 'delivered' }, fields: [{ label: 'Release', value: 'Dockside.2026.1080p.WEB-DL.DDP5.1.H.264-HARBOUR', mono: true }] },
  { id: 'h4', kind: 'transition', at: NOW - 100 * min, title: 'Download started', from: { family: 'download', value: 'queued' }, to: { family: 'download', value: 'downloading' } },
];

// Add movie: provider candidates (VariantPicker-style ranked list)
export const addCandidates: Variant[] = [
  { id: 'tmdb:10101', title: 'Dockside (2026)', quality: 'Movie · 1h 52m', size: 0, score: 98, reason: 'Exact title and year · Harbourline Pictures', indexer: 'tmdb:10101' },
  { id: 'tmdb:9302', title: 'Dockside (1987)', quality: 'Movie · 1h 38m', size: 0, score: 71, reason: 'Same title, earlier film', indexer: 'tmdb:9302' },
  { id: 'tmdb:55120', title: 'Dockside Stories (2019)', quality: 'Documentary · 54m', size: 0, score: 40, reason: 'Partial title match', indexer: 'tmdb:55120' },
];
export const rootFolders: PathLocation[] = [
  { path: '/data/media/movies', label: 'Movies', free: 1.82e12, total: 8e12 },
  { path: '/data/media/movies-4k', label: 'Movies 4K', free: 410e9, total: 4e12 },
];

// Activity › Queue
export const downloads: QueueCardData[] = [
  { id: 'q1', title: 'Dockside', kind: 'movie', subtitle: '2026 · WEB-DL 1080p', state: 'downloading', progress: 0.46, size: 6.2e9, downloaded: 2.85e9, speed: 12.4e6, eta: 270, href: '#/activity/queue/harbourbox/q1' },
  { id: 'q2', title: 'The Long Haul', kind: 'tv', subtitle: 'S02E03 · Customs', state: 'stalled', reason: 'no peers for 2h', progress: 0.81, size: 2.4e9, downloaded: 1.94e9, speed: 0, rule: { badge: 'stalled', at: NOW + 245_000, action: 'Removes' }, href: '#/activity/queue/harbourbox/q2' },
  { id: 'q3', title: 'The Long Haul', kind: 'tv', subtitle: 'S01 · 13 episodes', state: 'import-blocked', reason: 'sample only', size: 21e9, rule: { badge: 'stuck-import', at: NOW + 3 * hour, action: 'Retries', dryRun: true }, href: '#/activity/queue/harbourbox/q3' },
  { id: 'q4', title: 'Harbour Lights', kind: 'music', subtitle: 'Album · 11 tracks · FLAC', state: 'queued', paused: true, held: true, progress: 0.1, size: 412e6, downloaded: 41e6, href: '#/activity/queue/harbourbox/q4' },
  { id: 'q5', title: 'Quay Lights', kind: 'movie', subtitle: '2024 · Bluray 1080p', state: 'importing', progress: 1, size: 9.1e9, downloaded: 9.1e9, href: '#/activity/queue/harbourbox/q5' },
];

// Activity › History
export interface HistoryRow { id: string; at: number; type: string; title: string; kind: MediaKind; outcome: { family: 'importOutcome' | 'download' | 'verdict'; value: string }; source: string; actor: string; detail: string }
const hTypes: [string, HistoryRow['outcome'], string][] = [
  ['Imported', { family: 'importOutcome', value: 'imported' }, 'Moved to /data/media/movies/Quay Lights (2024)/Quay Lights (2024) Bluray-1080p.mkv'],
  ['Grabbed', { family: 'download', value: 'queued' }, 'Accepted rank 1 of 9, score +1250 (DD+ 5.1, trusted group)'],
  ['Import blocked', { family: 'importOutcome', value: 'blocked' }, 'Only a sample file was found (48 MB); the release looks incomplete'],
  ['Upgraded', { family: 'importOutcome', value: 'upgraded' }, 'Replaced HDTV 720p with WEB-DL 1080p; previous file kept in the recycle area for 30 days'],
  ['Rejected', { family: 'verdict', value: 'reject' }, 'Over the 40 GB per-movie cap; 0 seeders'],
  ['Removed', { family: 'download', value: 'removed' }, 'Stalled for 2 hours; rule "stalled downloads" removed it and searched again'],
];
export const historyRows: HistoryRow[] = Array.from({ length: 60 }, (_, i) => {
  const [type, outcome, detail] = hTypes[i % hTypes.length];
  return {
    id: `e${i}`, at: NOW - i * 47 * min, type, outcome, detail,
    title: [movies[i % 20].title, 'The Long Haul S02E0' + ((i % 8) + 1), 'Harbour Lights'][i % 3], kind: (['movie', 'tv', 'music'] as MediaKind[])[i % 3],
    source: ['Harbour Tracker', 'Dock Feeds', 'Usenet Pier'][i % 3], actor: ['Automatic', 'You', 'Rule: stalled downloads', 'Feed sync'][i % 4],
  };
});

// Wanted › Missing
export interface WantedRow { id: string; title: string; parent?: string; kind: MediaKind; profile: string; availableSince: number; lastSearched?: number; nextEligible?: number; lastResult: string }
export const wantedRows: WantedRow[] = Array.from({ length: 40 }, (_, i) => ({
  id: `w${i}`,
  title: i % 3 === 0 ? `S02E0${(i % 9) + 1} · ${['Customs', 'Low Tide', 'Berth', 'Night Shift'][i % 4]}` : movies[(i * 3) % 30].title,
  parent: i % 3 === 0 ? 'The Long Haul' : undefined,
  kind: i % 3 === 0 ? 'tv' : i % 5 === 0 ? 'music' : 'movie',
  profile: i % 4 === 0 ? 'UHD' : 'HD',
  availableSince: NOW - (i + 1) * 2 * day,
  lastSearched: i % 6 === 5 ? undefined : NOW - ((i * 3) % 20 + 1) * hour,
  nextEligible: NOW + ((i * 5) % 12 + 1) * hour,
  lastResult: ['9 found, none accepted (below cutoff)', 'Nothing found', '3 found, 1 accepted: grabbed', 'Indexers failed: 1 of 4'][i % 4],
}));

// System › Health
export interface HealthRow { id: string; name: string; type: string; status: 'ok' | 'degraded' | 'backed_off' | 'failing' | 'action_needed' | 'disabled'; since: number; lastOk?: number; message?: string; errorClass?: string; failures?: number; nextRetry?: number; fix?: string; fixHref?: string; extra?: string }
export const healthRows: HealthRow[] = [
  { id: 'nzbgeek', name: 'Usenet Pier', type: 'Indexer', status: 'action_needed', since: NOW - 3 * hour, lastOk: NOW - 3 * hour, errorClass: 'auth', message: 'The API key was refused (401).', failures: 6, fix: 'Paste a new API key in the indexer settings, then test it.', fixHref: '#/settings/indexers/usenet-pier' },
  { id: 'slug', name: 'Slow Slug', type: 'Indexer', status: 'backed_off', since: NOW - 40 * min, lastOk: NOW - 50 * min, errorClass: 'unavailable', message: '503 Service Unavailable', failures: 3, nextRetry: NOW + 20 * min },
  { id: 'box', name: 'Harbour box', type: 'Download client', status: 'degraded', since: NOW - 2 * day, lastOk: NOW - 2 * min, message: 'Free space 41.2 GB is under the 50 GB floor.', fix: 'Free space on /downloads or lower the floor.', extra: 'qBittorrent 5.1.2 · 41.2 GB free' },
  { id: 'harbour', name: 'Harbour Tracker', type: 'Indexer', status: 'ok', since: NOW - 12 * day },
  { id: 'dock', name: 'Dock Feeds', type: 'Indexer', status: 'ok', since: NOW - 4 * day },
  { id: 'tmdb', name: 'TMDB', type: 'Metadata', status: 'ok', since: NOW - 30 * day },
  { id: 'plex', name: 'Living room Plex', type: 'Media server', status: 'ok', since: NOW - 6 * day },
  { id: 'tg', name: 'Telegram', type: 'Notification', status: 'disabled', since: NOW - 9 * day, message: 'Turned off by you.' },
];

// System › Tasks
export interface TaskRow { id: string; name: string; schedule: string; enabled: boolean; lastRun?: number; duration?: number; result?: 'done' | 'failed' | 'running' | 'queued' | 'cancelled'; nextRun?: number; note?: string; progress?: number }
export const taskRows: TaskRow[] = [
  { id: 'rss', name: 'Feed sync', schedule: 'Every 15m (adaptive)', enabled: true, lastRun: NOW - 6 * min, duration: 1.8, result: 'done', nextRun: NOW + 9 * min, note: 'Two indexers asked for 15 minutes' },
  { id: 'search', name: 'Search for wanted', schedule: 'Every 6h', enabled: true, lastRun: NOW - 2 * hour, duration: 94, result: 'done', nextRun: NOW + 4 * hour },
  { id: 'refresh', name: 'Refresh metadata', schedule: 'Daily at 04:00', enabled: true, lastRun: NOW - 10 * hour, duration: 312, result: 'failed', nextRun: NOW + 14 * hour, note: 'TMDB timed out for 3 items' },
  { id: 'scan', name: 'Scan library folders', schedule: 'Every 12h', enabled: true, result: 'running', progress: 0.62, lastRun: NOW - 3 * min, nextRun: NOW + 12 * hour },
  { id: 'backup', name: 'Back up database', schedule: 'Weekly, Sunday 03:00', enabled: true, lastRun: NOW - 4 * day, duration: 8, result: 'done', nextRun: NOW + 3 * day },
  { id: 'clean', name: 'Empty recycle area', schedule: 'Daily at 05:00', enabled: false, lastRun: NOW - 20 * day, duration: 2, result: 'done' },
];

// Settings › Indexers: a Torznab instance
export const torznabSchema: SettingsSchema = {
  groups: [
    { id: 'connection', label: 'Connection', help: 'Where corsarr reaches the Torznab feed.' },
    { id: 'search', label: 'Search' },
  ],
  fields: [
    { key: 'name', type: 'text', label: 'Name', required: true },
    { key: 'url', type: 'url', label: 'Feed address', group: 'connection', required: true, placeholder: 'https://tracker.example.org/api/torznab', validation: { schemes: ['http', 'https'] } },
    { key: 'apiKey', type: 'text', label: 'API key', group: 'connection', secret: true, required: true, help: 'Paste the key, or a reference like ${env:HARBOUR_API_KEY}.' },
    { key: 'categories', type: 'multiselect', label: 'Categories', group: 'search', options: [
      { value: '2000', label: 'Movies' }, { value: '5000', label: 'TV' }, { value: '3000', label: 'Audio' }, { value: '7000', label: 'Books' },
    ] },
    { key: 'minSeeders', type: 'number', label: 'Minimum seeders', group: 'search', default: 1, validation: { min: 0, max: 100 } },
    { key: 'priority', type: 'number', label: 'Priority', group: 'search', default: 25, help: 'Lower numbers are preferred when releases tie.', validation: { min: 1, max: 50 } },
    { key: 'rss', type: 'bool', label: 'Use for feed sync', group: 'search', default: true },
    { key: 'interval', type: 'duration', label: 'Feed interval', group: 'search', default: 900, durationUnits: ['m', 'h'], visibleWhen: { field: 'rss', equals: true } },
  ],
};
export const torznabValues: FormValues = { name: 'Harbour Tracker', url: 'https://harbour.example.org/api/torznab', categories: ['2000', '5000'], minSeeders: 1, priority: 25, rss: true, interval: 900 };
export const torznabMeta: Record<string, FieldMeta> = { apiKey: { ref: '${env:HARBOUR_API_KEY}' } };
export const torznabManagedMeta: Record<string, FieldMeta> = {
  name: { managed: 'indexers.dock_feeds.name' }, url: { managed: 'indexers.dock_feeds.url' }, apiKey: { managed: 'indexers.dock_feeds.api_key', ref: '${file:/run/secrets/dock}' },
  categories: { managed: 'indexers.dock_feeds.categories' }, minSeeders: { managed: 'indexers.dock_feeds.min_seeders' },
  priority: { managed: 'indexers.dock_feeds.priority' }, rss: { managed: 'indexers.dock_feeds.rss' }, interval: { managed: 'indexers.dock_feeds.interval' },
};

// Settings › Notifications: shoutrrr services
export const telegramSchema: SettingsSchema = {
  fields: [
    { key: 'token', type: 'text', label: 'Bot token', secret: true, required: true, help: 'From @BotFather. Stored write-only.' },
    { key: 'chats', type: 'list', label: 'Chats', itemLabel: 'chat', required: true, help: 'Chat or channel IDs, like -1001234567890 or @dockside_alerts.', mono: true },
    { key: 'preview', type: 'bool', label: 'Show link previews', default: false },
    { key: 'silent', type: 'bool', label: 'Send silently at night', default: true },
  ],
};
export const ntfySchema: SettingsSchema = {
  fields: [
    { key: 'host', type: 'url', label: 'Server', default: 'https://ntfy.sh', required: true },
    { key: 'topic', type: 'text', label: 'Topic', required: true, mono: true },
    { key: 'priority', type: 'select', label: 'Priority', default: 'default', options: [{ value: 'low', label: 'Low' }, { value: 'default', label: 'Default' }, { value: 'high', label: 'High' }] },
  ],
};
export const pushoverSchema: SettingsSchema = {
  fields: [
    { key: 'user', type: 'text', label: 'User key', secret: true, required: true },
    { key: 'token', type: 'text', label: 'Application token', secret: true, required: true },
    { key: 'devices', type: 'list', label: 'Devices', itemLabel: 'device' },
  ],
};

// Settings section list
export const settingsSections: SettingsSection[] = [
  { id: 'general', label: 'General', href: '#/settings/general' },
  { id: 'media-kinds', label: 'Media kinds', href: '#/settings/media-kinds' },
  { id: 'profiles', label: 'Quality & profiles', href: '#/settings/profiles', count: 3 },
  { id: 'indexers', label: 'Indexers & sources', href: '#/settings/indexers', count: 4, attention: true },
  { id: 'download-clients', label: 'Download clients', href: '#/settings/download-clients', count: 1 },
  { id: 'import', label: 'Import & naming', href: '#/settings/import' },
  { id: 'automation', label: 'Automation', href: '#/settings/automation', count: 5 },
  { id: 'connections', label: 'Connections', href: '#/settings/connections', count: 2 },
  { id: 'metadata', label: 'Metadata', href: '#/settings/metadata' },
  { id: 'plugins', label: 'Plugins', href: '#/settings/plugins' },
  { id: 'configuration', label: 'Configuration', href: '#/settings/configuration' },
];

// Dashboard
export const dashboardAttention = [
  { id: 'a1', title: 'The Long Haul · S01, 13 episodes', detail: 'Import blocked: only a sample file was found. Open the download to import by hand or replace it.', status: { family: 'download' as const, value: 'import-blocked' }, kind: 'tv' as MediaKind, href: '#/activity/queue/harbourbox/q3' },
  { id: 'a2', title: 'Usenet Pier', detail: 'The API key was refused 6 times since 11:05. Paste a new key to resume searches.', status: { family: 'health' as const, value: 'action_needed' }, href: '#/settings/indexers/usenet-pier' },
];
export const dashboardTiles = [
  { label: 'Wanted', value: 34, href: '#/wanted/missing', linkLabel: 'Open wanted' },
  { label: 'Imported today', value: 7, trend: { delta: 2, better: 'up' as const, period: 'vs yesterday' } },
  { label: 'Rule actions, 24 h', value: 3, href: '#/activity/automation', linkLabel: 'Review' },
  { label: 'Free on /data/media', value: 1.82e12, kind: 'bytes' as const },
];
export const dashboardHealth = {
  ok: 5, total: 8,
  problems: [
    { id: 'pier', name: 'Usenet Pier', status: 'action_needed', message: 'API key refused (401)', href: '#/system/health' },
    { id: 'slug', name: 'Slow Slug', status: 'backed_off', message: '503; retrying in 20m', href: '#/system/health' },
    { id: 'box', name: 'Harbour box', status: 'degraded', message: '41.2 GB free, under the 50 GB floor', href: '#/system/health' },
  ],
};
export const dashboardHistory: TimelineEntry[] = [
  { id: 'r1', kind: 'transition', at: NOW - 12 * min, title: 'Imported Quay Lights (2024)', to: { family: 'download', value: 'imported' } },
  { id: 'r2', kind: 'apply', at: NOW - 2 * hour, title: 'Grabbed Dockside (2026), WEB-DL 1080p', status: { family: 'delivery', value: 'delivered' } },
  { id: 'r3', kind: 'attempt', at: NOW - 3 * hour, title: 'Rule removed a stalled download: Low Water', status: { family: 'job', value: 'done' }, actor: 'Rule: stalled downloads' },
  { id: 'r4', kind: 'transition', at: NOW - 5 * hour, title: 'Upgraded The Long Haul S02E02 to WEB-DL 1080p', to: { family: 'download', value: 'imported' } },
];

// Settings › General (General)
export const generalSchema: SettingsSchema = {
  groups: [{ id: 'server', label: 'Network' }, { id: 'upkeep', label: 'Logs and backups' }],
  fields: [
    { key: 'listen', type: 'url', label: 'Listen address', group: 'server', required: true, help: 'Where the web UI and API answer. Changing it needs a restart.' },
    { key: 'basePath', type: 'text', label: 'Base path', group: 'server', placeholder: '/corsarr', help: 'Set when a reverse proxy serves corsarr under a sub-path.', mono: true },
    { key: 'logLevel', type: 'select', label: 'Log level', group: 'upkeep', default: 'info', options: [
      { value: 'error', label: 'Errors only' }, { value: 'warn', label: 'Warnings' }, { value: 'info', label: 'Info' }, { value: 'debug', label: 'Debug' },
    ] },
    { key: 'backupKeep', type: 'number', label: 'Backups to keep', group: 'upkeep', default: 8, validation: { min: 1, max: 60 } },
  ],
};
export const generalValues: FormValues = { listen: 'http://0.0.0.0:8090', basePath: '', logLevel: 'info', backupKeep: 8 };
export const apiKey = '7731e0a94f1e9b2dc0a1harbour5e2f';

// Settings › Indexers list
export interface IndexerRow { id: string; name: string; plugin: string; status: HealthRow['status']; priority: number; categories: string; managed?: string }
export const indexerRows: IndexerRow[] = [
  { id: 'harbour', name: 'Harbour Tracker', plugin: 'Torznab', status: 'ok', priority: 25, categories: 'Movies, TV' },
  { id: 'dock', name: 'Dock Feeds', plugin: 'Torznab', status: 'ok', priority: 20, categories: 'Movies, TV, Audio', managed: 'indexers.dock_feeds' },
  { id: 'pier', name: 'Usenet Pier', plugin: 'Newznab', status: 'action_needed', priority: 30, categories: 'Movies, TV' },
  { id: 'slug', name: 'Slow Slug', plugin: 'Torznab', status: 'backed_off', priority: 40, categories: 'Books' },
];

// Settings › Notifications
export const notifyEvents = [
  { id: 'grab.sent', label: 'Release grabbed', group: 'Downloads' },
  { id: 'import.completed', label: 'Imported', group: 'Downloads' },
  { id: 'import.blocked', label: 'Import blocked', description: 'Needs a decision from you', group: 'Downloads' },
  { id: 'health.changed', label: 'Integration needs attention', group: 'System' },
  { id: 'rule.acted', label: 'Rule acted on a download', group: 'System' },
];
export const notifyColumns = [
  { id: 'telegram', label: 'Telegram' },
  { id: 'ntfy', label: 'ntfy' },
  { id: 'pushover', label: 'Pushover' },
  { id: 'priority', label: 'Priority', kind: 'select' as const, options: [{ value: 'low', label: 'Low' }, { value: 'normal', label: 'Normal' }, { value: 'high', label: 'High' }] },
];
export const notifyMatrix: Record<string, Record<string, boolean | string>> = {
  'grab.sent': { telegram: true, priority: 'low' },
  'import.completed': { telegram: true, ntfy: true, priority: 'normal' },
  'import.blocked': { telegram: true, ntfy: true, pushover: true, priority: 'high' },
  'health.changed': { ntfy: true, pushover: true, priority: 'high' },
  'rule.acted': { telegram: true, priority: 'normal' },
};
export const telegramValues: FormValues = { chats: ['@dockside_alerts'], preview: false, silent: true };
export const telegramMeta: Record<string, FieldMeta> = { token: { ref: '${env:TELEGRAM_BOT_TOKEN}' } };
export const ntfyValues: FormValues = { host: 'https://ntfy.sh', topic: 'dockside-alerts-7731', priority: 'default' };
export const pushoverMeta: Record<string, FieldMeta> = { user: { isSet: true }, token: { isSet: true } };
