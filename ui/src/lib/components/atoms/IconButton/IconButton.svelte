<!--
  IconButton: a square button whose only content is an icon. `label` is required: it is the
  accessible name and the hover/focus tooltip. `pressed` makes it a toggle (aria-pressed).
-->
<script lang="ts">
  import type { HTMLButtonAttributes } from 'svelte/elements';
  import Icon from '../Icon/Icon.svelte';
  import Spinner from '../Spinner/Spinner.svelte';
  import Tooltip from '../Tooltip/Tooltip.svelte';
  import type { IconName } from '../Icon/icons';
  import type { Size } from '../../../tokens';

  interface Props extends Omit<HTMLButtonAttributes, 'children'> {
    icon: IconName;
    label: string;
    variant?: 'ghost' | 'secondary' | 'primary';
    size?: Size;
    pressed?: boolean;
    loading?: boolean;
    /** Show the label as a tooltip (default). Turn off where the label is visible nearby. */
    tooltip?: boolean;
  }
  let {
    icon, label, variant = 'ghost', size = 'md', pressed, loading = false, tooltip = true,
    disabled = false, type = 'button', class: cls, ...rest
  }: Props = $props();
  const iconSize = $derived(size === 'lg' ? 'lg' : size === 'sm' ? 'sm' : 'md');
</script>

{#snippet button(trigger?: Record<string, unknown>)}
  <button
    {type}
    class={['ib', variant, size, cls]}
    aria-label={label}
    aria-pressed={pressed}
    aria-busy={loading || undefined}
    disabled={disabled || loading}
    {...trigger}
    {...rest}
  >
    {#if loading}<Spinner size={iconSize} />{:else}<Icon name={icon} size={iconSize} />{/if}
  </button>
{/snippet}

{#if tooltip}
  <Tooltip text={label} describe={false}>
    {#snippet children(trigger)}{@render button(trigger as unknown as Record<string, unknown>)}{/snippet}
  </Tooltip>
{:else}
  {@render button()}
{/if}

<style>
  .ib {
    --s: var(--control-h-md);
    position: relative;
    display: inline-grid;
    place-items: center;
    width: var(--s);
    height: var(--s);
    padding: 0;
    color: var(--ink-muted);
    background: transparent;
    border: var(--border-w) solid transparent;
    border-radius: var(--radius-md);
    transition: background-color var(--dur-fast) var(--ease-out), color var(--dur-fast) var(--ease-out);
  }
  .sm { --s: var(--control-h-sm); }
  .lg { --s: var(--control-h-lg); }
  .ib:hover { background: var(--surface-hover); color: var(--ink); }
  .ib:active { background: var(--surface-pressed); }
  .secondary { background: var(--surface-raised); border-color: var(--line-strong); color: var(--ink); }
  .primary { background: var(--accent); color: var(--ink-on-accent); }
  .primary:hover { background: var(--accent-hover); color: var(--ink-on-accent); }
  .ib[aria-pressed='true'] { background: var(--accent-tint); color: var(--accent-ink); }
  .ib:disabled { color: var(--ink-disabled); background: transparent; }

  /* Small visible mark, full-size hit area on touch. */
  @media (width < 48rem), (pointer: coarse) {
    .ib::before { content: ''; position: absolute; inset: min(0px, calc((var(--s) - var(--target-min)) / 2)); }
  }
</style>
