<!--
  Page composition (story only): the artist page (albums by type and date with monitoring,
  track progress, status and quality; releases outside the metadata profile behind a switch)
  and the album page (tracks of the chosen edition; `search` opens interactive search with a
  direct-source result ranked next to torrents).
-->
<script lang="ts">
  import Shell from './Shell.svelte';
  import DetailPage from '../lib/components/templates/DetailPage/DetailPage.svelte';
  import Button from '../lib/components/atoms/Button/Button.svelte';
  import Progress from '../lib/components/atoms/Progress/Progress.svelte';
  import Quantity from '../lib/components/atoms/Quantity/Quantity.svelte';
  import StatusBadge from '../lib/components/atoms/StatusBadge/StatusBadge.svelte';
  import Switch from '../lib/components/atoms/Switch/Switch.svelte';
  import MonitorToggle from '../lib/components/molecules/MonitorToggle/MonitorToggle.svelte';
  import PropertyList from '../lib/components/molecules/PropertyList/PropertyList.svelte';
  import SourceProgressStrip from '../lib/components/molecules/SourceProgressStrip/SourceProgressStrip.svelte';
  import DataTable, { type Column } from '../lib/components/organisms/DataTable/DataTable.svelte';
  import DecisionPanel from '../lib/components/organisms/DecisionPanel/DecisionPanel.svelte';
  import Drawer from '../lib/components/organisms/Drawer/Drawer.svelte';
  import { NOW } from '../lib/storybook/fixtures';
  import { lightsAlbums, lowTideDecisions, lowTideTracks, type AlbumRow, type TrackRow } from '../lib/storybook/music-fixtures';

  let { page = 'artist', search = false, all = false }: { page?: 'artist' | 'album'; search?: boolean; all?: boolean } = $props();
  // svelte-ignore state_referenced_locally
  let open = $state(search);
  // svelte-ignore state_referenced_locally
  let showAll = $state(all);

  const albums = $derived(showAll ? lightsAlbums : lightsAlbums.filter((a) => a.inProfile || a.monitored || a.files > 0));
  const albumColumns: Column<AlbumRow>[] = [
    { id: 'monitored', label: 'Monitored', width: '3rem', hideLabel: true, phone: 'hidden' },
    { id: 'title', label: 'Album', value: (a) => a.title, phone: 'title' },
    { id: 'year', label: 'Year', width: '4rem', phone: 'meta', value: (a) => a.year },
    { id: 'type', label: 'Type', width: '7rem', phone: 'hidden', value: (a) => a.type },
    { id: 'tracks', label: 'Tracks', width: '7rem', phone: 'trailing', value: (a) => `${a.files}/${a.tracks}` },
    { id: 'status', label: 'Status', width: '9rem', phone: 'hidden' },
    { id: 'size', label: 'Size', width: '6rem', align: 'end', phone: 'hidden', value: (a) => a.size },
  ];
  const trackColumns: Column<TrackRow>[] = [
    { id: 'pos', label: '#', width: '3.5rem', phone: 'meta', value: (t) => t.pos },
    { id: 'title', label: 'Title', value: (t) => t.title, phone: 'title' },
    { id: 'length', label: 'Length', width: '5rem', align: 'end', phone: 'trailing', value: (t) => t.length },
    { id: 'quality', label: 'File', width: '8rem', phone: 'hidden', value: (t) => t.file ?? 'Missing' },
  ];

  const artist = {
    title: 'Harbour Lights', kind: 'music' as const, kindLabel: 'Artist', art: null, meta: 'Group · GB', monitor: 'monitored' as const,
    status: [{ family: 'item' as const, value: 'missing' }], back: { href: '#/library/music', label: 'Music' },
    facts: [{ label: 'Albums on disk', value: '4 of 6 monitored' }, { label: 'Tracks', value: '41' }, { label: 'Size', value: '940 MB' }],
    menu: [{ id: 'refresh', label: 'Refresh metadata', icon: 'refresh' as const }, { id: 'delete', label: 'Delete…', icon: 'trash' as const, danger: true }],
  };
  const album = {
    title: 'Low Tide', kind: 'music' as const, art: null, meta: 'Harbour Lights · 2025 · Album', monitor: 'monitored' as const,
    status: [{ family: 'item' as const, value: 'missing', detail: 'FLAC' }], back: { href: '#/library/music/1', label: 'Harbour Lights' },
    facts: [{ label: 'Tracks on disk', value: '9 of 11' }, { label: 'Length', value: '48:07' }, { label: 'Size', value: '255 MB' }],
  };
  const sources = [
    { id: 'tracker', name: 'Harbour Tracker', state: 'ok' as const, count: 3, ms: 840 },
    { id: 'camp', name: 'TrubadurCamp', state: 'ok' as const, count: 1, ms: 1310 },
  ];
