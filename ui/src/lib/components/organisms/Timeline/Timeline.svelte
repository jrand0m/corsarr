<!--
  Timeline: what happened to one thing, in order. State transitions (from → to badges),
  observations (what corsarr saw), attempts (retries, searches, with their outcome) and applies
  (config plans, rule actions). Grouped by day, newest first by default; long histories collapse
  behind "Show earlier". Entries can link to their explanation and carry detail fields.
-->
<script lang="ts" module>
  import type { StatusFamily } from '../../../status/vocabulary';

  export type TimelineKind = 'transition' | 'observation' | 'attempt' | 'apply' | 'note';
  export interface TimelineStatus { family: StatusFamily; value: string }
  export interface TimelineEntry {
    id: string;
    at: Date | string | number;
    kind: TimelineKind;
    title: string;
    from?: TimelineStatus;
    to?: TimelineStatus;
    /** Outcome of an attempt or apply. */
    status?: TimelineStatus;
    detail?: string;
    /** Who or what did it: "Rule: stalled downloads", "You", "Scheduler". */
    actor?: string;
    /** Link to the explanation or the record. */
    href?: string;
    hrefLabel?: string;
    fields?: { label: string; value: string; mono?: boolean }[];
  }
</script>

<script lang="ts">
  import Icon from '../../atoms/Icon/Icon.svelte';
  import Link from '../../atoms/Link/Link.svelte';
  import Button from '../../atoms/Button/Button.svelte';
  import Skeleton from '../../atoms/Skeleton/Skeleton.svelte';
  import StatusBadge from '../../atoms/StatusBadge/StatusBadge.svelte';
  import Timestamp from '../../atoms/Timestamp/Timestamp.svelte';
  import EmptyState from '../../molecules/EmptyState/EmptyState.svelte';
  import PropertyList from '../../molecules/PropertyList/PropertyList.svelte';
  import type { IconName } from '../../atoms/Icon/icons';
  import { toDate } from '../../../format/time';
  import { addDays, formatDayLong, formatTimeOfDay, isoDay, sameDay, startOfDay } from '../../../format/calendar';
  import { formatCount } from '../../../format/quantity';

  interface Props {
    entries: TimelineEntry[];
    label: string;
    order?: 'newest' | 'oldest';
    /** Show this many entries, then "Show earlier". */
    limit?: number;
    groupByDay?: boolean;
    loading?: boolean;
    emptyTitle?: string;
    now?: Date;
  }
  let { entries, label, order = 'newest', limit, groupByDay = true, loading = false, emptyTitle = 'Nothing has happened yet', now }: Props = $props();

  let expanded = $state(false);
  const icons: Record<TimelineKind, IconName> = { transition: 'arrow-right', observation: 'eye', attempt: 'refresh', apply: 'check', note: 'pencil' };
  const kindLabels: Record<TimelineKind, string> = { transition: 'State change', observation: 'Observed', attempt: 'Attempt', apply: 'Applied', note: 'Note' };

  const sorted = $derived(
    entries.map((e) => ({ ...e, d: toDate(e.at) })).sort((a, b) => (order === 'newest' ? b.d.getTime() - a.d.getTime() : a.d.getTime() - b.d.getTime())),
  );
  const shown = $derived(limit && !expanded ? sorted.slice(0, limit) : sorted);
  const hidden = $derived(sorted.length - shown.length);
  const today = $derived(startOfDay(now ?? new Date()));
  const dayTitle = (d: Date) => (sameDay(d, today) ? 'Today' : sameDay(d, addDays(today, -1)) ? 'Yesterday' : formatDayLong(d));
  const groups = $derived.by(() => {
    const out: { key: string; title: string; items: typeof shown }[] = [];
    for (const e of shown) {
      const key = groupByDay ? isoDay(e.d) : 'all';
      const last = out[out.length - 1];
      if (last && last.key === key) last.items.push(e);
      else out.push({ key, title: dayTitle(e.d), items: [e] });
    }
    return out;
  });
</script>

