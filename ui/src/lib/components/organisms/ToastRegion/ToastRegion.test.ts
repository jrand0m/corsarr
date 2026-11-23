import { describe, it, expect, vi, afterEach } from 'vitest';
import { render, fireEvent, within } from '@testing-library/svelte';
import { flushSync } from 'svelte';
import ToastRegion from './ToastRegion.svelte';
import { ToastQueue } from '../../../toast/toasts.svelte';

afterEach(() => vi.useRealTimers());

describe('ToastRegion', () => {
  it('has empty polite and assertive live regions before any toast', () => {
    const { getByRole } = render(ToastRegion, { queue: new ToastQueue() });
    expect(getByRole('status').textContent).toBe('');
    expect(getByRole('alert').textContent).toBe('');
  });

  it('shows pushed toasts and announces outcomes politely, failures assertively', () => {
    const queue = new ToastQueue();
    const { getByRole } = render(ToastRegion, { queue });
    queue.push({ message: 'Removed The Long Haul S02E03', detail: 'Searching for another release.', onundo: () => {}, duration: 0 });
    queue.push({ message: "Couldn't pause Dockside", tone: 'error', duration: 0 });
    flushSync();
    const list = getByRole('list', { name: 'Notifications' });
    expect(within(list).getAllByRole('listitem')).toHaveLength(2);
    expect(getByRole('status').textContent).toBe('Removed The Long Haul S02E03. Searching for another release. Undo available.');
    expect(getByRole('alert').textContent).toBe("Couldn't pause Dockside.");
    // Toasts inside the region do not carry their own live role (no double announcements).
    expect(within(list).queryByRole('status')).toBeNull();
  });

  it('caps visible toasts and shows the next when one is dismissed', async () => {
    const queue = new ToastQueue();
    const { getByRole, getByText, queryByText } = render(ToastRegion, { queue, max: 2 });
    for (const m of ['Paused Dockside', 'Paused Harbour Lights', 'Paused The Long Haul']) queue.push({ message: m, duration: 0 });
    flushSync();
    expect(getByText('1 more waiting')).toBeTruthy();
    expect(queryByText('Paused The Long Haul')).toBeNull();
    await fireEvent.click(within(getByRole('list')).getAllByRole('button', { name: 'Dismiss' })[0]);
    expect(within(getByRole('list')).queryByText('Paused Dockside')).toBeNull();
    expect(within(getByRole('list')).getByText('Paused The Long Haul')).toBeTruthy();
    expect(getByRole('status').textContent).toContain('Paused The Long Haul');
  });

  it('wires Undo to the entry and removes it', async () => {
    const queue = new ToastQueue();
    const onundo = vi.fn();
    const { getByRole, queryByText } = render(ToastRegion, { queue });
    queue.push({ message: 'Killed Dockside', onundo });
    flushSync();
    await fireEvent.click(getByRole('button', { name: 'Undo' }));
    expect(onundo).toHaveBeenCalledOnce();
    expect(queryByText('Killed Dockside')).toBeNull();
    expect(queue.items).toHaveLength(0);
  });

  it('dismisses itself after its time (at least 10 s with undo)', () => {
    vi.useFakeTimers();
    const queue = new ToastQueue();
    render(ToastRegion, { queue });
    queue.push({ message: 'Unblocklisted Dockside', onundo: () => {}, duration: 4000 });
    flushSync();
    vi.advanceTimersByTime(9_000);
    expect(queue.items).toHaveLength(1);
    vi.advanceTimersByTime(1_100);
    expect(queue.items).toHaveLength(0);
  });
});
