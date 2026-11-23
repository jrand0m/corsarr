<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import Checkbox from './Checkbox.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';

  const { Story } = defineMeta({ title: 'Atoms/Checkbox', component: Checkbox, tags: ['autodocs'], args: { label: 'Season folders', checked: true } });
</script>

<Story name="Playground" />

<Story name="All states" asChild>
  <ThemePair>
    <Row label="Off, on, mixed">
      <Checkbox label="Monitor specials" />
      <Checkbox label="Season folders" checked />
      <Checkbox label="Select all loaded" indeterminate />
      <Checkbox aria-label="Select row" />
    </Row>
    <Row label="With description"><Checkbox label="Search on add" description="Runs an automatic search as soon as the item is added." checked /></Row>
    <Row label="Disabled"><Checkbox label="Off" disabled /><Checkbox label="On" checked disabled /></Row>
  </ThemePair>
</Story>
