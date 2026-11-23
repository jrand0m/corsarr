<!-- Library › Movies: table or posters, filters, sort and view in the URL; the add dialog at …/add. -->
<script lang="ts">
  import { onMount } from 'svelte';
  import ListPage from '../../../lib/components/templates/ListPage/ListPage.svelte';
  import Button from '../../../lib/components/atoms/Button/Button.svelte';
  import StatusBadge from '../../../lib/components/atoms/StatusBadge/StatusBadge.svelte';
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
  import type { Download, Movie, SearchSummary } from '../../api/types';
  import { movieStatus, queuedMovieIds, type ItemStatus } from '../../model';
  import { applyList, facetCounts } from '../../list';
  import AddMovie from './AddMovie.svelte';
  import type { ScreenProps } from '../../screen';

  let { screen }: ScreenProps = $props();
  const { api, live, movies, profiles, toasts, navigate } = app();

  const queue = new Resource((s) => api.list<Download>('queue', { limit: 200 }, s));
  onMount(() => {
    void movies.load(movies.loaded);
    void queue.load();
    if (!profiles.data) void profiles.load();
    const offs = [
      refetchOn(live, ['item.*', 'import.completed', 'file.deleted'], () => void movies.load(true), 600),
      refetchOn(live, ['grab.sent', 'download.state_changed', 'download.removed', 'download.failed', 'import.*'], () => void queue.load(true), 600),
    ];
    return () => offs.forEach((off) => off());
  });

  let filter = $state<FilterState>(emptyFilter());
  let sort = $state<SortState | undefined>({ column: 'title', direction: 'asc' });
  let view = $state<ListView>('table');
  let selected = $state<string[]>([]);

  const now = Date.now();
  const queued = $derived(queuedMovieIds(queue.data ?? []));
  const statusOf = (m: Movie): ItemStatus => movieStatus(m, now, queued);
  const statusLabels: Record<ItemStatus, string> = { missing: 'Missing', queued: 'Queued', 'cutoff-met': 'On disk', 'not-available': 'Not available yet' };

  const counts = $derived(facetCounts(movies.rows, statusOf));
  const facets = $derived<FilterFacet[]>([
    { id: 'status', label: 'Status', options: (Object.keys(statusLabels) as ItemStatus[]).map((v) => ({ value: v, label: statusLabels[v], count: counts[v] ?? 0 })) },
    { id: 'monitored', label: 'Monitoring', multiple: false, options: [{ value: 'yes', label: 'Monitored' }, { value: 'no', label: 'Unmonitored' }] },
    { id: 'profile', label: 'Profile', options: (profiles.data ?? []).map((p) => ({ value: p.key, label: p.name })) },
  ]);
  const rows = $derived(applyList(movies.rows, filter, sort, {
    text: (m) => `${m.title} ${m.originalTitle ?? ''} ${m.year}`,
    facets: { status: statusOf, monitored: (m) => (m.monitored ? 'yes' : 'no'), profile: (m) => m.profileKey },
    sortKeys: { title: (m) => m.title, year: (m) => m.year, added: (m) => Date.parse(m.addedAt) || 0 },
  }));

  const columns: Column<Movie>[] = [
    { id: 'monitored', label: 'Monitored', width: '3rem', hideLabel: true, phone: 'hidden' },
    { id: 'title', label: 'Title', sortable: true, value: (r) => r.title, phone: 'title' },
    { id: 'year', label: 'Year', width: '4.5rem', align: 'end', sortable: true, value: (r) => (r.year ? String(r.year) : '—') },
    { id: 'status', label: 'Status', width: '10rem', phone: 'trailing', value: (r) => statusLabels[statusOf(r)] },
    { id: 'profile', label: 'Profile', width: '7rem', phone: 'meta', value: (r) => profileName(profiles.data, r.profileKey) },
    { id: 'added', label: 'Added', width: '7rem', sortable: true, phone: 'hidden', value: (r) => r.addedAt },
  ];
  const poster = (r: Movie) => ({ id: String(r.id), title: r.title, kind: 'movie' as const, year: r.year || undefined, status: { family: 'item' as const, value: statusOf(r) } });

  async function setMonitored(m: Movie, on: boolean) {
    try {
      movies.upsert(await api.put<Movie>(`movies/${m.id}`, { monitored: on }));
    } catch (e) {
      toasts.push({ message: `Couldn't change monitoring for ${m.title}`, detail: toApiError(e).message, tone: 'error' });
    }
  }

  async function search(ids: number[]) {
    const results = await Promise.allSettled(ids.map((id) => api.post<SearchSummary>(`movies/${id}/search`)));
    const ok = results.filter((r) => r.status === 'fulfilled') as PromiseFulfilledResult<SearchSummary>[];
    const grabbed = ok.filter((r) => r.value.grabbed).length;
    const failed = results.length - ok.length;
    toasts.push({
      message: ids.length === 1 ? (grabbed ? 'Search found a release and grabbed it' : 'Search finished; nothing accepted to grab') : `Searched ${ids.length} movies: ${grabbed} grabbed`,
      detail: failed ? `${failed} search${failed === 1 ? '' : 'es'} failed; check System › Health.` : undefined,
      tone: failed ? 'warn' : grabbed ? 'ok' : 'info',
      ...(grabbed ? { actionLabel: 'Open queue', onaction: () => navigate('/activity/queue') } : {}),
    });
  }

  let deleting = $state<Movie | undefined>();
  let deleteOpen = $state(false);
  async function remove(m: Movie, choice: string | undefined) {
    deleteOpen = false;
    try {
      await api.del(`movies/${m.id}`, { deleteFiles: choice === 'files' });
      movies.remove(m.id);
      toasts.push({ message: `Deleted ${m.title}`, detail: choice === 'files' ? 'Its files were deleted too.' : 'Its files stay on disk.', tone: 'ok' });
    } catch (e) {
      toasts.push({ message: `Couldn't delete ${m.title}`, detail: toApiError(e).message, tone: 'error' });
    }
  }

  function onaction(action: string, m: Movie) {
    if (action === 'search') void search([m.id]);
    else if (action === 'open') navigate(`/library/movies/${m.id}`);
    else if (action === 'delete') { deleting = m; deleteOpen = true; }
  }

  const onDisk = $derived(movies.rows.filter((m) => m.fileId).length);
  const plural = (n: number) => `${n} ${n === 1 ? 'movie' : 'movies'}`;
  const description = $derived(movies.rows.length ? `${plural(movies.rows.length)} · ${onDisk} on disk` : undefined);

  function onkeydown(e: KeyboardEvent) {
    const t = e.target as HTMLElement | null;
    if (e.defaultPrevented || e.ctrlKey || e.metaKey || e.altKey || (t && (t.isContentEditable || /^(INPUT|TEXTAREA|SELECT)$/.test(t.tagName)))) return;
    if (document.querySelector('dialog[open]')) return;
    if (e.key === 'n') { e.preventDefault(); navigate('/library/movies/add'); }
    else if (e.key === 'v') { e.preventDefault(); view = view === 'table' ? 'posters' : 'table'; }
  }
