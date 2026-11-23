<!-- Wanted › Missing: monitored, available movies with no file; search one, the selection or all. -->
<script lang="ts">
  import { onMount } from 'svelte';
  import ListPage from '../../../lib/components/templates/ListPage/ListPage.svelte';
  import Button from '../../../lib/components/atoms/Button/Button.svelte';
  import MediaKindIcon from '../../../lib/components/atoms/MediaKindIcon/MediaKindIcon.svelte';
  import Timestamp from '../../../lib/components/atoms/Timestamp/Timestamp.svelte';
  import EmptyState from '../../../lib/components/molecules/EmptyState/EmptyState.svelte';
  import ConfirmDialog from '../../../lib/components/organisms/ConfirmDialog/ConfirmDialog.svelte';
  import type { Column, SortState } from '../../../lib/components/organisms/DataTable/DataTable.svelte';
  import { emptyFilter, type FilterFacet, type FilterState } from '../../../lib/filters/url';
  import { app, profileName } from '../../context.svelte';
  import { Resource } from '../../resource.svelte';
  import { refetchOn } from '../../live.svelte';
  import { toApiError } from '../../api/client';
  import type { Movie, SearchSummary, WantedEntry } from '../../api/types';
  import { availableAt } from '../../model';
  import { applyList } from '../../list';
  import KindSwitch from './KindSwitch.svelte';

  const { api, live, movies, profiles, status, toasts, navigate } = app();
  const showTv = $derived(status.data?.enabledKinds?.includes('tv') ?? false);

  let pages = $state<WantedEntry[]>([]);
  let cursor = $state<string | undefined>();
  let loadingMore = $state(false);
  const first = new Resource(async (s) => {
    const p = await api.page<WantedEntry>('wanted/missing', undefined, 200, undefined, s);
    pages = p.items;
    cursor = p.nextCursor;
    return p;
  });
  async function more() {
    if (!cursor || loadingMore) return;
    loadingMore = true;
    try {
      const p = await api.page<WantedEntry>('wanted/missing', cursor, 200);
      pages = [...pages, ...p.items];
      cursor = p.nextCursor;
    } catch (e) {
      toasts.push({ message: "Couldn't load more", detail: toApiError(e).message, tone: 'error' });
    } finally {
      loadingMore = false;
    }
  }

  onMount(() => {
    void first.load();
    if (!profiles.data) void profiles.load();
    const off = refetchOn(live, ['item.*', 'import.completed', 'search.completed', 'file.deleted'], () => void first.load(true), 800);
    return off;
  });

  let filter = $state<FilterState>(emptyFilter());
  let sort = $state<SortState | undefined>({ column: 'available', direction: 'desc' });
  let selected = $state<string[]>([]);
  const facets = $derived<FilterFacet[]>([
    { id: 'profile', label: 'Profile', options: (profiles.data ?? []).map((p) => ({ value: p.key, label: p.name })) },
  ]);
  const since = (m: Movie) => availableAt(m) ?? 0;
  const rows = $derived(applyList(pages, filter, sort, {
    text: (e) => `${e.movie.title} ${e.movie.year}`,
    facets: { profile: (e) => e.movie.profileKey },
    sortKeys: { title: (e) => e.movie.title, available: (e) => since(e.movie), searched: (e) => Date.parse(e.search?.lastSearched ?? '') || 0 },
  }));

  const columns: Column<WantedEntry>[] = [
    { id: 'title', label: 'Item', sortable: true, value: (r) => r.movie.title, phone: 'title' },
    { id: 'year', label: 'Year', width: '4.5rem', align: 'end', value: (r) => (r.movie.year ? String(r.movie.year) : '—') },
    { id: 'profile', label: 'Profile', width: '7rem', value: (r) => profileName(profiles.data, r.movie.profileKey), phone: 'hidden' },
    { id: 'available', label: 'Available', width: '8rem', sortable: true, value: (r) => String(since(r.movie)), phone: 'meta' },
    { id: 'searched', label: 'Last searched', width: '8.5rem', sortable: true, value: (r) => r.search?.lastSearched ?? '', phone: 'hidden' },
    { id: 'next', label: 'Next search', width: '8rem', value: (r) => r.search?.nextEligible ?? '', phone: 'hidden' },
  ];

  let searching = $state(false);
  async function search(ids: number[]) {
    searching = true;
    const results = await Promise.allSettled(ids.map((id) => api.post<SearchSummary>(`movies/${id}/search`)));
    searching = false;
    const ok = results.filter((r): r is PromiseFulfilledResult<SearchSummary> => r.status === 'fulfilled');
    const grabbed = ok.filter((r) => r.value.grabbed).length;
    const failed = results.length - ok.length;
    toasts.push({
      message: `Searched ${ids.length} ${ids.length === 1 ? 'movie' : 'movies'}: ${grabbed} grabbed`,
      detail: failed ? `${failed} search${failed === 1 ? '' : 'es'} failed; check System › Health.` : grabbed < ids.length ? 'The rest had no release good enough yet; corsarr keeps watching the feeds.' : undefined,
      tone: failed ? 'warn' : grabbed ? 'ok' : 'info',
      ...(grabbed ? { actionLabel: 'Open queue', onaction: () => navigate('/activity/queue') } : {}),
    });
    void first.load(true);
  }

  async function unmonitor(ids: number[]) {
    try {
      for (const id of ids) movies.upsert(await api.put<Movie>(`movies/${id}`, { monitored: false }));
      toasts.push({ message: `Stopped monitoring ${ids.length} ${ids.length === 1 ? 'movie' : 'movies'}`, tone: 'ok',
        onundo: () => { void Promise.all(ids.map((id) => api.put(`movies/${id}`, { monitored: true }))).then(() => first.load(true)); } });
      void first.load(true);
    } catch (e) {
      toasts.push({ message: 'Monitoring was not changed', detail: toApiError(e).message, tone: 'error' });
    }
  }

  let confirmAll = $state(false);
  const total = $derived(pages.length + (cursor ? 1 : 0));
