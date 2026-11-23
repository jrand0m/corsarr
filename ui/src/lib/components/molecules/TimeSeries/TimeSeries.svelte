<!--
  TimeSeries: small time-bucketed stats for one source: queries or grabs per hour over
  24 h / 7 d / 30 d, with failures stacked on top, or a self-test strip of pass/fail runs
  (`mode="strip"`). One series of successes plus failures, so the legend names both; marks are
  thin columns with a 2px surface gap, rounded at the data end only; failures are hatched
  (state is never colour alone). Hover, focus and arrow keys show the bucket under a crosshair;
  a table view carries every value. Heavy for first load, so organisms load it lazily
  (`loadTimeSeries` in the molecules barrel). No chart library.
-->
<script lang="ts" module>
  export interface TimeBucket {
    at: Date | string | number;
    /** Successes (answered queries, passed runs). */
    value: number;
    /** Failures in the same bucket. */
    failed?: number;
  }
  export type TimeRange = '24h' | '7d' | '30d';
</script>

<script lang="ts">
  import SegmentedControl from '../SegmentedControl/SegmentedControl.svelte';
  import Button from '../../atoms/Button/Button.svelte';
  import Skeleton from '../../atoms/Skeleton/Skeleton.svelte';
  import { formatCount } from '../../../format/quantity';
  import { formatAxisTime, formatShortStamp, toDate } from '../../../format/time';

  interface Props {
    buckets: TimeBucket[];
    /** Chart title: "Queries per hour". */
    title: string;
    /** Noun for values: "queries", "runs". */
    unit?: string;
    /** Noun for failures: "failed", "timed out". */
    failedLabel?: string;
    mode?: 'bars' | 'strip';
    range?: TimeRange;
    ranges?: TimeRange[];
    onrangechange?: (range: TimeRange) => void;
    loading?: boolean;
    height?: number;
  }
  let {
    buckets, title, unit = 'queries', failedLabel = 'failed', mode = 'bars', range = $bindable('24h'),
    ranges = ['24h', '7d', '30d'], onrangechange, loading = false, height = 120,
  }: Props = $props();

  const uid = $props.id();
  let width = $state(0);
  let active = $state(-1);
  let showTable = $state(false);
  // Announce the bucket only while the chart has keyboard focus, never on pointer hover.
  let focused = $state(false);

  const rangeLabels: Record<TimeRange, string> = { '24h': '24 h', '7d': '7 d', '30d': '30 d' };
  const pad = { top: 8, bottom: 20, left: 32, right: 4 };
  const rows = $derived(buckets.map((b) => ({ at: toDate(b.at), ok: b.value, failed: b.failed ?? 0 })));
  const span = $derived(rows.length > 1 ? (rows[rows.length - 1].at.getTime() - rows[0].at.getTime()) / 1000 : 0);
  const max = $derived(niceMax(Math.max(1, ...rows.map((r) => r.ok + r.failed))));
  const totals = $derived(rows.reduce((a, r) => ({ ok: a.ok + r.ok, failed: a.failed + r.failed }), { ok: 0, failed: 0 }));
  const plotW = $derived(Math.max(0, width - pad.left - pad.right));
  const plotH = $derived(height - pad.top - pad.bottom);
  const slot = $derived(rows.length ? plotW / rows.length : 0);
  const barW = $derived(Math.max(1, Math.min(24, slot - 2)));
  const hasFailures = $derived(totals.failed > 0);
  const summary = $derived(
    `${title}, last ${rangeLabels[range]}: ${formatCount(totals.ok)} ${unit}${hasFailures ? `, ${formatCount(totals.failed)} ${failedLabel}` : ''}.`,
  );
  const ticks = $derived(rows.length ? [0, Math.floor((rows.length - 1) / 2), rows.length - 1].filter((v, i, a) => a.indexOf(v) === i) : []);

  function niceMax(v: number) {
    const p = 10 ** Math.floor(Math.log10(v));
    for (const m of [1, 2, 2.5, 5, 10]) if (v <= m * p) return m * p;
    return 10 * p;
  }
  const y = (v: number) => pad.top + plotH - (v / max) * plotH;
  /** Column path with a 4px (capped) rounded data end, square at the baseline. */
  function column(x: number, top: number, bottom: number, w: number) {
    const h = bottom - top;
    if (h <= 0) return '';
    const r = Math.min(4, w / 2, h);
    return `M${x},${bottom}V${top + r}Q${x},${top} ${x + r},${top}H${x + w - r}Q${x + w},${top} ${x + w},${top + r}V${bottom}Z`;
  }
  function onkeydown(e: KeyboardEvent) {
    const n = rows.length;
    const next = { ArrowRight: active + 1, ArrowLeft: active < 0 ? n - 1 : active - 1, Home: 0, End: n - 1 }[e.key];
    if (next === undefined || !n) return;
    e.preventDefault();
    active = Math.max(0, Math.min(n - 1, next));
  }
  function onpointermove(e: PointerEvent) {
    const r = (e.currentTarget as SVGElement).getBoundingClientRect();
    const i = Math.floor((e.clientX - r.left - pad.left) / slot);
    active = i >= 0 && i < rows.length ? i : -1;
  }
  const tip = $derived(active >= 0 && rows[active] ? rows[active] : undefined);
