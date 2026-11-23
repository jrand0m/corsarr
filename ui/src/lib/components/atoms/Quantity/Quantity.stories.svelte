<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import Quantity from './Quantity.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';

  const { Story } = defineMeta({
    title: 'Atoms/Quantity',
    component: Quantity,
    tags: ['autodocs'],
    args: { kind: 'bytes', value: 4_700_000_000 },
    argTypes: { kind: { control: 'select', options: ['bytes', 'speed', 'duration', 'gbph', 'count', 'percent'] } },
  });
</script>

<Story name="Playground" />

<Story name="All kinds" asChild>
  <ThemePair>
    <Row label="Sizes"><Quantity kind="bytes" value={812} /><Quantity kind="bytes" value={734_000_000} /><Quantity kind="bytes" value={4_700_000_000} /><Quantity kind="bytes" value={2_100_000_000_000} /></Row>
    <Row label="Speed"><Quantity kind="speed" value={0} /><Quantity kind="speed" value={2_400_000} /><Quantity kind="speed" value={38_500_000} /></Row>
    <Row label="Duration"><Quantity kind="duration" value={42} /><Quantity kind="duration" value={1260} /><Quantity kind="duration" value={12_000} /><Quantity kind="duration" value={190_000} /></Row>
    <Row label="GB per hour"><Quantity kind="gbph" value={3_400_000_000} runtime={2700} /><Quantity kind="gbph" value={14_000_000_000} runtime={3600} over /></Row>
    <Row label="Count vs limit, percent"><Quantity kind="count" value={12} limit={13} /><Quantity kind="count" value={5000} /><Quantity kind="count" value={6} limit={5} over /><Quantity kind="percent" value={0.42} /></Row>
    <Row label="Column alignment (tabular figures)">
      <span style="display: grid; justify-items: end"><Quantity kind="bytes" value={111_100_000} /><Quantity kind="bytes" value={988_800_000} /><Quantity kind="bytes" value={404_000_000} /></span>
    </Row>
  </ThemePair>
</Story>
