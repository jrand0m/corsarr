<!--
  MonitorToggle: whether corsarr looks for and upgrades an item. A toggle button with a
  stable name ("Monitor S02E03") and aria-pressed. `pinned` freezes the item with a reason
  ("keeping the director's cut"): it cannot be toggled; with `onunpin` the button unpins it.
  `appearance="icon"` for table rows, `"labelled"` for headers and cards.
-->
<script lang="ts" module>
  export type MonitorState = 'monitored' | 'unmonitored' | 'pinned';
</script>

<script lang="ts">
  import Icon from '../../atoms/Icon/Icon.svelte';
  import Tooltip from '../../atoms/Tooltip/Tooltip.svelte';
  import type { IconName } from '../../atoms/Icon/icons';

  interface Props {
    state?: MonitorState;
    /** What is monitored, for the accessible name: "S02E03", "The Long Haul". */
    item: string;
    /** Why it is pinned. */
    reason?: string;
    appearance?: 'icon' | 'labelled';
    size?: 'sm' | 'md';
    disabled?: boolean;
    onchange?: (state: MonitorState) => void;
    onunpin?: () => void;
  }
  let { state = $bindable('monitored'), item, reason, appearance = 'icon', size = 'md', disabled = false, onchange, onunpin }: Props = $props();

  const glyph: Record<MonitorState, IconName> = { monitored: 'eye', unmonitored: 'eye-off', pinned: 'lock' };
  const text: Record<MonitorState, string> = { monitored: 'Monitored', unmonitored: 'Not monitored', pinned: 'Pinned' };
  const pinned = $derived(state === 'pinned');
  const name = $derived(pinned ? (onunpin ? `Unpin ${item}` : `${item} is pinned`) : `Monitor ${item}`);
  const tip = $derived(pinned ? `Pinned${reason ? `: ${reason}` : ''}` : state === 'monitored' ? 'Monitored: corsarr searches and upgrades it' : 'Not monitored: corsarr leaves it alone');

  function click() {
    if (pinned) { onunpin?.(); return; }
    state = state === 'monitored' ? 'unmonitored' : 'monitored';
    onchange?.(state);
  }
</script>

<Tooltip text={tip}>
  {#snippet children(trigger)}
    <button
      type="button" class={['mt', state, appearance, size]} {...trigger}
      aria-label={appearance === 'icon' ? name : undefined}
      aria-pressed={pinned ? undefined : state === 'monitored'}
      aria-disabled={(pinned && !onunpin) || disabled || undefined}
      disabled={disabled}
      onclick={() => { if (!(pinned && !onunpin)) click(); }}
    >
      <Icon name={glyph[state]} size="sm" />
      {#if appearance === 'labelled'}
        <span>{text[state]}</span><span class="visually-hidden">{pinned ? `${reason ? `: ${reason}` : ''}` : `: ${item}`}</span>
      {/if}
    </button>
  {/snippet}
</Tooltip>

<style>
  .mt {
    position: relative;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: var(--space-1-5);
    min-width: var(--control-h-md);
    min-height: var(--control-h-md);
    padding: 0;
    font-size: var(--text-md);
    font-weight: var(--weight-medium);
    color: var(--ink-subtle);
    background: none;
    border: var(--border-w) solid transparent;
    border-radius: var(--radius-md);
    transition: color var(--dur-fast) var(--ease-out), background-color var(--dur-fast) var(--ease-out);
  }
  .sm { min-width: var(--control-h-sm); min-height: var(--control-h-sm); font-size: var(--text-sm); }
  .labelled { padding: 0 var(--space-3); border-color: var(--line-strong); background: var(--surface-raised); }
  .sm.labelled { padding: 0 var(--space-2); }
  .monitored { color: var(--accent-ink); }
  .pinned { color: var(--ink-muted); }
  .labelled.pinned { border-style: dashed; }
  .mt:hover:not(:disabled, [aria-disabled='true']) { background: var(--surface-hover); }
  .mt[aria-disabled='true'] { cursor: not-allowed; }
  .mt:disabled { color: var(--ink-disabled); }
  @media (width < 48rem), (pointer: coarse) {
    .mt::before { content: ''; position: absolute; inset: min(0px, calc((var(--control-h-sm) - var(--target-min)) / 2)); }
    .labelled { min-height: var(--target-min); }
  }
</style>
