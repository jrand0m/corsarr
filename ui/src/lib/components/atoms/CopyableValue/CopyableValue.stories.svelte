<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import CopyableValue from './CopyableValue.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';

  const { Story } = defineMeta({
    title: 'Atoms/CopyableValue',
    component: CopyableValue,
    tags: ['autodocs'],
    args: { label: 'iCal URL', value: 'http://corsarr.lan/api/v1/calendar.ics' },
  });
</script>

<Story name="Playground" />

<Story name="Variants" asChild>
  <ThemePair>
    <Row label="URL"><CopyableValue label="Torznab URL" value="http://corsarr.lan/api/v1/torznab/harbour-public" /></Row>
    <Row label="Secret (masked until shown)"><CopyableValue label="API key" value="c0rs-4f9a2e71b3d8460a9e12" secret /></Row>
    <Row label="Plain text"><CopyableValue label="Version" value="corsarr 0.4.0 (linux/arm64)" mono={false} /></Row>
  </ThemePair>
</Story>
