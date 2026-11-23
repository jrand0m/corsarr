<!--
  Settings › Plugins & integrations (read-only MVP): configured instances with their health,
  each opening a SchemaForm generated from its plugin manifest in managed (locked) mode; the plugins
  the binary ships; and the config export (YAML or env) to copy or download.
-->
<script lang="ts">
  import { onMount } from 'svelte';
  import SettingsPage from '../../../lib/components/templates/SettingsPage/SettingsPage.svelte';
  import Button from '../../../lib/components/atoms/Button/Button.svelte';
  import Chip from '../../../lib/components/atoms/Chip/Chip.svelte';
  import Skeleton from '../../../lib/components/atoms/Skeleton/Skeleton.svelte';
  import StatusBadge from '../../../lib/components/atoms/StatusBadge/StatusBadge.svelte';
  import CodeViewer from '../../../lib/components/molecules/CodeViewer/CodeViewer.svelte';
  import EmptyState from '../../../lib/components/molecules/EmptyState/EmptyState.svelte';
  import ErrorState from '../../../lib/components/molecules/ErrorState/ErrorState.svelte';
  import SegmentedControl from '../../../lib/components/molecules/SegmentedControl/SegmentedControl.svelte';
  import DataTable, { type Column } from '../../../lib/components/organisms/DataTable/DataTable.svelte';
  import SchemaForm from '../../../lib/components/organisms/SchemaForm/SchemaForm.svelte';
  import { app } from '../../context.svelte';
  import { Resource } from '../../resource.svelte';
  import { refetchOn } from '../../live.svelte';
  import type { IntegrationHealth, PluginManifest } from '../../api/types';
  import { parseEnvExport, type ConfiguredInstance } from '../../model';
  import { instanceView, sectionLabel } from './plugin-form';
  import ManagedBanner from './ManagedBanner.svelte';
  import { settingsSections } from './sections';
  import type { ScreenProps } from '../../screen';

  let { params = {} }: ScreenProps = $props();
  const { api, live, status, navigate } = app();

  const plugins = new Resource((s) => api.list<PluginManifest>('plugins', undefined, s));
  const cfg = new Resource(async () => parseEnvExport(await api.text('config/export', { format: 'env', full: true })));
  const health = new Resource((s) => api.list<IntegrationHealth>('health/integrations', undefined, s));
  let format = $state<'yaml' | 'env'>('yaml');
  const exported = new Resource(async () => api.text('config/export', { format }));
  onMount(() => {
    void plugins.load();
    void cfg.load();
    void health.load();
    if (!status.data) void status.load();
    const offs = [
      refetchOn(live, ['health.changed'], () => void health.load(true)),
      refetchOn(live, ['config.applied', 'plugin.loaded'], () => { void cfg.load(true); void plugins.load(true); void exported.load(true); }),
    ];
    return () => offs.forEach((off) => off());
  });
  $effect(() => {
    void format;
    void exported.load();
  });

  const src = $derived(status.data?.configSource);
  const selectedId = $derived(params.id);
  const instances = $derived(cfg.data?.instances ?? []);
  const current = $derived(instances.find((i) => `${i.section}.${i.key}` === selectedId));
  const manifestOf = (i: ConfiguredInstance) => plugins.data?.find((p) => p.id === i.plugin);
  const healthOf = (i: ConfiguredInstance) => health.data?.find((h) => h.ref.key === i.key);

  const columns: Column<ConfiguredInstance>[] = [
    { id: 'name', label: 'Name', value: (r) => r.name ?? r.key, phone: 'title' },
    { id: 'plugin', label: 'Plugin', width: '7.5rem', value: (r) => manifestOf(r)?.name ?? r.plugin, phone: 'meta' },
    { id: 'section', label: 'Section', width: '8.5rem', value: (r) => sectionLabel(r.section), phone: 'hidden' },
    { id: 'status', label: 'Status', width: '8rem', value: (r) => healthOf(r)?.status ?? '', phone: 'trailing' },
    { id: 'source', label: 'Source', width: '10rem', value: () => 'Config', phone: 'hidden' },
  ];
  const pluginColumns: Column<PluginManifest>[] = [
    { id: 'name', label: 'Plugin', value: (r) => r.name, phone: 'title' },
    { id: 'kind', label: 'Kind', width: '8rem', value: (r) => r.kind, phone: 'meta' },
    { id: 'version', label: 'Version', width: '6rem', value: (r) => r.version, phone: 'hidden' },
    { id: 'delivery', label: 'Delivery', width: '7rem', value: (r) => r.delivery, phone: 'hidden' },
    { id: 'used', label: 'Instances', width: '6rem', align: 'end', value: (r) => instances.filter((i) => i.plugin === r.id).length, phone: 'trailing' },
  ];
  const view = $derived(current && manifestOf(current) ? instanceView(current, manifestOf(current)!) : undefined);
</script>

