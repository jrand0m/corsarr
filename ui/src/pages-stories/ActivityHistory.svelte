<!-- Page composition (story only): Activity › History, events stream with expandable rows. -->
<script lang="ts">
  import Shell from './Shell.svelte';
  import ListPage from '../lib/components/templates/ListPage/ListPage.svelte';
  import MediaKindIcon from '../lib/components/atoms/MediaKindIcon/MediaKindIcon.svelte';
  import StatusBadge from '../lib/components/atoms/StatusBadge/StatusBadge.svelte';
  import Timestamp from '../lib/components/atoms/Timestamp/Timestamp.svelte';
  import EmptyState from '../lib/components/molecules/EmptyState/EmptyState.svelte';
  import PropertyList from '../lib/components/molecules/PropertyList/PropertyList.svelte';
  import SegmentedControl from '../lib/components/molecules/SegmentedControl/SegmentedControl.svelte';
  import type { Column } from '../lib/components/organisms/DataTable/DataTable.svelte';
  import { historyRows, nowDate, type HistoryRow } from '../lib/storybook/fixtures';

  let { expanded = [], stream = 'events' }: { expanded?: string[]; stream?: string } = $props();

  // svelte-ignore state_referenced_locally
  let current = $state(stream);
  // svelte-ignore state_referenced_locally
  let open = $state(expanded);

  const columns: Column<HistoryRow>[] = [
    { id: 'at', label: 'When', width: '7rem', value: (r) => r.at, phone: 'meta' },
    { id: 'title', label: 'Item', value: (r) => r.title, phone: 'title' },
    { id: 'outcome', label: 'Event', width: '10rem', value: (r) => r.type, phone: 'trailing' },
    { id: 'source', label: 'Source', width: '9rem', value: (r) => r.source, phone: 'hidden' },
    { id: 'actor', label: 'By', width: '11rem', value: (r) => r.actor, phone: 'hidden' },
  ];
  const facets = [
    { id: 'kind', label: 'Kind', options: [{ value: 'tv', label: 'TV' }, { value: 'movie', label: 'Movies' }, { value: 'music', label: 'Music' }] },
    { id: 'type', label: 'Event', options: ['Imported', 'Grabbed', 'Import blocked', 'Upgraded', 'Rejected', 'Removed'].map((t) => ({ value: t, label: t })) },
    { id: 'actor', label: 'By', options: [{ value: 'auto', label: 'Automatic' }, { value: 'you', label: 'You' }, { value: 'rule', label: 'Rules' }] },
  ];
</script>

{#snippet cell(r: HistoryRow, c: Column<HistoryRow>)}
  {#if c.id === 'at'}<Timestamp value={r.at} now={nowDate} />
  {:else if c.id === 'title'}<span class="title"><MediaKindIcon kind={r.kind} /><span>{r.title}</span></span>
  {:else if c.id === 'outcome'}<StatusBadge family={r.outcome.family} value={r.outcome.value} label={r.type} size="sm" appearance="plain" />
  {:else}{c.value?.(r)}{/if}
{/snippet}

{#snippet detail(r: HistoryRow)}
  <PropertyList density="compact" label="Details for {r.title}" items={[
    { key: 'what', label: 'What happened', value: r.detail },
    { key: 'when', label: 'When', value: new Date(r.at).toUTCString().slice(5, 22) },
    { key: 'source', label: 'Source', value: r.source },
    { key: 'actor', label: 'By', value: r.actor },
    { key: 'item', label: 'Item', value: r.title, href: '#/library/movies/7' },
  ]} />
{/snippet}

<Shell path="/activity/history">
  <ListPage title="History" description="Everything corsarr did and why, newest first" {facets} searchLabel="Filter history" placeholder="Filter by item or release"
    rows={current === 'events' ? historyRows : []} {columns} rowKey={(r) => r.id} {cell} expand={detail} bind:expanded={open} noun="event" total={2840} hasMore={current === 'events'}>
    {#snippet actions()}
      <SegmentedControl label="Stream" bind:value={current} options={[
        { value: 'events', label: 'Events' }, { value: 'deliveries', label: 'Deliveries' }, { value: 'queries', label: 'Queries' },
      ]} />
    {/snippet}
    {#snippet empty()}
      <EmptyState title="No {current} recorded yet" icon="clock" level={2}>
        {current === 'deliveries' ? 'Notification deliveries show up here once a connection sends its first message.' : 'Indexer queries show up here after the next search or feed sync.'}
      </EmptyState>
    {/snippet}
  </ListPage>
</Shell>

<style>
  .title { display: flex; align-items: center; gap: var(--space-2); min-width: 0; }
  .title span { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
</style>
