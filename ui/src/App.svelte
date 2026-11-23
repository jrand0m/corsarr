<!--
  App root: session (first run, sign-in), the shell with live global status, the command
  palette and the current hash route, whose area loads as its own chunk. One API
  client, one SSE connection and one toast queue live here and reach screens through context.
-->
<script lang="ts">
  import type { Component } from 'svelte';
  import AppShell, { shellNav, type ShellStatus } from './lib/components/templates/AppShell/AppShell.svelte';
  import type { PaletteItem } from './lib/components/organisms/CommandPalette/CommandPalette.svelte';
  import EmptyState from './lib/components/molecules/EmptyState/EmptyState.svelte';
  import ErrorState from './lib/components/molecules/ErrorState/ErrorState.svelte';
  import Skeleton from './lib/components/atoms/Skeleton/Skeleton.svelte';
  import Button from './lib/components/atoms/Button/Button.svelte';
  import { ToastQueue } from './lib/toast/toasts.svelte';
  import { applyTheme, readTheme, type ThemePreference } from './lib/theme';
  import { currentRoute, inArea } from './lib/route/hash';
  import type { MediaKind } from './lib/tokens';
  import { ApiError, Client, toApiError } from './app/api/client';
  import type { Download, IntegrationHealth, QualityProfile, SystemStatus } from './app/api/types';
  import { LiveEvents, refetchOn } from './app/live.svelte';
  import { MovieIndex, navigateTo, setApp } from './app/context.svelte';
  import { Resource } from './app/resource.svelte';
  import { isAccess, resolve } from './app/router';
  import { areaLoaders } from './app/areas';
  import { healthRank, isActiveDownload, parseEnvExport } from './app/model';

  let theme = $state<ThemePreference>(readTheme());
  $effect(() => applyTheme(theme));

  let path = $state(currentRoute().path);
  let returnTo = '/';
  /** unknown until /firstrun and /auth/me answer. */
  let session = $state<'unknown' | 'setup' | 'signin' | 'in'>('unknown');
  let bootError = $state<ApiError | undefined>();

  function navigate(p: string, params?: URLSearchParams) {
    navigateTo(p, params);
  }

  const api = new Client({
    onunauthorized: (code) => {
      if (session !== 'in') return;
      live.stop();
      session = code === 'setup_required' ? 'setup' : 'signin';
      expired = code !== 'setup_required';
      returnTo = path;
      navigate(code === 'setup_required' ? '/setup' : '/login');
    },
  });
  const live = new LiveEvents('api/v1/events');
  const toasts = new ToastQueue();
  const movies = new MovieIndex(api);
  const profiles = new Resource((s) => api.list<QualityProfile>('qualityprofiles', undefined, s));
  const status = new Resource((s) => api.get<SystemStatus>('system/status', undefined, s));
  const health = new Resource((s) => api.list<IntegrationHealth>('health/integrations', undefined, s));
  const queue = new Resource((s) => api.list<Download>('queue', { limit: 200 }, s));
  const exportView = new Resource(async () => parseEnvExport(await api.text('config/export', { format: 'env', full: true })));
  let expired = $state(false);

  setApp({ api, live, toasts, movies, profiles, status, navigate });

  // ── session ──────────────────────────────────────────────────────────────
  async function boot() {
    bootError = undefined;
    try {
      const fr = await api.get<{ required: boolean }>('firstrun');
      if (fr.required) {
        session = 'setup';
        if (path !== '/setup') navigate('/setup');
        return;
      }
      const me = await api.get<{ authenticated: boolean }>('auth/me');
      if (!me.authenticated) {
        session = 'signin';
        if (!isAccess(path)) returnTo = path;
        if (path !== '/login') navigate('/login');
        return;
      }
      signedIn();
    } catch (e) {
      bootError = toApiError(e);
    }
  }

  function signedIn() {
    session = 'in';
    expired = false;
    live.start();
    void status.load();
    void profiles.load();
    void health.load();
    void queue.load();
    void exportView.load();
  }
  const afterSignIn = () => (returnTo && !isAccess(returnTo) ? returnTo : '/');

  async function signOut() {
    await api.post('auth/logout').catch(() => {});
    live.stop();
    session = 'signin';
    returnTo = '/';
    navigate('/login');
  }

  $effect(() => {
    void boot();
  });

  $effect(() => {
    if (session !== 'in') return;
    const offs = [
      refetchOn(live, ['health.changed'], () => void health.load(true)),
      refetchOn(live, ['download.state_changed', 'download.completed', 'download.failed', 'download.removed', 'grab.sent', 'import.*'], () => void queue.load(true), 600),
      refetchOn(live, ['item.added', 'item.updated', 'item.deleted', 'import.completed', 'file.deleted'], () => { if (movies.loaded) void movies.load(true); }, 800),
      refetchOn(live, ['config.applied', 'mediakind.mode_changed'], () => { void status.load(true); void exportView.load(true); }),
    ];
    return () => offs.forEach((off) => off());
  });

  // ── global status ─────────────────────────────────────────────────
  const kinds = $derived<MediaKind[]>(status.data?.enabledKinds?.length ? status.data.enabledKinds : ['movie']);
  const shellStatus = $derived.by<ShellStatus>(() => {
    const problems = (health.data ?? []).filter((h) => healthRank[h.status] < 4);
    const downloads = (queue.data ?? []).filter(isActiveDownload);
    const shadow = Object.entries(exportView.data?.kinds ?? {}).filter(([, m]) => m === 'shadow').map(([k]) => k);
    return {
      unhealthy: problems.length,
      healthTone: problems.some((h) => healthRank[h.status] <= 1) ? 'error' : problems.length ? 'warn' : undefined,
      active: downloads.filter((d) => d.state !== 'import-blocked').length,
      blocked: downloads.filter((d) => d.state === 'import-blocked').length,
      shadowKinds: shadow,
    };
  });
  const offlineAt = $derived(live.offlineSince ? new Date(live.offlineSince).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', hour12: false }) : undefined);

  // ── command palette: actions and every movie ──────────────────────
  const paletteItems = $derived<PaletteItem[]>([
    { id: 'action:add-movie', label: 'Add movie', group: 'Actions', icon: 'plus', keywords: ['new', 'film'] },
    { id: 'action:queue', label: 'Open the queue', group: 'Actions', icon: 'activity', keys: ['g', 'q'] },
    ...movies.rows.map((m) => ({ id: `movie:${m.id}`, label: m.title, subtitle: `Movie · ${m.year || 'year unknown'}`, group: 'Movies', kind: 'movie' as const, keywords: [m.originalTitle ?? ''] })),
  ]);
  function paletteSelect(item: PaletteItem) {
    if (item.id === 'action:add-movie') navigate('/library/movies/add');
    else if (item.id === 'action:queue') navigate('/activity/queue');
    else if (item.id.startsWith('movie:')) navigate(`/library/movies/${item.id.slice(6)}`);
  }

  // ── routing ─────────────────────────────────────────────────────────────
  const resolved = $derived(resolve(path));
  $effect(() => {
    if (resolved && 'redirect' in resolved) history.replaceState(history.state, '', `#${resolved.redirect}${location.hash.includes('?') ? location.hash.slice(location.hash.indexOf('?')) : ''}`);
    if (resolved && 'redirect' in resolved) path = resolved.redirect;
  });
  // Guard: signed-out visitors only see access screens; signed-in ones skip them.
  $effect(() => {
    if (session === 'setup' && path !== '/setup') navigate('/setup');
    else if (session === 'signin' && path !== '/login') { if (!isAccess(path)) returnTo = path; navigate('/login'); }
    else if (session === 'in' && isAccess(path)) navigate(afterSignIn());
  });

  let screen = $state<Component<Record<string, unknown>> | undefined>();
  let screenKey = $state('');
  let loadError = $state<ApiError | undefined>();
  $effect(() => {
    const r = resolved;
    if (!r || !('area' in r)) { screen = undefined; return; }
    const key = `${r.area}/${r.screen}`;
    if (key === screenKey && screen) return;
    loadError = undefined;
    areaLoaders[r.area]()
      .then((m) => { screen = m.screens[r.screen]; screenKey = key; })
      .catch(() => { loadError = new ApiError(0, { code: 'chunk', message: 'This screen could not be loaded. Reload the page to fetch the current version.' }); });
  });
  const match = $derived(resolved && 'area' in resolved ? resolved : undefined);
  const params = $derived(match?.params ?? {});

  const laterTitle = $derived.by(() => {
    for (const a of shellNav(kinds)) {
      const c = a.children?.find((l) => inArea(path, l.path));
      if (c) return c.label === a.label ? a.label : `${a.label} › ${c.label}`;
      if (inArea(path, a.path)) return a.label;
    }
    return 'Not found';
  });
</script>

<svelte:window onhashchange={() => (path = currentRoute().path)} />

{#snippet routed()}
  {#if loadError}
    <ErrorState title="Couldn't open this screen" code={loadError.label} onretry={() => location.reload()} level={2}>{loadError.message}</ErrorState>
  {:else if match && screen && screenKey === `${match.area}/${match.screen}`}
    {@const Screen = screen}
    {#key path}<Screen {params} screen={match.screen} {expired} onsignedin={signedIn} />{/key}
  {:else if match}
    <div class="loading" aria-busy="true" aria-label="Loading"><Skeleton shape="text" width="12rem" height="1.75rem" /><Skeleton shape="block" height="18rem" /></div>
  {:else if resolved && 'later' in resolved}
    <h1 class="title">{laterTitle}</h1>
    <EmptyState title="Not in this version yet" icon="tray" level={2}>
      This screen arrives in a later release. Movies, the queue, history, health and settings are ready now.
      {#snippet actions()}<Button href="#/">Go to the dashboard</Button>{/snippet}
    </EmptyState>
  {:else}
    <h1 class="title">Not found</h1>
    <EmptyState title="There is no screen at this address" icon="help-circle" level={2}>
      Check the link, or open the command palette with Ctrl K to find what you need.
      {#snippet actions()}<Button href="#/">Go to the dashboard</Button>{/snippet}
    </EmptyState>
  {/if}
{/snippet}

{#if bootError}
  <main class="boot"><ErrorState title="Couldn't reach corsarr" code={bootError.label} onretry={boot} level={2}>{bootError.message}</ErrorState></main>
{:else if session === 'unknown'}
  <main class="boot" aria-busy="true" aria-label="Starting"></main>
{:else if session !== 'in' || isAccess(path)}
  {@render routed()}
{:else}
  <AppShell {kinds} {path} bind:theme {toasts} status={shellStatus} connection={live.connection} offlineSince={offlineAt}
    onreconnect={() => live.reconnect()} onsignout={signOut} {paletteItems} paletteLoading={movies.res.loading && !movies.loaded}
    onpalettequery={() => movies.ensure()} onpaletteselect={paletteSelect}>
    {@render routed()}
  </AppShell>
{/if}

<style>
  .title { font-size: var(--text-xl); margin-bottom: var(--space-6); }
  .loading { display: grid; gap: var(--space-4); }
  .boot { min-height: 100dvh; display: grid; place-items: center; padding: var(--space-4); background: var(--surface-ground); }
</style>
