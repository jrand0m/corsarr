<!--
  StatusDot: a small tone dot with an always-present text label (visible or visually hidden)
  and optional count; the building block of the shell's global indicators.
  `pulse` marks a live connection; it is static under reduced motion.
-->
<script lang="ts">
  import type { Tone } from '../../../tokens';

  interface Props {
    tone: Tone;
    label: string;
    count?: number;
    showLabel?: boolean;
    pulse?: boolean;
  }
  let { tone, label, count, showLabel = false, pulse = false }: Props = $props();
</script>

<span class={['sd', tone]}>
  <span class="dot" class:pulse aria-hidden="true"></span>
  {#if count !== undefined}<span class="count" aria-hidden={showLabel ? undefined : 'true'}>{count}</span>{/if}
  {#if showLabel}<span class="label">{label}</span>{:else}<span class="visually-hidden">{label}{count !== undefined ? `: ${count}` : ''}</span>{/if}
</span>

<style>
  .sd { --solid: var(--tone-neutral-solid); display: inline-flex; align-items: center; gap: var(--space-1-5); font-size: var(--text-sm); }
  .ok { --solid: var(--tone-ok-solid); }
  .warn { --solid: var(--tone-warn-solid); }
  .error { --solid: var(--tone-error-solid); }
  .info { --solid: var(--tone-info-solid); }
  .managed { --solid: var(--tone-managed-solid); }
  .dot { position: relative; width: 0.5rem; height: 0.5rem; border-radius: var(--radius-round); background: var(--solid); }
  .warn .dot { border-radius: 1px; transform: rotate(45deg) scale(0.9); }
  .error .dot { border-radius: 1px; }
  .pulse::after {
    content: '';
    position: absolute;
    inset: 0;
    border-radius: inherit;
    background: var(--solid);
    animation: ping 2s var(--ease-out) infinite;
  }
  @keyframes ping { from { opacity: 0.6; transform: scale(1); } to { opacity: 0; transform: scale(2.6); } }
  .count { font-weight: var(--weight-semibold); font-variant-numeric: tabular-nums; }
</style>
