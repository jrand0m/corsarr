<!-- Wanted › Missing: links between the movie, TV and music lists (plain links, so focus and history behave). -->
<script lang="ts">
  import Button from '../../../lib/components/atoms/Button/Button.svelte';

  let { current }: { current: 'movie' | 'tv' | 'music' } = $props();
</script>

<nav class="kinds" aria-label="Media kind">
  <Button size="sm" icon="film" variant={current === 'movie' ? 'secondary' : 'ghost'} href="#/wanted/missing" aria-current={current === 'movie' ? 'page' : undefined}>Movies</Button>
  <Button size="sm" icon="tv" variant={current === 'tv' ? 'secondary' : 'ghost'} href="#/wanted/missing/tv" aria-current={current === 'tv' ? 'page' : undefined}>TV</Button>
  <Button size="sm" icon="music" variant={current === 'music' ? 'secondary' : 'ghost'} href="#/wanted/missing/music" aria-current={current === 'music' ? 'page' : undefined}>Music</Button>
</nav>

<style>
  .kinds { display: inline-flex; gap: var(--space-1); }
</style>
