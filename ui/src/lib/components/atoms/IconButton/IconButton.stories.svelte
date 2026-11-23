<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import IconButton from './IconButton.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';

  const { Story } = defineMeta({
    title: 'Atoms/IconButton',
    component: IconButton,
    tags: ['autodocs'],
    args: { icon: 'pause', label: 'Pause download', variant: 'ghost', size: 'md' },
  });
</script>

<Story name="Playground" />

<Story name="All variants and states" asChild>
  <ThemePair>
    <Row label="Variants">
      <IconButton icon="more" label="Row actions" />
      <IconButton icon="refresh" label="Refresh" variant="secondary" />
      <IconButton icon="plus" label="Add item" variant="primary" />
    </Row>
    <Row label="Sizes">
      <IconButton icon="x" label="Close" size="sm" />
      <IconButton icon="x" label="Close" />
      <IconButton icon="x" label="Close" size="lg" />
    </Row>
    <Row label="Toggle, disabled, loading">
      <IconButton icon="pause" label="Hold" pressed />
      <IconButton icon="pause" label="Hold" pressed={false} />
      <IconButton icon="trash" label="Delete" disabled />
      <IconButton icon="refresh" label="Refreshing" loading />
    </Row>
  </ThemePair>
</Story>
