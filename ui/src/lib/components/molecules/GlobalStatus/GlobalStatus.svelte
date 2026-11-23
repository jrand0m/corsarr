<!--
  GlobalStatus: the shell's always-visible indicators. Health dot with the count of
  integrations not OK (→ System › Health), queue counter active / blocked (→ Queue),
  live-connection state of the event stream, and a shadow marker when any kind runs in shadow
  mode. Drawn for the oxide shell (shell ink); `orientation` follows the rail or top bar.
  The connection state is announced politely when it changes; counts are not.
-->
<script lang="ts">
  import StatusDot from '../../atoms/StatusDot/StatusDot.svelte';
  import Icon from '../../atoms/Icon/Icon.svelte';
  import { formatCount } from '../../../format/quantity';
  import { vocabulary } from '../../../status/vocabulary';

  interface Props {
    /** Integrations not OK; 0 means healthy. */
    unhealthy: number;
    /** Worst health among them. */
    healthTone?: 'warn' | 'error';
    healthHref?: string;
    active: number;
    blocked: number;
    queueHref?: string;
    connection: 'live' | 'reconnecting' | 'offline';
    /** Kinds in shadow mode, by name: ["Music"]. */
    shadowKinds?: string[];
    shadowHref?: string;
    orientation?: 'horizontal' | 'vertical';
    /** Hide visible labels (narrow top bar); names stay accessible. */
    compact?: boolean;
  }
  let {
    unhealthy, healthTone = 'warn', healthHref = '#/system/health', active, blocked, queueHref = '#/activity/queue',
    connection, shadowKinds = [], shadowHref = '#/settings', orientation = 'horizontal', compact = false,
  }: Props = $props();

  const healthLabel = $derived(unhealthy === 0 ? 'All integrations OK' : `${formatCount(unhealthy)} ${unhealthy === 1 ? 'integration needs' : 'integrations need'} attention`);
  const queueLabel = $derived(`${formatCount(active)} downloading${blocked ? `, ${formatCount(blocked)} blocked` : ''}`);
  const conn = $derived(vocabulary.connection[connection]);
  const connLabel = $derived(connection === 'live' ? 'Live updates on' : connection === 'reconnecting' ? 'Reconnecting to live updates' : 'Live updates disconnected');
</script>

<ul class={['gs', orientation]} class:compact aria-label="Status">
  <li>
    <a href={healthHref} aria-label={healthLabel}>
      <StatusDot tone={unhealthy === 0 ? 'ok' : healthTone} label={healthLabel} />
      {#if !compact}<span>{unhealthy === 0 ? 'Healthy' : `${formatCount(unhealthy)} need attention`}</span>{:else if unhealthy}<span class="num">{unhealthy}</span>{/if}
    </a>
  </li>
  <li>
    <a href={queueHref} aria-label={queueLabel}>
      <Icon name="arrow-down" size="sm" />
      <span class="num">{formatCount(active)}</span>
      {#if blocked}<span class="blocked num"><Icon name="ban" size="sm" />{formatCount(blocked)}</span>{/if}
    </a>
  </li>
  {#if shadowKinds.length}
    <li>
      <a href={shadowHref} class="shadow" aria-label="Shadow mode: {shadowKinds.join(', ')}">
        <Icon name="shadow" size="sm" />{#if !compact}<span>Shadow: {shadowKinds.join(', ')}</span>{/if}
      </a>
    </li>
  {/if}
  <li class="conn">
    <span class="item">
      <StatusDot tone={conn.tone} label={connLabel} pulse={connection === 'reconnecting'} />
      {#if !compact}<span>{conn.label}</span>{/if}
    </span>
    <span class="visually-hidden" role="status">{connection === 'live' ? '' : connLabel}</span>
  </li>
</ul>

<style>
  .gs { display: flex; align-items: center; gap: var(--space-1); margin: 0; padding: 0; list-style: none; color: var(--shell-ink); font-size: var(--text-sm); }
  .vertical { flex-direction: column; align-items: stretch; }
  a, .item {
    display: inline-flex;
    align-items: center;
    gap: var(--space-1-5);
    min-height: var(--control-h-sm);
    padding: 0 var(--space-2);
    color: inherit;
    text-decoration: none;
    border-radius: var(--radius-md);
    white-space: nowrap;
  }
  .vertical a, .vertical .item { width: 100%; }
  a:hover { background: var(--shell-hover); }
  .blocked { display: inline-flex; align-items: center; gap: var(--space-0-5); color: var(--shell-ink); font-weight: var(--weight-semibold); }
  .gs :global(svg) { color: var(--shell-ink-muted); }
  .item { color: var(--shell-ink-muted); }
  @media (width < 48rem), (pointer: coarse) {
    a, .item { min-height: var(--target-min); }
  }
</style>
