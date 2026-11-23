<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import PathPicker from './PathPicker.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';
  const locations = [
    { path: '/data/media/tv', label: 'TV library', free: 412e9, total: 2e12, access: 'writable' as const },
    { path: '/mnt/archive/tv', label: 'Archive disk', free: 61e9, total: 4e12, access: 'writable' as const },
    { path: '/mnt/nas/tv', label: 'NAS share', free: 900e9, total: 8e12, access: 'read-only' as const, problem: 'mounted ro' },
    { path: '/mnt/usb/tv', label: 'USB disk', access: 'unreachable' as const, problem: 'not mounted' },
  ];
  const { Story } = defineMeta({ title: 'Molecules/PathPicker', component: PathPicker, tags: ['autodocs'], args: { label: 'Root folder', value: '/data/media/tv', locations } });
</script>

<Story name="Playground" />

<Story name="All states" asChild>
  <ThemePair>
    <Row label="Known locations, one chosen"><PathPicker label="Root folder" value="/data/media/tv" {locations} /></Row>
    <Row label="Another folder typed"><PathPicker label="Root folder" value="/srv/tv-new" locations={locations.slice(0, 2)} /></Row>
    <Row label="No known locations"><PathPicker label="Download folder" value="/downloads/complete" /></Row>
    <Row label="Read-only (managed)"><PathPicker label="Root folder" value="/data/media/tv" locations={locations.slice(0, 2)} readonly /></Row>
    <Row label="Disabled"><PathPicker label="Root folder" value="" locations={locations.slice(0, 2)} disabled /></Row>
  </ThemePair>
</Story>
