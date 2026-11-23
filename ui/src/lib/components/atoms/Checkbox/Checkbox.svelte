<!--
  Checkbox: a native checkbox with its label. `indeterminate` shows the mixed state
  (select-all with a partial selection, "varies" in the mass editor). Without a visible
  label (table row selection) pass aria-label.
-->
<script lang="ts">
  import type { HTMLInputAttributes } from 'svelte/elements';

  interface Props extends Omit<HTMLInputAttributes, 'type' | 'checked'> {
    checked?: boolean;
    indeterminate?: boolean;
    label?: string;
    description?: string;
  }
  let {
    checked = $bindable(false), indeterminate = $bindable(false), label, description, disabled,
    class: cls, ...rest
  }: Props = $props();
  const id = $props.id();
</script>

<label class={['cb', cls]} class:off={disabled}>
  <span class="control">
    <input type="checkbox" bind:checked bind:indeterminate {disabled} aria-labelledby={label ? `${id}-l` : undefined} aria-describedby={description ? `${id}-d` : undefined} {...rest} />
    <svg class="mark" viewBox="0 0 16 16" aria-hidden="true">
      {#if indeterminate}<path d="M4 8h8" />{:else}<path d="M3.5 8.5 6.5 11.5 12.5 5" />{/if}
    </svg>
  </span>
  {#if label}
    <span class="text">
      <span class="label" id="{id}-l">{label}</span>
      {#if description}<span class="desc" id="{id}-d">{description}</span>{/if}
    </span>
  {/if}
</label>

<style>
  .cb { display: inline-flex; align-items: flex-start; gap: var(--space-2); font-size: var(--text-md); cursor: pointer; }
  .off { cursor: not-allowed; color: var(--ink-disabled); }
  .control { position: relative; display: grid; place-items: center; width: 1rem; height: 1rem; margin-top: 0.1875rem; flex: none; }
  input {
    appearance: none;
    position: absolute;
    inset: 0;
    margin: 0;
    background: var(--surface-raised);
    border: 1.5px solid var(--line-strong);
    border-radius: var(--radius-sm);
    cursor: inherit;
    transition: background-color var(--dur-fast) var(--ease-out), border-color var(--dur-fast) var(--ease-out);
  }
  input:hover:not(:disabled) { border-color: var(--ink-muted); }
  input:checked, input:indeterminate { background: var(--accent); border-color: var(--accent); }
  input:disabled { background: var(--surface-sunken); border-color: var(--line); }
  input:checked:disabled, input:indeterminate:disabled { background: var(--ink-disabled); border-color: var(--ink-disabled); }
  .mark { position: relative; width: 100%; height: 100%; fill: none; stroke: var(--ink-on-accent); stroke-width: 2; stroke-linecap: round; stroke-linejoin: round; opacity: 0; pointer-events: none; }
  input:checked + .mark, input:indeterminate + .mark { opacity: 1; }
  .text { display: grid; gap: var(--space-0-5); }
  .desc { font-size: var(--text-sm); color: var(--ink-muted); }
  .off .desc { color: var(--ink-disabled); }
  @media (width < 48rem), (pointer: coarse) {
    .cb { min-height: var(--target-min); align-items: center; }
    .control { margin-top: 0; }
    .control::before { content: ''; position: absolute; inset: -0.875rem; }
  }
</style>
