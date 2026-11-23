<!--
  TextInput: single-line text. The visible label and help/error text belong to the Field
  molecule, which passes `id` and aria-describedby; standalone use needs aria-label.
  `invalid` sets aria-invalid; `readonly` is the managed-by-config lock look.
-->
<script lang="ts">
  import type { HTMLInputAttributes } from 'svelte/elements';
  import Icon from '../Icon/Icon.svelte';
  import type { IconName } from '../Icon/icons';
  import type { Size } from '../../../tokens';

  interface Props extends Omit<HTMLInputAttributes, 'size' | 'value'> {
    value?: string;
    type?: 'text' | 'search' | 'email' | 'url' | 'password' | 'tel';
    size?: Size;
    invalid?: boolean;
    icon?: IconName;
    mono?: boolean;
  }
  let {
    value = $bindable(''), type = 'text', size = 'md', invalid = false, icon, mono = false,
    class: cls, ...rest
  }: Props = $props();
</script>

<span class={['ti', size, cls]} class:has-icon={icon}>
  {#if icon}<Icon name={icon} size={size === 'sm' ? 'sm' : 'md'} />{/if}
  <input {type} bind:value class:mono aria-invalid={invalid || undefined} {...rest} />
</span>

<style>
  .ti { position: relative; display: inline-flex; align-items: center; width: 100%; min-width: 0; color: var(--ink); }
  .ti :global(svg) { position: absolute; left: var(--space-2); color: var(--ink-subtle); pointer-events: none; }
  input {
    width: 100%;
    min-height: var(--control-h-md);
    padding: 0 var(--space-3);
    font-size: var(--text-md);
    background: var(--surface-raised);
    border: var(--border-w) solid var(--line-strong);
    border-radius: var(--radius-md);
    transition: border-color var(--dur-fast) var(--ease-out);
  }
  .has-icon input { padding-left: var(--space-8); }
  .sm input { min-height: var(--control-h-sm); font-size: var(--text-sm); padding-inline: var(--space-2); }
  .sm.has-icon input { padding-left: var(--space-6); }
  .sm :global(svg) { left: var(--space-1-5); }
  .lg input { min-height: var(--control-h-lg); font-size: var(--text-base); }
  .mono { font-family: var(--font-mono); font-size: var(--text-sm); }
  input:hover:not(:disabled, :read-only) { border-color: var(--ink-subtle); }
  input:focus-visible { outline-offset: 0; border-color: var(--focus-ring); }
  input[aria-invalid='true'] { border-color: var(--tone-error-solid); }
  input:read-only { background: var(--surface-sunken); border-style: dashed; border-color: var(--line-strong); }
  input:disabled { background: var(--surface-sunken); color: var(--ink-disabled); border-color: var(--line); }
  input[type='search']::-webkit-search-cancel-button { appearance: none; }
  @media (width < 48rem), (pointer: coarse) {
    input, .sm input { min-height: var(--target-min); font-size: var(--text-base); }
  }
</style>
