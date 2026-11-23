<!--
  Activity › Queue: every download, live over SSE (download.progress patches rows in place;
  state changes refetch). Pause, resume, priority, and remove with the blocklist choice through
  ConfirmDialog, ending in a toast. Table on desktop, queue cards on phones.
-->
<script lang="ts">
  import { onMount } from 'svelte';
  import ListPage from '../../../lib/components/templates/ListPage/ListPage.svelte';
  import Button from '../../../lib/components/atoms/Button/Button.svelte';
  import Checkbox from '../../../lib/components/atoms/Checkbox/Checkbox.svelte';
  import MediaKindIcon from '../../../lib/components/atoms/MediaKindIcon/MediaKindIcon.svelte';
  import Progress from '../../../lib/components/atoms/Progress/Progress.svelte';
  import Quantity from '../../../lib/components/atoms/Quantity/Quantity.svelte';
  import Skeleton from '../../../lib/components/atoms/Skeleton/Skeleton.svelte';
  import StatusBadge from '../../../lib/components/atoms/StatusBadge/StatusBadge.svelte';
  import EmptyState from '../../../lib/components/molecules/EmptyState/EmptyState.svelte';
  import ErrorState from '../../../lib/components/molecules/ErrorState/ErrorState.svelte';
  import QueueCard, { type QueueAction, type QueueCardData } from '../../../lib/components/molecules/QueueCard/QueueCard.svelte';
  import ConfirmDialog, { type ConfirmChoice } from '../../../lib/components/organisms/ConfirmDialog/ConfirmDialog.svelte';
  import type { Column } from '../../../lib/components/organisms/DataTable/DataTable.svelte';
  import type { ActionItem } from '../../../lib/components/molecules/ActionMenu/ActionMenu.svelte';
  import { formatDuration, formatSpeed } from '../../../lib/format/quantity';
  import { app } from '../../context.svelte';
  import { Resource } from '../../resource.svelte';
  import { refetchOn } from '../../live.svelte';
  import { toApiError } from '../../api/client';
  import type { Download, JobState } from '../../api/types';
  import { isActiveDownload, toQueueCard } from '../../model';
  import { actionDone, pausedIds, queueAction, removeDownload } from '../../queue.svelte';

  const { api, live, movies, toasts } = app();

  const queue = new Resource((s) => api.list<Download>('queue', { limit: 200 }, s));
  const jobs = new Resource((s) => api.list<JobState>('jobs', undefined, s));
  onMount(() => {
    void queue.load();
    void jobs.load();
    movies.ensure();
    const offs = [
      live.on('download.progress', (ev) => {
        const d = ev.data as { downloadId?: number; progress?: number; speed?: number; eta?: number } | undefined;
        const row = queue.data?.find((r) => r.id === d?.downloadId);
        if (!row || !d) return;
        if (d.progress !== undefined) row.progress = d.progress;
        row.speed = d.speed ?? 0;
        row.eta = d.eta ?? 0;
      }),
      refetchOn(live, ['download.state_changed', 'download.completed', 'download.failed', 'download.removed', 'grab.sent', 'import.*', 'payload.rejected'], () => void queue.load(true), 400),
    ];
    return () => offs.forEach((off) => off());
  });

  let narrow = $state(typeof matchMedia === 'function' && matchMedia('(width < 48rem)').matches);
  $effect(() => {
    const mq = matchMedia('(width < 48rem)');
    const on = () => (narrow = mq.matches);
    mq.addEventListener('change', on);
    return () => mq.removeEventListener('change', on);
  });

  const downloads = $derived((queue.data ?? []).filter(isActiveDownload));
  const rows = $derived<QueueCardData[]>(downloads.map((d) => ({ ...toQueueCard(d, movies.title), paused: pausedIds.has(String(d.id)) })));
  const speed = $derived(downloads.reduce((n, d) => n + (d.speed || 0), 0));
  const description = $derived(downloads.length ? `${downloads.length} ${downloads.length === 1 ? 'download' : 'downloads'}${speed ? ` · ${formatSpeed(speed)}` : ''}` : undefined);
  const sweep = $derived(jobs.data?.find((j) => j.name === 'automation.rss'));

  let target = $state<QueueCardData | undefined>();
  let confirmOpen = $state(false);
  let deleteFiles = $state(false);
  let removing = $state(false);
  const removeChoices: ConfirmChoice[] = [
    { value: 'research', label: 'Blocklist and search again', description: 'Best when the release itself is bad.',
      consequences: ['Blocklists this release so it is never grabbed again', 'Searches for another release now'] },
    { value: 'blocklist', label: 'Blocklist only', description: 'Never grab this release again; wait for the next feed sync.' },
    { value: 'remove', label: 'Just remove', description: 'The item goes back to Wanted; nothing is blocklisted.' },
  ];

  async function act(action: QueueAction | string, id: string) {
    const d = rows.find((r) => r.id === id);
    if (!d) return;
    if (action === 'kill') { target = d; deleteFiles = false; confirmOpen = true; return; }
    if (!(action in actionDone)) {
      toasts.push({ message: 'Replacing a release comes in a later version', detail: 'Remove it with "Blocklist and search again" to get another release now.', tone: 'info' });
      return;
    }
    try {
      await queueAction(api, id, action);
      toasts.push({ message: `${actionDone[action]}: ${d.title}`, tone: 'ok' });
      void queue.load(true);
    } catch (e) {
      toasts.push({ message: 'The download client did not take that', detail: toApiError(e).message, tone: 'error' });
    }
  }

  async function remove(d: QueueCardData, choice: string | undefined) {
    removing = true;
    const blocklist = choice === 'research' || choice === 'blocklist';
    try {
      await removeDownload(api, d.id, { blocklist, research: choice === 'research', deleteFiles });
      confirmOpen = false;
      toasts.push({
        message: blocklist ? `Removed ${d.title} and blocklisted the release` : `Removed ${d.title} from the queue`,
        detail: choice === 'research' ? 'Searching for another release now.' : deleteFiles ? 'Downloaded files deleted.' : 'Downloaded files kept.',
        tone: 'ok', ...(blocklist ? { actionLabel: 'Open blocklist', onaction: () => (location.hash = '#/activity/blocklist') } : {}),
      });
      void queue.load(true);
    } catch (e) {
      toasts.push({ message: `${d.title} was not removed`, detail: toApiError(e).message, tone: 'error' });
    } finally {
      removing = false;
    }
  }

  async function bulk(action: 'pause' | 'resume', keys: string[]) {
    const r = await Promise.allSettled(keys.map((k) => queueAction(api, k, action)));
    const failed = r.filter((x) => x.status === 'rejected').length;
    toasts.push({ message: `${actionDone[action]} ${keys.length - failed} of ${keys.length}`, tone: failed ? 'warn' : 'ok' });
    void queue.load(true);
  }

  const columns: Column<QueueCardData>[] = [
    { id: 'title', label: 'Title', value: (r) => r.title, phone: 'title' },
    { id: 'state', label: 'State', width: '10.5rem', value: (r) => r.state, phone: 'trailing' },
    { id: 'progress', label: 'Progress', width: '7.5rem', value: (r) => (r.progress !== undefined ? `${Math.round(r.progress * 100)}%` : '—') },
    { id: 'size', label: 'Size', width: '5rem', align: 'end', value: (r) => r.size, phone: 'hidden' },
    { id: 'speed', label: 'Speed · left', width: '8.5rem', align: 'end', value: (r) => r.speed, phone: 'hidden' },
  ];
  const rowActions = (r: QueueCardData): ActionItem[] => [
    r.paused ? { id: 'resume', label: 'Resume', icon: 'play' } : { id: 'pause', label: 'Pause', icon: 'pause', hint: 'P' },
    { id: 'top', label: 'Move to top', icon: 'chevron-up', group: 'order', hint: 'Shift+Home' },
    { id: 'up', label: 'Move up', icon: 'arrow-up', group: 'order', hint: 'Shift+↑' },
    { id: 'down', label: 'Move down', icon: 'arrow-down', group: 'order', hint: 'Shift+↓' },
    { id: 'bottom', label: 'Move to bottom', icon: 'chevron-down', group: 'order', hint: 'Shift+End' },
    r.held ? { id: 'release', label: 'Release hold', icon: 'unlock', group: 'hold' } : { id: 'hold', label: 'Hold (automation leaves it alone)', icon: 'lock', group: 'hold' },
    { id: 'kill', label: 'Remove download…', icon: 'trash', danger: true, group: 'kill', hint: 'Del' },
  ];
  const tone = (r: QueueCardData) => (r.paused ? 'neutral' : r.state === 'stalled' ? 'warn' : r.state === 'import-blocked' || r.state === 'failed' ? 'error' : 'info');
