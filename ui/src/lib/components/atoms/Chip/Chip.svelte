<!--
  Chip: a compact token. Variants:
  - tag: a label on an item (plain);
  - filter: an active filter token "Profile: HD", usually `removable`;
  - score: a custom format with its score, signed and toned (+ ok, − error, 0 neutral).
  `onremove` adds a remove button; `onclick` (without onremove) makes the chip a toggle
  button with `selected` as aria-pressed.
-->
<script lang="ts">
  import Icon from '../Icon/Icon.svelte';

  interface Props {
    label: string;
    variant?: 'tag' | 'filter' | 'score';
    /** filter: the facet name shown before the label ("Profile"). */
    facet?: string;
    /** score: the format's score. */
    score?: number;
    selected?: boolean;
    onclick?: () => void;
    onremove?: () => void;
  }
  let { label, variant = 'tag', facet, score, selected, onclick, onremove }: Props = $props();

  const sign = $derived(score === undefined ? '' : score > 0 ? 'pos' : score < 0 ? 'neg' : 'zero');
  const scoreText = $derived(score === undefined ? '' : score > 0 ? `+${score}` : score < 0 ? `−${Math.abs(score)}` : '0');
</script>

{#snippet body()}
  {#if facet}<span class="facet">{facet}:</span>{/if}
  <span class="label">{label}</span>
  {#if score !== undefined}<span class="score">{scoreText}</span>{/if}
{/snippet}

{#if onclick && !onremove}
  <button type="button" class={['chip', variant, sign]} aria-pressed={selected ?? false} {onclick}>{@render body()}</button>
{:else}
  <span class={['chip', variant, sign]}>
    {@render body()}
    {#if onremove}
      <button type="button" class="remove" aria-label="Remove {facet ? `${facet}: ` : ''}{label}" onclick={onremove}><Icon name="x" size="sm" /></button>
    {/if}
  </span>
{/if}

<style>
  .chip {
    display: inline-flex;
    align-items: center;
    gap: var(--space-1);
    max-width: 100%;
    height: 1.5rem;
    padding: 0 var(--space-2);
    font-size: var(--text-sm);
    line-height: 1;
    white-space: nowrap;
    color: var(--ink);
    background: var(--surface-raised);
    border: var(--border-w) solid var(--line-strong);
    border-radius: var(--radius-sm);
    vertical-align: middle;
  }
  .label { overflow: hidden; text-overflow: ellipsis; }
  .facet { color: var(--ink-muted); }
  .filter { background: var(--accent-tint); border-color: transparent; color: var(--accent-ink); }
  .filter .facet { color: inherit; opacity: 0.85; }
  .score { font-variant-numeric: tabular-nums; font-weight: var(--weight-semibold); }
  .pos .score { color: var(--tone-ok-ink); }
  .neg .score { color: var(--tone-error-ink); }
  .zero .score { color: var(--ink-muted); }
  button.chip { cursor: pointer; }
  button.chip:hover { background: var(--surface-hover); }
  button.chip[aria-pressed='true'] { background: var(--accent-tint); color: var(--accent-ink); border-color: var(--accent); }
  .remove {
    position: relative;
    display: grid;
    place-items: center;
    width: 1rem;
    height: 1rem;
    margin-right: calc(var(--space-1) * -1);
    padding: 0;
    color: inherit;
    background: none;
    border: 0;
    border-radius: var(--radius-xs);
  }
  .remove:hover { background: var(--surface-hover); }
  @media (width < 48rem), (pointer: coarse) {
    .remove::before { content: ''; position: absolute; inset: -0.875rem; }
    button.chip { min-height: var(--target-min); }
  }
</style>
