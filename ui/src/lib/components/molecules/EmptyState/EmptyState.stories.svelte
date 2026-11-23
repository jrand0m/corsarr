<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import EmptyState from './EmptyState.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';
  import Button from '../../atoms/Button/Button.svelte';
  const { Story } = defineMeta({ title: 'Molecules/EmptyState', component: EmptyState, tags: ['autodocs'], args: { title: 'Nothing is downloading', icon: 'arrow-down' } });
</script>

<Story name="Playground">
  {#snippet template(args)}<EmptyState {...args}>The last wanted sweep ran 12 minutes ago and found nothing new.</EmptyState>{/snippet}
</Story>

<Story name="All states" asChild>
  <ThemePair>
    <Row label="Queue empty, with next action">
      <EmptyState title="Nothing is downloading" icon="arrow-down">
        The last wanted sweep ran 12 minutes ago and found nothing new.
        {#snippet actions()}<Button icon="search">Search wanted now</Button>{/snippet}
      </EmptyState>
    </Row>
    <Row label="Filtered to nothing">
      <EmptyState title="No series match these filters" icon="filter">
        3 filters are on.
        {#snippet actions()}<Button variant="ghost">Clear filters</Button>{/snippet}
      </EmptyState>
    </Row>
    <Row label="Small, inside a panel"><EmptyState size="sm" title="No blocklisted releases" icon="ban" /></Row>
  </ThemePair>
</Story>
