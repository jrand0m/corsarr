<!--
  ReleaseResults: the interactive search result list. A "Sort by" choice above an ordered list of
  DecisionPanels, and an optional status note (for example when automation would grab nothing).
  The parent owns the order and the sort value; this only renders and reports changes.
-->
<script lang="ts">
  import Select, { type SelectOption } from '../../atoms/Select/Select.svelte';
  import DecisionPanel, { type Decision, type DecisionReason } from '../DecisionPanel/DecisionPanel.svelte';

  interface Props {
    items: { key: string; decision: Decision }[];
    sort: string;
    sortOptions: SelectOption[];
    onsort?: (value: string) => void;
    ongrab?: (key: string, overridden: DecisionReason[]) => void;
    note?: string;
    /** Fixed "now" (ms) for stories and tests. */
    now?: number;
  }
  let { items, sort, sortOptions, onsort, ongrab, note, now }: Props = $props();

  const id = $props.id();
  const label = $derived(sortOptions.find((o) => o.value === sort)?.label ?? '');
</script>

<div class="rr">
  <div class="bar">
    <label for="{id}-sort">Sort by</label>
    <Select id="{id}-sort" size="sm" value={sort} options={sortOptions} onchange={(e) => onsort?.((e.currentTarget as HTMLSelectElement).value)} />
  </div>
  {#if note}<p class="note" role="status">{note}</p>{/if}
  {#if items.length}
    <ol class="results" aria-label="Releases, {label.toLowerCase()}">
      {#each items as it (it.key)}
        <li><DecisionPanel decision={it.decision} {now} ongrab={ongrab ? (over) => ongrab(it.key, over) : undefined} /></li>
      {/each}
    </ol>
  {/if}
</div>

<style>
  .rr { display: grid; gap: var(--space-3); }
  .bar { display: flex; align-items: center; justify-content: flex-end; gap: var(--space-2); font-size: var(--text-sm); color: var(--ink-muted); }
  .note { margin: 0; color: var(--ink-muted); font-size: var(--text-md); }
  .results { display: grid; gap: var(--space-3); margin: 0; padding: 0; list-style: none; }
</style>
