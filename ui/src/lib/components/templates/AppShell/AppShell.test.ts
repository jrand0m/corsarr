import { describe, it, expect, vi } from 'vitest';
import { render, fireEvent, within } from '@testing-library/svelte';
import { createRawSnippet } from 'svelte';
import AppShell, { shellNav } from './AppShell.svelte';

const status = { unhealthy: 2, healthTone: 'error' as const, active: 3, blocked: 1 };
const children = createRawSnippet(() => ({ render: () => '<h1>Movies</h1>' }));
const paletteOpen = () => !!document.querySelector('dialog[open]');

describe('shellNav', () => {
  it('lists only enabled kinds under Library, in a fixed order', () => {
    const lib = shellNav(['movie', 'tv'])[1].children!.map((c) => c.label);
    expect(lib).toEqual(['TV', 'Movies', 'Unmapped files', 'Recycle area']);
    expect(shellNav(['music'])[1].children![0].path).toBe('/library/music');
  });
});

describe('AppShell', () => {
  it('renders landmarks, a skip link to main and marks the current screen', () => {
    const { getByRole, getAllByRole } = render(AppShell, { kinds: ['movie'], path: '/library/movies/42', status, children });
    const main = getByRole('main');
    expect(getByRole('link', { name: 'Skip to content' }).getAttribute('href')).toBe(`#${main.id}`);
    const rail = getByRole('navigation', { name: 'Main' });
    expect(within(rail).queryByRole('link', { name: 'TV' })).toBeNull();
    expect(within(rail).getByRole('link', { name: 'Movies' }).getAttribute('aria-current')).toBe('page');
    expect(getAllByRole('link', { name: '2 integrations need attention' }).length).toBeGreaterThan(0);
  });

  it('opens the command palette with Ctrl+K and /, but leaves / to a local filter that took it', async () => {
    render(AppShell, { status, children });
    await fireEvent.keyDown(window, { key: 'k', ctrlKey: true });
    expect(paletteOpen()).toBe(true);
    document.querySelector('dialog')!.close();
    await fireEvent.keyDown(document.body, { key: '/' });
    expect(paletteOpen()).toBe(true);
  });

  it('ignores / when a local filter handled it first', async () => {
    render(AppShell, { status, children });
    const ev = new KeyboardEvent('keydown', { key: '/', bubbles: true, cancelable: true });
    ev.preventDefault();
    window.dispatchEvent(ev);
    expect(paletteOpen()).toBe(false);
  });

  it('goes to a screen with g then a letter, and from a palette screen entry', async () => {
    const onnavigate = vi.fn();
    const { getByRole, getAllByRole } = render(AppShell, { status, children, onnavigate });
    await fireEvent.keyDown(document.body, { key: 'g' });
    await fireEvent.keyDown(document.body, { key: 'q' });
    expect(onnavigate).toHaveBeenLastCalledWith('/activity/queue');
    await fireEvent.keyDown(document.body, { key: 'g' });
    await fireEvent.keyDown(document.body, { key: 'l' });
    expect(onnavigate).toHaveBeenLastCalledWith('/library/tv');
    await fireEvent.click(getByRole('button', { name: /Search titles, screens and actions/ }));
    await fireEvent.input(getByRole('combobox'), { target: { value: 'health' } });
    await fireEvent.click(getAllByRole('option').find((o) => o.textContent?.includes('Health'))!);
    expect(onnavigate).toHaveBeenLastCalledWith('/system/health');
  });

  it('toggles the phone nav sheet from More', async () => {
    const { getByRole } = render(AppShell, { status, children });
    const more = getByRole('button', { name: 'More' });
    expect(more.getAttribute('aria-expanded')).toBe('false');
    await fireEvent.click(more);
    expect(getByRole('button', { name: 'Close' }).getAttribute('aria-expanded')).toBe('true');
    expect(document.getElementById(more.getAttribute('aria-controls')!)!.classList.contains('open')).toBe(true);
  });

  it('switches theme and signs out from the account menu', async () => {
    const ontheme = vi.fn();
    const onsignout = vi.fn();
    const { getByRole } = render(AppShell, { status, children, ontheme, onsignout, theme: 'system' });
    const open = () => document.querySelector<HTMLElement>('.account [popover]')!.showPopover();
    expect(getByRole('button', { name: 'Theme and session' })).toBeTruthy();
    open();
    await fireEvent.click(getByRole('button', { name: 'Use dark theme' }));
    expect(ontheme).toHaveBeenCalledWith('dark');
    open();
    await fireEvent.click(getByRole('button', { name: 'Sign out' }));
    expect(onsignout).toHaveBeenCalled();
  });

  it('shows the connection banner while reconnecting or offline', () => {
    const { getByText, queryByText, rerender } = render(AppShell, { status, children, connection: 'live' });
    expect(queryByText('Reconnecting to live updates')).toBeNull();
    rerender({ status, children, connection: 'reconnecting' });
    expect(getByText('Reconnecting to live updates', { selector: '*:not(.visually-hidden)' })).toBeTruthy();
    rerender({ status, children, connection: 'offline', offlineSince: '14:05', onreconnect: () => {} });
    expect(getByText(/current at 14:05/)).toBeTruthy();
  });
});
