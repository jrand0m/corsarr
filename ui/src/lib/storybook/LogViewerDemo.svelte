<!-- Story helper: a LogViewer fed like the page feeds it from SSE (a line every 700 ms). -->
<script lang="ts">
  import LogViewer from '../components/organisms/LogViewer/LogViewer.svelte';
  import { logLines } from './log-lines';

  let { initial = 400, live = true }: { initial?: number; live?: boolean } = $props();
  // svelte-ignore state_referenced_locally
  let lines = $state(logLines(initial));
  $effect(() => {
    if (!live) return;
    const id = setInterval(() => { lines.push(...logLines(1, lines.length)); }, 700);
    return () => clearInterval(id);
  });
</script>

<LogViewer {lines} connection={live ? 'live' : 'offline'} height="24rem" />
