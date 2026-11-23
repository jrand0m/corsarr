<!--
  Select: a native single choice (best on phones and with keyboards). Use the Async picker
  for long or searchable lists. Pass `options`, or <option> elements as children.
-->
<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { HTMLSelectAttributes } from 'svelte/elements';
  import Icon from '../Icon/Icon.svelte';
  import type { Size } from '../../../tokens';

  export interface SelectOption { value: string; label: string; disabled?: boolean }

  interface Props extends Omit<HTMLSelectAttributes, 'size' | 'value'> {
    value?: string;
    options?: SelectOption[];
    placeholder?: string;
    size?: Size;
    invalid?: boolean;
    children?: Snippet;
  }
  let {
    value = $bindable(''), options, placeholder, size = 'md', invalid = false, class: cls, children, ...rest
  }: Props = $props();
</script>

<span class={['sel', size, cls]}>
  <select bind:value aria-invalid={invalid || undefined} {...rest}>
    {#if placeholder}<option value="" disabled>{placeholder}</option>{/if}
    {#if options}
      {#each options as o (o.value)}<option value={o.value} disabled={o.disabled}>{o.label}</option>{/each}
    {:else if children}
      {@render children()}
    {/if}
  </select>
  <Icon name="chevron-down" size="sm" />
</span>

<style>
  .sel { position: relative; display: inline-flex; align-items: center; min-width: 0; }
  .sel :global(svg) { position: absolute; right: var(--space-2); color: var(--ink-muted); pointer-events: none; }
  select {
    appearance: none;
    width: 100%;
    min-height: var(--control-h-md);
    padding: 0 var(--space-8) 0 var(--space-3);
    font-size: var(--text-md);
    background: var(--surface-raised);
    border: var(--border-w) solid var(--line-strong);
    border-radius: var(--radius-md);
    text-overflow: ellipsis;
  }
  .sm select { min-height: var(--control-h-sm); font-size: var(--text-sm); padding-left: var(--space-2); }
  .lg select { min-height: var(--control-h-lg); font-size: var(--text-base); }
  select:hover:not(:disabled) { border-color: var(--ink-subtle); }
  select:focus-visible { outline-offset: 0; border-color: var(--focus-ring); }
  select[aria-invalid='true'] { border-color: var(--tone-error-solid); }
  select:disabled { background: var(--surface-sunken); color: var(--ink-disabled); border-color: var(--line); }
  @media (width < 48rem), (pointer: coarse) {
    select, .sm select { min-height: var(--target-min); font-size: var(--text-base); }
  }
</style>
