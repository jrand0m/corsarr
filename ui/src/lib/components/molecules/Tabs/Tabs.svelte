<!--
  Tabs: switch between views of one object (an indexer's settings, stats, history).
  WAI-ARIA tabs with automatic activation: arrows move and select, Home/End jump, only the
  current tab is in the tab order. URL sync is the page's job: bind `value` or use `onchange`
  to write the query string. The panel renders `children(activeId)`. On phones the tab list
  scrolls sideways inside itself; the page never does.
-->
<script lang="ts" module>
  export interface TabItem { id: string; label: string; count?: number; disabled?: boolean }
</script>

<script lang="ts">
  import type { Snippet } from 'svelte';
  import { formatCount } from '../../../format/quantity';

  interface Props {
    tabs: TabItem[];
    value?: string;
    label: string;
    onchange?: (id: string) => void;
    children?: Snippet<[string]>;
  }
  let { tabs, value = $bindable(tabs[0]?.id ?? ''), label, onchange, children }: Props = $props();

  const id = $props.id();
  let list = $state<HTMLElement>();

  function select(tab: TabItem, focus = false) {
    if (tab.disabled) return;
    if (value !== tab.id) {
      value = tab.id;
      onchange?.(tab.id);
    }
    if (focus) list?.querySelector<HTMLElement>(`[data-tab="${CSS.escape(tab.id)}"]`)?.focus();
  }
  function onkeydown(e: KeyboardEvent) {
    const enabled = tabs.filter((t) => !t.disabled);
    const i = enabled.findIndex((t) => t.id === value);
    const next = { ArrowRight: i + 1, ArrowLeft: i - 1, Home: 0, End: enabled.length - 1 }[e.key];
    if (next === undefined) return;
    e.preventDefault();
    select(enabled[(next + enabled.length) % enabled.length], true);
  }
</script>

<div class="tabs">
  <!-- svelte-ignore a11y_interactive_supports_focus -->
  <div class="list" role="tablist" aria-label={label} bind:this={list} {onkeydown}>
    {#each tabs as t (t.id)}
      <button
        type="button" role="tab" id="{id}-t-{t.id}" data-tab={t.id} aria-selected={value === t.id}
        aria-controls="{id}-p" tabindex={value === t.id ? 0 : -1} disabled={t.disabled} onclick={() => select(t)}
      >
        {t.label}{#if t.count !== undefined}<span class="count num">{formatCount(t.count)}</span>{/if}
      </button>
    {/each}
  </div>
  {#if children}
    <div class="panel" role="tabpanel" id="{id}-p" aria-labelledby="{id}-t-{value}" tabindex="0">{@render children(value)}</div>
  {/if}
</div>

<style>
  .tabs { display: grid; gap: var(--space-4); min-width: 0; }
  .list { display: flex; gap: var(--space-1); overflow-x: auto; scrollbar-width: none; border-bottom: var(--border-w) solid var(--line); }
  button {
    position: relative;
    display: inline-flex;
    align-items: center;
    gap: var(--space-1-5);
    flex: none;
    min-height: var(--control-h-md);
    padding: 0 var(--space-3);
    font-size: var(--text-md);
    font-weight: var(--weight-medium);
    color: var(--ink-muted);
    white-space: nowrap;
    background: none;
    border: 0;
    border-radius: var(--radius-md) var(--radius-md) 0 0;
    transition: color var(--dur-fast) var(--ease-out), background-color var(--dur-fast) var(--ease-out);
  }
  button:hover:not(:disabled) { color: var(--ink); background: var(--surface-hover); }
  button:focus-visible { outline-offset: calc(var(--focus-w) * -1); }
  /* The current tab sits on the rule like a berth marker: a full-width underline, not a side bar. */
  button[aria-selected='true'] { color: var(--ink); }
  button[aria-selected='true']::after { content: ''; position: absolute; inset: auto 0 calc(var(--border-w) * -1); height: 2px; background: var(--accent); }
  button:disabled { color: var(--ink-disabled); }
  .count { padding: 0 var(--space-1-5); font-size: var(--text-xs); color: var(--ink-muted); background: var(--surface-sunken); border-radius: var(--radius-sm); }
  .panel { min-width: 0; }
  .panel:focus-visible { outline-offset: var(--space-1); }
  @media (width < 48rem), (pointer: coarse) {
    button { min-height: var(--target-min); }
  }
</style>
