import { describe, it, expect, vi } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import Tabs from './Tabs.svelte';
import SegmentedControl from '../SegmentedControl/SegmentedControl.svelte';

const tabs = [{ id: 'a', label: 'Settings' }, { id: 'b', label: 'Stats', disabled: true }, { id: 'c', label: 'Queries', count: 1284 }];

describe('Tabs', () => {
  it('has one tab stop and moves with arrows, skipping disabled tabs, wrapping', async () => {
    const onchange = vi.fn();
    const { getAllByRole, getByRole } = render(Tabs, { tabs, label: 'Indexer', onchange });
    const [a, , c] = getAllByRole('tab');
    expect(getByRole('tablist', { name: 'Indexer' })).toBeTruthy();
    expect(a.tabIndex).toBe(0);
    expect(c.tabIndex).toBe(-1);
    expect(c.textContent).toContain('1,284');
    await fireEvent.keyDown(a, { key: 'ArrowRight' });
    expect(onchange).toHaveBeenLastCalledWith('c');
    expect(c.getAttribute('aria-selected')).toBe('true');
    expect(document.activeElement).toBe(c);
    await fireEvent.keyDown(c, { key: 'ArrowRight' });
    expect(onchange).toHaveBeenLastCalledWith('a');
    await fireEvent.keyDown(a, { key: 'End' });
    expect(onchange).toHaveBeenLastCalledWith('c');
  });
});

describe('SegmentedControl', () => {
  it('is a radio group with roving focus', async () => {
    const onchange = vi.fn();
    const { getAllByRole, getByRole } = render(SegmentedControl, {
      label: 'Range', value: '24h', onchange,
      options: [{ value: '24h', label: '24 h' }, { value: '7d', label: '7 d' }],
    });
    expect(getByRole('radiogroup', { name: 'Range' })).toBeTruthy();
    const [d, w] = getAllByRole('radio');
    expect(d.getAttribute('aria-checked')).toBe('true');
    await fireEvent.keyDown(d, { key: 'ArrowLeft' });
    expect(onchange).toHaveBeenLastCalledWith('7d');
    expect(w.getAttribute('aria-checked')).toBe('true');
    await fireEvent.click(d);
    expect(onchange).toHaveBeenLastCalledWith('24h');
  });

  it('keeps labels as accessible names when icon only', () => {
    const { getByRole } = render(SegmentedControl, { label: 'View', iconOnly: true, options: [{ value: 't', label: 'Table', icon: 'list' }] });
    expect(getByRole('radio', { name: 'Table' })).toBeTruthy();
  });
});
