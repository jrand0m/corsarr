import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { Client } from './api/client';
import type { Download } from './api/types';
import { pausedIds, queueAction } from './queue.svelte';

const fakeApi = () => {
  const post = vi.fn(async (path: string) => ({ id: 7, title: path }) as unknown as Download);
  return { api: { post } as unknown as Client, post };
};

describe('queueAction', () => {
  beforeEach(() => pausedIds.clear());

  it('pauses and resumes, remembering the pause in this browser', async () => {
    const { api, post } = fakeApi();
    expect(await queueAction(api, 7, 'pause')).toMatchObject({ id: 7 });
    expect(post).toHaveBeenLastCalledWith('queue/7/pause');
    expect(pausedIds.has('7')).toBe(true);
    await queueAction(api, '7', 'resume');
    expect(post).toHaveBeenLastCalledWith('queue/7/resume');
    expect(pausedIds.has('7')).toBe(false);
  });

  it('moves priority with the move as the body', async () => {
    const { api, post } = fakeApi();
    for (const move of ['top', 'up', 'down', 'bottom']) {
      await queueAction(api, 3, move);
      expect(post).toHaveBeenLastCalledWith('queue/3/priority', { priority: move });
    }
    expect(pausedIds.size).toBe(0);
  });

  it('holds and releases a download', async () => {
    const { api, post } = fakeApi();
    await queueAction(api, 4, 'hold');
    expect(post).toHaveBeenLastCalledWith('queue/4/hold');
    await queueAction(api, 4, 'release');
    expect(post).toHaveBeenLastCalledWith('queue/4/release');
  });

  it('offers nothing for an unknown action', () => {
    const { api, post } = fakeApi();
    expect(queueAction(api, 4, 'explode')).toBeUndefined();
    expect(post).not.toHaveBeenCalled();
  });
});