</script>

<figure class="ts">
  <figcaption class="head">
    <span class="title">{title}</span>
    <span class="total num">{formatCount(totals.ok)} {mode === 'strip' ? 'passed' : unit}{#if hasFailures}{' · '}<span class="fail">{formatCount(totals.failed)} {failedLabel}</span>{/if}</span>
    {#if ranges.length > 1}
      <SegmentedControl size="sm" label="Time range for {title}" bind:value={range} options={ranges.map((r) => ({ value: r, label: rangeLabels[r] }))} onchange={(v) => onrangechange?.(v as TimeRange)} />
    {/if}
  </figcaption>

  {#if loading}
    <Skeleton shape="block" height="{height}px" />
  {:else if rows.length === 0}
    <p class="empty" style:min-height="{height}px">No data for the last {rangeLabels[range]} yet.</p>
  {:else}
    {#if hasFailures || mode === 'strip'}
      <ul class="legend" aria-hidden="true">
        <li><span class="key ok"></span>{mode === 'strip' ? 'Passed' : unit[0].toUpperCase() + unit.slice(1)}</li>
        <li><span class="key bad"></span>{mode === 'strip' ? 'Failed' : failedLabel[0].toUpperCase() + failedLabel.slice(1)}</li>
      </ul>
    {/if}
    <div class="plot" bind:clientWidth={width}>
      {#if width > 0}
        <!-- A focusable image explored with arrow keys; every value is also in the table view. -->
        <!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions -->
        <svg
          width={width} height={mode === 'strip' ? 28 : height} role="img" aria-label={summary} tabindex="0"
          aria-describedby="{uid}-tip" {onkeydown} {onpointermove} onpointerleave={() => (active = -1)} onfocus={() => (focused = true)} onblur={() => { active = -1; focused = false; }}
        >
          <defs>
            <pattern id="{uid}-hatch" width="4" height="4" patternUnits="userSpaceOnUse" patternTransform="rotate(45)">
              <rect width="4" height="4" class="hatch-bg" /><rect width="2" height="4" class="hatch-fg" />
            </pattern>
          </defs>
          {#if mode === 'strip'}
            {#each rows as r, i (i)}
              {@const x = pad.left + i * slot + (slot - barW) / 2}
              <rect x={x} y="4" width={barW} height="20" rx="2" class={r.failed > 0 ? 'bad' : r.ok > 0 ? 'ok' : 'none'} fill={r.failed > 0 ? `url(#${uid}-hatch)` : undefined} />
            {/each}
          {:else}
            <line class="grid" x1={pad.left} x2={width - pad.right} y1={y(max)} y2={y(max)} />
            <line class="base" x1={pad.left} x2={width - pad.right} y1={y(0)} y2={y(0)} />
            <text class="axis" x={pad.left - 6} y={y(max) + 4} text-anchor="end">{formatCount(max)}</text>
            <text class="axis" x={pad.left - 6} y={y(0) + 4} text-anchor="end">0</text>
            {#each rows as r, i (i)}
              {@const x = pad.left + i * slot + (slot - barW) / 2}
              {@const okTop = y(r.ok)}
              {@const failTop = y(r.ok + r.failed)}
              <path class="ok" d={column(x, okTop, y(0), barW)} />
              {#if r.failed}<path class="bad" fill="url(#{uid}-hatch)" d={column(x, failTop, r.ok ? okTop - 2 : y(0), barW)} />{/if}
            {/each}
            {#each ticks as t (t)}
              <text class="axis" x={pad.left + t * slot + slot / 2} y={height - 4} text-anchor={t === 0 ? 'start' : t === rows.length - 1 ? 'end' : 'middle'}>{formatAxisTime(rows[t].at, span)}</text>
            {/each}
          {/if}
          {#if tip}
            <line class="cross" x1={pad.left + active * slot + slot / 2} x2={pad.left + active * slot + slot / 2} y1={pad.top} y2={mode === 'strip' ? 28 : y(0)} />
          {/if}
        </svg>
      {/if}
      <p class="tip num" id="{uid}-tip" aria-live={focused ? 'polite' : 'off'} class:shown={!!tip}>
        {#if tip}<strong>{formatShortStamp(tip.at)}</strong> · {formatCount(tip.ok)} {mode === 'strip' ? 'passed' : unit}{#if tip.failed} · {formatCount(tip.failed)} {mode === 'strip' ? 'failed' : failedLabel}{/if}{:else}<span class="hint">Hover or use arrow keys for each {mode === 'strip' ? 'run' : 'bucket'}</span>{/if}
      </p>
    </div>
    <div class="foot">
      <Button size="sm" variant="ghost" icon="list" aria-expanded={showTable} aria-controls="{uid}-table" onclick={() => (showTable = !showTable)}>{showTable ? 'Hide table' : 'Show as table'}</Button>
    </div>
    <div class="table-wrap" id="{uid}-table" hidden={!showTable}>
      <table>
        <caption class="visually-hidden">{title}</caption>
        <thead><tr><th scope="col">Time</th><th scope="col">{mode === 'strip' ? 'Passed' : unit}</th><th scope="col">{mode === 'strip' ? 'Failed' : failedLabel}</th></tr></thead>
        <tbody>
          {#each rows as r, i (i)}<tr><td>{formatShortStamp(r.at)}</td><td>{formatCount(r.ok)}</td><td>{formatCount(r.failed)}</td></tr>{/each}
        </tbody>
      </table>
    </div>
  {/if}
</figure>

<style>
  .ts { display: grid; gap: var(--space-2); width: 100%; min-width: 0; margin: 0; }
  .head { display: flex; flex-wrap: wrap; align-items: center; gap: var(--space-1) var(--space-3); }
  .title { font-size: var(--text-md); font-weight: var(--weight-semibold); }
  .total { font-size: var(--text-sm); color: var(--ink-muted); }
  .fail { color: var(--tone-error-ink); }
  .head :global(.seg) { margin-left: auto; }
  .legend { display: flex; gap: var(--space-3); margin: 0; padding: 0; list-style: none; font-size: var(--text-xs); color: var(--ink-muted); }
  .legend li { display: inline-flex; align-items: center; gap: var(--space-1); }
  .key { width: 0.625rem; height: 0.625rem; border-radius: var(--radius-xs); }
  .key.ok { background: var(--tone-ok-solid); }
  .key.bad { background: repeating-linear-gradient(135deg, var(--tone-error-solid) 0 2px, transparent 2px 4px); outline: var(--border-w) solid var(--tone-error-solid); }
  .plot { position: relative; min-width: 0; }
  svg { display: block; overflow: visible; }
  svg:focus-visible { outline-offset: var(--space-1); }
  .ok { fill: var(--tone-ok-solid); }
  .bad { stroke: var(--tone-error-solid); stroke-width: 1; }
  .none { fill: var(--surface-sunken); stroke: var(--line-strong); stroke-dasharray: 2 2; }
  .hatch-bg { fill: var(--surface-raised); }
  .hatch-fg { fill: var(--tone-error-solid); }
  .grid, .base { stroke: var(--line); stroke-width: 1; }
  .base { stroke: var(--line-strong); }
  .cross { stroke: var(--ink-subtle); stroke-width: 1; }
  .axis { font-size: var(--text-2xs); fill: var(--ink-muted); font-variant-numeric: tabular-nums; }
  .tip { min-height: 1.35em; font-size: var(--text-xs); color: var(--ink-muted); max-width: none; }
  .tip strong { color: var(--ink); font-weight: var(--weight-medium); }
  .hint { color: var(--ink-subtle); }
  .empty { display: grid; place-items: center; font-size: var(--text-sm); color: var(--ink-subtle); background: var(--surface-sunken); border-radius: var(--radius-md); max-width: none; }
  .foot { display: flex; justify-content: flex-end; }
  .table-wrap { max-height: 16rem; overflow: auto; border: var(--border-w) solid var(--line); border-radius: var(--radius-md); }
  table { width: 100%; font-size: var(--text-sm); }
  th, td { padding: var(--space-1) var(--space-2); text-align: right; border-bottom: var(--border-w) solid var(--line); }
  th:first-child, td:first-child { text-align: left; }
  th { position: sticky; top: 0; background: var(--surface-sunken); font-weight: var(--weight-medium); color: var(--ink-muted); text-transform: capitalize; }
</style>
