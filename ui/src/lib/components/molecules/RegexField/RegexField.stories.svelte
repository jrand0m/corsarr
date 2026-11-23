<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import RegexField from './RegexField.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';

  const { Story } = defineMeta({ title: 'Molecules/RegexField', component: RegexField, tags: ['autodocs'], args: { label: 'Must contain', value: '\\b(1080p|2160p)\\b', sample: 'The.Long.Haul.S02E03.1080p.WEB.h264-HARBOUR' } });
</script>

<Story name="Playground" />

<Story name="All states" asChild>
  <ThemePair>
    <Row label="Matches"><RegexField label="Must contain" value={'\\b(1080p|2160p)\\b'} sample="The.Long.Haul.S02E03.1080p.WEB.h264-HARBOUR" help="Releases whose title does not match are rejected." /></Row>
    <Row label="No match"><RegexField label="Must not contain" value={'\\bCAM\\b'} sample="Dockside.2026.2160p.WEB-DL.DDP5.1" /></Row>
    <Row label="Invalid pattern"><RegexField label="Must contain" value={'(1080p'} sample="Dockside.2026.1080p" /></Row>
    <Row label="Empty"><RegexField label="Must contain" required /></Row>
    <Row label="Managed by config"><RegexField label="Ignore" value={'\\b(sample|trailer)\\b'} sample="Dockside.2026.Trailer.1080p" managed="formats.no-extras.pattern" /></Row>
    <Row label="Drift"><RegexField label="Ignore" value={'\\bsample\\b'} drift="The config file also ignores trailers." /></Row>
    <Row label="Disabled"><RegexField label="Ignore" value="" disabled /></Row>
  </ThemePair>
</Story>
