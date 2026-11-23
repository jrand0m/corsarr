<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import MapField from './MapField.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';

  const { Story } = defineMeta({ title: 'Molecules/MapField', component: MapField, tags: ['autodocs'], args: { label: 'Path mappings', keyLabel: 'Downloader path', valueLabel: 'corsarr path', mono: true, value: [{ key: '/downloads', value: '/data/downloads' }] } });
</script>

<Story name="Playground" />

<Story name="All states" asChild>
  <ThemePair>
    <Row label="Rows"><MapField label="Path mappings" keyLabel="Downloader path" valueLabel="corsarr path" mono value={[{ key: '/downloads', value: '/data/downloads' }, { key: '/incomplete', value: '/data/incomplete' }]} help="Where corsarr finds what the downloader reports." /></Row>
    <Row label="Empty"><MapField label="Extra headers" keyLabel="Header" valueLabel="Value" value={[]} /></Row>
    <Row label="Duplicate key"><MapField label="Category map" keyLabel="Indexer category" valueLabel="Kind" value={[{ key: '5040', value: 'tv' }, { key: '5040', value: 'movie' }]} /></Row>
    <Row label="Field error"><MapField label="Path mappings" keyLabel="Downloader path" valueLabel="corsarr path" mono value={[{ key: '/downloads', value: '' }]} error="Every downloader path needs a corsarr path." /></Row>
    <Row label="Managed by config"><MapField label="Path mappings" keyLabel="Downloader path" valueLabel="corsarr path" mono value={[{ key: '/downloads', value: '/data/downloads' }]} managed="downloaders.qbit.path_map" /></Row>
    <Row label="Drift"><MapField label="Path mappings" keyLabel="Downloader path" valueLabel="corsarr path" mono value={[{ key: '/dl', value: '/data/dl' }]} drift="The config file maps /downloads instead." /></Row>
    <Row label="Disabled"><MapField label="Extra headers" keyLabel="Header" valueLabel="Value" value={[{ key: 'X-Token', value: 'abc' }]} disabled /></Row>
  </ThemePair>
</Story>
