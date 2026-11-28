<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import AvailabilityLine from './AvailabilityLine.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';

  const { Story } = defineMeta({ title: 'Molecules/AvailabilityLine', component: AvailabilityLine, tags: ['autodocs'], args: { status: 'complete', protocol: 'torrent', grabs: 210, files: 3, minimumRatio: 1 } });
</script>

<Story name="Playground" />

<Story name="All states" asChild>
  <ThemePair>
    <Row label="Complete, with evidence"><AvailabilityLine status="complete" protocol="torrent" grabs={210} files={3} minimumRatio={1} /></Row>
    <Row label="May be incomplete"><AvailabilityLine status="may_be_incomplete" protocol="torrent" grabs={4} /></Row>
    <Row label="No peers"><AvailabilityLine status="no_peers" protocol="torrent" /></Row>
    <Row label="Unknown (torrent)"><AvailabilityLine status="unknown" protocol="torrent" /></Row>
    <Row label="Unknown (usenet)"><AvailabilityLine status="unknown" protocol="usenet" grabs={57} /></Row>
    <Row label="Direct source"><AvailabilityLine status="complete" protocol="source" files={12} /></Row>
  </ThemePair>
</Story>
