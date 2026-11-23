<!--
  Timestamp: relative time ("5m ago", "in 3h") with the absolute time on hover and
  keyboard focus; or the absolute time alone. Updates from the shared minute clock. In dense
  tables pass `focusable={false}` (the row owns focus and its detail shows absolute times).
-->
<script lang="ts">
  import Tooltip from '../Tooltip/Tooltip.svelte';
  import { formatAbsolute, formatRelative, toDate } from '../../../format/time';
  import { minuteClock } from '../../../clock.svelte';

  interface Props {
    value: Date | string | number;
    mode?: 'relative' | 'absolute';
    focusable?: boolean;
    /** Fixed "now" for stories and tests. */
    now?: Date;
  }
  let { value, mode = 'relative', focusable = true, now }: Props = $props();

  const date = $derived(toDate(value));
  const absolute = $derived(formatAbsolute(date));
  const text = $derived(mode === 'absolute' ? absolute : formatRelative(date, now ?? new Date(minuteClock.now)));
</script>

{#if mode === 'absolute'}
  <time class="ts" datetime={date.toISOString()}>{text}</time>
{:else}
  <Tooltip text={absolute}>
    {#snippet children(trigger)}
      <!-- Focusable so keyboard users can reveal the absolute time (WCAG 1.4.13); it has no action. -->
      <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
      <time class="ts" datetime={date.toISOString()} tabindex={focusable ? 0 : undefined} {...trigger}>{text}</time>
    {/snippet}
  </Tooltip>
{/if}

<style>
  .ts { white-space: nowrap; font-variant-numeric: tabular-nums; border-radius: var(--radius-xs); }
  [tabindex] { text-decoration: underline dotted var(--line-strong); text-underline-offset: 0.2em; cursor: default; }
</style>