</script>

<svelte:window {onkeydown} />

{#snippet cell(r: Movie, c: Column<Movie>)}
  {#if c.id === 'monitored'}<MonitorToggle item={r.title} state={r.monitored ? 'monitored' : 'unmonitored'} size="sm" onchange={(s) => setMonitored(r, s === 'monitored')} />
  {:else if c.id === 'title'}<a class="title" href="#/library/movies/{r.id}">{r.title}</a>
  {:else if c.id === 'status'}<StatusBadge family="item" value={statusOf(r)} size="sm" appearance="plain" />
  {:else if c.id === 'added'}<Timestamp value={r.addedAt} />
  {:else}{c.value?.(r)}{/if}
{/snippet}

{#snippet emptyLibrary()}
  <EmptyState title="No movies yet" icon="film" level={2}>
    Add a movie by name or TMDB id; corsarr searches for it and imports it into your root folder.
    {#snippet actions()}<Button variant="primary" icon="plus" href="#/library/movies/add">Add movie</Button>{/snippet}
  </EmptyState>
{/snippet}

{#if profiles.data || profiles.error}
  <ListPage title="Movies" {description} {facets} bind:filter bind:sort bind:view bind:selected syncUrl searchLabel="Filter movies" placeholder="Filter by title"
    summary={rows.length === movies.rows.length ? plural(movies.rows.length) : `${rows.length} of ${plural(movies.rows.length)}`}
    {rows} {columns} rowKey={(r) => String(r.id)} {cell} {poster} selectable noun="movie" total={movies.rows.length}
    loading={movies.res.loading && !movies.rows.length} error={movies.res.error?.message} errorCode={movies.res.error?.label} onretry={() => void movies.load()}
    onopen={(r) => navigate(`/library/movies/${r.id}`)} empty={emptyLibrary} {onaction}
    rowActions={() => [
      { id: 'open', label: 'Open', icon: 'arrow-right' },
      { id: 'search', label: 'Search now', icon: 'search' },
      { id: 'delete', label: 'Delete…', icon: 'trash', danger: true, group: 'd' },
    ]}>
    {#snippet actions()}
      <Button variant="primary" icon="plus" href="#/library/movies/add">Add movie</Button>
    {/snippet}
    {#snippet bulkActions(keys)}
      <Button size="sm" variant="primary" icon="search" onclick={() => { void search(keys.map(Number)); selected = []; }}>Search {keys.length}</Button>
    {/snippet}
  </ListPage>
{:else}
  <ListPage title="Movies" rows={[]} {columns} rowKey={(r) => String(r.id)} loading noun="movie" />
{/if}

{#if deleting}
  <ConfirmDialog bind:open={deleteOpen} title="Delete {deleting.title}?" confirmLabel="Delete movie" undoable={false}
    consequences={['Removes the movie from the library and stops searching for it', 'History about it is kept']}
    choices={[
      { value: 'keep', label: 'Keep its files', description: 'The folder stays on disk; you can add the movie again later.' },
      { value: 'files', label: 'Delete its files too', description: 'The movie folder is deleted from disk.', consequences: ['Deletes the files under ' + deleting.path] },
    ]}
    onconfirm={(c) => remove(deleting!, c)} oncancel={() => (deleteOpen = false)} />
{/if}

{#if screen === 'add'}
  <AddMovie onclose={() => navigate('/library/movies')} />
{/if}

<style>
  .title { color: var(--ink); text-decoration: none; font-weight: var(--weight-medium); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; display: block; }
  .title:hover { text-decoration: underline; }
</style>
