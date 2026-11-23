<!-- System › Tasks: background jobs with schedule, last result and run now; live via job.updated. -->
<script lang="ts">
  import { onMount } from 'svelte';
  import ListPage from '../../../lib/components/templates/ListPage/ListPage.svelte';
  import StatusBadge from '../../../lib/components/atoms/StatusBadge/StatusBadge.svelte';
  import Timestamp from '../../../lib/components/atoms/Timestamp/Timestamp.svelte';
  import type { Column } from '../../../lib/components/organisms/DataTable/DataTable.svelte';
  import { formatDuration } from '../../../lib/format/quantity';
  import { app } from '../../context.svelte';
  import { Resource } from '../../resource.svelte';
  import { refetchOn } from '../../live.svelte';
  import { toApiError } from '../../api/client';
  import type { JobState } from '../../api/types';

  const { api, live, toasts } = app();
  const jobs = new Resource((s) => api.list<JobState>('jobs', undefined, s));
  onMount(() => {
    void jobs.load();
    return refetchOn(live, ['job.updated', 'command.updated'], () => void jobs.load(true), 300);
  });

  const names: Record<string, { name: string; note: string }> = {
    'automation.rss': { name: 'Feed sync', note: 'Reads new releases from every indexer feed and grabs what is wanted.' },
    'history.prune': { name: 'Prune history', note: 'Drops history older than the retention window.' },
    'instances.test': { name: 'Test integrations', note: 'Checks every indexer, download client and service.' },
    'queue.poll': { name: 'Poll download clients', note: 'Runs while downloads are active; live updates come from it.' },
    'queue.sweep': { name: 'Queue sweep', note: 'Reconciles the queue with each download client.' },
    'rules.recyclepurge': { name: 'Empty recycle area', note: 'Deletes recycled files past their expiry.' },
    'sessions.prune': { name: 'Prune sign-ins', note: 'Removes expired sign-in sessions.' },
  };
  const label = (j: JobState) => names[j.name]?.name ?? j.name;
  const result = (j: JobState) => (j.running ? 'running' : j.lastResult === 'ok' ? 'done' : j.lastResult === 'failed' ? 'failed' : '');
  const took = (j: JobState) => (j.lastStart && j.lastFinish ? Math.max(0, (Date.parse(j.lastFinish) - Date.parse(j.lastStart)) / 1000) : undefined);
  const every = (s: number) => (s ? `Every ${formatDuration(s)}` : 'On demand');

  async function run(j: JobState) {
    try {
      await api.post(`jobs/${encodeURIComponent(j.name)}/run`);
      toasts.push({ message: `${label(j)} queued to run now`, tone: 'ok' });
      void jobs.load(true);
    } catch (e) {
      toasts.push({ message: `${label(j)} did not start`, detail: toApiError(e).message, tone: 'error' });
    }
  }

  const columns: Column<JobState>[] = [
    { id: 'name', label: 'Task', value: label, phone: 'title' },
    { id: 'schedule', label: 'Schedule', width: '8rem', value: (r) => (r.enabled ? every(r.intervalSeconds) : 'Off'), phone: 'meta' },
    { id: 'last', label: 'Last run', width: '6.5rem', value: (r) => r.lastStart, phone: 'hidden' },
    { id: 'duration', label: 'Took', width: '4rem', align: 'end', value: (r) => took(r), phone: 'hidden' },
    { id: 'result', label: 'Result', width: '7rem', value: result, phone: 'trailing' },
    { id: 'next', label: 'Next run', width: '7rem', value: (r) => r.nextRun, phone: 'hidden' },
  ];
</script>

{#snippet cell(r: JobState, c: Column<JobState>)}
  {#if c.id === 'name'}<span class="name"><span>{label(r)}</span><span class="note">{r.lastError ? r.lastError : (names[r.name]?.note ?? '')}</span></span>
  {:else if c.id === 'last'}{#if r.lastStart}<Timestamp value={r.lastStart} />{:else}Never{/if}
  {:else if c.id === 'duration'}{took(r) !== undefined ? formatDuration(Math.round(took(r)!)) : '—'}
  {:else if c.id === 'result'}{#if result(r)}<StatusBadge family="job" value={result(r)} size="sm" appearance="plain" />{:else}—{/if}
  {:else if c.id === 'next'}{#if r.enabled && r.nextRun}<Timestamp value={r.nextRun} />{:else}{r.enabled ? 'Pending' : 'Off'}{/if}
  {:else}{c.value?.(r)}{/if}
{/snippet}

<ListPage title="Tasks" description="Background work corsarr runs on a schedule" rows={jobs.data ?? []} {columns} rowKey={(r) => r.name} {cell} noun="task"
  loading={jobs.loading && !jobs.data} error={jobs.error?.message} errorCode={jobs.error?.label} onretry={jobs.retry}
  rowActions={(r) => [{ id: 'run', label: 'Run now', icon: 'play', disabled: r.running }]} onaction={(_, r) => run(r)} />

<style>
  .name { display: grid; min-width: 0; }
  .name span { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .note { font-size: var(--text-xs); color: var(--ink-muted); }
</style>
