import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { LiveEvents, matches, refetchOn, type SourceLike } from './live.svelte';

class FakeSource implements SourceLike {
  readyState = 0;
  onopen: ((e: Event) => void) | null = null;
  onerror: ((e: Event) => void) | null = null;
  listeners = new Map<string, (e: MessageEvent) => void>();
  closed = false;
  constructor(public url: string) {}
  addEventListener(type: string, fn: (e: MessageEvent) => void) { this.listeners.set(type, fn); }
  close() { this.closed = true; this.readyState = 2; }
  open() { this.readyState = 1; this.onopen?.(new Event('open')); }
  fail(closed = false) { this.readyState = closed ? 2 : 0; this.onerror?.(new Event('error')); }
  send(type: string, data: object) { this.listeners.get(type)?.(new MessageEvent(type, { data: JSON.stringify({ type, ...data }) })); }
}

let sources: FakeSource[] = [];
const factory = (u: string) => { const s = new FakeSource(u); sources.push(s); return s; };

beforeEach(() => { sources = []; vi.useFakeTimers(); });
afterEach(() => vi.useRealTimers());

describe('matches', () => {
  it('matches exact types, families and everything', () => {
    expect(matches('download.progress', 'download.progress')).toBe(true);
    expect(matches('download.*', 'download.state_changed')).toBe(true);
    expect(matches('download.*', 'grab.sent')).toBe(false);
    expect(matches('*', 'health.changed')).toBe(true);
    expect(matches('item.added', 'item.added.x')).toBe(false);
  });
});

describe('LiveEvents', () => {
  it('opens one EventSource and listens to every named event', () => {
    const live = new LiveEvents('api/v1/events', factory);
    live.start();
    live.start();
    expect(sources).toHaveLength(1);
    expect(sources[0].url).toBe('api/v1/events');
    expect(sources[0].listeners.has('download.progress')).toBe(true);
    expect(sources[0].listeners.has('health.changed')).toBe(true);
  });

  it('fans events out by type and family, and unsubscribes', () => {
    const live = new LiveEvents('e', factory);
    live.start();
    const exact = vi.fn();
    const family = vi.fn();
    const off = live.on('download.progress', exact);
    live.on('download.*', family);
    sources[0].send('download.progress', { data: { downloadId: 3, progress: 0.5 } });
    sources[0].send('download.removed', {});
    sources[0].send('health.changed', {});
    expect(exact).toHaveBeenCalledTimes(1);
    expect(exact.mock.calls[0][0].data).toEqual({ downloadId: 3, progress: 0.5 });
    expect(family).toHaveBeenCalledTimes(2);
    off();
    sources[0].send('download.progress', {});
    expect(exact).toHaveBeenCalledTimes(1);
  });

  it('ignores malformed event data', () => {
    const live = new LiveEvents('e', factory);
    live.start();
    const fn = vi.fn();
    live.on('*', fn);
    sources[0].listeners.get('item.added')?.(new MessageEvent('item.added', { data: '{oops' }));
    expect(fn).not.toHaveBeenCalled();
  });

  it('tracks the connection and fires reconnect handlers only after a drop', () => {
    const live = new LiveEvents('e', factory, 10_000);
    const back = vi.fn();
    live.onreconnect(back);
    live.start();
    sources[0].open();
    expect(live.connection).toBe('live');
    expect(back).not.toHaveBeenCalled();
    sources[0].fail();
    expect(live.connection).toBe('reconnecting');
    sources[0].open();
    expect(live.connection).toBe('live');
    expect(back).toHaveBeenCalledTimes(1);
  });

  it('goes offline when reconnecting takes too long or the browser gives up', () => {
    const live = new LiveEvents('e', factory, 10_000);
    live.start();
    sources[0].open();
    sources[0].fail();
    vi.advanceTimersByTime(10_001);
    expect(live.connection).toBe('offline');
    expect(live.offlineSince).toBeTruthy();

    const live2 = new LiveEvents('e', factory);
    live2.start();
    sources[1].open();
    sources[1].fail(true);
    expect(live2.connection).toBe('offline');
  });

  it('reconnect() opens a fresh source and refetch handlers run when it opens', () => {
    const live = new LiveEvents('e', factory);
    const back = vi.fn();
    live.onreconnect(back);
    live.start();
    sources[0].open();
    sources[0].fail(true);
    live.reconnect();
    expect(sources[0].closed).toBe(true);
    expect(sources).toHaveLength(2);
    sources[1].open();
    expect(back).toHaveBeenCalledTimes(1);
    expect(live.connection).toBe('live');
  });
});

describe('refetchOn', () => {
  it('coalesces bursts into one call and refetches on reconnect', () => {
    const live = new LiveEvents('e', factory);
    live.start();
    sources[0].open();
    const fn = vi.fn();
    const off = refetchOn(live, ['download.*'], fn, 300);
    for (let i = 0; i < 5; i++) sources[0].send('download.progress', {});
    expect(fn).not.toHaveBeenCalled();
    vi.advanceTimersByTime(301);
    expect(fn).toHaveBeenCalledTimes(1);
    sources[0].fail();
    sources[0].open();
    expect(fn).toHaveBeenCalledTimes(2);
    off();
    sources[0].send('download.progress', {});
    vi.advanceTimersByTime(301);
    expect(fn).toHaveBeenCalledTimes(2);
  });
});
