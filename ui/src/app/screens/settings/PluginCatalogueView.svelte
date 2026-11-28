<!--
  Settings › Plugins & integrations › Catalogue (view): the verified module catalogue with where
  it came from and how fresh it is (a stale copy shows its age and the fetch error; no copy shows
  the error alone). Each offer says whether it is new, installed, an update, an update waiting for
  approval because it asks for more, or incompatible with this corsarr. Install and update open
  PluginInstallDialog with the exact version shown; the config lock turns them off.
-->
<script lang="ts">
  import SettingsPage from '../../../lib/components/templates/SettingsPage/SettingsPage.svelte';
  import Button from '../../../lib/components/atoms/Button/Button.svelte';
  import ManagedValue from '../../../lib/components/atoms/ManagedValue/ManagedValue.svelte';
  import Skeleton from '../../../lib/components/atoms/Skeleton/Skeleton.svelte';
  import StatusBadge from '../../../lib/components/atoms/StatusBadge/StatusBadge.svelte';
  import Timestamp from '../../../lib/components/atoms/Timestamp/Timestamp.svelte';
  import Banner from '../../../lib/components/molecules/Banner/Banner.svelte';
  import EmptyState from '../../../lib/components/molecules/EmptyState/EmptyState.svelte';
  import ErrorState from '../../../lib/components/molecules/ErrorState/ErrorState.svelte';
  import PluginInstallDialog from '../../../lib/components/organisms/PluginInstallDialog/PluginInstallDialog.svelte';
  import { formatBytes } from '../../../lib/format/quantity';
  import type { CatalogueOffer, CatalogueView } from '../../api/types';
  import { lockOf, permissionSummary } from './plugin-modules';
  import { settingsSections } from './sections';

  interface Props {
    catalogue?: CatalogueView;
    source?: { kind?: string; path?: string };
    loading?: boolean;
    refreshing?: boolean;
    error?: { message: string; label: string };
    onretry?: () => void;
    onrefresh?: () => void;
    /** Installs or updates to the offer's version; resolves with the refusal text when refused. */
    oninstall?: (offer: CatalogueOffer) => Promise<string | void>;
    now?: Date;
  }
  let { catalogue, source, loading = false, refreshing = false, error, onretry, onrefresh, oninstall, now }: Props = $props();

  const lock = $derived(lockOf(catalogue?.policy, source));
  const offers = $derived(catalogue?.offers ?? []);
  const src = $derived(catalogue?.source);

  let pending = $state<CatalogueOffer>();
  let open = $state(false);
  let busy = $state(false);
  let refusal = $state<string>();
  function ask(o: CatalogueOffer) {
    pending = o;
    refusal = undefined;
    open = true;
  }
  async function confirm() {
    if (!pending || !oninstall) return;
    busy = true;
    try {
      const r = await oninstall(pending);
      if (r) refusal = r;
      else open = false;
    } finally {
      busy = false;
    }
  }

  const buttonLabel: Partial<Record<CatalogueOffer['state'], string>> = { available: 'Install', update: 'Update', update_needs_approval: 'Review and update' };
  const moduleHref = (o: CatalogueOffer) => `#/settings/plugins/modules/${encodeURIComponent(o.entry.id)}`;
</script>

