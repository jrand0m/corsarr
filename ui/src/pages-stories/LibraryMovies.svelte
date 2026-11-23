<!-- Page composition (story only): Library › Movies, table or poster view. -->
<script lang="ts">
  import Shell from './Shell.svelte';
  import ListPage from '../lib/components/templates/ListPage/ListPage.svelte';
  import Button from '../lib/components/atoms/Button/Button.svelte';
  import StatusBadge from '../lib/components/atoms/StatusBadge/StatusBadge.svelte';
  import Quantity from '../lib/components/atoms/Quantity/Quantity.svelte';
  import MonitorToggle from '../lib/components/molecules/MonitorToggle/MonitorToggle.svelte';
  import type { Column } from '../lib/components/organisms/DataTable/DataTable.svelte';
  import type { ListView } from '../lib/route/hash';
  import { movies, type MovieRow } from '../lib/storybook/fixtures';

  let { view = 'table', selected = [] }: { view?: ListView; selected?: string[] } = $props();

  const columns: Column<MovieRow>[] = [
    { id: 'monitored', label: 'Monitored', width: '3rem', hideLabel: true, phone: 'hidden' },
    { id: 'title', label: 'Title', sortable: true, value: (r) => r.title, phone: 'title' },
    { id: 'year', label: 'Year', width: '4.5rem', align: 'end', sortable: true, value: (r) => String(r.year) },
    { id: 'status', label: 'Status', width: '9.5rem', phone: 'trailing', value: (r) => r.status },
    { id: 'quality', label: 'Quality', width: '8.5rem', value: (r) => r.quality },
    { id: 'profile', label: 'Profile', width: '5rem', phone: 'hidden', value: (r) => r.profile },
    { id: 'size', label: 'Size', width: '6rem', align: 'end', sortable: true, phone: 'hidden', value: (r) => r.size },
  ];
  const facets = [
    { id: 'status', label: 'Status', options: [
      { value: 'missing', label: 'Missing', count: 17 }, { value: 'cutoff-unmet', label: 'Below cutoff', count: 17 },
      { value: 'cutoff-met', label: 'On disk', count: 52 }, { value: 'not-available', label: 'Not available yet', count: 17 },
    ] },
    { id: 'monitored', label: 'Monitoring', multiple: false, options: [{ value: 'yes', label: 'Monitored' }, { value: 'no', label: 'Unmonitored' }] },
    { id: 'profile', label: 'Profile', options: [{ value: 'HD', label: 'HD', count: 90 }, { value: 'UHD', label: 'UHD', count: 30 }] },
    { id: 'tag', label: 'Tag', options: [{ value: 'kids', label: 'kids' }, { value: 'classics', label: 'classics' }] },
  ];
  const poster = (r: MovieRow) => ({ id: r.id, title: r.title, kind: 'movie' as const, year: r.year, status: { family: 'item' as const, value: r.status } });
</script>

{#snippet cell(r: MovieRow, c: Column<MovieRow>)}
  {#if c.id === 'monitored'}<MonitorToggle item={r.title} state={r.monitored ? 'monitored' : 'unmonitored'} size="sm" />
  {:else if c.id === 'status'}<StatusBadge family="item" value={r.status} size="sm" appearance="plain" />
  {:else if c.id === 'size'}{#if r.size}<Quantity kind="bytes" value={r.size} />{:else}—{/if}
  {:else}{c.value?.(r)}{/if}
{/snippet}

<Shell path="/library/movies">
  <ListPage title="Movies" description="120 movies · 2.6 TB on disk" {facets} searchLabel="Filter movies" placeholder="Filter by title"
    summary="120 movies" rows={movies} {columns} rowKey={(r) => r.id} {cell} {poster} {view} {selected} selectable noun="movie"
    rowActions={() => [{ id: 'search', label: 'Search now', icon: 'search' }, { id: 'edit', label: 'Edit', icon: 'pencil' }, { id: 'delete', label: 'Delete…', icon: 'trash', danger: true, group: 'd' }]}>
    {#snippet actions()}
      <Button icon="folder">Import</Button>
      <Button variant="primary" icon="plus">Add movie</Button>
    {/snippet}
    {#snippet bulkActions(keys)}
      <Button size="sm" variant="primary" icon="search">Search {keys.length}</Button>
      <Button size="sm" icon="pencil">Edit {keys.length}</Button>
    {/snippet}
  </ListPage>
</Shell>
