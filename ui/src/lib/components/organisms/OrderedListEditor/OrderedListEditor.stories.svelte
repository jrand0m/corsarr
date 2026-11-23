<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import OrderedListEditor, { type OrderedItem } from './OrderedListEditor.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';

  const ladder: OrderedItem[] = [
    { id: 'remux-2160', label: 'Remux-2160p', enabled: false, detail: '40–80 GB per film' },
    { id: 'uhd', label: 'UHD 2160p', children: [
      { id: 'bluray-2160', label: 'Bluray-2160p', enabled: true },
      { id: 'web-2160', label: 'WEB-2160p', enabled: true },
    ], enabled: true },
    { id: 'bluray-1080', label: 'Bluray-1080p', enabled: true },
    { id: 'web-1080', label: 'WEB-1080p', enabled: true },
    { id: 'hdtv-1080', label: 'HDTV-1080p', enabled: true },
    { id: 'web-720', label: 'WEB-720p', enabled: true },
    { id: 'sdtv', label: 'SDTV', enabled: false, locked: 'Below the floor for this profile' },
  ];
  const indexers: OrderedItem[] = [
    { id: 'nzbgeek', label: 'nzbgeek', detail: 'Usenet · 42 ms' },
    { id: 'drunken', label: 'drunkenslug', detail: 'Usenet · 96 ms' },
    { id: 'tracker', label: 'harbour-tracker', detail: 'Torrent · 210 ms' },
  ];

  const { Story } = defineMeta({
    title: 'Organisms/OrderedListEditor',
    component: OrderedListEditor,
    tags: ['autodocs'],
    args: { items: ladder, label: 'Quality order for HD', hint: 'Top is most preferred. Rows in a group count the same.', toggleable: true, groupable: true },
  });
</script>

<Story name="Playground" />

<Story name="All states" asChild>
  <ThemePair>
    <Row label="Quality ladder with a group, a disabled row and a locked row">
      <div style="width: 100%"><OrderedListEditor items={ladder} label="Quality order for HD" hint="Top is most preferred. Rows in a group count the same." toggleable groupable /></div>
    </Row>
    <Row label="Plain order, compact">
      <div style="width: 100%"><OrderedListEditor items={indexers} label="Indexer priority" hint="Searched top to bottom." density="compact" /></div>
    </Row>
    <Row label="Read-only (managed by config)">
      <div style="width: 100%"><OrderedListEditor items={ladder} label="Quality order, managed" toggleable readonly /></div>
    </Row>
    <Row label="Single row">
      <div style="width: 100%"><OrderedListEditor items={indexers.slice(0, 1)} label="One indexer" /></div>
    </Row>
  </ThemePair>
</Story>
