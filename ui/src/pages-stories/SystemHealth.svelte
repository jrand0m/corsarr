<!-- Page composition (story only): System › Health. Problems first and expanded with their
     fix hints; healthy integrations follow. All OK collapses to one calm line. -->
<script lang="ts">
  import Shell from './Shell.svelte';
  import ListPage from '../lib/components/templates/ListPage/ListPage.svelte';
  import Button from '../lib/components/atoms/Button/Button.svelte';
  import Countdown from '../lib/components/atoms/Countdown/Countdown.svelte';
  import Icon from '../lib/components/atoms/Icon/Icon.svelte';
  import Link from '../lib/components/atoms/Link/Link.svelte';
  import StatusBadge from '../lib/components/atoms/StatusBadge/StatusBadge.svelte';
  import Timestamp from '../lib/components/atoms/Timestamp/Timestamp.svelte';
  import PropertyList from '../lib/components/molecules/PropertyList/PropertyList.svelte';
  import DataTable, { type Column } from '../lib/components/organisms/DataTable/DataTable.svelte';
  import type { RateLimitStatus } from '../app/api/types';
  import { ceilingText, noCheckReason, paceText, serviceName } from '../app/ratelimits';
  import { healthRows, NOW, nowDate, type HealthRow } from '../lib/storybook/fixtures';

  let { allOk = false }: { allOk?: boolean } = $props();

  const rank: Record<HealthRow['status'], number> = { action_needed: 0, failing: 1, backed_off: 2, degraded: 3, ok: 4, disabled: 5 };
  // svelte-ignore state_referenced_locally
  const rows = (allOk ? healthRows.map((r) => (r.status === 'disabled' ? r : { ...r, status: 'ok' as const, message: undefined, fix: undefined, nextRetry: undefined, failures: undefined })) : healthRows)
    .slice().sort((a, b) => rank[a.status] - rank[b.status]);
  const problems = rows.filter((r) => rank[r.status] < 4);
  let expanded = $state(problems.map((r) => r.id));

  const columns: Column<HealthRow>[] = [
    { id: 'name', label: 'Integration', value: (r) => r.name, phone: 'title' },
    { id: 'type', label: 'Type', width: '9rem', value: (r) => r.type, phone: 'meta' },
    { id: 'status', label: 'Status', width: '10rem', value: (r) => r.status, phone: 'trailing' },
    { id: 'since', label: 'Since', width: '7rem', value: (r) => r.since, phone: 'hidden' },
    { id: 'retry', label: 'Next retry', width: '9rem', value: (r) => r.nextRetry, phone: 'hidden' },
  ];

  const day = 86_400_000;
  const limits: RateLimitStatus[] = [
    { service: 'api.metadata.example', pace: 34, probe: 'normal', probing: false, recent: [],
      ceilings: [{ windowSeconds: 10, limit: 400, source: 'advertised', foundAt: new Date(NOW - 2 * day).toISOString() }] },
    { service: 'indexer/usenet', pace: 0.12, probe: 'gentle', probing: false, recent: [], lastThrottle: new Date(NOW - 3 * day).toISOString(),
      nextCheck: new Date(NOW + 4 * day).toISOString(),
      ceilings: [{ windowSeconds: 60, limit: 10, source: 'probe', foundAt: new Date(NOW - 3 * day).toISOString() }] },
    { service: 'indexer/publictracker', pace: 0.4, probe: 'off', probing: false, recent: [], lastThrottle: new Date(NOW - 9 * day).toISOString(),
      ceilings: [{ windowSeconds: 1, limit: 1, source: 'throttle', foundAt: new Date(NOW - 9 * day).toISOString() }] },
  ];
  const limitColumns: Column<RateLimitStatus>[] = [
    { id: 'service', label: 'Service', value: (r) => serviceName(r.service), phone: 'title' },
    { id: 'limit', label: 'Learned limit', value: (r) => r.ceilings.map(ceilingText).join(' · '), phone: 'meta' },
    { id: 'pace', label: 'Pace now', width: '9rem', value: (r) => paceText(r.pace), phone: 'trailing' },
    { id: 'throttle', label: 'Last throttle', width: '8rem', value: (r) => r.lastThrottle, phone: 'hidden' },
    { id: 'check', label: 'Next check', width: '10rem', value: (r) => r.nextCheck, phone: 'hidden' },
  ];
</script>

