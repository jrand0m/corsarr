<!--
  SourceProgressStrip: a search or migration fanned out over many indexers or sources.
  A segmented strip shows each source's state at a glance (pattern + tone, never colour
  alone); the list below names each one with its result count, time, skip reason or error.
  Progress counts answered sources; the summary is announced once, when all have answered.
-->
<script lang="ts" module>
  export interface SourceProgress {
    id: string;
    name: string;
    state: 'pending' | 'running' | 'ok' | 'skipped' | 'failed';
    count?: number;
    /** Milliseconds to answer. */
    ms?: number;
    /** Skip reason or error message. */
    reason?: string;
  }
</script>

<script lang="ts">
  import StatusBadge from '../../atoms/StatusBadge/StatusBadge.svelte';
  import { formatCount } from '../../../format/quantity';

  interface Props {
    sources: SourceProgress[];
    label?: string;
    /** Noun for counts: "results", "objects". */
    unit?: string;
    /** Singular of `unit`; default drops a trailing "s". */
    unitOne?: string;
    /** Show only the strip and summary (collapsed list). */
    compact?: boolean;
  }
  let { sources, label = 'Sources', unit = 'results', unitOne, compact = false }: Props = $props();
  const noun = (n: number) => (n === 1 ? (unitOne ?? unit.replace(/s$/, '')) : unit);

  const finished = $derived(sources.filter((s) => s.state !== 'pending' && s.state !== 'running').length);
  const total = $derived(sources.reduce((a, s) => a + (s.count ?? 0), 0));
  const failed = $derived(sources.filter((s) => s.state === 'failed').length);
  const done = $derived(finished === sources.length);
  const ms = (v: number) => (v < 1000 ? `${Math.round(v)} ms` : `${(v / 1000).toFixed(1)} s`);
</script>

<section class="sps" aria-label={label}>
  <div class="head">
    <span class="sum num">{formatCount(finished, sources.length)} answered · {formatCount(total)} {noun(total)}{#if failed}{` · ${failed} failed`}{/if}</span>
    <span class="visually-hidden" role="status">{done ? `All ${sources.length} ${label.toLowerCase()} answered: ${formatCount(total)} ${noun(total)}${failed ? `, ${failed} failed` : ''}` : ''}</span>
  </div>
  <div class="strip" aria-hidden="true">
    {#each sources as s (s.id)}<span class={['seg', s.state]}></span>{/each}
  </div>
  {#if !compact}
    <ul>
      {#each sources as s (s.id)}
        <li class={s.state}>
          <span class="name">{s.name}</span>
          <StatusBadge family="source" value={s.state} size="sm" appearance="plain" />
          <span class="facts num">
            {#if s.state === 'ok'}{formatCount(s.count ?? 0)} {noun(s.count ?? 0)}{#if s.ms !== undefined}{` · ${ms(s.ms)}`}{/if}{/if}
            {#if s.reason}<span class="reason">{s.reason}</span>{/if}
          </span>
        </li>
      {/each}
    </ul>
  {/if}
</section>

<style>
  .sps { display: grid; gap: var(--space-2); min-width: 0; }
  .sum { font-size: var(--text-sm); color: var(--ink-muted); }
  .strip { display: flex; gap: var(--space-0-5); height: 0.625rem; }
  .seg { flex: 1; min-width: var(--space-1); border-radius: var(--radius-xs); background: var(--surface-sunken); border: var(--border-w) solid var(--line-strong); }
  .seg.ok { background: var(--tone-ok-solid); border-color: var(--tone-ok-solid); }
  .seg.failed { background: repeating-linear-gradient(135deg, var(--tone-error-solid) 0 2px, transparent 2px 4px); border-color: var(--tone-error-solid); }
  .seg.skipped { border-style: dashed; border-color: var(--tone-neutral-solid); background: none; }
  .seg.running { background: repeating-linear-gradient(45deg, var(--tone-info-solid) 0 2px, transparent 2px 4px); border-color: var(--tone-info-solid); }
  ul { display: grid; padding: 0; list-style: none; }
  li {
    display: grid;
    grid-template-columns: minmax(6rem, 12rem) 7rem minmax(0, 1fr);
    align-items: center;
    gap: var(--space-0-5) var(--space-3);
    min-height: var(--row-h-compact);
    padding-block: var(--space-1);
    border-bottom: var(--border-w) solid var(--line);
    font-size: var(--text-sm);
  }
  li:last-child { border-bottom: 0; }
  .name { font-weight: var(--weight-medium); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .facts { display: flex; flex-wrap: wrap; gap: var(--space-1) var(--space-2); color: var(--ink-muted); }
  .failed .reason { color: var(--tone-error-ink); }
  @media (width < 48rem) {
    li { grid-template-columns: minmax(0, 1fr) auto; }
    .facts { grid-column: 1 / -1; }
  }
</style>
