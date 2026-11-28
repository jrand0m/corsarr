// Wire types of the corsarr /api/v1 surface the web UI reads (Go: internal/domain, internal/api,
// internal/{movies,queue,automation,importer,plugin}). Only the fields the screens use are typed.
import type { ManagedSource } from '../../lib/components/atoms/ManagedValue/ManagedValue.svelte';
import type { MediaKind } from '../../lib/tokens';

export interface ItemRef { kind: MediaKind; id: number }

export interface Movie {
  id: number;
  tmdbId: number;
  imdbId?: string;
  title: string;
  originalTitle?: string;
  year: number;
  runtimeMin?: number;
  inCinemas?: string;
  digital?: string;
  physical?: string;
  monitored: boolean;
  minAvailability: 'announced' | 'inCinemas' | 'released';
  profileKey: string;
  rootFolder: string;
  path: string;
  fileId?: number;
  addedAt: string;
  /** Cached artwork path (relative, like every API path); absent when the provider has none. */
  posterUrl?: string;
  fanartUrl?: string;
}

export interface MovieRecord {
  ids: Record<string, string>;
  title: string;
  originalTitle?: string;
  year: number;
  overview?: string;
  runtimeMin?: number;
  status?: string;
  inCinemas?: string;
  digital?: string;
  physical?: string;
  posterUrl?: string;
  genres?: string[];
}

export interface AddMovieRequest {
  tmdbId: number;
  rootFolder: string;
  profileKey: string;
  monitored: boolean;
  minAvailability: string;
  searchNow: boolean;
}

// ── TV ─────────────────────────────────────────────────────────────────

export type SeriesStatus = 'continuing' | 'ended' | 'upcoming' | '';
export type SeriesType = 'standard' | 'anime' | 'daily';
export type MonitorMode = 'all' | 'future' | 'missing' | 'existing' | 'firstSeason' | 'latestSeason' | 'pilot' | 'none';

export interface Series {
  id: number;
  tmdbId: number;
  tvdbId?: number;
  imdbId?: string;
  title: string;
  sortTitle?: string;
  year: number;
  status: SeriesStatus;
  network?: string;
  overview?: string;
  genres?: string[] | null;
  runtimeMin?: number;
  altTitles?: string[] | null;
  posterUrl?: string;
  fanartUrl?: string;
  rootFolder: string;
  path: string;
  profileKey: string;
  tags?: string[] | null;
  seriesType: SeriesType;
  seasonFolders: boolean;
  monitorNew: 'all' | 'none';
  monitored: boolean;
  addedAt: string;
}

export interface SeriesStats {
  seasonCount: number;
  /** Monitored episodes that have aired. */
  episodeCount: number;
  episodeFileCount: number;
  totalEpisodeCount: number;
  sizeOnDisk: number;
  nextAiring?: string | null;
  previousAiring?: string | null;
}

export interface SeriesSummary extends Series { stats: SeriesStats }

export interface SeriesRecord {
  ids: { tmdb: string; tvdb?: string; imdb?: string };
  title: string;
  year: number;
  network?: string;
  status?: SeriesStatus;
  overview?: string;
  posterUrl?: string;
  seasonCount?: number;
  genres?: string[] | null;
  runtimeMin?: number;
  /** 0 when the series is not in the library. */
  libraryId: number;
}

export interface Season {
  number: number;
  monitored: boolean;
  episodeCount: number;
  airedCount: number;
  monitoredCount: number;
  fileCount: number;
  sizeOnDisk: number;
}

export type EpisodeStatus = 'downloaded' | 'missing' | 'notAired' | 'unmonitored';

export interface Episode {
  id: number;
  seriesId: number;
  season: number;
  number: number;
  absolute?: number;
  title: string;
  airAt: string | null;
  runtimeMin?: number;
  monitored: boolean;
  fileId?: number;
  inMetadata: boolean;
  status: EpisodeStatus;
  file?: { path: string; size: number; quality: string; cutoffMet: boolean };
}

export interface SeriesDetail { series: SeriesSummary; seasons: Season[] | null; episodes: Episode[] | null }

