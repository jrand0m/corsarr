<!--
  MediaKindIcon: the media kind as icon, always labelled. `showLabel` prints the kind
  name next to the icon; otherwise the name is the icon's accessible label.
-->
<script lang="ts" module>
  import type { MediaKind } from '../../../tokens';
  export const kindLabels: Record<MediaKind, string> = { tv: 'TV', movie: 'Movie', music: 'Music', book: 'Book' };
</script>

<script lang="ts">
  import Icon from '../Icon/Icon.svelte';
  import type { IconName } from '../Icon/icons';
  import type { Size } from '../../../tokens';

  let { kind, showLabel = false, size = 'md' }: { kind: MediaKind; showLabel?: boolean; size?: Size } = $props();
  const glyphs: Record<MediaKind, IconName> = { tv: 'tv', movie: 'film', music: 'music', book: 'book' };
</script>

<span class={['kind', kind]}>
  <Icon name={glyphs[kind]} {size} label={showLabel ? undefined : kindLabels[kind]} />
  {#if showLabel}<span class="label">{kindLabels[kind]}</span>{/if}
</span>

<style>
  .kind { display: inline-flex; align-items: center; gap: var(--space-1); font-size: var(--text-sm); color: var(--ink-muted); vertical-align: middle; }
  .kind :global(svg) { color: var(--c); }
  .tv { --c: var(--kind-tv); }
  .movie { --c: var(--kind-movie); }
  .music { --c: var(--kind-music); }
  .book { --c: var(--kind-book); }
</style>
