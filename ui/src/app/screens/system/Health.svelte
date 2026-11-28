<!-- System › Health: problems first and expanded with their fix hints; all OK is one calm line. -->
<script lang="ts">
  import { onMount } from 'svelte';
  import ListPage from '../../../lib/components/templates/ListPage/ListPage.svelte';
  import Button from '../../../lib/components/atoms/Button/Button.svelte';
  import Countdown from '../../../lib/components/atoms/Countdown/Countdown.svelte';
  import Icon from '../../../lib/components/atoms/Icon/Icon.svelte';
  import Link from '../../../lib/components/atoms/Link/Link.svelte';
  import StatusBadge from '../../../lib/components/atoms/StatusBadge/StatusBadge.svelte';
  import Timestamp from '../../../lib/components/atoms/Timestamp/Timestamp.svelte';
  import PropertyList from '../../../lib/components/molecules/PropertyList/PropertyList.svelte';
  import DataTable, { type Column } from '../../../lib/components/organisms/DataTable/DataTable.svelte';
  import { formatAbsolute } from '../../../lib/format/time';
  import { app } from '../../context.svelte';
  import { Resource } from '../../resource.svelte';
  import { refetchOn } from '../../live.svelte';
  import { toApiError } from '../../api/client';
  import type { IntegrationHealth, RateLimits, RateLimitStatus } from '../../api/types';
  import { ceilingText, noCheckReason, paceText, serviceName } from '../../ratelimits';
  import { healthRank, integrationName, integrationType, isProblem, settingsFor } from '../../model';

  const { api, live, toasts } = app();
  const health = new Resource((s) => api.list<IntegrationHealth>('health/integrations', undefined, s));
  const limits = new Resource((s) => api.get<RateLimits>('system/ratelimits', undefined, s));
  onMount(() => {
    void health.load();
    void limits.load();
    return refetchOn(live, ['health.changed'], () => void health.load(true), 300);
  });

  const keyOf = (h: IntegrationHealth) => `${h.ref.type}:${h.ref.key}`;
  const rows = $derived((health.data ?? []).slice().sort((a, b) => healthRank[a.status] - healthRank[b.status] || keyOf(a).localeCompare(keyOf(b))));
  const problems = $derived(rows.filter(isProblem));
  let expanded = $state<string[]>([]);
  let seeded = false;
  $effect(() => {
    if (!seeded && health.data) { expanded = problems.map(keyOf); seeded = true; }
  });

  let testing = $state(false);
  async function testAll() {
    testing = true;
    try {
      await api.post('jobs/instances.test/run');
      toasts.push({ message: 'Testing every integration', detail: 'Results update here as each one answers.', tone: 'info' });
    } catch (e) {
      toasts.push({ message: 'The test did not start', detail: toApiError(e).message, tone: 'error' });
    } finally {
      testing = false;
    }
  }

  const columns: Column<IntegrationHealth>[] = [
    { id: 'name', label: 'Integration', value: (r) => integrationName(r.ref), phone: 'title' },
    { id: 'type', label: 'Type', width: '10rem', value: (r) => integrationType(r.ref.type), phone: 'meta' },
    { id: 'status', label: 'Status', width: '10.5rem', value: (r) => r.status, phone: 'trailing' },
    { id: 'since', label: 'Since', width: '7rem', value: (r) => r.since, phone: 'hidden' },
    { id: 'retry', label: 'Next retry', width: '9rem', value: (r) => r.nextRetryAt, phone: 'hidden' },
  ];
  const limitRows = $derived(limits.data?.items ?? []);
  const limitColumns: Column<RateLimitStatus>[] = [
    { id: 'service', label: 'Service', value: (r) => serviceName(r.service), phone: 'title' },
    { id: 'limit', label: 'Learned limit', value: (r) => r.ceilings.map(ceilingText).join(' · ') || 'Not found yet', phone: 'meta' },
    { id: 'pace', label: 'Pace now', width: '9rem', value: (r) => (r.probing ? `${paceText(r.pace)} (checking)` : paceText(r.pace)), phone: 'trailing' },
    { id: 'throttle', label: 'Last throttle', width: '8rem', value: (r) => r.lastThrottle, phone: 'hidden' },
    { id: 'check', label: 'Next check', width: '10rem', value: (r) => r.nextCheck, phone: 'hidden' },
  ];
  const lastCheck = $derived(Math.max(0, ...rows.map((r) => Date.parse(r.lastOkAt ?? '') || 0)));
