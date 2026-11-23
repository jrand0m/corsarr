<!-- Page composition (story only): Activity › Queue. Table on desktop, queue cards on phones;
     kill → confirm → toast with undo. -->
<script lang="ts">
  import Shell from './Shell.svelte';
  import ListPage from '../lib/components/templates/ListPage/ListPage.svelte';
  import Button from '../lib/components/atoms/Button/Button.svelte';
  import Checkbox from '../lib/components/atoms/Checkbox/Checkbox.svelte';
  import Countdown from '../lib/components/atoms/Countdown/Countdown.svelte';
  import MediaKindIcon from '../lib/components/atoms/MediaKindIcon/MediaKindIcon.svelte';
  import Progress from '../lib/components/atoms/Progress/Progress.svelte';
  import Quantity from '../lib/components/atoms/Quantity/Quantity.svelte';
  import StatusBadge from '../lib/components/atoms/StatusBadge/StatusBadge.svelte';
  import EmptyState from '../lib/components/molecules/EmptyState/EmptyState.svelte';
  import QueueCard, { type QueueAction, type QueueCardData } from '../lib/components/molecules/QueueCard/QueueCard.svelte';
  import ConfirmDialog, { type ConfirmChoice } from '../lib/components/organisms/ConfirmDialog/ConfirmDialog.svelte';
  import type { Column } from '../lib/components/organisms/DataTable/DataTable.svelte';
  import type { ActionItem } from '../lib/components/molecules/ActionMenu/ActionMenu.svelte';
  import { ToastQueue } from '../lib/toast/toasts.svelte';
  import { formatDuration } from '../lib/format/quantity';
  import { downloads as fixture, NOW } from '../lib/storybook/fixtures';

  interface Props {
    empty?: boolean;
    killOpen?: boolean;
    toastShown?: boolean;
  }
  let { empty = false, killOpen = false, toastShown = false }: Props = $props();

  const toasts = new ToastQueue();
  // svelte-ignore state_referenced_locally
  let rows = $state<QueueCardData[]>(empty ? [] : fixture.map((d) => ({ ...d })));
  // svelte-ignore state_referenced_locally
  let target = $state<QueueCardData | undefined>(killOpen ? fixture[1] : undefined);
  // svelte-ignore state_referenced_locally
  let confirmOpen = $state(killOpen);
  let deleteFiles = $state(false);

  let narrow = $state(typeof matchMedia === 'function' && matchMedia('(width < 48rem)').matches);
  $effect(() => {
    const mq = matchMedia('(width < 48rem)');
    const on = () => (narrow = mq.matches);
    mq.addEventListener('change', on);
    return () => mq.removeEventListener('change', on);
  });

  const killChoices: ConfirmChoice[] = [
    { value: 'research', label: 'Blocklist and search again', description: 'Best when the release itself is bad.',
      consequences: ['Blocklists this release so it is never grabbed again', 'Searches for another release now'] },
    { value: 'remove', label: 'Just remove', description: 'The item goes back to Wanted; nothing is blocklisted.' },
  ];

  function killed(d: QueueCardData, choice: string | undefined, keep = false) {
    const index = rows.findIndex((r) => r.id === d.id);
    rows = rows.filter((r) => r.id !== d.id);
    toasts.push({
      message: choice === 'research' ? `Killed ${d.title} and blocklisted the release` : `Removed ${d.title} from the queue`,
      detail: choice === 'research' ? 'Searching for another release now.' : deleteFiles ? 'Downloaded files deleted.' : 'Downloaded files kept.',
      onundo: () => { rows.splice(Math.max(0, index), 0, d); },
      duration: keep ? 0 : 10_000,
    });
  }
  // svelte-ignore state_referenced_locally
  if (toastShown) killed(fixture[1], 'research', true);

  function act(action: QueueAction | string, id: string) {
    const d = rows.find((r) => r.id === id);
    if (!d) return;
    if (action === 'kill') { target = d; deleteFiles = false; confirmOpen = true; }
    else if (action === 'pause' || action === 'resume') d.paused = action === 'pause';
  }

  const columns: Column<QueueCardData>[] = [
    { id: 'title', label: 'Title', value: (r) => r.title, phone: 'title' },
    { id: 'state', label: 'State', width: '11.5rem', value: (r) => r.state },
    { id: 'progress', label: 'Progress', width: '8rem', value: (r) => r.progress },
    { id: 'size', label: 'Size', width: '5rem', align: 'end', value: (r) => r.size },
    { id: 'speed', label: 'Speed · left', width: '8rem', align: 'end', value: (r) => r.speed },
    { id: 'rule', label: 'Rule', width: '9.5rem', value: (r) => r.rule?.action },
  ];
  const rowActions = (r: QueueCardData): ActionItem[] => [
    r.paused ? { id: 'resume', label: 'Resume', icon: 'play' } : { id: 'pause', label: 'Pause', icon: 'pause', hint: 'P' },
    { id: 'top', label: 'Move to top', icon: 'chevron-up', group: 'order', hint: 'Shift+Home' },
    { id: 'replace', label: 'Replace with another release…', icon: 'swap', group: 'replace', hint: 'R' },
    { id: 'replace-lower', label: 'Replace with lower resolution…', icon: 'lower', group: 'replace', hint: 'L' },
    { id: 'kill', label: 'Kill download…', icon: 'trash', danger: true, group: 'kill', hint: 'Del' },
  ];
  const tone = (r: QueueCardData) => (r.paused ? 'neutral' : r.state === 'stalled' ? 'warn' : r.state === 'import-blocked' ? 'error' : 'info');