export interface WantedEpisode {
  episode: Episode;
  series: { id: number; title: string; year: number; posterUrl?: string };
  reason: 'missing' | 'cutoff_unmet';
  available: boolean;
}

export interface QualityProfile { key: string; name: string; kind: string; cutoff: string; upgrades: boolean; isDefault: boolean }

export interface RootFolder {
  path: string; kind: MediaKind; accessible: boolean; freeSpace: number;
  writable?: boolean; problem?: '' | 'missing' | 'read-only' | 'full'; lastChecked?: string;
}

export type DownloadState =
  | 'queued' | 'downloading' | 'stalled' | 'completed' | 'import-pending' | 'importing' | 'import-blocked' | 'imported' | 'failed' | 'removed';

export interface Download {
  id: number;
  downloader: string;
  downloadId: string;
  protocol: string;
  kind: MediaKind;
  grabId: number;
  title: string;
  state: DownloadState;
  links: { item: ItemRef; status: string }[] | null;
  progress: number;
  size: number;
  downloaded: number;
  speed: number;
  eta: number;
  seeders: number;
  peers: number;
  category: string;
  held: boolean;
  statusReason?: string;
  addedAt: string;
}

export type ReasonCategory = 'safety' | 'permanent' | 'preference' | 'temporary';
export interface Reason { ruleId: string; category: ReasonCategory; message: string; summary?: string }

export interface Release {
  id: string;
  title: string;
  protocol: 'torrent' | 'usenet' | 'source';
  size: number;
  publishedAt: string;
  indexers: { key: string; name: string }[] | null;
  infoHash?: string;
  guid?: string;
  seeders: number;
  peers: number;
  grabs?: number;
  /** The indexer sent seeders or peers (absent: zero may mean "not reported"). */
  swarmReported?: boolean;
  files?: number;
  minimumRatio?: number;
}

export type Position = 'above' | 'on_target' | 'below';

/** How close a release is to the profile's target; absent when its quality is outside the profile. */
export interface Desirability {
  score: number;
  position: Position;
  qualitySteps: number;
  cutoff: string;
  sizeDelta?: number;
  preferredSize?: number;
  formatShortfall?: number;
  formatTarget?: number;
}

export type SwarmHealth = 'unknown' | 'none' | 'few' | 'some' | 'good';
export interface Swarm { seeders: number; leechers?: number; reported: boolean; health: SwarmHealth }

export type AvailabilityStatus = 'complete' | 'may_be_incomplete' | 'no_peers' | 'unknown';
export interface Availability { status: AvailabilityStatus; grabs?: number; files?: number; minimumRatio?: number }

export type OutcomeStatus = 'grab' | 'shadow' | 'outranked' | 'declined';
export interface Outcome { status: OutcomeStatus; lostTo?: { criterion: string; message: string } }
/** What automatic search would do with a result set. */
export interface AutomationVerdict { mode: 'active' | 'shadow'; pick?: string; outcomes: Record<string, Outcome> }

export interface Decision {
  releaseId: string;
  accepted: boolean;
  reasons: Reason[] | null;
  quality: string;
  resolution: number;
  formatScore: number;
  rankKey: string;
  override: 'allowed' | 'confirm' | 'never';
  desirability?: Desirability;
}

export interface Decided { release: Release; decision: Decision; swarm?: Swarm; availability?: Availability }

/** One line of GET /movies/{id}/releases (NDJSON). */
export type ReleaseLine =
  | { record: Decided }
  | { error: { source: string; message: string } }
  | { summary: { done: true; count: number; errors: number; indexers: number; failed: number; releases: number; accepted: number;
      automation?: AutomationVerdict | null; rejections?: ReasonCount[] | null } };

export interface SearchSummary { searches?: number; indexers: number; failed: number; releases: number; accepted: number; grabbed?: string; shadowed?: string }

export interface ReasonCount { ruleId: string; category: string; message: string; count: number }

export interface SearchState {
  lastSearched?: string;
  nextEligible?: string;
  misses: number;
  lastResult?: { indexers: number; failed: number; releases: number; accepted: number; grabbed?: string; rejections?: ReasonCount[] };
}

