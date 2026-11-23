import { describe, it, expect, vi } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import BayGrid, { summarise, type BayCell } from './BayGrid.svelte';

const cells: BayCell[] = [
  { state: 'have', label: 'E01' }, { state: 'have', label: 'E02' }, { state: 'missing', label: 'E03' }, { state: 'unaired', label: 'E04' },
];

describe('BayGrid', () => {
  it('summarises counts per state in a fixed order', () => {
    expect(summarise(cells)).toBe('2 on disk, 1 missing, 1 not available yet');
  });
  it('is one labelled image when static', () => {
    const { getByRole } = render(BayGrid, { cells, label: 'Season 2' });
    expect(getByRole('img').getAttribute('aria-label')).toBe('Season 2: 2 on disk, 1 missing, 1 not available yet');
  });
  it('uses roving focus and reports selection when interactive', async () => {
    const onselect = vi.fn();
    const { getAllByRole } = render(BayGrid, { cells, label: 'Season 2', onselect });
    const buttons = getAllByRole('button');
    expect(buttons.map((b) => b.tabIndex)).toEqual([0, -1, -1, -1]);
    await fireEvent.keyDown(buttons[0], { key: 'End' });
    expect(buttons[3].tabIndex).toBe(0);
    await fireEvent.click(buttons[2]);
    expect(onselect).toHaveBeenCalledWith(2, cells[2]);
    expect(buttons[2].getAttribute('aria-label')).toBe('E03, Missing');
  });
});
