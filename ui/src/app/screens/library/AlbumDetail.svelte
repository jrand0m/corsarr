<!--
  Album detail: header facts, the selected edition's tracks per disc (position, title, length,
  file and quality), automatic search, and interactive search in a drawer that streams the
  decisions for this album from every indexer and direct source. A grab goes through the
  override rules: 409 asks to confirm, 403 refuses, shadow mode only records it.
-->
<script lang="ts">
  import { onMount } from 'svelte';
  import DetailPage from '../../../lib/components/templates/DetailPage/DetailPage.svelte';
  import Button from '../../../lib/components/atoms/Button/Button.svelte';
  import Skeleton from '../../../lib/components/atoms/Skeleton/Skeleton.svelte';
  import EmptyState from '../../../lib/components/molecules/EmptyState/EmptyState.svelte';
  import ErrorState from '../../../lib/components/molecules/ErrorState/ErrorState.svelte';
  import type { MonitorState } from '../../../lib/components/molecules/MonitorToggle/MonitorToggle.svelte';
  import PropertyList from '../../../lib/components/molecules/PropertyList/PropertyList.svelte';
  import SourceProgressStrip from '../../../lib/components/molecules/SourceProgressStrip/SourceProgressStrip.svelte';
  import ConfirmDialog from '../../../lib/components/organisms/ConfirmDialog/ConfirmDialog.svelte';
  import DataTable, { type Column } from '../../../lib/components/organisms/DataTable/DataTable.svelte';
  import ReleaseResults from '../../../lib/components/organisms/ReleaseResults/ReleaseResults.svelte';
  import Drawer from '../../../lib/components/organisms/Drawer/Drawer.svelte';
  import { formatBytes } from '../../../lib/format/quantity';
  import { app } from '../../context.svelte';
  import { Resource } from '../../resource.svelte';
  import { refetchOn } from '../../live.svelte';
  import { toApiError } from '../../api/client';
  import type { AlbumDetail, Decided, SearchSummary, Track } from '../../api/types';
  import { grabFailure, InteractiveSearch, nothingAcceptedNote, sortOptions, type SortKey } from '../../interactive.svelte';
  import { albumItemStatus, albumTypes, albumYear, duration, trackPosition } from '../../music';
  import type { ScreenProps } from '../../screen';

  let { params = {} }: ScreenProps = $props();
  const { api, live, toasts, navigate } = app();
  // svelte-ignore state_referenced_locally
  const id = Number(params.id);

  const page = new Resource((s) => api.get<AlbumDetail>(`albums/${id}`, undefined, s));
  onMount(() => {
    void page.load();
    return refetchOn(live, ['item.*', 'import.completed', 'file.deleted'], () => void page.load(true), 600);
  });

  const al = $derived(page.data?.album);
  const artist = $derived(page.data?.artist);
  const tracks = $derived(page.data?.tracks ?? []);

  const entity = $derived({
    title: al?.title ?? 'Album', kind: 'music' as const, art: null,
    meta: [artist?.name, albumYear(al ?? { releaseDate: null }) || '', al ? albumTypes(al) : ''].filter(Boolean).join(' · '),
    back: artist ? { href: `#/library/music/${artist.id}`, label: artist.name } : { href: '#/library/music', label: 'Music' },
    monitor: (al?.monitored ? 'monitored' : 'unmonitored') as MonitorState,
    monitorReason: al?.pinReason ? `kept: ${al.pinReason === 'owner' ? 'monitored by hand' : al.pinReason}` : undefined,
    status: al ? [{ family: 'item' as const, value: albumItemStatus(al), detail: al.quality }] : [],
    facts: al ? [
      { label: 'Tracks on disk', value: `${al.fileCount} of ${al.trackCount}` },
      { label: 'Length', value: duration(al.durationSec) },
      { label: 'Size', value: formatBytes(al.sizeOnDisk) },
    ] : [],
  });

  async function setMonitor(s: MonitorState) {
    try {
      await api.put('albums/monitor', { ids: [id], monitored: s === 'monitored' });
      void page.load(true);
    } catch (e) {
      toasts.push({ message: 'Monitoring was not changed', detail: toApiError(e).message, tone: 'error' });
    }
  }

  let searching = $state(false);
  async function searchNow() {
    if (searching) return;
    searching = true;
    try {
      const s = await api.post<SearchSummary>(`albums/${id}/search`);
      toasts.push(s.grabbed
        ? { message: `Grabbed ${s.grabbed}`, tone: 'ok', actionLabel: 'Open queue', onaction: () => navigate('/activity/queue') }
        : { message: s.releases ? 'No release was good enough to grab' : 'No releases found', detail: `${s.releases} releases checked across ${s.indexers} indexers.`,
          tone: s.failed ? 'warn' : 'info', actionLabel: s.releases ? 'See why' : undefined, onaction: s.releases ? openInteractive : undefined });
    } catch (e) {
      toasts.push({ message: 'The search did not run', detail: toApiError(e).message, tone: 'error' });
    } finally {
      searching = false;
    }
  }

  // ── interactive search (drawer) ────────────────────────────────────
  const search = new InteractiveSearch(api);
  let drawerOpen = $state(false);
  function openInteractive() {
    drawerOpen = true;
    void search.start(`albums/${id}/releases`);
  }

  let confirmGrab = $state<{ d: Decided; reasons: string[] } | undefined>();
  let confirmOpen = $state(false);
  let grabbing = $state(false);
  function grabKey(key: string, over: unknown[]) {
    const d = search.decided.find((x) => x.release.id === key);
    if (d) void grab(d, over.length > 0);
  }
  async function grab(d: Decided, confirm: boolean) {
    grabbing = true;
    try {
      await api.post(`albums/${id}/grabs`, { releaseId: d.release.id, confirm });
      confirmOpen = false;
      drawerOpen = false;
      search.stop();
      toasts.push({ message: `Grab sent: ${d.release.title}`, detail: 'Watch it in the queue.', tone: 'ok', actionLabel: 'Open queue', onaction: () => navigate('/activity/queue') });
    } catch (e) {
      const f = grabFailure(e);
      if (f.confirm) { confirmGrab = { d, reasons: f.reasons }; confirmOpen = true; return; }
      confirmOpen = false;
      toasts.push({ message: f.refused ? 'corsarr will not grab this release' : 'The grab was not sent', detail: (f.refused && f.reasons[0]) || f.message, tone: 'error' });
    } finally {
      grabbing = false;
    }
  }

  const columns: Column<Track>[] = [
    { id: 'pos', label: '#', width: '3.5rem', phone: 'meta', value: (t) => trackPosition(t, al?.mediumCount ?? 1) },
    { id: 'title', label: 'Title', value: (t) => t.title, phone: 'title' },
    { id: 'length', label: 'Length', width: '5rem', align: 'end', phone: 'trailing', value: (t) => duration(t.durationSec) },
    { id: 'quality', label: 'File', width: '8rem', phone: 'hidden', value: (t) => t.file?.quality ?? 'Missing' },
  ];
