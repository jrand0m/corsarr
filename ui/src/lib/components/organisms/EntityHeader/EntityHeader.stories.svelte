<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import EntityHeader from './EntityHeader.svelte';
  import Button from '../../atoms/Button/Button.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';

  const menu = [
    { id: 'rename', label: 'Rename files…', icon: 'pencil' as const },
    { id: 'refresh', label: 'Refresh metadata', icon: 'refresh' as const },
    { id: 'delete', label: 'Delete series…', icon: 'trash' as const, danger: true, group: 'danger' },
  ];
  const { Story } = defineMeta({
    title: 'Organisms/EntityHeader',
    component: EntityHeader,
    tags: ['autodocs'],
    args: {
      title: 'The Long Haul', kind: 'tv', art: null, meta: '2024 · Harbourline · 52m', monitor: 'monitored',
      status: [{ family: 'item', value: 'cutoff-unmet', detail: '3 episodes' }],
      facts: [{ label: 'On disk', value: '38 of 41' }, { label: 'Size', value: '182 GB' }, { label: 'Profile', value: 'HD 1080p' }, { label: 'Next airing', value: 'Thu 26 Nov' }],
      back: { href: '#/library/tv', label: 'TV' },
    },
  });
</script>

{#snippet primary()}<Button variant="primary" icon="search">Search monitored</Button><Button icon="pencil">Edit</Button>{/snippet}

<Story name="Playground">
  {#snippet template(args)}<EntityHeader {...args} actions={primary} {menu} />{/snippet}
</Story>

<Story name="All states" asChild>
  <ThemePair>
    <Row label="TV series, monitored, below cutoff">
      <EntityHeader
        title="The Long Haul" kind="tv" art={null} meta="2024 · Harbourline · 52m" monitor="monitored"
        status={[{ family: 'item', value: 'cutoff-unmet', detail: '3 episodes' }]}
        facts={[{ label: 'On disk', value: '38 of 41' }, { label: 'Size', value: '182 GB' }, { label: 'Profile', value: 'HD 1080p' }]}
        actions={primary} {menu} back={{ href: '#/library/tv', label: 'TV' }}
      />
    </Row>
    <Row label="Movie, pinned, missing">
      <EntityHeader
        title="Dockside" kind="movie" art={null} meta="2026 · 1h 58m" monitor="pinned" monitorReason="Kept at 1080p for the projector"
        status={[{ family: 'item', value: 'missing' }]} facts={[{ label: 'Released', value: '14 Oct 2026' }]} actions={primary}
      />
    </Row>
    <Row label="Album (cover), CJK title, unmonitored">
      <EntityHeader title="港の灯り" kind="music" art={null} meta="2025 · Kaori Minato · 9 tracks" monitor="unmonitored" status={[{ family: 'item', value: 'cutoff-met' }]} />
    </Row>
    <Row label="Book, long RTL title, managed by config">
      <EntityHeader
        title="مرفأ الضوء: حكايات الحاويات والموانئ التي غيرت العالم" kind="book" art={null} meta="2023 · EPUB"
        status={[{ family: 'config', value: 'managed' }, { family: 'item', value: 'not-available' }]}
      />
    </Row>
  </ThemePair>
</Story>
