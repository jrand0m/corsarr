<!--
  Artwork: poster, cover or avatar. Lazy, async-decoded, with reserved dimensions so
  lists never shift. When the image is missing or fails, shows the title's initials and the
  kind icon on the kind's colour. `title` is the accessible name (alt text).
-->
<script lang="ts">
  import Icon from '../Icon/Icon.svelte';
  import type { IconName } from '../Icon/icons';
  import type { MediaKind } from '../../../tokens';

  interface Props {
    src?: string | null;
    title: string;
    kind: MediaKind;
    shape?: 'poster' | 'cover' | 'avatar';
    /** Rendered width in px; height follows the shape. */
    width?: number;
    /** Above-the-fold artwork (entity header) loads eagerly. */
    eager?: boolean;
  }
  let { src, title, kind, shape = 'poster', width = 96, eager = false }: Props = $props();

  let failed = $state(false);
  $effect(() => { void src; failed = false; });

  const ratio = $derived(shape === 'poster' ? 1.5 : 1);
  const height = $derived(Math.round(width * ratio));
  const glyphs: Record<MediaKind, IconName> = { tv: 'tv', movie: 'film', music: 'music', book: 'book' };
  const initials = $derived.by(() => {
    const small = /^(the|a|an|and|of|in|on|to|for|&)$/i;
    const all = title.split(/\s+/).filter(Boolean);
    const words = all.filter((w) => !small.test(w));
    if (!words.length) words.push(...all);
    const chars = words.slice(0, 2).map((w) => Array.from(w)[0] ?? '');
    return chars.join('').toUpperCase();
  });
</script>

<span class={['art', shape, kind]} style:width="{width}px" style:height="{height}px">
  {#if src && !failed}
    <img {src} alt={title} {width} {height} loading={eager ? 'eager' : 'lazy'} decoding="async" onerror={() => (failed = true)} />
  {:else}
    <span class="fallback" role="img" aria-label={title}>
      <span class="initials" aria-hidden="true" style:font-size="{Math.max(12, width / 3.2)}px">{initials}</span>
      {#if width >= 48}<Icon name={glyphs[kind]} size={width >= 96 ? 'md' : 'sm'} />{/if}
    </span>
  {/if}
</span>

<style>
  .art {
    position: relative;
    display: inline-block;
    flex: none;
    overflow: hidden;
    background: var(--surface-sunken);
    border-radius: var(--radius-md);
    box-shadow: inset 0 0 0 1px var(--line);
    vertical-align: middle;
  }
  .avatar { border-radius: var(--radius-round); }
  img { width: 100%; height: 100%; object-fit: cover; }
  .fallback {
    position: absolute;
    inset: 0;
    display: grid;
    place-items: center;
    align-content: center;
    gap: var(--space-1);
    background: var(--c);
    color: var(--ink-on-kind);
  }
  .initials { font-weight: var(--weight-semibold); line-height: 1; letter-spacing: 0.02em; }
  .fallback :global(svg) { opacity: 0.85; }
  .tv { --c: var(--kind-tv-fill); }
  .movie { --c: var(--kind-movie-fill); }
  .music { --c: var(--kind-music-fill); }
  .book { --c: var(--kind-book-fill); }
</style>
