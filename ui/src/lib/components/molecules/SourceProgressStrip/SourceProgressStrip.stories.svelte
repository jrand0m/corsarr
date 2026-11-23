<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import SourceProgressStrip from './SourceProgressStrip.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';
  import type { SourceProgress } from './SourceProgressStrip.svelte';
  const done: SourceProgress[] = [
    { id: 'a', name: 'NZBgeek', state: 'ok', count: 38, ms: 840 },
    { id: 'b', name: 'DrunkenSlug', state: 'ok', count: 21, ms: 1320 },
    { id: 'c', name: 'Harbour Torrents', state: 'ok', count: 0, ms: 410 },
    { id: 'd', name: 'Pier Public', state: 'skipped', reason: 'no TV categories' },
    { id: 'e', name: 'OldDock', state: 'failed', reason: 'timed out after 15 s' },
  ];
  const live: SourceProgress[] = done.map((s, i) => (i < 2 ? s : { id: s.id, name: s.name, state: i === 2 ? 'running' : 'pending' }));
  const { Story } = defineMeta({ title: 'Molecules/SourceProgressStrip', component: SourceProgressStrip, tags: ['autodocs'], args: { sources: done, label: 'Indexers' } });
</script>

<Story name="Playground" />

<Story name="All states" asChild>
  <ThemePair>
    <Row label="Searching"><SourceProgressStrip sources={live} label="Indexers" /></Row>
    <Row label="All answered, one skipped, one failed"><SourceProgressStrip sources={done} label="Indexers" /></Row>
    <Row label="Compact (strip only)"><SourceProgressStrip sources={live} label="Indexers" compact /></Row>
    <Row label="Migration sources"><SourceProgressStrip label="Sources" unit="objects" sources={[{ id: 's', name: 'Sonarr export', state: 'ok', count: 412, ms: 2300 }, { id: 'r', name: 'Radarr export', state: 'running' }]} /></Row>
  </ThemePair>
</Story>
