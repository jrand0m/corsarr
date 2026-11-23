<!-- Settings › General: root folders and media kind modes (read-only, from config), the API key, the server. -->
<script lang="ts">
  import { onMount } from 'svelte';
  import SettingsPage from '../../../lib/components/templates/SettingsPage/SettingsPage.svelte';
  import Button from '../../../lib/components/atoms/Button/Button.svelte';
  import CopyableValue from '../../../lib/components/atoms/CopyableValue/CopyableValue.svelte';
  import MediaKindIcon from '../../../lib/components/atoms/MediaKindIcon/MediaKindIcon.svelte';
  import Skeleton from '../../../lib/components/atoms/Skeleton/Skeleton.svelte';
  import StatusBadge from '../../../lib/components/atoms/StatusBadge/StatusBadge.svelte';
  import EmptyState from '../../../lib/components/molecules/EmptyState/EmptyState.svelte';
  import ErrorState from '../../../lib/components/molecules/ErrorState/ErrorState.svelte';
  import PropertyList from '../../../lib/components/molecules/PropertyList/PropertyList.svelte';
  import ConfirmDialog from '../../../lib/components/organisms/ConfirmDialog/ConfirmDialog.svelte';
  import { formatBytes } from '../../../lib/format/quantity';
  import type { MediaKind } from '../../../lib/tokens';
  import { app } from '../../context.svelte';
  import { Resource } from '../../resource.svelte';
  import { toApiError } from '../../api/client';
  import type { RootFolder } from '../../api/types';
  import { parseEnvExport } from '../../model';
  import ManagedBanner from './ManagedBanner.svelte';
  import { settingsSections, sourceName } from './sections';

  const { api, status, toasts } = app();

  const roots = new Resource((s) => api.list<RootFolder>('rootfolders', undefined, s));
  const rootLabel: Record<string, string> = { missing: 'Unreachable', 'read-only': 'Read-only', full: 'Full' };
  const key = new Resource((s) => api.get<{ apiKey: string }>('auth/apikey', undefined, s));
  const cfg = new Resource(async () => parseEnvExport(await api.text('config/export', { format: 'env', full: true })));
  onMount(() => {
    void roots.load();
    void key.load();
    void cfg.load();
    if (!status.data) void status.load();
  });

  const kindList: { kind: MediaKind; label: string }[] = [
    { kind: 'tv', label: 'TV' }, { kind: 'movie', label: 'Movies' }, { kind: 'music', label: 'Music' }, { kind: 'book', label: 'Books' },
  ];
  const modeHelp: Record<string, string> = {
    active: 'Searches, grabs and imports on its own.',
    shadow: 'Decides and logs what it would grab; changes nothing yet.',
    disabled: 'Hidden from the library and never searched.',
  };
  const src = $derived(status.data?.configSource);
  const shadow = $derived(kindList.filter((k) => cfg.data?.kinds[k.kind] === 'shadow').map((k) => k.label));

  let confirmOpen = $state(false);
  let rotating = $state(false);
  async function rotate() {
    rotating = true;
    try {
      key.data = await api.post<{ apiKey: string }>('auth/apikey/rotate');
      confirmOpen = false;
      toasts.push({ message: 'New API key created', detail: 'Paste it into your bots, request tools and scripts.', tone: 'ok' });
    } catch (e) {
      toasts.push({ message: 'The API key was not changed', detail: toApiError(e).message, tone: 'error' });
    } finally {
      rotating = false;
    }
  }
</script>

