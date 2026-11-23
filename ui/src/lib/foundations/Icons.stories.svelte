<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import ThemePair from '../storybook/ThemePair.svelte';
  import Icon from '../components/atoms/Icon/Icon.svelte';
  import { iconNames } from '../components/atoms/Icon/icons';

  const { Story } = defineMeta({ title: 'Foundations/Icons', tags: ['autodocs'] });
</script>

<Story name="Set" asChild>
  <ThemePair>
    <p style="font-size: var(--text-sm); color: var(--ink-muted)">{iconNames.length} authored icons · 24 px grid · 1.75 stroke · round caps and joins · currentColor.</p>
    <div style="display: grid; grid-template-columns: repeat(auto-fill, minmax(6.5rem, 1fr)); gap: var(--space-2)">
      {#each iconNames as name (name)}
        <div style="display: grid; justify-items: center; gap: var(--space-1); padding: var(--space-2); border: 1px solid var(--line); border-radius: var(--radius-md); background: var(--surface-raised)">
          <Icon {name} size="lg" />
          <code style="font-size: var(--text-2xs); color: var(--ink-muted)">{name}</code>
        </div>
      {/each}
    </div>
  </ThemePair>
</Story>
