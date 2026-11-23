<!--
  Countdown: time left until something happens (rule action, next retry, next run,
  airing). Ticks each second inside the last hour; under reduced motion it never ticks by the
  second and shows whole minutes. Calls `onexpire` once at zero and then shows `expiredLabel`.
  Not a live region: the accessible text is stable ("in 4m"), so screen readers are not spammed.
-->
<script lang="ts">
  import Icon from '../Icon/Icon.svelte';
  import { formatClock, formatDuration } from '../../../format/quantity';
  import { formatAbsolute, toDate } from '../../../format/time';
  import { secondClock, minuteClock, prefersReducedMotion } from '../../../clock.svelte';
  import type { Tone } from '../../../tokens';

  interface Props {
    to: Date | string | number;
    /** What happens at zero, read before the time: "Removes", "Retries", "Airs". */
    action?: string;
    expiredLabel?: string;
    tone?: Tone;
    onexpire?: () => void;
    /** Fixed "now" (ms) for stories and tests. */
    now?: number;
  }
  let { to, action, expiredLabel = 'now', tone = 'neutral', onexpire, now }: Props = $props();

  const reduced = prefersReducedMotion();
  const target = $derived(toDate(to).getTime());
  const current = $derived(now ?? (reduced ? minuteClock.now : secondClock.now));
  const left = $derived(Math.max(0, (target - current) / 1000));
  const expired = $derived(left <= 0);
  const text = $derived(
    expired ? expiredLabel : !reduced && left < 3600 ? formatClock(left) : `${formatDuration(reduced ? Math.ceil(left / 60) * 60 : left)}`,
  );
  const spoken = $derived(expired ? expiredLabel : `in ${formatDuration(Math.ceil(left / 60) * 60)}`);

  let fired = false;
  $effect(() => {
    if (expired && !fired) {
      fired = true;
      onexpire?.();
    }
  });
</script>

<span class={['cd', tone]} class:expired title={formatAbsolute(new Date(target))}>
  <Icon name="timer" size="sm" />
  <span aria-hidden="true">
    {#if action}<span class="action">{action}</span>{/if}
    <time datetime={new Date(target).toISOString()}>{expired ? text : `in ${text}`}</time>
  </span>
  <span class="visually-hidden">{action ? `${action} ` : ''}{spoken}</span>
</span>

<style>
  .cd { display: inline-flex; align-items: center; gap: var(--space-1); font-size: var(--text-sm); color: var(--tone-neutral-ink); white-space: nowrap; }
  .cd time { font-variant-numeric: tabular-nums; font-weight: var(--weight-medium); }
  .warn { color: var(--tone-warn-ink); }
  .error { color: var(--tone-error-ink); }
  .info { color: var(--tone-info-ink); }
  .ok { color: var(--tone-ok-ink); }
  .managed { color: var(--tone-managed-ink); }
</style>
