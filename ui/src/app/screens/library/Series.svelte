<!-- Library › TV: series as a table or posters, filters, sort and view in the URL; the add dialog at …/add. -->
<script lang="ts">
  import { onMount } from 'svelte';
  import ListPage from '../../../lib/components/templates/ListPage/ListPage.svelte';
  import Button from '../../../lib/components/atoms/Button/Button.svelte';
  import Progress from '../../../lib/components/atoms/Progress/Progress.svelte';
  import Quantity from '../../../lib/components/atoms/Quantity/Quantity.svelte';
  import Timestamp from '../../../lib/components/atoms/Timestamp/Timestamp.svelte';
  import EmptyState from '../../../lib/components/molecules/EmptyState/EmptyState.svelte';
  import MonitorToggle from '../../../lib/components/molecules/MonitorToggle/MonitorToggle.svelte';
  import ConfirmDialog from '../../../lib/components/organisms/ConfirmDialog/ConfirmDialog.svelte';
  import type { Column, SortState } from '../../../lib/components/organisms/DataTable/DataTable.svelte';
  import { emptyFilter, type FilterFacet, type FilterState } from '../../../lib/filters/url';
  import type { ListView } from '../../../lib/route/hash';
  import { app, profileName } from '../../context.svelte';
  import { Resource } from '../../resource.svelte';
  import { refetchOn } from '../../live.svelte';
  import { toApiError } from '../../api/client';
  import type { SearchSummary, Series, SeriesStatus, SeriesSummary } from '../../api/types';
  import { applyList, facetCounts } from '../../list';
  import { count, seriesItemStatus, seriesStatusLabel } from '../../tv';
  import AddSeries from './AddSeries.svelte';
  import type { ScreenProps } from '../../screen';

  let { screen }: ScreenProps = $props();
  const { api, live, profiles, toasts, navigate } = app();

  let rowsAll = $state<SeriesSummary[]>([]);
  const list = new Resource((signal) => api.all<SeriesSummary>('series', { signal, onpage: (r) => { if (!signal.aborted) rowsAll = r; } }));
  async function load(quiet = false) {
    await list.load(quiet);
    if (list.data) rowsAll = list.data;
  }
  onMount(() => {
    void load();
    if (!profiles.data) void profiles.load();
    return refetchOn(live, ['item.*', 'import.completed', 'file.deleted'], () => void load(true), 600);
  });

  let filter = $state<FilterState>(emptyFilter());
  let sort = $state<SortState | undefined>({ column: 'title', direction: 'asc' });
  let view = $state<ListView>('table');
  let selected = $state<string[]>([]);

  const statusOf = (s: SeriesSummary): SeriesStatus => s.status || '';
  const counts = $derived(facetCounts(rowsAll, statusOf));
  const facets = $derived<FilterFacet[]>([
    { id: 'status', label: 'Status', options: (['continuing', 'ended', 'upcoming'] as const).map((v) => ({ value: v, label: seriesStatusLabel[v], count: counts[v] ?? 0 })) },
    { id: 'monitored', label: 'Monitoring', multiple: false, options: [{ value: 'yes', label: 'Monitored' }, { value: 'no', label: 'Unmonitored' }] },
    { id: 'profile', label: 'Profile', options: (profiles.data ?? []).map((p) => ({ value: p.key, label: p.name })) },
  ]);
  const ratio = (s: SeriesSummary) => (s.stats?.episodeCount ? s.stats.episodeFileCount / s.stats.episodeCount : 1);
  const airing = (s: SeriesSummary) => (s.stats?.nextAiring ? Date.parse(s.stats.nextAiring) : Infinity);
  const rows = $derived(applyList(rowsAll, filter, sort, {
    text: (s) => `${s.title} ${(s.altTitles ?? []).join(' ')} ${s.year} ${s.network ?? ''}`,
    facets: { status: statusOf, monitored: (s) => (s.monitored ? 'yes' : 'no'), profile: (s) => s.profileKey },
    sortKeys: {
      title: (s) => s.sortTitle || s.title, network: (s) => s.network ?? '', episodes: ratio, size: (s) => s.stats?.sizeOnDisk ?? 0,
      next: airing,
    },
  }));

  const episodesText = (s: SeriesSummary) => `${s.stats?.episodeFileCount ?? 0}/${s.stats?.episodeCount ?? 0}`;
  const columns: Column<SeriesSummary>[] = [
    { id: 'monitored', label: 'Monitored', width: '3rem', hideLabel: true, phone: 'hidden' },
    { id: 'title', label: 'Title', sortable: true, value: (r) => (r.year ? `${r.title} (${r.year})` : r.title), phone: 'title' },
    { id: 'network', label: 'Network', width: '8rem', sortable: true, phone: 'hidden', value: (r) => r.network || '—' },
    { id: 'status', label: 'Status', width: '7rem', phone: 'hidden', value: (r) => seriesStatusLabel[statusOf(r)] },
    { id: 'profile', label: 'Profile', width: '7rem', phone: 'hidden', value: (r) => profileName(profiles.data, r.profileKey) },
    { id: 'episodes', label: 'Episodes', width: '9rem', sortable: true, phone: 'trailing', value: (r) => `${episodesText(r)} episodes` },
    { id: 'size', label: 'Size', width: '6rem', align: 'end', sortable: true, phone: 'hidden', value: (r) => r.stats?.sizeOnDisk ?? 0 },
    { id: 'next', label: 'Next airing', width: '8rem', sortable: true, phone: 'meta', value: (r) => r.stats?.nextAiring ?? '—' },
  ];
  const poster = (r: SeriesSummary) => ({
    id: String(r.id), title: r.title, kind: 'tv' as const, src: r.posterUrl || null, year: r.year || undefined,
    detail: `${r.stats?.episodeFileCount ?? 0} of ${count(r.stats?.episodeCount ?? 0, 'episode')}`,
    status: { family: 'item' as const, value: seriesItemStatus(r) },
  });

  function upsert(s: Series) {
    const i = rowsAll.findIndex((r) => r.id === s.id);
    if (i >= 0) rowsAll[i] = { ...rowsAll[i], ...s };
  }

  async function setMonitored(s: SeriesSummary, on: boolean) {
    try {
      upsert(await api.put<Series>(`series/${s.id}`, { monitored: on }));
    } catch (e) {
      toasts.push({ message: `Couldn't change monitoring for ${s.title}`, detail: toApiError(e).message, tone: 'error' });
    }
  }

  async function search(ids: number[]) {
    const results = await Promise.allSettled(ids.map((id) => api.post<SearchSummary>(`series/${id}/search`)));
    const ok = results.filter((r): r is PromiseFulfilledResult<SearchSummary> => r.status === 'fulfilled');
    const grabbed = ok.filter((r) => r.value.grabbed).length;
    const failed = results.length - ok.length;
    toasts.push({
      message: ids.length === 1 ? (grabbed ? 'Search grabbed releases for missing episodes' : 'Search finished; nothing accepted to grab') : `Searched ${count(ids.length, 'series', 'series')}: ${grabbed} with grabs`,
      detail: failed ? `${count(failed, 'search', 'searches')} failed; check System › Health.` : undefined,
      tone: failed ? 'warn' : grabbed ? 'ok' : 'info',
      ...(grabbed ? { actionLabel: 'Open queue', onaction: () => navigate('/activity/queue') } : {}),
    });
  }

  let deleting = $state<SeriesSummary | undefined>();
  let deleteOpen = $state(false);
  async function remove(s: SeriesSummary, choice: string | undefined) {
    deleteOpen = false;
    try {
      await api.del(`series/${s.id}`, { deleteFiles: choice === 'files' });
      rowsAll = rowsAll.filter((r) => r.id !== s.id);
      toasts.push({ message: `Deleted ${s.title}`, detail: choice === 'files' ? 'Its files were deleted too.' : 'Its files stay on disk.', tone: 'ok' });
    } catch (e) {
      toasts.push({ message: `Couldn't delete ${s.title}`, detail: toApiError(e).message, tone: 'error' });
    }
  }

  function onaction(action: string, s: SeriesSummary) {
    if (action === 'search') void search([s.id]);
    else if (action === 'open') navigate(`/library/tv/${s.id}`);
    else if (action === 'delete') { deleting = s; deleteOpen = true; }
  }

  const fileCount = $derived(rowsAll.reduce((n, s) => n + (s.stats?.episodeFileCount ?? 0), 0));
  const plural = (n: number) => count(n, 'series', 'series');
  const description = $derived(rowsAll.length ? `${plural(rowsAll.length)} · ${count(fileCount, 'episode')} on disk` : undefined);

  function onkeydown(e: KeyboardEvent) {
    const t = e.target as HTMLElement | null;
    if (e.defaultPrevented || e.ctrlKey || e.metaKey || e.altKey || (t && (t.isContentEditable || /^(INPUT|TEXTAREA|SELECT)$/.test(t.tagName)))) return;
    if (document.querySelector('dialog[open]')) return;
    if (e.key === 'n') { e.preventDefault(); navigate('/library/tv/add'); }
    else if (e.key === 'v') { e.preventDefault(); view = view === 'table' ? 'posters' : 'table'; }
  }