</script>

{#snippet cell(r: WantedEntry, c: Column<WantedEntry>)}
  {#if c.id === 'title'}
    <span class="title"><MediaKindIcon kind="movie" /><a href="#/library/movies/{r.movie.id}">{r.movie.title}</a></span>
  {:else if c.id === 'available'}
    {#if since(r.movie)}<Timestamp value={since(r.movie)} />{:else}Announced{/if}
  {:else if c.id === 'searched'}
    {#if r.search?.lastSearched}<Timestamp value={r.search.lastSearched} />{:else}Never{/if}
  {:else if c.id === 'next'}
    {#if r.search?.nextEligible && Date.parse(r.search.nextEligible) > Date.now()}<Timestamp value={r.search.nextEligible} />{:else}Next backlog run{/if}
  {:else}{c.value?.(r)}{/if}
{/snippet}

<ListPage title="Missing" description={pages.length ? `${pages.length}${cursor ? '+' : ''} monitored movies with no file yet` : undefined} {facets} bind:filter bind:sort bind:selected syncUrl
  searchLabel="Filter wanted" placeholder="Filter by title" {rows} {columns} rowKey={(r) => String(r.movie.id)} {cell} selectable noun="movie"
  loading={first.loading && !first.data} error={first.error?.message} errorCode={first.error?.label} onretry={first.retry}
  hasMore={!!cursor} {loadingMore} onloadmore={more} onopen={(r) => navigate(`/library/movies/${r.movie.id}`)}
  onaction={(a, r) => (a === 'search' ? search([r.movie.id]) : unmonitor([r.movie.id]))}
  rowActions={() => [{ id: 'search', label: 'Search now', icon: 'search', hint: 'S' }, { id: 'unmonitor', label: 'Stop monitoring', icon: 'eye-off' }]}>
  {#snippet actions()}{#if showTv}<KindSwitch current="movie" />{/if}{#if pages.length}<Button variant="primary" icon="search" loading={searching} disabled={searching} onclick={() => (confirmAll = true)}>Search all</Button>{/if}{/snippet}
  {#snippet bulkActions(keys)}
    <Button size="sm" variant="primary" icon="search" onclick={() => { void search(keys.map(Number)); selected = []; }}>Search {keys.length}</Button>
    <Button size="sm" icon="eye-off" onclick={() => { void unmonitor(keys.map(Number)); selected = []; }}>Stop monitoring</Button>
  {/snippet}
  {#snippet empty()}
    <EmptyState title="Everything wanted is in the library" icon="check-circle" level={2}>
      Movies appear here when they become available and are not on disk yet.
    </EmptyState>
  {/snippet}
</ListPage>

<ConfirmDialog bind:open={confirmAll} variant="primary" title="Search all {total} missing movies?" confirmLabel="Search {pages.length} movies" undoable={false}
  consequences={[`One search per movie across every indexer (${pages.length} searches)`, 'Accepted releases are grabbed straight away', 'Feed sync keeps running meanwhile']}
  onconfirm={() => { confirmAll = false; void search(pages.map((p) => p.movie.id)); }} oncancel={() => (confirmAll = false)} />

<style>
  .title { display: flex; align-items: center; gap: var(--space-2); min-width: 0; }
  .title a { color: var(--ink); text-decoration: none; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .title a:hover { text-decoration: underline; }
</style>
