<!-- Activity › History: what corsarr did and why, newest first, cursor paged, rows expand to details. -->
<script lang="ts">
  import { onMount } from 'svelte';
  import ListPage from '../../../lib/components/templates/ListPage/ListPage.svelte';
  import MediaKindIcon from '../../../lib/components/atoms/MediaKindIcon/MediaKindIcon.svelte';
  import StatusBadge from '../../../lib/components/atoms/StatusBadge/StatusBadge.svelte';
  import Timestamp from '../../../lib/components/atoms/Timestamp/Timestamp.svelte';
  import EmptyState from '../../../lib/components/molecules/EmptyState/EmptyState.svelte';
  import PropertyList from '../../../lib/components/molecules/PropertyList/PropertyList.svelte';
  import type { Column } from '../../../lib/components/organisms/DataTable/DataTable.svelte';
  import { emptyFilter, type FilterFacet, type FilterState } from '../../../lib/filters/url';
  import { formatAbsolute } from '../../../lib/format/time';
  import { app } from '../../context.svelte';
  import { Resource } from '../../resource.svelte';
  import { toApiError } from '../../api/client';
  import type { HistoryEntry } from '../../api/types';
  import { actorLabel, eventDetail, eventSubject, eventView } from '../../model';
  import { applyList } from '../../list';

  const { api, live, toasts } = app();

  let rowsAll = $state<HistoryEntry[]>([]);
  let cursor = $state<string | undefined>();
  let loadingMore = $state(false);
  let expanded = $state<string[]>([]);
  const first = new Resource(async (s) => {
    const p = await api.page<HistoryEntry>('history', undefined, 100, undefined, s);
    rowsAll = p.items;
    cursor = p.nextCursor;
    return p;
  });
  async function more() {
    if (!cursor || loadingMore) return;
    loadingMore = true;
    try {
      const p = await api.page<HistoryEntry>('history', cursor, 100);
      rowsAll = [...rowsAll, ...p.items];
      cursor = p.nextCursor;
    } catch (e) {
      toasts.push({ message: "Couldn't load older history", detail: toApiError(e).message, tone: 'error' });
    } finally {
      loadingMore = false;
    }
  }

  onMount(() => {
    void first.load();
    // New history arrives as events; prepend once the server has stored it (refetch the newest page).
    let t: ReturnType<typeof setTimeout> | undefined;
    const newest = async () => {
      try {
        const p = await api.page<HistoryEntry>('history', undefined, 50);
        const known = new Set(rowsAll.map((r) => r.id));
        const fresh = p.items.filter((r) => !known.has(r.id));
        if (fresh.length) rowsAll = [...fresh, ...rowsAll];
      } catch { /* the next event or reconnect retries */ }
    };
    const offs = [
      live.on('*', (ev) => { if (persisted.has(ev.type) && !t) t = setTimeout(() => { t = undefined; void newest(); }, 700); }),
      live.onreconnect(() => void newest()),
    ];
    return () => { clearTimeout(t); offs.forEach((off) => off()); };
  });
  const persisted = new Set(['item.added', 'item.deleted', 'grab.sent', 'grab.failed', 'decision.shadowed', 'downloader.shadowed', 'download.failed', 'download.removed', 'import.completed', 'import.blocked',
    'import.failed', 'payload.rejected', 'file.renamed', 'file.deleted', 'blocklist.added', 'blocklist.removed', 'rule.acted', 'rule.undone', 'health.changed',
    'config.applied', 'config.apply_failed', 'config.drifted', 'mediakind.mode_changed']);

  let filter = $state<FilterState>(emptyFilter());
  const typeGroups: Record<string, string[]> = {
    grabs: ['grab.sent', 'grab.failed', 'decision.shadowed'], imports: ['import.completed', 'import.blocked', 'import.failed', 'file.renamed', 'file.deleted'],
    downloads: ['download.failed', 'download.removed', 'payload.rejected', 'downloader.shadowed'], library: ['item.added', 'item.deleted'],
    blocklist: ['blocklist.added', 'blocklist.removed'], rules: ['rule.acted', 'rule.undone'], health: ['health.changed'],
    config: ['config.applied', 'config.apply_failed', 'config.drifted', 'mediakind.mode_changed'],
  };
  const groupOf = (type: string) => Object.entries(typeGroups).find(([, t]) => t.includes(type))?.[0] ?? 'other';
  const actorOf = (a: string) => (a === 'user' ? 'you' : a === 'automation' ? 'auto' : a === 'system' ? 'system' : 'other');
  const facets: FilterFacet[] = [
    { id: 'type', label: 'Event', options: [
      { value: 'grabs', label: 'Grabs' }, { value: 'imports', label: 'Imports and files' }, { value: 'downloads', label: 'Downloads' }, { value: 'library', label: 'Library' },
      { value: 'blocklist', label: 'Blocklist' }, { value: 'rules', label: 'Rules' }, { value: 'health', label: 'Health' }, { value: 'config', label: 'Configuration' },
    ] },
    { id: 'actor', label: 'By', options: [{ value: 'you', label: 'You' }, { value: 'auto', label: 'Automatic' }, { value: 'system', label: 'corsarr' }, { value: 'other', label: 'Plugins and rules' }] },
  ];
  const rows = $derived(applyList(rowsAll, filter, undefined, {
    text: (h) => `${eventSubject(h.event)} ${eventDetail(h.event) ?? ''} ${eventView(h.event).label}`,
    facets: { type: (h) => groupOf(h.event.type), actor: (h) => actorOf(h.event.actor) },
  }));

  const columns: Column<HistoryEntry>[] = [
    { id: 'at', label: 'When', width: '7rem', value: (r) => r.event.occurredAt, phone: 'meta' },
    { id: 'title', label: 'Item', value: (r) => eventSubject(r.event), phone: 'title' },
    { id: 'outcome', label: 'Event', width: '12rem', value: (r) => eventView(r.event).label, phone: 'trailing' },
    { id: 'actor', label: 'By', width: '8rem', value: (r) => actorLabel(r.event.actor), phone: 'hidden' },
  ];
