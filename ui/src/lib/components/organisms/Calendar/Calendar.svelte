<!--
  Calendar (lazy chunk): what airs, releases or arrives, by week, month or agenda.
  Each event is a chip with its media kind, time, title and status (icon + tone; the label is in
  the chip's name and in week and agenda views). The page owns `view` and `date` (URL state:
  `/wanted/calendar?view=&from=`) through `onnavigate`. Load it with `loadCalendar()`.
-->
<script lang="ts" module>
  import type { MediaKind } from '../../../tokens';
  import type { StatusFamily } from '../../../status/vocabulary';

  export type CalendarView = 'week' | 'month' | 'agenda';
  export interface CalendarEvent {
    id: string;
    title: string;
    at: Date | string | number;
    kind: MediaKind;
    /** "S02E03", "Digital release", "Vol. 4". */
    detail?: string;
    status?: { family: StatusFamily; value: string };
  }
</script>

<script lang="ts">
  import Button from '../../atoms/Button/Button.svelte';
  import IconButton from '../../atoms/IconButton/IconButton.svelte';
  import Icon from '../../atoms/Icon/Icon.svelte';
  import MediaKindIcon, { kindLabels } from '../../atoms/MediaKindIcon/MediaKindIcon.svelte';
  import Skeleton from '../../atoms/Skeleton/Skeleton.svelte';
  import StatusBadge from '../../atoms/StatusBadge/StatusBadge.svelte';
  import SegmentedControl from '../../molecules/SegmentedControl/SegmentedControl.svelte';
  import EmptyState from '../../molecules/EmptyState/EmptyState.svelte';
  import { lookupStatus } from '../../../status/vocabulary';
  import { toDate } from '../../../format/time';
  import {
    addDays, addMonths, formatDayLong, formatMonthTitle, formatTimeOfDay, formatWeekRange, formatWeekday,
    isoDay, monthGrid, sameDay, startOfDay, startOfWeek,
  } from '../../../format/calendar';

  interface Props {
    events: CalendarEvent[];
    view?: CalendarView;
    /** Any day inside the shown period. */
    date?: Date;
    /** Fixed "now" for stories and tests. */
    now?: Date;
    weekStartsOn?: 0 | 1;
    /** Days the agenda covers from `date`. */
    agendaDays?: number;
    loading?: boolean;
    label?: string;
    onnavigate?: (state: { view: CalendarView; date: Date }) => void;
    onopen?: (event: CalendarEvent) => void;
  }
  let {
    events, view = $bindable('week'), date = $bindable(new Date()), now, weekStartsOn = 1, agendaDays = 14,
    loading = false, label = 'Calendar', onnavigate, onopen,
  }: Props = $props();

  const today = $derived(startOfDay(now ?? new Date()));
  const MONTH_CHIPS = 3;

  function go(next: { view?: CalendarView; date?: Date }) {
    if (next.view) view = next.view;
    if (next.date) date = next.date;
    onnavigate?.({ view, date });
  }
  function step(dir: 1 | -1) {
    go({ date: view === 'month' ? addMonths(date, dir) : addDays(date, dir * (view === 'week' ? 7 : agendaDays)) });
  }

  const byDay = $derived.by(() => {
    const m = new Map<string, (CalendarEvent & { d: Date })[]>();
    for (const e of events) {
      const d = toDate(e.at);
      const k = isoDay(d);
      if (!m.has(k)) m.set(k, []);
      m.get(k)!.push({ ...e, d });
    }
    for (const list of m.values()) list.sort((a, b) => a.d.getTime() - b.d.getTime());
    return m;
  });
  const on = (d: Date) => byDay.get(isoDay(d)) ?? [];

  const weekStart = $derived(startOfWeek(date, weekStartsOn));
  const weekDays = $derived(Array.from({ length: 7 }, (_, i) => addDays(weekStart, i)));
  const grid = $derived(monthGrid(date, weekStartsOn));
  const agenda = $derived(Array.from({ length: agendaDays }, (_, i) => addDays(startOfDay(date), i)).filter((d) => on(d).length));
  const title = $derived(
    view === 'month' ? formatMonthTitle(date) : view === 'week' ? formatWeekRange(weekStart) : `${formatDayLong(startOfDay(date))} and the next ${agendaDays - 1} days`,
  );
  const periodHasEvents = $derived(
    view === 'agenda' ? agenda.length > 0 : (view === 'week' ? weekDays : grid.filter((d) => d.getMonth() === date.getMonth())).some((d) => on(d).length),
  );
  const statusText = (e: CalendarEvent) => (e.status ? (lookupStatus(e.status.family, e.status.value)?.label ?? e.status.value) : undefined);
  const chipName = (e: CalendarEvent & { d: Date }) =>
    [formatTimeOfDay(e.d), kindLabels[e.kind], e.title, e.detail, statusText(e)].filter(Boolean).join(', ');
