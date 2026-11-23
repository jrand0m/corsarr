<!-- Story helper: a DataTable wired like a page would wire it (local sort, cursor pages, bulk actions). -->
<script lang="ts">
  import DataTable, { type Column, type SortState } from '../components/organisms/DataTable/DataTable.svelte';
  import StatusBadge from '../components/atoms/StatusBadge/StatusBadge.svelte';
  import MediaKindIcon from '../components/atoms/MediaKindIcon/MediaKindIcon.svelte';
  import Quantity from '../components/atoms/Quantity/Quantity.svelte';
  import Timestamp from '../components/atoms/Timestamp/Timestamp.svelte';
  import Button from '../components/atoms/Button/Button.svelte';
  import type { Density } from '../tokens';
  import { libraryRows, libraryNow, type LibraryRow } from './library-rows';

  interface Props {
    count?: number;
    pageSize?: number;
    density?: Density;
    mode?: 'auto' | 'table' | 'list';
    height?: string;
    selectable?: boolean;
  }
  let { count = 10_000, pageSize = 100, density = 'comfortable', mode = 'auto', height = '28rem', selectable = true }: Props = $props();

  // svelte-ignore state_referenced_locally
  let all = $state(libraryRows(count));
  // svelte-ignore state_referenced_locally
  let loaded = $state(Math.min(pageSize, count));
  let loadingMore = $state(false);
  let sort = $state<SortState>({ column: 'title', direction: 'asc' });
  let current = $state<string>();

  const columns: Column<LibraryRow>[] = [
    { id: 'title', label: 'Title', sortable: true, value: (r) => r.title, phone: 'title' },
    { id: 'kind', label: 'Kind', width: '4.5rem', phone: 'hidden', value: (r) => r.kind },
    { id: 'status', label: 'Status', width: '10rem', phone: 'trailing', value: (r) => r.status },
    { id: 'profile', label: 'Profile', width: '6rem', sortable: true, value: (r) => r.profile },
    { id: 'episodes', label: 'Units', width: '6rem', align: 'end', value: (r) => `${r.have} of ${r.total}` },
    { id: 'size', label: 'Size', width: '6.5rem', align: 'end', sortable: true, value: (r) => r.size },
    { id: 'added', label: 'Added', width: '7rem', align: 'end', sortable: true, phone: 'hidden', value: (r) => r.added },
  ];
  const sorted = $derived.by(() => {
    const dir = sort.direction === 'asc' ? 1 : -1;
    const key = sort.column as keyof LibraryRow;
    return [...all].sort((a, b) => (a[key] < b[key] ? -dir : a[key] > b[key] ? dir : 0));
  });
  const rows = $derived(sorted.slice(0, loaded));
  function more() {
    loadingMore = true;
    setTimeout(() => { loaded = Math.min(all.length, loaded + pageSize); loadingMore = false; }, 400);
  }
</script>

<DataTable
  {rows} {columns} rowKey={(r) => r.id} label="Library" noun="item" total={all.length}
  bind:sort onsort={() => (loaded = Math.min(pageSize, all.length))}
  {selectable} densitySwitch {density} {mode} {height} {current}
  hasMore={loaded < all.length} {loadingMore} onloadmore={more}
  onopen={(r) => (current = r.id)}
  rowActions={() => [{ id: 'search', label: 'Search now', icon: 'search' }, { id: 'edit', label: 'Edit', icon: 'pencil' }, { id: 'delete', label: 'Delete…', icon: 'trash', danger: true, group: 'd' }]}
>
  {#snippet cell(row, col)}
    {#if col.id === 'status'}<StatusBadge family="item" value={row.status} size="sm" appearance="plain" />
    {:else if col.id === 'kind'}<MediaKindIcon kind={row.kind} />
    {:else if col.id === 'size'}{#if row.size}<Quantity kind="bytes" value={row.size} />{:else}—{/if}
    {:else if col.id === 'episodes'}<Quantity kind="count" value={row.have} limit={row.total} />
    {:else if col.id === 'added'}<Timestamp value={row.added} now={new Date(libraryNow)} />
    {:else}{col.value?.(row)}{/if}
  {/snippet}
  {#snippet bulkActions(keys)}
    <Button size="sm" variant="primary" icon="search">Search {keys.length}</Button>
    <Button size="sm" icon="pencil">Edit</Button>
  {/snippet}
</DataTable>
