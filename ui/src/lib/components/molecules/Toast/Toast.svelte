<!--
  Toast: the outcome of an action the owner just took ("Removed The Long Haul S02E03
  and blocklisted the release"), with Undo when it can be undone. Stays at least 10 s
 , pauses while hovered or focused, and is announced: `role=status` for outcomes,
  `role=alert` for failures. Stacking, placement and the region belong to ToastRegion.
  `duration={0}` keeps it until dismissed (stories, errors that need reading).
-->
<script lang="ts">
  import Icon from '../../atoms/Icon/Icon.svelte';
  import IconButton from '../../atoms/IconButton/IconButton.svelte';
  import Button from '../../atoms/Button/Button.svelte';
  import type { IconName } from '../../atoms/Icon/icons';

  interface Props {
    message: string;
    detail?: string;
    tone?: 'ok' | 'info' | 'warn' | 'error';
    /** Undo handler; adds the Undo button. */
    onundo?: () => void;
    undoLabel?: string;
    /** Another follow-up action ("Open import"). */
    actionLabel?: string;
    onaction?: () => void;
    ondismiss?: () => void;
    /** Milliseconds before it dismisses itself; at least 10 s with undo; 0 keeps it. */
    duration?: number;
    /** Carry its own live role. ToastRegion turns this off and announces through its
        persistent live regions, which screen readers pick up more reliably. */
    live?: boolean;
  }
  let {
    message, detail, tone = 'ok', onundo, undoLabel = 'Undo', actionLabel, onaction, ondismiss, duration = 10_000, live = true,
  }: Props = $props();

  const icons: Record<NonNullable<Props['tone']>, IconName> = { ok: 'check-circle', info: 'info', warn: 'alert-triangle', error: 'alert-octagon' };
  const ms = $derived(duration > 0 && onundo ? Math.max(duration, 10_000) : duration);

  let paused = $state(false);
  let left = 0;
  let started = 0;
  let timer: ReturnType<typeof setTimeout> | undefined;

  function run() {
    clearTimeout(timer);
    started = Date.now();
    timer = setTimeout(() => ondismiss?.(), left);
  }
  $effect(() => {
    if (ms <= 0) return;
    left = ms;
    run();
    return () => clearTimeout(timer);
  });
  function pause() {
    if (ms <= 0 || paused) return;
    paused = true;
    clearTimeout(timer);
    left = Math.max(0, left - (Date.now() - started));
  }
  function resume(e?: FocusEvent) {
    if (ms <= 0 || !paused) return;
    if (e && (e.currentTarget as HTMLElement).contains(e.relatedTarget as Node)) return;
    paused = false;
    run();
  }
  function undo() {
    clearTimeout(timer);
    onundo?.();
    ondismiss?.();
  }
</script>

<div
  class={['toast', tone]} role={live ? (tone === 'error' ? 'alert' : 'status') : undefined}
  onpointerenter={pause} onpointerleave={() => resume()} onfocusin={pause} onfocusout={resume}
>
  <Icon name={icons[tone]} size="md" />
  <div class="text">
    <span class="msg">{message}</span>
    {#if detail}<span class="detail">{detail}</span>{/if}
  </div>
  <div class="actions">
    {#if onundo}<Button size="sm" variant="ghost" icon="undo" onclick={undo}>{undoLabel}</Button>{/if}
    {#if actionLabel && onaction}<Button size="sm" variant="ghost" onclick={onaction}>{actionLabel}</Button>{/if}
    {#if ondismiss}<IconButton icon="x" label="Dismiss" size="sm" onclick={() => { clearTimeout(timer); ondismiss?.(); }} />{/if}
  </div>
</div>

<style>
  .toast {
    --mark: var(--tone-ok-ink);
    display: flex;
    align-items: flex-start;
    gap: var(--space-2) var(--space-3);
    width: min(100%, 28rem);
    padding: var(--space-2) var(--space-2) var(--space-2) var(--space-3);
    color: var(--ink);
    background: var(--surface-overlay);
    border: var(--border-w) solid var(--line);
    border-radius: var(--radius-lg);
    box-shadow: var(--shadow-md);
  }
  .toast > :global(svg) { margin-top: 0.4375rem; color: var(--mark); }
  .info { --mark: var(--tone-info-ink); }
  .warn { --mark: var(--tone-warn-ink); }
  .error { --mark: var(--tone-error-ink); border-color: var(--tone-error-line); }
  .text { display: grid; gap: var(--space-0-5); flex: 1; min-width: 0; padding-block: var(--space-1); font-size: var(--text-md); line-height: var(--leading-snug); }
  .msg { font-weight: var(--weight-medium); }
  .detail { font-size: var(--text-sm); color: var(--ink-muted); }
  .actions { display: flex; align-items: center; gap: var(--space-0-5); flex: none; }
  @media (width < 48rem) {
    .toast { flex-wrap: wrap; width: 100%; }
    .actions { margin-left: auto; }
  }
</style>
