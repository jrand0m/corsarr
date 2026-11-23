<!-- Dashboard: what needs attention first, then active downloads, counts, health and recent history. -->
<script lang="ts">
  import { onMount } from 'svelte';
  import DashboardPage, { type AttentionItem, type DashboardRegion, type HealthProblem } from '../../../lib/components/templates/DashboardPage/DashboardPage.svelte';
  import type { QueueAction } from '../../../lib/components/molecules/QueueCard/QueueCard.svelte';
  import type { TimelineEntry } from '../../../lib/components/organisms/Timeline/Timeline.svelte';
  import { app } from '../../context.svelte';
  import { Resource } from '../../resource.svelte';
  import { refetchOn } from '../../live.svelte';
  import { toApiError } from '../../api/client';
  import type { Download, HistoryEntry, IntegrationHealth, WantedEntry } from '../../api/types';
  import { actorLabel, eventDetail, eventSubject, eventView, healthRank, integrationName, integrationType, isActiveDownload, settingsFor, toQueueCard } from '../../model';
  import { actionDone, pausedIds, queueAction } from '../../queue.svelte';

  const { api, live, movies, toasts, navigate } = app();

  const queue = new Resource((s) => api.list<Download>('queue', { limit: 200 }, s));
  const health = new Resource((s) => api.list<IntegrationHealth>('health/integrations', undefined, s));
  const history = new Resource((s) => api.page<HistoryEntry>('history', undefined, 8, undefined, s));
  const wanted = new Resource((s) => api.page<WantedEntry>('wanted/missing', undefined, 200, undefined, s));

  onMount(() => {
    void queue.load();
    void health.load();
    void history.load();
    void wanted.load();
    movies.ensure();
    const offs = [
      refetchOn(live, ['download.*', 'grab.sent', 'import.*'], () => void queue.load(true), 500),
      refetchOn(live, ['health.changed'], () => void health.load(true)),
      refetchOn(live, ['grab.*', 'download.failed', 'download.removed', 'import.*', 'payload.rejected', 'blocklist.*', 'rule.*', 'health.changed', 'item.added', 'item.deleted'], () => void history.load(true), 800),
      refetchOn(live, ['item.*', 'import.completed', 'file.deleted'], () => void wanted.load(true), 800),
    ];
    return () => offs.forEach((off) => off());
  });

  const active = $derived((queue.data ?? []).filter(isActiveDownload));
  const cards = $derived(active.slice(0, 4).map((d) => ({ ...toQueueCard(d, movies.title), paused: pausedIds.has(String(d.id)) })));
  const problems = $derived((health.data ?? []).filter((h) => healthRank[h.status] < 4).sort((a, b) => healthRank[a.status] - healthRank[b.status]));

  const attention = $derived<AttentionItem[]>([
    ...active.filter((d) => d.state === 'import-blocked' || d.state === 'failed' || d.state === 'stalled').map((d) => {
      const card = toQueueCard(d, movies.title);
      return { id: `dl:${d.id}`, title: card.title, detail: d.statusReason || card.subtitle, kind: d.kind || 'movie' as const,
        status: { family: 'download' as const, value: d.state }, href: '#/activity/queue' };
    }),
    ...problems.filter((h) => healthRank[h.status] <= 1).map((h) => ({
      id: `h:${h.ref.type}:${h.ref.key}`, title: `${integrationType(h.ref.type)} ${integrationName(h.ref)}`, detail: h.fixHint ?? h.lastErrorMessage,
      status: { family: 'health' as const, value: h.status }, href: '#/system/health',
    })),
  ]);

  const tiles = $derived([
    { label: 'Movies', value: movies.loaded || movies.rows.length ? movies.rows.length : null, kind: 'count' as const, href: '#/library/movies', linkLabel: 'Open the library', loading: !movies.loaded && !movies.rows.length && !movies.res.error },
    { label: 'Missing', value: wanted.data ? wanted.data.items.length : null, kind: 'count' as const, href: '#/wanted/missing', linkLabel: 'See what is missing',
      tone: (wanted.data?.items.length ? 'warn' : undefined) as 'warn' | undefined, loading: wanted.loading && !wanted.data },
    { label: 'Downloading', value: queue.data ? active.filter((d) => d.state === 'downloading').length : null, kind: 'count' as const, href: '#/activity/queue', linkLabel: 'Open the queue', loading: queue.loading && !queue.data },
    { label: 'Integrations OK', value: health.data ? health.data.length - problems.length : null, limit: health.data?.length, kind: 'count' as const, href: '#/system/health', linkLabel: 'Open health',
      tone: (problems.length ? 'warn' : 'ok') as 'warn' | 'ok', loading: health.loading && !health.data },
  ]);

  const healthView = $derived(health.data ? {
    ok: health.data.length - problems.length, total: health.data.length,
    problems: problems.map<HealthProblem>((h) => ({ id: `${h.ref.type}:${h.ref.key}`, name: `${integrationType(h.ref.type)} ${integrationName(h.ref)}`, status: h.status,
      message: h.lastErrorMessage, href: settingsFor(h.ref.type) })),
  } : undefined);

  const timeline = $derived<TimelineEntry[]>((history.data?.items ?? []).map((h) => {
    const v = eventView(h.event);
    return { id: String(h.id), at: h.event.occurredAt, kind: 'observation', title: `${v.label}: ${eventSubject(h.event)}`, status: { family: v.family, value: v.value },
      actor: actorLabel(h.event.actor), detail: eventDetail(h.event) };
  }));

  const since = $derived.by(() => {
    const t = Math.max(0, ...(health.data ?? []).map((h) => Date.parse(h.since) || 0));
    return t ? `since ${new Date(t).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', hour12: false })}` : undefined;
  });

  const loading = $derived<DashboardRegion[]>([
    ...(queue.loading && !queue.data ? ['attention', 'downloads'] as const : []),
    ...(health.loading && !health.data ? ['health'] as const : []),
    ...(history.loading && !history.data ? ['history'] as const : []),
  ]);
  const errors = $derived<Partial<Record<DashboardRegion, string>>>({
    ...(queue.error ? { downloads: queue.error.message, attention: queue.error.message } : {}),
    ...(health.error ? { health: health.error.message } : {}),
    ...(history.error ? { history: history.error.message } : {}),
  });
  function retry(r: DashboardRegion) {
    if (r === 'downloads' || r === 'attention') void queue.load();
    else if (r === 'health') void health.load();
    else if (r === 'history') void history.load();
    else void wanted.load();
  }

  async function act(action: QueueAction, id: string) {
    if (action === 'kill' || action === 'replace' || action === 'replace-lower') { navigate('/activity/queue'); return; }
    try {
      await queueAction(api, id, action);
      toasts.push({ message: actionDone[action] ?? 'Done', tone: 'ok' });
      void queue.load(true);
    } catch (e) {
      toasts.push({ message: 'The download client did not take that', detail: toApiError(e).message, tone: 'error' });
    }
  }
</script>

<DashboardPage {attention} healthySince={since} {tiles} downloads={cards} downloadsTotal={active.length > cards.length ? active.length - cards.length : undefined}
  totalSpeed={active.reduce((n, d) => n + (d.speed || 0), 0)} onqueueaction={act} health={healthView} history={timeline} {loading} {errors} onretry={retry} />
