<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import ThemePair from '../storybook/ThemePair.svelte';
  import { tokenGroups } from '../tokens';

  const { Story } = defineMeta({ title: 'Foundations/Spacing', tags: ['autodocs'] });
</script>

<Story name="Scale and density" asChild>
  <ThemePair>
    <div style="display: grid; gap: var(--space-1-5)">
      {#each tokenGroups.space as t (t)}
        <div style="display: flex; align-items: center; gap: var(--space-3); font-size: var(--text-xs)">
          <code style="width: 6rem; color: var(--ink-muted)">--{t}</code>
          <span style="height: 0.75rem; width: var(--{t}); background: var(--accent); border-radius: 1px"></span>
        </div>
      {/each}
    </div>
    <div style="display: grid; gap: var(--space-2)">
      <span style="font-size: var(--text-xs); color: var(--ink-muted)">Row heights: comfortable 44 px (default, phone always), compact 32 px (desktop opt-in)</span>
      <div style="display: grid; border: 1px solid var(--line); border-radius: var(--radius-md); overflow: hidden">
        <div style="height: var(--row-h-comfortable); display: flex; align-items: center; padding: 0 var(--space-3); border-bottom: 1px solid var(--line); font-size: var(--text-md)">Comfortable row</div>
        <div style="height: var(--row-h-compact); display: flex; align-items: center; padding: 0 var(--space-3); font-size: var(--text-sm)">Compact row</div>
      </div>
      <span style="font-size: var(--text-xs); color: var(--ink-muted)">Controls: sm 28 · md 36 · lg 44 px; every target grows to 44 px on phones and coarse pointers.</span>
    </div>
  </ThemePair>
</Story>
