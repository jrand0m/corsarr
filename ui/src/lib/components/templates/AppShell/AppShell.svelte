<!--
  AppShell (template): the oxide frame around every authenticated screen.
  Desktop: side rail with the nav (only enabled media kinds), top bar with global search, global
  status, theme and sign-out. Phone (< 48rem): top bar, bottom bar Home · Library · Activity ·
  Search · More, and More opens the whole nav as a collapsible sheet. Mounts the command palette
  ("/" and Ctrl/Cmd+K, `g` then a letter to go somewhere), the toast region once, a skip
  link, the main landmark and the live-connection banner. No data fetching: routes pass state in.
-->
<script lang="ts" module>
  import type { IconName } from '../../atoms/Icon/icons';
  import type { MediaKind } from '../../../tokens';

  export interface NavLink { path: string; label: string; icon?: IconName; kind?: MediaKind }
  export interface NavArea extends NavLink { icon: IconName; children?: NavLink[]; key?: string }

  export const kindPaths: Record<MediaKind, { path: string; label: string }> = {
    tv: { path: '/library/tv', label: 'TV' },
    movie: { path: '/library/movies', label: 'Movies' },
    music: { path: '/library/music', label: 'Music' },
    book: { path: '/library/books', label: 'Books' },
  };

  /** The app navigation, with Library showing only the enabled kinds. `key` is the `g` go-to letter. */
  export function shellNav(kinds: MediaKind[]): NavArea[] {
    const order: MediaKind[] = ['tv', 'movie', 'music', 'book'];
    const library: NavLink[] = order.filter((k) => kinds.includes(k)).map((k) => ({ ...kindPaths[k], kind: k }));
    return [
      { path: '/', label: 'Dashboard', icon: 'home', key: 'd' },
      { path: '/library', label: 'Library', icon: 'library', key: 'l', children: [
        ...library, { path: '/library/unmapped', label: 'Unmapped files' }, { path: '/library/recycle', label: 'Recycle area' },
      ] },
      { path: '/wanted', label: 'Wanted', icon: 'target', key: 'w', children: [
        { path: '/wanted/missing', label: 'Missing' }, { path: '/wanted/cutoff', label: 'Cutoff unmet' }, { path: '/wanted/calendar', label: 'Calendar' },
      ] },
      { path: '/activity', label: 'Activity', icon: 'activity', children: [
        { path: '/activity/queue', label: 'Queue' }, { path: '/activity/history', label: 'History' },
        { path: '/activity/blocklist', label: 'Blocklist' }, { path: '/activity/automation', label: 'Automation' },
      ] },
      { path: '/search', label: 'Search', icon: 'search', children: [
        { path: '/search', label: 'Releases' }, { path: '/search/parse', label: 'Parse tester' },
      ] },
      { path: '/settings', label: 'Settings', icon: 'settings', key: 's' },
      { path: '/system', label: 'System', icon: 'server', key: 'y', children: [
        { path: '/system/health', label: 'Health' }, { path: '/system/tasks', label: 'Tasks' }, { path: '/system/logs', label: 'Logs' },
        { path: '/system/backups', label: 'Backups' }, { path: '/system/migration', label: 'Migration' }, { path: '/system/about', label: 'About' },
      ] },
    ];
  }

  /** `g` then this letter goes to the path. */
  export const goKeys: Record<string, string> = {
    d: '/', l: '/library', w: '/wanted/missing', q: '/activity/queue', h: '/activity/history', s: '/settings', y: '/system/health',
  };

  export interface ShellStatus {
    unhealthy: number;
    healthTone?: 'warn' | 'error';
    active: number;
    blocked: number;
    shadowKinds?: string[];
  }
  export type Connection = 'live' | 'reconnecting' | 'offline';
</script>

