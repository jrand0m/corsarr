<!--
  StatTile: one figure the owner checks at a glance (wanted, downloading, free space),
  with its label and either a trend against the previous period or a link to the list behind
  it. With `href` the whole tile is one link. `tone` is only for a state ("3 failing" is
  error); healthy figures stay ink. Loading shows a skeleton of the same size.
-->
<script lang="ts">
  import Icon from '../../atoms/Icon/Icon.svelte';
  import Quantity, { type QuantityKind } from '../../atoms/Quantity/Quantity.svelte';
  import Skeleton from '../../atoms/Skeleton/Skeleton.svelte';
  import { formatCount } from '../../../format/quantity';
  import type { Tone } from '../../../tokens';

  interface Trend {
    /** Signed change against the previous period, in the value's own unit. */
    delta: number;
    /** Which direction is good. */
    better: 'up' | 'down';
    /** "vs last week". */
    period: string;
    /** Display of the change ("+4", "−1.2 GB"); defaults to the signed count. */
    text?: string;
  }
  interface Props {
    label: string;
    value: number | null;
    kind?: QuantityKind;
    limit?: number;
    trend?: Trend;
    href?: string;
    /** Link text after the figure: "Open queue". Shown when there is no trend. */
    linkLabel?: string;
    tone?: Exclude<Tone, 'managed'>;
    loading?: boolean;
  }
  let { label, value, kind = 'count', limit, trend, href, linkLabel, tone, loading = false }: Props = $props();

  const dir = $derived(trend ? (trend.delta > 0 ? 'up' : trend.delta < 0 ? 'down' : 'flat') : undefined);
  const good = $derived(trend && dir !== 'flat' ? dir === trend.better : undefined);
  const deltaText = $derived(
    trend ? (trend.text ?? (trend.delta > 0 ? `+${formatCount(trend.delta)}` : trend.delta < 0 ? `−${formatCount(-trend.delta)}` : 'No change')) : '',
  );
</script>

{#snippet body()}
  <span class="label">{label}</span>
  {#if loading}
    <span class="value"><Skeleton shape="block" width="4rem" height="1.75rem" /></span>
    <span class="foot"><Skeleton width="6rem" /></span>
  {:else}
    <span class={['value', tone]}>
      {#if value === null}<span class="none" aria-label="Not known yet">—</span>{:else}<Quantity {kind} {value} {limit} />{/if}
    </span>
    {#if trend}
      <span class={['foot', 'trend', good === undefined ? 'flat' : good ? 'good' : 'bad']}>
        {#if dir !== 'flat'}<Icon name={dir === 'up' ? 'arrow-up' : 'arrow-down'} size="sm" />{/if}
        <span><span class="num">{deltaText}</span> {trend.period}</span>
      </span>
    {:else if href && linkLabel}
      <span class="foot link">{linkLabel}<Icon name="arrow-right" size="sm" /></span>
    {/if}
  {/if}
{/snippet}

{#if href && !loading}
  <a class="tile" {href}>{@render body()}</a>
{:else}
  <div class="tile" aria-busy={loading || undefined}>{@render body()}</div>
{/if}

<style>
  .tile {
    display: grid;
    align-content: start;
    gap: var(--space-1);
    min-width: 0;
    padding: var(--space-3) var(--space-4);
    color: var(--ink);
    text-decoration: none;
    background: var(--surface-raised);
    border: var(--border-w) solid var(--line);
    border-radius: var(--radius-lg);
  }
  a.tile { transition: border-color var(--dur-fast) var(--ease-out), background-color var(--dur-fast) var(--ease-out); }
  a.tile:hover { border-color: var(--line-strong); background: var(--surface-hover); }
  .label { font-size: var(--text-sm); color: var(--ink-muted); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .value { font-size: var(--text-xl); font-weight: var(--weight-semibold); line-height: var(--leading-tight); font-variant-numeric: tabular-nums; }
  .value.ok { color: var(--tone-ok-ink); }
  .value.warn { color: var(--tone-warn-ink); }
  .value.error { color: var(--tone-error-ink); }
  .value.info { color: var(--tone-info-ink); }
  .none { color: var(--ink-subtle); }
  .foot { display: inline-flex; align-items: center; gap: var(--space-1); font-size: var(--text-xs); color: var(--ink-muted); }
  .good { color: var(--tone-ok-ink); }
  .bad { color: var(--tone-error-ink); }
  .link { color: var(--accent-ink); }
  @media (width < 48rem), (pointer: coarse) {
    .tile { min-height: var(--target-min); }
  }
</style>
