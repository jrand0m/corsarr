<!--
  ListPage (template): one list screen (Library, Wanted, Activity, Settings
  lists). Page header with actions, FilterBar, a table (with its bulk action bar, phone
  list and expandable rows) or a poster grid, and the standard states: loading, empty,
  filtered-empty, error and a banner slot for stale or shadow notices. With `syncUrl` the filters,
  sort and view live in the hash query so back/forward and shared links restore them.
-->
<script lang="ts" generics="T">
  import { untrack, type Snippet } from 'svelte';
  import Button from '../../atoms/Button/Button.svelte';
  import SegmentedControl from '../../molecules/SegmentedControl/SegmentedControl.svelte';
  import EmptyState from '../../molecules/EmptyState/EmptyState.svelte';
  import ErrorState from '../../molecules/ErrorState/ErrorState.svelte';
  import FilterBar from '../../organisms/FilterBar/FilterBar.svelte';
  import DataTable, { type Column, type SortState } from '../../organisms/DataTable/DataTable.svelte';
  import PosterGrid, { type PosterItem } from '../../organisms/PosterGrid/PosterGrid.svelte';
  import type { ActionItem } from '../../molecules/ActionMenu/ActionMenu.svelte';
  import { activeCount, emptyFilter, type FilterFacet, type FilterState } from '../../../filters/url';
  import { currentRoute, listStateFromParams, listStateToParams, replaceParams, type ListState, type ListView } from '../../../route/hash';
  import type { Density } from '../../../tokens';

  interface Props {
    title: string;
    /** Line under the title: "412 movies · 2.1 TB". */
    description?: string;
    actions?: Snippet;
    banner?: Snippet;
    facets?: FilterFacet[];
    filter?: FilterState;
    searchLabel?: string;
    placeholder?: string;
    /** "7 of 412 movies" next to the filters. */
    summary?: string;
    /** Offer the poster view; needs `poster`. */
    poster?: (row: T) => PosterItem;
    view?: ListView;
    sort?: SortState;
    /** Read and write filters, sort and view in the URL. */
    syncUrl?: boolean;
    onstatechange?: (state: ListState) => void;
    rows: T[];
    columns: Column<T>[];
    rowKey: (row: T) => string;
    cell?: Snippet<[T, Column<T>]>;
    expand?: Snippet<[T]>;
    /** Keys of expanded rows. */
    expanded?: string[];
    rowActions?: (row: T) => ActionItem[];
    onaction?: (action: string, row: T) => void;
    onopen?: (row: T) => void;
    current?: string;
    selectable?: boolean;
    selected?: string[];
    bulkActions?: Snippet<[string[]]>;
    density?: Density;
    noun?: string;
    nounPlural?: string;
    total?: number;
    loading?: boolean;
    error?: string;
    errorCode?: string;
    onretry?: () => void;
    hasMore?: boolean;
    loadingMore?: boolean;
    onloadmore?: () => void;
    /** Shown when the list is empty with no filters (cause + next action). */
    empty?: Snippet;
    /** Height of the scrolling region. */
    height?: string;
  }
  let {
    title, description, actions, banner, facets = [], filter = $bindable(emptyFilter()), searchLabel, placeholder, summary,
    poster, view = $bindable('table'), sort = $bindable(), syncUrl = false, onstatechange, rows, columns, rowKey, cell, expand, expanded = $bindable([]),
    rowActions, onaction, onopen, current, selectable = false, selected = $bindable([]), bulkActions, density = $bindable('comfortable'),
    noun = 'item', nounPlural, total, loading = false, error, errorCode, onretry, hasMore = false, loadingMore = false, onloadmore,
    empty, height = 'max(22rem, calc(100dvh - 17rem))',
  }: Props = $props();

  const plural = $derived(nounPlural ?? `${noun}s`);
  const filtered = $derived(activeCount(filter) > 0);
  const posters = $derived(poster && view === 'posters' ? rows.map(poster) : []);

  // Read the URL once at mount; later changes flow the other way.
  untrack(() => {
    if (!syncUrl) return;
    const s = listStateFromParams(currentRoute().params, facets, { sort, view });
    filter = s.filter;
    sort = s.sort;
    view = poster ? s.view : 'table';
  });
  $effect(() => {
    const state: ListState = { filter, sort, view };
    if (syncUrl) replaceParams(listStateToParams(state, untrack(() => currentRoute().params)));
    untrack(() => onstatechange?.(state));
  });
  const clear = () => (filter = emptyFilter());
</script>

{#snippet filteredEmpty()}
  <EmptyState title="No {plural} match these filters" icon="filter">
    Change or clear the filters to see more.
    {#snippet actions()}<Button onclick={clear}>Clear filters</Button>{/snippet}
  </EmptyState>
{/snippet}

{#snippet emptyState()}
  {#if filtered}{@render filteredEmpty()}{:else if empty}{@render empty()}{:else}<EmptyState title="No {plural} yet" />{/if}
{/snippet}

<div class="lp">
  <header class="head">
    <div class="titles">
      <h1>{title}</h1>
      {#if description}<p class="desc">{description}</p>{/if}
    </div>
    {#if actions}<div class="actions">{@render actions()}</div>{/if}
  </header>

  {#if banner}{@render banner()}{/if}

  {#if facets.length || searchLabel || poster}
    <FilterBar {facets} bind:value={filter} {searchLabel} {placeholder} {summary}>
      {#snippet trailing()}
        {#if poster}
          <SegmentedControl label="View" iconOnly size="sm" bind:value={view}
            options={[{ value: 'table', label: 'Table', icon: 'list' }, { value: 'posters', label: 'Posters', icon: 'library' }]} />
        {/if}
      {/snippet}
    </FilterBar>
  {/if}

  {#if view === 'posters' && poster}
    {#if error && !rows.length}
      <div class="state"><ErrorState title="Couldn't load the {plural}" code={errorCode} {onretry} level={2}>{error}</ErrorState></div>
    {:else}
      <PosterGrid items={posters} label={title} {height} {loading} {hasMore} {loadingMore} {onloadmore} {current}
        onopen={onopen ? (p) => { const r = rows.find((x) => rowKey(x) === p.id); if (r) onopen(r); } : undefined}>
        {#snippet empty()}{@render emptyState()}{/snippet}
      </PosterGrid>
    {/if}
  {:else}
    <DataTable {rows} {columns} {rowKey} label={title} bind:sort bind:selected bind:density {selectable} {cell} {expand} bind:expanded
      {rowActions} {onaction} {onopen} {current} {bulkActions} {noun} {nounPlural} {total} {loading} {error} {errorCode} {onretry}
      {hasMore} {loadingMore} {onloadmore} {height} densitySwitch>
      {#snippet empty()}{@render emptyState()}{/snippet}
    </DataTable>
  {/if}
</div>

<style>
  .lp { display: grid; gap: var(--space-4); min-width: 0; }
  .head { display: flex; align-items: flex-end; justify-content: space-between; gap: var(--space-3) var(--space-4); flex-wrap: wrap; }
  .titles { display: grid; gap: var(--space-1); min-width: 0; }
  h1 { font-size: var(--text-xl); line-height: var(--leading-tight); text-wrap: balance; }
  .desc { font-size: var(--text-md); color: var(--ink-muted); }
  .actions { display: flex; gap: var(--space-2); flex-wrap: wrap; }
  .state { border: var(--border-w) solid var(--line); border-radius: var(--radius-lg); background: var(--surface-raised); padding: var(--space-8) var(--space-4); }
</style>
