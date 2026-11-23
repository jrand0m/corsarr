// Queue actions the API offers: pause, resume, priority and remove with the blocklist choice.
import type { Client } from './api/client';
import type { Download } from './api/types';
import { SvelteSet } from 'svelte/reactivity';

/**
 * Downloads paused from this browser. The Download record has no paused flag yet (missing API field),
 * so the UI remembers its own pause/resume until the page reloads.
 */
export const pausedIds = new SvelteSet<string>();

export type PriorityMove = 'top' | 'up' | 'down' | 'bottom';
export const priorityMoves: PriorityMove[] = ['top', 'up', 'down', 'bottom'];

export interface RemoveChoice { blocklist: boolean; research: boolean; deleteFiles: boolean }

export function queueAction(api: Client, id: string | number, action: string): Promise<Download> | undefined {
  if (action === 'pause' || action === 'resume') {
    return api.post<Download>(`queue/${id}/${action}`).then((d) => {
      if (action === 'pause') pausedIds.add(String(id)); else pausedIds.delete(String(id));
      return d;
    });
  }
  if ((priorityMoves as string[]).includes(action)) return api.post<Download>(`queue/${id}/priority`, { priority: action });
  if (action === 'hold' || action === 'release') return api.post<Download>(`queue/${id}/${action}`);
  return undefined;
}

export const removeDownload = (api: Client, id: string | number, c: RemoveChoice) =>
  api.post<Download>(`queue/${id}/remove`, c);

/** Past-tense toast line for an action. */
export const actionDone: Record<string, string> = {
  pause: 'Paused', resume: 'Resumed', top: 'Moved to the top', up: 'Moved up', down: 'Moved down', bottom: 'Moved to the bottom',
  hold: 'On hold', release: 'Hold released',
};
