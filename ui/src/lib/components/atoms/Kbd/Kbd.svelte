<!--
  Kbd: a keyboard shortcut. `keys` is one chord; "mod" renders as ⌘ on Apple platforms and
  Ctrl elsewhere. Pass several chords as a sequence with `then` (e.g. g then d).
-->
<script lang="ts">
  let { keys, then: next }: { keys: string[]; then?: string[] } = $props();

  const apple = typeof navigator !== 'undefined' && /Mac|iPhone|iPad/.test(navigator.platform || navigator.userAgent);
  const names: Record<string, [string, string]> = {
    mod: apple ? ['⌘', 'Command'] : ['Ctrl', 'Control'],
    shift: ['Shift', 'Shift'], alt: apple ? ['⌥', 'Option'] : ['Alt', 'Alt'],
    enter: ['Enter', 'Enter'], esc: ['Esc', 'Escape'], up: ['↑', 'Up'], down: ['↓', 'Down'],
    left: ['←', 'Left'], right: ['→', 'Right'], space: ['Space', 'Space'], delete: ['Del', 'Delete'],
  };
  const show = (k: string) => names[k.toLowerCase()]?.[0] ?? (k.length === 1 ? k.toUpperCase() : k);
  const say = (k: string) => names[k.toLowerCase()]?.[1] ?? k;
  const spoken = $derived([keys, next].filter(Boolean).map((c) => c!.map(say).join(' plus ')).join(' then '));
</script>

<span class="kbd" aria-label={spoken} role="img">
  {#each [keys, next].filter((c) => c && c.length) as chord, ci (ci)}
    {#if ci > 0}<span class="then" aria-hidden="true">then</span>{/if}
    {#each chord! as k, i (i)}<kbd aria-hidden="true">{show(k)}</kbd>{/each}
  {/each}
</span>

<style>
  .kbd { display: inline-flex; align-items: center; gap: var(--space-0-5); vertical-align: middle; }
  kbd {
    display: inline-grid;
    place-items: center;
    min-width: 1.4em;
    height: 1.4rem;
    padding: 0 var(--space-1);
    font-family: var(--font-sans);
    font-size: var(--text-2xs);
    font-weight: var(--weight-medium);
    line-height: 1;
    color: var(--ink-muted);
    background: var(--surface-raised);
    border: var(--border-w) solid var(--line);
    border-bottom-width: 2px;
    border-radius: var(--radius-xs);
  }
  .then { font-size: var(--text-2xs); color: var(--ink-subtle); padding: 0 var(--space-0-5); }
</style>
