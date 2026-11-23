<!--
  Drawer: a detail side sheet over a list (a download, an item, a run), so the owner
  inspects one row without losing their place. Slides in from the end edge; full screen on
  phones. Native modal `<dialog>`: the list behind is inert, Tab stays inside, Escape or a
  scrim click closes, focus returns to the row that opened it. URL addressing belongs to the
  route (`/activity/queue/{downloader}/{downloadId}`): bind `open` to it.
-->
<script lang="ts">
  import type { Snippet } from 'svelte';
  import IconButton from '../../atoms/IconButton/IconButton.svelte';
  import { trapTab, initialFocus } from '../../../overlay/focus';
  import type { DialogCloseReason } from '../Dialog/Dialog.svelte';

  interface Props {
    open?: boolean;
    title: string;
    /** Line under the title: "qBittorrent · tv-corsarr". */
    subtitle?: string;
    size?: 'sm' | 'md' | 'lg';
    closeLabel?: string;
    /** Header tools beside the close button (an ActionMenu, "Open full page"). */
    actions?: Snippet;
    /** Sticky footer for the drawer's primary action. */
    footer?: Snippet;
    children?: Snippet;
    onclose?: (reason: DialogCloseReason) => void;
    /** Off: renders open in place, without scrim or focus capture (previews and docs). */
    modal?: boolean;
  }
  let {
    open = $bindable(false), title, subtitle, size = 'md', closeLabel = 'Close details', actions, footer, children,
    onclose, modal = true,
  }: Props = $props();

  const id = $props.id();
  let el = $state<HTMLDialogElement>();
  let returnTo: HTMLElement | null = null;
  let reason: DialogCloseReason = 'programmatic';

  $effect(() => {
    if (!el) return;
    if (open && !el.open) {
      reason = 'programmatic';
      if (!modal) {
        el.show();
        return;
      }
      returnTo = document.activeElement instanceof HTMLElement ? document.activeElement : null;
      el.showModal();
      initialFocus(el).focus();
    } else if (!open && el.open) {
      el.close();
    }
  });

  function dismiss(why: DialogCloseReason) {
    reason = why;
    open = false;
  }
  function onclosed() {
    open = false;
    const target = returnTo;
    returnTo = null;
    if (target?.isConnected) target.focus();
    onclose?.(reason);
    reason = 'programmatic';
  }
</script>

<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_noninteractive_element_interactions -->
<dialog
  bind:this={el} class={['drawer', size]} class:inline={!modal} aria-labelledby="{id}-title"
  onclose={onclosed}
  oncancel={(e) => { e.preventDefault(); dismiss('escape'); }}
  onclick={(e) => { if (e.target === el) dismiss('scrim'); }}
  onkeydown={(e) => { if (el && modal) trapTab(e, el); }}
>
  <div class="sheet">
    <header>
      <div class="heading">
        <h2 id="{id}-title">{title}</h2>
        {#if subtitle}<p class="sub">{subtitle}</p>{/if}
      </div>
      {#if actions}<div class="tools">{@render actions()}</div>{/if}
      <span data-overlay-close><IconButton icon="x" label={closeLabel} tooltip={false} onclick={() => dismiss('button')} /></span>
    </header>
    <div class="body">{@render children?.()}</div>
    {#if footer}<footer>{@render footer()}</footer>{/if}
  </div>
</dialog>

<style>
  .drawer {
    --w: 32rem;
    position: fixed;
    inset: 0 0 0 auto;
    width: min(var(--w), 100vw);
    max-width: none;
    height: 100dvh;
    max-height: none;
    margin: 0;
    padding: 0;
    color: var(--ink);
    background: var(--surface-overlay);
    border: var(--border-w) solid var(--line);
    border-block: 0;
    border-inline-end: 0;
    box-shadow: var(--shadow-lg);
    translate: 0 0;
    transition:
      translate var(--dur-slow) var(--ease-out),
      display var(--dur-slow) allow-discrete,
      overlay var(--dur-slow) allow-discrete;
  }
  .drawer:not([open]) { translate: 100% 0; }
  @starting-style { .drawer[open] { translate: 100% 0; } }
  .drawer::backdrop { background: var(--scrim); }
  .sm { --w: 24rem; }
  .lg { --w: 46rem; }
  .inline {
    position: static;
    width: min(var(--w), 100%);
    height: 28rem;
    border: var(--border-w) solid var(--line);
    border-radius: var(--radius-lg);
    box-shadow: none;
  }

  .sheet { display: flex; flex-direction: column; height: 100%; }
  header {
    display: flex;
    align-items: flex-start;
    gap: var(--space-2);
    padding: var(--space-3) var(--space-3) var(--space-3) var(--space-5);
    border-bottom: var(--border-w) solid var(--line);
  }
  .heading { display: grid; gap: var(--space-0-5); flex: 1; min-width: 0; padding-top: var(--space-1-5); }
  h2 { font-size: var(--text-lg); overflow-wrap: anywhere; }
  .sub { font-size: var(--text-sm); color: var(--ink-muted); }
  .tools { display: flex; align-items: center; gap: var(--space-1); }
  .body { flex: 1; min-height: 0; overflow: auto; padding: var(--space-4) var(--space-5) var(--space-6); overscroll-behavior: contain; }
  footer {
    display: flex;
    flex-wrap: wrap;
    justify-content: flex-end;
    gap: var(--space-2);
    padding: var(--space-3) var(--space-5);
    background: var(--surface-sunken);
    border-top: var(--border-w) solid var(--line);
  }

  @media (width < 48rem) {
    .drawer:not(.inline) { width: 100vw; border: 0; }
    header { padding-left: var(--space-4); }
    .body { padding-inline: var(--space-4); }
    footer { padding-inline: var(--space-4); }
    footer > :global(*) { flex: 1; }
  }
</style>
