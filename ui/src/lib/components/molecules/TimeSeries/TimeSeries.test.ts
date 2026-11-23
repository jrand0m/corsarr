import { describe, it, expect, vi } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import TimeSeries from './TimeSeries.svelte';

const end = Date.parse('2026-11-22T14:00:00Z');
const buckets = [
  { at: end - 2 * 3600_000, value: 10 },
  { at: end - 3600_000, value: 4, failed: 3 },
  { at: end, value: 12 },
];

describe('TimeSeries', () => {
  it('summarises totals, names failures in a legend and keeps a table of every value', async () => {
    const { container, getByRole } = render(TimeSeries, { title: 'Queries per hour', buckets });
    expect(container.querySelector('.total')!.textContent).toBe('26 queries · 3 failed');
    expect(container.querySelector('.legend')!.textContent).toContain('Failed');
    const table = container.querySelector('table')!;
    expect(table.closest('[hidden]')).toBeTruthy();
    await fireEvent.click(getByRole('button', { name: 'Show as table' }));
    expect(table.closest('[hidden]')).toBeNull();
    expect(table.querySelectorAll('tbody tr')).toHaveLength(3);
  });

  it('switches range through the segmented control', async () => {
    const onrangechange = vi.fn();
    const { getByRole } = render(TimeSeries, { title: 'Queries', buckets, onrangechange });
    await fireEvent.click(getByRole('radio', { name: '7 d' }));
    expect(onrangechange).toHaveBeenCalledWith('7d');
  });

  it('shows the empty and loading states', () => {
    const e = render(TimeSeries, { title: 'Queries', buckets: [], range: '7d' });
    expect(e.getByText('No data for the last 7 d yet.')).toBeTruthy();
    e.unmount();
    const l = render(TimeSeries, { title: 'Queries', buckets: [], loading: true });
    expect(l.container.querySelector('.skeleton, [class*="sk"]')).toBeTruthy();
  });
});
