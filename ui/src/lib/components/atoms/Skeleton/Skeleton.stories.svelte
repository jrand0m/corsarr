<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import Skeleton from './Skeleton.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';

  const { Story } = defineMeta({ title: 'Atoms/Skeleton', component: Skeleton, tags: ['autodocs'], args: { shape: 'text', lines: 3, width: '16rem' } });
</script>

<Story name="Playground" />

<Story name="Shapes" asChild>
  <ThemePair>
    <Row label="Text lines"><Skeleton lines={3} width="18rem" /></Row>
    <Row label="Poster + title (list row shape)">
      <div style="display: flex; gap: var(--space-3); align-items: center" aria-busy="true">
        <Skeleton shape="block" width="32px" height="48px" />
        <Skeleton lines={2} width="12rem" />
      </div>
    </Row>
    <Row label="Circle"><Skeleton shape="circle" width="2rem" /></Row>
  </ThemePair>
</Story>
