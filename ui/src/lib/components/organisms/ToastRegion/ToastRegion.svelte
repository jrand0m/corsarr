<!--
  ToastRegion (region): where outcome toasts appear. Shows up to `max` at once, oldest
  first, and holds the rest until a slot frees, so every toast with Undo gets its full 10 s on
  screen. Announces through two live regions that exist before any toast does: polite for
  outcomes, assertive for failures. Each Toast pauses while hovered or focused; Undo runs the
  entry's `onundo` and dismisses it. Bottom-end on desktop, full width at the bottom on phones
  (set `--toast-offset` to clear the phone bottom bar).
-->
<script lang="ts">
  import Toast from '../../molecules/Toast/Toast.svelte';
  import type { ToastQueue, ToastEntry } from '../../../toast/toasts.svelte';

  interface Props {
    queue: ToastQueue;
    placement?: 'bottom-end' | 'bottom-center' | 'top-end' | 'top-center';
    /** Toasts visible at once; the rest wait their turn. */
    max?: number;
    label?: string;
  }
  let { queue, placement = 'bottom-end', max = 3, label = 'Notifications' }: Props = $props();

  const visible = $derived(queue.items.slice(0, max));
  const waiting = $derived(Math.max(0, queue.items.length - max));

  // Announce each toast once, when it first becomes visible.
  let polite = $state<{ id: number; text: string }[]>([]);
  let assertive = $state<{ id: number; text: string }[]>([]);
  const announced = new Set<number>();
  const sentence = (s: string) => (/[.!?…]$/.test(s) ? s : `${s}.`);
  const spoken = (t: ToastEntry) => [t.message, t.detail, t.onundo ? 'Undo available' : undefined].filter((s): s is string => !!s).map(sentence).join(' ');

  $effect(() => {
    for (const t of visible) {
      if (announced.has(t.id)) continue;
      announced.add(t.id);
      const entry = { id: t.id, text: spoken(t) };
      if (t.tone === 'error') assertive = [...assertive.slice(-2), entry];
      else polite = [...polite.slice(-2), entry];
    }
  });
</script>

<div class={['region', placement]}>
  {#if visible.length}
    <ol aria-label={label}>
      {#each visible as t (t.id)}
        <li>
          <Toast
            message={t.message} detail={t.detail} tone={t.tone} onundo={t.onundo} undoLabel={t.undoLabel}
            actionLabel={t.actionLabel} onaction={t.onaction} duration={t.duration} live={false}
            ondismiss={() => queue.dismiss(t.id)}
          />
        </li>
      {/each}
      {#if waiting}<li class="more">{waiting} more waiting</li>{/if}
    </ol>
  {/if}
</div>
<div class="visually-hidden" role="status" aria-live="polite">{#each polite as a (a.id)}<p>{a.text}</p>{/each}</div>
<div class="visually-hidden" role="alert" aria-live="assertive">{#each assertive as a (a.id)}<p>{a.text}</p>{/each}</div>

<style>
  .region {
    position: fixed;
    z-index: var(--z-toast);
    inset-inline: var(--space-4);
    display: flex;
    pointer-events: none;
  }
  .bottom-end, .bottom-center { bottom: calc(var(--space-4) + var(--toast-offset, 0px)); }
  .top-end, .top-center { top: var(--space-4); }
  .bottom-end, .top-end { justify-content: flex-end; }
  .bottom-center, .top-center { justify-content: center; }

  ol {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    width: min(100%, 28rem);
    padding: 0;
    list-style: none;
  }
  .top-end ol, .top-center ol { flex-direction: column-reverse; }
  li {
    display: flex;
    justify-content: flex-end;
    pointer-events: auto;
    opacity: 1;
    translate: 0 0;
    transition: opacity var(--dur-base) var(--ease-out), translate var(--dur-base) var(--ease-out);
  }
  @starting-style { li { opacity: 0; translate: 0 var(--space-2); } }
  .more {
    align-self: flex-end;
    padding: var(--space-1) var(--space-2);
    font-size: var(--text-xs);
    color: var(--ink-muted);
    background: var(--surface-overlay);
    border: var(--border-w) solid var(--line);
    border-radius: var(--radius-sm);
  }

  @media (width < 48rem) {
    .region { inset-inline: var(--space-2); justify-content: center; }
    .bottom-end, .bottom-center { bottom: calc(var(--space-2) + var(--toast-offset, 0px)); }
    ol { width: 100%; }
  }
</style>