</script>

{#snippet headActions()}
  <Button icon="search" loading={searching} disabled={searching} onclick={searchNow}>Search now</Button>
  <Button variant="primary" icon="list" onclick={openInteractive}>Interactive search</Button>
{/snippet}

{#snippet body()}
  {#if !al}
    <Skeleton lines={6} />
  {:else}
    <DataTable label="{al.title} tracks" rows={tracks} {columns} rowKey={(t) => String(t.id)} noun="track"
      height="min(40rem, calc({Math.max(tracks.length, 3)} * 2.75rem + 3rem))">
      {#snippet empty()}<EmptyState size="sm" title="The track list is not known yet" icon="music" level={3}>It is fetched from MusicBrainz in the background; refresh the artist if it stays empty.</EmptyState>{/snippet}
    </DataTable>
  {/if}
{/snippet}

{#snippet aside()}
  {#if al}
    <PropertyList label="Edition" items={[
      { key: 'discs', label: 'Discs', value: String(al.mediumCount || '—') },
      { key: 'edition', label: 'Edition', value: al.editionMbid || 'not chosen yet', mono: true },
      { key: 'mb', label: 'MusicBrainz', value: al.mbid, href: `https://musicbrainz.org/release-group/${al.mbid}` },
    ]} />
  {/if}
{/snippet}

{#snippet drawer()}
  <Drawer bind:open={drawerOpen} title="Interactive search" subtitle="{artist?.name ?? ''} – {al?.title ?? ''}" size="lg" onclose={() => search.stop()}>
    <div class="search">
      {#if search.sources.length}<SourceProgressStrip sources={search.sources} label="Indexers and sources" />{/if}
      {#if search.error}
        <ErrorState size="sm" title="The search stopped" code={search.error.label} onretry={openInteractive} healthHref="#/system/health" level={3}>{search.error.message}</ErrorState>
      {/if}
      {#if search.summary && !search.ranked.length}
        <EmptyState size="sm" title={search.summary.indexers ? 'No releases found' : 'No indexers are configured'} icon="search" level={3}>
          {search.summary.indexers ? 'None of the indexers or sources had a release of this album.' : 'Add an indexer or a music source to the config, then search again.'}
        </EmptyState>
      {/if}
      {#if search.ranked.length}
        <ReleaseResults items={search.sorted.map((d) => ({ key: d.release.id, decision: search.panel(d, al?.durationSec || undefined) }))}
          sort={search.sort} {sortOptions} onsort={(v) => search.setSort(v as SortKey)} ongrab={grabKey}
          note={search.summary && !search.accepted ? nothingAcceptedNote(search.ranked.length, search.summary.rejections) : undefined} />
      {/if}
      {#if search.streaming}
        <div class="pending" aria-busy="true" aria-label="Waiting for indexers"><Skeleton shape="block" height="6.5rem" /></div>
      {/if}
    </div>
  </Drawer>
{/snippet}

<DetailPage entity={{ ...entity, actions: al ? headActions : undefined, onmonitor: setMonitor }} children={body} {aside} {drawer}
  loading={page.loading && !al} error={page.error?.message} errorCode={page.error?.label} onretry={page.retry} />

{#if confirmGrab}
  <ConfirmDialog bind:open={confirmOpen} variant="primary" title="Grab {confirmGrab.d.release.title} anyway?" confirmLabel="Grab anyway" loading={grabbing}
    consequences={confirmGrab.reasons.length ? confirmGrab.reasons : ['The release failed a rule that needs your confirmation.']} undoable="You can remove the download from the queue afterwards."
    onconfirm={() => grab(confirmGrab!.d, true)} oncancel={() => (confirmOpen = false)} />
{/if}

<style>
  .search { display: grid; gap: var(--space-4); }
</style>
