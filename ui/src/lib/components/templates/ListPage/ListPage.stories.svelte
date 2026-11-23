<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import ListPage from './ListPage.svelte';
  import Button from '../../atoms/Button/Button.svelte';
  import StatusBadge from '../../atoms/StatusBadge/StatusBadge.svelte';
  import Quantity from '../../atoms/Quantity/Quantity.svelte';
  import Banner from '../../molecules/Banner/Banner.svelte';
  import EmptyState from '../../molecules/EmptyState/EmptyState.svelte';
  import type { Column } from '../../organisms/DataTable/DataTable.svelte';
  import { libraryRows, type LibraryRow } from '../../../storybook/library-rows';

  const rows = libraryRows(240);
  const columns: Column<LibraryRow>[] = [
    { id: 'title', label: 'Title', sortable: true, value: (r) => r.title, phone: 'title' },
    { id: 'year', label: 'Year', width: '5rem', align: 'end', sortable: true, value: (r) => String(r.year) },
    { id: 'status', label: 'Status', width: '10rem', phone: 'trailing', value: (r) => r.status },
    { id: 'profile', label: 'Profile', width: '6rem', value: (r) => r.profile },
    { id: 'size', label: 'Size', width: '6.5rem', align: 'end', sortable: true, value: (r) => r.size },
  ];
  const facets = [
    { id: 'status', label: 'Status', options: [{ value: 'missing', label: 'Missing', count: 34 }, { value: 'cutoff-unmet', label: 'Below cutoff', count: 34 }, { value: 'cutoff-met', label: 'On disk', count: 103 }] },
    { id: 'profile', label: 'Profile', options: [{ value: 'HD', label: 'HD' }, { value: 'UHD', label: 'UHD' }] },
  ];
  const poster = (r: LibraryRow) => ({ id: r.id, title: r.title, kind: r.kind, year: r.year, status: { family: 'item' as const, value: r.status } });
  const key = (r: LibraryRow) => r.id;
  const phone = { viewport: { value: 'phone', isRotated: false } };
  const desktop = { viewport: { value: 'desktop', isRotated: false } };

  const { Story } = defineMeta({
    title: 'Templates/ListPage',
    component: ListPage,
    tags: ['autodocs'],
    parameters: { layout: 'padded' },
  });
</script>

{#snippet cell(r: LibraryRow, c: Column<LibraryRow>)}
  {#if c.id === 'status'}<StatusBadge family="item" value={r.status} size="sm" appearance="plain" />
  {:else if c.id === 'size'}{#if r.size}<Quantity kind="bytes" value={r.size} />{:else}—{/if}
  {:else}{c.value?.(r)}{/if}
{/snippet}
{#snippet add()}<Button variant="primary" icon="plus">Add movie</Button>{/snippet}

<Story name="Playground" asChild globals={desktop}>
  <ListPage title="Movies" description="240 movies · 9.8 TB" actions={add} {facets} searchLabel="Filter movies" {rows} {columns} rowKey={key} {cell} {poster} noun="movie" selectable height="28rem" />
</Story>

<Story name="Posters" asChild globals={desktop}>
  <ListPage title="Movies" actions={add} {facets} searchLabel="Filter movies" {rows} {columns} rowKey={key} {cell} {poster} view="posters" noun="movie" height="28rem" />
</Story>

<Story name="Selection with bulk bar" asChild globals={{ ...desktop, theme: 'dark' }}>
  <ListPage title="Movies" actions={add} {facets} searchLabel="Filter movies" {rows} {columns} rowKey={key} {cell} selectable selected={['item-1', 'item-4']} noun="movie" height="24rem">
    {#snippet bulkActions(keys)}<Button size="sm" variant="primary" icon="search">Search {keys.length}</Button><Button size="sm" icon="pencil">Edit</Button>{/snippet}
  </ListPage>
</Story>

<Story name="Loading" asChild globals={desktop}>
  <ListPage title="Movies" {facets} searchLabel="Filter movies" rows={[]} {columns} rowKey={key} loading noun="movie" height="22rem" />
</Story>

<Story name="Empty kind" asChild globals={desktop}>
  <ListPage title="Movies" actions={add} rows={[]} {columns} rowKey={key} noun="movie" height="22rem">
    {#snippet empty()}
      <EmptyState title="No movies yet" icon="film" level={2}>Add a movie, import a folder you already have, or migrate from your current setup.
        {#snippet actions()}<Button variant="primary" icon="plus">Add movie</Button><Button icon="folder">Import from folder</Button>{/snippet}
      </EmptyState>
    {/snippet}
  </ListPage>
</Story>

<Story name="Filtered to nothing" asChild globals={desktop}>
  <ListPage title="Movies" {facets} searchLabel="Filter movies" filter={{ q: 'zebra', facets: { status: ['missing'] } }} rows={[]} {columns} rowKey={key} noun="movie" height="22rem" />
</Story>

<Story name="Error" asChild globals={{ ...desktop, theme: 'dark' }}>
  <ListPage title="Movies" {facets} searchLabel="Filter movies" rows={[]} {columns} rowKey={key} noun="movie" error="The server did not answer in 10 s." errorCode="HTTP 504 · api.timeout" onretry={() => {}} height="22rem" />
</Story>

<Story name="Stale banner, phone" asChild globals={phone}>
  <ListPage title="Movies" actions={add} {facets} searchLabel="Filter movies" rows={rows.slice(0, 30)} {columns} rowKey={key} {cell} {poster} noun="movie" height="30rem">
    {#snippet banner()}<Banner preset="stale">Live updates are paused; this list was loaded at 14:05.</Banner>{/snippet}
  </ListPage>
</Story>
