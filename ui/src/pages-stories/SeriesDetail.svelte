<!--
  Page composition (story only): S06 series detail. Seasons newest first with monitoring, counts,
  a bay grid and season search; the open season lists its episodes. `search` opens the interactive search drawer
  for a season pack with decisions already in.
-->
<script lang="ts">
  import Shell from './Shell.svelte';
  import DetailPage from '../lib/components/templates/DetailPage/DetailPage.svelte';
  import BayGrid from '../lib/components/atoms/BayGrid/BayGrid.svelte';
  import Button from '../lib/components/atoms/Button/Button.svelte';
  import Progress from '../lib/components/atoms/Progress/Progress.svelte';
  import Quantity from '../lib/components/atoms/Quantity/Quantity.svelte';
  import StatusBadge from '../lib/components/atoms/StatusBadge/StatusBadge.svelte';
  import MonitorToggle from '../lib/components/molecules/MonitorToggle/MonitorToggle.svelte';
  import PropertyList from '../lib/components/molecules/PropertyList/PropertyList.svelte';
  import QueueCard from '../lib/components/molecules/QueueCard/QueueCard.svelte';
  import SourceProgressStrip from '../lib/components/molecules/SourceProgressStrip/SourceProgressStrip.svelte';
  import DataTable, { type Column } from '../lib/components/organisms/DataTable/DataTable.svelte';
  import DecisionPanel from '../lib/components/organisms/DecisionPanel/DecisionPanel.svelte';
  import Drawer from '../lib/components/organisms/Drawer/Drawer.svelte';
  import { artSwatch, docksideDecisions, docksideHistory, downloads, NOW, nowDate, searchSources } from '../lib/storybook/fixtures';
  import { bayCells, longHaulSeasons, type EpisodeRow, type SeasonFixture } from '../lib/storybook/tv-fixtures';

  let { search = false }: { search?: boolean } = $props();
  // svelte-ignore state_referenced_locally
  let open = $state(search);
  let expanded = $state<number[]>([3]);

  const name = (n: number) => (n === 0 ? 'Specials' : `Season ${n}`);
  const code = (r: EpisodeRow) => `S${String(r.season).padStart(2, '0')}E${String(r.number).padStart(2, '0')}`;
  const badge: Record<string, string> = {
    have: 'cutoff-met', upgrade: 'cutoff-unmet', downloading: 'queued', queued: 'queued', missing: 'missing', unaired: 'not-available', unmonitored: 'unmonitored', blocked: 'missing',
  };
  const day = (ms?: number) => (ms ? new Date(ms).toLocaleDateString('en-GB', { day: 'numeric', month: 'short', year: 'numeric', timeZone: 'UTC' }) : 'Not announced');
  const columns: Column<EpisodeRow>[] = [
    { id: 'monitored', label: 'Monitored', width: '3rem', hideLabel: true, phone: 'hidden' },
    { id: 'number', label: 'Episode', width: '5.5rem', value: code, phone: 'meta' },
    { id: 'title', label: 'Title', value: (r) => r.title || 'To be announced', phone: 'title' },
    { id: 'air', label: 'Air date', width: '8rem', value: (r) => day(r.airAt), phone: 'hidden' },
    { id: 'status', label: 'Status', width: '13rem', value: (r) => badge[r.state], phone: 'trailing' },
  ];

  const entity = {
    title: 'The Long Haul', kind: 'tv' as const, art: artSwatch(30), meta: '2019 · Harbourline · Continuing · 52m', monitor: 'monitored' as const,
    status: [{ family: 'item' as const, value: 'queued', detail: '1 download' }],
    facts: [{ label: 'Profile', value: 'HD' }, { label: 'Episodes', value: '21 of 23 on disk' }, { label: 'Next airing', value: '26 Nov 2026' }, { label: 'Folder', value: '/data/media/tv/The Long Haul' }],
    back: { href: '#/library/tv', label: 'TV' },
    menu: [
      { id: 'search', label: 'Search missing episodes', icon: 'search' as const },
      { id: 'refresh', label: 'Refresh metadata', icon: 'refresh' as const },
      { id: 'edit', label: 'Edit series…', icon: 'pencil' as const },
      { id: 'delete', label: 'Delete series…', icon: 'trash' as const, danger: true, group: 'danger' },
    ],
  };
  const tabs = [{ id: 'seasons', label: 'Seasons', count: longHaulSeasons.length }, { id: 'details', label: 'Details' }];
  const toggle = (n: number) => (expanded = expanded.includes(n) ? expanded.filter((x) => x !== n) : [...expanded, n]);
</script>

