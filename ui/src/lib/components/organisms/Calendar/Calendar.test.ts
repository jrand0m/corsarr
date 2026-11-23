import { describe, it, expect, vi } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import Calendar, { type CalendarEvent } from './Calendar.svelte';
import { monthGrid, startOfWeek, formatWeekRange } from '../../../format/calendar';

const now = new Date(2026, 10, 22, 14, 5); // Sunday
const events: CalendarEvent[] = [
  { id: 'a', title: 'The Long Haul', detail: 'S02E03', kind: 'tv', at: new Date(2026, 10, 16, 21), status: { family: 'item', value: 'cutoff-met' } },
  { id: 'b', title: 'Quay Lights', kind: 'movie', at: new Date(2026, 10, 17, 0), status: { family: 'item', value: 'missing' } },
  ...Array.from({ length: 5 }, (_, i) => ({ id: `c${i}`, title: `Busy ${i}`, kind: 'tv' as const, at: new Date(2026, 10, 23, 18 + i) })),
];

describe('calendar maths', () => {
  it('starts weeks on Monday and covers whole weeks of the month', () => {
    expect(startOfWeek(now).getDate()).toBe(16);
    const g = monthGrid(now);
    expect(g.length % 7).toBe(0);
    expect(g[0].getDay()).toBe(1);
    expect(formatWeekRange(startOfWeek(now))).toBe('16 – 22 Nov 2026');
    expect(formatWeekRange(new Date(2026, 10, 30))).toBe('30 Nov – 6 Dec 2026');
  });
});

describe('Calendar', () => {
  it('shows the week with chips named by time, kind, title and status', async () => {
    const onopen = vi.fn();
    const { getByRole, getByText } = render(Calendar, { props: { events, view: 'week', date: now, now, onopen } });
    expect(getByRole('heading', { level: 2 }).textContent).toBe('16 – 22 Nov 2026');
    const chip = getByRole('button', { name: '21:00, TV, The Long Haul, S02E03, On disk' });
    await fireEvent.click(chip);
    expect(onopen).toHaveBeenCalledWith(expect.objectContaining({ id: 'a' }));
    expect(getByText('22').closest('li')?.getAttribute('aria-current')).toBe('date');
  });

  it('steps by week and reports navigation', async () => {
    const onnavigate = vi.fn();
    const { getByRole } = render(Calendar, { props: { events, view: 'week', date: now, now, onnavigate } });
    await fireEvent.click(getByRole('button', { name: 'Next week' }));
    expect(getByRole('heading', { level: 2 }).textContent).toBe('23 – 29 Nov 2026');
    expect(onnavigate.mock.lastCall![0].date.getDate()).toBe(29);
  });

  it('caps month chips and opens the week of a busy day', async () => {
    const { getByRole } = render(Calendar, { props: { events, view: 'month', date: now, now } });
    expect(getByRole('heading', { level: 2 }).textContent).toBe('November 2026');
    await fireEvent.click(getByRole('button', { name: '2 more' }));
    expect(getByRole('heading', { level: 2 }).textContent).toBe('23 – 29 Nov 2026');
  });

  it('lists only days with events in the agenda, or says nothing is coming', async () => {
    const { getAllByRole, unmount } = render(Calendar, { props: { events, view: 'agenda', date: new Date(2026, 10, 16), now, agendaDays: 14 } });
    expect(getAllByRole('heading', { level: 3 }).map((h) => h.textContent)).toEqual(['Monday 16 November', 'Tuesday 17 November', 'Monday 23 November']);
    unmount();
    const r = render(Calendar, { props: { events: [], view: 'agenda', date: now, now } });
    expect(r.getByText('Nothing in the next 14 days')).toBeTruthy();
  });
});
