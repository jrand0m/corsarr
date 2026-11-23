<!-- VisuallyHidden: text for assistive technology only. -->
<script lang="ts">
  import type { Snippet } from 'svelte';
  let { children }: { children: Snippet } = $props();
</script>

<span class="visually-hidden">{@render children()}</span>
