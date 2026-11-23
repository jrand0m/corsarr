<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import FilterBar from './FilterBar.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';
  import SegmentedControl from '../../molecules/SegmentedControl/SegmentedControl.svelte';
  import type { FilterFacet } from '../../../filters/url';

  const facets: FilterFacet[] = [
    { id: 'status', label: 'Status', options: [
      { value: 'missing', label: 'Missing', count: 23 }, { value: 'cutoff-unmet', label: 'Below cutoff', count: 41 },
      { value: 'cutoff-met', label: 'On disk', count: 336 }, { value: 'not-available', label: 'Not available yet', count: 12 },
    ] },
    { id: 'profile', label: 'Profile', options: [
      { value: 'hd', label: 'HD', count: 280 }, { value: 'uhd', label: 'UHD', count: 92 }, { value: 'any', label: 'Any', count: 40 },
    ] },
    { id: 'monitored', label: 'Monitoring', multiple: false, options: [
      { value: 'yes', label: 'Monitored' }, { value: 'no', label: 'Unmonitored' }, { value: 'pinned', label: 'Pinned' },
    ] },
    { id: 'tag', label: 'Tag', options: [{ value: 'kids', label: 'kids' }, { value: 'anime', label: 'anime' }, { value: '4k-only', label: '4k-only' }] },
  ];

  const { Story } = defineMeta({
    title: 'Organisms/FilterBar',
    component: FilterBar,
    tags: ['autodocs'],
    args: { facets, searchLabel: 'Filter series', placeholder: 'Filter by title', summary: '412 series', shortcut: false },
  });
</script>

<Story name="Playground" />

<Story name="All states" asChild>
  <ThemePair>
    <Row label="Nothing active">
      <div style="width: 100%"><FilterBar {facets} searchLabel="Filter series" summary="412 series" shortcut={false} /></div>
    </Row>
    <Row label="Text and facets active, with a view switch">
      <div style="width: 100%">
        <FilterBar {facets} searchLabel="Filter series" summary="7 of 412 series" shortcut={false}
          value={{ q: 'haul', facets: { status: ['missing', 'cutoff-unmet'], monitored: ['yes'] } }}>
          {#snippet trailing()}<SegmentedControl label="View" iconOnly size="sm" options={[{ value: 'table', label: 'Table', icon: 'list' }, { value: 'posters', label: 'Posters', icon: 'library' }]} />{/snippet}
        </FilterBar>
      </div>
    </Row>
    <Row label="Text only, no facets">
      <div style="width: 100%"><FilterBar searchLabel="Filter logs" placeholder="Filter messages" shortcut={false} value={{ q: 'timeout', facets: {} }} summary="18 lines" /></div>
    </Row>
  </ThemePair>
</Story>
