<!-- Page composition (story only): System › Logs. The log viewer is heavy and loads on
     demand; a skeleton holds its place meanwhile. -->
<script lang="ts">
  import Shell from './Shell.svelte';
  import Skeleton from '../lib/components/atoms/Skeleton/Skeleton.svelte';
  import ErrorState from '../lib/components/molecules/ErrorState/ErrorState.svelte';
  import { loadLogViewer } from '../lib/components/organisms';
  import type { LogLine } from '../lib/format/log';
  import { logLines } from '../lib/storybook/log-lines';

  let { loading = false, live = false, connection = 'live' }: { loading?: boolean; live?: boolean; connection?: 'live' | 'reconnecting' | 'offline' } = $props();

  let lines = $state<LogLine[]>(logLines(400));
  $effect(() => {
    if (!live) return;
    const id = setInterval(() => lines.push(...logLines(1, lines.length)), 700);
    return () => clearInterval(id);
  });
  // svelte-ignore state_referenced_locally
  const viewer = loading ? new Promise<never>(() => {}) : loadLogViewer();
</script>

<Shell path="/system/logs">
  <div class="page">
    <header class="head">
      <h1>Logs</h1>
      <p class="desc">The most recent 10,000 lines, live while this page is open.</p>
    </header>
    {#await viewer}
      <div class="skel" aria-busy="true" aria-label="Loading the log viewer">
        <Skeleton height="2.25rem" />
        <Skeleton shape="block" height="max(20rem, calc(100dvh - 18rem))" />
      </div>
    {:then LogViewer}
      <LogViewer {lines} {connection} height="max(20rem, calc(100dvh - 18rem))" />
    {:catch}
      <ErrorState title="Couldn't load the log viewer" onretry={() => location.reload()} level={2}>Reload the page to try again.</ErrorState>
    {/await}
  </div>
</Shell>

<style>
  .page { display: grid; gap: var(--space-4); min-width: 0; }
  .head { display: grid; gap: var(--space-1); }
  h1 { font-size: var(--text-xl); line-height: var(--leading-tight); }
  .desc { color: var(--ink-muted); font-size: var(--text-md); }
  .skel { display: grid; gap: var(--space-2); }
</style>
