<!-- Wanted › Missing › TV: monitored episodes that aired with no file; search one or the selection. -->
<script lang="ts">
  import { onMount } from 'svelte';
  import ListPage from '../../../lib/components/templates/ListPage/ListPage.svelte';
  import Button from '../../../lib/components/atoms/Button/Button.svelte';
  import MediaKindIcon from '../../../lib/components/atoms/MediaKindIcon/MediaKindIcon.svelte';
  import Timestamp from '../../../lib/components/atoms/Timestamp/Timestamp.svelte';
  import EmptyState from '../../../lib/components/molecules/EmptyState/EmptyState.svelte';
  import type { Column, SortState } from '../../../lib/components/organisms/DataTable/DataTable.svelte';
  import { emptyFilter, type FilterFacet, type FilterState } from '../../../lib/filters/url';
  import { app } from '../../context.svelte';
  import { Resource } from '../../resource.svelte';
  import { refetchOn } from '../../live.svelte';
  import { toApiError } from '../../api/client';
  import type { SearchSummary, WantedEpisode } from '../../api/types';
  import { applyList } from '../../list';
  import { count, episodeCode } from '../../tv';
  import KindSwitch from './KindSwitch.svelte';

  const { api, live, toasts, navigate } = app();

  let pages = $state<WantedEpisode[]>([]);
  let cursor = $state<string | undefined>();
  let loadingMore = $state(false);
  const first = new Resource(async (s) => {
    const p = await api.page<WantedEpisode>('wanted/missing/episodes', undefined, 200, undefined, s);
    pages = p.items;
    cursor = p.nextCursor;
    return p;
  });
  async function more() {
    if (!cursor || loadingMore) return;
    loadingMore = true;
    try {
      const p = await api.page<WantedEpisode>('wanted/missing/episodes', cursor, 200);
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
    return refetchOn(live, ['item.*', 'import.completed', 'search.completed', 'file.deleted'], () => void first.load(true), 800);
  });

  let filter = $state<FilterState>(emptyFilter());
  let sort = $state<SortState | undefined>({ column: 'air', direction: 'desc' });
  let selected = $state<string[]>([]);
  const facets = $derived<FilterFacet[]>([
    { id: 'series', label: 'Series', options: [...new Map(pages.map((p) => [String(p.series.id), p.series.title]))].map(([value, label]) => ({ value, label })) },
    { id: 'reason', label: 'Reason', options: [{ value: 'missing', label: 'No file' }, { value: 'cutoff_unmet', label: 'Below cutoff' }] },
  ]);
  const code = (r: WantedEpisode) => episodeCode(r.episode.season, r.episode.number);
  const aired = (r: WantedEpisode) => (r.episode.airAt ? Date.parse(r.episode.airAt) : 0);
  const rows = $derived(applyList(pages, filter, sort, {
    text: (r) => `${r.series.title} ${code(r)} ${r.episode.title}`,
    facets: { series: (r) => String(r.series.id), reason: (r) => r.reason },
    sortKeys: { series: (r) => `${r.series.title} ${code(r)}`, air: aired },
  }));

  const columns: Column<WantedEpisode>[] = [
    { id: 'series', label: 'Series', sortable: true, value: (r) => r.series.title, phone: 'title' },
    { id: 'code', label: 'Episode', width: '6rem', value: code, phone: 'meta' },
    { id: 'title', label: 'Title', value: (r) => r.episode.title || 'To be announced', phone: 'hidden' },
    { id: 'air', label: 'Aired', width: '8rem', sortable: true, value: (r) => r.episode.airAt ?? '—', phone: 'trailing' },
  ];

  let searching = $state(false);
  async function search(ids: number[]) {
    searching = true;
    const results = await Promise.allSettled(ids.map((id) => api.post<SearchSummary>(`episodes/${id}/search`)));
    searching = false;
    const ok = results.filter((r): r is PromiseFulfilledResult<SearchSummary> => r.status === 'fulfilled');
    const grabbed = ok.filter((r) => r.value.grabbed).length;
    const failed = results.length - ok.length;
    toasts.push({
      message: `Searched ${count(ids.length, 'episode')}: ${grabbed} grabbed`,
      detail: failed ? `${count(failed, 'search', 'searches')} failed; check System › Health.` : grabbed < ids.length ? 'The rest had no release good enough yet; corsarr keeps watching the feeds.' : undefined,
      tone: failed ? 'warn' : grabbed ? 'ok' : 'info',
      ...(grabbed ? { actionLabel: 'Open queue', onaction: () => navigate('/activity/queue') } : {}),
    });
    void first.load(true);
  }
</script>

{#snippet cell(r: WantedEpisode, c: Column<WantedEpisode>)}
  {#if c.id === 'series'}
    <span class="title"><MediaKindIcon kind="tv" /><a href="#/library/tv/{r.series.id}">{r.series.title}</a></span>
  {:else if c.id === 'code'}<span class="num">{code(r)}</span>
  {:else if c.id === 'air'}{#if r.episode.airAt}<Timestamp value={r.episode.airAt} />{:else}—{/if}
  {:else}{c.value?.(r)}{/if}
{/snippet}

<ListPage title="Missing" description={pages.length ? `${pages.length}${cursor ? '+' : ''} monitored ${pages.length === 1 && !cursor ? 'episode' : 'episodes'} aired with no file yet` : undefined}
  {facets} bind:filter bind:sort bind:selected syncUrl searchLabel="Filter missing episodes" placeholder="Filter by series or episode"
  {rows} {columns} rowKey={(r) => String(r.episode.id)} {cell} selectable noun="episode"
  loading={first.loading && !first.data} error={first.error?.message} errorCode={first.error?.label} onretry={first.retry}
  hasMore={!!cursor} {loadingMore} onloadmore={more} onopen={(r) => navigate(`/library/tv/${r.series.id}`)}
  onaction={(a, r) => (a === 'search' ? search([r.episode.id]) : navigate(`/library/tv/${r.series.id}`))}
  rowActions={() => [{ id: 'search', label: 'Search now', icon: 'search', hint: 'S' }, { id: 'open', label: 'Open series', icon: 'arrow-right' }]}>
  {#snippet actions()}<KindSwitch current="tv" />{/snippet}
  {#snippet bulkActions(keys)}
    <Button size="sm" variant="primary" icon="search" disabled={searching} onclick={() => { void search(keys.map(Number)); selected = []; }}>Search {keys.length}</Button>
  {/snippet}
  {#snippet empty()}
    <EmptyState title="No episodes are missing" icon="check-circle" level={2}>
      Monitored episodes appear here once they have aired and are not on disk yet.
    </EmptyState>
  {/snippet}
</ListPage>

<style>
  .title { display: flex; align-items: center; gap: var(--space-2); min-width: 0; }
  .title a { color: var(--ink); text-decoration: none; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .title a:hover { text-decoration: underline; }
</style>
