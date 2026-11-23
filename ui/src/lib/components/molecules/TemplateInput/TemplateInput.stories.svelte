<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import TemplateInput from './TemplateInput.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';
  import type { TemplateToken, TemplateSample } from './TemplateInput.svelte';
  const tokens: TemplateToken[] = [
    { token: 'Series Title', group: 'Series' }, { token: 'Series Year', group: 'Series' },
    { token: 'Season', group: 'Episode' }, { token: 'Episode', group: 'Episode' }, { token: 'Episode Title', group: 'Episode' },
    { token: 'Quality', group: 'Release' }, { token: 'Release Group', group: 'Release' },
  ];
  const samples: TemplateSample[] = [
    { name: 'Single episode', values: { 'Series Title': 'The Long Haul', 'Series Year': 2024, Season: 2, Episode: 3, 'Episode Title': 'Customs', Quality: 'WEBDL-1080p', 'Release Group': 'HARBOUR' } },
    { name: 'No episode title', values: { 'Series Title': 'Dockside Nights', 'Series Year': 2026, Season: 1, Episode: 12, 'Episode Title': '', Quality: 'HDTV-720p', 'Release Group': '' } },
  ];
  const msgTokens: TemplateToken[] = [{ token: 'Title', group: 'Item' }, { token: 'Quality', group: 'Release' }, { token: 'Size', group: 'Release' }, { token: 'Indexer', group: 'Release' }];
  const msgSamples: TemplateSample[] = [{ name: 'Telegram', values: { Title: 'The Long Haul S02E03', Quality: '1080p WEB', Size: '2.1 GB', Indexer: 'NZBgeek' } }];
  const { Story } = defineMeta({ title: 'Molecules/TemplateInput', component: TemplateInput, tags: ['autodocs'], args: { label: 'Episode file name', value: '{Series Title} - S{Season:00}E{Episode:00} - {Episode Title} [{Quality}]', tokens, samples } });
</script>

<Story name="Playground" />

<Story name="All states" asChild>
  <ThemePair>
    <Row label="File name with examples"><TemplateInput label="Episode file name" value={'{Series Title} - S{Season:00}E{Episode:00} - {Episode Title} [{Quality}]'} {tokens} {samples} /></Row>
    <Row label="Unknown token"><TemplateInput label="Episode file name" value={'{Series Title} - {Air Date} [{Quality}]'} {tokens} {samples} /></Row>
    <Row label="Empty"><TemplateInput label="Folder name" value="" {tokens} samples={samples.slice(0, 1)} placeholder={'{Series Title} ({Series Year})'} /></Row>
    <Row label="Multiline message"><TemplateInput label="Import message" multiline value={'Imported: {Title}\n{Quality} · {Size} · from {Indexer}'} tokens={msgTokens} samples={msgSamples} /></Row>
    <Row label="Read-only (managed)"><TemplateInput label="Episode file name" readonly value={'{Series Title} - S{Season:00}E{Episode:00}'} {tokens} samples={samples.slice(0, 1)} /></Row>
  </ThemePair>
</Story>
