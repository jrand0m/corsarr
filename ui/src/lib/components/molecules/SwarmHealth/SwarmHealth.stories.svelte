<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import SwarmHealth from './SwarmHealth.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';

  const { Story } = defineMeta({ title: 'Molecules/SwarmHealth', component: SwarmHealth, tags: ['autodocs'], args: { seeders: 120, leechers: 10, reported: true, health: 'good' } });
</script>

<Story name="Playground" />

<Story name="All states" asChild>
  <ThemePair>
    <Row label="Well seeded"><SwarmHealth seeders={120} leechers={10} reported health="good" /></Row>
    <Row label="Some seeders, busy"><SwarmHealth seeders={8} leechers={40} reported health="some" /></Row>
    <Row label="Few seeders, many leechers"><SwarmHealth seeders={2} leechers={90} reported health="few" /></Row>
    <Row label="No seeders"><SwarmHealth seeders={0} leechers={15} reported health="none" /></Row>
    <Row label="Empty swarm"><SwarmHealth seeders={0} leechers={0} reported health="none" /></Row>
    <Row label="Leechers not reported"><SwarmHealth seeders={30} reported health="good" /></Row>
    <Row label="Swarm not reported"><SwarmHealth seeders={0} reported={false} health="unknown" /></Row>
  </ThemePair>
</Story>