<section class="tl" aria-label={label} aria-busy={loading}>
  {#if loading}
    <div class="skel">{#each Array(4) as _, i (i)}<Skeleton lines={2} />{/each}</div>
  {:else if !entries.length}
    <EmptyState title={emptyTitle} icon="clock" size="sm" />
  {:else}
    {#each groups as g (g.key)}
      <div class="group">
        {#if groupByDay}<h3 class="day">{g.title}</h3>{/if}
        <ol class="entries">
          {#each g.items as e (e.id)}
            <li class="entry kind-{e.kind}">
              <span class="marker" aria-hidden="true"><Icon name={icons[e.kind]} size="sm" /></span>
              <div class="body">
                <div class="head">
                  <span class="visually-hidden">{kindLabels[e.kind]}:</span>
                  <span class="title">{e.title}</span>
                  <span class="when"><time class="clock num" datetime={e.d.toISOString()}>{formatTimeOfDay(e.d)}</time> · <Timestamp value={e.d} {now} /></span>
                </div>
                {#if e.from || e.to || e.status}
                  <div class="states">
                    {#if e.from}<StatusBadge family={e.from.family} value={e.from.value} size="sm" />{/if}
                    {#if e.from && e.to}<span class="arrow"><Icon name="arrow-right" size="sm" label="to" /></span>{/if}
                    {#if e.to}<StatusBadge family={e.to.family} value={e.to.value} size="sm" />{/if}
                    {#if e.status}<StatusBadge family={e.status.family} value={e.status.value} size="sm" />{/if}
                  </div>
                {/if}
                {#if e.detail || e.actor || e.href}
                  <p class="detail">
                    {#if e.detail}<span>{e.detail}</span>{/if}
                    {#if e.actor}<span class="actor">{e.actor}</span>{/if}
                    {#if e.href}<Link href={e.href}>{e.hrefLabel ?? 'Why?'}</Link>{/if}
                  </p>
                {/if}
                {#if e.fields?.length}
                  <details class="fields">
                    <summary>Details</summary>
                    <PropertyList density="compact" items={e.fields.map((f, i) => ({ key: String(i), label: f.label, value: f.value, mono: f.mono }))} />
                  </details>
                {/if}
              </div>
            </li>
          {/each}
        </ol>
      </div>
    {/each}
    {#if hidden > 0}
      <div class="earlier"><Button size="sm" variant="ghost" icon="chevron-down" onclick={() => (expanded = true)}>Show {formatCount(hidden)} {order === 'newest' ? 'earlier' : 'later'}</Button></div>
    {:else if limit && expanded && sorted.length > limit}
      <div class="earlier"><Button size="sm" variant="ghost" icon="chevron-up" onclick={() => (expanded = false)}>Show fewer</Button></div>
    {/if}
  {/if}
</section>

<style>
  .tl { display: grid; gap: var(--space-5); min-width: 0; }
  .skel { display: grid; gap: var(--space-4); }
  .group { display: grid; gap: var(--space-2); }
  .day { font-size: var(--text-sm); font-weight: var(--weight-semibold); color: var(--ink-muted); }
  .entries { list-style: none; margin: 0; padding: 0; display: grid; }
  .entry { display: grid; grid-template-columns: 1.75rem minmax(0, 1fr); gap: var(--space-3); position: relative; padding-bottom: var(--space-4); }
  /* The axis: a hairline through the markers, ending at the last entry. */
  .entry::after { content: ''; position: absolute; left: calc(0.875rem - 0.5px); top: 1.75rem; bottom: 0; width: var(--border-w); background: var(--line); }
  .entry:last-child { padding-bottom: 0; }
  .entry:last-child::after { display: none; }
  .marker {
    display: grid; place-items: center; width: 1.75rem; height: 1.75rem; border-radius: var(--radius-round);
    background: var(--surface-sunken); color: var(--ink-muted); border: var(--border-w) solid var(--line); position: relative; z-index: 1;
  }
  .kind-transition .marker { background: var(--surface-raised); color: var(--ink); border-color: var(--line-strong); }
  .body { display: grid; gap: var(--space-1-5); min-width: 0; padding-top: var(--space-0-5); }
  .head { display: flex; flex-wrap: wrap; align-items: baseline; gap: var(--space-1) var(--space-3); }
  .title { font-size: var(--text-md); font-weight: var(--weight-medium); color: var(--ink); }
  .when { font-size: var(--text-xs); color: var(--ink-muted); }
  .states { display: flex; flex-wrap: wrap; align-items: center; gap: var(--space-1-5); }
  .arrow { display: inline-flex; color: var(--ink-subtle); }
  .detail { display: flex; flex-wrap: wrap; gap: var(--space-1) var(--space-3); font-size: var(--text-sm); color: var(--ink-muted); max-width: var(--measure); }
  .actor { color: var(--ink-subtle); }
  .fields summary { font-size: var(--text-sm); color: var(--accent-ink); cursor: pointer; width: fit-content; }
  .fields[open] summary { margin-bottom: var(--space-1-5); }
  .earlier { padding-left: var(--space-10); }
  @media (width < 48rem), (pointer: coarse) {
    .fields summary { min-height: var(--target-min); display: flex; align-items: center; }
  }
</style>
