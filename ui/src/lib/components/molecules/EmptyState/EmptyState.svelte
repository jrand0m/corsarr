<!--
  EmptyState: why a region is empty and the one thing to do next. The title names the
  cause in corsarr's words ("Nothing is downloading"), the body adds context (last sweep, an
  active filter), `actions` holds the next step. `size="sm"` fits inside a table body or panel.
-->
<script lang="ts">
  import type { Snippet } from 'svelte';
  import Icon from '../../atoms/Icon/Icon.svelte';
  import type { IconName } from '../../atoms/Icon/icons';

  interface Props {
    title: string;
    icon?: IconName;
    size?: 'sm' | 'md';
    children?: Snippet;
    actions?: Snippet;
    /** Heading level for the title inside the page outline. */
    level?: 2 | 3 | 4;
  }
  let { title, icon = 'tray', size = 'md', children, actions, level = 3 }: Props = $props();
</script>

<div class={['empty', size]}>
  <span class="mark"><Icon name={icon} size={size === 'sm' ? 'md' : 'lg'} /></span>
  <svelte:element this={`h${level}`} class="title">{title}</svelte:element>
  {#if children}<p class="body">{@render children()}</p>{/if}
  {#if actions}<div class="actions">{@render actions()}</div>{/if}
</div>

<style>
  .empty { display: grid; justify-items: center; gap: var(--space-2); padding: var(--space-10) var(--space-4); text-align: center; color: var(--ink); }
  .sm { padding: var(--space-6) var(--space-3); gap: var(--space-1-5); }
  .mark {
    display: grid;
    place-items: center;
    width: var(--space-12);
    height: var(--space-12);
    margin-bottom: var(--space-1);
    color: var(--ink-subtle);
    background: var(--surface-sunken);
    border-radius: var(--radius-lg);
  }
  .sm .mark { width: var(--space-8); height: var(--space-8); border-radius: var(--radius-md); }
  .title { font-size: var(--text-lg); font-weight: var(--weight-semibold); }
  .sm .title { font-size: var(--text-base); }
  .body { font-size: var(--text-md); color: var(--ink-muted); max-width: 52ch; }
  .actions { display: flex; flex-wrap: wrap; justify-content: center; gap: var(--space-2); margin-top: var(--space-2); }
</style>