</script>

{#snippet cell(r: IntegrationHealth, c: Column<IntegrationHealth>)}
  {#if c.id === 'status'}<StatusBadge family="health" value={r.status} size="sm" />
  {:else if c.id === 'since'}<Timestamp value={r.since} />
  {:else if c.id === 'retry'}{#if r.nextRetryAt}<Countdown to={r.nextRetryAt} action="Retries" tone="warn" />{:else}—{/if}
  {:else}{c.value?.(r)}{/if}
{/snippet}

{#snippet detail(r: IntegrationHealth)}
  <div class="detail">
    {#if r.fixHint}<p class="fix"><Icon name="info" size="sm" /><span><strong>How to fix:</strong> {r.fixHint}</span></p>{/if}
    <PropertyList density="compact" label="Details for {integrationName(r.ref)}" items={[
      ...(r.lastErrorMessage ? [{ key: 'msg', label: 'Problem', value: r.lastErrorMessage }] : []),
      ...(r.lastErrorClass ? [{ key: 'class', label: 'Error class', value: r.lastErrorClass, mono: true }] : []),
      { key: 'fail', label: 'Failures in a row', value: r.consecutiveFailures },
      ...(r.lastOkAt ? [{ key: 'ok', label: 'Last OK', value: formatAbsolute(new Date(r.lastOkAt)) }] : [{ key: 'ok', label: 'Last OK', value: 'Not yet' }]),
    ]} />
    <div class="acts">
      <Button size="sm" icon="bolt" loading={testing} onclick={testAll}>Test now</Button>
      <Link href={settingsFor(r.ref.type)}>Open settings</Link>
    </div>
  </div>
{/snippet}

<ListPage title="Health" description={rows.length ? (problems.length ? `${problems.length} of ${rows.length} integrations need attention` : `All ${rows.length} integrations are OK`) : undefined}
  {rows} {columns} rowKey={keyOf} {cell} expand={detail} bind:expanded noun="integration"
  loading={health.loading && !health.data} error={health.error?.message} errorCode={health.error?.label} onretry={health.retry}
  rowActions={(r) => [{ id: 'test', label: 'Test now', icon: 'bolt' }, { id: 'settings', label: 'Open settings', icon: 'settings' }]}
  onaction={(a, r) => (a === 'test' ? testAll() : (location.hash = settingsFor(r.ref.type)))}>
  {#snippet actions()}<Button icon="bolt" loading={testing} onclick={testAll}>Test all</Button>{/snippet}
  {#snippet banner()}
    {#if health.data && !problems.length}<p class="calm"><Icon name="check-circle" /><span>Every integration answered on its last check{#if lastCheck}, <Timestamp value={lastCheck} />{/if}.</span></p>{/if}
  {/snippet}
</ListPage>

{#snippet limitCell(r: RateLimitStatus, c: Column<RateLimitStatus>)}
  {#if c.id === 'throttle'}{#if r.lastThrottle}<Timestamp value={r.lastThrottle} />{:else}Never{/if}
  {:else if c.id === 'check'}{#if r.heldUntil}<Countdown to={r.heldUntil} action="Resumes" tone="warn" />{:else if r.nextCheck}<Countdown to={r.nextCheck} action="Checks" />{:else}{noCheckReason(r)}{/if}
  {:else}{c.value?.(r)}{/if}
{/snippet}

<section class="limits" aria-labelledby="rate-limits">
  <div class="limits-head">
    <h2 id="rate-limits">Rate limits</h2>
    <p>{limits.data && !limits.data.enabled ? 'Learning is off: only the configured limits apply.'
      : 'Where each service starts throttling, how that was found, and the pace corsarr keeps below it.'}</p>
  </div>
  <DataTable rows={limitRows} columns={limitColumns} rowKey={(r) => r.service} cell={limitCell} label="Rate limits" height="auto" density="compact"
    noun="service" loading={limits.loading && !limits.data} error={limits.error?.message} errorCode={limits.error?.label} onretry={limits.retry} />
</section>

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
