<!-- Page composition (story only): Settings › Indexers list and editor (Torznab), with the managed-by-config lock. -->
<script lang="ts">
  import Shell from './Shell.svelte';
  import SettingsPage from '../lib/components/templates/SettingsPage/SettingsPage.svelte';
  import Button from '../lib/components/atoms/Button/Button.svelte';
  import StatusBadge from '../lib/components/atoms/StatusBadge/StatusBadge.svelte';
  import CheckResults from '../lib/components/molecules/CheckResults/CheckResults.svelte';
  import DataTable, { type Column } from '../lib/components/organisms/DataTable/DataTable.svelte';
  import {
    indexerRows, NOW, nowDate, settingsSections, torznabManagedMeta, torznabMeta, torznabSchema, torznabValues, type IndexerRow,
  } from '../lib/storybook/fixtures';

  let { view = 'editor' }: { view?: 'list' | 'editor' | 'managed' } = $props();

  const checks = [
    { id: 'reach', name: 'Feed answers', status: 'pass' as const, ms: 412 },
    { id: 'auth', name: 'API key accepted', status: 'pass' as const, ms: 380 },
    { id: 'caps', name: 'Categories read', status: 'warn' as const, detail: 'Books (7000) is not offered by this feed; book searches skip it.' },
    { id: 'search', name: 'Test search returns releases', status: 'pass' as const, ms: 1210, detail: '37 results for "Dockside"' },
  ];
  const columns: Column<IndexerRow>[] = [
    { id: 'name', label: 'Name', value: (r) => r.name, phone: 'title' },
    { id: 'plugin', label: 'Plugin', width: '7rem', value: (r) => r.plugin },
    { id: 'status', label: 'Status', width: '10rem', phone: 'trailing', value: (r) => r.status },
    { id: 'source', label: 'Source', width: '11.5rem', phone: 'meta', value: (r) => (r.managed ? 'Config file' : 'Settings') },
    { id: 'categories', label: 'Categories', width: '11rem', phone: 'hidden', value: (r) => r.categories },
    { id: 'priority', label: 'Priority', width: '5.5rem', align: 'end', phone: 'hidden', value: (r) => r.priority },
  ];
  const dockValues = { ...torznabValues, name: 'Dock Feeds', url: 'https://dock.example.net/api/torznab', categories: ['2000', '5000', '3000'], priority: 20 };
</script>

{#snippet cell(r: IndexerRow, c: Column<IndexerRow>)}
  {#if c.id === 'status'}<StatusBadge family="health" value={r.status} size="sm" appearance="plain" />
  {:else if c.id === 'source'}{#if r.managed}<StatusBadge family="config" value="managed" size="sm" />{:else}<span class="muted">Settings</span>{/if}
  {:else}{c.value?.(r)}{/if}
{/snippet}
{#snippet testSlot()}<CheckResults title="Test on save" {checks} ranAt={NOW - 4000} now={nowDate} onrerun={() => {}} />{/snippet}

<Shell path="/settings">
  {#if view === 'list'}
    <SettingsPage sections={settingsSections} current="indexers" title="Indexers & sources" description="Where corsarr looks for releases. Lower priority numbers win ties.">
      {#snippet actions()}<Button icon="bolt">Test all</Button><Button variant="primary" icon="plus">Add indexer</Button>{/snippet}
      <DataTable rows={indexerRows} {columns} rowKey={(r) => r.id} label="Indexers" noun="indexer" {cell} height="auto" onopen={() => {}}
        rowActions={(r) => [{ id: 'test', label: 'Test', icon: 'bolt' }, { id: 'edit', label: r.managed ? 'View' : 'Edit', icon: r.managed ? 'eye' : 'pencil' }, { id: 'delete', label: 'Delete…', icon: 'trash', danger: true, disabled: !!r.managed, group: 'd' }]} />
    </SettingsPage>
  {:else if view === 'managed'}
    <SettingsPage sections={settingsSections} current="indexers" title="Dock Feeds" description="Torznab feed · from the config file"
      managed="indexers.dock_feeds" managedSource="corsarr.yaml, line 42" schema={torznabSchema} values={dockValues} meta={torznabManagedMeta} now={NOW}>
      {#snippet actions()}<Button icon="bolt">Test</Button>{/snippet}
      {#snippet managedActions()}<Button size="sm" icon="copy">Copy as YAML</Button>{/snippet}
    </SettingsPage>
  {:else}
    <SettingsPage sections={settingsSections} current="indexers" title="Harbour Tracker" description="Torznab feed · added 12 Nov"
      schema={torznabSchema} values={torznabValues} meta={torznabMeta} test={testSlot} onsave={() => {}} oncancel={() => {}} now={NOW}>
      {#snippet actions()}<Button icon="bolt">Test</Button>{/snippet}
    </SettingsPage>
  {/if}
</Shell>

<style>
  .muted { color: var(--ink-muted); }
</style>
