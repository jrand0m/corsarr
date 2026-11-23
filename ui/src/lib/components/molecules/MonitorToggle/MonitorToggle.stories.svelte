<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import MonitorToggle from './MonitorToggle.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';

  const { Story } = defineMeta({ title: 'Molecules/MonitorToggle', component: MonitorToggle, tags: ['autodocs'], args: { item: 'S02E03', state: 'monitored', appearance: 'icon' } });
</script>

<Story name="Playground" />

<Story name="All states" asChild>
  <ThemePair>
    <Row label="Icon (rows)">
      <MonitorToggle item="S02E03" state="monitored" />
      <MonitorToggle item="S02E04" state="unmonitored" />
      <MonitorToggle item="S02E05" state="pinned" reason="keeping the extended cut" />
      <MonitorToggle item="S02E06" state="monitored" size="sm" />
      <MonitorToggle item="S02E07" state="monitored" disabled />
    </Row>
    <Row label="Labelled (headers, cards)">
      <MonitorToggle item="The Long Haul" state="monitored" appearance="labelled" />
      <MonitorToggle item="Dockside" state="unmonitored" appearance="labelled" />
      <MonitorToggle item="Harbour Lights" state="pinned" reason="the 2019 remaster stays" appearance="labelled" onunpin={() => {}} />
    </Row>
  </ThemePair>
</Story>