{#snippet headActions()}
  <Button icon="search">Search missing</Button>
  <Button icon="refresh">Refresh</Button>
  <Button variant="primary" icon="pencil">Edit</Button>
{/snippet}

{#snippet cell(r: EpisodeRow, c: Column<EpisodeRow>)}
  {#if c.id === 'monitored'}<MonitorToggle item={code(r)} state={r.monitored ? 'monitored' : 'unmonitored'} size="sm" />
  {:else if c.id === 'number'}<span class="num">{code(r)}</span>
  {:else if c.id === 'status'}<StatusBadge family="item" value={badge[r.state]} detail={r.quality ?? (r.progress ? `${Math.round(r.progress * 100)}%` : undefined)} size="sm" appearance="plain" />
  {:else}{c.value?.(r)}{/if}
{/snippet}

{#snippet season(s: SeasonFixture)}
  <section class="season" aria-labelledby="story-season-{s.number}">
    <div class="head">
      <MonitorToggle item={name(s.number)} state={s.monitored ? 'monitored' : 'unmonitored'} />
      <h2 id="story-season-{s.number}">{name(s.number)}</h2>
      <span class="counts num">{s.files}/{s.episodes} episodes{#if s.size} · <Quantity kind="bytes" value={s.size} />{/if}</span>
      <span class="bar"><Progress label="{name(s.number)} episodes on disk" done={s.files} total={s.episodes} size="sm" tone={s.files >= s.episodes ? 'ok' : 'warn'} /></span>
      <span class="tools">
        <Button size="sm" icon="search">Search</Button>
        <Button size="sm" icon="list" onclick={() => (open = true)}>Interactive</Button>
        <Button size="sm" variant="ghost" iconEnd={expanded.includes(s.number) ? 'chevron-up' : 'chevron-down'} aria-expanded={expanded.includes(s.number)} onclick={() => toggle(s.number)}>
          {expanded.includes(s.number) ? 'Hide episodes' : 'Show episodes'}
        </Button>
      </span>
    </div>
    <BayGrid cells={bayCells(s)} label={name(s.number)} size="sm" />
    {#if expanded.includes(s.number)}
      <DataTable label="{name(s.number)} episodes" rows={s.rows} {columns} rowKey={(r) => r.id} {cell} noun="episode"
        height="min(32rem, calc({s.rows.length} * var(--ep-row) + 3rem))"
        rowActions={() => [{ id: 'search', label: 'Search now', icon: 'search' }, { id: 'interactive', label: 'Interactive search', icon: 'list' }]} />
    {/if}
  </section>
{/snippet}

{#snippet body(tab: string)}
  {#if tab === 'seasons'}
    <div class="seasons">{#each longHaulSeasons as s (s.number)}{@render season(s)}{/each}</div>
  {:else}
    <div class="overview">
      <p>A night-shift crew keeps a container port running while the cargo, the weather and the owners all want something different.</p>
      <PropertyList label="Series" items={[
        { key: 'status', label: 'Status', value: 'Continuing' }, { key: 'network', label: 'Network', value: 'Harbourline' },
        { key: 'genres', label: 'Genres', value: 'Drama, Crime' }, { key: 'new', label: 'New seasons', value: 'Monitored as they appear' },
        { key: 'folders', label: 'Season folders', value: 'Yes' }, { key: 'tags', label: 'Tags', value: 'drama' },
      ]} />
    </div>
  {/if}
{/snippet}

{#snippet aside()}
  <section class="side">
    <h2>Downloading</h2>
    <QueueCard download={{ ...downloads[0], title: 'The Long Haul · S03 · 2 episodes', subtitle: 'The.Long.Haul.S03E04-E05.1080p.WEB', kind: 'tv' }} now={NOW} onaction={() => {}} />
  </section>
  <PropertyList label="Identifiers" items={[{ key: 'tmdb', label: 'TMDB', value: '84012', href: '#' }, { key: 'tvdb', label: 'TVDB', value: '371950' }]} />
{/snippet}

{#snippet drawer()}
  <Drawer bind:open title="Interactive search" subtitle="The Long Haul · Season 3 · HD profile" size="lg">
    <div class="search">
      <SourceProgressStrip sources={searchSources} label="Indexers" />
      <ol class="results" aria-label="Releases, best first">
        {#each docksideDecisions as d (d.release.title)}<li><DecisionPanel decision={d} now={NOW} ongrab={() => {}} /></li>{/each}
      </ol>
    </div>
  </Drawer>
{/snippet}

<Shell path="/library/tv/the-long-haul">
  <DetailPage entity={{ ...entity, actions: headActions }} {tabs} children={body} {aside} history={docksideHistory} now={nowDate} {drawer} />
</Shell>

<style>
  .seasons { display: grid; gap: var(--space-6); --ep-row: var(--row-h-comfortable); }
  .season { display: grid; gap: var(--space-3); }
  .head { display: flex; align-items: center; gap: var(--space-2) var(--space-3); flex-wrap: wrap; }
  h2 { font-size: var(--text-lg); }
  .counts { font-size: var(--text-sm); color: var(--ink-muted); }
  .bar { flex: 1 1 6rem; min-width: 6rem; max-width: 14rem; }
  .tools { display: flex; gap: var(--space-2); flex-wrap: wrap; margin-left: auto; }
  .overview { display: grid; gap: var(--space-4); max-width: var(--measure); }
  .side, .search { display: grid; gap: var(--space-3); }
  .results { list-style: none; margin: 0; padding: 0; display: grid; gap: var(--space-3); }
  @media (width < 48rem) {
    .seasons { --ep-row: 4.5rem; }
    .tools { margin-left: 0; }
  }
</style>
