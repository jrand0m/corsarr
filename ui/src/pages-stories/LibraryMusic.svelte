<!-- Page composition (story only): Library › Music, the artist table. -->
<script lang="ts">
  import Artwork from '../lib/components/atoms/Artwork/Artwork.svelte';
  import Shell from './Shell.svelte';
  import ListPage from '../lib/components/templates/ListPage/ListPage.svelte';
  import Button from '../lib/components/atoms/Button/Button.svelte';
  import Progress from '../lib/components/atoms/Progress/Progress.svelte';
  import Quantity from '../lib/components/atoms/Quantity/Quantity.svelte';
  import Timestamp from '../lib/components/atoms/Timestamp/Timestamp.svelte';
  import EmptyState from '../lib/components/molecules/EmptyState/EmptyState.svelte';
  import MonitorToggle from '../lib/components/molecules/MonitorToggle/MonitorToggle.svelte';
  import type { Column } from '../lib/components/organisms/DataTable/DataTable.svelte';
  import { artSwatch, nowDate } from '../lib/storybook/fixtures';
  import { artistRows, type ArtistRow } from '../lib/storybook/music-fixtures';

  let { selected = [], empty = false }: { selected?: string[]; empty?: boolean } = $props();

  // svelte-ignore state_referenced_locally
  const rows = empty ? [] : artistRows;
  const columns: Column<ArtistRow>[] = [
    { id: 'monitored', label: 'Monitored', width: '3rem', hideLabel: true, phone: 'hidden' },
    { id: 'name', label: 'Artist', sortable: true, value: (r) => r.name, phone: 'title' },
    { id: 'profile', label: 'Quality', width: '7rem', phone: 'hidden', value: (r) => r.profile },
    { id: 'metadata', label: 'Metadata', width: '9rem', phone: 'hidden', value: (r) => r.metadata },
    { id: 'albums', label: 'Albums', width: '9rem', sortable: true, phone: 'trailing', value: (r) => `${r.withFiles}/${r.monitored} albums` },
    { id: 'size', label: 'Size', width: '6rem', align: 'end', sortable: true, phone: 'hidden', value: (r) => r.size },
    { id: 'next', label: 'Next release', width: '8rem', sortable: true, phone: 'meta', value: (r) => (r.nextRelease ? String(r.nextRelease) : '—') },
  ];
  const facets = [
    { id: 'monitored', label: 'Monitoring', multiple: false, options: [{ value: 'yes', label: 'Monitored' }, { value: 'no', label: 'Unmonitored' }] },
    { id: 'profile', label: 'Quality profile', options: [{ value: 'standard', label: 'Standard', count: 6 }, { value: 'lossless', label: 'Lossless', count: 4 }] },
    { id: 'metadata', label: 'Metadata profile', options: [{ value: 'standard', label: 'Standard', count: 7 }, { value: 'standardsingles', label: 'Standard + Singles', count: 3 }] },
  ];
</script>

{#snippet cell(r: ArtistRow, c: Column<ArtistRow>)}
  {#if c.id === 'monitored'}<MonitorToggle item={r.name} state={r.watched ? 'monitored' : 'unmonitored'} size="sm" />
  {:else if c.id === 'name'}<span class="artist"><Artwork src={rows.indexOf(r) % 4 === 3 ? null : artSwatch((rows.indexOf(r) * 61) % 360, 'cover')} title={r.name} kind="music" shape="cover" width={32} />{r.name}{#if r.disambiguation}{' '}({r.disambiguation}){/if}</span>
  {:else if c.id === 'albums'}<Progress label="Albums of {r.name} on disk" done={r.withFiles} total={r.monitored} size="sm" showValue tone={r.withFiles >= r.monitored ? 'ok' : 'warn'} />
  {:else if c.id === 'size'}{#if r.size}<Quantity kind="bytes" value={r.size} />{:else}—{/if}
  {:else if c.id === 'next'}{#if r.nextRelease}<Timestamp value={r.nextRelease} now={nowDate} />{:else}—{/if}
  {:else}{c.value?.(r)}{/if}
{/snippet}

<Shell path="/library/music">
  <ListPage title="Music" description={rows.length ? '10 artists · 41 albums on disk' : undefined} {facets} searchLabel="Filter artists" placeholder="Filter by name"
    summary="{rows.length} artists" {rows} {columns} rowKey={(r) => r.id} {cell} {selected} selectable noun="artist" nounPlural="artists"
    rowActions={() => [{ id: 'open', label: 'Open', icon: 'arrow-right' }, { id: 'search', label: 'Search wanted albums', icon: 'search' }, { id: 'delete', label: 'Delete…', icon: 'trash', danger: true, group: 'd' }]}>
    {#snippet actions()}
      <Button variant="primary" icon="plus">Add artist</Button>
    {/snippet}
    {#snippet bulkActions(keys)}
      <Button size="sm" variant="primary" icon="search">Search {keys.length}</Button>
    {/snippet}
    {#snippet empty()}
      <EmptyState title="No artists yet" icon="music" level={2}>
        Add an artist by name or MusicBrainz id; corsarr keeps their discography and fetches the albums you monitor.
        {#snippet actions()}<Button variant="primary" icon="plus">Add artist</Button>{/snippet}
      </EmptyState>
    {/snippet}
  </ListPage>
</Shell>

<style>
  .artist { display: flex; align-items: center; gap: var(--space-2); min-width: 0; }
</style>