</script>

{#snippet cell(r: HistoryEntry, c: Column<HistoryEntry>)}
  {#if c.id === 'at'}<Timestamp value={r.event.occurredAt} />
  {:else if c.id === 'title'}<span class="title">{#if r.event.items?.length}<MediaKindIcon kind={r.event.items[0].ref.kind} />{/if}<span>{eventSubject(r.event)}</span></span>
  {:else if c.id === 'outcome'}{@const v = eventView(r.event)}<StatusBadge family={v.family} value={v.value} label={v.label} size="sm" appearance="plain" />
  {:else}{c.value?.(r)}{/if}
{/snippet}

{#snippet detail(r: HistoryEntry)}
  {@const item = r.event.items?.[0]}
  <PropertyList density="compact" label="Details for {eventSubject(r.event)}" items={[
    { key: 'what', label: 'What happened', value: eventDetail(r.event) ?? eventView(r.event).label },
    { key: 'when', label: 'When', value: formatAbsolute(new Date(r.event.occurredAt)) },
    { key: 'actor', label: 'By', value: actorLabel(r.event.actor) },
    { key: 'type', label: 'Event type', value: r.event.type, mono: true },
    ...(item && item.ref.kind === 'movie' ? [{ key: 'item', label: 'Item', value: item.title, href: `#/library/movies/${item.ref.id}` }] : []),
    ...(item && item.ref.kind === 'tv' && item.ref.variant === 'series' ? [{ key: 'item', label: 'Series', value: item.title, href: `#/library/tv/${item.ref.id}` }] : []),
    ...(item && item.ref.kind === 'tv' && item.ref.variant !== 'series' && item.series ? [{ key: 'item', label: 'Series', value: item.series }] : []),
  ]} />
{/snippet}

<ListPage title="History" description="Everything corsarr did and why, newest first" {facets} bind:filter syncUrl searchLabel="Filter history" placeholder="Filter by item or release"
  {rows} {columns} rowKey={(r) => String(r.id)} {cell} expand={detail} bind:expanded noun="event" hasMore={!!cursor} {loadingMore} onloadmore={more}
  loading={first.loading && !first.data} error={first.error?.message} errorCode={first.error?.label} onretry={first.retry}>
  {#snippet empty()}
    <EmptyState title="Nothing has happened yet" icon="clock" level={2}>
      Grabs, imports, blocklist changes and health changes are recorded here as they happen.
    </EmptyState>
  {/snippet}
</ListPage>

<style>
  .title { display: flex; align-items: center; gap: var(--space-2); min-width: 0; }
  .title span { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
</style>