</script>

{#snippet chip(e: CalendarEvent & { d: Date }, dense: boolean)}
  {@const st = e.status ? lookupStatus(e.status.family, e.status.value) : undefined}
  <button type="button" class="chip tone-{st?.tone ?? 'neutral'}" class:dense aria-label={chipName(e)} title={dense ? chipName(e) : undefined} onclick={() => onopen?.(e)}>
    {#if st}<span class="st"><Icon name={st.icon} size="sm" /></span>{/if}
    <span class="time num">{formatTimeOfDay(e.d)}</span>
    <span class="ct">{e.title}</span>
    {#if e.detail && !dense}<span class="cd">{e.detail}</span>{/if}
  </button>
{/snippet}

<section class="cal" aria-label={label} aria-busy={loading}>
  <header class="bar">
    <div class="nav">
      <IconButton icon="chevron-left" label={view === 'month' ? 'Previous month' : view === 'week' ? 'Previous week' : 'Earlier'} variant="secondary" onclick={() => step(-1)} />
      <Button onclick={() => go({ date: today })}>Today</Button>
      <IconButton icon="chevron-right" label={view === 'month' ? 'Next month' : view === 'week' ? 'Next week' : 'Later'} variant="secondary" onclick={() => step(1)} />
    </div>
    <h2 class="title" aria-live="polite">{title}</h2>
    <span class="views">
      <SegmentedControl label="Calendar view" size="sm" value={view} onchange={(v) => go({ view: v as CalendarView })}
        options={[{ value: 'week', label: 'Week' }, { value: 'month', label: 'Month' }, { value: 'agenda', label: 'Agenda' }]} />
    </span>
  </header>

  {#if loading}
    <div class="loading"><Skeleton shape="block" height="18rem" /></div>
  {:else if view === 'month'}
    <table class="month">
      <thead><tr>{#each weekDays as d (d.getTime())}<th scope="col">{formatWeekday(d)}</th>{/each}</tr></thead>
      <tbody>
        {#each Array(grid.length / 7) as _, w (w)}
          <tr>
            {#each grid.slice(w * 7, w * 7 + 7) as d (d.getTime())}
              {@const list = on(d)}
              <td class:out={d.getMonth() !== date.getMonth()} class:today={sameDay(d, today)}>
                <button type="button" class="daynum num" aria-label="{formatDayLong(d)}{sameDay(d, today) ? ', today' : ''}, {list.length === 1 ? '1 event' : `${list.length} events`}. Show the week."
                  aria-current={sameDay(d, today) ? 'date' : undefined}
                  onclick={() => go({ view: 'week', date: d })}>{d.getDate()}</button>
                {#if list.length}<span class="phone-count num" aria-hidden="true">{list.length}</span>{/if}
                <div class="chips">
                  {#each list.slice(0, MONTH_CHIPS) as e (e.id)}{@render chip(e, true)}{/each}
                  {#if list.length > MONTH_CHIPS}
                    <button type="button" class="more" onclick={() => go({ view: 'week', date: d })}>{list.length - MONTH_CHIPS} more</button>
                  {/if}
                </div>
              </td>
            {/each}
          </tr>
        {/each}
      </tbody>
    </table>
  {:else if view === 'week'}
    <ol class="week">
      {#each weekDays as d (d.getTime())}
        {@const list = on(d)}
        <li class="day" class:today={sameDay(d, today)} aria-current={sameDay(d, today) ? 'date' : undefined}>
          <h3 class="dayhead"><span class="wd">{formatWeekday(d)}</span> <span class="dn num">{d.getDate()}</span>{#if sameDay(d, today)}<span class="visually-hidden">, today</span>{/if}</h3>
          {#if list.length}
            <ul class="daylist">{#each list as e (e.id)}<li>{@render chip(e, false)}</li>{/each}</ul>
          {:else}
            <p class="none">Nothing</p>
          {/if}
        </li>
      {/each}
    </ol>
    {#if !periodHasEvents}<p class="quiet">Nothing airs or releases this week.</p>{/if}
  {:else}
    {#if agenda.length}
      <div class="agenda">
        {#each agenda as d (d.getTime())}
          <section class="aday" aria-labelledby="a-{isoDay(d)}">
            <h3 id="a-{isoDay(d)}" class:today={sameDay(d, today)}>{formatDayLong(d)}{#if sameDay(d, today)}<span class="tag">Today</span>{/if}</h3>
            <ul>
              {#each on(d) as e (e.id)}
                <li>
                  <button type="button" class="arow" onclick={() => onopen?.(e)}>
                    <span class="time num">{formatTimeOfDay(e.d)}</span>
                    <MediaKindIcon kind={e.kind} />
                    <span class="at">{e.title}</span>
                    {#if e.detail}<span class="cd">{e.detail}</span>{/if}
                    {#if e.status}<span class="ab"><StatusBadge family={e.status.family} value={e.status.value} size="sm" appearance="plain" /></span>{/if}
                  </button>
                </li>
              {/each}
            </ul>
          </section>
        {/each}
      </div>
    {:else}
      <EmptyState title="Nothing in the next {agendaDays} days" icon="calendar" size="sm">Monitored items with known dates show up here.</EmptyState>
    {/if}
  {/if}
</section>

<style>
  .cal { display: grid; gap: var(--space-3); min-width: 0; container: cal / inline-size; }
  .bar { display: flex; align-items: center; gap: var(--space-3); flex-wrap: wrap; }
  .nav { display: flex; align-items: center; gap: var(--space-1); }
  .title { font-size: var(--text-lg); font-weight: var(--weight-semibold); line-height: var(--leading-tight); }
  .views { margin-left: auto; }
  .loading { min-height: 18rem; }
  .quiet { font-size: var(--text-sm); color: var(--ink-muted); }

  /* Chips: tone only with a status (Placard Rule). */
  .chip {
    display: flex; align-items: center; gap: var(--space-1); width: 100%; min-width: 0; padding: var(--space-0-5) var(--space-1-5);
    font: inherit; font-size: var(--text-xs); line-height: var(--leading-snug); text-align: start; color: var(--ink); cursor: pointer;
    background: var(--surface-raised); border: var(--border-w) solid var(--line); border-radius: var(--radius-sm); overflow: hidden;
  }
  .chip:hover { background: var(--surface-hover); }
  .chip .st { display: inline-flex; flex: none; }
  .tone-ok { background: var(--tone-ok-tint); border-color: var(--tone-ok-line); } .tone-ok .st { color: var(--tone-ok-ink); }
  .tone-warn { background: var(--tone-warn-tint); border-color: var(--tone-warn-line); } .tone-warn .st { color: var(--tone-warn-ink); }
  .tone-error { background: var(--tone-error-tint); border-color: var(--tone-error-line); } .tone-error .st { color: var(--tone-error-ink); }
  .tone-info { background: var(--tone-info-tint); border-color: var(--tone-info-line); } .tone-info .st { color: var(--tone-info-ink); }
  .tone-neutral .st { color: var(--tone-neutral-ink); }
  .time { color: var(--ink-muted); flex: none; }
  .ct { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; min-width: 0; font-weight: var(--weight-medium); }
  .cd { color: var(--ink-muted); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; min-width: 0; font-family: var(--font-mono); font-size: var(--text-xs); }
  .chip:not(.dense) { flex-wrap: wrap; padding: var(--space-1) var(--space-2); }
  .chip:not(.dense) .ct { white-space: normal; flex-basis: 100%; order: 3; }
  .chip:not(.dense) .cd { order: 4; }

  .month { width: 100%; table-layout: fixed; border-collapse: collapse; background: var(--surface-raised); border: var(--border-w) solid var(--line); border-radius: var(--radius-lg); }
  .month th { font-size: var(--text-xs); font-weight: var(--weight-medium); color: var(--ink-muted); padding: var(--space-1-5); background: var(--surface-sunken); text-align: start; }
  .month td { vertical-align: top; height: 7rem; padding: var(--space-1); border: var(--border-w) solid var(--line); }
  .month td.out { background: var(--surface-ground); }
  .month td.out .daynum { color: var(--ink-subtle); }
  .daynum {
    font: inherit; font-size: var(--text-sm); color: var(--ink); background: none; border: 0; cursor: pointer;
    min-width: 1.75rem; height: 1.75rem; border-radius: var(--radius-round); padding: 0 var(--space-1);
  }
  .daynum:hover { background: var(--surface-hover); }
  .today .daynum { background: var(--accent); color: var(--ink-on-accent); font-weight: var(--weight-semibold); }
  .chips { display: grid; gap: var(--space-0-5); margin-top: var(--space-0-5); min-width: 0; }
  .more { font: inherit; font-size: var(--text-xs); color: var(--accent-ink); background: none; border: 0; padding: var(--space-0-5) var(--space-1); text-align: start; cursor: pointer; }
  .more:hover { text-decoration: underline; }
  .phone-count { display: none; }

  .week { list-style: none; margin: 0; padding: 0; display: grid; grid-template-columns: repeat(7, minmax(0, 1fr)); border: var(--border-w) solid var(--line); border-radius: var(--radius-lg); background: var(--surface-raised); overflow: clip; }
  .day { display: grid; align-content: start; gap: var(--space-2); padding: var(--space-2); min-height: 12rem; border-left: var(--border-w) solid var(--line); min-width: 0; }
  .day:first-child { border-left: 0; }
  .day.today { background: var(--accent-tint); }
  .dayhead { display: flex; align-items: baseline; gap: var(--space-1); font-size: var(--text-sm); font-weight: var(--weight-medium); color: var(--ink-muted); }
  .dn { font-size: var(--text-lg); color: var(--ink); font-weight: var(--weight-semibold); }
  .today .dn { color: var(--accent-ink); }
  .daylist { list-style: none; margin: 0; padding: 0; display: grid; gap: var(--space-1); }
  .none { font-size: var(--text-xs); color: var(--ink-subtle); }

  .agenda { display: grid; gap: var(--space-5); }
  .aday { display: grid; gap: var(--space-1-5); }
  .aday h3 { display: flex; align-items: center; gap: var(--space-2); font-size: var(--text-md); font-weight: var(--weight-semibold); }
  .tag { font-size: var(--text-xs); font-weight: var(--weight-medium); color: var(--accent-ink); background: var(--accent-tint); padding: 0 var(--space-1-5); border-radius: var(--radius-sm); }
  .aday ul { list-style: none; margin: 0; padding: 0; border: var(--border-w) solid var(--line); border-radius: var(--radius-lg); background: var(--surface-raised); }
  .aday li + li { border-top: var(--border-w) solid var(--line); }
  .arow {
    display: flex; align-items: center; gap: var(--space-3); width: 100%; min-height: var(--row-h-comfortable); padding: 0 var(--space-3);
    font: inherit; font-size: var(--text-md); color: var(--ink); background: none; border: 0; text-align: start; cursor: pointer;
  }
  .arow:hover { background: var(--surface-hover); }
  .at { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .ab { flex: none; }

  /* Narrow (a phone, or a narrow panel): stack the week, reduce the month to counts. */
  @container cal (width < 44rem) {
    .views { margin-left: 0; width: 100%; }
    .title { order: -1; width: 100%; }
    .week { grid-template-columns: 1fr; }
    .day { border-left: 0; border-top: var(--border-w) solid var(--line); min-height: 0; }
    .day:first-child { border-top: 0; }
    .month td { height: 3.25rem; padding: var(--space-0-5); text-align: center; }
    .month .chips { display: none; }
    .daynum { min-width: var(--target-min); height: var(--target-min); }
    .phone-count { display: block; font-size: var(--text-2xs); color: var(--ink-muted); }
    .arow { flex-wrap: wrap; padding: var(--space-2) var(--space-3); }
  }
</style>
