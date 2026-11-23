import { describe, expect, it, vi } from 'vitest';
import { flushSync } from 'svelte';
import { Resource } from './resource.svelte';
import { ApiError } from './api/client';

const settle = () => new Promise((r) => setTimeout(r, 0));

describe('Resource', () => {
  it('loads, exposes data and clears loading', async () => {
    const r = new Resource(async () => 42);
    expect(r.loading).toBe(true);
    await r.load();
    expect(r.data).toBe(42);
    expect(r.loading).toBe(false);
    expect(r.error).toBeUndefined();
  });

  it('keeps the last data and reports the error as ApiError', async () => {
    let fail = false;
    const r = new Resource(async () => { if (fail) throw new ApiError(502, { code: 'upstream', message: 'down' }); return 'ok'; });
    await r.load();
    fail = true;
    await r.load(true);
    expect(r.data).toBe('ok');
    expect(r.error?.code).toBe('upstream');
  });

  it('lets the newest load win', async () => {
    const resolvers: ((v: number) => void)[] = [];
    const r = new Resource(() => new Promise<number>((res) => resolvers.push(res)));
    const first = r.load();
    const second = r.load();
    resolvers[1](2);
    resolvers[0](1);
    await Promise.all([first, second]);
    expect(r.data).toBe(2);
  });

  it('does not make the calling effect depend on its own state (no refetch loop)', async () => {
    const fn = vi.fn(async () => 1);
    const r = new Resource(fn);
    let runs = 0;
    const stop = $effect.root(() => {
      $effect(() => {
        runs++;
        void r.load(true);
      });
    });
    flushSync();
    await settle();
    flushSync();
    await settle();
    expect(r.data).toBe(1);
    expect(runs).toBe(1);
    expect(fn).toHaveBeenCalledTimes(1);
    stop();
  });
});