{#snippet limitCell(r: RateLimitStatus, c: Column<RateLimitStatus>)}
  {#if c.id === 'throttle'}{#if r.lastThrottle}<Timestamp value={r.lastThrottle} now={nowDate} />{:else}Never{/if}
  {:else if c.id === 'check'}{#if r.nextCheck}<Countdown to={r.nextCheck} action="Checks" now={NOW} />{:else}{noCheckReason(r)}{/if}
  {:else}{c.value?.(r)}{/if}
{/snippet}

{#snippet cell(r: HealthRow, c: Column<HealthRow>)}
  {#if c.id === 'status'}<StatusBadge family="health" value={r.status} size="sm" />
  {:else if c.id === 'since'}<Timestamp value={r.since} now={nowDate} />
  {:else if c.id === 'retry'}{#if r.nextRetry}<Countdown to={r.nextRetry} action="Retries" tone="warn" now={NOW} />{:else}—{/if}
  {:else}{c.value?.(r)}{/if}
{/snippet}

{#snippet detail(r: HealthRow)}
  <div class="detail">
    {#if r.fix}<p class="fix"><Icon name="info" size="sm" /><span><strong>How to fix:</strong> {r.fix}</span></p>{/if}
    <PropertyList density="compact" label="Details for {r.name}" items={[
      ...(r.message ? [{ key: 'msg', label: 'Problem', value: r.message }] : []),
      ...(r.errorClass ? [{ key: 'class', label: 'Error class', value: r.errorClass, mono: true }] : []),
      ...(r.failures !== undefined ? [{ key: 'fail', label: 'Failures in a row', value: r.failures }] : []),
      ...(r.lastOk ? [{ key: 'ok', label: 'Last OK', value: new Date(r.lastOk).toISOString().slice(11, 16) + ' UTC' }] : []),
      ...(r.extra ? [{ key: 'extra', label: 'Reported', value: r.extra }] : []),
    ]} />
    <div class="acts">
      <Button size="sm" icon="bolt">Test now</Button>
      {#if r.fixHref}<Link href={r.fixHref}>Open settings</Link>{:else}<Link href="#/settings">Open settings</Link>{/if}
    </div>
  </div>
{/snippet}

<Shell path="/system/health" {...allOk ? { status: { unhealthy: 0, active: 3, blocked: 0 } } : {}}>
  <ListPage title="Health" description={problems.length ? `${problems.length} of ${rows.length} integrations need attention` : `All ${rows.length - 1} enabled integrations are OK`}
    {rows} {columns} rowKey={(r) => r.id} {cell} expand={detail} bind:expanded noun="integration"
    rowActions={() => [{ id: 'test', label: 'Test now', icon: 'bolt' }, { id: 'settings', label: 'Open settings', icon: 'settings' }]}>
    {#snippet actions()}<Button icon="bolt">Test all</Button>{/snippet}
    {#snippet banner()}
      {#if !problems.length}<p class="calm"><Icon name="check-circle" /><span>Every integration answered on its last check, 2 minutes ago.</span></p>{/if}
    {/snippet}
  </ListPage>
  <section class="limits" aria-labelledby="rate-limits">
    <div class="limits-head">
      <h2 id="rate-limits">Rate limits</h2>
      <p>Where each service starts throttling, how that was found, and the pace corsarr keeps below it.</p>
    </div>
    <DataTable rows={limits} columns={limitColumns} rowKey={(r) => r.service} cell={limitCell} label="Rate limits" height="auto" density="compact" noun="service" />
  </section>
</Shell>

<style>
  .limits { display: grid; gap: var(--space-3); margin-top: var(--space-6); }
  .limits-head { display: grid; gap: var(--space-1); }
  .limits-head h2 { font-size: var(--text-lg); line-height: var(--leading-tight); }
  .limits-head p { font-size: var(--text-md); color: var(--ink-muted); }
  .detail { display: grid; gap: var(--space-3); max-width: 48rem; }
  .fix { display: flex; gap: var(--space-2); align-items: flex-start; font-size: var(--text-md); color: var(--ink); }
  .fix :global(svg) { color: var(--tone-info-ink); flex: none; margin-top: var(--space-0-5); }
  .acts { display: flex; gap: var(--space-3); align-items: center; flex-wrap: wrap; }
  .calm { display: flex; align-items: center; gap: var(--space-2); padding: var(--space-3) var(--space-4); color: var(--ink-muted);
    background: var(--surface-raised); border: var(--border-w) solid var(--line); border-radius: var(--radius-lg); }
  .calm :global(svg) { color: var(--tone-ok-solid); flex: none; }
</style>
