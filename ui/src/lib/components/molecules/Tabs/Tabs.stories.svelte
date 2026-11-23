<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import Tabs from './Tabs.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';
  const tabs = [
    { id: 'settings', label: 'Settings' }, { id: 'stats', label: 'Stats' }, { id: 'queries', label: 'Queries', count: 1284 },
    { id: 'caps', label: 'Capabilities' }, { id: 'lists', label: 'Lists', disabled: true },
  ];
  const { Story } = defineMeta({ title: 'Molecules/Tabs', component: Tabs, tags: ['autodocs'], args: { label: 'Indexer', tabs, value: 'settings' } });
</script>

<Story name="Playground">
  {#snippet template(args)}
    <Tabs {...args}>{#snippet children(id)}<p>Panel for <strong>{id}</strong>: NZBgeek.</p>{/snippet}</Tabs>
  {/snippet}
</Story>

<Story name="All states" asChild>
  <ThemePair>
    <Row label="With counts and a disabled tab">
      <Tabs label="Indexer" {tabs} value="queries">{#snippet children(id)}<p>Showing {id} for NZBgeek.</p>{/snippet}</Tabs>
    </Row>
    <Row label="Tab list only (panel rendered by the page)"><Tabs label="History streams" tabs={[{ id: 'events', label: 'Events' }, { id: 'deliveries', label: 'Deliveries' }, { id: 'queries', label: 'Queries' }]} /></Row>
  </ThemePair>
</Story>
