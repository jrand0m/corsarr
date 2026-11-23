<!--
  ErrorState: a region that failed to load. Says what failed in plain words, shows the
  error code for support and logs, offers Retry (busy while retrying) and a link to System ›
  Health when the cause is an integration. Partial pages render this in the failed section
  only. `announce` makes it an alert for failures that happen after the page settled.
-->
<script lang="ts">
  import type { Snippet } from 'svelte';
  import Icon from '../../atoms/Icon/Icon.svelte';
  import Button from '../../atoms/Button/Button.svelte';
  import Link from '../../atoms/Link/Link.svelte';

  interface Props {
    /** "Couldn't load the queue". */
    title: string;
    /** Machine-readable code: "HTTP 503 · downloader.unreachable". */
    code?: string;
    children?: Snippet;
    onretry?: () => void;
    retrying?: boolean;
    healthHref?: string;
    size?: 'sm' | 'md';
    announce?: boolean;
    level?: 2 | 3 | 4;
  }
  let { title, code, children, onretry, retrying = false, healthHref, size = 'md', announce = false, level = 3 }: Props = $props();
</script>

<div class={['err', size]} role={announce ? 'alert' : undefined}>
  <span class="mark"><Icon name="alert-octagon" size={size === 'sm' ? 'md' : 'lg'} /></span>
  <svelte:element this={`h${level}`} class="title">{title}</svelte:element>
  {#if children}<p class="body">{@render children()}</p>{/if}
  {#if code}<code class="code">{code}</code>{/if}
  {#if onretry || healthHref}
    <div class="actions">
      {#if onretry}<Button icon="refresh" loading={retrying} onclick={onretry}>{retrying ? 'Retrying' : 'Retry'}</Button>{/if}
      {#if healthHref}<Link href={healthHref}>Open health</Link>{/if}
    </div>
  {/if}
</div>

<style>
  .err { display: grid; justify-items: center; gap: var(--space-2); padding: var(--space-10) var(--space-4); text-align: center; }
  .sm { padding: var(--space-6) var(--space-3); gap: var(--space-1-5); }
  .mark {
    display: grid;
    place-items: center;
    width: var(--space-12);
    height: var(--space-12);
    margin-bottom: var(--space-1);
    color: var(--tone-error-ink);
    background: var(--tone-error-tint);
    border: var(--border-w) solid var(--tone-error-line);
    border-radius: var(--radius-lg);
  }
  .sm .mark { width: var(--space-8); height: var(--space-8); border-radius: var(--radius-md); }
  .title { font-size: var(--text-lg); font-weight: var(--weight-semibold); }
  .sm .title { font-size: var(--text-base); }
  .body { font-size: var(--text-md); color: var(--ink-muted); max-width: 52ch; }
  .code { font-size: var(--text-xs); color: var(--ink-muted); background: var(--surface-sunken); padding: var(--space-0-5) var(--space-1-5); border-radius: var(--radius-xs); overflow-wrap: anywhere; }
  .actions { display: flex; flex-wrap: wrap; justify-content: center; align-items: center; gap: var(--space-3); margin-top: var(--space-2); }
</style>
