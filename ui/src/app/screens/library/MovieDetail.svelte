<!--
  Movie detail: facts, file, active download and history; Search now; the interactive
  search drawer streams decisions from GET /movies/{id}/releases (NDJSON) as indexers answer, and a
  grab goes through the override rules (409 asks to confirm, 403 refuses).
-->
<script lang="ts">
  import { onMount } from 'svelte';
  import DetailPage from '../../../lib/components/templates/DetailPage/DetailPage.svelte';
  import Button from '../../../lib/components/atoms/Button/Button.svelte';
  import Skeleton from '../../../lib/components/atoms/Skeleton/Skeleton.svelte';
  import PropertyList from '../../../lib/components/molecules/PropertyList/PropertyList.svelte';
  import QueueCard, { type QueueAction } from '../../../lib/components/molecules/QueueCard/QueueCard.svelte';
  import SourceProgressStrip, { type SourceProgress } from '../../../lib/components/molecules/SourceProgressStrip/SourceProgressStrip.svelte';
  import EmptyState from '../../../lib/components/molecules/EmptyState/EmptyState.svelte';
  import ErrorState from '../../../lib/components/molecules/ErrorState/ErrorState.svelte';
  import Drawer from '../../../lib/components/organisms/Drawer/Drawer.svelte';
  import DecisionPanel, { type Decision as PanelDecision, type DecisionReason } from '../../../lib/components/organisms/DecisionPanel/DecisionPanel.svelte';
  import ConfirmDialog from '../../../lib/components/organisms/ConfirmDialog/ConfirmDialog.svelte';
  import type { TimelineEntry } from '../../../lib/components/organisms/Timeline/Timeline.svelte';
  import type { MonitorState } from '../../../lib/components/molecules/MonitorToggle/MonitorToggle.svelte';
  import { formatAbsolute } from '../../../lib/format/time';
  import { app, profileName } from '../../context.svelte';
  import { Resource } from '../../resource.svelte';
  import { refetchOn } from '../../live.svelte';
  import { ApiError, toApiError } from '../../api/client';
  import type { Decided, Download, HistoryEntry, Movie, ReleaseLine, RootFolder, SearchSummary } from '../../api/types';
  import { actorLabel, availableAt, eventDetail, eventView, isActiveDownload, movieStatus, parseEnvExport, queuedMovieIds, toQueueCard } from '../../model';
  import { actionDone, pausedIds, queueAction } from '../../queue.svelte';
  import type { ScreenProps } from '../../screen';

  let { params = {} }: ScreenProps = $props();
  const { api, live, movies, profiles, toasts, navigate } = app();
  // svelte-ignore state_referenced_locally
  const id = Number(params.id);

  const movie = new Resource((s) => api.get<Movie>(`movies/${id}`, undefined, s));
  const queue = new Resource((s) => api.list<Download>('queue', { limit: 200 }, s));
  const roots = new Resource((s) => api.list<RootFolder>('rootfolders', undefined, s));
  // Workaround: History has no item filter yet, so read the newest 200 events and keep this movie's.
  const history = new Resource(async (s) => (await api.page<HistoryEntry>('history', undefined, 200, undefined, s)).items
    .filter((h) => h.event.items?.some((i) => i.ref.kind === 'movie' && i.ref.id === id)));

  onMount(() => {
    void movie.load();
    void roots.load();
    void queue.load();
    void history.load();
    if (!profiles.data) void profiles.load();
    const mine = (ev: { items?: { ref: { kind: string; id: number } }[] }) => !ev.items || ev.items.some((i) => i.ref.kind === 'movie' && i.ref.id === id);
    const offs = [
      live.on('item.*', (ev) => { if (mine(ev)) void movie.load(true); }),
      live.on('import.completed', (ev) => { if (mine(ev)) void movie.load(true); }),
      refetchOn(live, ['download.*', 'grab.sent', 'import.*'], () => void queue.load(true), 500),
      refetchOn(live, ['grab.*', 'download.failed', 'download.removed', 'import.*', 'item.added', 'blocklist.*'], () => void history.load(true), 800),
      live.onreconnect(() => void movie.load(true)),
    ];
    return () => offs.forEach((off) => off());
  });

  const m = $derived(movie.data);
  const otherRoots = $derived((roots.data ?? []).filter((r) => r.kind === 'movie' && r.path !== m?.rootFolder));
  const now = Date.now();
  const downloads = $derived((queue.data ?? []).filter((d) => isActiveDownload(d) && (d.links ?? []).some((l) => l.item.kind === 'movie' && l.item.id === id)));
  const status = $derived(m ? movieStatus(m, now, queuedMovieIds(queue.data ?? [])) : 'missing');
  const day = (s?: string) => (s ? formatAbsolute(new Date(s)).replace(/,.*$/, '') : undefined);
  const availLabel: Record<string, string> = { announced: 'Announced', inCinemas: 'In cinemas', released: 'Released' };
  const availability = $derived.by(() => {
    if (!m) return '';
    const at = availableAt(m);
    if (at === undefined) return 'Date not known yet';
    if (at <= now) return at === 0 ? 'Available (announced)' : `Available since ${day(new Date(at).toISOString())}`;
    return `Expected ${day(new Date(at).toISOString())}`;
  });

  const entity = $derived(m ? {
    title: m.title, kind: 'movie' as const, art: null,
    meta: [m.year || undefined, m.runtimeMin ? `${Math.floor(m.runtimeMin / 60)}h ${m.runtimeMin % 60}m` : undefined].filter(Boolean).join(' · '),
    monitor: (m.monitored ? 'monitored' : 'unmonitored') as MonitorState,
    status: [{ family: 'item' as const, value: status, detail: downloads[0] ? downloads[0].state : undefined }],
    facts: [
      { label: 'Profile', value: profileName(profiles.data, m.profileKey) },
      { label: 'Availability', value: availability },
      { label: 'Folder', value: m.path },
    ],
    back: { href: '#/library/movies', label: 'Movies' },
    menu: [
      { id: 'search', label: 'Search now', icon: 'search' as const },
      { id: 'interactive', label: 'Interactive search', icon: 'list' as const },
      ...(otherRoots.length ? [{ id: 'move', label: 'Move to another root folder…', icon: 'folder' as const }] : []),
      { id: 'delete', label: 'Delete movie…', icon: 'trash' as const, danger: true, group: 'danger' },
    ],
  } : { title: 'Movie', kind: 'movie' as const });

  async function setMonitor(state: MonitorState) {
    if (!m) return;
    try {
      const next = await api.put<Movie>(`movies/${id}`, { monitored: state === 'monitored' });
      movie.data = next;
      movies.upsert(next);
    } catch (e) {
      toasts.push({ message: 'Monitoring was not changed', detail: toApiError(e).message, tone: 'error' });
    }
  }

  let searching = $state(false);
  async function searchNow() {
    if (searching) return;
    searching = true;
    try {
      const s = await api.post<SearchSummary>(`movies/${id}/search`);
      toasts.push(s.grabbed
        ? { message: `Grabbed ${s.grabbed}`, detail: `${s.accepted} of ${s.releases} releases accepted.`, tone: 'ok', actionLabel: 'Open queue', onaction: () => navigate('/activity/queue') }
        : { message: s.releases ? 'No release was good enough to grab' : 'No releases found', detail: s.failed ? `${s.failed} of ${s.indexers} indexers failed.` : `${s.releases} releases checked across ${s.indexers} indexers.`, tone: s.failed ? 'warn' : 'info',
          actionLabel: s.releases ? 'See why' : undefined, onaction: s.releases ? () => void openInteractive() : undefined });
    } catch (e) {
      toasts.push({ message: 'The search did not run', detail: toApiError(e).message, tone: 'error' });
    } finally {
      searching = false;
    }
  }

  // ── interactive search (drawer) ────────────────────────────────────
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

  async function openInteractive() {
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
      for await (const line of api.stream<ReleaseLine>(`movies/${id}/releases`, ctl.signal)) {
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
        runtimeSeconds: m?.runtimeMin ? m.runtimeMin * 60 : undefined,
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
    grabbing = true;
    try {
      await api.post('grabs', { movieId: id, releaseId: d.release.id, confirm });
      confirmOpen = false;
      drawerOpen = false; // the toast and the download card show on the page
      closeDrawer();
      toasts.push({ message: `Grab sent: ${d.release.title}`, detail: 'Watch it in the queue.', tone: 'ok', actionLabel: 'Open queue', onaction: () => navigate('/activity/queue') });
      void queue.load(true);
    } catch (e) {
      const x = toApiError(e);
      const reasons = ((x.body.decision as { reasons?: { message: string }[] } | undefined)?.reasons ?? []).map((r) => r.message);
      if (x.status === 409) { confirmGrab = { d, reasons }; confirmOpen = true; }
      else {
        confirmOpen = false;
        toasts.push(x.status === 403
          ? { message: 'corsarr will not grab this release', detail: reasons[0] ?? x.message, tone: 'error' }
          : { message: 'The grab was not sent', detail: x.status === 404 ? 'The search results expired. Run the search again.' : x.message, tone: 'error' });
      }
    } finally {
      grabbing = false;
    }
  }

  // ── queue card actions, delete ─────────────────────────────────────────
  async function onqueue(action: QueueAction, did: string) {
    if (!(action in actionDone)) { navigate('/activity/queue'); return; }
    try {
      await queueAction(api, did, action);
      toasts.push({ message: actionDone[action] ?? 'Done', tone: 'ok' });
    } catch (e) {
      toasts.push({ message: 'The download client did not take that', detail: toApiError(e).message, tone: 'error' });
    }
  }

  // ── move to another root folder ────────────────────────────────────────
  const folderName = (path: string) => path.split('/').filter(Boolean).pop() ?? path;
  let moveOpen = $state(false);
  let moving = $state(false);
  async function moveTo(root: string | undefined) {
    if (!m || !root) return;
    const back = m.rootFolder;
    moving = true;
    try {
      const next = await api.put<Movie>(`movies/${id}`, { rootFolder: root });
      movies.upsert(next);
      moveOpen = false;
      toasts.push({ message: `Moved ${next.title}`, detail: `Now in ${next.path}`, tone: 'ok',
        onundo: () => { void api.put<Movie>(`movies/${id}`, { rootFolder: back }).then((b) => { movies.upsert(b); void movie.load(true); }); } });
      void movie.load(true);
    } catch (e) {
      toasts.push({ message: 'The movie was not moved', detail: toApiError(e).message, tone: 'error' });
    } finally {
      moving = false;
    }
  }

  let deleteOpen = $state(false);
  async function remove(choice: string | undefined) {
    deleteOpen = false;
    try {
      await api.del(`movies/${id}`, { deleteFiles: choice === 'files' });
      movies.remove(id);
      toasts.push({ message: `Deleted ${m?.title ?? 'the movie'}`, tone: 'ok' });
      navigate('/library/movies');
    } catch (e) {
      toasts.push({ message: 'The movie was not deleted', detail: toApiError(e).message, tone: 'error' });
    }
  }
  function onmenu(item: string) {
    if (item === 'search') void searchNow();
    else if (item === 'interactive') void openInteractive();
    else if (item === 'delete') deleteOpen = true;
    else if (item === 'move') moveOpen = true;
  }

  const timeline = $derived<TimelineEntry[]>((history.data ?? []).map((h) => {
    const v = eventView(h.event);
    return { id: String(h.id), at: h.event.occurredAt, kind: 'observation', title: v.label, status: { family: v.family, value: v.value }, actor: actorLabel(h.event.actor), detail: eventDetail(h.event) };
  }));

  function onkeydown(e: KeyboardEvent) {
    const t = e.target as HTMLElement | null;
    if (e.defaultPrevented || e.ctrlKey || e.metaKey || e.altKey || (t && (t.isContentEditable || /^(INPUT|TEXTAREA|SELECT)$/.test(t.tagName)))) return;
    if (document.querySelector('dialog[open]') || !m) return;
    if (e.key === 's') { e.preventDefault(); void searchNow(); }
    else if (e.key === 'i') { e.preventDefault(); void openInteractive(); }
    else if (e.key === 'm') { e.preventDefault(); void setMonitor(m.monitored ? 'unmonitored' : 'monitored'); }
  }

  const tabs = $derived([{ id: 'overview', label: 'Overview' }, { id: 'files', label: 'Files', count: m?.fileId ? 1 : 0 }]);
</script>

<svelte:window {onkeydown} />

{#snippet headActions()}
  <Button icon="search" loading={searching} disabled={searching} onclick={searchNow}>Search now</Button>
  <Button variant="primary" icon="list" onclick={openInteractive}>Interactive search</Button>
{/snippet}

{#snippet body(tab: string)}
  {#if !m}
    <Skeleton lines={3} />
  {:else if tab === 'overview'}
    <div class="overview">
      <PropertyList label="Release" items={[
        { key: 'cinema', label: 'In cinemas', value: day(m.inCinemas) ?? '—' },
        { key: 'digital', label: 'Digital', value: day(m.digital) ?? '—' },
        { key: 'physical', label: 'Disc', value: day(m.physical) ?? '—' },
        { key: 'min', label: 'Minimum availability', value: availLabel[m.minAvailability] ?? m.minAvailability },
        { key: 'added', label: 'Added', value: formatAbsolute(new Date(m.addedAt)) },
        ...(m.originalTitle && m.originalTitle !== m.title ? [{ key: 'orig', label: 'Original title', value: m.originalTitle }] : []),
      ]} />
    </div>
  {:else if m.fileId}
    <PropertyList label="File" items={[
      { key: 'folder', label: 'Folder', value: m.path, mono: true, copy: true },
      { key: 'file', label: 'File record', value: `#${m.fileId}` },
    ]} />
  {:else}
    <EmptyState size="sm" title="No file yet" icon="file" level={3}>
      {downloads.length ? 'A download is running; the file appears here once it is imported.' : 'Search now, or pick a release with interactive search.'}
    </EmptyState>
  {/if}
{/snippet}

{#snippet aside()}
  {#if downloads.length}
    <section class="side">
      <h2>Downloading</h2>
      {#each downloads as d (d.id)}
        <QueueCard download={{ ...toQueueCard(d, movies.title), paused: pausedIds.has(String(d.id)) }} actions={['pause', 'resume', 'top', 'bottom', 'kill']} onaction={onqueue} />
      {/each}
    </section>
  {/if}
  {#if m}
    <PropertyList label="Identifiers" items={[
      { key: 'tmdb', label: 'TMDB', value: String(m.tmdbId), href: `https://www.themoviedb.org/movie/${m.tmdbId}` },
      ...(m.imdbId ? [{ key: 'imdb', label: 'IMDb', value: m.imdbId, href: `https://www.imdb.com/title/${m.imdbId}/` }] : []),
    ]} />
  {/if}
{/snippet}

{#snippet drawer()}
  <Drawer bind:open={drawerOpen} title="Interactive search" subtitle="{m?.title ?? ''}{m?.year ? ` (${m.year})` : ''} · {profileName(profiles.data, m?.profileKey ?? '')} profile" size="lg" onclose={closeDrawer}>
    <div class="search">
      {#if sources.length}<SourceProgressStrip {sources} label="Indexers" />{/if}
      {#if streamError}
        <ErrorState size="sm" title="The search stopped" code={streamError.label} onretry={openInteractive} healthHref="#/system/health" level={3}>{streamError.message}</ErrorState>
      {/if}
      {#if summary && !ranked.length}
        <EmptyState size="sm" title={summary.indexers ? 'No releases found' : 'No indexers are configured'} icon="search" level={3}>
          {summary.indexers ? (summary.failed ? `${summary.failed} of ${summary.indexers} indexers failed; the others had nothing for this movie.` : 'None of the indexers had a release for this movie.') : 'Add an indexer to the config, then search again.'}
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
{/snippet}

<DetailPage entity={{ ...entity, actions: m ? headActions : undefined, onmonitor: setMonitor, onmenu }} {tabs} syncUrl children={body} {aside}
  history={timeline} historyLoading={history.loading && !history.data} {drawer}
  loading={movie.loading && !m} error={movie.error?.message} errorCode={movie.error?.label} onretry={movie.retry} />

{#if confirmGrab}
  <ConfirmDialog bind:open={confirmOpen} variant="primary" title="Grab {confirmGrab.d.release.title} anyway?" confirmLabel="Grab anyway" loading={grabbing}
    consequences={confirmGrab.reasons.length ? confirmGrab.reasons : ['The release failed a rule that needs your confirmation.']} undoable="You can remove the download from the queue afterwards."
    onconfirm={() => grab(confirmGrab!.d, true)} oncancel={() => (confirmOpen = false)} />
{/if}

{#if m && otherRoots.length}
  <ConfirmDialog bind:open={moveOpen} variant="primary" title="Move {m.title} to another root folder?" confirmLabel="Move folder" loading={moving}
    undoable="Undo moves it back."
    consequences={['Moves the movie folder and every file in it', 'The media server rescans both folders']}
    choices={otherRoots.map((r) => ({ value: r.path, label: r.path, consequences: [`${m!.path} → ${r.path}/${folderName(m!.path)}`] }))}
    choice={otherRoots[0].path} onconfirm={moveTo} oncancel={() => (moveOpen = false)} />
{/if}

{#if m}
  <ConfirmDialog bind:open={deleteOpen} title="Delete {m.title}?" confirmLabel="Delete movie" undoable={false}
    consequences={['Removes the movie from the library and stops searching for it', 'History about it is kept']}
    choices={[
      { value: 'keep', label: 'Keep its files', description: 'The folder stays on disk.' },
      { value: 'files', label: 'Delete its files too', description: 'The movie folder is deleted from disk.', consequences: [`Deletes the files under ${m.path}`] },
    ]}
    onconfirm={remove} oncancel={() => (deleteOpen = false)} />
{/if}

<style>
  .overview { display: grid; gap: var(--space-4); max-width: var(--measure); }
  .side { display: grid; gap: var(--space-3); }
  h2 { font-size: var(--text-lg); }
  .search { display: grid; gap: var(--space-4); }
  .results { list-style: none; margin: 0; padding: 0; display: grid; gap: var(--space-3); }
  .note { color: var(--ink-muted); font-size: var(--text-md); }
</style>
