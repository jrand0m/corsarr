import { describe, it, expect, vi } from 'vitest';
import { render, fireEvent, waitFor } from '@testing-library/svelte';
import AsyncPicker, { type PickerOption } from './AsyncPicker.svelte';

const opts: PickerOption[] = [
  { value: 'hd', label: 'HD 1080p' }, { value: 'legacy', label: 'Legacy', disabled: true }, { value: 'uhd', label: 'Ultra HD' },
];
const search = vi.fn(async (q: string) => opts.filter((o) => o.label.toLowerCase().includes(q.toLowerCase())));

describe('AsyncPicker', () => {
  it('opens on focus, lists results, moves past disabled options and picks with Enter', async () => {
    const onchange = vi.fn();
    const { getByRole, findAllByRole } = render(AsyncPicker, { search, label: 'Profile', noun: 'profiles', debounce: 0, onchange });
    const box = getByRole('combobox', { name: 'Profile' });
    await fireEvent.focus(box);
    expect(box.getAttribute('aria-expanded')).toBe('true');
    const options = await findAllByRole('option');
    expect(options).toHaveLength(3);
    expect(box.getAttribute('aria-activedescendant')).toBe(options[0].id);
    await fireEvent.keyDown(box, { key: 'ArrowDown' });
    expect(box.getAttribute('aria-activedescendant')).toBe(options[2].id);
    await fireEvent.keyDown(box, { key: 'Enter' });
    expect(onchange).toHaveBeenLastCalledWith([opts[2]]);
    expect((box as HTMLInputElement).value).toBe('Ultra HD');
    expect(box.getAttribute('aria-expanded')).toBe('false');
  });

  it('says when nothing matches and when the search fails, with retry', async () => {
    const { getByRole, findByText } = render(AsyncPicker, { search, label: 'Profile', noun: 'profiles', debounce: 0 });
    const box = getByRole('combobox');
    await fireEvent.focus(box);
    await fireEvent.input(box, { target: { value: 'zzz' } });
    expect(await findByText('No profiles match “zzz”', { selector: '.msg span' })).toBeTruthy();
    const failing = render(AsyncPicker, { search: () => Promise.reject(new Error('HTTP 503')), label: 'Tags', noun: 'tags', debounce: 0 });
    await fireEvent.focus(failing.getByRole('combobox', { name: 'Tags' }));
    expect(await failing.findByText("Couldn't search tags: HTTP 503", { selector: '.msg span' })).toBeTruthy();
    expect(failing.getByRole('button', { name: 'Retry' })).toBeTruthy();
  });

  it('multiple: toggles picks as chips and Backspace removes the last', async () => {
    const onchange = vi.fn();
    const { getByRole, findAllByRole, getByLabelText } = render(AsyncPicker, { search, label: 'Tags', multiple: true, value: [opts[0]], debounce: 0, onchange });
    expect(getByLabelText('Remove HD 1080p')).toBeTruthy();
    const box = getByRole('combobox');
    await fireEvent.focus(box);
    const options = await findAllByRole('option');
    expect(options[0].getAttribute('aria-selected')).toBe('true');
    await fireEvent.click(options[2]);
    expect(onchange).toHaveBeenLastCalledWith([opts[0], opts[2]]);
    await fireEvent.keyDown(box, { key: 'Backspace' });
    expect(onchange).toHaveBeenLastCalledWith([opts[0]]);
  });

  it('closes on Escape and ignores stale responses', async () => {
    let resolveFirst: (v: PickerOption[]) => void = () => {};
    const slow = vi.fn((q: string) => (q === 'h' ? new Promise<PickerOption[]>((r) => (resolveFirst = r)) : Promise.resolve([opts[2]])));
    const { getByRole, findAllByRole, queryAllByRole } = render(AsyncPicker, { search: slow, label: 'P', minChars: 1, debounce: 0 });
    const box = getByRole('combobox');
    await fireEvent.focus(box);
    await fireEvent.input(box, { target: { value: 'h' } });
    await waitFor(() => expect(slow).toHaveBeenCalledWith('h', expect.anything()));
    await fireEvent.input(box, { target: { value: 'ul' } });
    expect(await findAllByRole('option')).toHaveLength(1);
    resolveFirst(opts);
    await new Promise((r) => setTimeout(r, 0));
    expect(queryAllByRole('option')).toHaveLength(1);
    await fireEvent.keyDown(box, { key: 'Escape' });
    expect(box.getAttribute('aria-expanded')).toBe('false');
  });
});
