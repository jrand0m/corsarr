<!--
  Meter: a level within a known range: free space, completion, quota used. Unlike
  Progress it does not move towards done. `better` says which end is good, and `warnAt` /
  `errorAt` (fractions of the range) pick the tone; the value text always names the level.
-->
<script lang="ts">
  import type { Tone } from '../../../tokens';

  interface Props {
    label: string;
    value: number;
    min?: number;
    max?: number;
    /** Which end of the range is good. "low" for quota used, "high" for free space. */
    better?: 'high' | 'low';
    warnAt?: number;
    errorAt?: number;
    /** Human value, e.g. "412 GB free of 2 TB". */
    valueText?: string;
    showValue?: boolean;
  }
  let { label, value, min = 0, max = 1, better = 'low', warnAt, errorAt, valueText, showValue = false }: Props = $props();

  const fraction = $derived(max > min ? Math.min(1, Math.max(0, (value - min) / (max - min))) : 0);
  const tone = $derived.by((): Tone => {
    const level = better === 'low' ? fraction : 1 - fraction;
    if (errorAt !== undefined && level >= (better === 'low' ? errorAt : 1 - errorAt)) return 'error';
    if (warnAt !== undefined && level >= (better === 'low' ? warnAt : 1 - warnAt)) return 'warn';
    return 'neutral';
  });
  const text = $derived(valueText ?? `${Math.round(fraction * 100)}%`);
</script>

<span class={['meter', tone]}>
  <span
    class="track"
    role="meter"
    aria-label={label}
    aria-valuemin={min}
    aria-valuemax={max}
    aria-valuenow={value}
    aria-valuetext={text}
  ><span class="fill" style:inline-size="{fraction * 100}%"></span></span>
  {#if showValue}<span class="value" aria-hidden="true">{text}</span>{/if}
</span>

<style>
  .meter { --solid: var(--tone-neutral-solid); display: flex; align-items: center; gap: var(--space-2); min-width: 4rem; }
  .warn { --solid: var(--tone-warn-solid); }
  .error { --solid: var(--tone-error-solid); }
  .track {
    flex: 1;
    display: flex;
    height: 0.5rem;
    overflow: hidden;
    background: var(--surface-sunken);
    box-shadow: inset 0 0 0 1px var(--line);
    border-radius: var(--radius-xs);
  }
  .fill { background: var(--solid); border-radius: var(--radius-xs) 0 0 var(--radius-xs); }
  .value { font-size: var(--text-sm); color: var(--ink-muted); font-variant-numeric: tabular-nums; white-space: nowrap; }
  .warn .value { color: var(--tone-warn-ink); }
  .error .value { color: var(--tone-error-ink); }
</style>
