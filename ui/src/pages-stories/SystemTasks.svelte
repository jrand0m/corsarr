<!-- Page composition (story only): System › Tasks, background jobs with run now. -->
<script lang="ts">
  import Shell from './Shell.svelte';
  import ListPage from '../lib/components/templates/ListPage/ListPage.svelte';
  import Progress from '../lib/components/atoms/Progress/Progress.svelte';
  import StatusBadge from '../lib/components/atoms/StatusBadge/StatusBadge.svelte';
  import Switch from '../lib/components/atoms/Switch/Switch.svelte';
  import Timestamp from '../lib/components/atoms/Timestamp/Timestamp.svelte';
  import type { Column } from '../lib/components/organisms/DataTable/DataTable.svelte';
  import { formatDuration } from '../lib/format/quantity';
  import { taskRows, nowDate, type TaskRow } from '../lib/storybook/fixtures';

  let rows = $state(taskRows.map((t) => ({ ...t })));

  const columns: Column<TaskRow>[] = [
    { id: 'enabled', label: 'Enabled', width: '4.5rem', hideLabel: true, phone: 'hidden' },
    { id: 'name', label: 'Task', value: (r) => r.name, phone: 'title' },
    { id: 'schedule', label: 'Schedule', width: '12rem', value: (r) => r.schedule, phone: 'meta' },
    { id: 'last', label: 'Last run', width: '7rem', value: (r) => r.lastRun, phone: 'hidden' },
    { id: 'duration', label: 'Took', width: '5rem', align: 'end', value: (r) => r.duration, phone: 'hidden' },
    { id: 'result', label: 'Result', width: '9rem', value: (r) => r.result, phone: 'trailing' },
    { id: 'next', label: 'Next run', width: '7rem', value: (r) => r.nextRun, phone: 'hidden' },
  ];
</script>

{#snippet cell(r: TaskRow, c: Column<TaskRow>)}
  {#if c.id === 'enabled'}<Switch label="Run {r.name} on schedule" hideLabel bind:checked={r.enabled} />
  {:else if c.id === 'name'}<span class="name"><span>{r.name}</span>{#if r.note}<span class="note">{r.note}</span>{/if}</span>
  {:else if c.id === 'last'}{#if r.lastRun}<Timestamp value={r.lastRun} now={nowDate} />{:else}Never{/if}
  {:else if c.id === 'duration'}{r.duration !== undefined ? formatDuration(r.duration) : '—'}
  {:else if c.id === 'result'}
    {#if r.result === 'running' && r.progress !== undefined}<Progress label="{r.name} progress" value={r.progress} size="sm" showValue />
    {:else if r.result}<StatusBadge family="job" value={r.result} size="sm" appearance="plain" />{:else}—{/if}
  {:else if c.id === 'next'}{#if r.enabled && r.nextRun}<Timestamp value={r.nextRun} now={nowDate} />{:else}Off{/if}
  {:else}{c.value?.(r)}{/if}
{/snippet}

<Shell path="/system/tasks">
  <ListPage title="Tasks" description="Background work corsarr runs on a schedule" {rows} {columns} rowKey={(r) => r.id} {cell} noun="task"
    rowActions={(r) => r.result === 'running'
      ? [{ id: 'cancel', label: 'Cancel', icon: 'x' }]
      : [{ id: 'run', label: 'Run now', icon: 'play' }, { id: 'interval', label: 'Change interval…', icon: 'clock' }]} />
</Shell>

<style>
  .name { display: grid; min-width: 0; }
  .name span { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .note { font-size: var(--text-xs); color: var(--ink-muted); }
</style>
