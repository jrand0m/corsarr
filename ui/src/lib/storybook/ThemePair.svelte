<!--
  Story helper (not a design-system component): renders the same content in the light and the
  dark theme side by side. Tokens are re-declared on any [data-theme] element (tokens.css), so
  each panel carries its own theme regardless of the toolbar.
-->
<script lang="ts">
  import type { Snippet } from 'svelte';
  let { children }: { children: Snippet } = $props();
</script>

<div class="pair">
  <section class="panel" data-theme="light" aria-label="Light theme">
    {@render children()}
  </section>
  <section class="panel" data-theme="dark" aria-label="Dark theme">
    {@render children()}
  </section>
</div>

<style>
  .pair { display: grid; grid-template-columns: repeat(auto-fit, minmax(min(100%, 22rem), 1fr)); gap: var(--space-4); }
  .panel {
    background: var(--surface-ground);
    color: var(--ink);
    border: var(--border-w) solid var(--line);
    border-radius: var(--radius-lg);
    padding: var(--space-6);
    display: grid;
    gap: var(--space-5);
    align-content: start;
  }
</style>
