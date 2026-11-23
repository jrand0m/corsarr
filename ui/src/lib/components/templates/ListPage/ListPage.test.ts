import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import { createRawSnippet } from 'svelte';
import ListPage from './ListPage.svelte';
import type { Column } from '../../organisms/DataTable/DataTable.svelte';
import { libraryRows, type LibraryRow } from '../../../storybook/library-rows';

const columns: Column<LibraryRow>[] = [
  { id: 'title', label: 'Title', sortable: true, value: (r) => r.title, phone: 'title' },
  { id: 'year', label: 'Year', value: (r) => String(r.year) },
];
const facets = [{ id: 'status', label: 'Status', options: [{ value: 'missing', label: 'Missing' }] }];
const poster = (r: LibraryRow) => ({ id: r.id, title: r.title, kind: r.kind, year: r.year });
const base = { title: 'Movies', columns, rowKey: (r: LibraryRow) => r.id, noun: 'movie', facets, searchLabel: 'Filter movies' };

beforeEach(() => history.replaceState(null, '', '#/library/movies'));

describe('ListPage', () => {
  it('renders the header, actions and table', () => {
    const actions = createRawSnippet(() => ({ render: () => '<button>Add movie</button>' }));
    const { getByRole } = render(ListPage<LibraryRow>, { ...base, rows: libraryRows(5), actions, description: '5 movies' });
    expect(getByRole('heading', { level: 1, name: 'Movies' })).toBeTruthy();
    expect(getByRole('button', { name: 'Add movie' })).toBeTruthy();
    expect(getByRole('table', { name: 'Movies' })).toBeTruthy();
  });

  it('switches to posters and back', async () => {
    const onstatechange = vi.fn();
    const { getByRole, queryByRole } = render(ListPage<LibraryRow>, { ...base, rows: libraryRows(5), poster, onstatechange });
    await fireEvent.click(getByRole('radio', { name: 'Posters' }));
    expect(queryByRole('table')).toBeNull();
    expect(onstatechange.mock.lastCall![0].view).toBe('posters');
  });

  it('offers to clear filters when they hide every row', async () => {
    const { getByRole, getByText } = render(ListPage<LibraryRow>, { ...base, rows: [], filter: { q: 'zzz', facets: {} } });
    expect(getByText('No movies match these filters')).toBeTruthy();
    await fireEvent.click(getByRole('button', { name: 'Clear filters' }));
    expect((getByRole('searchbox', { name: 'Filter movies' }) as HTMLInputElement).value).toBe('');
  });

  it('shows the error state in poster view', () => {
    history.replaceState(null, '', '#/library/movies?view=posters');
    const { getByText } = render(ListPage<LibraryRow>, { ...base, rows: [], poster, syncUrl: true, error: 'timeout' });
    expect(getByText("Couldn't load the movies")).toBeTruthy();
  });

  it('reads and writes view state in the URL when syncUrl is on', async () => {
    history.replaceState(null, '', '#/library/movies?status=missing&sort=year:desc');
    const { getByRole } = render(ListPage<LibraryRow>, { ...base, rows: libraryRows(3), poster, syncUrl: true });
    expect(getByRole('columnheader', { name: /Title/ }).getAttribute('aria-sort')).toBe('none');
    await fireEvent.input(getByRole('searchbox', { name: 'Filter movies' }), { target: { value: 'salt' } });
    await fireEvent.click(getByRole('radio', { name: 'Posters' }));
    const q = new URLSearchParams(location.hash.split('?')[1]);
    expect(location.hash.startsWith('#/library/movies?')).toBe(true);
    expect(q.get('status')).toBe('missing');
    expect(q.get('q')).toBe('salt');
    expect(q.get('sort')).toBe('year:desc');
    expect(q.get('view')).toBe('posters');
  });
});
