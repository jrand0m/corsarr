<!-- Page composition (story only): Library › TV, table or poster view. -->
<script lang="ts">
  import Shell from './Shell.svelte';
  import ListPage from '../lib/components/templates/ListPage/ListPage.svelte';
  import Button from '../lib/components/atoms/Button/Button.svelte';
  import Progress from '../lib/components/atoms/Progress/Progress.svelte';
  import Quantity from '../lib/components/atoms/Quantity/Quantity.svelte';
  import Timestamp from '../lib/components/atoms/Timestamp/Timestamp.svelte';
  import EmptyState from '../lib/components/molecules/EmptyState/EmptyState.svelte';
  import MonitorToggle from '../lib/components/molecules/MonitorToggle/MonitorToggle.svelte';
  import type { Column } from '../lib/components/organisms/DataTable/DataTable.svelte';
  import type { ListView } from '../lib/route/hash';
  import { nowDate } from '../lib/storybook/fixtures';
  import { seriesRows, type SeriesRow } from '../lib/storybook/tv-fixtures';

  let { view = 'table', selected = [], empty = false }: { view?: ListView; selected?: string[]; empty?: boolean } = $props();

  // svelte-ignore state_referenced_locally
  const rows = empty ? [] : seriesRows;
  const statusLabel = { continuing: 'Continuing', ended: 'Ended', upcoming: 'Upcoming' };
  const columns: Column<SeriesRow>[] = [
    { id: 'monitored', label: 'Monitored', width: '3rem', hideLabel: true, phone: 'hidden' },
    { id: 'title', label: 'Title', sortable: true, value: (r) => `${r.title} (${r.year})`, phone: 'title' },
    { id: 'network', label: 'Network', width: '8rem', sortable: true, phone: 'hidden', value: (r) => r.network },
    { id: 'status', label: 'Status', width: '7rem', phone: 'hidden', value: (r) => statusLabel[r.status] },
    { id: 'profile', label: 'Profile', width: '5rem', phone: 'hidden', value: (r) => r.profile },
    { id: 'episodes', label: 'Episodes', width: '9rem', sortable: true, phone: 'trailing', value: (r) => `${r.files}/${r.episodes} episodes` },
    { id: 'size', label: 'Size', width: '6rem', align: 'end', sortable: true, phone: 'hidden', value: (r) => r.size },
    { id: 'next', label: 'Next airing', width: '8rem', sortable: true, phone: 'meta', value: (r) => (r.nextAiring ? String(r.nextAiring) : '—') },
  ];
  const facets = [
    { id: 'status', label: 'Status', options: [
      { value: 'continuing', label: 'Continuing', count: 6 }, { value: 'ended', label: 'Ended', count: 5 }, { value: 'upcoming', label: 'Upcoming', count: 3 },
    ] },
    { id: 'monitored', label: 'Monitoring', multiple: false, options: [{ value: 'yes', label: 'Monitored' }, { value: 'no', label: 'Unmonitored' }] },
    { id: 'profile', label: 'Profile', options: [{ value: 'HD', label: 'HD', count: 10 }, { value: 'UHD', label: 'UHD', count: 4 }] },
  ];
  const complete = (r: SeriesRow) => r.files >= r.episodes;
  const poster = (r: SeriesRow) => ({
    id: r.id, title: r.title, kind: 'tv' as const, year: r.year, detail: `${r.files} of ${r.episodes} episodes`,
    status: { family: 'item' as const, value: !r.episodes ? 'not-available' : complete(r) ? 'cutoff-met' : 'missing' },
  });
</script>

{#snippet cell(r: SeriesRow, c: Column<SeriesRow>)}
  {#if c.id === 'monitored'}<MonitorToggle item={r.title} state={r.monitored ? 'monitored' : 'unmonitored'} size="sm" />
  {:else if c.id === 'episodes'}<Progress label="Episodes of {r.title} on disk" done={r.files} total={r.episodes} size="sm" showValue tone={complete(r) ? 'ok' : 'warn'} />
  {:else if c.id === 'size'}{#if r.size}<Quantity kind="bytes" value={r.size} />{:else}—{/if}
  {:else if c.id === 'next'}{#if r.nextAiring}<Timestamp value={r.nextAiring} now={nowDate} />{:else}—{/if}
  {:else}{c.value?.(r)}{/if}
{/snippet}

<Shell path="/library/tv">
  <ListPage title="TV" description={rows.length ? '14 series · 312 episodes on disk' : undefined} {facets} searchLabel="Filter series" placeholder="Filter by title or network"
    summary="{rows.length} series" {rows} {columns} rowKey={(r) => r.id} {cell} {poster} {view} {selected} selectable noun="series" nounPlural="series"
    rowActions={() => [{ id: 'open', label: 'Open', icon: 'arrow-right' }, { id: 'search', label: 'Search missing episodes', icon: 'search' }, { id: 'delete', label: 'Delete…', icon: 'trash', danger: true, group: 'd' }]}>
    {#snippet actions()}
      <Button variant="primary" icon="plus">Add series</Button>
    {/snippet}
    {#snippet bulkActions(keys)}
      <Button size="sm" variant="primary" icon="search">Search {keys.length}</Button>
    {/snippet}
    {#snippet empty()}
      <EmptyState title="No series yet" icon="tv" level={2}>
        Add a series by name or provider id; corsarr tracks its episodes and fetches the ones you monitor as they air.
        {#snippet actions()}<Button variant="primary" icon="plus">Add series</Button>{/snippet}
      </EmptyState>
    {/snippet}
  </ListPage>
</Shell>