</script>

<svelte:window {onkeydown} />

{#snippet cell(r: SeriesSummary, c: Column<SeriesSummary>)}
  {#if c.id === 'monitored'}<MonitorToggle item={r.title} state={r.monitored ? 'monitored' : 'unmonitored'} size="sm" onchange={(s) => setMonitored(r, s === 'monitored')} />
  {:else if c.id === 'title'}<a class="title" href="#/library/tv/{r.id}">{r.title}{#if r.year}{' '}<span class="year num">({r.year})</span>{/if}</a>
  {:else if c.id === 'episodes'}
    <Progress label="Episodes of {r.title} on disk" done={r.stats?.episodeFileCount ?? 0} total={r.stats?.episodeCount ?? 0} size="sm" showValue
      tone={seriesItemStatus(r) === 'cutoff-met' ? 'ok' : 'warn'} />
  {:else if c.id === 'size'}{#if r.stats?.sizeOnDisk}<Quantity kind="bytes" value={r.stats.sizeOnDisk} />{:else}—{/if}
  {:else if c.id === 'next'}{#if r.stats?.nextAiring}<Timestamp value={r.stats.nextAiring} />{:else}—{/if}
  {:else}{c.value?.(r)}{/if}
{/snippet}

{#snippet emptyLibrary()}
  <EmptyState title="No series yet" icon="tv" level={2}>
    Add a series by name or provider id; corsarr tracks its episodes and fetches the ones you monitor as they air.
    {#snippet actions()}<Button variant="primary" icon="plus" href="#/library/tv/add">Add series</Button>{/snippet}
  </EmptyState>
{/snippet}

{#if profiles.data || profiles.error}
  <ListPage title="TV" {description} {facets} bind:filter bind:sort bind:view bind:selected syncUrl searchLabel="Filter series" placeholder="Filter by title or network"
    summary={rows.length === rowsAll.length ? plural(rowsAll.length) : `${rows.length} of ${plural(rowsAll.length)}`}
    {rows} {columns} rowKey={(r) => String(r.id)} {cell} {poster} selectable noun="series" nounPlural="series" total={rowsAll.length}
    loading={list.loading && !rowsAll.length} error={list.error?.message} errorCode={list.error?.label} onretry={() => void load()}
    onopen={(r) => navigate(`/library/tv/${r.id}`)} empty={emptyLibrary} {onaction}
    rowActions={() => [
      { id: 'open', label: 'Open', icon: 'arrow-right' },
      { id: 'search', label: 'Search missing episodes', icon: 'search' },
      { id: 'delete', label: 'Delete…', icon: 'trash', danger: true, group: 'd' },
    ]}>
    {#snippet actions()}
      <Button variant="primary" icon="plus" href="#/library/tv/add">Add series</Button>
    {/snippet}
    {#snippet bulkActions(keys)}
      <Button size="sm" variant="primary" icon="search" onclick={() => { void search(keys.map(Number)); selected = []; }}>Search {keys.length}</Button>
    {/snippet}
  </ListPage>
{:else}
  <ListPage title="TV" rows={[]} {columns} rowKey={(r) => String(r.id)} loading noun="series" nounPlural="series" />
{/if}

{#if deleting}
  <ConfirmDialog bind:open={deleteOpen} title="Delete {deleting.title}?" confirmLabel="Delete series" undoable={false}
    consequences={['Removes the series and its episodes from the library and stops searching for them', 'History about it is kept']}
    choices={[
      { value: 'keep', label: 'Keep its files', description: 'The folder stays on disk; you can add the series again later.' },
      { value: 'files', label: 'Delete its files too', description: 'The series folder is deleted from disk.', consequences: ['Deletes the files under ' + deleting.path] },
    ]}
    onconfirm={(c) => remove(deleting!, c)} oncancel={() => (deleteOpen = false)} />
{/if}

{#if screen === 'addSeries'}
  <AddSeries onclose={() => navigate('/library/tv')} onadded={() => void load(true)} />
{/if}

<style>
  .title { color: var(--ink); text-decoration: none; font-weight: var(--weight-medium); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; display: block; }
  .title:hover { text-decoration: underline; }
  .year { font-weight: var(--weight-regular); color: var(--ink-muted); }
</style>
