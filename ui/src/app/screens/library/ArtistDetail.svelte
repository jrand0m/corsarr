<!--
  Artist detail: header facts, the artist's albums by type and date (monitoring, track progress,
  status, quality, search) with albums outside the metadata profile behind a switch, and the
  artist's identifiers. Monitoring an album by hand pins it, so profile changes never drop it.
-->
<script lang="ts">
  import { onMount } from 'svelte';
  import DetailPage from '../../../lib/components/templates/DetailPage/DetailPage.svelte';
  import Button from '../../../lib/components/atoms/Button/Button.svelte';
  import Progress from '../../../lib/components/atoms/Progress/Progress.svelte';
  import Quantity from '../../../lib/components/atoms/Quantity/Quantity.svelte';
  import Skeleton from '../../../lib/components/atoms/Skeleton/Skeleton.svelte';
  import StatusBadge from '../../../lib/components/atoms/StatusBadge/StatusBadge.svelte';
  import Switch from '../../../lib/components/atoms/Switch/Switch.svelte';
  import EmptyState from '../../../lib/components/molecules/EmptyState/EmptyState.svelte';
  import MonitorToggle, { type MonitorState } from '../../../lib/components/molecules/MonitorToggle/MonitorToggle.svelte';
  import PropertyList from '../../../lib/components/molecules/PropertyList/PropertyList.svelte';
  import ConfirmDialog from '../../../lib/components/organisms/ConfirmDialog/ConfirmDialog.svelte';
  import DataTable, { type Column } from '../../../lib/components/organisms/DataTable/DataTable.svelte';
  import { formatBytes } from '../../../lib/format/quantity';
  import { app } from '../../context.svelte';
  import { Resource } from '../../resource.svelte';
  import { refetchOn } from '../../live.svelte';
  import { toApiError } from '../../api/client';
  import type { AlbumView, Artist, ArtistDetail, SearchSummary } from '../../api/types';
  import { albumItemStatus, albumOrder, albumTypes, albumYear, artistItemStatus, shownAlbums } from '../../music';
  import { count } from '../../tv';
  import type { ScreenProps } from '../../screen';
  import { artSrc } from '../../art';
  import Artwork from '../../../lib/components/atoms/Artwork/Artwork.svelte';

  let { params = {} }: ScreenProps = $props();
  const { api, live, toasts, navigate } = app();
  // svelte-ignore state_referenced_locally
  const id = Number(params.id);

  const detail = new Resource((s) => api.get<ArtistDetail>(`artists/${id}`, undefined, s));
  onMount(() => {
    void detail.load();
    return refetchOn(live, ['item.*', 'import.completed', 'file.deleted', 'grab.sent'], () => void detail.load(true), 600);
  });

  const artist = $derived(detail.data?.artist);
  let showAll = $state(false);
  const albums = $derived(shownAlbums((detail.data?.albums ?? []).slice().sort(albumOrder), showAll));
  const hidden = $derived((detail.data?.albums?.length ?? 0) - shownAlbums(detail.data?.albums ?? [], false).length);

  const entity = $derived({
    title: artist?.name ?? 'Artist', kind: 'music' as const, kindLabel: 'Artist', art: artSrc(artist?.coverUrl, 128),
    meta: [artist?.disambiguation, artist?.type, artist?.country].filter(Boolean).join(' · '),
    back: { href: '#/library/music', label: 'Music' },
    monitor: (artist?.monitored ? 'monitored' : 'unmonitored') as MonitorState,
    status: artist ? [{ family: 'item' as const, value: artistItemStatus(artist.stats) }] : [],
    facts: artist ? [
      { label: 'Albums on disk', value: `${artist.stats.albumsWithFiles} of ${artist.stats.monitoredAlbums} monitored` },
      { label: 'Tracks', value: String(artist.stats.trackFileCount) },
      { label: 'Size', value: formatBytes(artist.stats.sizeOnDisk) },
    ] : [],
    menu: [{ id: 'refresh', label: 'Refresh metadata', icon: 'refresh' as const }, { id: 'delete', label: 'Delete…', icon: 'trash' as const, danger: true }],
  });

  async function setMonitor(s: MonitorState) {
    try {
      await api.put<Artist>(`artists/${id}`, { monitored: s === 'monitored' });
      void detail.load(true);
    } catch (e) {
      toasts.push({ message: 'Monitoring was not changed', detail: toApiError(e).message, tone: 'error' });
    }
  }

  async function monitorAlbum(a: AlbumView, on: boolean) {
    try {
      await api.put('albums/monitor', { ids: [a.id], monitored: on });
      void detail.load(true);
    } catch (e) {
      toasts.push({ message: `Couldn't change monitoring for ${a.title}`, detail: toApiError(e).message, tone: 'error' });
    }
  }

  let searching = $state(false);
  async function search(path: string) {
    if (searching) return;
    searching = true;
    try {
      const s = await api.post<SearchSummary>(path);
      toasts.push(s.grabbed || s.shadowed
        ? { message: s.grabbed ? `Grabbed ${s.grabbed}` : `Would grab ${s.shadowed} (shadow mode)`, tone: 'ok', actionLabel: 'Open queue', onaction: () => navigate('/activity/queue') }
        : { message: s.releases ? 'No release was good enough to grab' : 'No releases found', detail: `${s.releases} releases checked across ${s.indexers} indexers.`, tone: s.failed ? 'warn' : 'info' });
    } catch (e) {
      toasts.push({ message: 'The search did not run', detail: toApiError(e).message, tone: 'error' });
    } finally {
      searching = false;
    }
  }

  let deleteOpen = $state(false);
  async function remove(choice: string | undefined) {
    deleteOpen = false;
    try {
      await api.del(`artists/${id}`, { deleteFiles: choice === 'files' });
      toasts.push({ message: `Deleted ${artist?.name}`, tone: 'ok' });
      navigate('/library/music');
    } catch (e) {
      toasts.push({ message: 'The artist was not deleted', detail: toApiError(e).message, tone: 'error' });
    }
  }

  async function onmenu(action: string) {
    if (action === 'delete') { deleteOpen = true; return; }
    try {
      await api.post(`artists/${id}/refresh`);
      void detail.load(true);
      toasts.push({ message: 'Metadata refreshed', tone: 'ok' });
    } catch (e) {
      toasts.push({ message: 'The refresh failed', detail: toApiError(e).message, tone: 'error' });
    }
  }

  function onalbum(action: string, a: AlbumView) {
    if (action === 'open') navigate(`/library/music/albums/${a.id}`);
    else if (action === 'search') void search(`albums/${a.id}/search`);
  }

  const columns: Column<AlbumView>[] = [
    { id: 'monitored', label: 'Monitored', width: '3rem', hideLabel: true, phone: 'hidden' },
    { id: 'title', label: 'Album', value: (a) => a.title, phone: 'title' },
    { id: 'year', label: 'Year', width: '4rem', phone: 'meta', value: (a) => albumYear(a) || '—' },
    { id: 'type', label: 'Type', width: '7rem', phone: 'hidden', value: (a) => albumTypes(a) },
    { id: 'tracks', label: 'Tracks', width: '7rem', phone: 'trailing', value: (a) => `${a.fileCount}/${a.trackCount}` },
    { id: 'status', label: 'Status', width: '9rem', phone: 'hidden' },
    { id: 'size', label: 'Size', width: '6rem', align: 'end', phone: 'hidden', value: (a) => a.sizeOnDisk },
  ];
