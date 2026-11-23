<!--
  CheckResults: the outcome of a connection test, a media-server self-test or plugin
  conformance: one row per check with its status, how long it took and what went wrong, plus
  a counted summary and Rerun. Failures and warnings sort first so the problem is on top.
  The summary is a polite live region: it changes only when a run finishes.
-->
<script lang="ts" module>
  export interface CheckResult {
    id: string;
    name: string;
    status: 'pass' | 'warn' | 'fail' | 'running' | 'pending' | 'skipped';
    detail?: string;
    /** Milliseconds the check took. */
    ms?: number;
    /** Where to fix it. */
    href?: string;
    hrefLabel?: string;
  }
</script>

<script lang="ts">
  import Button from '../../atoms/Button/Button.svelte';
  import Link from '../../atoms/Link/Link.svelte';
  import StatusBadge from '../../atoms/StatusBadge/StatusBadge.svelte';
  import Timestamp from '../../atoms/Timestamp/Timestamp.svelte';
  import { formatDuration } from '../../../format/quantity';

  interface Props {
    checks: CheckResult[];
    title?: string;
    onrerun?: () => void;
    /** When the last run finished. */
    ranAt?: Date | string | number;
    now?: Date;
    /** Keep the given order instead of problems first. */
    keepOrder?: boolean;
  }
  let { checks, title = 'Checks', onrerun, ranAt, now, keepOrder = false }: Props = $props();

  const rank: Record<CheckResult['status'], number> = { fail: 0, warn: 1, running: 2, pending: 3, skipped: 4, pass: 5 };
  const sorted = $derived(keepOrder ? checks : [...checks].sort((a, b) => rank[a.status] - rank[b.status]));
  const running = $derived(checks.some((c) => c.status === 'running'));
  const n = (s: CheckResult['status']) => checks.filter((c) => c.status === s).length;
  const summary = $derived.by(() => {
    if (running) return `Checking: ${checks.length - n('running') - n('pending')} of ${checks.length} done`;
    const ran = checks.length - n('pending') - n('skipped');
    if (ran === 0) return 'Not run yet';
    const parts = [`${n('pass')} of ${ran} passed`];
    if (n('fail')) parts.push(`${n('fail')} failed`);
    if (n('warn')) parts.push(`${n('warn')} with warnings`);
    return parts.join(', ');
  });
  const overall = $derived(running ? 'running' : n('fail') ? 'fail' : n('warn') ? 'warn' : checks.length && n('pass') ? 'pass' : 'pending');
  const ms = (v: number) => (v < 1000 ? `${Math.round(v)} ms` : v < 60000 ? `${(v / 1000).toFixed(1)} s` : formatDuration(v / 1000));
</script>

<section class="cr" aria-label={title}>
  <header class="head">
    <StatusBadge family="check" value={overall} />
    <span class="summary" role="status">{summary}</span>
    {#if ranAt && !running}<span class="when">· <Timestamp value={ranAt} {now} /></span>{/if}
    {#if onrerun}<Button size="sm" icon="refresh" loading={running} onclick={onrerun} class="rerun">{running ? 'Checking' : 'Rerun'}</Button>{/if}
  </header>
  <ul>
    {#each sorted as c (c.id)}
      <li class={c.status}>
        <span class="badge"><StatusBadge family="check" value={c.status} size="sm" /></span>
        <span class="name">{c.name}</span>
        {#if c.ms !== undefined}<span class="ms num">{ms(c.ms)}</span>{/if}
        {#if c.detail || c.href}
          <span class="detail">
            {#if c.detail}{c.detail}{/if}
            {#if c.href}<Link href={c.href}>{c.hrefLabel ?? 'Fix this'}</Link>{/if}
          </span>
        {/if}
      </li>
    {/each}
  </ul>
</section>

<style>
  .cr { display: grid; gap: var(--space-2); min-width: 0; }
  .head { display: flex; flex-wrap: wrap; align-items: center; gap: var(--space-2); }
  .summary { font-size: var(--text-md); font-weight: var(--weight-medium); }
  .when { font-size: var(--text-sm); color: var(--ink-muted); }
  .head :global(.rerun) { margin-left: auto; }
  ul { display: grid; padding: 0; list-style: none; background: var(--surface-raised); border: var(--border-w) solid var(--line); border-radius: var(--radius-lg); }
  li {
    display: grid;
    grid-template-columns: 7.5rem minmax(0, 1fr) auto;
    align-items: center;
    gap: var(--space-0-5) var(--space-3);
    min-height: var(--row-h-comfortable);
    padding: var(--space-2) var(--space-3);
    border-bottom: var(--border-w) solid var(--line);
    font-size: var(--text-md);
  }
  li:last-child { border-bottom: 0; }
  .name { min-width: 0; }
  .ms { font-size: var(--text-xs); color: var(--ink-subtle); text-align: right; }
  .detail { grid-column: 2 / -1; display: flex; flex-wrap: wrap; gap: var(--space-1) var(--space-2); font-size: var(--text-sm); color: var(--ink-muted); }
  .fail .detail { color: var(--tone-error-ink); }
  .warn .detail { color: var(--tone-warn-ink); }
  @media (width < 48rem) {
    li { grid-template-columns: minmax(0, 1fr) auto; }
    .badge { grid-column: 1 / -1; }
    .detail { grid-column: 1 / -1; }
  }
</style>
