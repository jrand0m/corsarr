<!--
  Page composition (story only): movie detail with the interactive search drawer. Results
  stream in per indexer as they answer (source progress strip on top, decision panels ranked below).
-->
<script lang="ts">
  import Shell from './Shell.svelte';
  import DetailPage from '../lib/components/templates/DetailPage/DetailPage.svelte';
  import Button from '../lib/components/atoms/Button/Button.svelte';
  import Skeleton from '../lib/components/atoms/Skeleton/Skeleton.svelte';
  import PropertyList from '../lib/components/molecules/PropertyList/PropertyList.svelte';
  import QueueCard from '../lib/components/molecules/QueueCard/QueueCard.svelte';
  import SourceProgressStrip, { type SourceProgress } from '../lib/components/molecules/SourceProgressStrip/SourceProgressStrip.svelte';
  import EmptyState from '../lib/components/molecules/EmptyState/EmptyState.svelte';
  import Drawer from '../lib/components/organisms/Drawer/Drawer.svelte';
  import DecisionPanel from '../lib/components/organisms/DecisionPanel/DecisionPanel.svelte';
  import { ToastQueue } from '../lib/toast/toasts.svelte';
  import { docksideDecisions, docksideHistory, downloads, NOW, nowDate, searchSources } from '../lib/storybook/fixtures';

  interface Props { search?: boolean; stream?: boolean }
  let { search = true, stream = true }: Props = $props();

  // svelte-ignore state_referenced_locally
  let open = $state(search);
  // svelte-ignore state_referenced_locally
  let shown = $state(stream ? 0 : docksideDecisions.length);
  const toasts = new ToastQueue();

  // Indexers answer one by one: Harbour first (2 results), Dock next (1), Usenet Pier last (1); Slow Slug fails.
  const answeredAt = [1, 2, 4];
  const sources = $derived<SourceProgress[]>(searchSources.map((s, i) => {
    if (s.state === 'failed') return shown >= 1 ? s : { ...s, state: 'running', reason: undefined };
    const done = shown >= (answeredAt[i] ?? 9);
    return done ? { ...s, state: 'ok', count: i === 2 ? 1 : s.count } : { ...s, state: 'running', count: undefined, ms: undefined };
  }));
  const decisions = $derived(docksideDecisions.slice(0, shown).sort((a, b) => (a.rank ?? 0) - (b.rank ?? 0)));
  $effect(() => {
    if (!open || !stream) return;
    shown = 0;
    const id = setInterval(() => { shown = Math.min(docksideDecisions.length, shown + 1); if (shown >= docksideDecisions.length) clearInterval(id); }, 700);
    return () => clearInterval(id);
  });

  const entity = {
    title: 'Dockside', kind: 'movie' as const, art: null, meta: '2026 · Harbourline Pictures · 1h 52m', monitor: 'monitored' as const,
    status: [{ family: 'item' as const, value: 'queued', detail: 'downloading' }],
    facts: [{ label: 'Profile', value: 'HD' }, { label: 'Availability', value: 'Released 14 Nov' }, { label: 'Folder', value: '/data/media/movies/Dockside (2026)' }],
    back: { href: '#/library/movies', label: 'Movies' },
    menu: [
      { id: 'refresh', label: 'Refresh metadata', icon: 'refresh' as const },
      { id: 'rename', label: 'Preview renames…', icon: 'pencil' as const },
      { id: 'delete', label: 'Delete movie…', icon: 'trash' as const, danger: true, group: 'danger' },
    ],
  };
  const tabs = [{ id: 'overview', label: 'Overview' }, { id: 'files', label: 'Files', count: 0 }];
</script>

{#snippet headActions()}
  <Button icon="search">Search</Button>
  <Button variant="primary" icon="list" onclick={() => (open = true)}>Interactive search</Button>
{/snippet}

{#snippet body(id: string)}
  {#if id === 'overview'}
    <div class="overview">
      <p>A harbour pilot finds a container that is not on any manifest, and the night shift has until the tide turns to decide who it belongs to.</p>
      <PropertyList label="Release" items={[
        { key: 'cinema', label: 'In cinemas', value: '3 Oct 2026' }, { key: 'digital', label: 'Digital', value: '14 Nov 2026' },
        { key: 'min', label: 'Minimum availability', value: 'Digital release' }, { key: 'tags', label: 'Tags', value: 'thriller' },
      ]} />
    </div>
  {:else}
    <EmptyState size="sm" title="No file yet" icon="file" level={3}>The download is at 46%; the file appears here once it is imported.</EmptyState>
  {/if}
{/snippet}

{#snippet aside()}
  <section class="side">
    <h2>Downloading</h2>
    <QueueCard download={downloads[0]} now={NOW} onaction={() => {}} />
  </section>
  <PropertyList label="Identifiers" items={[{ key: 'tmdb', label: 'TMDB', value: '10101', href: '#' }, { key: 'imdb', label: 'IMDb', value: 'tt3141592', href: '#' }]} />
{/snippet}

{#snippet drawer()}
  <Drawer bind:open title="Interactive search" subtitle="Dockside (2026) · HD profile" size="lg">
    <div class="search">
      <SourceProgressStrip sources={sources} label="Indexers" />
      {#if decisions.length}
        <ol class="results" aria-label="Releases, best first">
          {#each decisions as d (d.release.title)}
            <li><DecisionPanel decision={d} now={NOW} ongrab={() => toasts.push({ message: `Grab sent: ${d.release.title}`, detail: 'Watch it in the queue.', actionLabel: 'Open queue', onaction: () => {} })} /></li>
          {/each}
        </ol>
      {/if}
      {#if shown < docksideDecisions.length}
        <div class="pending" aria-hidden="true"><Skeleton shape="block" height="6.5rem" /></div>
      {/if}
    </div>
  </Drawer>
{/snippet}

<Shell path="/library/movies/dockside" {toasts}>
  <DetailPage entity={{ ...entity, actions: headActions }} {tabs} children={body} {aside} history={docksideHistory} now={nowDate} {drawer} />
</Shell>

<style>
  .overview { display: grid; gap: var(--space-4); max-width: var(--measure); }
  .side { display: grid; gap: var(--space-3); }
  h2 { font-size: var(--text-lg); }
  .search { display: grid; gap: var(--space-4); }
  .results { list-style: none; margin: 0; padding: 0; display: grid; gap: var(--space-3); }
</style>
