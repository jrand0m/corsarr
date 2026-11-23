<!--
  Switch: an immediate on/off setting (enable an indexer, kind on/off). Changes apply at once;
  inside a form that saves later, use Checkbox instead. Always carries a label.
-->
<script lang="ts">
  import type { HTMLButtonAttributes } from 'svelte/elements';

  interface Props extends Omit<HTMLButtonAttributes, 'children' | 'onchange'> {
    checked?: boolean;
    label: string;
    /** Hide the visible label (it stays the accessible name). */
    hideLabel?: boolean;
    onchange?: (checked: boolean) => void;
  }
  let { checked = $bindable(false), label, hideLabel = false, disabled, onchange, class: cls, ...rest }: Props = $props();

  function toggle() {
    checked = !checked;
    onchange?.(checked);
  }
</script>

<button
  type="button"
  role="switch"
  aria-checked={checked}
  class={['sw', cls]}
  {disabled}
  onclick={toggle}
  aria-label={hideLabel ? label : undefined}
  {...rest}
>
  <span class="track" aria-hidden="true"><span class="thumb"></span></span>
  {#if !hideLabel}<span class="label">{label}</span>{/if}
</button>

<style>
  .sw {
    position: relative;
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
    padding: 0;
    font-size: var(--text-md);
    text-align: left;
    background: none;
    border: 0;
    border-radius: var(--radius-sm);
  }
  .track {
    position: relative;
    width: 2.25rem;
    height: 1.25rem;
    flex: none;
    background: var(--surface-sunken);
    border: 1.5px solid var(--line-strong);
    border-radius: var(--radius-round);
    transition: background-color var(--dur-fast) var(--ease-out), border-color var(--dur-fast) var(--ease-out);
  }
  .thumb {
    position: absolute;
    top: 1.5px;
    left: 1.5px;
    width: 0.875rem;
    height: 0.875rem;
    background: var(--ink-muted);
    border-radius: var(--radius-round);
    transition: transform var(--dur-base) var(--ease-out), background-color var(--dur-fast) var(--ease-out);
  }
  .sw:hover:not(:disabled) .track { border-color: var(--ink-muted); }
  [aria-checked='true'] .track { background: var(--accent); border-color: var(--accent); }
  [aria-checked='true'] .thumb { transform: translateX(1rem); background: var(--ink-on-accent); }
  .sw:disabled { color: var(--ink-disabled); }
  .sw:disabled .track { background: var(--surface-sunken); border-color: var(--line); }
  .sw:disabled .thumb { background: var(--ink-disabled); }
  @media (width < 48rem), (pointer: coarse) {
    .sw { min-height: var(--target-min); min-width: var(--target-min); }
  }
</style>
