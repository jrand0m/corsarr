import { describe, it, expect, vi } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import { createRawSnippet } from 'svelte';
import DataTable, { type Column } from './DataTable.svelte';
import { libraryRows, type LibraryRow } from '../../../storybook/library-rows';

const columns: Column<LibraryRow>[] = [
  { id: 'title', label: 'Title', sortable: true, value: (r) => r.title, phone: 'title' },
  { id: 'profile', label: 'Profile', value: (r) => r.profile },
  { id: 'size', label: 'Size', align: 'end', sortable: true, value: (r) => r.size },
];
const base = { columns, rowKey: (r: LibraryRow) => r.id, label: 'Library', mode: 'table' as const };
const dataRows = (c: HTMLElement) => c.querySelectorAll('[data-index]');

describe('DataTable', () => {
  it('keeps the DOM bounded for 10,000 rows', () => {
    const { container, getByRole } = render(DataTable<LibraryRow>, { ...base, rows: libraryRows(10_000) });
    expect(dataRows(container).length).toBeGreaterThan(5);
    expect(dataRows(container).length).toBeLessThan(40);
    expect(getByRole('table', { name: 'Library' }).getAttribute('aria-rowcount')).toBe('10001');
  });

  it('sorts from the header and reflects it in aria-sort', async () => {
    const onsort = vi.fn();
    const { getByRole, getAllByRole } = render(DataTable<LibraryRow>, { ...base, rows: libraryRows(5), onsort });
    await fireEvent.click(getByRole('button', { name: 'Size' }));
    expect(onsort).toHaveBeenLastCalledWith({ column: 'size', direction: 'asc' });
    expect(getAllByRole('columnheader')[2].getAttribute('aria-sort')).toBe('ascending');
    await fireEvent.click(getByRole('button', { name: 'Size' }));
    expect(onsort).toHaveBeenLastCalledWith({ column: 'size', direction: 'desc' });
  });

  it('moves, opens and selects rows from the keyboard', async () => {
    const onopen = vi.fn();
    const rows = libraryRows(20);
    const { container, getByRole } = render(DataTable<LibraryRow>, { ...base, rows, onopen, selectable: true });
    const first = container.querySelector<HTMLElement>('[data-index="0"]')!;
    expect(first.tabIndex).toBe(0);
    first.focus();
    await fireEvent.keyDown(first, { key: 'j' });
    await new Promise((r) => setTimeout(r));
    const second = container.querySelector<HTMLElement>('[data-index="1"]')!;
    expect(document.activeElement).toBe(second);
    await fireEvent.keyDown(second, { key: 'Enter' });
    expect(onopen).toHaveBeenCalledWith(rows[1]);
    await fireEvent.keyDown(second, { key: 'x' });
    expect(second.getAttribute('aria-selected')).toBe('true');
    await fireEvent.keyDown(second, { key: 'ArrowDown' });
    await new Promise((r) => setTimeout(r));
    await fireEvent.keyDown(document.activeElement!, { key: 'ArrowDown' });
    await new Promise((r) => setTimeout(r));
    await fireEvent.keyDown(document.activeElement!, { key: 'x', shiftKey: true });
    expect(getByRole('status').textContent).toBe('3 rows selected');
    await fireEvent.keyDown(document.activeElement!, { key: 'a', ctrlKey: true });
    expect(getByRole('status').textContent).toBe('20 rows selected');
  });

  it('opens the row actions with "."', async () => {
    const onaction = vi.fn();
    const { container } = render(DataTable<LibraryRow>, {
      ...base, rows: libraryRows(3), rowActions: () => [{ id: 'search', label: 'Search now' }], onaction,
    });
    const row = container.querySelector<HTMLElement>('[data-index="0"]')!;
    const trigger = row.querySelector<HTMLButtonElement>('[data-actions] button')!;
    const click = vi.fn();
    trigger.addEventListener('click', click);
    await fireEvent.keyDown(row, { key: '.' });
    expect(click).toHaveBeenCalled();
  });

  it('selects all loaded from the header and shows the bulk bar', async () => {
    const onselectionchange = vi.fn();
    const { getByRole, getByText } = render(DataTable<LibraryRow>, { ...base, rows: libraryRows(4), selectable: true, total: 400, onselectionchange });
    await fireEvent.click(getByRole('checkbox', { name: 'Select all 4 loaded' }));
    expect(onselectionchange).toHaveBeenLastCalledWith(['item-0', 'item-1', 'item-2', 'item-3']);
    expect(getByText('4 of 400 loaded')).toBeTruthy();
  });

  it('asks for the next page once when the end is in view', () => {
    const onloadmore = vi.fn();
    const { rerender } = render(DataTable<LibraryRow>, { ...base, rows: libraryRows(10), hasMore: true, onloadmore });
    expect(onloadmore).toHaveBeenCalledTimes(1);
    rerender({ ...base, rows: libraryRows(10), hasMore: true, onloadmore });
    expect(onloadmore).toHaveBeenCalledTimes(1);
  });

  it('renders a phone list with title, meta and selection', () => {
    const { getByRole, getAllByRole } = render(DataTable<LibraryRow>, { ...base, mode: 'list', rows: libraryRows(3), selectable: true });
    const items = getAllByRole('listitem');
    expect(getByRole('list', { name: 'Library' })).toBeTruthy();
    expect(items[0].textContent).toContain(libraryRows(1)[0].title);
    expect(getByRole('checkbox', { name: `Select ${libraryRows(1)[0].title}` })).toBeTruthy();
  });

  it('shows empty and error states', () => {
    const { getByText, unmount } = render(DataTable<LibraryRow>, { ...base, rows: [], noun: 'series', nounPlural: 'series' });
    expect(getByText('No series to show')).toBeTruthy();
    unmount();
    const r = render(DataTable<LibraryRow>, { ...base, rows: [], noun: 'series', nounPlural: 'series', error: 'timeout', onretry: () => {} });
    expect(r.getByText("Couldn't load the series")).toBeTruthy();
  });

  it('expands a row to show its detail, by button and by arrow keys', async () => {
    const rows = libraryRows(5);
    const onexpandchange = vi.fn();
    const expand = createRawSnippet((row: () => LibraryRow) => ({ render: () => `<p>Detail of ${row().title}</p>` }));
    for (const mode of ['table', 'list'] as const) {
      const { getByRole, queryByText, container, unmount } = render(DataTable<LibraryRow>, { ...base, mode, rows, expand, onexpandchange });
      const btn = getByRole('button', { name: `Show details for ${rows[1].title}` });
      expect(btn.getAttribute('aria-expanded')).toBe('false');
      await fireEvent.click(btn);
      expect(onexpandchange).toHaveBeenLastCalledWith([rows[1].id]);
      expect(queryByText(`Detail of ${rows[1].title}`)).toBeTruthy();
      expect(getByRole('button', { name: `Hide details for ${rows[1].title}` }).getAttribute('aria-expanded')).toBe('true');
      const row = container.querySelector<HTMLElement>('[data-index="1"]')!;
      await fireEvent.keyDown(row, { key: 'ArrowLeft' });
      expect(queryByText(`Detail of ${rows[1].title}`)).toBeNull();
      await fireEvent.keyDown(row, { key: 'ArrowRight' });
      expect(queryByText(`Detail of ${rows[1].title}`)).toBeTruthy();
      unmount();
    }
  });
});
