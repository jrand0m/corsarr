<!-- Activity › Blocklist: releases corsarr will not grab again; lift a block with undo in the toast. -->
<script lang="ts">
  import { onMount } from 'svelte';
  import ListPage from '../../../lib/components/templates/ListPage/ListPage.svelte';
  import Button from '../../../lib/components/atoms/Button/Button.svelte';
  import MediaKindIcon from '../../../lib/components/atoms/MediaKindIcon/MediaKindIcon.svelte';
  import Timestamp from '../../../lib/components/atoms/Timestamp/Timestamp.svelte';
  import EmptyState from '../../../lib/components/molecules/EmptyState/EmptyState.svelte';
  import type { Column } from '../../../lib/components/organisms/DataTable/DataTable.svelte';
  import { emptyFilter, type FilterFacet, type FilterState } from '../../../lib/filters/url';
  import { app } from '../../context.svelte';
  import { Resource } from '../../resource.svelte';
  import { refetchOn } from '../../live.svelte';
  import { toApiError } from '../../api/client';
  import type { BlocklistEntry } from '../../api/types';
  import { applyList } from '../../list';

  const { api, live, movies, toasts } = app();

  let all = $state<BlocklistEntry[]>([]);
  let cursor = $state<string | undefined>();
  let loadingMore = $state(false);
  let selected = $state<string[]>([]);
  const first = new Resource(async (s) => {
    const p = await api.page<BlocklistEntry>('blocklist', undefined, 200, undefined, s);
    all = p.items;
    cursor = p.nextCursor;
    return p;
  });
  async function more() {
    if (!cursor || loadingMore) return;
    loadingMore = true;
    try {
      const p = await api.page<BlocklistEntry>('blocklist', cursor, 200);
      all = [...all, ...p.items];
      cursor = p.nextCursor;
    } finally {
      loadingMore = false;
    }
  }
  onMount(() => {
    void first.load();
    movies.ensure();
    return refetchOn(live, ['blocklist.*'], () => void first.load(true), 500);
  });

  const sourceLabel: Record<string, string> = { manual: 'Added by hand', user: 'You', downloader: 'Download failed', payload: 'Unsafe payload', rule: 'Rule' };
  let filter = $state<FilterState>(emptyFilter());
  const facets: FilterFacet[] = [{ id: 'source', label: 'Source', options: Object.entries(sourceLabel).map(([value, label]) => ({ value, label })) }];
  // TV entries point at an episode; there is no episode index here, so they read as "Episode #id".
  const itemName = (e: BlocklistEntry) => (e.item.kind === 'movie' ? movies.title(e.item.id) ?? `Movie #${e.item.id}` : `Episode #${e.item.id}`);
  const rows = $derived(applyList(all, filter, undefined, {
    text: (e) => `${e.release.title} ${itemName(e)} ${e.reason}`,
    facets: { source: (e) => e.source },
  }));

  async function lift(entries: BlocklistEntry[]) {
    const done: BlocklistEntry[] = [];
    for (const e of entries) {
      try {
        await api.del(`blocklist/${e.id}`);
        done.push(e);
      } catch (err) {
        toasts.push({ message: `Couldn't lift the block on ${e.release.title}`, detail: toApiError(err).message, tone: 'error' });
      }
    }
    if (!done.length) return;
    all = all.filter((r) => !done.includes(r));
    toasts.push({
      message: done.length === 1 ? `Lifted the block on ${done[0].release.title}` : `Lifted ${done.length} blocks`,
      detail: 'corsarr may grab these releases again.', tone: 'ok',
      onundo: async () => {
        for (const e of done) {
          await api.post('blocklist', { movieId: e.item.id, release: e.release, reason: e.reason }).catch((err) =>
            toasts.push({ message: `Couldn't block ${e.release.title} again`, detail: toApiError(err).message, tone: 'error' }));
        }
        void first.load(true);
      },
    });
  }

  const columns: Column<BlocklistEntry>[] = [
    { id: 'release', label: 'Release', value: (r) => r.release.title, phone: 'title' },
    { id: 'item', label: 'Item', width: '11rem', value: itemName, phone: 'meta' },
    { id: 'source', label: 'Source', width: '8rem', value: (r) => sourceLabel[r.source] ?? r.source, phone: 'hidden' },
    { id: 'reason', label: 'Reason', value: (r) => r.reason || '—', phone: 'hidden' },
    { id: 'added', label: 'Since', width: '7rem', value: (r) => r.addedAt, phone: 'trailing' },
  ];
</script>

{#snippet cell(r: BlocklistEntry, c: Column<BlocklistEntry>)}
  {#if c.id === 'release'}<span class="mono" title={r.release.title}>{r.release.title}</span>
  {:else if c.id === 'item'}<span class="item"><MediaKindIcon kind={r.item.kind} />{#if r.item.kind === 'movie'}<a href="#/library/movies/{r.item.id}">{itemName(r)}</a>{:else}<span>{itemName(r)}</span>{/if}</span>
  {:else if c.id === 'added'}<Timestamp value={r.addedAt} />
  {:else}{c.value?.(r)}{/if}
{/snippet}

<ListPage title="Blocklist" description={all.length ? `${all.length}${cursor ? '+' : ''} ${all.length === 1 && !cursor ? 'release' : 'releases'} corsarr will not grab again` : undefined} {facets} bind:filter bind:selected syncUrl
  searchLabel="Filter blocklist" placeholder="Filter by release or item" {rows} {columns} rowKey={(r) => String(r.id)} {cell} selectable noun="release"
  loading={first.loading && !first.data} error={first.error?.message} errorCode={first.error?.label} onretry={first.retry} hasMore={!!cursor} {loadingMore} onloadmore={more}
  rowActions={(r) => [{ id: 'lift', label: 'Lift block', icon: 'unlock' }, ...(r.item.kind === 'movie' ? [{ id: 'item', label: 'Open item', icon: 'arrow-right' as const }] : [])]}
  onaction={(a, r) => (a === 'lift' ? lift([r]) : (location.hash = `#/library/movies/${r.item.id}`))}>
  {#snippet bulkActions(keys)}
    <Button size="sm" icon="unlock" onclick={() => { void lift(all.filter((e) => keys.includes(String(e.id)))); selected = []; }}>Lift {keys.length}</Button>
  {/snippet}
  {#snippet empty()}
    <EmptyState title="Nothing is blocklisted" icon="check-circle" level={2}>
      Releases land here when a download fails, a payload is unsafe, or you remove a download with “Blocklist”.
    </EmptyState>
  {/snippet}
</ListPage>

<style>
  .mono { font-family: var(--font-mono); font-size: var(--text-sm); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; display: block; }
  .item { display: flex; align-items: center; gap: var(--space-2); min-width: 0; }
  .item a { color: var(--ink); text-decoration: none; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .item a:hover { text-decoration: underline; }
</style>
