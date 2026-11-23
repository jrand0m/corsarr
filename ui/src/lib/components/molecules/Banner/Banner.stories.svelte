<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import Banner from './Banner.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';
  import Button from '../../atoms/Button/Button.svelte';
  const { Story } = defineMeta({ title: 'Molecules/Banner', component: Banner, tags: ['autodocs'], args: { preset: 'disconnected' } });
</script>

<Story name="Playground">
  {#snippet template(args)}<Banner {...args}>Showing what was true at 14:05. corsarr retries every few seconds.</Banner>{/snippet}
</Story>

<Story name="All presets" asChild>
  <ThemePair>
    <Row label="Disconnected"><Banner preset="disconnected" live>Showing what was true at 14:05; reconnecting.{#snippet actions()}<Button size="sm" icon="refresh">Reconnect now</Button>{/snippet}</Banner></Row>
    <Row label="Stale"><Banner preset="stale">The downloader has not answered for 6 minutes; progress may lag.</Banner></Row>
    <Row label="Shadow"><Banner preset="shadow">Music is in shadow mode: corsarr shows what it would grab and import, and changes nothing.</Banner></Row>
    <Row label="Managed"><Banner preset="managed">This indexer comes from corsarr.yaml. Edit the file to change it.{#snippet actions()}<Button size="sm" icon="copy">Copy as YAML</Button>{/snippet}</Banner></Row>
    <Row label="Restart needed"><Banner preset="restart" ondismiss={() => {}}>The new listen port takes effect after a restart.</Banner></Row>
    <Row label="Preview"><Banner preset="preview">Unsaved dry run of the stalled rule: nothing here has happened.</Banner></Row>
    <Row label="Error tone, custom title"><Banner tone="error" icon="alert-octagon" title="Imports are blocked">The library disk is read-only.</Banner></Row>
  </ThemePair>
</Story>
