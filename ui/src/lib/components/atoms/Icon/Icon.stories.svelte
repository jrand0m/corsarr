<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import Icon from './Icon.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';

  const { Story } = defineMeta({
    title: 'Atoms/Icon',
    component: Icon,
    tags: ['autodocs'],
    args: { name: 'download', size: 'md' },
  });
</script>

<Story name="Playground" />

<Story name="Sizes and colour" asChild>
  <ThemePair>
    <Row label="sm 14 · md 16 · lg 20">
      <Icon name="heart-pulse" size="sm" /><Icon name="heart-pulse" /><Icon name="heart-pulse" size="lg" />
    </Row>
    <Row label="Inherits currentColor">
      <span style="color: var(--tone-ok-ink)"><Icon name="check-circle" /></span>
      <span style="color: var(--tone-warn-ink)"><Icon name="alert-triangle" /></span>
      <span style="color: var(--tone-error-ink)"><Icon name="x-circle" /></span>
      <span style="color: var(--accent-ink)"><Icon name="search" label="Search" /></span>
    </Row>
  </ThemePair>
</Story>