</script>

{#snippet albumCell(a: AlbumView, c: Column<AlbumView>)}
  {#if c.id === 'monitored'}<MonitorToggle item={a.title} state={a.monitored ? 'monitored' : 'unmonitored'} size="sm" onchange={(s) => monitorAlbum(a, s === 'monitored')} />
  {:else if c.id === 'title'}<span class="album"><Artwork src={artSrc(a.coverUrl, 32)} title={a.title} kind="music" shape="cover" width={32} /><a class="title" href="#/library/music/albums/{a.id}">{a.title}</a></span>
  {:else if c.id === 'tracks'}
    {#if a.trackCount}<Progress label="Tracks of {a.title} on disk" done={a.fileCount} total={a.trackCount} size="sm" showValue tone={a.status === 'downloaded' ? 'ok' : 'warn'} />{:else}—{/if}
  {:else if c.id === 'status'}<StatusBadge family="item" value={albumItemStatus(a)} size="sm" detail={a.quality} />
  {:else if c.id === 'size'}{#if a.sizeOnDisk}<Quantity kind="bytes" value={a.sizeOnDisk} />{:else}—{/if}
  {:else}{c.value?.(a)}{/if}
{/snippet}

{#snippet headActions()}
  <Button variant="primary" icon="search" loading={searching} disabled={searching} onclick={() => search(`artists/${id}/search`)}>Search wanted albums</Button>
{/snippet}

{#snippet body()}
  {#if !artist}
    <Skeleton lines={4} />
  {:else}
    <section class="albums">
      <div class="bar">
        <h2>Albums</h2>
        {#if hidden > 0 || showAll}<Switch label="Show releases outside the metadata profile ({hidden})" bind:checked={showAll} />{/if}
      </div>
      <DataTable label="{artist.name} albums" rows={albums} {columns} rowKey={(a) => String(a.id)} cell={albumCell} noun="album"
        onopen={(a) => navigate(`/library/music/albums/${a.id}`)} onaction={onalbum}
        rowActions={() => [{ id: 'open', label: 'Open', icon: 'arrow-right' }, { id: 'search', label: 'Search this album', icon: 'search' }]}>
        {#snippet empty()}<EmptyState size="sm" title="No albums listed" icon="music" level={3}>Refresh the metadata, or show releases outside the profile.</EmptyState>{/snippet}
      </DataTable>
    </section>
  {/if}
{/snippet}

{#snippet aside()}
  {#if artist}
    <PropertyList label="Library" items={[
      { key: 'path', label: 'Folder', value: artist.path, mono: true, copy: true },
      { key: 'albums', label: 'Release groups', value: `${artist.stats.albumCount} in profile, ${count(artist.stats.totalAlbums, 'release group')} in all` },
      { key: 'mb', label: 'MusicBrainz', value: artist.mbid, href: `https://musicbrainz.org/artist/${artist.mbid}` },
    ]} />
  {/if}
{/snippet}

<DetailPage entity={{ ...entity, actions: artist ? headActions : undefined, onmonitor: setMonitor, onmenu }} children={body} {aside}
  loading={detail.loading && !artist} error={detail.error?.message} errorCode={detail.error?.label} onretry={detail.retry} />

{#if artist}
  <ConfirmDialog bind:open={deleteOpen} title="Delete {artist.name}?" confirmLabel="Delete artist" undoable={false}
    consequences={['Removes the artist and every album from the library and stops searching for them', 'History about it is kept']}
    choices={[
      { value: 'keep', label: 'Keep its files', description: 'The folder stays on disk.' },
      { value: 'files', label: 'Delete its files too', description: 'The track files are deleted from disk.', consequences: [`Deletes the files under ${artist.path}`] },
    ]}
    onconfirm={remove} oncancel={() => (deleteOpen = false)} />
{/if}

<style>
  .album { display: flex; align-items: center; gap: var(--space-2); min-width: 0; }
  .albums { display: grid; gap: var(--space-3); }
  .bar { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: var(--space-2); }
  h2 { font-size: var(--text-lg); }
  .title { color: var(--ink); text-decoration: none; font-weight: var(--weight-medium); }
  .title:hover { text-decoration: underline; }
</style>
