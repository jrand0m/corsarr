<!--
  StatusBadge: label + icon + tone, never colour alone. Prefer `family` + `value` from
  the shared vocabulary (status/vocabulary.ts); pass `label`/`icon`/`tone` only for a value the
  vocabulary lacks (and add it there). `detail` appends a qualifier such as a failure class.
  `appearance="plain"` drops the fill for dense table cells.
-->
<script lang="ts">
  import Icon from '../Icon/Icon.svelte';
  import type { IconName } from '../Icon/icons';
  import { lookupStatus, type StatusFamily } from '../../../status/vocabulary';
  import type { Tone } from '../../../tokens';

  interface Props {
    family?: StatusFamily;
    value?: string;
    label?: string;
    icon?: IconName;
    tone?: Tone;
    detail?: string;
    size?: 'sm' | 'md';
    appearance?: 'tint' | 'plain';
  }
  let { family, value, label, icon, tone, detail, size = 'md', appearance = 'tint' }: Props = $props();

  const entry = $derived(family && value ? lookupStatus(family, value) : undefined);
  const t = $derived(tone ?? entry?.tone ?? 'neutral');
  const text = $derived(label ?? entry?.label ?? value ?? '');
  const glyph = $derived(icon ?? entry?.icon ?? 'info');
</script>

<span class={['badge', size, appearance, t]} class:active={entry?.active}>
  <Icon name={glyph} size="sm" />
  <span class="text">{text}{#if detail}<span class="detail">{` · ${detail}`}</span>{/if}</span>
</span>

<style>
  .badge {
    --ink: var(--tone-neutral-ink);
    --tint: var(--tone-neutral-tint);
    --line: var(--tone-neutral-line);
    display: inline-flex;
    align-items: center;
    gap: var(--space-1);
    max-width: 100%;
    height: 1.5rem;
    padding: 0 var(--space-2) 0 var(--space-1-5);
    font-size: var(--text-sm);
    font-weight: var(--weight-medium);
    line-height: 1;
    white-space: nowrap;
    color: var(--ink);
    background: var(--tint);
    border: var(--border-w) solid var(--line);
    border-radius: var(--radius-sm);
    vertical-align: middle;
  }
  .sm { height: 1.25rem; padding: 0 var(--space-1-5) 0 var(--space-1); font-size: var(--text-xs); }
  .plain { background: none; border-color: transparent; padding-inline: 0; height: auto; }
  /* Room for descenders: overflow clips at the line box, so it must be taller than 1. */
  .text { overflow: hidden; text-overflow: ellipsis; line-height: var(--leading-snug); }
  .detail { font-weight: var(--weight-regular); }
  .ok { --ink: var(--tone-ok-ink); --tint: var(--tone-ok-tint); --line: var(--tone-ok-line); }
  .warn { --ink: var(--tone-warn-ink); --tint: var(--tone-warn-tint); --line: var(--tone-warn-line); }
  .error { --ink: var(--tone-error-ink); --tint: var(--tone-error-tint); --line: var(--tone-error-line); }
  .info { --ink: var(--tone-info-ink); --tint: var(--tone-info-tint); --line: var(--tone-info-line); }
  .managed { --ink: var(--tone-managed-ink); --tint: var(--tone-managed-tint); --line: var(--tone-managed-line); }
  .active :global(svg) { animation: breathe 1.6s var(--ease-in-out) infinite; }
  @keyframes breathe { 50% { opacity: 0.35; } }
</style>
