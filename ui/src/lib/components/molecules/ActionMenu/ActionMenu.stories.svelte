<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import ActionMenu from './ActionMenu.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';
  import type { ActionItem } from './ActionMenu.svelte';
  const items: ActionItem[] = [
    { id: 'search', label: 'Search now', icon: 'search', group: 'a' },
    { id: 'rename', label: 'Rename files…', icon: 'pencil', group: 'a' },
    { id: 'refresh', label: 'Refresh metadata', icon: 'refresh', group: 'a', disabled: true },
    { id: 'delete', label: 'Delete…', icon: 'trash', group: 'b', danger: true },
  ];
  const { Story } = defineMeta({ title: 'Molecules/ActionMenu', component: ActionMenu, tags: ['autodocs'], args: { label: 'More actions for Dockside (2026)', items } });
</script>

<Story name="Playground" />

<Story name="All states" asChild>
  <ThemePair>
    <Row label="Icon trigger, text trigger, secondary, small">
      <ActionMenu label="More actions for Dockside (2026)" {items} />
      <ActionMenu label="Act on 3 series" text="Act on 3" icon="chevron-down" variant="secondary" {items} />
      <ActionMenu label="Row actions" size="sm" items={items.slice(0, 2)} />
    </Row>
  </ThemePair>
</Story>
