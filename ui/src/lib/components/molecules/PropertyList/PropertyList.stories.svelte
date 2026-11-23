<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import PropertyList from './PropertyList.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';
  import StatusBadge from '../../atoms/StatusBadge/StatusBadge.svelte';
  import Timestamp from '../../atoms/Timestamp/Timestamp.svelte';
  import type { Property } from './PropertyList.svelte';
  const now = new Date('2026-11-22T14:05:00Z');
  const props: Property[] = [
    { key: 'client', label: 'Downloader', value: 'qBittorrent (home-nas)', href: '#downloader' },
    { key: 'category', label: 'Category', value: 'corsarr-tv', managed: true },
    { key: 'hash', label: 'Info hash', value: '3f9a1c0b8e2d47a6b5c4d3e2f1a0b9c8d7e6f5a4', copy: true },
    { key: 'path', label: 'Save path', value: '/downloads/complete/The.Long.Haul.S02', mono: true },
    { key: 'state', label: 'State', custom: true },
    { key: 'added', label: 'Added', custom: true },
    { key: 'ratio', label: 'Seed ratio', value: null },
  ];
  const { Story } = defineMeta({ title: 'Molecules/PropertyList', component: PropertyList, tags: ['autodocs'], args: { items: props } });
</script>

<Story name="Playground" />

{#snippet custom(p: Property)}
  {#if p.key === 'state'}<StatusBadge family="download" value="stalled" detail="no peers" />{:else}<Timestamp value="2026-11-22T11:40:00Z" {now} />{/if}
{/snippet}

<Story name="All states" asChild>
  <ThemePair>
    <Row label="Comfortable, with link, copy, mono, custom, managed and missing"><PropertyList items={props} value={custom} label="Download properties" /></Row>
    <Row label="Compact"><PropertyList density="compact" items={props.slice(0, 4)} /></Row>
    <Row label="Stacked"><PropertyList layout="stacked" items={[{ key: 'api', label: 'API key', value: 'c0rs4rr-9f3e-41aa-b7d2-5e1c', copy: true, secret: true }, { key: 'ical', label: 'iCal URL', value: 'http://corsarr.lan:8090/feed/calendar.ics?key=…', copy: true }]} /></Row>
  </ThemePair>
</Story>
