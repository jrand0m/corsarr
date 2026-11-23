<!--
  ItemRefSummary: which library items a download or event covers, in one line
  ("S02 · 13 episodes") with a counted link-status summary; expands to each link's status.
  The disclosure is a real button (aria-expanded); the list is plain markup, so it also renders
  without scripts (Telegram widgets show it collapsed).
-->
<script lang="ts" module>
  export interface ItemRef {
    /** "S02E03 · The Long Haul", "Track 4 · Dry Dock". */
    label: string;
    status: 'pending' | 'imported' | 'blocked' | 'skipped' | 'failed';
    /** Why: "already on disk at higher quality". */
    reason?: string;
    href?: string;
  }
</script>

<script lang="ts">
  import Icon from '../../atoms/Icon/Icon.svelte';
  import StatusBadge from '../../atoms/StatusBadge/StatusBadge.svelte';
  import { vocabulary } from '../../../status/vocabulary';

  interface Props {
    /** The one-line summary: "S02 · 13 episodes", "Harbour Lights · 11 tracks". */
    summary: string;
    items: ItemRef[];
    open?: boolean;
    /** Never expandable (single item, dense rows). */
    static?: boolean;
  }
  let { summary, items, open = $bindable(false), static: fixed = false }: Props = $props();

  const id = $props.id();
  const order: ItemRef['status'][] = ['failed', 'blocked', 'pending', 'skipped', 'imported'];
  const counts = $derived(
    order.map((s) => ({ s, n: items.filter((i) => i.status === s).length })).filter((c) => c.n > 0),
  );
  const uniform = $derived(counts.length === 1);
  const expandable = $derived(!fixed && items.length > 1);
</script>

<div class="irs">
  <div class="line">
    {#if expandable}
      <button type="button" class="toggle" aria-expanded={open} aria-controls="{id}-list" onclick={() => (open = !open)}>
        <Icon name={open ? 'chevron-down' : 'chevron-right'} size="sm" />
        <span class="summary">{summary}</span>
      </button>
    {:else}
      <span class="summary">{summary}</span>
    {/if}
    <span class="counts">
      {#if uniform}
        <StatusBadge family="link" value={counts[0].s} size="sm" appearance="plain" detail={items.length > 1 ? 'all' : undefined} />
      {:else}
        {#each counts as c (c.s)}
          <StatusBadge family="link" value={c.s} size="sm" appearance="plain" label="{c.n} {vocabulary.link[c.s].label.toLowerCase()}" />
        {/each}
      {/if}
    </span>
  </div>
  {#if expandable}
    <ul id="{id}-list" hidden={!open}>
      {#each items as item, i (i)}
        <li>
          {#if item.href}<a href={item.href} class="name">{item.label}</a>{:else}<span class="name">{item.label}</span>{/if}
          <StatusBadge family="link" value={item.status} size="sm" detail={item.reason} />
        </li>
      {/each}
    </ul>
  {/if}
</div>

<style>
  .irs { display: grid; gap: var(--space-1-5); min-width: 0; font-size: var(--text-md); }
  .line { display: flex; flex-wrap: wrap; align-items: center; gap: var(--space-1) var(--space-3); min-width: 0; }
  .toggle {
    display: inline-flex;
    align-items: center;
    gap: var(--space-1);
    margin-left: calc(var(--space-1) * -1);
    padding: var(--space-0-5) var(--space-1);
    color: var(--ink);
    background: none;
    border: 0;
    border-radius: var(--radius-sm);
  }
  .toggle:hover { background: var(--surface-hover); }
  .toggle :global(svg) { color: var(--ink-muted); }
  .summary { font-weight: var(--weight-medium); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .counts { display: inline-flex; flex-wrap: wrap; gap: var(--space-1) var(--space-3); }
  ul { display: grid; gap: var(--space-1); padding: 0 0 0 var(--space-5); list-style: none; }
  li { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: var(--space-1) var(--space-2); min-height: var(--control-h-sm); border-bottom: var(--border-w) solid var(--line); padding-bottom: var(--space-1); }
  li:last-child { border-bottom: 0; }
  .name { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: var(--text-sm); }
  @media (width < 48rem), (pointer: coarse) {
    .toggle { min-height: var(--target-min); }
    ul { padding-left: 0; }
  }
</style>
