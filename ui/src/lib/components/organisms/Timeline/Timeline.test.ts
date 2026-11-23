import { describe, it, expect } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import Timeline, { type TimelineEntry } from './Timeline.svelte';

const now = new Date(2026, 10, 22, 14, 5);
const entries: TimelineEntry[] = [
  { id: 'a', kind: 'transition', at: new Date(2026, 10, 21, 23, 40), title: 'Marked stalled', from: { family: 'download', value: 'downloading' }, to: { family: 'download', value: 'stalled' }, href: '#why' },
  { id: 'b', kind: 'attempt', at: new Date(2026, 10, 22, 9, 10), title: 'Retry 2 of 3', status: { family: 'job', value: 'done' }, actor: 'Rule: stalled downloads' },
  { id: 'c', kind: 'observation', at: new Date(2026, 10, 20, 8, 0), title: 'Seen on the client', fields: [{ label: 'Hash', value: 'abc123', mono: true }] },
];

describe('Timeline', () => {
  it('groups by day, newest first, with transitions as from → to badges', () => {
    const { getAllByRole, getByText } = render(Timeline, { entries, label: 'History', now });
    expect(getAllByRole('heading', { level: 3 }).map((h) => h.textContent)).toEqual(['Today', 'Yesterday', 'Friday 20 November']);
    const items = getAllByRole('listitem');
    expect(items[0].textContent).toContain('Retry 2 of 3');
    expect(items[1].textContent).toMatch(/Downloading.*Stalled/);
    expect(getByText('Why?').getAttribute('href')).toBe('#why');
    expect(items[0].textContent).toContain('Attempt:');
  });

  it('collapses behind "Show earlier" and expands', async () => {
    const { getAllByRole, getByRole } = render(Timeline, { entries, label: 'History', limit: 1, now });
    expect(getAllByRole('listitem').length).toBe(1);
    await fireEvent.click(getByRole('button', { name: 'Show 2 earlier' }));
    expect(getAllByRole('listitem').length).toBe(3);
    expect(getByRole('button', { name: 'Show fewer' })).toBeTruthy();
  });

  it('shows details on demand and an empty state', () => {
    const { getByText, unmount } = render(Timeline, { entries, label: 'History', now });
    expect(getByText('Details').closest('details')).toBeTruthy();
    unmount();
    const r = render(Timeline, { entries: [], label: 'History', emptyTitle: 'No attempts yet' });
    expect(r.getByText('No attempts yet')).toBeTruthy();
  });
});
