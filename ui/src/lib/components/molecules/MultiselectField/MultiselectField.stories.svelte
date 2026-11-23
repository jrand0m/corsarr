<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import MultiselectField from './MultiselectField.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';
  const options = [
    { value: '5030', label: 'TV SD' }, { value: '5040', label: 'TV HD' }, { value: '5045', label: 'TV UHD' },
    { value: '5070', label: 'TV Anime' }, { value: '5080', label: 'TV Documentary' }, { value: '5020', label: 'TV Foreign' },
  ];
  const { Story } = defineMeta({ title: 'Molecules/MultiselectField', component: MultiselectField, tags: ['autodocs'], args: { label: 'Categories searched', value: ['5030', '5040'], options, columns: 2 } });
</script>

<Story name="Playground" />

<Story name="All states" asChild>
  <ThemePair>
    <Row label="Two columns"><MultiselectField label="Categories searched" value={['5030', '5040']} {options} columns={2} help="Newznab categories corsarr asks this indexer for." /></Row>
    <Row label="Required, none selected"><MultiselectField label="Categories searched" value={[]} options={options.slice(0, 3)} required /></Row>
    <Row label="Error"><MultiselectField label="Categories searched" value={['5080']} options={options.slice(0, 4)} error="Pick at least one TV category this indexer supports." /></Row>
    <Row label="Managed by config"><MultiselectField label="Categories searched" value={['5040', '5045']} options={options.slice(0, 3)} managed="indexers.nzbgeek.categories" /></Row>
    <Row label="Drift"><MultiselectField label="Categories searched" value={['5040']} options={options.slice(0, 3)} drift="The config file also lists TV UHD." /></Row>
    <Row label="Disabled"><MultiselectField label="Categories searched" value={[]} options={options.slice(0, 3)} disabled /></Row>
  </ThemePair>
</Story>