export interface WantedEntry { movie: Movie; reason: string; available: boolean; search?: SearchState }

export interface BlocklistEntry {
  id: number;
  item: ItemRef;
  release: { indexer?: string; guid?: string; infoHash?: string; title: string };
  source: string;
  reason: string;
  addedAt: string;
}

export interface LiveEvent {
  eventId: string;
  type: string;
  occurredAt: string;
  actor: string;
  mediaKind?: MediaKind;
  items?: { ref: ItemRef & { variant?: string }; title: string; series?: string }[];
  data?: Record<string, unknown>;
}

export interface HistoryEntry { id: number; event: LiveEvent }

export type HealthStatus = 'ok' | 'degraded' | 'backed_off' | 'failing' | 'action_needed' | 'disabled';
export interface IntegrationHealth {
  ref: { type: string; key: string };
  status: HealthStatus;
  since: string;
  lastOkAt?: string;
  lastErrorClass?: string;
  lastErrorMessage?: string;
  consecutiveFailures: number;
  nextRetryAt?: string;
  fixHint?: string;
}

/** One window's learned rate limit and how it was found. */
export interface RateCeiling {
  windowSeconds: number;
  limit: number;
  source: 'advertised' | 'declared' | 'throttle' | 'probe';
  foundAt: string;
}

/** What the outbound client learned about one service's rate limit. */
export interface RateLimitStatus {
  service: string;
  ceilings: RateCeiling[];
  recent: { windowSeconds: number; count: number }[];
  /** Requests per second allowed now; 0 = not paced. */
  pace: number;
  probe: 'normal' | 'gentle' | 'off';
  probing: boolean;
  lastThrottle?: string;
  checkedAt?: string;
  nextCheck?: string;
  heldUntil?: string;
}

export interface RateLimits {
  enabled: boolean;
  items: RateLimitStatus[] | null;
}

export interface JobState {
  name: string;
  enabled: boolean;
  heavy: boolean;
  running: boolean;
  lastStart?: string;
  lastFinish?: string;
  lastResult: '' | 'ok' | 'failed';
  lastError?: string;
  failures: number;
  nextRun?: string;
  intervalSeconds: number;
}

export interface BuildInfo {
  version: string;
  commit: string;
  tag: string;
  goVersion: string;
  buildDate: string;
  dirty: boolean;
}

export interface SystemStatus {
  version: string;
  uptimeSeconds: number;
  configSource: { kind: 'file' | 'args' | 'env' | 'defaults' | string; path?: string; ignored?: string[] };
  enabledKinds: MediaKind[];
  pluginCount: number;
  build?: BuildInfo;
}

export interface PluginField {
  key: string;
  type: 'text' | 'secret' | 'number' | 'bool' | 'enum' | 'multiselect' | 'duration' | 'size' | 'url' | 'path' | 'regex' | 'list' | 'map';
  label: string;
  help?: string;
  required?: boolean;
  default?: unknown;
  options?: string[];
  showIf?: string;
  min?: number;
  max?: number;
  secret?: boolean;
}

export interface PluginManifest {
  id: string;
  kind: string;
  version: string;
  apiVersion: number;
  name: string;
  delivery: string;
  capabilities?: string[];
  settings: PluginField[];
}

/** An indexer from the config file or the web UI (secret values never included). */
export interface IndexerRow {
  key: string;
  name: string;
  plugin: string;
  pluginName: string;
  delivery: string;
  source: 'config' | 'ui';
  /** The config key and source that set a config indexer. */
  managedBy?: ManagedSource;
  enabled: boolean;
  running: boolean;
  priority: number;
  settings: Record<string, unknown>;
  secretsSet: string[];
  health?: IntegrationHealth;
  torznab: string;
}

/** A tracker definition file: bundled or the owner's, valid or with its problems. */
export interface DefinitionEntry {
  id: string;
  name: string;
  description?: string;
  language?: string;
  privacy?: 'public' | 'semi-private' | 'private';
  version?: string;
  origin: 'bundled' | 'user';
  file: string;
  overrides?: boolean;
  valid: boolean;
  problems?: { path: string; line: number; message: string }[];
  login?: string;
  modes?: string[];
}

