<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import DurationField from './DurationField.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';

  const { Story } = defineMeta({ title: 'Molecules/DurationField', component: DurationField, tags: ['autodocs'], args: { label: 'Stall grace', value: 5400, help: 'How long a download may sit without progress before the stalled rule considers it.' } });
</script>

<Story name="Playground" />

<Story name="All states" asChild>
  <ThemePair>
    <Row label="Minutes"><DurationField label="Stall grace" value={5400} help="How long a download may sit without progress." /></Row>
    <Row label="Hours"><DurationField label="Retry after" value={21600} /></Row>
    <Row label="Required, empty"><DurationField label="RSS interval" value={null} required /></Row>
    <Row label="Error"><DurationField label="RSS interval" value={300} error="Indexers ask for 10 minutes or more between RSS reads." /></Row>
    <Row label="Managed by config"><DurationField label="Stall grace" value={3600} managed="rules.stalled.grace" /></Row>
    <Row label="Drift"><DurationField label="Stall grace" value={7200} drift="The config file says 1h." /></Row>
    <Row label="Disabled"><DurationField label="Stall grace" value={1800} disabled /></Row>
  </ThemePair>
</Story>
