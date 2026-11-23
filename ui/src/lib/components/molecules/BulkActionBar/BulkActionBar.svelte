<!--
  BulkActionBar: appears when rows are selected. Shows the count, offers "Select all
  loaded" (cursor pages: only what is loaded, said plainly), Clear, and the actions for the
  selection (`actions` snippet: Buttons, the first may be primary). A toolbar; the count is a
  polite status so selection changes are heard. Sticks to the bottom of the viewport on phones.
-->
<script lang="ts">
  import type { Snippet } from 'svelte';
  import Button from '../../atoms/Button/Button.svelte';
  import { formatCount } from '../../../format/quantity';

  interface Props {
    selected: number;
    /** Rows loaded so far (cursor pages). */
    loaded: number;
    /** All rows in the list, when the server knows. */
    total?: number;
    /** Noun for one row: "series", "download". */
    noun?: string;
    nounPlural?: string;
    onselectall?: () => void;
    onclear?: () => void;
    actions?: Snippet;
  }
  let { selected, loaded, total, noun = 'item', nounPlural, onselectall, onclear, actions }: Props = $props();

  const word = (n: number) => (n === 1 ? noun : (nounPlural ?? `${noun}s`));
</script>

{#if selected > 0}
  <div class="bar" role="toolbar" aria-label="Actions for selected {nounPlural ?? `${noun}s`}">
    <span class="count num" role="status">{formatCount(selected)} {word(selected)} selected</span>
    {#if onselectall && selected < loaded}
      <Button size="sm" variant="ghost" onclick={onselectall}>Select all {formatCount(loaded)} loaded</Button>
    {:else if total !== undefined && loaded < total}
      <span class="hint num">{formatCount(loaded, total)} loaded</span>
    {/if}
    {#if onclear}<Button size="sm" variant="ghost" onclick={onclear}>Clear</Button>{/if}
    {#if actions}<div class="actions">{@render actions()}</div>{/if}
  </div>
{/if}

<style>
  .bar {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-2);
    padding: var(--space-1-5) var(--space-2) var(--space-1-5) var(--space-3);
    background: var(--surface-selected);
    border: var(--border-w) solid var(--accent);
    border-radius: var(--radius-lg);
  }
  .count { font-size: var(--text-md); font-weight: var(--weight-semibold); color: var(--ink); }
  .hint { font-size: var(--text-sm); color: var(--ink-muted); }
  .actions { display: flex; flex-wrap: wrap; gap: var(--space-2); margin-left: auto; }
  @media (width < 48rem) {
    .bar { position: sticky; bottom: var(--space-2); z-index: var(--z-sticky); box-shadow: var(--shadow-sm); }
    .actions { margin-left: 0; width: 100%; }
  }
</style>
