<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import ItemRefSummary from './ItemRefSummary.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';
  import type { ItemRef } from './ItemRefSummary.svelte';
  const titles = ['Low Tide', 'The Long Haul', 'Customs', 'Berth Nine', 'Night Shift', 'Manifest', 'Dry Dock', 'Pilot Boat', 'Ballast', 'Fog Signal', 'Gantry', 'Draft Marks', 'Slack Water'];
  const season: ItemRef[] = titles.map((t, i) => ({
    label: `S02E${String(i + 1).padStart(2, '0')} · ${t}`,
    status: i < 10 ? 'imported' : i === 10 ? 'blocked' : 'pending',
    reason: i === 10 ? 'sample file only' : undefined,
  }));
  const allDone: ItemRef[] = season.map((s) => ({ ...s, status: 'imported', reason: undefined }));
  const { Story } = defineMeta({ title: 'Molecules/ItemRefSummary', component: ItemRefSummary, tags: ['autodocs'], args: { summary: 'S02 · 13 episodes', items: season } });
</script>

<Story name="Playground" />

<Story name="All states" asChild>
  <ThemePair>
    <Row label="Mixed, collapsed"><ItemRefSummary summary="S02 · 13 episodes" items={season} /></Row>
    <Row label="Mixed, expanded"><ItemRefSummary summary="S02 · 13 episodes" items={season.slice(8)} open /></Row>
    <Row label="All imported"><ItemRefSummary summary="S02 · 13 episodes" items={allDone} /></Row>
    <Row label="Single item"><ItemRefSummary summary="Dockside (2026)" items={[{ label: 'Dockside (2026)', status: 'failed', reason: 'unpack error' }]} /></Row>
    <Row label="Album, all pending"><ItemRefSummary summary="Harbour Lights · 11 tracks" items={Array.from({ length: 11 }, (_, i) => ({ label: `Track ${i + 1}`, status: 'pending' as const }))} /></Row>
  </ThemePair>
</Story>
