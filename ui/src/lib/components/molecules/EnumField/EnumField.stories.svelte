<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import EnumField from './EnumField.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';
  const options = [{ value: 'low', label: 'Low' }, { value: 'normal', label: 'Normal' }, { value: 'high', label: 'High' }];
  const modes = [
    { value: 'off', label: 'Off', description: 'The rule never runs.' },
    { value: 'dry-run', label: 'Dry run', description: 'Logs what it would do; changes nothing.' },
    { value: 'live', label: 'Live', description: 'Acts on matching downloads.' },
  ];
  const { Story } = defineMeta({ title: 'Molecules/EnumField', component: EnumField, tags: ['autodocs'], args: { label: 'Priority', value: 'normal', options } });
</script>

<Story name="Playground" />

<Story name="All states" asChild>
  <ThemePair>
    <Row label="Select"><EnumField label="Priority" value="normal" {options} help="Priority qBittorrent gives new downloads." /></Row>
    <Row label="Radios"><EnumField label="Rule mode" value="dry-run" options={modes} appearance="radio" /></Row>
    <Row label="Required, placeholder"><EnumField label="Priority" value="" {options} placeholder="Choose a priority" required /></Row>
    <Row label="Error"><EnumField label="Priority" value="high" {options} error="High priority needs qBittorrent 4.5 or later." /></Row>
    <Row label="Managed by config"><EnumField label="Rule mode" value="live" options={modes} appearance="radio" managed="rules.stalled.mode" /></Row>
    <Row label="Drift"><EnumField label="Priority" value="low" {options} drift="The config file says normal." /></Row>
    <Row label="Disabled"><EnumField label="Priority" value="normal" {options} disabled /></Row>
  </ThemePair>
</Story>