<SettingsPage sections={settingsSections()} current="plugins" title="Catalogue" description="Plugin modules you can install, checked against the catalogue's signature.">
  {#snippet actions()}
    <Button icon="arrow-left" href="#/settings/plugins/modules">Installed modules</Button>
    <Button icon="refresh" loading={refreshing} onclick={() => onrefresh?.()}>Refresh</Button>
  {/snippet}
  {#snippet banner()}
    {#if lock}
      <Banner preset="managed" title="Module changes are off">
        Installing and updating from here is locked by <ManagedValue value="plugins.uiInstall" managed={lock} />. Modules listed under plugins.install in the config still install at start.
      </Banner>
    {/if}
    {#if src?.stale}
      <Banner preset="stale" title="Showing the last verified copy">
        {#if src.fetchedAt}Fetched <Timestamp value={src.fetchedAt} {now} />.{/if}
        {#if src.error}The latest fetch failed: {src.error}{/if}
      </Banner>
    {/if}
  {/snippet}

  {#if loading && !catalogue}
    <Skeleton shape="block" height="16rem" />
  {:else if error}
    <ErrorState title="Couldn't load the catalogue" code={error.label} {onretry} level={2}>{error.message}</ErrorState>
  {:else if src?.error && !offers.length}
    <ErrorState title="The catalogue is not available" {onretry} level={2}>{src.error}</ErrorState>
  {:else if catalogue}
    <p class="source">
      From <span class="mono">{src?.url}</span>{#if src?.release} · release <span class="mono">{src.release}</span>{/if}{#if src?.fetchedAt && !src.stale} · fetched <Timestamp value={src.fetchedAt} {now} />{/if}
    </p>
    {#if offers.length}
      <ul class="offers" aria-label="Catalogue modules">
        {#each offers as o (o.entry.id)}
          <li class="offer">
            <div class="head">
              <h2 class="title">{o.entry.name}</h2>
              <span class="mono ver">{o.installed && o.installed !== o.entry.version ? `${o.installed} → ${o.entry.version}` : o.entry.version}</span>
              <StatusBadge family="offer" value={o.state} size="sm" detail={o.state === 'incompatible' ? o.reason : undefined} />
            </div>
            {#if o.entry.description}<p class="desc">{o.entry.description}</p>{/if}
            <p class="meta">
              {o.entry.kind}{#if o.entry.author} · {o.entry.author}{/if}{#if o.entry.license} · {o.entry.license}{/if} · {formatBytes(o.entry.size)} · {permissionSummary(o.entry.permissions)}
            </p>
            {#if o.entry.homepage}<p class="meta">Homepage: <span class="mono">{o.entry.homepage}</span></p>{/if}
            <div class="buttons">
              {#if buttonLabel[o.state]}
                <Button size="sm" variant={o.state === 'available' ? 'primary' : 'secondary'} icon={o.state === 'available' ? 'download' : 'arrow-up'} disabled={!!lock}
                  onclick={() => ask(o)}>{buttonLabel[o.state]}</Button>
              {/if}
              {#if o.installed}<Button size="sm" variant="ghost" href={moduleHref(o)}>Open</Button>{/if}
            </div>
          </li>
        {/each}
      </ul>
    {:else}
      <EmptyState title="The catalogue lists no modules" icon="plug" level={2}>Check again later.</EmptyState>
    {/if}
  {/if}
</SettingsPage>

{#if pending}
  <PluginInstallDialog bind:open change={pending.installed ? 'update' : 'install'} name={pending.entry.name} version={pending.entry.version} installed={pending.installed}
    permissions={pending.entry.permissions} diff={pending.diff} {lock} {busy} error={refusal} onconfirm={confirm} />
{/if}

<style>
  .source { color: var(--ink-muted); font-size: var(--text-md); overflow-wrap: anywhere; }
  .mono { font-family: var(--font-mono); font-size: var(--text-sm); }
  .offers { list-style: none; padding: 0; display: grid; gap: var(--space-3); }
  .offer { display: grid; gap: var(--space-2); padding: var(--space-4) var(--space-5); background: var(--surface-raised); border: var(--border-w) solid var(--line); border-radius: var(--radius-lg); min-width: 0; }
  .head { display: flex; align-items: center; gap: var(--space-2) var(--space-3); flex-wrap: wrap; }
  .title { font-size: var(--text-lg); font-weight: var(--weight-semibold); }
  .ver { color: var(--ink-muted); }
  .desc { white-space: pre-line; max-width: var(--measure); overflow-wrap: anywhere; }
  .meta { color: var(--ink-muted); font-size: var(--text-sm); overflow-wrap: anywhere; }
  .buttons { display: flex; gap: var(--space-2); flex-wrap: wrap; }
  @media (width < 48rem) { .offer { padding: var(--space-3); } }
</style>
