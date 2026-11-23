<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import Calendar, { type CalendarEvent } from './Calendar.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';

  const now = new Date(2026, 10, 22, 14, 5);
  const at = (day: number, h: number, m = 0) => new Date(2026, 10, day, h, m);
  const events: CalendarEvent[] = [
    { id: 'e1', title: 'The Long Haul', detail: 'S02E03', kind: 'tv', at: at(16, 21), status: { family: 'item', value: 'cutoff-met' } },
    { id: 'e2', title: 'Quay Lights', detail: 'Digital release', kind: 'movie', at: at(17, 0), status: { family: 'item', value: 'missing' } },
    { id: 'e3', title: 'Night Shift', detail: 'S05E10', kind: 'tv', at: at(18, 22), status: { family: 'item', value: 'queued' } },
    { id: 'e4', title: 'Salt Road', detail: 'Album', kind: 'music', at: at(20, 0), status: { family: 'item', value: 'cutoff-unmet' } },
    { id: 'e5', title: 'The Long Haul', detail: 'S02E04', kind: 'tv', at: at(23, 21), status: { family: 'item', value: 'not-available' } },
    { id: 'e6', title: 'Northern Crossing', detail: 'S01E01', kind: 'tv', at: at(23, 22), status: { family: 'item', value: 'not-available' } },
    { id: 'e7', title: 'Iron Tide', detail: 'S03E07', kind: 'tv', at: at(23, 23), status: { family: 'item', value: 'not-available' } },
    { id: 'e8', title: 'Harbour Watch', detail: 'S01E02', kind: 'tv', at: at(23, 23, 30), status: { family: 'item', value: 'not-available' } },
    { id: 'e9', title: 'Pilot Signal', detail: 'Vol. 4', kind: 'book', at: at(26, 9), status: { family: 'item', value: 'not-available' } },
    { id: 'e10', title: 'Grey Manifest', detail: 'Cinema release', kind: 'movie', at: at(22, 18), status: { family: 'item', value: 'not-available' } },
    { id: 'e11', title: 'Night Shift', detail: 'S05E09', kind: 'tv', at: at(11, 22), status: { family: 'item', value: 'cutoff-met' } },
    { id: 'e12', title: 'Low Water', detail: 'S01E08', kind: 'tv', at: at(4, 20), status: { family: 'item', value: 'cutoff-met' } },
  ];

  const { Story } = defineMeta({
    title: 'Organisms/Calendar',
    component: Calendar,
    tags: ['autodocs'],
    args: { events, view: 'week', date: now, now },
  });
</script>

<Story name="Playground" />

<Story name="Views" asChild>
  <ThemePair>
    <Row label="Week"><div style="width: 100%"><Calendar {events} view="week" date={now} {now} /></div></Row>
    <Row label="Month, with a busy day"><div style="width: 100%"><Calendar {events} view="month" date={now} {now} /></div></Row>
    <Row label="Agenda"><div style="width: 100%"><Calendar {events} view="agenda" date={now} {now} /></div></Row>
  </ThemePair>
</Story>

<Story name="States" asChild>
  <ThemePair>
    <Row label="Loading"><div style="width: 100%"><Calendar events={[]} view="week" date={now} {now} loading /></div></Row>
    <Row label="Empty week"><div style="width: 100%"><Calendar events={[]} view="week" date={now} {now} /></div></Row>
    <Row label="Empty agenda"><div style="width: 100%"><Calendar events={[]} view="agenda" date={now} {now} /></div></Row>
  </ThemePair>
</Story>
