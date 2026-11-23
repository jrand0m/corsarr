import { describe, it, expect, vi } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import BulkActionBar from './BulkActionBar.svelte';

describe('BulkActionBar', () => {
  it('renders nothing without a selection', () => {
    const { container } = render(BulkActionBar, { selected: 0, loaded: 10 });
    expect(container.querySelector('[role="toolbar"]')).toBeNull();
  });

  it('counts the selection, offers select all loaded and clear', async () => {
    const onselectall = vi.fn();
    const onclear = vi.fn();
    const { getByRole, rerender, queryByRole, getByText } = render(BulkActionBar, {
      selected: 3, loaded: 100, total: 412, noun: 'series', nounPlural: 'series', onselectall, onclear,
    });
    expect(getByRole('toolbar', { name: 'Actions for selected series' })).toBeTruthy();
    expect(getByRole('status').textContent).toBe('3 series selected');
    await fireEvent.click(getByRole('button', { name: 'Select all 100 loaded' }));
    expect(onselectall).toHaveBeenCalledOnce();
    await fireEvent.click(getByRole('button', { name: 'Clear' }));
    expect(onclear).toHaveBeenCalledOnce();
    await rerender({ selected: 100, loaded: 100, total: 412, noun: 'series', nounPlural: 'series', onselectall, onclear });
    expect(queryByRole('button', { name: /Select all/ })).toBeNull();
    expect(getByText('100 of 412 loaded')).toBeTruthy();
  });
});
