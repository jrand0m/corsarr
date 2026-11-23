<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import BayGrid, { type BayCell, type BayState } from './BayGrid.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';

  const season = (states: BayState[]): BayCell[] => states.map((state, i) => ({ state, label: `E${String(i + 1).padStart(2, '0')}` }));
  const s2 = season(['have', 'have', 'have', 'have', 'upgrade', 'have', 'have', 'missing', 'downloading', 'queued', 'unaired', 'unaired']);
  const album = season(['have', 'have', 'have', 'blocked', 'have', 'have', 'have', 'have', 'have', 'missing']);
  const big = season(Array.from({ length: 64 }, (_, i): BayState => (i < 50 ? 'have' : i < 54 ? 'upgrade' : i < 58 ? 'unmonitored' : i < 60 ? 'missing' : 'unaired')));

  const { Story } = defineMeta({ title: 'Atoms/BayGrid', component: BayGrid, tags: ['autodocs'], args: { cells: s2, label: 'Season 2', legend: true } });
</script>

<script lang="ts">
  let picked = $state('none');
</script>

<Story name="Playground" />

<Story name="States and sizes" asChild>
  <ThemePair>
    <Row label="Season (with legend)"><BayGrid cells={s2} label="Season 2" legend /></Row>
    <Row label="Album tracks, small"><BayGrid cells={album} label="Salt and Signal" size="sm" legend /></Row>
    <Row label="Long season, small"><BayGrid cells={big} label="Season 1" size="sm" /></Row>
    <Row label="Interactive (arrow keys, Enter)">
      <BayGrid cells={s2} label="Season 2" onselect={(_, c) => (picked = c.label)} />
      <span>Selected: {picked}</span>
    </Row>
  </ThemePair>
</Story>
