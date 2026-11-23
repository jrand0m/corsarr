import { describe, it, expect, vi } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import QueueCard from './QueueCard.svelte';

const now = Date.parse('2026-11-22T14:05:00Z');
const d = {
  id: 'd1', title: 'Dockside', kind: 'movie' as const, subtitle: '2026', state: 'stalled' as const, reason: 'no peers',
  progress: 0.81, size: 18.2e9, downloaded: 14.7e9, speed: 0,
  rule: { badge: 'stalled' as const, at: now + 245_000, action: 'Removes' },
};

describe('QueueCard', () => {
  it('shows title, state with reason, progress facts and the rule countdown', () => {
    const { getByRole, container } = render(QueueCard, { download: d, now });
    expect(getByRole('article', { name: 'Dockside' })).toBeTruthy();
    expect(container.textContent).toContain('Stalled · no peers');
    expect(getByRole('progressbar', { name: 'Progress of Dockside' }).getAttribute('aria-valuenow')).toBe('81');
    expect(container.querySelector('.facts')!.textContent).toContain('14.7 GB of 18.2 GB');
    expect(container.querySelector('time')!.textContent).toBe('in 4:05');
  });

  it('offers Pause or Resume up front and the rest in the menu', async () => {
    const onaction = vi.fn();
    const { getByRole, getByText, rerender } = render(QueueCard, { download: d, now, onaction });
    await fireEvent.click(getByRole('button', { name: 'Pause' }));
    expect(onaction).toHaveBeenLastCalledWith('pause', 'd1');
    expect(getByRole('button', { name: 'More actions for Dockside' })).toBeTruthy();
    for (const label of ['Move to top', 'Move up', 'Move down', 'Move to bottom', 'Replace with another release…', 'Replace with lower resolution…', 'Kill download…']) {
      expect(getByText(label)).toBeTruthy();
    }
    await fireEvent.click(getByText('Replace with lower resolution…'));
    expect(onaction).toHaveBeenLastCalledWith('replace-lower', 'd1');
    await rerender({ download: { ...d, paused: true }, now, onaction });
    await fireEvent.click(getByRole('button', { name: 'Resume' }));
    expect(onaction).toHaveBeenLastCalledWith('resume', 'd1');
  });

  it('has no actions without a handler or once finished, and honours the allowed set', () => {
    const ro = render(QueueCard, { download: d, now });
    expect(ro.queryByRole('button')).toBeNull();
    ro.unmount();
    const fin = render(QueueCard, { download: { ...d, state: 'imported' as const }, now, onaction: () => {} });
    expect(fin.queryByRole('button')).toBeNull();
    expect(fin.queryByRole('progressbar')).toBeNull();
    fin.unmount();
    const lim = render(QueueCard, { download: d, now, onaction: () => {}, actions: ['kill'] });
    expect(lim.queryByRole('button', { name: 'Pause' })).toBeNull();
    expect(lim.queryByText('Move up')).toBeNull();
    expect(lim.getByText('Kill download…')).toBeTruthy();
  });

  it('shows dry-run rules as "would"', () => {
    const { container } = render(QueueCard, { download: { ...d, rule: { ...d.rule, dryRun: true } }, now });
    expect(container.querySelector('.rule')!.textContent).toContain('Dry run:');
    expect(container.querySelector('.rule .visually-hidden')!.textContent).toBe('Removes in 5m');
  });
});