</script>

{#snippet albumCell(a: AlbumRow, c: Column<AlbumRow>)}
  {#if c.id === 'monitored'}<MonitorToggle item={a.title} state={a.monitored ? 'monitored' : 'unmonitored'} size="sm" />
  {:else if c.id === 'tracks'}{#if a.tracks}<Progress label="Tracks of {a.title} on disk" done={a.files} total={a.tracks} size="sm" showValue tone={a.files >= a.tracks ? 'ok' : 'warn'} />{:else}—{/if}
  {:else if c.id === 'status'}<StatusBadge family="item" value={a.status} size="sm" detail={a.quality} />
  {:else if c.id === 'size'}{#if a.size}<Quantity kind="bytes" value={a.size} />{:else}—{/if}
  {:else}{c.value?.(a)}{/if}
{/snippet}

{#snippet artistActions()}<Button variant="primary" icon="search">Search wanted albums</Button>{/snippet}
{#snippet albumActions()}<Button icon="search">Search now</Button><Button variant="primary" icon="list">Interactive search</Button>{/snippet}

{#snippet artistBody()}
  <section class="albums">
    <div class="bar"><h2>Albums</h2><Switch label="Show releases outside the metadata profile (1)" bind:checked={showAll} /></div>
    <DataTable label="Harbour Lights albums" rows={albums} columns={albumColumns} rowKey={(a) => a.id} cell={albumCell} noun="album"
      rowActions={() => [{ id: 'open', label: 'Open', icon: 'arrow-right' }, { id: 'search', label: 'Search this album', icon: 'search' }]} />
  </section>
{/snippet}

{#snippet albumBody()}
  <DataTable label="Low Tide tracks" rows={lowTideTracks} columns={trackColumns} rowKey={(t) => t.id} noun="track" height="34rem" />
{/snippet}

{#snippet artistAside()}
  <PropertyList label="Library" items={[
    { key: 'path', label: 'Folder', value: '/data/media/music/Harbour Lights', mono: true, copy: true },
    { key: 'albums', label: 'Release groups', value: '6 in profile, 7 release groups in all' },
  ]} />
{/snippet}

{#snippet albumAside()}
  <PropertyList label="Edition" items={[{ key: 'discs', label: 'Discs', value: '1' }, { key: 'format', label: 'Format', value: 'Digital Media' }]} />
{/snippet}

{#snippet drawer()}
  <Drawer bind:open title="Interactive search" subtitle="Harbour Lights – Low Tide" size="lg">
    <div class="search">
      <SourceProgressStrip {sources} label="Indexers and sources" />
      <ol class="results" aria-label="Releases, best first">
        {#each lowTideDecisions as d (d.release.title)}<li><DecisionPanel decision={d} now={NOW} ongrab={() => {}} /></li>{/each}
      </ol>
    </div>
  </Drawer>
{/snippet}

{#if page === 'artist'}
  <Shell path="/library/music/1">
    <DetailPage entity={{ ...artist, actions: artistActions }} children={artistBody} aside={artistAside} />
  </Shell>
{:else}
  <Shell path="/library/music/albums/1">
    <DetailPage entity={{ ...album, actions: albumActions }} children={albumBody} aside={albumAside} {drawer} />
  </Shell>
{/if}

<style>
  .albums { display: grid; gap: var(--space-3); }
  .bar { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: var(--space-2); }
  h2 { font-size: var(--text-lg); }
  .search { display: grid; gap: var(--space-4); }
  .results { list-style: none; margin: 0; padding: 0; display: grid; gap: var(--space-3); }
</style>
