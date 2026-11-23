import { describe, it, expect, vi } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import FilterBar from './FilterBar.svelte';
import { filterFromParams, filterToParams, activeCount, type FilterFacet } from '../../../filters/url';

const facets: FilterFacet[] = [
  { id: 'status', label: 'Status', options: [{ value: 'missing', label: 'Missing', count: 23 }, { value: 'queued', label: 'Queued' }] },
  { id: 'monitored', label: 'Monitoring', multiple: false, options: [{ value: 'yes', label: 'Monitored' }, { value: 'no', label: 'Unmonitored' }] },
];

describe('filter URL state', () => {
  it('round-trips through the query string and drops unknown values', () => {
    const p = filterToParams({ q: ' haul ', facets: { status: ['missing', 'queued'], monitored: [] } }, new URLSearchParams('view=posters'));
    expect(p.toString()).toBe('view=posters&q=haul&status=missing%2Cqueued');
    expect(filterFromParams(p, facets)).toEqual({ q: 'haul', facets: { status: ['missing', 'queued'] } });
    expect(filterFromParams(new URLSearchParams('status=gone,missing&monitored=yes,no&x=1'), facets)).toEqual({ q: '', facets: { status: ['missing'], monitored: ['yes'] } });
    expect(activeCount({ q: 'a', facets: { status: ['missing', 'queued'] } })).toBe(3);
  });
});

describe('FilterBar', () => {
  it('emits text and facet changes and shows active filters as removable chips', async () => {
    const onchange = vi.fn();
    const { getByRole, getAllByRole } = render(FilterBar, { facets, searchLabel: 'Filter series', onchange });
    await fireEvent.input(getByRole('searchbox', { name: 'Filter series' }), { target: { value: 'haul' } });
    expect(onchange.mock.lastCall![0]).toEqual({ q: 'haul', facets: {} });
    const open = (name: RegExp) => document.getElementById(getByRole('button', { name }).getAttribute('popovertarget')!)!.showPopover();
    open(/^Status/);
    await fireEvent.click(getByRole('checkbox', { name: 'Missing' }));
    expect(onchange.mock.lastCall![0].facets).toEqual({ status: ['missing'] });
    expect(getByRole('list', { name: 'Active filters' })).toBeTruthy();
    open(/^Monitoring/);
    await fireEvent.click(getAllByRole('radio', { name: 'Unmonitored' })[0]);
    expect(onchange.mock.lastCall![0].facets.monitored).toEqual(['no']);
    await fireEvent.click(getByRole('button', { name: 'Remove Status: Missing' }));
    expect(onchange.mock.lastCall![0].facets).toEqual({ monitored: ['no'] });
    await fireEvent.click(getByRole('button', { name: 'Clear all' }));
    expect(onchange.mock.lastCall![0]).toEqual({ q: '', facets: {} });
  });

  it('focuses the text filter on / unless typing elsewhere', async () => {
    const { getByRole } = render(FilterBar, { facets, searchLabel: 'Filter series' });
    await fireEvent.keyDown(document.body, { key: '/' });
    expect(document.activeElement).toBe(getByRole('searchbox', { name: 'Filter series' }));
  });

  it('counts selected values on the facet button', () => {
    const { getByLabelText } = render(FilterBar, { facets, value: { q: '', facets: { status: ['missing', 'queued'] } } });
    expect(getByLabelText('2 selected')).toBeTruthy();
  });
});
