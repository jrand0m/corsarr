<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import Chip from './Chip.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';

  const { Story } = defineMeta({ title: 'Atoms/Chip', component: Chip, tags: ['autodocs'], args: { label: 'anime', variant: 'tag' } });
</script>

<script lang="ts">
  let on = $state(true);
</script>

<Story name="Playground" />

<Story name="All variants" asChild>
  <ThemePair>
    <Row label="Tags"><Chip label="anime" /><Chip label="kids" /><Chip label="4k-only" /></Row>
    <Row label="Filter tokens (removable)"><Chip variant="filter" facet="Profile" label="HD 1080p" onremove={() => {}} /><Chip variant="filter" facet="Tag" label="kids" onremove={() => {}} /></Row>
    <Row label="Format scores"><Chip variant="score" label="HDR10" score={150} /><Chip variant="score" label="Remux" score={-50} /><Chip variant="score" label="x265" score={0} /></Row>
    <Row label="Toggle"><Chip label="Missing" selected={on} onclick={() => (on = !on)} /><Chip label="Cutoff unmet" selected={false} onclick={() => {}} /></Row>
  </ThemePair>
</Story>