</script>

{#snippet cell(r: QueueCardData, c: Column<QueueCardData>)}
  {#if c.id === 'title'}
    <span class="title"><MediaKindIcon kind={r.kind} /><span class="t"><a href={r.href}>{r.title}</a>{#if r.subtitle}<span class="sub">{r.subtitle}</span>{/if}</span></span>
  {:else if c.id === 'state'}
    <span class="state"><StatusBadge family="download" value={r.state} detail={r.reason} size="sm" />{#if r.paused}<StatusBadge family="downloadModifier" value="paused" size="sm" />{/if}</span>
  {:else if c.id === 'progress'}
    {#if r.progress !== undefined}<Progress label="Progress of {r.title}" value={r.progress} size="sm" showValue tone={tone(r)} />{:else}—{/if}
  {:else if c.id === 'size'}{#if r.size}<Quantity kind="bytes" value={r.size} />{:else}—{/if}
  {:else if c.id === 'speed'}{#if r.speed && !r.paused}<Quantity kind="speed" value={r.speed} />{#if r.eta !== undefined && r.state === 'downloading'}<span class="eta">{` · ${formatDuration(r.eta)}`}</span>{/if}{:else}—{/if}
  {:else if c.id === 'rule'}
    {#if r.rule}<Countdown to={r.rule.at} action={r.rule.dryRun ? `Dry run: ${r.rule.action}` : r.rule.action} tone="warn" now={NOW} />{:else}—{/if}
  {/if}
{/snippet}

{#snippet emptyQueue()}
  <EmptyState title="Nothing downloading" icon="arrow-down" level={2}>
    The last feed sync ran 6 minutes ago and found nothing new to grab. New grabs show up here as soon as they are sent.
    {#snippet actions()}<Button icon="rss">Sync feeds now</Button>{/snippet}
  </EmptyState>
{/snippet}

<Shell path="/activity/queue" {toasts}>
  {#if narrow}
    <div class="phone">
      <header class="head"><h1>Queue</h1><p class="desc">{rows.length} downloads · 12.4 MB/s</p></header>
      {#if rows.length}
        <div class="cards">{#each rows as d (d.id)}<QueueCard download={d} onaction={act} now={NOW} />{/each}</div>
      {:else}
        {@render emptyQueue()}
      {/if}
    </div>
  {:else}
    <ListPage title="Queue" description="{rows.length} downloads · 12.4 MB/s" {rows} {columns} rowKey={(r) => r.id} {cell} {rowActions}
      onaction={(a, r) => act(a, r.id)} noun="download" selectable empty={emptyQueue}>
      {#snippet actions()}<Button icon="pause">Pause all</Button>{/snippet}
      {#snippet bulkActions(keys)}
        <Button size="sm" icon="pause">Pause {keys.length}</Button>
        <Button size="sm" icon="trash">Kill {keys.length}…</Button>
      {/snippet}
    </ListPage>
  {/if}

  {#if target}
    <ConfirmDialog bind:open={confirmOpen} title="Kill {target.title}{target.subtitle ? ` · ${target.subtitle}` : ''}?"
      consequences={['Removes the download from Harbour box', deleteFiles ? 'Deletes the files downloaded so far' : 'Keeps the files downloaded so far']}
      choices={killChoices} confirmLabel="Kill download" undoable="You can undo this for 10 seconds, or from History for 30 days."
      onconfirm={(choice) => { const d = target!; confirmOpen = false; killed(d, choice); }} oncancel={() => (confirmOpen = false)}>
      <Checkbox bind:checked={deleteFiles} label="Also delete downloaded files" />
    </ConfirmDialog>
  {/if}
</Shell>

<style>
  .title { display: flex; align-items: center; gap: var(--space-2); min-width: 0; }
  .t { display: grid; min-width: 0; }
  .t a, .sub { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .t a { color: var(--ink); text-decoration: none; font-weight: var(--weight-medium); }
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
