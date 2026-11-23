<!--
  BayGrid: corsarr's signature mark, a stowage-plan grid with one cell per unit (episodes in
  a season, tracks on an album, files in a download). Each state has its own fill pattern as
  well as its tone, so the grid reads without colour. Static grids are one image with a
  counted summary; pass `onselect` to make cells keyboard-navigable buttons (roving focus,
  arrow keys, Home/End).
-->
<script lang="ts" module>
  export type BayState = 'have' | 'upgrade' | 'downloading' | 'queued' | 'missing' | 'blocked' | 'unaired' | 'unmonitored';
  export interface BayCell { state: BayState; label: string }

  export const bayStates: Record<BayState, string> = {
    have: 'On disk',
    upgrade: 'Below cutoff',
    downloading: 'Downloading',
    queued: 'Queued',
    missing: 'Missing',
    blocked: 'Blocked',
    unaired: 'Not available yet',
    unmonitored: 'Unmonitored',
  };

  export function summarise(cells: BayCell[]): string {
    const counts = new Map<BayState, number>();
    for (const c of cells) counts.set(c.state, (counts.get(c.state) ?? 0) + 1);
    return (Object.keys(bayStates) as BayState[])
      .filter((s) => counts.get(s))
      .map((s) => `${counts.get(s)} ${bayStates[s].toLowerCase()}`)
      .join(', ');
  }
</script>

<script lang="ts">
  interface Props {
    cells: BayCell[];
    /** Group name used in the accessible summary: "Season 2". */
    label: string;
    size?: 'sm' | 'md';
    legend?: boolean;
    onselect?: (index: number, cell: BayCell) => void;
  }
  let { cells, label, size = 'md', legend = false, onselect }: Props = $props();

  const summary = $derived(`${label}: ${summarise(cells)}`);
  const present = $derived((Object.keys(bayStates) as BayState[]).filter((s) => cells.some((c) => c.state === s)));
  let active = $state(0);
  let grid = $state<HTMLElement>();

  function onkeydown(e: KeyboardEvent) {
    const cols = Math.max(1, Math.round((grid?.clientWidth ?? 0) / ((grid?.firstElementChild as HTMLElement)?.offsetWidth + 3 || 1)));
    const moves: Record<string, number> = { ArrowRight: 1, ArrowLeft: -1, ArrowDown: cols, ArrowUp: -cols, Home: -Infinity, End: Infinity };
    if (!(e.key in moves)) return;
    e.preventDefault();
    const d = moves[e.key];
    active = Math.min(cells.length - 1, Math.max(0, d === Infinity ? cells.length - 1 : d === -Infinity ? 0 : active + d));
    (grid?.children[active] as HTMLElement | undefined)?.focus();
  }
</script>

<div class={['bay', size]}>
  {#if onselect}
    <div class="grid" role="group" aria-label={summary} bind:this={grid}>
      {#each cells as cell, i (i)}
        <button
          type="button"
          class={['cell', cell.state]}
          tabindex={i === active ? 0 : -1}
          aria-label="{cell.label}, {bayStates[cell.state]}"
          {onkeydown}
          onclick={() => { active = i; onselect(i, cell); }}
        ></button>
      {/each}
    </div>
  {:else}
    <div class="grid" role="img" aria-label={summary}>
      {#each cells as cell, i (i)}<span class={['cell', cell.state]} title="{cell.label} · {bayStates[cell.state]}"></span>{/each}
    </div>
  {/if}
  {#if legend}
    <ul class="legend" aria-hidden="true">
      {#each present as s (s)}<li><span class={['cell', s]}></span>{bayStates[s]}</li>{/each}
    </ul>
  {/if}
</div>

<style>
  .bay { --cell: 0.875rem; display: grid; gap: var(--space-2); }
  .sm { --cell: 0.625rem; }
  .grid { display: flex; flex-wrap: wrap; gap: 3px; }
  .cell {
    --solid: var(--tone-neutral-solid);
    display: block;
    width: var(--cell);
    height: var(--cell);
    padding: 0;
    background: none;
    border: 1.5px solid var(--solid);
    border-radius: var(--radius-xs);
    flex: none;
  }
  button.cell { cursor: pointer; }
  button.cell:hover { outline: 1.5px solid var(--ink); outline-offset: 1px; }
  /* State = tone + pattern. */
  .have { --solid: var(--tone-ok-solid); background: var(--solid); }
  .upgrade { --solid: var(--tone-warn-solid); background: linear-gradient(to top, var(--solid) 55%, transparent 55%); }
  .downloading { --solid: var(--tone-info-solid); background: repeating-linear-gradient(-45deg, var(--solid) 0 2px, transparent 2px 4px); }
  .queued { --solid: var(--tone-info-solid); background: radial-gradient(circle, var(--solid) 0 28%, transparent 30%); }
  .missing { --solid: var(--tone-error-solid); }
  .blocked { --solid: var(--tone-error-solid); background: linear-gradient(45deg, transparent 42%, var(--solid) 42% 58%, transparent 58%), linear-gradient(-45deg, transparent 42%, var(--solid) 42% 58%, transparent 58%); }
  .unaired { --solid: var(--tone-neutral-solid); border-style: dashed; }
  .unmonitored { --solid: var(--line-strong); background: var(--surface-pressed); border-color: var(--line); }
  .legend { display: flex; flex-wrap: wrap; gap: var(--space-1) var(--space-3); padding: 0; list-style: none; font-size: var(--text-xs); color: var(--ink-muted); }
  .legend li { display: inline-flex; align-items: center; gap: var(--space-1); }
  .legend .cell { --cell: 0.625rem; }
  @media (width < 48rem), (pointer: coarse) {
    .grid:has(button) { gap: 4px; }
    .grid:has(button) .cell { --cell: var(--target-min); }
  }
</style>