<script lang="ts">
  import type { Snippet } from 'svelte';
  import Icon from '../../atoms/Icon/Icon.svelte';
  import IconButton from '../../atoms/IconButton/IconButton.svelte';
  import Kbd from '../../atoms/Kbd/Kbd.svelte';
  import Button from '../../atoms/Button/Button.svelte';
  import GlobalStatus from '../../molecules/GlobalStatus/GlobalStatus.svelte';
  import Banner from '../../molecules/Banner/Banner.svelte';
  import ActionMenu, { type ActionItem } from '../../molecules/ActionMenu/ActionMenu.svelte';
  import CommandPalette, { type PaletteItem } from '../../organisms/CommandPalette/CommandPalette.svelte';
  import ToastRegion from '../../organisms/ToastRegion/ToastRegion.svelte';
  import { ToastQueue } from '../../../toast/toasts.svelte';
  import { formatHash, inArea } from '../../../route/hash';
  import type { ThemePreference } from '../../../theme';

  interface Props {
    /** Enabled media kinds; Library lists only these. */
    kinds?: MediaKind[];
    /** Current route path ("/library/movies/42"); marks the nav. */
    path?: string;
    status: ShellStatus;
    connection?: Connection;
    /** When live updates stopped, for the offline banner. */
    offlineSince?: string;
    onreconnect?: () => void;
    theme?: ThemePreference;
    ontheme?: (pref: ThemePreference) => void;
    onsignout?: () => void;
    /** Extra palette entries (items, settings objects, actions in context); screens are added from the nav. */
    paletteItems?: PaletteItem[];
    paletteRecent?: string[];
    paletteLoading?: boolean;
    paletteOpen?: boolean;
    onpalettequery?: (query: string) => void;
    onpaletteselect?: (item: PaletteItem) => void;
    /** Go to a path; default sets the hash. */
    onnavigate?: (path: string) => void;
    /** The one toast queue of the app. */
    toasts?: ToastQueue;
    /** Replaces the built-in connection banner (or adds one, such as "Restart needed"). */
    banner?: Snippet;
    children?: Snippet;
  }
  let {
    kinds = ['tv', 'movie'], path = '/', status, connection = 'live', offlineSince, onreconnect,
    theme = $bindable('system'), ontheme, onsignout, paletteItems = [], paletteRecent = [], paletteLoading = false,
    paletteOpen = $bindable(false), onpalettequery, onpaletteselect, onnavigate, toasts = new ToastQueue(), banner, children,
  }: Props = $props();

  const id = $props.id();
  const nav = $derived(shellNav(kinds));
  const areaOf = (p: string) => nav.find((a) => (a.children ?? []).some((c) => inArea(p, c.path)) || inArea(p, a.path));
  const currentArea = $derived(areaOf(path));
  const linkHref = (a: NavArea) => formatHash(a.children?.[0]?.path ?? a.path);
  // A child whose path equals its area's ("/search") is current only on an exact match.
  const childCurrent = (a: NavArea, c: NavLink) => (c.path === a.path ? path === c.path : inArea(path, c.path));

  let menuOpen = $state(false);
  const quick = $derived([
    { path: '/', label: 'Home', icon: 'home' as IconName, area: '/' },
    { path: linkHref(nav[1]).slice(1), label: 'Library', icon: 'library' as IconName, area: '/library' },
    { path: '/activity/queue', label: 'Activity', icon: 'activity' as IconName, area: '/activity' },
  ]);

  const screenItems = $derived<PaletteItem[]>(
    nav.flatMap((a) => (a.children?.length ? a.children.map((c) => ({ ...c, area: a })) : [{ ...a, area: a }])).map((l) => ({
      id: `nav:${l.path}`, label: l.area.children && l.label !== l.area.label ? `${l.area.label} › ${l.label}` : l.label, group: 'Screens',
      icon: l.icon ?? l.area.icon, kind: l.kind,
      keys: Object.entries(goKeys).find(([, p]) => p === l.path)?.[0] ? ['g', Object.entries(goKeys).find(([, p]) => p === l.path)![0]] : undefined,
    })),
  );
  const allItems = $derived([...paletteItems, ...screenItems]);

  function go(p: string) {
    menuOpen = false;
    if (onnavigate) onnavigate(p);
    else if (typeof location !== 'undefined') location.hash = formatHash(p);
  }
  function select(item: PaletteItem) {
    if (item.id.startsWith('nav:')) go(item.id.slice(4));
    else onpaletteselect?.(item);
  }
  function openPalette() {
    menuOpen = false;
    paletteOpen = true;
  }

  const themeItems = $derived<ActionItem[]>([
    ...([
      { id: 'theme:system', label: 'Match system theme', icon: 'sliders' },
      { id: 'theme:light', label: 'Use light theme', icon: 'sun' },
      { id: 'theme:dark', label: 'Use dark theme', icon: 'moon' },
    ] satisfies ActionItem[]).filter((i) => i.id !== `theme:${theme}`),
    ...(onsignout ? [{ id: 'signout', label: 'Sign out', icon: 'logout' as IconName, group: 'session' }] : []),
  ]);
  function onaccount(itemId: string) {
    if (itemId === 'signout') { onsignout?.(); return; }
    theme = itemId.slice(6) as ThemePreference;
    ontheme?.(theme);
  }

  // Global keys: "/" (unless a local filter took it), Ctrl/Cmd+K, "?" and "g" then a letter.
  let pendingG = 0;
  function typing(t: EventTarget | null) {
    const el = t as HTMLElement | null;
    return !!el && (el.isContentEditable || /^(INPUT|TEXTAREA|SELECT)$/.test(el.tagName));
  }
  function onkeydown(e: KeyboardEvent) {
    if (e.defaultPrevented) return;
    const mod = e.ctrlKey || e.metaKey;
    if (mod && e.key.toLowerCase() === 'k') { e.preventDefault(); paletteOpen = !paletteOpen; return; }
    if (mod || e.altKey || typing(e.target) || paletteOpen) return;
    if (e.key === 'Escape' && menuOpen) { menuOpen = false; return; }
    if (e.key === '/' || e.key === '?') { e.preventDefault(); openPalette(); return; }
    if (pendingG && Date.now() - pendingG < 1500 && goKeys[e.key]) {
      e.preventDefault();
      pendingG = 0;
      const target = goKeys[e.key];
      go(target === '/library' ? linkHref(nav[1]).slice(1) : target);
      return;
    }
    pendingG = e.key === 'g' ? Date.now() : 0;
  }
