<!--
  ActionMenu: a "more actions" button that opens a short list of actions. Built on the
  Popover API (popover="auto"), so it opens, light-dismisses and closes on Escape without
  scripts, and sits in the top layer above drawers and clipped tables. Scripts add placement
  next to the trigger, arrow-key movement and focus return. Items are plain buttons (a
  disclosure, not an application menu); destructive items say so with `danger` and confirm
  in the caller.
-->
<script lang="ts" module>
  import type { IconName } from '../../atoms/Icon/icons';
  export interface ActionItem {
    id: string;
    label: string;
    icon?: IconName;
    danger?: boolean;
    disabled?: boolean;
    /** Items with a different group get a divider before them. */
    group?: string;
    /** Shortcut hint, display only: "Shift+↑". */
    hint?: string;
  }
</script>

<script lang="ts">
  import Icon from '../../atoms/Icon/Icon.svelte';
  import Button from '../../atoms/Button/Button.svelte';
  import { placeNear } from '../../../overlay/anchor';
  import type { Size } from '../../../tokens';

  interface Props {
    items: ActionItem[];
    /** Accessible name of the trigger: "More actions for The Long Haul S02E03". */
    label: string;
    /** Visible trigger text; icon-only when omitted. */
    text?: string;
    icon?: IconName;
    size?: Size;
    variant?: 'ghost' | 'secondary';
    onselect?: (id: string) => void;
  }
  let { items, label, text, icon = 'more', size = 'md', variant = 'ghost', onselect }: Props = $props();

  const id = $props.id();
  let open = $state(false);
  let panel = $state<HTMLElement>();
  let trigger = $state<HTMLElement>();
  let pos = $state({ top: 0, left: 0 });

  function place() {
    if (!trigger || !panel) return;
    pos = placeNear(
      trigger.getBoundingClientRect(),
      { width: panel.offsetWidth, height: panel.offsetHeight },
      { width: innerWidth, height: innerHeight },
    );
  }
  function ontoggle(e: Event) {
    open = (e as ToggleEvent).newState === 'open';
    if (open) {
      place();
      panel?.querySelector<HTMLButtonElement>('button:not(:disabled)')?.focus();
    }
  }
  function choose(item: ActionItem) {
    panel?.hidePopover?.();
    trigger?.querySelector('button')?.focus();
    onselect?.(item.id);
  }
  function onkeydown(e: KeyboardEvent) {
    const btns = [...(panel?.querySelectorAll<HTMLButtonElement>('button:not(:disabled)') ?? [])];
    const i = btns.indexOf(document.activeElement as HTMLButtonElement);
    const next = { ArrowDown: i + 1, ArrowUp: i - 1, Home: 0, End: btns.length - 1 }[e.key];
    if (next === undefined) return;
    e.preventDefault();
    btns[(next + btns.length) % btns.length]?.focus();
  }
</script>

<span class="am" bind:this={trigger}>
  <Button
    {size} {variant} icon={icon} popovertarget="{id}-menu" aria-expanded={open} aria-controls="{id}-menu"
    aria-label={text ? undefined : label} class={text ? undefined : 'icon-only'}
  >{#if text}{text}{/if}</Button>
</span>
<!-- Arrow keys are handled for the item buttons inside (delegated); the group itself is not focusable. -->
<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
<div
  class="panel" id="{id}-menu" popover="auto" bind:this={panel} {ontoggle} {onkeydown}
  style:top="{pos.top}px" style:left="{pos.left}px" aria-label={label} role="group"
>
  <ul>
    {#each items as item, i (item.id)}
      {#if i > 0 && item.group !== items[i - 1].group}<li class="sep" role="separator"></li>{/if}
      <li>
        <button type="button" class:danger={item.danger} disabled={item.disabled} onclick={() => choose(item)}>
          {#if item.icon}<Icon name={item.icon} size="sm" />{:else}<span class="pad"></span>{/if}
          <span class="label">{item.label}</span>
          {#if item.hint}<span class="hint">{item.hint}</span>{/if}
        </button>
      </li>
    {/each}
  </ul>
</div>

<style>
  .am { display: inline-flex; }
  .am :global(.icon-only) { padding: 0; width: var(--control-h-md); }
  .am :global(.icon-only.sm) { width: var(--control-h-sm); }
  .panel {
    position: fixed;
    inset: auto;
    margin: 0;
    min-width: 12rem;
    max-width: min(20rem, calc(100vw - var(--space-4)));
    padding: var(--space-1);
    color: var(--ink);
    background: var(--surface-overlay);
    border: var(--border-w) solid var(--line);
    border-radius: var(--radius-md);
    box-shadow: var(--shadow-sm);
  }
  ul { display: grid; padding: 0; margin: 0; list-style: none; }
  .sep { height: var(--border-w); margin: var(--space-1) 0; background: var(--line); }
  button {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    width: 100%;
    min-height: var(--control-h-md);
    padding: 0 var(--space-2);
    font-size: var(--text-md);
    text-align: left;
    color: var(--ink);
    background: none;
    border: 0;
    border-radius: var(--radius-sm);
  }
  button :global(svg) { color: var(--ink-muted); }
  button:hover:not(:disabled), button:focus-visible { background: var(--surface-hover); }
  button:focus-visible { outline-offset: calc(var(--focus-w) * -1); }
  .danger, .danger :global(svg) { color: var(--tone-error-ink); }
  button:disabled, button:disabled :global(svg) { color: var(--ink-disabled); }
  .pad { width: var(--icon-sm); flex: none; }
  .label { flex: 1; }
  .hint { font-size: var(--text-xs); color: var(--ink-subtle); }
  @media (width < 48rem), (pointer: coarse) {
    button { min-height: var(--target-min); }
    .hint { display: none; }
  }
</style>
