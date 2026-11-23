<!-- Story helper: a labelled row of specimens. -->
<script lang="ts">
  import type { Snippet } from 'svelte';
  let { label, children }: { label: string; children: Snippet } = $props();
</script>

<div class="row">
  <span class="label">{label}</span>
  <div class="items">{@render children()}</div>
</div>

<style>
  .row { display: grid; gap: var(--space-2); }
  .label { font-size: var(--text-xs); color: var(--ink-muted); font-weight: var(--weight-medium); }
  .items { display: flex; flex-wrap: wrap; gap: var(--space-3); align-items: center; }
</style>
