<!-- Library › Music: artists as a table, filters and sort in the URL; the add dialog at …/add. -->
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
  import { app } from '../../context.svelte';
  import { Resource } from '../../resource.svelte';
  import { refetchOn } from '../../live.svelte';
  import { toApiError } from '../../api/client';
  import type { Artist, ArtistSummary, MusicProfiles, SearchSummary } from '../../api/types';
  import { applyList } from '../../list';
  import { artistItemStatus } from '../../music';
  import { count } from '../../tv';
  import AddArtist from './AddArtist.svelte';
  import type { ScreenProps } from '../../screen';
  import { artSrc } from '../../art';
  import Artwork from '../../../lib/components/atoms/Artwork/Artwork.svelte';

  let { screen }: ScreenProps = $props();
  const { api, live, toasts, navigate } = app();

  let rowsAll = $state<ArtistSummary[]>([]);
  const list = new Resource((signal) => api.all<ArtistSummary>('artists', { signal, onpage: (r) => { if (!signal.aborted) rowsAll = r; } }));
  const profiles = new Resource((s) => api.get<MusicProfiles>('music/profiles', undefined, s));
  async function load(quiet = false) {
    await list.load(quiet);
    if (list.data) rowsAll = list.data;
  }
  onMount(() => {
    void load();
    void profiles.load();
    return refetchOn(live, ['item.*', 'import.completed', 'file.deleted'], () => void load(true), 600);
  });

  let filter = $state<FilterState>(emptyFilter());
  let sort = $state<SortState | undefined>({ column: 'name', direction: 'asc' });
  let selected = $state<string[]>([]);

  const nameOf = (list: { key: string; name: string }[] | undefined, key: string) => list?.find((p) => p.key === key)?.name ?? key;
  const facets = $derived<FilterFacet[]>([
    { id: 'monitored', label: 'Monitoring', multiple: false, options: [{ value: 'yes', label: 'Monitored' }, { value: 'no', label: 'Unmonitored' }] },
    { id: 'profile', label: 'Quality profile', options: (profiles.data?.quality ?? []).map((p) => ({ value: p.key, label: p.name })) },
    { id: 'metadata', label: 'Metadata profile', options: (profiles.data?.metadata ?? []).map((p) => ({ value: p.key, label: p.name })) },
  ]);
  const ratio = (a: ArtistSummary) => (a.stats?.monitoredAlbums ? a.stats.albumsWithFiles / a.stats.monitoredAlbums : 1);
  const rows = $derived(applyList(rowsAll, filter, sort, {
    text: (a) => `${a.name} ${a.sortName} ${a.disambiguation ?? ''}`,
    facets: { monitored: (a) => (a.monitored ? 'yes' : 'no'), profile: (a) => a.profileKey, metadata: (a) => a.metadataProfile },
    sortKeys: { name: (a) => a.sortName || a.name, albums: ratio, size: (a) => a.stats?.sizeOnDisk ?? 0, next: (a) => a.stats?.nextRelease ?? '' },
  }));

  const columns: Column<ArtistSummary>[] = [
    { id: 'monitored', label: 'Monitored', width: '3rem', hideLabel: true, phone: 'hidden' },
    { id: 'name', label: 'Artist', sortable: true, value: (r) => r.name, phone: 'title' },
    { id: 'profile', label: 'Quality', width: '7rem', phone: 'hidden', value: (r) => nameOf(profiles.data?.quality, r.profileKey) },
    { id: 'metadata', label: 'Metadata', width: '9rem', phone: 'hidden', value: (r) => nameOf(profiles.data?.metadata, r.metadataProfile) },
    { id: 'albums', label: 'Albums', width: '9rem', sortable: true, phone: 'trailing', value: (r) => `${r.stats?.albumsWithFiles ?? 0}/${r.stats?.monitoredAlbums ?? 0} albums` },
    { id: 'size', label: 'Size', width: '6rem', align: 'end', sortable: true, phone: 'hidden', value: (r) => r.stats?.sizeOnDisk ?? 0 },
    { id: 'next', label: 'Next release', width: '8rem', sortable: true, phone: 'meta', value: (r) => r.stats?.nextRelease ?? '—' },
  ];

  function upsert(a: Artist) {
    const i = rowsAll.findIndex((r) => r.id === a.id);
    if (i >= 0) rowsAll[i] = { ...rowsAll[i], ...a };
  }

  async function setMonitored(a: ArtistSummary, on: boolean) {
    try {
      upsert(await api.put<Artist>(`artists/${a.id}`, { monitored: on }));
    } catch (e) {
      toasts.push({ message: `Couldn't change monitoring for ${a.name}`, detail: toApiError(e).message, tone: 'error' });
    }
  }

  async function search(ids: number[]) {
    const results = await Promise.allSettled(ids.map((id) => api.post<SearchSummary>(`artists/${id}/search`)));
    const ok = results.filter((r): r is PromiseFulfilledResult<SearchSummary> => r.status === 'fulfilled');
    const grabbed = ok.filter((r) => r.value.grabbed).length;
    const failed = results.length - ok.length;
    toasts.push({
      message: grabbed ? `Grabbed releases for ${count(grabbed, 'artist')}` : 'Search finished; nothing accepted to grab',
      detail: failed ? `${count(failed, 'search', 'searches')} failed; check System › Health.` : undefined,
      tone: failed ? 'warn' : grabbed ? 'ok' : 'info',
      ...(grabbed ? { actionLabel: 'Open queue', onaction: () => navigate('/activity/queue') } : {}),
    });
  }

  let deleting = $state<ArtistSummary | undefined>();
  let deleteOpen = $state(false);
  async function remove(a: ArtistSummary, choice: string | undefined) {
    deleteOpen = false;
    try {
      await api.del(`artists/${a.id}`, { deleteFiles: choice === 'files' });
      rowsAll = rowsAll.filter((r) => r.id !== a.id);
      toasts.push({ message: `Deleted ${a.name}`, detail: choice === 'files' ? 'Its files were deleted too.' : 'Its files stay on disk.', tone: 'ok' });
    } catch (e) {
      toasts.push({ message: `Couldn't delete ${a.name}`, detail: toApiError(e).message, tone: 'error' });
    }
  }

  function onaction(action: string, a: ArtistSummary) {
    if (action === 'search') void search([a.id]);
    else if (action === 'open') navigate(`/library/music/${a.id}`);
    else if (action === 'delete') { deleting = a; deleteOpen = true; }
  }

  const albumsOnDisk = $derived(rowsAll.reduce((n, a) => n + (a.stats?.albumsWithFiles ?? 0), 0));
  const description = $derived(rowsAll.length ? `${count(rowsAll.length, 'artist')} · ${count(albumsOnDisk, 'album')} on disk` : undefined);
