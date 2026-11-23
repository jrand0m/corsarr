import { describe, it, expect, beforeEach } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import { createRawSnippet } from 'svelte';
import DetailPage from './DetailPage.svelte';

const entity = { title: 'Dockside', kind: 'movie' as const, meta: '2026 · 1h 52m' };
const tabs = [{ id: 'overview', label: 'Overview' }, { id: 'files', label: 'Files', count: 2 }];
const children = createRawSnippet((id: () => string) => ({ render: () => `<p>Panel ${id()}</p>` }));
const aside = createRawSnippet(() => ({ render: () => '<p>Side panel</p>' }));
const now = new Date('2026-11-22T14:05:00Z');
const history = [{ id: 'h1', at: '2026-11-22T13:00:00Z', kind: 'transition' as const, title: 'Imported Dockside' }];

beforeEach(() => window.history.replaceState(null, '', '#/library/movies/7'));

describe('DetailPage', () => {
  it('renders header, tabs, side panels and history', async () => {
    const { getByRole, getByText } = render(DetailPage, { entity, tabs, children, aside, history, now });
    expect(getByRole('heading', { level: 1, name: 'Dockside' })).toBeTruthy();
    expect(getByText('Panel overview')).toBeTruthy();
    expect(getByRole('complementary', { name: 'Details' }).textContent).toContain('Side panel');
    expect(getByRole('heading', { level: 2, name: 'History' })).toBeTruthy();
    expect(getByText('Imported Dockside')).toBeTruthy();
    await fireEvent.click(getByRole('tab', { name: /Files/ }));
    expect(getByRole('tab', { name: /Files/ }).getAttribute('aria-selected')).toBe('true');
  });

  it('keeps the tab in the URL when syncUrl is on', async () => {
    window.history.replaceState(null, '', '#/library/movies/7?tab=files');
    const { getByRole, getByText } = render(DetailPage, { entity, tabs, children, syncUrl: true });
    expect(getByText('Panel files')).toBeTruthy();
    await fireEvent.click(getByRole('tab', { name: 'Overview' }));
    expect(location.hash).toBe('#/library/movies/7');
    await fireEvent.click(getByRole('tab', { name: /Files/ }));
    expect(location.hash).toBe('#/library/movies/7?tab=files');
  });

  it('shows loading and error states', () => {
    const { getByLabelText, unmount } = render(DetailPage, { entity, loading: true });
    expect(getByLabelText('Loading Dockside').getAttribute('aria-busy')).toBe('true');
    unmount();
    const r = render(DetailPage, { entity, error: 'Not found', onretry: () => {} });
    expect(r.getByText("Couldn't load Dockside")).toBeTruthy();
    expect(r.getByRole('heading', { level: 1, name: 'Dockside' })).toBeTruthy();
  });
});
