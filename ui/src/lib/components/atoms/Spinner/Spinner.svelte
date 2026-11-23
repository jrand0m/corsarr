<!--
  Spinner: small inline busy mark for buttons and inline loads. Regions use Skeleton
  instead. Decorative unless `label` is given. Under reduced motion it holds still.
-->
<script lang="ts">
  import type { Size } from '../../../tokens';
  let { size = 'md', label }: { size?: Size; label?: string } = $props();
</script>

<svg
  class={['spinner', size]}
  viewBox="0 0 24 24"
  role={label ? 'img' : undefined}
  aria-label={label}
  aria-hidden={label ? undefined : 'true'}
>
  <circle class="track" cx="12" cy="12" r="9" />
  <path class="arc" d="M12 3a9 9 0 0 1 9 9" />
</svg>

<style>
  .spinner { width: var(--icon-md); height: var(--icon-md); fill: none; stroke-width: 2.5; animation: spin 0.8s linear infinite; }
  .sm { width: var(--icon-sm); height: var(--icon-sm); }
  .lg { width: var(--icon-lg); height: var(--icon-lg); }
  .track { stroke: currentColor; opacity: 0.25; }
  .arc { stroke: currentColor; stroke-linecap: round; }
  @keyframes spin { to { transform: rotate(360deg); } }
</style>
