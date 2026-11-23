// The one SSE connection of the app. It owns the connection state the
// AppShell banner shows, fans events out by type ("download.progress") or family ("download.*"), and
// tells screens when the stream came back so they refetch what they show: there is no replay and no
// polling anywhere.
import type { LiveEvent } from './api/types';
import type { Connection } from '../lib/components/templates/AppShell/AppShell.svelte';

/** Every streamed type of the server's event catalogue; EventSource needs a listener per named event. */
export const EVENT_TYPES = [
  'item.added', 'item.updated', 'item.deleted', 'item.available', 'search.completed', 'grab.sent', 'grab.failed', 'decision.shadowed',
  'download.progress', 'download.state_changed', 'download.completed', 'download.failed', 'download.removed',
  'import.completed', 'import.blocked', 'import.failed', 'payload.rejected', 'file.renamed', 'file.deleted',
  'blocklist.added', 'blocklist.removed', 'rule.acted', 'rule.run_completed', 'rule.undone', 'health.changed',
  'config.applied', 'config.apply_failed', 'config.drifted', 'mediakind.mode_changed', 'plugin.loaded', 'job.updated',
  'command.updated',
] as const;

export type Handler = (ev: LiveEvent) => void;

/** The part of EventSource the store uses (tests pass a fake). */
export interface SourceLike {
  readyState: number;
  onopen: ((e: Event) => void) | null;
  onerror: ((e: Event) => void) | null;
  addEventListener(type: string, fn: (e: MessageEvent) => void): void;
  close(): void;
}
export type SourceFactory = (url: string) => SourceLike;

const CLOSED = 2;

/** True when the pattern ("download.*", "health.changed", "*") covers the type. */
export function matches(pattern: string, type: string): boolean {
  if (pattern === '*') return true;
  return pattern.endsWith('.*') ? type.startsWith(pattern.slice(0, -1)) : pattern === type;
}

export class LiveEvents {
  connection = $state<Connection>('live');
  offlineSince = $state<string | undefined>();

  readonly #url: string;
  readonly #factory: SourceFactory;
  readonly #offlineAfter: number;
  #source: SourceLike | null = null;
  #handlers = new Set<{ pattern: string; fn: Handler }>();
  #reconnect = new Set<() => void>();
  #opened = false;
  #lostAt = 0;
  #timer: ReturnType<typeof setTimeout> | undefined;

  constructor(url: string, factory: SourceFactory = (u) => new EventSource(u, { withCredentials: true }), offlineAfterMs = 20_000) {
    this.#url = url;
    this.#factory = factory;
    this.#offlineAfter = offlineAfterMs;
  }

  /** Opens the stream (idempotent). */
  start(): void {
    if (this.#source) return;
    const src = this.#factory(this.#url);
    this.#source = src;
    src.onopen = () => this.#onopen();
    src.onerror = () => this.#onerror();
    for (const t of EVENT_TYPES) src.addEventListener(t, (e) => this.#dispatch(t, e));
  }

  stop(): void {
    clearTimeout(this.#timer);
    this.#source?.close();
    this.#source = null;
    this.#opened = false;
  }

  /** Reopens after the browser gave up (the banner's "Reconnect"). */
  reconnect(): void {
    this.stop();
    this.#wasLost();
    this.start();
  }

  /** Subscribes to a type or family; returns the unsubscribe function. */
  on(pattern: string, fn: Handler): () => void {
    const h = { pattern, fn };
    this.#handlers.add(h);
    return () => this.#handlers.delete(h);
  }

  /** Called after the stream came back from a drop (not on the first open). */
  onreconnect(fn: () => void): () => void {
    this.#reconnect.add(fn);
    return () => this.#reconnect.delete(fn);
  }

  /** Emits as if the server sent it (screens use it after their own actions; tests use it). */
  emit(ev: LiveEvent): void {
    for (const h of [...this.#handlers]) if (matches(h.pattern, ev.type)) h.fn(ev);
  }

  #dispatch(type: string, e: MessageEvent) {
    let ev: LiveEvent;
    try {
      ev = JSON.parse(String(e.data)) as LiveEvent;
    } catch {
      return;
    }
    this.emit({ ...ev, type: ev.type || type });
  }

  #onopen() {
    clearTimeout(this.#timer);
    const back = this.#opened && this.#lostAt > 0;
    this.#opened = true;
    this.#lostAt = 0;
    this.connection = 'live';
    this.offlineSince = undefined;
    if (back) for (const fn of [...this.#reconnect]) fn();
  }

  #wasLost() {
    if (!this.#lostAt) this.#lostAt = Date.now();
    this.#opened = true; // a later open counts as "back", even if the first open never happened
  }

  #onerror() {
    this.#wasLost();
    if (this.#source?.readyState === CLOSED) {
      this.#offline();
      return;
    }
    this.connection = 'reconnecting';
    clearTimeout(this.#timer);
    this.#timer = setTimeout(() => this.#offline(), this.#offlineAfter);
  }

  #offline() {
    this.connection = 'offline';
    this.offlineSince = new Date(this.#lostAt || Date.now()).toISOString();
  }
}

/**
 * Wires a refetch to events and reconnects, coalescing bursts (download.progress arrives often) into
 * one call per `wait` ms. Use inside `$effect`; returns the cleanup.
 */
export function refetchOn(live: LiveEvents, patterns: string[], fn: () => void, wait = 400): () => void {
  let t: ReturnType<typeof setTimeout> | undefined;
  const later = () => {
    if (t) return;
    t = setTimeout(() => { t = undefined; fn(); }, wait);
  };
  const offs = patterns.map((p) => live.on(p, later));
  offs.push(live.onreconnect(fn));
  return () => {
    clearTimeout(t);
    for (const off of offs) off();
  };
}
