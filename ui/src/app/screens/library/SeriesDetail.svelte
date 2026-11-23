<!--
  Series detail: header facts, seasons (monitoring, counts, bay grid, season search) and the
  episodes of each season (monitoring, status, search). Interactive search streams decisions for a
  season or an episode into the interactive search drawer; a grab goes through the override rules (409 asks to
  confirm, 403 refuses, shadow mode only records it). Edit and delete live in the header menu.
-->
<script lang="ts">
  import { onMount } from 'svelte';
  import DetailPage from '../../../lib/components/templates/DetailPage/DetailPage.svelte';
  import BayGrid, { type BayCell } from '../../../lib/components/atoms/BayGrid/BayGrid.svelte';
  import Button from '../../../lib/components/atoms/Button/Button.svelte';
  import Progress from '../../../lib/components/atoms/Progress/Progress.svelte';
  import Quantity from '../../../lib/components/atoms/Quantity/Quantity.svelte';
  import Skeleton from '../../../lib/components/atoms/Skeleton/Skeleton.svelte';
  import StatusBadge from '../../../lib/components/atoms/StatusBadge/StatusBadge.svelte';
  import BoolField from '../../../lib/components/molecules/BoolField/BoolField.svelte';
  import EmptyState from '../../../lib/components/molecules/EmptyState/EmptyState.svelte';
  import EnumField from '../../../lib/components/molecules/EnumField/EnumField.svelte';
  import ErrorState from '../../../lib/components/molecules/ErrorState/ErrorState.svelte';
  import ListField from '../../../lib/components/molecules/ListField/ListField.svelte';
  import MonitorToggle, { type MonitorState } from '../../../lib/components/molecules/MonitorToggle/MonitorToggle.svelte';
  import PropertyList from '../../../lib/components/molecules/PropertyList/PropertyList.svelte';
  import QueueCard, { type QueueAction } from '../../../lib/components/molecules/QueueCard/QueueCard.svelte';
  import SourceProgressStrip, { type SourceProgress } from '../../../lib/components/molecules/SourceProgressStrip/SourceProgressStrip.svelte';
  import ConfirmDialog from '../../../lib/components/organisms/ConfirmDialog/ConfirmDialog.svelte';
  import DataTable, { type Column } from '../../../lib/components/organisms/DataTable/DataTable.svelte';
  import DecisionPanel, { type Decision as PanelDecision, type DecisionReason } from '../../../lib/components/organisms/DecisionPanel/DecisionPanel.svelte';
  import Drawer from '../../../lib/components/organisms/Drawer/Drawer.svelte';
  import type { TimelineEntry } from '../../../lib/components/organisms/Timeline/Timeline.svelte';
  import { formatAbsolute } from '../../../lib/format/time';
  import { formatBytes } from '../../../lib/format/quantity';
  import { app, profileName } from '../../context.svelte';
  import { Resource } from '../../resource.svelte';
  import { refetchOn } from '../../live.svelte';
  import { ApiError, toApiError } from '../../api/client';
  import type { Decided, Download, Episode, HistoryEntry, LiveEvent, ReleaseLine, SearchSummary, Season, Series, SeriesDetail } from '../../api/types';
  import { actorLabel, eventDetail, eventView, isActiveDownload, parseEnvExport, toQueueCard } from '../../model';
  import { actionDone, pausedIds, queueAction } from '../../queue.svelte';
  import {
    count, episodeCode, episodeRange, episodeState, itemStatusOf, monitorModes, queuedEpisodes, seasonName, seasonOrder, seriesItemStatus, seriesStatusLabel,
  } from '../../tv';
  import type { ScreenProps } from '../../screen';

  let { params = {} }: ScreenProps = $props();
  const { api, live, profiles, toasts, navigate } = app();
  // svelte-ignore state_referenced_locally
  const id = Number(params.id);

  const detail = new Resource((s) => api.get<SeriesDetail>(`series/${id}`, undefined, s));
  const queue = new Resource((s) => api.list<Download>('queue', { limit: 200 }, s));
  // Workaround: History has no item filter yet, so read the newest 200 events and keep this series'.
  const history = new Resource(async (s) => (await api.page<HistoryEntry>('history', undefined, 200, undefined, s)).items.filter((h) => mine(h.event, true)));

  const show = $derived(detail.data?.series);
  const episodes = $derived(detail.data?.episodes ?? []);
  const seasons = $derived((detail.data?.seasons ?? []).slice().sort(seasonOrder));
  const episodeIds = $derived(new Set(episodes.map((e) => e.id)));
  const byId = $derived(new Map(episodes.map((e) => [e.id, e])));

  /** The event concerns this series (or, loosely, any TV item when it names none). */
  function mine(ev: Pick<LiveEvent, 'items' | 'mediaKind'>, strict = false): boolean {
    if (!ev.items?.length) return !strict && (ev.mediaKind === 'tv' || !ev.mediaKind);
    return ev.items.some((i) => i.ref.kind === 'tv' && (i.ref.variant === 'series' ? i.ref.id === id : episodeIds.has(i.ref.id)));
  }

  onMount(() => {
    void detail.load();
    void queue.load();
    void history.load();
    if (!profiles.data) void profiles.load();
    const offs = [
      live.on('item.*', (ev) => { if (mine(ev)) void detail.load(true); }),
      live.on('import.completed', (ev) => { if (mine(ev)) void detail.load(true); }),
      live.on('file.deleted', (ev) => { if (mine(ev)) void detail.load(true); }),
      refetchOn(live, ['download.*', 'grab.sent', 'import.*'], () => void queue.load(true), 500),
      refetchOn(live, ['grab.*', 'download.failed', 'download.removed', 'import.*', 'item.added', 'blocklist.*'], () => void history.load(true), 800),
      live.onreconnect(() => void detail.load(true)),
    ];
    return () => offs.forEach((off) => off());
  });

  const queued = $derived(queuedEpisodes(queue.data ?? []));
  const downloads = $derived((queue.data ?? []).filter((d) => isActiveDownload(d) && (d.links ?? []).some((l) => l.item.kind === 'tv' && episodeIds.has(l.item.id))));
  const namedRange = (ids: number[]) => episodeRange(show?.title ?? '', ids.map((i) => byId.get(i)).filter((e): e is Episode => !!e));
  const day = (s?: string | null) => (s ? formatAbsolute(new Date(s)).replace(/,.*$/, '') : undefined);
  const runtime = (min?: number) => (min ? (min >= 60 ? `${Math.floor(min / 60)}h ${min % 60}m` : `${min}m`) : undefined);

  const entity = $derived(show ? {
    title: show.title, kind: 'tv' as const, art: show.posterUrl || null,
    meta: [show.year || undefined, show.network, seriesStatusLabel[show.status || ''], runtime(show.runtimeMin)].filter(Boolean).join(' · '),
    monitor: (show.monitored ? 'monitored' : 'unmonitored') as MonitorState,
    status: [{ family: 'item' as const, value: downloads.length ? 'queued' : seriesItemStatus(show), detail: downloads.length ? count(downloads.length, 'download') : undefined }],
    facts: [
      { label: 'Profile', value: profileName(profiles.data, show.profileKey) },
      { label: 'Episodes', value: `${show.stats?.episodeFileCount ?? 0} of ${show.stats?.episodeCount ?? 0} on disk` },
      ...(show.stats?.nextAiring ? [{ label: 'Next airing', value: day(show.stats.nextAiring)! }] : []),
      { label: 'Folder', value: show.path },
    ],
    back: { href: '#/library/tv', label: 'TV' },
    menu: [
      { id: 'search', label: 'Search missing episodes', icon: 'search' as const },
      { id: 'refresh', label: 'Refresh metadata', icon: 'refresh' as const },
      { id: 'edit', label: 'Edit series…', icon: 'pencil' as const },
      { id: 'delete', label: 'Delete series…', icon: 'trash' as const, danger: true, group: 'danger' },
    ],
  } : { title: 'Series', kind: 'tv' as const });

  // ── monitoring ─────────────────────────────────────────────────────────
  async function update(body: Record<string, unknown>, failure: string): Promise<boolean> {
    try {
      const next = await api.put<Series>(`series/${id}`, body);
      if (detail.data) detail.data = { ...detail.data, series: { ...detail.data.series, ...next } };
      if ('monitor' in body) void detail.load(true);
      return true;
    } catch (e) {
      toasts.push({ message: failure, detail: toApiError(e).message, tone: 'error' });
      return false;
    }
  }
  const setMonitor = (state: MonitorState) => update({ monitored: state === 'monitored' }, 'Monitoring was not changed');

  async function setSeason(n: number, on: boolean) {
    try {
      detail.data = await api.put<SeriesDetail>(`series/${id}/seasons/${n}`, { monitored: on });
    } catch (e) {
      toasts.push({ message: `${seasonName(n)} monitoring was not changed`, detail: toApiError(e).message, tone: 'error' });
    }
  }
  async function setEpisodes(ids: number[], on: boolean) {
    try {
      await api.put<{ updated: number }>('episodes/monitor', { ids, monitored: on });
      void detail.load(true);
    } catch (e) {
      toasts.push({ message: 'Episode monitoring was not changed', detail: toApiError(e).message, tone: 'error' });
    }
  }

  // ── searches ───────────────────────────────────────────────────────────
  let searching = $state<string | undefined>();
  async function search(path: string, what: string) {
    if (searching) return;
    searching = path;
    try {
      const s = await api.post<SearchSummary>(path);
      toasts.push(s.grabbed
        ? { message: `Grabbed releases for ${what}`, detail: `${s.accepted} of ${s.releases} releases accepted.`, tone: 'ok', actionLabel: 'Open queue', onaction: () => navigate('/activity/queue') }
        : s.shadowed
          ? { message: `Shadow mode: nothing grabbed for ${what}`, detail: 'TV is in shadow mode; History shows what corsarr would have grabbed.', tone: 'info' }
          : { message: s.releases ? 'No release was good enough to grab' : 'No releases found', detail: s.failed ? `${s.failed} of ${s.indexers} indexers failed.` : `${s.releases} releases checked across ${s.indexers} indexers.`, tone: s.failed ? 'warn' : 'info' });
    } catch (e) {
      toasts.push({ message: 'The search did not run', detail: toApiError(e).message, tone: 'error' });
    } finally {
      searching = undefined;
    }
  }
  const searchMissing = () => search(`series/${id}/search`, 'missing episodes');
  const searchSeason = (n: number) => search(`series/${id}/seasons/${n}/search`, seasonName(n).toLowerCase());
  const searchEpisode = (e: Episode) => search(`episodes/${e.id}/search`, episodeCode(e.season, e.number));

  // ── interactive search (drawer) ────────────────────────────────────
  interface Target { label: string; path: string; grab: { season?: number; episodeId?: number }; runtimeMin?: number }
  let target = $state<Target | undefined>();
  let drawerOpen = $state(false);
  let decided = $state<Decided[]>([]);
  let sources = $state<SourceProgress[]>([]);
  let streamError = $state<ApiError | undefined>();
  let streaming = $state(false);
  let summary = $state<Extract<ReleaseLine, { summary: unknown }>['summary'] | undefined>();
  let streamCtl: AbortController | undefined;

  async function indexerSources(): Promise<SourceProgress[]> {
    try {
      const v = parseEnvExport(await api.text('config/export', { format: 'env', full: true }));
      return v.instances.filter((i) => i.section === 'indexers').map((i) => ({ id: i.key, name: i.name ?? i.key, state: 'running' as const }));
    } catch {
      return [];
    }
  }
  const seasonTarget = (n: number): Target => ({ label: seasonName(n), path: `series/${id}/seasons/${n}/releases`, grab: { season: n }, runtimeMin: show?.runtimeMin });
  const episodeTarget = (e: Episode): Target => ({
    label: `${episodeCode(e.season, e.number)}${e.title ? ` · ${e.title}` : ''}`, path: `episodes/${e.id}/releases`, grab: { episodeId: e.id }, runtimeMin: e.runtimeMin ?? show?.runtimeMin,
  });

  async function openInteractive(t: Target) {
    target = t;
    drawerOpen = true;
    streamCtl?.abort();
    const ctl = new AbortController();
    streamCtl = ctl;
    decided = [];
    summary = undefined;
    streamError = undefined;
    streaming = true;
    const started = performance.now();
    sources = await indexerSources();
    const counts = new Map<string, number>();
    try {
      for await (const line of api.stream<ReleaseLine>(t.path, ctl.signal)) {
        if ('record' in line) {
          decided = [...decided, line.record];
          for (const ix of line.record.release.indexers ?? []) {
            counts.set(ix.key, (counts.get(ix.key) ?? 0) + 1);
            sources = upsertSource(sources, { id: ix.key, name: ix.name || ix.key, state: 'ok', count: counts.get(ix.key), ms: Math.round(performance.now() - started) });
          }
        } else if ('error' in line) {
          sources = upsertSource(sources, { id: line.error.source, name: sources.find((s) => s.id === line.error.source)?.name ?? line.error.source, state: 'failed', reason: line.error.message });
        } else if ('summary' in line) {
          summary = line.summary;
          sources = sources.map((s) => (s.state === 'running' ? { ...s, state: 'ok', count: counts.get(s.id) ?? 0, ms: Math.round(performance.now() - started) } : s));
        }
      }
    } catch (e) {
      if (!ctl.signal.aborted) streamError = toApiError(e);
    } finally {
      if (streamCtl === ctl) streaming = false;
    }
  }
  function upsertSource(list: SourceProgress[], s: SourceProgress): SourceProgress[] {
    const i = list.findIndex((x) => x.id === s.id);
    if (i < 0) return [...list, s];
    const next = list.slice();
    next[i] = { ...list[i], ...s, name: list[i].name || s.name };
    return next;
  }
  function closeDrawer() {
    streamCtl?.abort();
    streaming = false;
  }

  const ranked = $derived(decided.slice().sort((a, b) =>
    Number(b.decision.accepted) - Number(a.decision.accepted) || (a.decision.rankKey < b.decision.rankKey ? 1 : a.decision.rankKey > b.decision.rankKey ? -1 : 0)));
  const accepted = $derived(ranked.filter((d) => d.decision.accepted).length);
  function toPanel(d: Decided, i: number): PanelDecision {
    const r = d.release;
    return {
      release: {
        title: r.title, indexer: (r.indexers ?? []).map((x) => x.name || x.key).join(', ') || 'unknown indexer', protocol: r.protocol === 'usenet' ? 'usenet' : 'torrent',
        size: r.size, publishedAt: r.publishedAt, seeders: r.protocol === 'usenet' ? undefined : r.seeders, quality: d.decision.quality || 'unknown',
        runtimeSeconds: target?.runtimeMin && target.grab.episodeId ? target.runtimeMin * 60 : undefined,
      },
      verdict: d.decision.accepted ? 'accept' : 'reject',
      rank: d.decision.accepted ? i + 1 : undefined,
      candidates: d.decision.accepted ? accepted : undefined,
      score: { total: d.decision.formatScore, formats: [] },
      reasons: (d.decision.reasons ?? []).map<DecisionReason>((x) => ({ rule: x.ruleId, category: x.category, message: x.message, blocklist: x.ruleId.includes('blocklist') })),
    };
  }

  let confirmGrab = $state<{ d: Decided; reasons: string[] } | undefined>();
  let confirmOpen = $state(false);
  let grabbing = $state(false);
  async function grab(d: Decided, confirm: boolean) {
    if (!target) return;
    grabbing = true;
    try {
      await api.post(`series/${id}/grabs`, { releaseId: d.release.id, confirm, ...target.grab });
      confirmOpen = false;
      drawerOpen = false;
      closeDrawer();
      toasts.push({ message: `Grab sent: ${d.release.title}`, detail: 'Watch it in the queue.', tone: 'ok', actionLabel: 'Open queue', onaction: () => navigate('/activity/queue') });
      void queue.load(true);
    } catch (e) {
      const x = toApiError(e);
      const reasons = ((x.body.decision as { reasons?: { message: string }[] } | undefined)?.reasons ?? []).map((r) => r.message);
      if (x.status === 409 && x.code !== 'shadowed') { confirmGrab = { d, reasons }; confirmOpen = true; }
      else {
        confirmOpen = false;
        toasts.push(x.code === 'shadowed'
          ? { message: 'Shadow mode: the grab was recorded, not sent', detail: 'TV is in shadow mode; History shows the decision.', tone: 'info' }
          : x.status === 403
            ? { message: 'corsarr will not grab this release', detail: reasons[0] ?? x.message, tone: 'error' }
            : { message: 'The grab was not sent', detail: x.status === 404 ? 'The search results expired. Run the search again.' : x.message, tone: 'error' });
      }
    } finally {
      grabbing = false;
    }
  }

  // ── header actions: refresh, edit, delete ──────────────────────────────
  let refreshing = $state(false);
  async function refresh() {
    refreshing = true;
    try {
      detail.data = await api.post<SeriesDetail>(`series/${id}/refresh`);
      toasts.push({ message: 'Metadata refreshed', tone: 'ok' });
    } catch (e) {
      toasts.push({ message: 'The refresh failed', detail: toApiError(e).message, tone: 'error' });
    } finally {
      refreshing = false;
    }
  }

  let editOpen = $state(false);
  let edit = $state({ profileKey: '', monitor: '', monitorNew: 'all', seasonFolders: true, monitored: true, tags: [] as string[] });
  let saving = $state(false);
  function openEdit() {
    if (!show) return;
    edit = { profileKey: show.profileKey, monitor: '', monitorNew: show.monitorNew, seasonFolders: show.seasonFolders, monitored: show.monitored, tags: [...(show.tags ?? [])] };
    editOpen = true;
  }
  async function saveEdit() {
    saving = true;
    const { monitor, ...rest } = edit;
    if (await update(monitor ? { ...rest, monitor } : rest, 'The series was not saved')) {
      editOpen = false;
      toasts.push({ message: `Saved ${show?.title ?? 'the series'}`, detail: monitor ? 'The monitoring mode was applied to the existing episodes.' : undefined, tone: 'ok' });
    }
    saving = false;
  }

  let deleteOpen = $state(false);
  async function remove(choice: string | undefined) {
    deleteOpen = false;
    try {
      await api.del(`series/${id}`, { deleteFiles: choice === 'files' });
      toasts.push({ message: `Deleted ${show?.title ?? 'the series'}`, tone: 'ok' });
      navigate('/library/tv');
    } catch (e) {
      toasts.push({ message: 'The series was not deleted', detail: toApiError(e).message, tone: 'error' });
    }
  }
  function onmenu(item: string) {
    if (item === 'search') void searchMissing();
    else if (item === 'refresh') void refresh();
    else if (item === 'edit') openEdit();
    else if (item === 'delete') deleteOpen = true;
  }

  async function onqueue(action: QueueAction, did: string) {
    if (!(action in actionDone)) { navigate('/activity/queue'); return; }
    try {
      await queueAction(api, did, action);
      toasts.push({ message: actionDone[action] ?? 'Done', tone: 'ok' });
    } catch (e) {
      toasts.push({ message: 'The download client did not take that', detail: toApiError(e).message, tone: 'error' });
    }
  }

  // ── seasons and episodes ───────────────────────────────────────────────
  let expanded = $state<number[] | undefined>();
  $effect(() => {
    if (expanded === undefined && seasons.length) expanded = [seasons.find((s) => s.number > 0)?.number ?? seasons[0].number];
  });
  const isOpen = (n: number) => expanded?.includes(n) ?? false;
  const toggle = (n: number) => (expanded = isOpen(n) ? (expanded ?? []).filter((x) => x !== n) : [...(expanded ?? []), n]);

  const seasonEpisodes = (n: number) => episodes.filter((e) => e.season === n).sort((a, b) => a.number - b.number);
  const stateOf = (e: Episode) => episodeState(e, queued);
  const cells = (n: number): BayCell[] => seasonEpisodes(n).map((e) => ({ state: stateOf(e), label: `${episodeCode(e.season, e.number)}${e.title ? ` ${e.title}` : ''}` }));
  function badgeDetail(e: Episode): string | undefined {
    if (e.file) return e.file.quality;
    const d = queued.get(e.id);
    return d && d.size ? `${Math.round(d.progress * 100)}%` : undefined;
  }

  const columns: Column<Episode>[] = [
    { id: 'monitored', label: 'Monitored', width: '3rem', hideLabel: true, phone: 'hidden' },
    { id: 'number', label: 'Episode', width: '5.5rem', value: (e) => episodeCode(e.season, e.number), phone: 'meta' },
    { id: 'title', label: 'Title', value: (e) => e.title || 'To be announced', phone: 'title' },
    { id: 'air', label: 'Air date', width: '8rem', value: (e) => day(e.airAt) ?? 'Not announced', phone: 'hidden' },
    { id: 'status', label: 'Status', width: '13rem', value: (e) => itemStatusOf[stateOf(e)], phone: 'trailing' },
  ];
  const episodeActions = () => [
    { id: 'search', label: 'Search now', icon: 'search' as const },
    { id: 'interactive', label: 'Interactive search', icon: 'list' as const },
  ];
  function onepisode(action: string, e: Episode) {
    if (action === 'search') void searchEpisode(e);
    else if (action === 'interactive') void openInteractive(episodeTarget(e));
  }

  const timeline = $derived<TimelineEntry[]>((history.data ?? []).map((h) => {
    const v = eventView(h.event);
    return { id: String(h.id), at: h.event.occurredAt, kind: 'observation', title: v.label, status: { family: v.family, value: v.value }, actor: actorLabel(h.event.actor), detail: eventDetail(h.event) };
  }));

  function onkeydown(e: KeyboardEvent) {
    const t = e.target as HTMLElement | null;
    if (e.defaultPrevented || e.ctrlKey || e.metaKey || e.altKey || (t && (t.isContentEditable || /^(INPUT|TEXTAREA|SELECT)$/.test(t.tagName)))) return;
    if (document.querySelector('dialog[open]') || !show) return;
    if (e.key === 's') { e.preventDefault(); void searchMissing(); }
    else if (e.key === 'e') { e.preventDefault(); openEdit(); }
    else if (e.key === 'm') { e.preventDefault(); void setMonitor(show.monitored ? 'unmonitored' : 'monitored'); }
  }

  const tabs = $derived([{ id: 'seasons', label: 'Seasons', count: seasons.length }, { id: 'details', label: 'Details' }]);
