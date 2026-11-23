<!--
  AccessPage (template): S01 first run and S02 sign-in, the only screens outside the shell
 . A thin oxide band carries the mark; one centred column holds the title, the form
  and an optional notice (reverse-proxy trust, config source). Single column at every width.
-->
<script lang="ts">
  import type { Snippet } from 'svelte';

  interface Props {
    title: string;
    description?: string;
    /** The form. */
    children: Snippet;
    /** Under the form: notices, the API key on first run. */
    footer?: Snippet;
    /** Wider column for first run, which shows more. */
    width?: 'narrow' | 'wide';
  }
  let { title, description, children, footer, width = 'narrow' }: Props = $props();
</script>

<div class="ap">
  <header class="band">
    <span class="brand">
      <svg class="mark" viewBox="0 0 20 20" aria-hidden="true"><path d="M2 11h10v6H2zM8 4h10v6H8z" /></svg>
      <span>corsarr</span>
    </span>
  </header>
  <main class={['col', width]}>
    <div class="titles">
      <h1>{title}</h1>
      {#if description}<p class="desc">{description}</p>{/if}
    </div>
    <div class="card">{@render children()}</div>
    {#if footer}<div class="foot">{@render footer()}</div>{/if}
  </main>
</div>

<style>
  .ap { min-height: 100dvh; display: grid; grid-template-rows: auto 1fr; background: var(--surface-ground); color: var(--ink); }
  .band { display: flex; align-items: center; min-height: 3.5rem; padding: 0 var(--space-4); background: var(--shell-bg); color: var(--shell-ink); }
  .brand { display: flex; align-items: center; gap: var(--space-2); font-size: var(--text-lg); font-weight: var(--weight-semibold); letter-spacing: -0.01em; }
  .mark { width: 1.25rem; height: 1.25rem; fill: none; stroke: currentColor; stroke-width: 1.75; stroke-linejoin: round; }
  .col { width: 100%; margin: 0 auto; padding: var(--space-12) var(--space-4) var(--space-10); display: grid; gap: var(--space-5); align-content: start; }
  .narrow { max-width: 26rem; }
  .wide { max-width: 34rem; }
  .titles { display: grid; gap: var(--space-2); }
  h1 { font-size: var(--text-2xl); line-height: var(--leading-tight); text-wrap: balance; }
  .desc { color: var(--ink-muted); }
  .card { padding: var(--space-6); background: var(--surface-raised); border: var(--border-w) solid var(--line); border-radius: var(--radius-lg); }
  .foot { display: grid; gap: var(--space-3); font-size: var(--text-md); color: var(--ink-muted); }
  @media (width < 48rem) {
    .col { padding: var(--space-8) var(--space-4); }
    .card { padding: var(--space-5) var(--space-4); }
  }
</style>
