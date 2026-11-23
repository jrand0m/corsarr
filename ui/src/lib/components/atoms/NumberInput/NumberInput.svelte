<!--
  NumberInput: a number with optional unit suffix ("MB", "min", "%") and − / + steppers.
  Arrow keys step natively; the value is clamped to [min, max] on blur. `value` is null when
  empty. Steppers are skipped by Tab (keyboard users have the arrows).
-->
<script lang="ts">
  import type { HTMLInputAttributes } from 'svelte/elements';
  import Icon from '../Icon/Icon.svelte';
  import type { Size } from '../../../tokens';

  interface Props extends Omit<HTMLInputAttributes, 'size' | 'value' | 'min' | 'max' | 'step' | 'type'> {
    value?: number | null;
    min?: number;
    max?: number;
    step?: number;
    unit?: string;
    size?: Size;
    invalid?: boolean;
    steppers?: boolean;
    onvaluechange?: (value: number | null) => void;
  }
  let {
    value = $bindable(null), min, max, step = 1, unit, size = 'md', invalid = false, steppers = true,
    disabled, readonly, onvaluechange, class: cls, ...rest
  }: Props = $props();

  const clamp = (n: number) => Math.min(max ?? Infinity, Math.max(min ?? -Infinity, n));
  const decimals = $derived((String(step).split('.')[1] ?? '').length);

  function set(n: number | null) {
    value = n === null ? null : Number(clamp(n).toFixed(decimals));
    onvaluechange?.(value);
  }
  function bump(dir: 1 | -1) {
    set((value ?? min ?? 0) + dir * step);
  }
  function oninput(e: Event) {
    const raw = (e.currentTarget as HTMLInputElement).value;
    value = raw === '' ? null : Number(raw);
  }
  function onblur() {
    if (value !== null && Number.isFinite(value)) set(value);
    else if (value !== null) set(null);
  }
</script>

<span class={['ni', size, cls]} class:locked={readonly} class:off={disabled} aria-invalid={invalid || undefined}>
  {#if steppers && !readonly}
    <button type="button" tabindex="-1" aria-label="Decrease" disabled={disabled || (min !== undefined && (value ?? 0) <= min)} onclick={() => bump(-1)}><Icon name="minus" size="sm" /></button>
  {/if}
  <input
    type="number"
    inputmode={decimals ? 'decimal' : 'numeric'}
    value={value ?? ''}
    {min} {max} {step} {disabled} {readonly}
    aria-invalid={invalid || undefined}
    {oninput} {onblur}
    {...rest}
  />
  {#if unit}<span class="unit">{unit}</span>{/if}
  {#if steppers && !readonly}
    <button type="button" tabindex="-1" aria-label="Increase" disabled={disabled || (max !== undefined && (value ?? 0) >= max)} onclick={() => bump(1)}><Icon name="plus" size="sm" /></button>
  {/if}
</span>

<style>
  .ni {
    display: inline-flex;
    align-items: stretch;
    min-height: var(--control-h-md);
    background: var(--surface-raised);
    border: var(--border-w) solid var(--line-strong);
    border-radius: var(--radius-md);
    font-variant-numeric: tabular-nums;
    overflow: hidden;
  }
  .ni:hover:not(.off, .locked) { border-color: var(--ink-subtle); }
  .ni:focus-within { border-color: var(--focus-ring); box-shadow: 0 0 0 1px var(--focus-ring); }
  .ni[aria-invalid='true'] { border-color: var(--tone-error-solid); }
  .locked { background: var(--surface-sunken); border-style: dashed; }
  .off { background: var(--surface-sunken); color: var(--ink-disabled); border-color: var(--line); }
  .sm { min-height: var(--control-h-sm); }
  .lg { min-height: var(--control-h-lg); }
  input {
    width: 7ch;
    min-width: 0;
    flex: 1;
    padding: 0 var(--space-2);
    font-size: var(--text-md);
    text-align: right;
    background: transparent;
    border: 0;
    appearance: textfield;
    -moz-appearance: textfield;
  }
  input:focus-visible { outline: none; }
  input::-webkit-inner-spin-button, input::-webkit-outer-spin-button { appearance: none; margin: 0; }
  .unit { align-self: center; padding-right: var(--space-2); font-size: var(--text-sm); color: var(--ink-muted); }
  button {
    display: grid;
    place-items: center;
    width: var(--control-h-sm);
    padding: 0;
    color: var(--ink-muted);
    background: transparent;
    border: 0;
  }
  button:first-child { border-right: var(--border-w) solid var(--line); }
  button:last-child { border-left: var(--border-w) solid var(--line); }
  button:hover:not(:disabled) { background: var(--surface-hover); color: var(--ink); }
  button:disabled { color: var(--ink-disabled); }
  @media (width < 48rem), (pointer: coarse) {
    .ni, .sm { min-height: var(--target-min); }
    input { font-size: var(--text-base); }
    button { width: var(--target-min); }
  }
</style>
