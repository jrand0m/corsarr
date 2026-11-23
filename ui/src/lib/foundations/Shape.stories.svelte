<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import ThemePair from '../storybook/ThemePair.svelte';
  import Row from '../storybook/Row.svelte';
  import { tokenGroups } from '../tokens';

  const { Story } = defineMeta({ title: 'Foundations/Radii and elevation', tags: ['autodocs'] });
</script>

<Story name="Radii" asChild>
  <ThemePair>
    <Row label="xs bay cells, progress · sm badges, chips · md controls · lg panels, dialogs · round dots, switch">
      {#each tokenGroups.radius as t (t)}
        <div style="display: grid; gap: var(--space-1); justify-items: center; font-size: var(--text-xs); color: var(--ink-muted)">
          <span style="width: 3.5rem; height: 3.5rem; background: var(--surface-raised); border: 1px solid var(--line-strong); border-radius: var(--{t})"></span>
          <code>{t}</code>
        </div>
      {/each}
    </Row>
  </ThemePair>
</Story>

<Story name="Elevation" asChild>
  <ThemePair>
    <Row label="Surfaces are flat at rest; shadows only on overlays">
      {#each tokenGroups.shadow as t (t)}
        <div style="display: grid; place-items: center; width: 8rem; height: 5rem; background: var(--surface-overlay); border: 1px solid var(--line); border-radius: var(--radius-lg); box-shadow: var(--{t}); font-size: var(--text-xs)">
          <code>{t}</code>
        </div>
      {/each}
    </Row>
    <p style="font-size: var(--text-sm); color: var(--ink-muted)">sm: menus and popovers · md: tooltips, toasts · lg: dialogs and drawers.</p>
  </ThemePair>
</Story>
