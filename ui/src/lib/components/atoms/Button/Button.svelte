<!--
  Button: the one action control. `primary` for the screen's main action (one per view),
  `secondary` for the rest, `ghost` inside dense rows and toolbars, `danger` only as the
  confirming action of a destructive dialog. Pass `href` to render a link that looks
  like a button (navigation only, never for actions).
-->
<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { HTMLButtonAttributes } from 'svelte/elements';
  import Icon from '../Icon/Icon.svelte';
  import Spinner from '../Spinner/Spinner.svelte';
  import type { IconName } from '../Icon/icons';
  import type { Size } from '../../../tokens';

  export type ButtonVariant = 'primary' | 'secondary' | 'ghost' | 'danger';

  interface Props extends Omit<HTMLButtonAttributes, 'children'> {
    variant?: ButtonVariant;
    size?: Size;
    /** Leading icon. */
    icon?: IconName;
    /** Trailing icon (menus, disclosure). */
    iconEnd?: IconName;
    loading?: boolean;
    href?: string;
    children?: Snippet;
  }
  let {
    variant = 'secondary',
    size = 'md',
    icon,
    iconEnd,
    loading = false,
    disabled = false,
    href,
    type = 'button',
    class: cls,
    children,
    ...rest
  }: Props = $props();

  const iconSize = $derived(size === 'sm' ? 'sm' : 'md');
</script>

{#snippet content()}
  {#if loading}<Spinner size={iconSize} />{:else if icon}<Icon name={icon} size={iconSize} />{/if}
  {#if children}<span class="label">{@render children()}</span>{/if}
  {#if iconEnd}<Icon name={iconEnd} size={iconSize} />{/if}
{/snippet}

{#if href && !disabled}
  <a {href} class={['btn', variant, size, cls]} {...rest as Record<string, unknown>}>{@render content()}</a>
{:else}
  <button
    {type}
    class={['btn', variant, size, cls]}
    disabled={disabled || loading}
    aria-busy={loading || undefined}
    {...rest}
  >{@render content()}</button>
{/if}

<style>
  .btn {
    --h: var(--control-h-md);
    --px: var(--space-3);
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: var(--space-1-5);
    min-height: var(--h);
    padding: 0 var(--px);
    border: var(--border-w) solid transparent;
    border-radius: var(--radius-md);
    font-size: var(--text-md);
    font-weight: var(--weight-medium);
    line-height: 1;
    white-space: nowrap;
    text-decoration: none;
    user-select: none;
    transition:
      background-color var(--dur-fast) var(--ease-out),
      border-color var(--dur-fast) var(--ease-out),
      color var(--dur-fast) var(--ease-out);
  }
  .sm { --h: var(--control-h-sm); --px: var(--space-2); font-size: var(--text-sm); gap: var(--space-1); }
  .lg { --h: var(--control-h-lg); --px: var(--space-4); font-size: var(--text-base); }
  .label { overflow: hidden; text-overflow: ellipsis; }

  .primary { background: var(--accent); color: var(--ink-on-accent); }
  .primary:hover { background: var(--accent-hover); }
  .primary:active { background: var(--accent-pressed); }

  .secondary { background: var(--surface-raised); color: var(--ink); border-color: var(--line-strong); }
  .secondary:hover { background: var(--surface-hover); }
  .secondary:active { background: var(--surface-pressed); }

  .ghost { background: transparent; color: var(--ink); }
  .ghost:hover { background: var(--surface-hover); }
  .ghost:active { background: var(--surface-pressed); }

  .danger { background: var(--danger); color: var(--ink-on-danger); }
  .danger:hover { background: var(--danger-hover); }

  .btn:disabled { background: var(--surface-sunken); color: var(--ink-disabled); border-color: var(--line); }
  .ghost:disabled { background: transparent; border-color: transparent; }
  .btn[aria-busy='true'] { color: var(--ink-muted); cursor: progress; }

  @media (width < 48rem), (pointer: coarse) {
    .btn { min-height: var(--target-min); }
  }
</style>