</script>

<svelte:window {onkeydown} />

{#snippet navList(where: 'rail' | 'sheet')}
  <ul class="areas">
    {#each nav as a (a.path)}
      {@const here = currentArea?.path === a.path}
      <li>
        <a class="area" class:here href={linkHref(a)} aria-current={here && !a.children ? 'page' : undefined} onclick={() => (menuOpen = false)}>
          <Icon name={a.icon} /><span>{a.label}</span>
        </a>
        {#if a.children && (here || where === 'sheet')}
          <ul class="sub" aria-label={a.label}>
            {#each a.children as c (c.path)}
              <li>
                <a href={formatHash(c.path)} class:here={childCurrent(a, c)} aria-current={childCurrent(a, c) ? 'page' : undefined} onclick={() => (menuOpen = false)}>
                  {c.label}
                </a>
              </li>
            {/each}
          </ul>
        {/if}
      </li>
    {/each}
  </ul>
{/snippet}

<div class="shell">
  <a class="skip" href="#{id}-main">Skip to content</a>

  <aside class="rail" aria-label="Sidebar">
    <a class="brand" href="#/" aria-label="corsarr, dashboard">
      <svg class="mark" viewBox="0 0 20 20" aria-hidden="true"><path d="M2 11h10v6H2zM8 4h10v6H8z" /></svg>
      <span>corsarr</span>
    </a>
    <nav aria-label="Main">{@render navList('rail')}</nav>
  </aside>

  <header class="top">
    <a class="brand phone-only" href="#/" aria-label="corsarr, dashboard">
      <svg class="mark" viewBox="0 0 20 20" aria-hidden="true"><path d="M2 11h10v6H2zM8 4h10v6H8z" /></svg>
    </a>
    <button type="button" class="search" onclick={openPalette} aria-keyshortcuts="/ Control+K Meta+K" aria-haspopup="dialog">
      <Icon name="search" /><span class="search-label">Search titles, screens and actions</span>
      <span class="search-keys" aria-hidden="true"><Kbd keys={['/']} /></span>
    </button>
    <div class="status">
      <span class="wide-only"><GlobalStatus {...status} {connection} healthHref="#/system/health" queueHref="#/activity/queue" /></span>
      <span class="phone-only"><GlobalStatus {...status} {connection} compact healthHref="#/system/health" queueHref="#/activity/queue" /></span>
    </div>
    <span class="account"><ActionMenu items={themeItems} label="Theme and session" icon="user" onselect={onaccount} /></span>
  </header>

  <div class="sheet" class:open={menuOpen} id="{id}-sheet">
    <nav aria-label="All screens">{@render navList('sheet')}</nav>
  </div>

  <div class="body">
    {#if banner}
      <div class="banner">{@render banner()}</div>
    {:else if connection !== 'live'}
      <div class="banner">
        <Banner preset="disconnected" title={connection === 'reconnecting' ? 'Reconnecting to live updates' : 'Live updates stopped'}>
          {connection === 'reconnecting'
            ? 'What you see stays on screen and may be out of date until the connection is back.'
            : `What you see was current${offlineSince ? ` at ${offlineSince}` : ''}. Pages refresh when the connection is back.`}
          {#snippet actions()}{#if onreconnect && connection === 'offline'}<Button size="sm" icon="refresh" onclick={onreconnect}>Reconnect</Button>{/if}{/snippet}
        </Banner>
      </div>
    {/if}
    <main id="{id}-main" tabindex="-1">
      {#if children}{@render children()}{/if}
    </main>
  </div>

  <nav class="quick" aria-label="Quick">
    <ul>
      {#each quick as q (q.label)}
        {@const here = !menuOpen && (q.area === '/' ? path === '/' : inArea(path, q.area))}
        <li><a href={formatHash(q.path)} class:here aria-current={here ? 'page' : undefined} onclick={() => (menuOpen = false)}><Icon name={q.icon} /><span>{q.label}</span></a></li>
      {/each}
      <li><button type="button" onclick={openPalette}><Icon name="search" /><span>Search</span></button></li>
      <li>
        <button type="button" class:here={menuOpen} aria-expanded={menuOpen} aria-controls="{id}-sheet" onclick={() => (menuOpen = !menuOpen)}>
          <Icon name={menuOpen ? 'x' : 'menu'} /><span>{menuOpen ? 'Close' : 'More'}</span>
        </button>
      </li>
    </ul>
  </nav>

  <CommandPalette bind:open={paletteOpen} items={allItems} recent={paletteRecent} loading={paletteLoading} onquery={onpalettequery} onselect={select} />
  <ToastRegion queue={toasts} />
</div>

<style>
  .shell {
    --top-h: 3.5rem;
    --quick-h: 4rem;
    min-height: 100dvh;
    display: grid;
    grid-template-columns: var(--rail-w) minmax(0, 1fr);
    grid-template-rows: var(--top-h) 1fr;
    grid-template-areas: 'rail top' 'rail body';
    background: var(--surface-ground);
    color: var(--ink);
  }
  .skip {
    position: fixed; inset: var(--space-2) auto auto var(--space-2); z-index: var(--z-tooltip);
    padding: var(--space-2) var(--space-3); border-radius: var(--radius-md);
    background: var(--surface-overlay); color: var(--accent-ink); box-shadow: var(--shadow-md);
    transform: translateY(-200%);
  }
  .skip:focus-visible { transform: none; }

  /* Rail (desktop) */
  .rail {
    grid-area: rail; position: sticky; top: 0; height: 100dvh; overflow-y: auto; overscroll-behavior: contain;
    display: flex; flex-direction: column; gap: var(--space-4); padding: 0 var(--space-2) var(--space-4);
    background: var(--shell-bg); color: var(--shell-ink); border-right: var(--border-w) solid var(--shell-line);
    scrollbar-color: var(--shell-line) transparent;
  }
  .brand {
    display: flex; align-items: center; gap: var(--space-2); min-height: var(--top-h); padding: 0 var(--space-2);
    color: var(--shell-ink); text-decoration: none; font-size: var(--text-lg); font-weight: var(--weight-semibold); letter-spacing: -0.01em;
  }
  .mark { width: 1.25rem; height: 1.25rem; fill: none; stroke: currentColor; stroke-width: 1.75; stroke-linejoin: round; flex: none; }
  .areas, .sub { list-style: none; margin: 0; padding: 0; display: grid; gap: var(--space-0-5); }
  .sub { margin: var(--space-0-5) 0 var(--space-2); }
  .area, .sub a {
    display: flex; align-items: center; gap: var(--space-2); min-height: var(--control-h-md); padding: 0 var(--space-2);
    border-radius: var(--radius-md); color: var(--shell-ink); text-decoration: none; font-size: var(--text-md);
    transition: background-color var(--dur-fast) var(--ease-out);
  }
  .area :global(svg) { color: var(--shell-ink-muted); }
  .sub a { min-height: var(--control-h-sm); padding-left: calc(var(--space-2) + var(--icon-md) + var(--space-2)); color: var(--shell-ink-muted); }
  .area:hover, .sub a:hover { background: var(--shell-hover); color: var(--shell-ink); }
  .area.here { font-weight: var(--weight-semibold); }
  .area.here :global(svg) { color: var(--shell-ink); }
  .area[aria-current='page'], .sub a[aria-current='page'] { background: var(--shell-active); color: var(--shell-ink); font-weight: var(--weight-semibold); }

  /* Top bar */
  .top {
    grid-area: top; position: sticky; top: 0; z-index: var(--z-sticky);
    display: flex; align-items: center; gap: var(--space-3); padding: 0 var(--space-4);
    background: var(--shell-bg); color: var(--shell-ink); border-bottom: var(--border-w) solid var(--shell-line);
  }
  .search {
    display: flex; align-items: center; gap: var(--space-2); flex: 0 1 26rem; min-width: 0; height: var(--control-h-md);
    padding: 0 var(--space-2) 0 var(--space-3); border-radius: var(--radius-md); cursor: pointer;
    font: inherit; font-size: var(--text-md); color: var(--shell-ink-muted); text-align: start;
    background: var(--shell-hover); border: var(--border-w) solid var(--shell-line);
    transition: background-color var(--dur-fast) var(--ease-out), color var(--dur-fast) var(--ease-out);
  }
  .search:hover { background: var(--shell-active); color: var(--shell-ink); }
  .search-label { flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .search-keys { flex: none; }
  .status { margin-left: auto; display: flex; min-width: 0; }
  .account { display: flex; }
  .account :global(button) { color: var(--shell-ink-muted); }
  .account :global(button:hover), .account :global(button[aria-expanded='true']) { background: var(--shell-hover); color: var(--shell-ink); }
  .phone-only { display: none; }

  /* Body */
  .body { grid-area: body; min-width: 0; display: flex; flex-direction: column; }
  .banner { padding: var(--space-3) var(--space-6) 0; }
  main { flex: 1; min-width: 0; padding: var(--space-6); outline: none; }

  .sheet { display: none; }
  .quick { display: none; }

  @media (width < 48rem) {
    .shell {
      grid-template-columns: minmax(0, 1fr);
      grid-template-areas: 'top' 'body';
      padding-bottom: calc(var(--quick-h) + env(safe-area-inset-bottom, 0px));
      --toast-offset: calc(var(--quick-h) + env(safe-area-inset-bottom, 0px));
    }
    .rail, .wide-only, .search-keys { display: none; }
    .search-label { position: absolute; width: 1px; height: 1px; overflow: hidden; clip-path: inset(50%); }
    .phone-only { display: flex; }
    .top { padding: 0 var(--space-2) 0 var(--space-1); gap: var(--space-1); }
    .brand { min-height: var(--target-min); padding: 0 var(--space-3); }
    .search { flex: none; width: var(--target-min); height: var(--target-min); padding: 0; justify-content: center; background: none; border-color: transparent; color: var(--shell-ink); }
    .banner { padding: var(--space-2) var(--space-3) 0; }
    main { padding: var(--space-4) var(--space-3); }
    .sheet.open {
      display: block; position: fixed; z-index: var(--z-drawer); inset: var(--top-h) 0 calc(var(--quick-h) + env(safe-area-inset-bottom, 0px)) 0;
      overflow-y: auto; overscroll-behavior: contain; padding: var(--space-2) var(--space-2) var(--space-6);
      background: var(--shell-bg); color: var(--shell-ink);
    }
    .sheet .area, .sheet .sub a { min-height: var(--target-min); }
    .quick {
      display: block; position: fixed; z-index: var(--z-drawer); inset: auto 0 0 0;
      padding-bottom: env(safe-area-inset-bottom, 0px);
      background: var(--shell-bg); border-top: var(--border-w) solid var(--shell-line);
    }
    .quick ul { list-style: none; margin: 0; padding: 0; display: grid; grid-template-columns: repeat(5, 1fr); height: var(--quick-h); }
    .quick a, .quick button {
      display: flex; flex-direction: column; align-items: center; justify-content: center; gap: var(--space-0-5); height: 100%;
      font: inherit; font-size: var(--text-xs); color: var(--shell-ink-muted); text-decoration: none; background: none; border: 0; cursor: pointer;
    }
    .quick .here { color: var(--shell-ink); font-weight: var(--weight-semibold); }
    .quick .here :global(svg) { color: var(--shell-ink); }
  }
</style>
