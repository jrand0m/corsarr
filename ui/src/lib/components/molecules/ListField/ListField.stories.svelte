<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import ListField from './ListField.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';

  const { Story } = defineMeta({ title: 'Molecules/ListField', component: ListField, tags: ['autodocs'], args: { label: 'Extra trackers', itemLabel: 'tracker', mono: true, value: ['udp://tracker.example.org:1337/announce'] } });
</script>

<Story name="Playground" />

<Story name="All states" asChild>
  <ThemePair>
    <Row label="Entries"><ListField label="Ignored words" itemLabel="word" value={['sample', 'trailer', 'extras']} help="Releases containing any of these are skipped." /></Row>
    <Row label="Empty"><ListField label="Extra trackers" itemLabel="tracker" value={[]} /></Row>
    <Row label="Entry error"><ListField label="Extra trackers" itemLabel="tracker" mono value={['udp://tracker.example.org:1337/announce', 'tracker.example']} itemErrors={{ 1: 'Add the scheme: udp:// or https://' }} /></Row>
    <Row label="Field error"><ListField label="Tags" itemLabel="tag" value={[]} required error="Add at least one tag; the rule matches on tags." /></Row>
    <Row label="Managed by config"><ListField label="Ignored words" itemLabel="word" value={['sample', 'trailer']} managed="formats.ignore.words" /></Row>
    <Row label="Drift"><ListField label="Tags" itemLabel="tag" value={['4k']} drift="The config file also lists hdr." /></Row>
    <Row label="Disabled"><ListField label="Tags" itemLabel="tag" value={['kids']} disabled /></Row>
  </ThemePair>
</Story>