/** What a connection test found. */
export interface IndexerTest {
  ok: boolean;
  class?: string;
  message: string;
  caps?: { modes: string[] | null; idParams: string[] | null; categories: Record<string, string> | null; limit: number };
  elapsedMs: number;
}

/** A create or edit request; omitted fields keep their value on edit. */
export interface IndexerInput {
  key?: string;
  plugin?: string;
  name?: string;
  enabled?: boolean;
  settings?: Record<string, unknown>;
}

export interface TorznabState {
  enabled: boolean;
  all: string;
}

// ── music ──────────────────────────────────────────────────────────────────
export type AlbumType = 'album' | 'ep' | 'single' | 'broadcast' | 'other';
export type AlbumMonitor = 'all' | 'missing' | 'existing' | 'future' | 'first' | 'latest' | 'none';
export type MonitorNewItems = 'all' | 'new' | 'none';
export type AlbumStatus = 'downloaded' | 'partial' | 'missing' | 'notReleased' | 'unmonitored';

export interface Artist {
  id: number;
  mbid: string;
  name: string;
  sortName: string;
  disambiguation?: string;
  type?: string;
  country?: string;
  rootFolder: string;
  path: string;
  profileKey: string;
  metadataProfile: string;
  monitorNew: MonitorNewItems;
  monitored: boolean;
  tags: string[];
  addedAt: string;
}

export interface ArtistStats {
  albumCount: number;
  totalAlbums: number;
  monitoredAlbums: number;
  albumsWithFiles: number;
  trackFileCount: number;
  sizeOnDisk: number;
  nextRelease: string | null;
  lastRelease: string | null;
}

export interface ArtistSummary extends Artist { stats: ArtistStats; coverUrl?: string }

export interface ArtistRecord {
  mbid: string;
  name: string;
  sortName: string;
  disambiguation?: string;
  type?: string;
  country?: string;
  beginYear?: number;
  libraryId: number;
}

export interface Album {
  id: number;
  artistId: number;
  mbid: string;
  title: string;
  primaryType: AlbumType;
  secondaryTypes: string[] | null;
  releaseDate: string | null;
  monitored: boolean;
  pinReason?: '' | 'owner' | 'list' | 'import';
  editionMbid?: string;
  editionLocked: boolean;
  mediumCount: number;
  trackCount: number;
  durationSec: number;
}

export interface AlbumView extends Album {
  status: AlbumStatus;
  inProfile: boolean;
  fileCount: number;
  sizeOnDisk: number;
  quality?: string;
  cutoffMet: boolean;
  coverUrl?: string;
}

export interface ArtistDetail { artist: ArtistSummary; albums: AlbumView[] | null }

export interface Track {
  id: number;
  albumId: number;
  recordingMbid?: string;
  medium: number;
  mediumFormat?: string;
  number: number;
  title: string;
  durationSec: number;
  fileId?: number;
  file?: { path: string; size: number; quality: string };
}

export interface AlbumDetail { album: AlbumView; artist: { id: number; name: string }; tracks: Track[] | null }

export interface MusicProfileOption { key: string; name: string }
export interface MusicProfiles { quality: (MusicProfileOption & { cutoff: string; isDefault: boolean })[]; metadata: MusicProfileOption[] }

/** One run of the artwork cache's shrinking (counts and bytes freed). */
export interface CacheRun { at: string; reencoded: number; resized: number; evicted: number; metadataEvicted: number; bytesFreed: number }

/** GET system/cache: the artwork cache's usage. */
export interface CacheStatus {
  enabled: boolean;
  dir: string;
  maxSize: number;
  usage: number;
  imageBytes: number;
  metadataBytes: number;
  images: number;
  variants: number;
  misses: number;
  requests: number;
  hits: number;
  hitRate: number;
  highWater: number;
  lowWater: number;
  lastRun?: CacheRun;
}