</script>

<svelte:window {onkeydown} />

{#snippet headActions()}
  <Button icon="search" loading={searching === `series/${id}/search`} disabled={!!searching} onclick={searchMissing}>Search missing</Button>
  <Button icon="refresh" loading={refreshing} disabled={refreshing} onclick={refresh}>Refresh</Button>
  <Button variant="primary" icon="pencil" onclick={openEdit}>Edit</Button>
{/snippet}

{#snippet episodeCell(e: Episode, c: Column<Episode>)}
  {#if c.id === 'monitored'}
    <MonitorToggle item={episodeCode(e.season, e.number)} state={e.monitored ? 'monitored' : 'unmonitored'} size="sm" onchange={(s) => setEpisodes([e.id], s === 'monitored')} />
  {:else if c.id === 'number'}<span class="num">{episodeCode(e.season, e.number)}</span>
  {:else if c.id === 'title'}<span class={['ep', { tba: !e.title }]}>{e.title || 'To be announced'}</span>
  {:else if c.id === 'status'}<StatusBadge family="item" value={itemStatusOf[stateOf(e)]} detail={badgeDetail(e)} size="sm" appearance="plain" />
  {:else}{c.value?.(e)}{/if}
{/snippet}

{#snippet season(s: Season)}
  {@const eps = seasonEpisodes(s.number)}
  {@const name = seasonName(s.number)}
  <section class="season" aria-labelledby="season-{s.number}">
    <div class="head">
      <MonitorToggle item={name} state={s.monitored ? 'monitored' : 'unmonitored'} onchange={(st) => setSeason(s.number, st === 'monitored')} />
      <h2 id="season-{s.number}">{name}</h2>
      <span class="counts num">{s.fileCount}/{count(s.monitoredCount || s.episodeCount, 'episode')}{#if s.sizeOnDisk} · <Quantity kind="bytes" value={s.sizeOnDisk} />{/if}</span>
      <span class="bar"><Progress label="{name} episodes on disk" done={s.fileCount} total={s.monitoredCount || s.episodeCount} size="sm" tone={s.fileCount >= (s.monitoredCount || s.episodeCount) ? 'ok' : 'warn'} /></span>
      <span class="tools">
        <Button size="sm" icon="search" loading={searching === `series/${id}/seasons/${s.number}/search`} disabled={!!searching} onclick={() => searchSeason(s.number)}>Search</Button>
        <Button size="sm" icon="list" onclick={() => openInteractive(seasonTarget(s.number))}>Interactive</Button>
        <Button size="sm" variant="ghost" iconEnd={isOpen(s.number) ? 'chevron-up' : 'chevron-down'} aria-expanded={isOpen(s.number)} aria-controls="episodes-{s.number}" onclick={() => toggle(s.number)}>
          {isOpen(s.number) ? 'Hide episodes' : 'Show episodes'}
        </Button>
      </span>
    </div>
    {#if eps.length}<BayGrid cells={cells(s.number)} label={name} size="sm" onselect={() => { if (!isOpen(s.number)) toggle(s.number); }} />{/if}
    <div id="episodes-{s.number}" class="episodes" hidden={!isOpen(s.number)}>
      {#if isOpen(s.number)}
        <DataTable label="{name} episodes" rows={eps} {columns} rowKey={(e) => String(e.id)} cell={episodeCell} noun="episode"
          height="min(32rem, calc({eps.length} * var(--ep-row) + 3rem))" rowActions={episodeActions} onaction={onepisode}>
          {#snippet empty()}<EmptyState size="sm" title="No episodes listed yet" icon="calendar" level={3}>Refresh the metadata once the season is announced.</EmptyState>{/snippet}
        </DataTable>
      {/if}
    </div>
  </section>
{/snippet}

{#snippet body(tab: string)}
  {#if !show}
    <Skeleton lines={3} />
  {:else if tab === 'seasons'}
    {#if seasons.length}
      <div class="seasons">{#each seasons as s (s.number)}{@render season(s)}{/each}</div>
    {:else}
      <EmptyState size="sm" title="No seasons yet" icon="calendar" level={3}>The metadata source lists no seasons for this series. Refresh it later.</EmptyState>
    {/if}
  {:else}
    <div class="overview">
      {#if show.overview}<p>{show.overview}</p>{/if}
      <PropertyList label="Series" items={[
        { key: 'status', label: 'Status', value: seriesStatusLabel[show.status || ''] },
        { key: 'network', label: 'Network', value: show.network || '—' },
        { key: 'runtime', label: 'Runtime', value: runtime(show.runtimeMin) ?? '—' },
        { key: 'genres', label: 'Genres', value: (show.genres ?? []).join(', ') || '—' },
        { key: 'size', label: 'Size on disk', value: show.stats?.sizeOnDisk ? formatBytes(show.stats.sizeOnDisk) : '—' },
        { key: 'new', label: 'New seasons', value: show.monitorNew === 'all' ? 'Monitored as they appear' : 'Left unmonitored' },
        { key: 'folders', label: 'Season folders', value: show.seasonFolders ? 'Yes' : 'No' },
        { key: 'tags', label: 'Tags', value: (show.tags ?? []).join(', ') || '—' },
        { key: 'path', label: 'Folder', value: show.path, mono: true, copy: true },
        { key: 'added', label: 'Added', value: formatAbsolute(new Date(show.addedAt)) },
      ]} />
    </div>
  {/if}
{/snippet}

{#snippet aside()}
  {#if downloads.length}
    <section class="side">
      <h2>Downloading</h2>
      {#each downloads as d (d.id)}
        <QueueCard download={{ ...toQueueCard(d, () => undefined, namedRange), paused: pausedIds.has(String(d.id)) }} actions={['pause', 'resume', 'top', 'bottom', 'kill']} onaction={onqueue} />
      {/each}
    </section>
  {/if}
  {#if show}
    <PropertyList label="Identifiers" items={[
      { key: 'tmdb', label: 'TMDB', value: String(show.tmdbId), href: `https://www.themoviedb.org/tv/${show.tmdbId}` },
      ...(show.tvdbId ? [{ key: 'tvdb', label: 'TVDB', value: String(show.tvdbId) }] : []),
      ...(show.imdbId ? [{ key: 'imdb', label: 'IMDb', value: show.imdbId, href: `https://www.imdb.com/title/${show.imdbId}/` }] : []),
    ]} />
  {/if}
{/snippet}

{#snippet drawer()}
  <Drawer bind:open={drawerOpen} title="Interactive search" subtitle="{show?.title ?? ''} · {target?.label ?? ''} · {profileName(profiles.data, show?.profileKey ?? '')} profile" size="lg" onclose={closeDrawer}>
    <div class="search">
      {#if sources.length}<SourceProgressStrip {sources} label="Indexers" />{/if}
      {#if streamError}
        <ErrorState size="sm" title="The search stopped" code={streamError.label} onretry={() => target && openInteractive(target)} healthHref="#/system/health" level={3}>{streamError.message}</ErrorState>
      {/if}
      {#if summary && !ranked.length}
        <EmptyState size="sm" title={summary.indexers ? 'No releases found' : 'No indexers are configured'} icon="search" level={3}>
          {summary.indexers ? (summary.failed ? `${summary.failed} of ${summary.indexers} indexers failed; the others had nothing for ${target?.label ?? 'this'}.` : `None of the indexers had a release for ${target?.label ?? 'this'}.`) : 'Add an indexer to the config, then search again.'}
        </EmptyState>
      {:else if summary && !accepted}
        <p class="note" role="status">None of the {ranked.length} releases passed the profile. The reasons are listed on each; you can grab one anyway where the rules allow it.</p>
      {/if}
      {#if ranked.length}
        <ol class="results" aria-label="Releases, best first">
          {#each ranked as d, i (d.release.id)}
            <li><DecisionPanel decision={toPanel(d, i)} ongrab={(over) => grab(d, over.length > 0)} /></li>
          {/each}
        </ol>
      {/if}
      {#if streaming}
        <div class="pending" aria-busy="true" aria-label="Waiting for indexers"><Skeleton shape="block" height="6.5rem" /></div>
      {/if}
    </div>
  </Drawer>
  <Drawer bind:open={editOpen} title="Edit {show?.title ?? 'series'}" size="md">
    <div class="edit">
      <BoolField label="Monitored" help="Off: corsarr leaves the whole series alone." bind:checked={edit.monitored} />
      <EnumField label="Quality profile" bind:value={edit.profileKey} options={(profiles.data ?? []).map((p) => ({ value: p.key, label: p.name }))} />
      <EnumField label="Apply a monitoring mode" bind:value={edit.monitor} placeholder="Keep the current episode choices" options={monitorModes}
        help={edit.monitor ? `${monitorModes.find((o) => o.value === edit.monitor)?.description} Applied once, to the episodes that exist now.` : 'Pick a mode to reset which episodes are monitored.'} />
      <EnumField label="New seasons" bind:value={edit.monitorNew} options={[{ value: 'all', label: 'Monitor them as they appear' }, { value: 'none', label: 'Leave them unmonitored' }]} />
      <BoolField label="Season folders" help="Put each season in its own folder." bind:checked={edit.seasonFolders} />
      <ListField label="Tags" itemLabel="tag" bind:value={edit.tags} />
    </div>
    {#snippet footer()}
      <Button onclick={() => (editOpen = false)}>Cancel</Button>
      <Button variant="primary" loading={saving} disabled={saving} onclick={saveEdit}>Save</Button>
    {/snippet}
  </Drawer>
{/snippet}

<DetailPage entity={{ ...entity, actions: show ? headActions : undefined, onmonitor: setMonitor, onmenu }} {tabs} syncUrl children={body} {aside}
  history={timeline} historyLoading={history.loading && !history.data} {drawer}
  loading={detail.loading && !show} error={detail.error?.message} errorCode={detail.error?.label} onretry={detail.retry} />

{#if confirmGrab}
  <ConfirmDialog bind:open={confirmOpen} variant="primary" title="Grab {confirmGrab.d.release.title} anyway?" confirmLabel="Grab anyway" loading={grabbing}
    consequences={confirmGrab.reasons.length ? confirmGrab.reasons : ['The release failed a rule that needs your confirmation.']} undoable="You can remove the download from the queue afterwards."
    onconfirm={() => grab(confirmGrab!.d, true)} oncancel={() => (confirmOpen = false)} />
{/if}

{#if show}
  <ConfirmDialog bind:open={deleteOpen} title="Delete {show.title}?" confirmLabel="Delete series" undoable={false}
    consequences={['Removes the series and its episodes from the library and stops searching for them', 'History about it is kept']}
    choices={[
      { value: 'keep', label: 'Keep its files', description: 'The folder stays on disk.' },
      { value: 'files', label: 'Delete its files too', description: 'The series folder is deleted from disk.', consequences: [`Deletes the files under ${show.path}`] },
    ]}
    onconfirm={remove} oncancel={() => (deleteOpen = false)} />
{/if}

<style>
  .seasons { display: grid; gap: var(--space-6); --ep-row: var(--row-h-comfortable); }
  .season { display: grid; gap: var(--space-3); }
  .head { display: flex; align-items: center; gap: var(--space-2) var(--space-3); flex-wrap: wrap; }
  h2 { font-size: var(--text-lg); }
  .counts { font-size: var(--text-sm); color: var(--ink-muted); }
  .bar { flex: 1 1 6rem; min-width: 6rem; max-width: 14rem; }
  .tools { display: flex; gap: var(--space-2); flex-wrap: wrap; margin-left: auto; }
  .ep { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; display: block; }
  .tba { color: var(--ink-subtle); font-style: italic; }
  .overview { display: grid; gap: var(--space-4); max-width: var(--measure); }
  .side { display: grid; gap: var(--space-3); }
  .search, .edit { display: grid; gap: var(--space-4); }
  .results { list-style: none; margin: 0; padding: 0; display: grid; gap: var(--space-3); }
  .note { color: var(--ink-muted); font-size: var(--text-md); }
  @media (width < 48rem) {
    .seasons { --ep-row: 4.5rem; }
    .tools { margin-left: 0; }
  }
</style>
