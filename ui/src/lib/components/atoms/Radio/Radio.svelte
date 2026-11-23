<!--
  Radio: one option of a group. Bind the group's value with `bind:group`; wrap related
  radios in a fieldset with a legend (the Field molecule's group mode).
-->
<script lang="ts">
  import type { HTMLInputAttributes } from 'svelte/elements';

  interface Props extends Omit<HTMLInputAttributes, 'type' | 'value'> {
    group?: string;
    value: string;
    label: string;
    description?: string;
  }
  let { group = $bindable(''), value, label, description, disabled, class: cls, ...rest }: Props = $props();
  const id = $props.id();
</script>

<label class={['rd', cls]} class:off={disabled}>
  <span class="control">
    <input type="radio" bind:group {value} {disabled} aria-labelledby="{id}-l" aria-describedby={description ? `${id}-d` : undefined} {...rest} />
  </span>
  <span class="text">
    <span id="{id}-l">{label}</span>
    {#if description}<span class="desc" id="{id}-d">{description}</span>{/if}
  </span>
</label>

<style>
  .rd { display: inline-flex; align-items: flex-start; gap: var(--space-2); font-size: var(--text-md); cursor: pointer; }
  .off { cursor: not-allowed; color: var(--ink-disabled); }
  .control { position: relative; display: grid; width: 1rem; height: 1rem; margin-top: 0.1875rem; flex: none; }
  input {
    appearance: none;
    margin: 0;
    width: 100%;
    height: 100%;
    background: var(--surface-raised);
    border: 1.5px solid var(--line-strong);
    border-radius: var(--radius-round);
    cursor: inherit;
    transition: border-color var(--dur-fast) var(--ease-out), border-width var(--dur-fast) var(--ease-out);
  }
  input:hover:not(:disabled) { border-color: var(--ink-muted); }
  input:checked { border: 5px solid var(--accent); background: var(--ink-on-accent); }
  input:disabled { background: var(--surface-sunken); border-color: var(--line); }
  input:checked:disabled { border-color: var(--ink-disabled); }
  .text { display: grid; gap: var(--space-0-5); }
  .desc { font-size: var(--text-sm); color: var(--ink-muted); }
  .off .desc { color: var(--ink-disabled); }
  @media (width < 48rem), (pointer: coarse) {
    .rd { min-height: var(--target-min); align-items: center; }
    .control { margin-top: 0; }
    .control::before { content: ''; position: absolute; inset: -0.875rem; }
  }
</style>