<SettingsPage sections={settingsSections(cfg.data?.instances.length)} current="general" title="General" description="Where media lives, how other tools reach corsarr, and which kinds it manages.">
  {#snippet banner()}<ManagedBanner kind={src?.kind} path={src?.path} />{/snippet}

  <section class="block" aria-labelledby="sg-roots">
    <h2 id="sg-roots">Root folders</h2>
    {#if roots.loading && !roots.data}
      <Skeleton shape="block" height="3.5rem" />
    {:else if roots.error}
      <ErrorState size="sm" title="Couldn't read the root folders" code={roots.error.label} onretry={roots.retry} level={3}>{roots.error.message}</ErrorState>
    {:else if !roots.data?.length}
      <EmptyState size="sm" title="No root folders yet" icon="folder" level={3}>Add one under <code>rootFolders</code> in the config (path and kind) so imports have somewhere to go.</EmptyState>
    {:else}
      <ul class="roots">
        {#each roots.data as f (f.path)}
          <li class="root">
            <div class="root-main">
              <span class="path">{f.path}</span>
              <span class="meta">{kindList.find((k) => k.kind === f.kind)?.label ?? f.kind} · {f.accessible ? `${formatBytes(f.freeSpace)} free` : 'corsarr cannot reach this folder'}</span>
            </div>
            <StatusBadge family="health" value={f.problem ? 'action_needed' : 'ok'} size="sm" label={rootLabel[f.problem ?? ''] ?? 'Writable'} />
          </li>
        {/each}
      </ul>
    {/if}
  </section>

  <section class="block" aria-labelledby="sg-key">
    <h2 id="sg-key">API key</h2>
    <p class="help">Bots, request tools and scripts sign in with this key in the <code>X-Api-Key</code> header. Keep it out of screenshots and chat.</p>
    {#if key.data}
      <div class="key">
        <CopyableValue value={key.data.apiKey} label="API key" secret />
        <Button icon="refresh" onclick={() => (confirmOpen = true)}>Regenerate…</Button>
      </div>
    {:else if key.error}
      <ErrorState size="sm" title="Couldn't read the API key" code={key.error.label} onretry={key.retry} level={3}>{key.error.message}</ErrorState>
    {:else}
      <Skeleton shape="block" height="2.25rem" />
    {/if}
  </section>

  <section class="block" aria-labelledby="sg-kinds">
    <h2 id="sg-kinds">Media kinds</h2>
    {#if shadow.length}<p class="help">{shadow.join(' and ')} {shadow.length === 1 ? 'is' : 'are'} in shadow mode: corsarr logs what it would grab and import, and changes nothing there.</p>{/if}
    {#if cfg.data}
      <ul class="kinds">
        {#each kindList as k (k.kind)}
          {@const mode = cfg.data.kinds[k.kind] ?? 'disabled'}
          <li class="kind">
            <span class="kind-name"><MediaKindIcon kind={k.kind} /><span>{k.label}</span></span>
            <span class="kind-help">{modeHelp[mode] ?? ''}</span>
            <StatusBadge family="kindMode" value={mode} size="sm" />
          </li>
        {/each}
      </ul>
    {:else if cfg.error}
      <ErrorState size="sm" title="Couldn't read the media kind modes" code={cfg.error.label} onretry={cfg.retry} level={3}>{cfg.error.message}</ErrorState>
    {:else}
      <Skeleton shape="block" height="10rem" />
    {/if}
  </section>

  <section class="block" aria-labelledby="sg-server">
    <h2 id="sg-server">Server</h2>
    <PropertyList label="Server" items={[
      { key: 'version', label: 'Version', value: status.data?.version ?? '—', mono: true },
      { key: 'source', label: 'Configuration from', value: sourceName(src?.kind, src?.path) },
      { key: 'address', label: 'Listening on', value: cfg.data?.server['server.address'] ?? '—', mono: true },
      { key: 'data', label: 'Data folder', value: cfg.data?.server['data.dir'] ?? '—', mono: true },
      { key: 'auth', label: 'Sign-in', value: cfg.data?.server['auth.mode'] === 'password' ? 'Password' : (cfg.data?.server['auth.mode'] ?? '—') },
      { key: 'log', label: 'Log level', value: cfg.data?.server['log.level'] ?? '—' },
    ]} />
  </section>
</SettingsPage>

<ConfirmDialog bind:open={confirmOpen} title="Regenerate the API key?" confirmLabel="Regenerate key" undoable={false} loading={rotating}
  consequences={['The current key stops working at once.', 'Bots, request tools and scripts using it lose access until you paste the new key into them.']}
  onconfirm={rotate} oncancel={() => (confirmOpen = false)} />

<style>
  .block { display: grid; gap: var(--space-3); min-width: 0; }
  h2 { font-size: var(--text-lg); margin-top: var(--space-2); }
  .help { color: var(--ink-muted); font-size: var(--text-md); max-width: var(--measure); }
  code { font-family: var(--font-mono); font-size: var(--text-sm); color: var(--ink); }
  .roots, .kinds { list-style: none; margin: 0; padding: 0; background: var(--surface-raised); border: var(--border-w) solid var(--line); border-radius: var(--radius-lg); }
  .roots li + li, .kinds li + li { border-top: var(--border-w) solid var(--line); }
  .root { display: flex; align-items: center; gap: var(--space-3); padding: var(--space-3) var(--space-4); min-width: 0; }
  .root-main { display: grid; gap: var(--space-0-5); flex: 1; min-width: 0; }
  .path { font-family: var(--font-mono); font-size: var(--text-sm); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .meta { font-size: var(--text-sm); color: var(--ink-muted); }
  .key { display: flex; align-items: center; gap: var(--space-2); flex-wrap: wrap; min-width: 0; }
  .key > :global(:first-child) { flex: 1 1 16rem; min-width: 0; }
  .kind { display: grid; grid-template-columns: 8rem minmax(0, 1fr) auto; align-items: center; gap: var(--space-2) var(--space-4); padding: var(--space-3) var(--space-4); }
  .kind-name { display: flex; align-items: center; gap: var(--space-2); font-weight: var(--weight-medium); }
  .kind-help { font-size: var(--text-md); color: var(--ink-muted); }
  @media (width < 48rem) {
    .kind { grid-template-columns: minmax(0, 1fr) auto; padding: var(--space-3); }
    .kind-help { grid-column: 1 / -1; grid-row: 2; }
  }
</style>
