<!--
  Progress: how far a task has come. Pass `value` (0–1) for determinate, nothing for
  indeterminate, or `done` + `total` for countable work ("12 of 13"). `label` is the
  accessible name; `showValue` prints the value beside the bar. Tone follows the task state
  (info while running, warn when stalled, error when failed, ok when complete).
-->
<script lang="ts">
  import type { Tone } from '../../../tokens';
  import { formatPercent, formatCount } from '../../../format/quantity';

  interface Props {
    label: string;
    value?: number;
    done?: number;
    total?: number;
    tone?: Tone;
    size?: 'sm' | 'md';
    showValue?: boolean;
  }
  let { label, value, done, total, tone = 'info', size = 'md', showValue = false }: Props = $props();

  const fraction = $derived(
    total !== undefined && done !== undefined && total > 0 ? done / total : value !== undefined ? Math.min(1, Math.max(0, value)) : undefined,
  );
  const text = $derived(
    total !== undefined && done !== undefined ? formatCount(done, total) : fraction !== undefined ? formatPercent(fraction) : undefined,
  );
</script>

<span class={['progress', size, tone]}>
  <span
    class="track"
    role="progressbar"
    aria-label={label}
    aria-valuemin={fraction === undefined ? undefined : 0}
    aria-valuemax={fraction === undefined ? undefined : 100}
    aria-valuenow={fraction === undefined ? undefined : Math.round(fraction * 100)}
    aria-valuetext={text}
  >
    {#if fraction === undefined}
      <span class="fill indeterminate"></span>
    {:else}
      <span class="fill" style:inline-size="{fraction * 100}%"></span>
    {/if}
  </span>
  {#if showValue && text}<span class="value" aria-hidden="true">{text}</span>{/if}
</span>

<style>
  .progress { --solid: var(--tone-info-solid); display: flex; align-items: center; gap: var(--space-2); min-width: 4rem; }
  .ok { --solid: var(--tone-ok-solid); }
  .warn { --solid: var(--tone-warn-solid); }
  .error { --solid: var(--tone-error-solid); }
  .neutral { --solid: var(--tone-neutral-solid); }
  .managed { --solid: var(--tone-managed-solid); }
  .track {
    position: relative;
    flex: 1;
    height: 0.375rem;
    overflow: hidden;
    background: var(--surface-sunken);
    box-shadow: inset 0 0 0 1px var(--line);
    border-radius: var(--radius-xs);
  }
  .sm .track { height: 0.25rem; }
  .fill {
    display: block;
    height: 100%;
    background: var(--solid);
    border-radius: var(--radius-xs);
    transition: inline-size var(--dur-slow) var(--ease-out);
  }
  .indeterminate {
    width: 35%;
    animation: slide 1.4s var(--ease-in-out) infinite;
  }
  @keyframes slide { from { transform: translateX(-100%); } to { transform: translateX(290%); } }
  @media (prefers-reduced-motion: reduce) {
    .indeterminate {
      width: 100%;
      animation: none;
      background: repeating-linear-gradient(-45deg, var(--solid) 0 4px, transparent 4px 8px);
    }
  }
  .value { font-size: var(--text-sm); color: var(--ink-muted); font-variant-numeric: tabular-nums; white-space: nowrap; min-width: 4ch; text-align: right; }
</style>
