<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import StatTile from './StatTile.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';

  const { Story } = defineMeta({ title: 'Molecules/StatTile', component: StatTile, tags: ['autodocs'], args: { label: 'Wanted', value: 42, href: '#wanted', linkLabel: 'Open wanted' } });
</script>

<Story name="Playground" />

<Story name="All states" asChild>
  <ThemePair>
    <Row label="With link">
      <StatTile label="Downloading" value={3} href="#queue" linkLabel="Open queue" />
      <StatTile label="Total speed" value={12_400_000} kind="speed" />
    </Row>
    <Row label="Trends (good, bad, flat)">
      <StatTile label="Wanted" value={42} trend={{ delta: -6, better: 'down', period: 'vs last week' }} />
      <StatTile label="Failed imports" value={4} tone="error" trend={{ delta: 3, better: 'down', period: 'vs yesterday' }} />
      <StatTile label="Free space" value={412e9} kind="bytes" trend={{ delta: 0, better: 'up', period: 'vs last week' }} />
    </Row>
    <Row label="Count of limit, unknown, loading">
      <StatTile label="Grabs today" value={12} limit={25} />
      <StatTile label="Indexer answers" value={null} />
      <StatTile label="Wanted" value={0} loading />
    </Row>
  </ThemePair>
</Story>