</script>

{#snippet cell(r: QueueCardData, c: Column<QueueCardData>)}
  {#if c.id === 'title'}
    <span class="title"><MediaKindIcon kind={r.kind} /><span class="t">{#if r.href}<a href={r.href}>{r.title}</a>{:else}<span class="name">{r.title}</span>{/if}{#if r.subtitle}<span class="sub">{r.subtitle}</span>{/if}</span></span>
  {:else if c.id === 'state'}
    <span class="state"><StatusBadge family="download" value={r.state} detail={r.reason} size="sm" />{#if r.paused}<StatusBadge family="downloadModifier" value="paused" size="sm" />{/if}{#if r.held}<StatusBadge family="downloadModifier" value="held" size="sm" />{/if}</span>
  {:else if c.id === 'progress'}
    {#if r.progress !== undefined}<Progress label="Progress of {r.title}" value={r.progress} size="sm" showValue tone={tone(r)} />{:else}<Progress label="Progress of {r.title}" size="sm" />{/if}
  {:else if c.id === 'size'}{#if r.size}<Quantity kind="bytes" value={r.size} />{:else}—{/if}
  {:else if c.id === 'speed'}{#if r.speed && !r.paused}<Quantity kind="speed" value={r.speed} />{#if r.eta !== undefined && r.state === 'downloading'}<span class="eta">{` · ${formatDuration(r.eta)}`}</span>{/if}{:else}—{/if}
  {/if}
{/snippet}

{#snippet emptyQueue()}
  <EmptyState title="Nothing downloading" icon="arrow-down" level={2}>
    {sweep?.lastFinish ? 'The feed sync finds new releases on its schedule; grabs show up here as soon as they are sent.' : 'New grabs show up here as soon as they are sent.'}
    {#snippet actions()}<Button icon="rss" onclick={async () => { try { await api.post('jobs/automation.rss/run'); toasts.push({ message: 'Feed sync started', tone: 'ok' }); } catch (e) { toasts.push({ message: 'Feed sync did not start', detail: toApiError(e).message, tone: 'error' }); } }}>Sync feeds now</Button>{/snippet}
  </EmptyState>
{/snippet}

{#if narrow}
  <div class="phone">
    <header class="head"><h1>Queue</h1>{#if description}<p class="desc">{description}</p>{/if}</header>
    {#if queue.loading && !queue.data}
      <div aria-busy="true"><Skeleton shape="block" height="7rem" /></div>
    {:else if queue.error && !queue.data}
      <ErrorState title="Couldn't load the queue" code={queue.error.label} onretry={queue.retry} level={2}>{queue.error.message}</ErrorState>
    {:else if rows.length}
      <div class="cards">{#each rows as d (d.id)}<QueueCard download={d} onaction={act} />{/each}</div>
    {:else}
      {@render emptyQueue()}
    {/if}
  </div>
{:else}
  <ListPage title="Queue" {description} {rows} {columns} rowKey={(r) => r.id} {cell} {rowActions} onaction={(a, r) => act(a, r.id)} noun="download" selectable
    loading={queue.loading && !queue.data} error={queue.error?.message} errorCode={queue.error?.label} onretry={queue.retry} empty={emptyQueue}>
    {#snippet bulkActions(keys)}
      <Button size="sm" icon="pause" onclick={() => bulk('pause', keys)}>Pause {keys.length}</Button>
      <Button size="sm" icon="play" onclick={() => bulk('resume', keys)}>Resume {keys.length}</Button>
    {/snippet}
  </ListPage>
{/if}

{#if target}
  <ConfirmDialog bind:open={confirmOpen} title="Remove {target.title}?" loading={removing}
    consequences={[`Removes the download from ${downloads.find((d) => String(d.id) === target?.id)?.downloader ?? 'the download client'}`, deleteFiles ? 'Deletes the files downloaded so far' : 'Keeps the files downloaded so far']}
    choices={removeChoices} choice="research" confirmLabel="Remove download" undoable="This cannot be undone; a blocklisted release can be lifted from Activity › Blocklist."
    onconfirm={(choice) => remove(target!, choice)} oncancel={() => (confirmOpen = false)}>
    <Checkbox bind:checked={deleteFiles} label="Also delete downloaded files" />
  </ConfirmDialog>
{/if}

<style>
  .title { display: flex; align-items: center; gap: var(--space-2); min-width: 0; }
  .t { display: grid; min-width: 0; }
  .t a, .name, .sub { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .t a, .name { color: var(--ink); text-decoration: none; font-weight: var(--weight-medium); }
  .t a:hover { text-decoration: underline; }
  .sub { font-size: var(--text-xs); color: var(--ink-muted); }
  .eta { color: var(--ink-muted); }
  .state { display: flex; gap: var(--space-1); min-width: 0; overflow: hidden; }
  .phone { display: grid; gap: var(--space-4); }
  .head { display: grid; gap: var(--space-1); }
  h1 { font-size: var(--text-xl); line-height: var(--leading-tight); }
  .desc { color: var(--ink-muted); font-size: var(--text-md); }
  .cards { display: grid; gap: var(--space-3); }
</style>
