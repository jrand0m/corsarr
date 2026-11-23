<!--
  Quantity: a measured value with its unit: size, speed, duration, GB per hour, count
  (optionally "of" a limit) or percent. The number is tabular; the unit is set quieter.
  Raw value goes into <data value> for copy and tests.
-->
<script lang="ts">
  import { formatBytes, formatSpeed, formatDuration, formatGbPerHour, formatCount, formatPercent } from '../../../format/quantity';

  export type QuantityKind = 'bytes' | 'speed' | 'duration' | 'gbph' | 'count' | 'percent';
  interface Props {
    kind: QuantityKind;
    value: number;
    /** count: the limit ("3 of 5"). */
    limit?: number;
    /** gbph: runtime in seconds. */
    runtime?: number;
    /** Over the limit or out of budget: set in warn ink. */
    over?: boolean;
  }
  let { kind, value, limit, runtime = 0, over = false }: Props = $props();

  const text = $derived.by(() => {
    switch (kind) {
      case 'bytes': return formatBytes(value);
      case 'speed': return formatSpeed(value);
      case 'duration': return formatDuration(value);
      case 'gbph': return formatGbPerHour(value, runtime);
      case 'count': return formatCount(value, limit);
      case 'percent': return formatPercent(value);
    }
  });
  /** Split "4.7 GB" → ["4.7", " GB"], "12 of 13" stays whole, "3h 20m" stays whole. */
  const parts = $derived.by(() => {
    const m = /^(−?[\d.,]+)(\s?(?:[KMGTP]?B(?:\/s)?|GB\/h|%))$/.exec(text);
    return m ? [m[1], m[2]] : [text, ''];
  });
</script>

<data class="qty" class:over value={String(value)}>{parts[0]}{#if parts[1]}<span class="unit">{parts[1]}</span>{/if}</data>

<style>
  .qty { white-space: nowrap; font-variant-numeric: tabular-nums; }
  .unit { color: var(--ink-muted); font-size: 0.92em; }
  .over { color: var(--tone-warn-ink); }
  .over .unit { color: inherit; }
</style>
