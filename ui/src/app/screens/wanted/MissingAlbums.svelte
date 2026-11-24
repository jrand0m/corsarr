<!-- Wanted › Missing › Music: monitored albums that are out with tracks still missing; search one or the selection. -->
<script lang="ts">
  import { onMount } from 'svelte';
  import ListPage from '../../../lib/components/templates/ListPage/ListPage.svelte';
  import Button from '../../../lib/components/atoms/Button/Button.svelte';
  import MediaKindIcon from '../../../lib/components/atoms/MediaKindIcon/MediaKindIcon.svelte';
  import EmptyState from '../../../lib/components/molecules/EmptyState/EmptyState.svelte';
  import type { Column, SortState } from '../../../lib/components/organisms/DataTable/DataTable.svelte';
  import { emptyFilter, type FilterFacet, type FilterState } from '../../../lib/filters/url';
  import { app } from '../../context.svelte';
  import { Resource } from '../../resource.svelte';
  import { refetchOn } from '../../live.svelte';
  import type { Album, SearchSummary } from '../../api/types';
  import { applyList } from '../../list';
  import { albumYear } from '../../music';
  import { count } from '../../tv';
  import KindSwitch from './KindSwitch.svelte';

  interface WantedAlbum { album: Album; artist: { id: number; name: string }; reason: string; files: number }

  const { api, live, toasts, navigate } = app();
  const list = new Resource((signal) => api.all<WantedAlbum>('wanted/missing/albums', { signal }));
  onMount(() => {
    void list.load();
    return refetchOn(live, ['item.*', 'import.completed', 'search.completed', 'file.deleted'], () => void list.load(true), 800);
  });

  const all = $derived(list.data ?? []);
  let filter = $state<FilterState>(emptyFilter());
  let sort = $state<SortState | undefined>({ column: 'year', direction: 'desc' });
  let selected = $state<string[]>([]);
  const facets = $derived<FilterFacet[]>([
    { id: 'artist', label: 'Artist', options: [...new Map(all.map((r) => [String(r.artist.id), r.artist.name]))].map(([value, label]) => ({ value, label })) },
  ]);
  const rows = $derived(applyList(all, filter, sort, {
    text: (r) => `${r.artist.name} ${r.album.title}`,
    facets: { artist: (r) => String(r.artist.id) },
    sortKeys: { artist: (r) => `${r.artist.name} ${r.album.title}`, year: (r) => albumYear(r.album) },
  }));
  const columns: Column<WantedAlbum>[] = [
    { id: 'artist', label: 'Artist', sortable: true, value: (r) => r.artist.name, phone: 'meta' },
    { id: 'album', label: 'Album', value: (r) => r.album.title, phone: 'title' },
    { id: 'year', label: 'Year', width: '5rem', sortable: true, value: (r) => albumYear(r.album) || '—', phone: 'hidden' },
    { id: 'tracks', label: 'Tracks on disk', width: '8rem', value: (r) => (r.album.trackCount ? `${r.files} of ${r.album.trackCount}` : '—'), phone: 'trailing' },
  ];

  let searching = $state(false);
  async function search(ids: number[]) {
    searching = true;
    const results = await Promise.allSettled(ids.map((id) => api.post<SearchSummary>(`albums/${id}/search`)));
    searching = false;
    const grabbed = results.filter((r) => r.status === 'fulfilled' && r.value.grabbed).length;
    const failed = results.filter((r) => r.status === 'rejected').length;
    toasts.push({
      message: `Searched ${count(ids.length, 'album')}: ${grabbed} grabbed`,
      detail: failed ? `${count(failed, 'search', 'searches')} failed; check System › Health.` : undefined,
      tone: failed ? 'warn' : grabbed ? 'ok' : 'info',
      ...(grabbed ? { actionLabel: 'Open queue', onaction: () => navigate('/activity/queue') } : {}),
    });
    void list.load(true);
  }
</script>

{#snippet cell(r: WantedAlbum, c: Column<WantedAlbum>)}
  {#if c.id === 'album'}
    <span class="title"><MediaKindIcon kind="music" /><a href="#/library/music/albums/{r.album.id}">{r.album.title}</a></span>
  {:else}{c.value?.(r)}{/if}
{/snippet}

<ListPage title="Missing" description={all.length ? `${count(all.length, 'monitored album')} out with tracks still missing` : undefined}
  {facets} bind:filter bind:sort bind:selected syncUrl searchLabel="Filter missing albums" placeholder="Filter by artist or album"
  {rows} {columns} rowKey={(r) => String(r.album.id)} {cell} selectable noun="album"
  loading={list.loading && !list.data} error={list.error?.message} errorCode={list.error?.label} onretry={list.retry}
  onopen={(r) => navigate(`/library/music/albums/${r.album.id}`)}
  onaction={(a, r) => (a === 'search' ? search([r.album.id]) : navigate(`/library/music/albums/${r.album.id}`))}
  rowActions={() => [{ id: 'search', label: 'Search now', icon: 'search', hint: 'S' }, { id: 'open', label: 'Open album', icon: 'arrow-right' }]}>
  {#snippet actions()}<KindSwitch current="music" />{/snippet}
  {#snippet bulkActions(keys)}
    <Button size="sm" variant="primary" icon="search" disabled={searching} onclick={() => { void search(keys.map(Number)); selected = []; }}>Search {keys.length}</Button>
  {/snippet}
  {#snippet empty()}
    <EmptyState title="No albums are missing" icon="check-circle" level={2}>
      Monitored albums appear here once they are out and some of their tracks are not on disk yet.
    </EmptyState>
  {/snippet}
</ListPage>

<style>
  .title { display: flex; align-items: center; gap: var(--space-2); min-width: 0; }
  .title a { color: var(--ink); text-decoration: none; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .title a:hover { text-decoration: underline; }
</style>
