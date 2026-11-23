<!-- Foundations helper: one colour token. -->
<script lang="ts">
  let { token, ink }: { token: string; ink?: string } = $props();
</script>

<div class="sw">
  <span class="chip" style:background="var(--{token})" style:color={ink ? `var(--${ink})` : undefined}>{ink ? 'Aa' : ''}</span>
  <code>--{token}</code>
</div>

<style>
  .sw { display: grid; gap: var(--space-1); width: 9.5rem; }
  .chip { display: grid; place-items: center; height: 2.75rem; border-radius: var(--radius-md); box-shadow: inset 0 0 0 1px var(--line); font-weight: var(--weight-semibold); }
  code { font-size: var(--text-xs); color: var(--ink-muted); overflow-wrap: anywhere; }
</style>
