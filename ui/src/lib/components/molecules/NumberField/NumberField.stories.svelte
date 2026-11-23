<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import NumberField from './NumberField.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';

  const { Story } = defineMeta({ title: 'Molecules/NumberField', component: NumberField, tags: ['autodocs'], args: { label: 'Minimum seeders', value: 3, min: 0, max: 500 } });
</script>

<Story name="Playground" />

<Story name="All states" asChild>
  <ThemePair>
    <Row label="Default"><NumberField label="Minimum seeders" value={3} min={0} help="Releases with fewer seeders are rejected as a preference." /></Row>
    <Row label="With unit"><NumberField label="Seed ratio" value={1.5} step={0.1} unit="×" /></Row>
    <Row label="Required, empty"><NumberField label="Port" required value={null} /></Row>
    <Row label="Error"><NumberField label="Port" value={80800} error="Port must be between 1 and 65535." /></Row>
    <Row label="Managed by config"><NumberField label="Port" value={8080} managed="downloaders.qbit.port" /></Row>
    <Row label="Drift"><NumberField label="Daily grab limit" value={40} drift="The config file says 25." /></Row>
    <Row label="Disabled"><NumberField label="Daily grab limit" value={25} disabled /></Row>
  </ThemePair>
</Story>
