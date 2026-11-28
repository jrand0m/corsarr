<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import DesirabilityGauge from './DesirabilityGauge.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';

  const on = { score: 100, position: 'on_target' as const, qualitySteps: 0, cutoff: 'Bluray-1080p', sizeDelta: 0.04, preferredSize: 9.5e9 };
  const above = { score: 63, position: 'above' as const, qualitySteps: 1, cutoff: 'Bluray-1080p', sizeDelta: 0.34, preferredSize: 9.5e9 };
  const below = { score: 55, position: 'below' as const, qualitySteps: -1, cutoff: 'Bluray-1080p', sizeDelta: -0.5, preferredSize: 9.5e9, formatShortfall: 40, formatTarget: 100 };
  const { Story } = defineMeta({ title: 'Molecules/DesirabilityGauge', component: DesirabilityGauge, tags: ['autodocs'], args: { desirability: above, parts: true } });
</script>

<Story name="Playground" />

<Story name="All states" asChild>
  <ThemePair>
    <Row label="On target"><DesirabilityGauge desirability={on} parts /></Row>
    <Row label="Above target"><DesirabilityGauge desirability={above} parts /></Row>
    <Row label="Below target"><DesirabilityGauge desirability={below} parts /></Row>
    <Row label="Size unknown"><DesirabilityGauge desirability={{ score: 75, position: 'below', qualitySteps: -1, cutoff: 'WEBDL-1080p' }} parts /></Row>
    <Row label="Outside the profile"><DesirabilityGauge /></Row>
    <Row label="Headline only"><DesirabilityGauge desirability={below} /></Row>
  </ThemePair>
</Story>
