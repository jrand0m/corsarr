<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import BoolField from './BoolField.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';

  const { Story } = defineMeta({ title: 'Molecules/BoolField', component: BoolField, tags: ['autodocs'], args: { label: 'Remove completed downloads', checked: true, help: 'After import, corsarr removes the download from qBittorrent once seeding goals are met.' } });
</script>

<Story name="Playground" />

<Story name="All states" asChild>
  <ThemePair>
    <Row label="On, with help"><BoolField label="Remove completed downloads" checked help="After import, corsarr removes the download once seeding goals are met." /></Row>
    <Row label="Off"><BoolField label="Use SSL" /></Row>
    <Row label="Error"><BoolField label="Accept the indexer's terms" error="This indexer needs its terms accepted before searching." /></Row>
    <Row label="Managed by config"><BoolField label="Use SSL" checked managed="downloaders.qbit.tls" /></Row>
    <Row label="Drift"><BoolField label="Start paused" drift="The config file says off; the downloader reports on." checked /></Row>
    <Row label="Disabled"><BoolField label="Sequential download" disabled /></Row>
  </ThemePair>
</Story>
