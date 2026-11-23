import { describe, it, expect, vi, afterEach } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import { flushSync } from 'svelte';
import VariantPicker, { type Variant } from './VariantPicker.svelte';

afterEach(() => vi.useRealTimers());

const now = Date.parse('2026-11-22T14:05:00Z');
const variants: Variant[] = [
  { id: 'a', title: 'Dockside.2026.1080p.WEB-DL.DDP5.1-HARBOUR', quality: 'WEB-DL 1080p', size: 6.2e9, score: 1250, reason: 'Same group, seeded 1,240', indexer: 'Harbour Tracker' },
  { id: 'b', title: 'Dockside.2026.1080p.BluRay.x264-QUAY', quality: 'Bluray 1080p', size: 9.8e9, score: 900, reason: 'Larger, well seeded' },
  { id: 'c', title: 'Dockside.2026.720p.WEB-DL-HARBOUR', quality: 'WEB-DL 720p', size: 2.9e9, score: -100, reason: 'Below cutoff' },
];

describe('VariantPicker', () => {
  it('lists candidates as radios with the default selected and marked', () => {
    const { getAllByRole, getByRole, getByText } = render(VariantPicker, { variants, defaultId: 'a', now });
    expect(getByRole('form', { name: 'Pick a replacement release' })).toBeTruthy();
    const radios = getAllByRole('radio') as HTMLInputElement[];
    expect(radios).toHaveLength(3);
    expect(radios[0].checked).toBe(true);
    expect(getByText('Default')).toBeTruthy();
    expect(getByText('Score −100')).toBeTruthy();
    expect(getByText('6.2 GB')).toBeTruthy();
  });

  it('shows a countdown and picks the default automatically when it ends', async () => {
    vi.useFakeTimers({ now });
    const onpick = vi.fn();
    const { container } = render(VariantPicker, { variants, defaultId: 'a', autoAcceptAt: now + 30_000, onpick });
    expect(container.querySelector('.auto .visually-hidden')!.textContent).toBe('Picks the default in 1m');
    vi.advanceTimersByTime(31_000);
    flushSync();
    expect(onpick).toHaveBeenCalledWith('a', true);
  });

  it('stops the countdown when another candidate is chosen, and picks it with Enter', async () => {
    vi.useFakeTimers({ now });
    const onpick = vi.fn();
    const { getAllByRole, getByText } = render(VariantPicker, { variants, defaultId: 'a', autoAcceptAt: now + 30_000, onpick });
    const radios = getAllByRole('radio');
    await fireEvent.click(radios[1]);
    expect(getByText('Timer stopped. Pick when ready.')).toBeTruthy();
    vi.advanceTimersByTime(60_000);
    expect(onpick).not.toHaveBeenCalled();
    // Enter in a radio submits the form without a submitter: picks the selection.
    await fireEvent.submit(radios[1].closest('form')!);
    expect(onpick).toHaveBeenCalledWith('b', false);
  });

  it('cancels with the button and with Escape', async () => {
    const oncancel = vi.fn();
    const onpick = vi.fn();
    const { getByRole, getAllByRole } = render(VariantPicker, { variants, oncancel, onpick, now });
    await fireEvent.click(getByRole('button', { name: 'Cancel' }));
    expect(oncancel).toHaveBeenCalledTimes(1);
    await fireEvent.keyDown(getAllByRole('radio')[0], { key: 'Escape' });
    expect(oncancel).toHaveBeenCalledTimes(2);
    expect(onpick).not.toHaveBeenCalled();
  });

  it('offers Search again when there is nothing to pick', async () => {
    const onsearch = vi.fn();
    const { getByRole, getByText } = render(VariantPicker, { variants: [], onsearch, cap: 'Below 1080p' });
    expect(getByText(/No other releases fit/)).toBeTruthy();
    expect(getByText('Below 1080p')).toBeTruthy();
    await fireEvent.click(getByRole('button', { name: 'Search again' }));
    expect(onsearch).toHaveBeenCalled();
  });

  it('works as a script-free form: named radios and submit buttons', () => {
    const { container } = render(VariantPicker, { variants, formAction: '/tg/replace/d2', now });
    const form = container.querySelector('form')!;
    expect(form.getAttribute('action')).toBe('/tg/replace/d2');
    expect(form.querySelectorAll('input[name="variant"]')).toHaveLength(3);
    expect([...form.querySelectorAll('button[type="submit"]')].map((b) => (b as HTMLButtonElement).value)).toEqual(['cancel', 'pick']);
  });
});