{#snippet cell(r: ConfiguredInstance, c: Column<ConfiguredInstance>)}
  {#if c.id === 'status'}{#if healthOf(r)}<StatusBadge family="health" value={healthOf(r)!.status} size="sm" appearance="plain" />{:else}<span class="muted">Not checked yet</span>{/if}
  {:else if c.id === 'source'}<StatusBadge family="config" value="managed" size="sm" />
  {:else if c.id === 'name'}<a class="name" href="#/settings/plugins/{r.section}.{r.key}">{r.name ?? r.key}</a>
  {:else}{c.value?.(r)}{/if}
{/snippet}

{#snippet pluginCell(r: PluginManifest, c: Column<PluginManifest>)}
  {#if c.id === 'name'}<span class="pl"><span>{r.name}</span>{#each (r.capabilities ?? []).slice(0, 3) as cap (cap)}<Chip label={cap} />{/each}</span>
  {:else}{c.value?.(r)}{/if}
{/snippet}

{#if selectedId}
  <SettingsPage sections={settingsSections(instances.length)} current="plugins" title={current?.name ?? current?.key ?? 'Integration'}
    description={current ? `${manifestOf(current)?.name ?? current.plugin} · ${sectionLabel(current.section)} · from config` : undefined}>
    {#snippet actions()}<Button icon="arrow-left" href="#/settings/plugins">All integrations</Button>{/snippet}
    {#snippet banner()}<ManagedBanner kind={src?.kind} path={src?.path} />{/snippet}
    {#if (cfg.loading && !cfg.data) || (plugins.loading && !plugins.data)}
      <Skeleton shape="block" height="16rem" />
    {:else if cfg.error || plugins.error}
      <ErrorState title="Couldn't read the configuration" code={(cfg.error ?? plugins.error)!.label} onretry={() => { void cfg.load(); void plugins.load(); }} level={2}>{(cfg.error ?? plugins.error)!.message}</ErrorState>
    {:else if !current}
      <EmptyState title="No integration called {selectedId}" icon="plug" level={2}>
        It may have been removed from the config.
        {#snippet actions()}<Button href="#/settings/plugins">See all integrations</Button>{/snippet}
      </EmptyState>
    {:else if !view}
      <EmptyState title="The plugin {current.plugin} is not in this build" icon="plug" level={2}>corsarr ignores this instance until a build with that plugin runs.</EmptyState>
    {:else}
      <div class="editor"><SchemaForm schema={view.schema} values={view.values} meta={view.meta} label="{current.name ?? current.key} settings" readonly managedBanner={false} /></div>
    {/if}
  </SettingsPage>
{:else}
  <SettingsPage sections={settingsSections(instances.length)} current="plugins" title="Plugins & integrations" description="What corsarr talks to: configured instances and the plugins this build ships.">
    {#snippet banner()}<ManagedBanner kind={src?.kind} path={src?.path} exportHref="#/settings/plugins" />{/snippet}

    <section class="block" aria-labelledby="sp-inst">
      <h2 id="sp-inst">Configured</h2>
      <DataTable rows={instances} {columns} rowKey={(r) => `${r.section}.${r.key}`} label="Configured integrations" noun="integration" {cell} height="auto"
        loading={cfg.loading && !cfg.data} error={cfg.error?.message} errorCode={cfg.error?.label} onretry={cfg.retry}
        onopen={(r) => navigate(`/settings/plugins/${r.section}.${r.key}`)}
        rowActions={() => [{ id: 'view', label: 'View', icon: 'eye' }]} onaction={(_, r) => navigate(`/settings/plugins/${r.section}.${r.key}`)}>
        {#snippet empty()}
          <EmptyState title="Nothing is configured yet" icon="plug" level={3}>
            Add indexers, a download client and TMDB under their sections in the config (for example <code>indexers.myindexer.type=torznab</code>).
          </EmptyState>
        {/snippet}
      </DataTable>
    </section>

    <section class="block" aria-labelledby="sp-plugins">
      <h2 id="sp-plugins">Plugins in this build</h2>
      <DataTable rows={plugins.data ?? []} columns={pluginColumns} rowKey={(r) => r.id} label="Plugins" noun="plugin" cell={pluginCell} height="auto"
        loading={plugins.loading && !plugins.data} error={plugins.error?.message} errorCode={plugins.error?.label} onretry={plugins.retry} />
    </section>

    <section class="block" aria-labelledby="sp-export">
      <div class="row">
        <h2 id="sp-export">Export</h2>
        <SegmentedControl label="Format" size="sm" bind:value={format} options={[{ value: 'yaml', label: 'YAML' }, { value: 'env', label: 'Environment' }]} />
      </div>
      <p class="help">The running configuration with secrets replaced by references. Use it as a config file, or as environment variables for a container.</p>
      {#if exported.data !== undefined}
        <CodeViewer code={exported.data} language={format} filename={format === 'yaml' ? 'corsarr.yaml' : 'corsarr.env'} label="Config export" download maxHeight="28rem" />
      {:else if exported.error}
        <ErrorState size="sm" title="Couldn't export the configuration" code={exported.error.label} onretry={exported.retry} level={3}>{exported.error.message}</ErrorState>
      {:else}
        <Skeleton shape="block" height="12rem" />
      {/if}
    </section>
  </SettingsPage>
{/if}

<style>
  .block { display: grid; gap: var(--space-3); min-width: 0; }
  .row { display: flex; align-items: center; justify-content: space-between; gap: var(--space-3); flex-wrap: wrap; }
  h2 { font-size: var(--text-lg); margin-top: var(--space-2); }
  .help { color: var(--ink-muted); font-size: var(--text-md); max-width: var(--measure); }
  .muted { color: var(--ink-muted); }
  code { font-family: var(--font-mono); font-size: var(--text-sm); color: var(--ink); }
  .name { color: var(--ink); font-weight: var(--weight-medium); text-decoration: none; }
  .name:hover { text-decoration: underline; }
  .pl { display: flex; align-items: center; gap: var(--space-1-5); flex-wrap: wrap; min-width: 0; }
  .editor { padding: var(--space-5); background: var(--surface-raised); border: var(--border-w) solid var(--line); border-radius: var(--radius-lg); min-width: 0; }
  @media (width < 48rem) { .editor { padding: var(--space-4) var(--space-3); } }
</style>
