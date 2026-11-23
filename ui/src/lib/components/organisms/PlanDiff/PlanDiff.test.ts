import { describe, it, expect, vi } from 'vitest';
import { render, fireEvent, within } from '@testing-library/svelte';
import PlanDiff, { type PlanGroup } from './PlanDiff.svelte';

const SECRET = 'sk-7f3a9c1e5b';
const groups: PlanGroup[] = [
  {
    type: 'Indexers',
    ops: [
      { id: 'i1', op: 'create', name: 'Harbour Tracker', fields: [{ path: 'url', after: 'https://feeds.example.net/api' }] },
      { id: 'i2', op: 'update', name: 'Dock Feeds', fields: [
        { path: 'priority', before: '25', after: '10' },
        { path: 'api_key', before: SECRET, after: 'sk-other', secret: true },
      ] },
    ],
  },
  {
    type: 'Quality profiles',
    ops: [
      { id: 'q1', op: 'prune', name: 'SD only' },
      { id: 'q2', op: 'skip', name: 'Ultra HD', reason: 'Last profile for Movies; at least one is kept.', inUseBy: [{ label: '214 movies', href: '#/library/movies?profile=uhd' }] },
    ],
  },
];

describe('PlanDiff', () => {
  it('summarises ops and groups them by type', () => {
    const { getByRole } = render(PlanDiff, { groups });
    const summary = within(getByRole('list', { name: 'Summary' })).getAllByRole('listitem').map((li) => li.textContent!.trim());
    expect(summary).toEqual(['Create · 1', 'Update · 1', 'Prune · 1', 'Skip · 1']);
    expect(getByRole('region', { name: 'Indexers 2' })).toBeTruthy();
    expect(getByRole('region', { name: 'Quality profiles 2' })).toBeTruthy();
  });

  it('shows field-level before → after, and secrets only as "differs"', () => {
    const { container, getByRole } = render(PlanDiff, { groups });
    const table = getByRole('table', { name: 'Field changes for Dock Feeds' });
    const rows = within(table).getAllByRole('row').slice(1);
    expect(rows[0].textContent!.replace(/\s+/g, '')).toBe('priority2510');
    expect(rows[1].textContent).toContain('differs');
    expect(container.innerHTML).not.toContain(SECRET);
    expect(container.innerHTML).not.toContain('sk-other');
  });

  it('explains skipped prunes with what still uses them', () => {
    const { getByRole, container } = render(PlanDiff, { groups });
    expect(container.textContent).toContain('Last profile for Movies; at least one is kept.');
    expect(getByRole('link', { name: '214 movies' }).getAttribute('href')).toBe('#/library/movies?profile=uhd');
  });

  it('offers include toggles and applies only the included ops', async () => {
    const onconfirm = vi.fn();
    const { getByRole, queryByRole } = render(PlanDiff, { groups, onconfirm, oncancel: () => {} });
    expect(queryByRole('checkbox', { name: /Ultra HD/ })).toBeNull();
    await fireEvent.click(getByRole('checkbox', { name: 'Include prune SD only' }));
    await fireEvent.click(getByRole('button', { name: 'Apply 2 changes' }));
    expect(onconfirm).toHaveBeenCalledWith(['i1', 'i2']);
  });

  it('is read-only without onconfirm, and says when there is nothing to change', () => {
    const r = render(PlanDiff, { groups });
    expect(r.queryByRole('checkbox')).toBeNull();
    expect(r.queryByRole('button', { name: /Apply/ })).toBeNull();
    r.unmount();
    expect(render(PlanDiff, { groups: [] }).getByText('Nothing to change')).toBeTruthy();
  });
});
