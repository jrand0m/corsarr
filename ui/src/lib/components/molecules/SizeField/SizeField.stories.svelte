<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import SizeField from './SizeField.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';

  const { Story } = defineMeta({ title: 'Molecules/SizeField', component: SizeField, tags: ['autodocs'], args: { label: 'Maximum size', value: 4_700_000_000, help: 'Releases above this are rejected as a preference.' } });
</script>

<Story name="Playground" />

<Story name="All states" asChild>
  <ThemePair>
    <Row label="Gigabytes"><SizeField label="Maximum size" value={4_700_000_000} help="Releases above this are rejected as a preference." /></Row>
    <Row label="Megabytes"><SizeField label="Minimum size" value={350_000_000} /></Row>
    <Row label="Required, empty"><SizeField label="Free space to keep" value={null} required /></Row>
    <Row label="Error"><SizeField label="Minimum size" value={9_000_000_000} error="Minimum size is above the maximum (4.7 GB)." /></Row>
    <Row label="Managed by config"><SizeField label="Free space to keep" value={50_000_000_000} managed="library.min_free" /></Row>
    <Row label="Drift"><SizeField label="Maximum size" value={8_000_000_000} drift="The config file says 6 GB." /></Row>
    <Row label="Disabled"><SizeField label="Maximum size" value={null} disabled /></Row>
  </ThemePair>
</Story>
