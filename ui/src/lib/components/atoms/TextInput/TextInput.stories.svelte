<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import TextInput from './TextInput.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';

  const { Story } = defineMeta({
    title: 'Atoms/TextInput',
    component: TextInput,
    tags: ['autodocs'],
    args: { placeholder: 'Search titles or provider:id', 'aria-label': 'Search', size: 'md' },
  });
</script>

<Story name="Playground" />

<Story name="All variants and states" asChild>
  <ThemePair>
    <Row label="Default, icon, mono"><TextInput aria-label="Name" value="Living-room TV" /><TextInput aria-label="Search" type="search" icon="search" placeholder="Filter (/)" /><TextInput aria-label="Path" mono value="/srv/media/tv" /></Row>
    <Row label="Sizes"><TextInput aria-label="Small" size="sm" value="sm" /><TextInput aria-label="Large" size="lg" value="lg" /></Row>
    <Row label="Invalid"><TextInput aria-label="URL" invalid value="htp:/indexer" /></Row>
    <Row label="Managed by config (read-only)"><TextInput aria-label="API URL" readonly value="http://qbittorrent:8080" /></Row>
    <Row label="Disabled"><TextInput aria-label="Disabled" disabled value="Off while kind is disabled" /></Row>
  </ThemePair>
</Story>
