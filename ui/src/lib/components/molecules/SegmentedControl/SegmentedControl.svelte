<!--
  SegmentedControl: one choice among two to five view modes or ranges that apply at once
  (table / posters, 24 h / 7 d / 30 d, week / month / agenda). A radio group: arrows move and
  select, one tab stop. Icons are optional; labels are always present (visible or accessible).
-->
<script lang="ts" module>
  import type { IconName } from '../../atoms/Icon/icons';
  export interface Segment { value: string; label: string; icon?: IconName; disabled?: boolean }
</script>

<script lang="ts">
  import Icon from '../../atoms/Icon/Icon.svelte';
  import type { Size } from '../../../tokens';

  interface Props {
    options: Segment[];
    value?: string;
    label: string;
    /** Show only icons (labels stay accessible names). */
    iconOnly?: boolean;
    size?: Exclude<Size, 'lg'>;
    onchange?: (value: string) => void;
  }
  let { options, value = $bindable(options[0]?.value ?? ''), label, iconOnly = false, size = 'md', onchange }: Props = $props();

  let group = $state<HTMLElement>();

  function select(o: Segment, focus = false) {
    if (o.disabled) return;
    if (value !== o.value) {
      value = o.value;
      onchange?.(o.value);
    }
    if (focus) group?.querySelector<HTMLElement>(`[data-value="${CSS.escape(o.value)}"]`)?.focus();
  }
  function onkeydown(e: KeyboardEvent) {
    const enabled = options.filter((o) => !o.disabled);
    const i = enabled.findIndex((o) => o.value === value);
    const step = { ArrowRight: 1, ArrowDown: 1, ArrowLeft: -1, ArrowUp: -1 }[e.key];
    if (step === undefined) return;
    e.preventDefault();
    select(enabled[(i + step + enabled.length) % enabled.length], true);
  }
</script>

<!-- svelte-ignore a11y_interactive_supports_focus -->
<div class={['seg', size]} role="radiogroup" aria-label={label} bind:this={group} {onkeydown}>
  {#each options as o (o.value)}
    <button
      type="button" role="radio" data-value={o.value} aria-checked={value === o.value}
      aria-label={iconOnly ? o.label : undefined} tabindex={value === o.value ? 0 : -1}
      disabled={o.disabled} onclick={() => select(o)}
    >
      {#if o.icon}<Icon name={o.icon} size="sm" />{/if}
      {#if !iconOnly}<span>{o.label}</span>{/if}
    </button>
  {/each}
</div>

<style>
  .seg {
    display: inline-flex;
    padding: var(--space-0-5);
    gap: var(--space-0-5);
    max-width: 100%;
    overflow-x: auto;
    background: var(--surface-sunken);
    border: var(--border-w) solid var(--line);
    border-radius: var(--radius-md);
  }
  button {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: var(--space-1);
    flex: none;
    min-height: calc(var(--control-h-md) - var(--space-1) - 2 * var(--border-w));
    min-width: calc(var(--control-h-md) - var(--space-1));
    padding: 0 var(--space-3);
    font-size: var(--text-md);
    font-weight: var(--weight-medium);
    color: var(--ink-muted);
    background: none;
    border: var(--border-w) solid transparent;
    border-radius: var(--radius-sm);
    transition: color var(--dur-fast) var(--ease-out), background-color var(--dur-fast) var(--ease-out);
  }
  .sm button { min-height: calc(var(--control-h-sm) - var(--space-1) - 2 * var(--border-w)); padding: 0 var(--space-2); font-size: var(--text-sm); }
  button:hover:not(:disabled) { color: var(--ink); }
  button[aria-checked='true'] { color: var(--ink); background: var(--surface-raised); border-color: var(--line-strong); }
  button:disabled { color: var(--ink-disabled); }
  @media (width < 48rem), (pointer: coarse) {
    button, .sm button { min-height: var(--target-min); min-width: var(--target-min); }
  }
</style>
