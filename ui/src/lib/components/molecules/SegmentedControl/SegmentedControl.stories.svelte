<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import SegmentedControl from './SegmentedControl.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';
  const views = [{ value: 'table', label: 'Table', icon: 'list' as const }, { value: 'posters', label: 'Posters', icon: 'library' as const }];
  const ranges = [{ value: '24h', label: '24 h' }, { value: '7d', label: '7 d' }, { value: '30d', label: '30 d' }];
  const { Story } = defineMeta({ title: 'Molecules/SegmentedControl', component: SegmentedControl, tags: ['autodocs'], args: { label: 'View', value: 'table', options: views } });
</script>

<Story name="Playground" />

<Story name="All states" asChild>
  <ThemePair>
    <Row label="With icons"><SegmentedControl label="View" value="table" options={views} /></Row>
    <Row label="Icon only"><SegmentedControl label="View" value="posters" options={views} iconOnly /></Row>
    <Row label="Small, text"><SegmentedControl label="Range" value="7d" options={ranges} size="sm" /></Row>
    <Row label="Disabled option"><SegmentedControl label="Calendar view" value="week" options={[{ value: 'week', label: 'Week' }, { value: 'month', label: 'Month' }, { value: 'agenda', label: 'Agenda', disabled: true }]} /></Row>
  </ThemePair>
</Story>
