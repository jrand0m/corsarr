import { describe, it, expect, vi } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import ToggleMatrix, { type MatrixColumn, type MatrixRow } from './ToggleMatrix.svelte';

const rows: MatrixRow[] = [
  { id: 'grab', label: 'Release grabbed' },
  { id: 'import', label: 'Imported' },
];
const columns: MatrixColumn[] = [
  { id: 'telegram', label: 'Telegram' },
  { id: 'email', label: 'Email' },
  { id: 'priority', label: 'Priority', kind: 'select', options: [{ value: 'low', label: 'Low' }, { value: 'high', label: 'High' }] },
];

describe('ToggleMatrix', () => {
  it('toggles a cell and sets a select cell', async () => {
    const onchange = vi.fn();
    const { getByRole } = render(ToggleMatrix, { rows, columns, value: { grab: { priority: 'low' } }, label: 'Events', onchange });
    await fireEvent.click(getByRole('checkbox', { name: 'Telegram for Imported' }));
    expect(onchange.mock.lastCall![0].import.telegram).toBe(true);
    const sel = getByRole('combobox', { name: 'Priority for Release grabbed' }) as HTMLSelectElement;
    expect(sel.value).toBe('low');
    await fireEvent.change(sel, { target: { value: 'high' } });
    expect(onchange.mock.lastCall![0].grab.priority).toBe('high');
  });

  it('sets a whole column from its header and skips unavailable cells', async () => {
    const onchange = vi.fn();
    const unavailable = (r: MatrixRow, c: MatrixColumn) => (r.id === 'grab' && c.id === 'email' ? 'Too frequent for email' : undefined);
    const { getByRole } = render(ToggleMatrix, { rows, columns, label: 'Events', unavailable, onchange });
    await fireEvent.click(getByRole('checkbox', { name: 'All rows: Email' }));
    const v = onchange.mock.lastCall![0];
    expect(v.import.email).toBe(true);
    expect(v.grab?.email).toBeUndefined();
    expect(getByRole('button', { name: /Email for Release grabbed: unavailable, Too frequent/ })).toBeTruthy();
  });

  it('has one tab stop and moves focus with the arrow keys', async () => {
    const { getByRole } = render(ToggleMatrix, { rows, columns, label: 'Events' });
    const first = getByRole('checkbox', { name: 'Telegram for Release grabbed' });
    expect(first.getAttribute('tabindex')).toBe('0');
    expect(getByRole('checkbox', { name: 'Email for Release grabbed' }).getAttribute('tabindex')).toBe('-1');
    first.focus();
    await fireEvent.keyDown(first, { key: 'ArrowRight' });
    expect(document.activeElement).toBe(getByRole('checkbox', { name: 'Email for Release grabbed' }));
    await fireEvent.keyDown(document.activeElement!, { key: 'ArrowDown' });
    expect(document.activeElement).toBe(getByRole('checkbox', { name: 'Email for Imported' }));
    expect(getByRole('checkbox', { name: 'Email for Imported' }).getAttribute('tabindex')).toBe('0');
  });

  it('disables every control when read-only', () => {
    const { getAllByRole, queryByRole } = render(ToggleMatrix, { rows, columns, label: 'Events', readonly: true });
    expect(getAllByRole('checkbox').every((c) => (c as HTMLInputElement).disabled)).toBe(true);
    expect(queryByRole('checkbox', { name: /All rows/ })).toBeNull();
  });
});
