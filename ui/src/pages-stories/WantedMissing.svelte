<!-- Page composition (story only): Wanted › Missing, with search selected and search all
     behind a budget estimate. -->
<script lang="ts">
  import Shell from './Shell.svelte';
  import ListPage from '../lib/components/templates/ListPage/ListPage.svelte';
  import Button from '../lib/components/atoms/Button/Button.svelte';
  import MediaKindIcon from '../lib/components/atoms/MediaKindIcon/MediaKindIcon.svelte';
  import Timestamp from '../lib/components/atoms/Timestamp/Timestamp.svelte';
  import EmptyState from '../lib/components/molecules/EmptyState/EmptyState.svelte';
  import ConfirmDialog from '../lib/components/organisms/ConfirmDialog/ConfirmDialog.svelte';
  import type { Column } from '../lib/components/organisms/DataTable/DataTable.svelte';
  import { wantedRows, nowDate, type WantedRow } from '../lib/storybook/fixtures';

  interface Props { empty?: boolean; selected?: string[]; searchAllOpen?: boolean }
  let { empty = false, selected = [], searchAllOpen = false }: Props = $props();

  // svelte-ignore state_referenced_locally
  let confirm = $state(searchAllOpen);
  // svelte-ignore state_referenced_locally
  const rows = empty ? [] : wantedRows;

  const columns: Column<WantedRow>[] = [
    { id: 'title', label: 'Item', value: (r) => r.title, phone: 'title' },
    { id: 'profile', label: 'Profile', width: '5rem', value: (r) => r.profile },
    { id: 'available', label: 'Available', width: '7rem', value: (r) => r.availableSince, phone: 'meta' },
    { id: 'searched', label: 'Last searched', width: '8rem', value: (r) => r.lastSearched, phone: 'hidden' },
    { id: 'next', label: 'Next search', width: '7rem', value: (r) => r.nextEligible, phone: 'hidden' },
    { id: 'result', label: 'Last result', width: '17rem', value: (r) => r.lastResult, phone: 'hidden' },
  ];
  const facets = [
    { id: 'kind', label: 'Kind', options: [{ value: 'tv', label: 'TV', count: 14 }, { value: 'movie', label: 'Movies', count: 21 }, { value: 'music', label: 'Music', count: 5 }] },
    { id: 'profile', label: 'Profile', options: [{ value: 'HD', label: 'HD' }, { value: 'UHD', label: 'UHD' }] },
  ];
</script>

{#snippet cell(r: WantedRow, c: Column<WantedRow>)}
  {#if c.id === 'title'}
    <span class="title"><MediaKindIcon kind={r.kind} /><span class="t">{#if r.parent}<span class="parent">{r.parent}</span>{/if}<span>{r.title}</span></span></span>
  {:else if c.id === 'available'}<Timestamp value={r.availableSince} now={nowDate} />
  {:else if c.id === 'searched'}{#if r.lastSearched}<Timestamp value={r.lastSearched} now={nowDate} />{:else}Never{/if}
  {:else if c.id === 'next'}{#if r.nextEligible}<Timestamp value={r.nextEligible} now={nowDate} />{:else}—{/if}
  {:else}{c.value?.(r)}{/if}
{/snippet}

<Shell path="/wanted/missing">
  <ListPage title="Missing" description="{rows.length} monitored items with no file yet" {facets} searchLabel="Filter wanted" {rows} {columns}
    rowKey={(r) => r.id} {cell} {selected} selectable noun="item"
    rowActions={() => [{ id: 'search', label: 'Search now', icon: 'search', hint: 'S' }, { id: 'unmonitor', label: 'Stop monitoring', icon: 'eye-off' }]}>
    {#snippet actions()}{#if rows.length}<Button variant="primary" icon="search" onclick={() => (confirm = true)}>Search all</Button>{/if}{/snippet}
    {#snippet bulkActions(keys)}
      <Button size="sm" variant="primary" icon="search">Search {keys.length}</Button>
      <Button size="sm" icon="eye-off">Stop monitoring</Button>
    {/snippet}
    {#snippet empty()}
      <EmptyState title="Everything wanted is in the library" icon="check-circle" level={2}>
        New episodes and releases appear here when they become available and are not on disk yet.
      </EmptyState>
    {/snippet}
  </ListPage>

  <ConfirmDialog bind:open={confirm} variant="primary" title="Search all 40 missing items?" confirmLabel="Search 40 items" undoable={false}
    consequences={['40 items × 4 indexers ≈ 160 queries', 'About 12 min at the current limits; feed sync keeps running meanwhile', 'Accepted releases are grabbed straight away']}
    onconfirm={() => (confirm = false)} oncancel={() => (confirm = false)} />
</Shell>

<style>
  .title { display: flex; align-items: center; gap: var(--space-2); min-width: 0; }
  .t { display: flex; gap: var(--space-1-5); min-width: 0; overflow: hidden; white-space: nowrap; }
  .t span { overflow: hidden; text-overflow: ellipsis; }
  .parent { color: var(--ink-muted); flex: none; }
</style>
