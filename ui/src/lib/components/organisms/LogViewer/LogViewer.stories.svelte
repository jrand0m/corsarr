<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import LogViewer from './LogViewer.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';
  import LogViewerDemo from '../../../storybook/LogViewerDemo.svelte';
  import { logLines } from '../../../storybook/log-lines';

  const many = logLines(50_000);
  const some = logLines(60);

  const { Story } = defineMeta({ title: 'Organisms/LogViewer', component: LogViewer, tags: ['autodocs'] });
</script>

<Story name="Playground" asChild>
  <LogViewerDemo />
</Story>

<Story name="All states" asChild>
  <ThemePair>
    <Row label="Live tail, following"><div style="width: 100%"><LogViewerDemo /></div></Row>
    <Row label="50,000 lines, filtered to warnings and errors"><div style="width: 100%"><LogViewer lines={many} minLevel="warn" connection="live" height="18rem" /></div></Row>
    <Row label="Text filter with highlights, paused"><div style="width: 100%"><LogViewer lines={some} text="indexer" following={false} connection="reconnecting" height="14rem" /></div></Row>
    <Row label="Nothing matches"><div style="width: 100%"><LogViewer lines={some} text="no such words" height="10rem" /></div></Row>
    <Row label="No lines yet, disconnected"><div style="width: 100%"><LogViewer lines={[]} connection="offline" height="10rem" /></div></Row>
  </ThemePair>
</Story>
