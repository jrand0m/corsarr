<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import ManagedValue from './ManagedValue.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';

  const { Story } = defineMeta({
    title: 'Atoms/ManagedValue', component: ManagedValue, tags: ['autodocs'],
    args: { value: 'Shadow', managed: { key: 'mediaKinds.tv.mode', kind: 'file', file: '/config/corsarr.yml' } },
  });
</script>

<Story name="Playground" />

<Story name="Sources" asChild>
  <ThemePair>
    <Row label="Config file"><ManagedValue value="Shadow" managed={{ key: 'mediaKinds.tv.mode', kind: 'file', file: '/config/corsarr.yml' }} /></Row>
    <Row label="Command-line arguments"><ManagedValue value="Active" managed={{ key: 'mediaKinds.movie.mode', kind: 'args', name: '--mediaKinds.movie.mode' }} /></Row>
    <Row label="Environment variable"><ManagedValue value="Shadow" managed={{ key: 'mediaKinds.tv.mode', kind: 'env', name: 'CORSARR_MEDIAKINDS_TV_MODE' }} /></Row>
    <Row label="Source unknown"><ManagedValue value="Config" managed={{ key: 'indexers.nzbgeek' }} /></Row>
    <Row label="Lock only (inside a field)"><ManagedValue managed={{ key: 'indexers.nzbgeek.api_key', kind: 'env', name: 'CORSARR_INDEXERS_NZBGEEK_API_KEY' }} /></Row>
    <Row label="Medium"><ManagedValue size="md" value="Shadow" managed={{ key: 'mediaKinds.tv.mode', kind: 'file', file: '/config/corsarr.yml' }} /></Row>
  </ThemePair>
</Story>
