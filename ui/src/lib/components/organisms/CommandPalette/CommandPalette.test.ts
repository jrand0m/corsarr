import { describe, it, expect, vi } from 'vitest';
import { render, fireEvent, within } from '@testing-library/svelte';
import CommandPalette, { type PaletteItem } from './CommandPalette.svelte';

const items: PaletteItem[] = [
  { id: 'go-queue', label: 'Queue', group: 'Screens', icon: 'activity', keys: ['g', 'q'], keywords: ['downloads'] },
  { id: 'go-history', label: 'History', group: 'Screens', icon: 'clock' },
  { id: 'pause-all', label: 'Pause all downloads', group: 'Actions', icon: 'pause' },
  { id: 'kill', label: 'Kill selected download…', group: 'Actions', icon: 'trash', danger: true },
  { id: 'tlh', label: 'The Long Haul', group: 'Items', kind: 'tv', subtitle: 'TV · 2024' },
  { id: 'dock', label: 'Dockside', group: 'Items', kind: 'movie', subtitle: 'Movie · 2026' },
];
const labels = (el: HTMLElement) => within(el).getAllByRole('option').map((o) => o.querySelector('.label')!.textContent);

describe('CommandPalette', () => {
  it('opens with focus in a combobox and shows recent picks first', () => {
    const { getByRole } = render(CommandPalette, { open: true, items, recent: ['dock'] });
    const box = getByRole('combobox');
    expect(document.activeElement).toBe(box);
    const groups = within(getByRole('listbox')).getAllByRole('group');
    expect(groups[0].textContent).toContain('Recent');
    expect(labels(groups[0])).toEqual(['Dockside']);
    expect(getByRole('dialog', { name: 'Command palette' })).toBeTruthy();
  });

  it('fuzzy-filters across labels and keywords, highlighting matches', async () => {
    const onquery = vi.fn();
    const { getByRole } = render(CommandPalette, { open: true, items, onquery });
    const box = getByRole('combobox');
    await fireEvent.input(box, { target: { value: 'lh' } });
    expect(onquery).toHaveBeenLastCalledWith('lh');
    expect(labels(getByRole('listbox'))).toEqual(['The Long Haul']);
    expect([...getByRole('listbox').querySelectorAll('mark')].map((m) => m.textContent)).toEqual(['L', 'H']);
    await fireEvent.input(box, { target: { value: 'downloads' } });
    expect(labels(getByRole('listbox'))).toContain('Queue');
  });

  it('moves with arrows (wrapping), tracks aria-activedescendant and opens with Enter', async () => {
    const onselect = vi.fn();
    const { getByRole } = render(CommandPalette, { open: true, items, onselect });
    const box = getByRole('combobox');
    const options = within(getByRole('listbox')).getAllByRole('option');
    expect(box.getAttribute('aria-activedescendant')).toBe(options[0].id);
    expect(options[0].getAttribute('aria-selected')).toBe('true');
    await fireEvent.keyDown(box, { key: 'ArrowDown' });
    expect(box.getAttribute('aria-activedescendant')).toBe(options[1].id);
    await fireEvent.keyDown(box, { key: 'ArrowUp' });
    await fireEvent.keyDown(box, { key: 'ArrowUp' });
    expect(box.getAttribute('aria-activedescendant')).toBe(options[options.length - 1].id);
    await fireEvent.keyDown(box, { key: 'Enter' });
    expect(onselect).toHaveBeenCalledWith(items[items.length - 1]);
    expect(getByRole('dialog', { hidden: true }).hasAttribute('open')).toBe(false);
  });

  it('selects on click', async () => {
    const onselect = vi.fn();
    const { getByRole } = render(CommandPalette, { open: true, items, onselect });
    await fireEvent.click(getByRole('option', { name: /Pause all downloads/ }));
    expect(onselect).toHaveBeenCalledWith(items[2]);
  });

  it('says when nothing matches, and shows loading for item search', async () => {
    const { getByRole, getByText, rerender } = render(CommandPalette, { open: true, items });
    await fireEvent.input(getByRole('combobox'), { target: { value: 'zzqx' } });
    expect(getByText(/Nothing matches “zzqx”/)).toBeTruthy();
    await rerender({ loading: true });
    expect(getByRole('img', { name: 'Searching items' })).toBeTruthy();
  });
});
