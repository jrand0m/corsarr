<!--
  InlineCheck: the outcome of a quick check shown right under a field: a status badge, one plain
  sentence, and optional detail rows (matched and unmapped folders, sample paths). It renders
  nothing for an idle check, so a field without a result takes no space.
-->
<script lang="ts">
  import StatusBadge from '../../atoms/StatusBadge/StatusBadge.svelte';
  import type { StatusFamily } from '../../../status/vocabulary';

  interface Props {
    family: StatusFamily;
    /** Status value in the family; nothing shows without one. */
    value?: string;
    /** Badge label when the vocabulary label is too generic ("Matches 2 of 3"). */
    label?: string;
    detail?: string;
    items?: string[];
    /** Rows shown before "and N more". */
    maxItems?: number;
  }
  let { family, value, label, detail, items = [], maxItems = 6 }: Props = $props();
  const shown = $derived(items.slice(0, maxItems));
</script>

{#if value}
  <div class="ic">
    <p class="line"><StatusBadge {family} {value} {label} size="sm" />{#if detail}<span class="detail">{detail}</span>{/if}</p>
    {#if shown.length}
      <ul>
        {#each shown as item, i (i)}<li>{item}</li>{/each}
        {#if items.length > maxItems}<li class="more">and {items.length - maxItems} more</li>{/if}
      </ul>
    {/if}
  </div>
{/if}

<style>
  .ic { display: grid; gap: var(--space-1); min-width: 0; }
  .line { display: flex; flex-wrap: wrap; align-items: center; gap: var(--space-1-5); font-size: var(--text-sm); }
  .detail { color: var(--ink-muted); overflow-wrap: anywhere; }
  ul { display: grid; gap: var(--space-0-5); margin: 0; padding-left: var(--space-4); font-family: var(--font-mono); font-size: var(--text-xs); color: var(--ink-muted); }
  li { overflow-wrap: anywhere; }
  .more { font-family: inherit; list-style: none; }
</style>
