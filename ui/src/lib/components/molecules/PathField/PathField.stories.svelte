<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import PathField from './PathField.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';
  const locations = [
    { path: '/data/media/tv', label: 'TV library', free: 412e9, total: 2e12, access: 'writable' as const },
    { path: '/mnt/archive/tv', label: 'Archive disk', free: 61e9, total: 4e12, access: 'writable' as const },
    { path: '/mnt/usb/tv', label: 'USB disk', access: 'unreachable' as const, problem: 'not mounted' },
  ];
  const { Story } = defineMeta({ title: 'Molecules/PathField', component: PathField, tags: ['autodocs'], args: { label: 'Root folder', value: '/data/media/tv', locations } });
</script>

<Story name="Playground" />

<Story name="All states" asChild>
  <ThemePair>
    <Row label="Known locations"><PathField label="Root folder" value="/data/media/tv" {locations} help="New series go here unless you pick another." /></Row>
    <Row label="Plain path (no known locations)"><PathField label="Download folder" value="/downloads/complete" /></Row>
    <Row label="Required, empty"><PathField label="Download folder" required /></Row>
    <Row label="Error"><PathField label="Download folder" value="/downloads" error="corsarr cannot write to /downloads (permission denied for uid 1000)." /></Row>
    <Row label="Managed by config"><PathField label="Root folder" value="/data/media/tv" locations={locations.slice(0, 2)} managed="library.tv.root" /></Row>
    <Row label="Drift"><PathField label="Download folder" value="/downloads/tv" drift="The config file says /downloads/complete." /></Row>
    <Row label="Disabled"><PathField label="Download folder" value="" disabled /></Row>
  </ThemePair>
</Story>
