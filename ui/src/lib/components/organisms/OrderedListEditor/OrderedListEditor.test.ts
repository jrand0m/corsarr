import { describe, it, expect, vi } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import OrderedListEditor, { moveItem, groupWithNext, ungroup, type OrderedItem } from './OrderedListEditor.svelte';

const items: OrderedItem[] = [
  { id: 'a', label: 'Bluray-1080p', enabled: true },
  { id: 'b', label: 'WEB-1080p', enabled: true },
  { id: 'c', label: 'HDTV-720p', enabled: true },
  { id: 's', label: 'SDTV', enabled: false, locked: 'Below the floor' },
];
const ids = (list: OrderedItem[]) => list.map((i) => i.id).join(',');

describe('OrderedListEditor helpers', () => {
  it('moves within a level and groups and ungroups rows', () => {
    expect(ids(moveItem(items, undefined, 0, 2))).toBe('b,c,a,s');
    const g = groupWithNext(items, 0);
    expect(g[0].children?.map((c) => c.id)).toEqual(['a', 'b']);
    expect(g[0].label).toBe('Bluray-1080p + WEB-1080p');
    expect(ids(moveItem(g, g[0].id, 0, 1)[0].children!)).toBe('b,a');
    expect(ids(ungroup(g, 0))).toBe('a,b,c,s');
    expect(items[0].children).toBeUndefined();
  });
});

describe('OrderedListEditor', () => {
  it('lifts, moves and drops a row with the keyboard and announces it', async () => {
    const onchange = vi.fn();
    const { getByRole } = render(OrderedListEditor, { items, label: 'Quality order', onchange });
    const handle = getByRole('button', { name: /Reorder Bluray-1080p/ });
    await fireEvent.keyDown(handle, { key: ' ' });
    expect(handle.getAttribute('aria-pressed')).toBe('true');
    expect(getByRole('status').textContent).toMatch('lifted');
    await fireEvent.keyDown(handle, { key: 'ArrowDown' });
    expect(ids(onchange.mock.lastCall![0])).toBe('b,a,c,s');
    expect(getByRole('status').textContent).toBe('Bluray-1080p, position 2 of 4');
    const moved = getByRole('button', { name: /Reorder Bluray-1080p/ });
    await fireEvent.keyDown(moved, { key: ' ' });
    expect(getByRole('status').textContent).toMatch('dropped at position 2');
  });

  it('puts the row back on Escape', async () => {
    const onchange = vi.fn();
    const { getByRole } = render(OrderedListEditor, { items, label: 'Quality order', onchange });
    const handle = getByRole('button', { name: /Reorder WEB-1080p/ });
    await fireEvent.keyDown(handle, { key: 'Enter' });
    await fireEvent.keyDown(handle, { key: 'End' });
    const h2 = getByRole('button', { name: /Reorder WEB-1080p/ });
    await fireEvent.keyDown(h2, { key: 'Escape' });
    expect(ids(onchange.mock.lastCall![0])).toBe('a,b,c,s');
  });

  it('moves at once with Alt+arrow and moves focus with plain arrows', async () => {
    const onchange = vi.fn();
    const { getByRole } = render(OrderedListEditor, { items, label: 'Quality order', onchange });
    const c = getByRole('button', { name: /Reorder HDTV-720p/ });
    await fireEvent.keyDown(c, { key: 'ArrowUp', altKey: true });
    expect(ids(onchange.mock.lastCall![0])).toBe('a,c,b,s');
    const a = getByRole('button', { name: /Reorder Bluray-1080p/ });
    a.focus();
    await fireEvent.keyDown(a, { key: 'ArrowDown' });
    await new Promise((r) => setTimeout(r));
    expect(document.activeElement?.getAttribute('aria-label')).toMatch('Reorder HDTV-720p');
  });

  it('toggles rows but not locked ones', async () => {
    const onchange = vi.fn();
    const { getByRole } = render(OrderedListEditor, { items, label: 'Quality order', toggleable: true, onchange });
    expect((getByRole('checkbox', { name: 'Allow SDTV' }) as HTMLInputElement).disabled).toBe(true);
    await fireEvent.click(getByRole('checkbox', { name: 'Allow WEB-1080p' }));
    expect(onchange.mock.lastCall![0][1].enabled).toBe(false);
  });

  it('renders groups as nested equal-rank lists', () => {
    const { getByRole } = render(OrderedListEditor, { items: groupWithNext(items, 0), label: 'Quality order', groupable: true });
    expect(getByRole('list', { name: 'Bluray-1080p + WEB-1080p, equal rank' }).children.length).toBe(2);
  });
});
