<!--
  Tooltip: a short, non-interactive hint on hover (after a delay) and on keyboard focus;
  Escape dismisses it. The trigger is rendered by the `children` snippet, which receives the
  props to spread onto the focusable element:

    <Tooltip text="Absolute time">
      {#snippet children(trigger)}<button {...trigger}>…</button>{/snippet}
    </Tooltip>

  `describe` (default true) links the hint as aria-describedby; set it false when the text
  repeats the trigger's accessible name (IconButton). Positioned `fixed`, so it escapes
  scrolling and clipping containers.
-->
<script lang="ts" module>
  export interface TooltipTrigger {
    'aria-describedby'?: string;
    onpointerenter: (e: PointerEvent) => void;
    onpointerleave: () => void;
    onfocus: (e: FocusEvent) => void;
    onblur: () => void;
    onkeydown: (e: KeyboardEvent) => void;
  }
</script>

<script lang="ts">
  import type { Snippet } from 'svelte';

  interface Props {
    text: string;
    children: Snippet<[TooltipTrigger]>;
    describe?: boolean;
    placement?: 'top' | 'bottom';
    delay?: number;
  }
  let { text, children, describe = true, placement = 'top', delay = 400 }: Props = $props();

  const id = $props.id();
  let open = $state(false);
  let anchor = $state<DOMRect | null>(null);
  let bubble = $state<HTMLElement>();
  let pos = $state({ x: 0, y: 0, below: false });
  let timer: ReturnType<typeof setTimeout> | undefined;

  function show(el: Element, wait: number) {
    clearTimeout(timer);
    timer = setTimeout(() => {
      anchor = el.getBoundingClientRect();
      open = true;
    }, wait);
  }
  function hide() {
    clearTimeout(timer);
    open = false;
  }

  $effect(() => {
    if (!open || !anchor || !bubble) return;
    const b = bubble.getBoundingClientRect();
    const gap = 6, margin = 8;
    const below = placement === 'bottom' || anchor.top - b.height - gap < margin;
    const x = Math.min(Math.max(anchor.left + anchor.width / 2 - b.width / 2, margin), window.innerWidth - b.width - margin);
    const y = below ? anchor.bottom + gap : anchor.top - b.height - gap;
    pos = { x, y, below };
  });

  $effect(() => () => clearTimeout(timer));

  const trigger: TooltipTrigger = $derived({
    ...(describe ? { 'aria-describedby': id } : {}),
    onpointerenter: (e) => { if (e.pointerType !== 'touch') show(e.currentTarget as Element, delay); },
    onpointerleave: hide,
    onfocus: (e) => show(e.currentTarget as Element, 0),
    onblur: hide,
    onkeydown: (e) => { if (e.key === 'Escape' && open) { e.stopPropagation(); hide(); } },
  });
</script>

{@render children(trigger)}
<span
  bind:this={bubble}
  {id}
  role="tooltip"
  class="tooltip"
  class:open
  aria-hidden={describe ? undefined : 'true'}
  style:left="{pos.x}px"
  style:top="{pos.y}px"
>{text}</span>

<style>
  .tooltip {
    position: fixed;
    z-index: var(--z-tooltip);
    max-width: min(20rem, calc(100vw - 1rem));
    padding: var(--space-1) var(--space-2);
    font-size: var(--text-xs);
    font-weight: var(--weight-medium);
    line-height: var(--leading-snug);
    color: var(--ink-inverse);
    background: var(--surface-inverse);
    border-radius: var(--radius-sm);
    box-shadow: var(--shadow-md);
    pointer-events: none;
    visibility: hidden;
    opacity: 0;
    transition: opacity var(--dur-fast) var(--ease-out), visibility 0s linear var(--dur-fast);
  }
  .open { visibility: visible; opacity: 1; transition: opacity var(--dur-fast) var(--ease-out); }
</style>
