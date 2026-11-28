<!--
  SwarmHealth: a torrent's seeders against its leechers. Both counts as text, a bar with the
  seeders' share of the swarm, and a health word, so the swarm reads without colour. Leechers
  the indexer did not report say so instead of showing 0; an unreported swarm says that.
-->
<script lang="ts" module>
  export type Health = 'unknown' | 'none' | 'few' | 'some' | 'good';
  export const healthLabel: Record<Health, string> = {
    unknown: 'Swarm not reported', none: 'No seeders', few: 'Few seeders', some: 'Some seeders', good: 'Well seeded',
  };
</script>

<script lang="ts">
  import { formatCount } from '../../../format/quantity';

  interface Props {
    seeders: number;
    /** Undefined when the indexer did not report leechers. */
    leechers?: number;
    reported: boolean;
    health: Health;
  }
  let { seeders, leechers, reported, health }: Props = $props();

  const total = $derived(leechers === undefined ? 0 : seeders + leechers);
  const share = $derived(total > 0 ? seeders / total : 0);
  const ratio = $derived(leechers === undefined || total === 0 ? undefined : ratioText(seeders, leechers));

  function per(a: number, b: number): string {
    const r = a / b;
    return r >= 10 ? r.toFixed(0) : r.toFixed(1).replace(/\.0$/, '');
  }
  function ratioText(s: number, l: number): string {
    if (l === 0) return 'no leechers';
    if (s === 0) return 'no seeders';
    return s >= l ? `${per(s, l)} seeders per leecher` : `${per(l, s)} leechers per seeder`;
  }
</script>

<span class={['swarm', health]}>
  {#if !reported}
    <span class="word">{healthLabel.unknown}</span>
  {:else}
    <span class="word">{healthLabel[health]}</span>
    <span class="counts num">{formatCount(seeders)} {seeders === 1 ? 'seeder' : 'seeders'} · {leechers === undefined ? 'leechers not reported' : `${formatCount(leechers)} ${leechers === 1 ? 'leecher' : 'leechers'}`}</span>
    {#if ratio}
      <span class="bar" role="img" aria-label="Seeders {Math.round(share * 100)}% of the swarm, {ratio}">
        <span class="seed" style:inline-size="{share * 100}%"></span>
      </span>
    {/if}
  {/if}
</span>

<style>
  .swarm { --solid: var(--tone-neutral-solid); display: inline-flex; flex-wrap: wrap; align-items: center; gap: var(--space-0-5) var(--space-2); font-size: var(--text-sm); color: var(--ink-muted); }
  .good { --solid: var(--tone-ok-solid); }
  .some { --solid: var(--tone-info-solid); }
  .few { --solid: var(--tone-warn-solid); }
  .none { --solid: var(--tone-error-solid); }
  .word { font-weight: var(--weight-medium); color: var(--ink); }
  .none .word { color: var(--tone-error-ink); }
  .few .word { color: var(--tone-warn-ink); }
  .counts { font-variant-numeric: tabular-nums; }
  .bar { display: inline-flex; inline-size: 4rem; block-size: 0.5rem; overflow: hidden; background: var(--surface-sunken); box-shadow: inset 0 0 0 1px var(--line); border-radius: var(--radius-xs); }
  .seed { background: var(--solid); }
</style>
