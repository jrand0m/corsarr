<!--
  Dialog: a modal for a task that needs protected focus (add an item, sign in again,
  confirm). Native `<dialog>` opened with showModal, so the page behind is inert and the
  dialog sits in the top layer above drawers and clipped tables. Tab cycles inside; focus
  goes to `[data-autofocus]` or the first field on open and returns to the opener on close.
  Escape and a click on the scrim close it unless `dismissible` is off (a step that must be
  finished or cancelled explicitly). URL addressing belongs to the route: bind `open` to it.
-->
<script lang="ts" module>
  export type DialogCloseReason = 'escape' | 'scrim' | 'button' | 'programmatic';
</script>

<script lang="ts">
  import type { Snippet } from 'svelte';
  import IconButton from '../../atoms/IconButton/IconButton.svelte';
  import { trapTab, initialFocus } from '../../../overlay/focus';

  interface Props {
    open?: boolean;
    title: string;
    /** One line under the title; also the dialog's accessible description. */
    description?: string;
    size?: 'sm' | 'md' | 'lg';
    /** Escape, scrim click and the close button close it. */
    dismissible?: boolean;
    closeLabel?: string;
    /** Danger role for confirmations: `alertdialog`. */
    role?: 'dialog' | 'alertdialog';
    children?: Snippet;
    /** Footer buttons, end-aligned; the primary action goes last. */
    actions?: Snippet;
    onclose?: (reason: DialogCloseReason) => void;
    /** Off: renders open in place, without scrim or focus capture (previews and docs). */
    modal?: boolean;
    /** Title for screen readers only and no close button (the palette; Escape still closes). */
    hideHeader?: boolean;
    class?: string;
  }
  let {
    open = $bindable(false), title, description, size = 'md', dismissible = true, closeLabel = 'Close',
    role = 'dialog', children, actions, onclose, modal = true, hideHeader = false, class: cls,
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
  function oncancel(e: Event) {
    e.preventDefault();
    if (dismissible) dismiss('escape');
  }
  function onclick(e: MouseEvent) {
    if (e.target === el && dismissible) dismiss('scrim');
  }
  function onkeydown(e: KeyboardEvent) {
    if (el && modal) trapTab(e, el);
  }
</script>

<!-- The scrim click is a pointer convenience; Escape and the close button are the keyboard paths. -->
<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_noninteractive_element_interactions -->
<dialog
  bind:this={el} class={['dlg', size, cls]} class:inline={!modal} {role}
  aria-labelledby="{id}-title" aria-describedby={description ? `${id}-desc` : undefined}
  onclose={onclosed} {oncancel} {onclick} {onkeydown}
>
  <div class="frame">
    <header class:visually-hidden={hideHeader}>
      <div class="heading">
        <h2 id="{id}-title">{title}</h2>
        {#if description}<p id="{id}-desc" class="desc">{description}</p>{/if}
      </div>
      {#if dismissible && !hideHeader}
        <span data-overlay-close><IconButton icon="x" label={closeLabel} tooltip={false} onclick={() => dismiss('button')} /></span>
      {/if}
    </header>
    {#if children}<div class="body">{@render children()}</div>{/if}
    {#if actions}<footer>{@render actions()}</footer>{/if}
  </div>
</dialog>

<style>
  .dlg {
    --w: 32rem;
    width: min(var(--w), calc(100vw - var(--space-8)));
    max-width: none;
    max-height: calc(100dvh - var(--space-12));
    /* The base reset zeroes margins; the top layer centres a modal with auto margins. */
    margin: auto;
    padding: 0;
    color: var(--ink);
    background: var(--surface-overlay);
    border: var(--border-w) solid var(--line);
    border-radius: var(--radius-lg);
    box-shadow: var(--shadow-lg);
    overflow: hidden;
    opacity: 1;
    translate: 0 0;
    transition:
      opacity var(--dur-base) var(--ease-out),
      translate var(--dur-base) var(--ease-out),
      display var(--dur-base) allow-discrete,
      overlay var(--dur-base) allow-discrete;
  }
  .dlg:not([open]) { opacity: 0; translate: 0 var(--space-2); }
  @starting-style { .dlg[open] { opacity: 0; translate: 0 var(--space-2); } }
  .dlg::backdrop { background: var(--scrim); }
  .sm { --w: 26rem; }
  .lg { --w: 46rem; }
  .inline { position: static; width: min(var(--w), 100%); margin: 0; }

  .frame { display: flex; flex-direction: column; max-height: inherit; }
  header {
    display: flex;
    align-items: flex-start;
    gap: var(--space-3);
    padding: var(--space-4) var(--space-3) var(--space-3) var(--space-5);
  }
  .heading { display: grid; gap: var(--space-1); flex: 1; min-width: 0; padding-top: var(--space-1-5); }
  h2 { font-size: var(--text-lg); overflow-wrap: anywhere; }
  .desc { font-size: var(--text-md); color: var(--ink-muted); }
  .visually-hidden + .body { padding-top: var(--space-3); }
  .body {
    flex: 1;
    min-height: 0;
    overflow: auto;
    padding: 0 var(--space-5) var(--space-5);
    font-size: var(--text-md);
    overscroll-behavior: contain;
  }
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
    .dlg { width: calc(100vw - var(--space-4)); max-height: calc(100dvh - var(--space-4)); }
    header { padding-left: var(--space-4); }
    .body { padding-inline: var(--space-4); }
    footer { padding-inline: var(--space-4); }
    footer > :global(*) { flex: 1; }
  }
</style>
