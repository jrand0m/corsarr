<!--
  Settings › Plugins & integrations › Installed modules (view): every installed module with its
  status, version, kind, what it may touch and the instances using it; rollback and uninstall go
  through PluginInstallDialog. When the config locks module changes, the lock and its source show
  and the change actions are off.
-->
<script lang="ts">
  import SettingsPage from '../../../lib/components/templates/SettingsPage/SettingsPage.svelte';
  import Button from '../../../lib/components/atoms/Button/Button.svelte';
  import ManagedValue from '../../../lib/components/atoms/ManagedValue/ManagedValue.svelte';
  import StatusBadge from '../../../lib/components/atoms/StatusBadge/StatusBadge.svelte';
  import Banner from '../../../lib/components/molecules/Banner/Banner.svelte';
  import EmptyState from '../../../lib/components/molecules/EmptyState/EmptyState.svelte';
  import DataTable, { type Column } from '../../../lib/components/organisms/DataTable/DataTable.svelte';
  import PluginInstallDialog from '../../../lib/components/organisms/PluginInstallDialog/PluginInstallDialog.svelte';
  import type { ModuleInfo, PluginPolicy } from '../../api/types';
  import { fieldLabels, lockOf, permissionSummary, sortModules } from './plugin-modules';
  import { settingsSections } from './sections';

  type Change = 'rollback' | 'uninstall';
  interface Props {
    modules: ModuleInfo[];
    policy?: PluginPolicy;
    source?: { kind?: string; path?: string };
    loading?: boolean;
    error?: { message: string; label: string };
    onretry?: () => void;
    /** Runs a change; resolves with the refusal text when the server says no. */
    onchange?: (change: Change, m: ModuleInfo) => Promise<string | void>;
    onopen?: (m: ModuleInfo) => void;
  }
  let { modules, policy, source, loading = false, error, onretry, onchange, onopen }: Props = $props();

  const lock = $derived(lockOf(policy, source));
  const rows = $derived(sortModules(modules));
  let pending = $state<{ change: Change; m: ModuleInfo }>();
  let open = $state(false);
  let busy = $state(false);
  let refusal = $state<string>();

  function ask(change: Change, m: ModuleInfo) {
    pending = { change, m };
    refusal = undefined;
    open = true;
  }
  async function confirm() {
    if (!pending || !onchange) return;
    busy = true;
    try {
      const r = await onchange(pending.change, pending.m);
      if (r) refusal = r;
      else open = false;
    } finally {
      busy = false;
    }
  }

  const href = (m: ModuleInfo) => `#/settings/plugins/modules/${encodeURIComponent(m.id)}`;
  const columns: Column<ModuleInfo>[] = [
    { id: 'name', label: 'Module', value: (r) => r.manifest.name || r.id, phone: 'title' },
    { id: 'status', label: 'Status', width: '9rem', value: (r) => r.status, phone: 'trailing' },
    { id: 'version', label: 'Version', width: '6.5rem', value: (r) => r.version, phone: 'meta' },
    { id: 'kind', label: 'Kind', width: '7.5rem', value: (r) => r.kind, phone: 'hidden' },
    { id: 'perms', label: 'May touch', width: '9rem', value: (r) => permissionSummary(r.manifest.permissions), phone: 'hidden' },
    { id: 'used', label: 'Instances', width: '6rem', align: 'end', value: (r) => r.usedBy?.length ?? 0, phone: 'hidden' },
  ];
</script>

{#snippet cell(r: ModuleInfo, c: Column<ModuleInfo>)}
  {#if c.id === 'name'}<a class="name" href={href(r)}>{r.manifest.name || r.id}</a>{#if r.error}<span class="err">{r.error}</span>{/if}
  {:else if c.id === 'status'}<StatusBadge family="module" value={r.status} size="sm" appearance="plain" />
  {:else if c.id === 'version'}<span class="mono">{r.version}</span>
  {:else}{c.value?.(r)}{/if}
{/snippet}

<SettingsPage sections={settingsSections()} current="plugins" title="Installed modules" description="Plugin modules this corsarr runs, what each may touch and the instances that use it.">
  {#snippet actions()}
    <Button icon="arrow-left" href="#/settings/plugins">Integrations</Button>
    <Button icon="download" variant="primary" href="#/settings/plugins/catalogue">Catalogue</Button>
  {/snippet}
  {#snippet banner()}
    {#if lock}
      <Banner preset="managed" title="Module changes are off">
        Installing, updating and removing modules is locked by <ManagedValue value="plugins.uiInstall" managed={lock} />. Actions and checks still work.
      </Banner>
    {/if}
  {/snippet}
  <DataTable rows={rows} {columns} rowKey={(r) => r.id} label="Installed modules" noun="module" {cell} height="auto"
    {loading} error={error?.message} errorCode={error?.label} {onretry}
    onopen={(r) => onopen?.(r)}
    rowActions={(r) => [
      { id: 'open', label: 'Open', icon: 'eye' },
      { id: 'rollback', label: r.previous ? `Roll back to ${r.previous}` : 'Roll back', icon: 'undo', disabled: !!lock || !r.previous || r.status === 'uninstalled' },
      { id: 'uninstall', label: 'Uninstall', icon: 'trash', danger: true, disabled: !!lock || r.status === 'uninstalled', group: 'danger' },
    ]}
    onaction={(a, r) => (a === 'open' ? onopen?.(r) : ask(a as Change, r))}>
    {#snippet empty()}
      <EmptyState title="No modules installed yet" icon="plug" level={2}>
        Modules add download clients without a new corsarr build.
        {#snippet actions()}<Button href="#/settings/plugins/catalogue">Browse the catalogue</Button>{/snippet}
      </EmptyState>
    {/snippet}
  </DataTable>
</SettingsPage>

{#if pending}
  <PluginInstallDialog bind:open change={pending.change} name={pending.m.manifest.name || pending.m.id}
    version={pending.change === 'rollback' ? pending.m.previous : undefined} installed={pending.m.version}
    usedBy={pending.m.usedBy ?? []} labels={fieldLabels(pending.m)} {lock} {busy} error={refusal} onconfirm={confirm} />
{/if}

<style>
  .name { color: var(--ink); font-weight: var(--weight-medium); text-decoration: none; }
  .name:hover { text-decoration: underline; }
  .err { display: block; font-size: var(--text-sm); color: var(--tone-error-ink); overflow-wrap: anywhere; }
  .mono { font-family: var(--font-mono); font-size: var(--text-sm); }
</style>
