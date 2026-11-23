<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import PosterGrid, { type PosterItem } from './PosterGrid.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';
  import Button from '../../atoms/Button/Button.svelte';
  import EmptyState from '../../molecules/EmptyState/EmptyState.svelte';
  import { libraryRows } from '../../../storybook/library-rows';

  // No remote art in stories: every tile shows the initials fallback, and a few use a
  // local data-URI swatch to show loaded art.
  const swatch = (h: number) =>
    `data:image/svg+xml,${encodeURIComponent(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 2 3"><rect width="2" height="3" fill="oklch(0.45 0.06 ${h})"/><rect y="2" width="2" height="1" fill="oklch(0.3 0.04 ${h})"/></svg>`)}`;
  const toItems = (n: number): PosterItem[] =>
    libraryRows(n).map((r, i) => ({
      id: r.id, title: r.title, kind: r.kind, year: r.year,
      src: i % 3 === 0 ? swatch((i * 47) % 360) : null,
      detail: r.kind === 'tv' ? `${r.have} of ${r.total}` : undefined,
      status: { family: 'item', value: r.status },
    }));
  const many = toItems(5000);
  const few = toItems(9);

  const { Story } = defineMeta({ title: 'Organisms/PosterGrid', component: PosterGrid, tags: ['autodocs'], args: { items: many, label: 'Library', height: '32rem' } });
</script>

<Story name="Playground" />

<Story name="All states" asChild>
  <ThemePair>
    <Row label="5,000 items, virtualised by row"><div style="width: 100%"><PosterGrid items={many} label="Library" height="28rem" current="item-4" /></div></Row>
    <Row label="A few items"><div style="width: 100%"><PosterGrid items={few} label="Small library" height="auto" /></div></Row>
    <Row label="Loading"><div style="width: 100%"><PosterGrid items={[]} label="Library loading" loading height="22rem" /></div></Row>
    <Row label="Loading more"><div style="width: 100%"><PosterGrid items={few} label="Library, more" loadingMore hasMore height="auto" /></div></Row>
    <Row label="Empty">
      <div style="width: 100%">
        <PosterGrid items={[]} label="Library, empty">
          {#snippet empty()}<EmptyState title="No movies match these filters" icon="film">Clear a filter to see more.{#snippet actions()}<Button>Clear all</Button>{/snippet}</EmptyState>{/snippet}
        </PosterGrid>
      </div>
    </Row>
  </ThemePair>
</Story>
