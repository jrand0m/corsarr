<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import DetailPage from './DetailPage.svelte';
  import Button from '../../atoms/Button/Button.svelte';
  import Banner from '../../molecules/Banner/Banner.svelte';
  import PropertyList from '../../molecules/PropertyList/PropertyList.svelte';
  import QueueCard from '../../molecules/QueueCard/QueueCard.svelte';
  import { docksideHistory, downloads, NOW, nowDate } from '../../../storybook/fixtures';

  const entity = {
    title: 'Dockside', kind: 'movie' as const, art: null, meta: '2026 · Harbourline Pictures · 1h 52m', monitor: 'monitored' as const,
    status: [{ family: 'item' as const, value: 'queued', detail: 'downloading' }],
    facts: [{ label: 'Profile', value: 'HD' }, { label: 'Availability', value: 'Released 14 Nov' }, { label: 'Folder', value: '/data/media/movies/Dockside (2026)' }],
    back: { href: '#/library/movies', label: 'Movies' },
  };
  const tabs = [{ id: 'overview', label: 'Overview' }, { id: 'files', label: 'Files', count: 0 }, { id: 'history', label: 'History' }];
  const phone = { viewport: { value: 'phone', isRotated: false } };
  const desktop = { viewport: { value: 'desktop', isRotated: false } };

  const { Story } = defineMeta({ title: 'Templates/DetailPage', component: DetailPage, tags: ['autodocs'] });
</script>

{#snippet body(id: string)}
  {#if id === 'overview'}
    <p style="max-width: var(--measure)">A harbour pilot finds a container that is not on any manifest, and the night shift has until the tide turns to decide who it belongs to.</p>
  {:else}
    <p>Section “{id}” goes here.</p>
  {/if}
{/snippet}
{#snippet aside()}
  <section style="display: grid; gap: var(--space-3)">
    <h2 style="font-size: var(--text-lg)">Active download</h2>
    <QueueCard download={downloads[0]} now={NOW} />
  </section>
  <PropertyList label="Identifiers" items={[{ key: 'tmdb', label: 'TMDB', value: '10101', href: '#' }, { key: 'imdb', label: 'IMDb', value: 'tt3141592', href: '#' }]} />
{/snippet}
{#snippet headActions()}<Button variant="primary" icon="search">Search</Button><Button icon="list">Interactive search</Button>{/snippet}

<Story name="Playground" asChild globals={desktop}>
  <DetailPage entity={{ ...entity, actions: headActions }} {tabs} children={body} {aside} history={docksideHistory} now={nowDate} />
</Story>

<Story name="Phone" asChild globals={{ ...phone, theme: 'dark' }}>
  <DetailPage entity={{ ...entity, actions: headActions }} {tabs} children={body} {aside} history={docksideHistory} now={nowDate} />
</Story>

<Story name="Stale metadata banner" asChild globals={desktop}>
  <DetailPage entity={entity} {tabs} children={body} now={nowDate}>
    {#snippet banner()}<Banner preset="stale" title="Metadata is 9 days old">TMDB has not answered since 13 Nov.{#snippet actions()}<Button size="sm" icon="refresh">Refresh now</Button>{/snippet}</Banner>{/snippet}
  </DetailPage>
</Story>

<Story name="Loading" asChild globals={desktop}>
  <DetailPage entity={entity} loading />
</Story>

<Story name="Error" asChild globals={desktop}>
  <DetailPage entity={entity} error="This movie is no longer in the library." errorCode="HTTP 404 · item.not_found" onretry={() => {}} />
</Story>