</script>

{#snippet cell(r: ArtistSummary, c: Column<ArtistSummary>)}
  {#if c.id === 'monitored'}<MonitorToggle item={r.name} state={r.monitored ? 'monitored' : 'unmonitored'} size="sm" onchange={(s) => setMonitored(r, s === 'monitored')} />
  {:else if c.id === 'name'}<span class="artist"><Artwork src={artSrc(r.coverUrl, 32)} title={r.name} kind="music" shape="cover" width={32} /><a class="title" href="#/library/music/{r.id}">{r.name}{#if r.disambiguation}{' '}<span class="dis">({r.disambiguation})</span>{/if}</a></span>
  {:else if c.id === 'albums'}
    <Progress label="Albums of {r.name} on disk" done={r.stats?.albumsWithFiles ?? 0} total={r.stats?.monitoredAlbums ?? 0} size="sm" showValue
      tone={artistItemStatus(r.stats) === 'missing' ? 'warn' : 'ok'} />
  {:else if c.id === 'size'}{#if r.stats?.sizeOnDisk}<Quantity kind="bytes" value={r.stats.sizeOnDisk} />{:else}—{/if}
  {:else if c.id === 'next'}{#if r.stats?.nextRelease}<Timestamp value={r.stats.nextRelease} />{:else}—{/if}
  {:else}{c.value?.(r)}{/if}
{/snippet}

{#snippet emptyLibrary()}
  <EmptyState title="No artists yet" icon="music" level={2}>
    Add an artist by name or MusicBrainz id; corsarr keeps their discography and fetches the albums you monitor.
    {#snippet actions()}<Button variant="primary" icon="plus" href="#/library/music/add">Add artist</Button>{/snippet}
  </EmptyState>
{/snippet}

<ListPage title="Music" {description} {facets} bind:filter bind:sort bind:selected syncUrl searchLabel="Filter artists" placeholder="Filter by name"
  summary={rows.length === rowsAll.length ? count(rowsAll.length, 'artist') : `${rows.length} of ${count(rowsAll.length, 'artist')}`}
  {rows} {columns} rowKey={(r) => String(r.id)} {cell} selectable noun="artist" nounPlural="artists" total={rowsAll.length}
  loading={list.loading && !rowsAll.length} error={list.error?.message} errorCode={list.error?.label} onretry={() => void load()}
  onopen={(r) => navigate(`/library/music/${r.id}`)} empty={emptyLibrary} {onaction}
  rowActions={() => [
    { id: 'open', label: 'Open', icon: 'arrow-right' },
    { id: 'search', label: 'Search wanted albums', icon: 'search' },
    { id: 'delete', label: 'Delete…', icon: 'trash', danger: true, group: 'd' },
  ]}>
  {#snippet actions()}
    <Button variant="primary" icon="plus" href="#/library/music/add">Add artist</Button>
  {/snippet}
  {#snippet bulkActions(keys)}
    <Button size="sm" variant="primary" icon="search" onclick={() => { void search(keys.map(Number)); selected = []; }}>Search {keys.length}</Button>
  {/snippet}
</ListPage>

{#if deleting}
  <ConfirmDialog bind:open={deleteOpen} title="Delete {deleting.name}?" confirmLabel="Delete artist" undoable={false}
    consequences={['Removes the artist and every album from the library and stops searching for them', 'History about it is kept']}
    choices={[
      { value: 'keep', label: 'Keep its files', description: 'The folder stays on disk; you can add the artist again later.' },
      { value: 'files', label: 'Delete its files too', description: 'The track files are deleted from disk.', consequences: ['Deletes the files under ' + deleting.path] },
    ]}
    onconfirm={(c) => remove(deleting!, c)} oncancel={() => (deleteOpen = false)} />
{/if}

{#if screen === 'addArtist'}
  <AddArtist profiles={profiles.data} onclose={() => navigate('/library/music')} onadded={() => void load(true)} />
{/if}

<style>
  .artist { display: flex; align-items: center; gap: var(--space-2); min-width: 0; }
  .title { color: var(--ink); text-decoration: none; font-weight: var(--weight-medium); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; display: block; }
  .title:hover { text-decoration: underline; }
  .dis { font-weight: var(--weight-regular); color: var(--ink-muted); }
</style>
