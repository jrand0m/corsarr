<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import UrlField from './UrlField.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';

  const { Story } = defineMeta({ title: 'Molecules/UrlField', component: UrlField, tags: ['autodocs'], args: { label: 'Address', required: true, value: 'http://10.0.0.20:8080' } });
</script>

<Story name="Playground" />

<Story name="All states" asChild>
  <ThemePair>
    <Row label="Valid"><UrlField label="Address" value="http://10.0.0.20:8080" help="The qBittorrent Web UI, as corsarr reaches it." /></Row>
    <Row label="Empty, required"><UrlField label="Address" required /></Row>
    <Row label="Server error"><UrlField label="Address" value="http://nas.local:8080" error="No answer from nas.local:8080 within 5 s." /></Row>
    <Row label="Managed by config"><UrlField label="Jellyfin address" value="http://jellyfin:8096" managed="mediaservers.jellyfin.url" /></Row>
    <Row label="Drift"><UrlField label="Address" value="http://10.0.0.5:8080" drift="The config file says http://qbit:8080." /></Row>
    <Row label="Disabled"><UrlField label="Address" value="" disabled /></Row>
  </ThemePair>
</Story>
