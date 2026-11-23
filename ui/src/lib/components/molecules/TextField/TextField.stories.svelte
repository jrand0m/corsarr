<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import TextField from './TextField.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';

  const { Story } = defineMeta({ title: 'Molecules/TextField', component: TextField, tags: ['autodocs'], args: { label: 'Category', help: 'Downloads land in this qBittorrent category.', value: 'corsarr-tv' } });
</script>

<Story name="Playground" />

<Story name="All states" asChild>
  <ThemePair>
    <Row label="Default"><TextField label="Category" value="corsarr-tv" help="Downloads land in this qBittorrent category." /></Row>
    <Row label="Required, empty"><TextField label="Name" required placeholder="Home NAS" /></Row>
    <Row label="Mono identifier"><TextField label="Indexer ID" mono value="nzbgeek-main" /></Row>
    <Row label="Error"><TextField label="Name" value="qbit" error="Another downloader is already called qbit." /></Row>
    <Row label="Managed by config"><TextField label="Category" value="tv-sonarr-old" managed="downloaders.qbit.category" /></Row>
    <Row label="Drift"><TextField label="Category" value="tv" drift="The config file says corsarr-tv; the running value is tv." /></Row>
    <Row label="Disabled"><TextField label="Category" value="" disabled /></Row>
  </ThemePair>
</Story>
