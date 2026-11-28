<!--
  Settings › Indexers: every indexer from the config file and the web UI with its health, the
  outbound Torznab endpoint, and the tracker definition catalogue. Adding and editing use the
  form generated from the plugin's settings; an enabled indexer is tested before it is saved.
  Presentational: data and actions come in as props, so the routed screen and its story share it.
-->
<script lang="ts">
  import SettingsPage from '../../../lib/components/templates/SettingsPage/SettingsPage.svelte';
  import Button from '../../../lib/components/atoms/Button/Button.svelte';
  import Chip from '../../../lib/components/atoms/Chip/Chip.svelte';
  import CopyableValue from '../../../lib/components/atoms/CopyableValue/CopyableValue.svelte';
  import ManagedValue from '../../../lib/components/atoms/ManagedValue/ManagedValue.svelte';
  import StatusBadge from '../../../lib/components/atoms/StatusBadge/StatusBadge.svelte';
  import Switch from '../../../lib/components/atoms/Switch/Switch.svelte';
  import Banner from '../../../lib/components/molecules/Banner/Banner.svelte';
  import CheckResults from '../../../lib/components/molecules/CheckResults/CheckResults.svelte';
  import EmptyState from '../../../lib/components/molecules/EmptyState/EmptyState.svelte';
  import EnumField from '../../../lib/components/molecules/EnumField/EnumField.svelte';
  import ConfirmDialog from '../../../lib/components/organisms/ConfirmDialog/ConfirmDialog.svelte';
  import DataTable, { type Column } from '../../../lib/components/organisms/DataTable/DataTable.svelte';
  import Dialog from '../../../lib/components/organisms/Dialog/Dialog.svelte';
  import SchemaForm from '../../../lib/components/organisms/SchemaForm/SchemaForm.svelte';
  import type { ActionItem } from '../../../lib/components/molecules/ActionMenu/ActionMenu.svelte';
  import type { FormValues } from '../../../lib/forms/schema';
  import type { DefinitionEntry, IndexerInput, IndexerRow, IndexerTest, PluginManifest, TorznabState } from '../../api/types';
  import {
    addSchema, editView, pluginChoices, problemLines, rowStatus, suggestKey, testChecks, toEdit, toInput, torznabURL,
  } from './indexers';
  import { settingsSections } from './sections';

  /** A refused save: field messages, or the failed connection test. */
  export interface Refusal { fields?: Record<string, string>; test?: IndexerTest; message?: string }

  interface Props {
    rows: IndexerRow[];
    manifests: PluginManifest[];
    definitions: DefinitionEntry[];
    directory: string;
    torznab?: TorznabState;
    /** Where browsers reach corsarr ("http://box:8989"), for absolute Torznab addresses. */
    origin: string;
    loading?: boolean;
    error?: { message: string; label: string };
    onretry?: () => void;
    onadd: (input: IndexerInput) => Promise<Refusal | void>;
    onedit: (key: string, input: IndexerInput) => Promise<Refusal | void>;
    ontest: (key: string) => Promise<IndexerTest>;
    ondelete: (key: string) => Promise<void>;
    onreload: () => Promise<void>;
    ontorznab: (on: boolean) => Promise<void>;
    /** Story states: a dialog open from the start. */
    initialAdd?: string;
    initialEdit?: string;
    initialTest?: { key: string; result: IndexerTest };
  }
  let {
    rows, manifests, definitions, directory, torznab, origin, loading = false, error, onretry,
    onadd, onedit, ontest, ondelete, onreload, ontorznab, initialAdd, initialEdit, initialTest,
  }: Props = $props();

  // svelte-ignore state_referenced_locally
  let adding = $state(initialAdd !== undefined);
  // svelte-ignore state_referenced_locally
  let addPlugin = $state(initialAdd ?? '');
  let addTest = $state<IndexerTest>();
  // svelte-ignore state_referenced_locally
  let editing = $state<string | undefined>(initialEdit);
  // svelte-ignore state_referenced_locally
  let tested = $state<{ key: string; result?: IndexerTest } | undefined>(initialTest);
  let deleting = $state<IndexerRow>();
  let busy = $state(false);
  let reloading = $state(false);

  const choices = $derived(pluginChoices(manifests, definitions));
  const manifestOf = (id: string) => manifests.find((m) => m.id === id);
  const addManifest = $derived(addPlugin ? manifestOf(addPlugin) : undefined);
  const addForm = $derived.by(() => {
    if (!addManifest) return undefined;
    const s = addSchema(addManifest);
    s.fields[0].placeholder = suggestKey(addManifest.name, rows.map((r) => r.key));
    return s;
  });
  const editRow = $derived(rows.find((r) => r.key === editing));
  const editForm = $derived(editRow && manifestOf(editRow.plugin) ? editView(editRow, manifestOf(editRow.plugin)!) : undefined);
  const invalidDefs = $derived(definitions.filter((d) => !d.valid).length);

  function refused(r: Refusal | void, onTest: (t: IndexerTest) => void): Record<string, string> | void {
    if (!r) return;
    if (r.test) {
      onTest(r.test);
      throw new Error(`Not saved: the test failed. ${r.test.message}`);
    }
    if (r.fields && Object.keys(r.fields).length) return r.fields;
    if (r.message) throw new Error(r.message);
  }

  async function saveNew(v: FormValues) {
    addTest = undefined;
    const res = refused(await onadd(toInput(addPlugin, v)), (t) => (addTest = t));
    if (res) return res;
    adding = false;
    addPlugin = '';
  }

  async function saveEdit(v: FormValues) {
    if (!editRow) return;
    const res = refused(await onedit(editRow.key, toEdit(v)), (t) => (tested = { key: editRow!.key, result: t }));
    if (res) return res;
  }

  async function runTest(key: string) {
    tested = { key };
    tested = { key, result: await ontest(key) };
  }

  async function toggle(r: IndexerRow, on: boolean) {
    const res = await onedit(r.key, { enabled: on });
    if (res?.test) tested = { key: r.key, result: res.test };
  }

  async function confirmDelete() {
    if (!deleting) return;
    busy = true;
    try {
      await ondelete(deleting.key);
      deleting = undefined;
    } finally {
      busy = false;
    }
  }

  async function reload() {
    reloading = true;
    try {
      await onreload();
    } finally {
      reloading = false;
    }
  }

  const columns: Column<IndexerRow>[] = [
    { id: 'name', label: 'Name', value: (r) => r.name, phone: 'title' },
    { id: 'plugin', label: 'Type', width: '11rem', value: (r) => r.pluginName, phone: 'meta' },
    { id: 'status', label: 'Status', width: '9rem', value: (r) => rowStatus(r) ?? '', phone: 'trailing' },
    { id: 'priority', label: 'Priority', width: '5.5rem', align: 'end', value: (r) => r.priority, phone: 'hidden' },
    { id: 'source', label: 'Source', width: '10rem', value: (r) => r.source, phone: 'hidden' },
    { id: 'enabled', label: 'Enabled', width: '6rem', value: (r) => (r.enabled ? 'on' : 'off'), phone: 'hidden' },
  ];
  const defColumns: Column<DefinitionEntry>[] = [
    { id: 'name', label: 'Definition', value: (d) => d.name, phone: 'title' },
    { id: 'origin', label: 'Origin', width: '8rem', value: (d) => (d.origin === 'user' ? 'Your file' : 'Bundled'), phone: 'meta' },
    { id: 'file', label: 'File', width: '12rem', value: (d) => d.file, phone: 'hidden' },
    { id: 'valid', label: 'State', width: '8rem', value: (d) => (d.valid ? 'Valid' : 'Invalid'), phone: 'trailing' },
  ];
  function rowActions(r: IndexerRow): ActionItem[] {
    const managed = r.source === 'config';
    return [
      { id: 'test', label: 'Test', icon: 'bolt' },
      { id: 'edit', label: managed ? 'View' : 'Edit', icon: managed ? 'eye' : 'pencil' },
      { id: 'delete', label: 'Delete…', icon: 'trash', danger: true, disabled: managed, group: 'danger' },
    ];
  }
  function onaction(id: string, r: IndexerRow) {
    if (id === 'test') void runTest(r.key);
    else if (id === 'edit') editing = r.key;
    else if (id === 'delete') deleting = r;
  }
</script>

{#snippet cell(r: IndexerRow, c: Column<IndexerRow>)}
  {#if c.id === 'name'}<button type="button" class="name" onclick={() => (editing = r.key)}>{r.name}</button>{#if r.name !== r.key}<span class="key">{r.key}</span>{/if}
  {:else if c.id === 'status'}{#if rowStatus(r)}<StatusBadge family="health" value={rowStatus(r)} size="sm" appearance="plain" detail={r.health?.lastErrorMessage} />{:else}<span class="muted">Not checked yet</span>{/if}
  {:else if c.id === 'source'}{#if r.source === 'config'}<ManagedValue value="Config" managed={r.managedBy ?? { key: `indexers.${r.key}` }} />{:else}<span class="muted">Added here</span>{/if}
  {:else if c.id === 'enabled'}<Switch checked={r.enabled} label="Enable {r.name}" hideLabel disabled={r.source === 'config'} onchange={(on) => void toggle(r, on)} />
  {:else}{c.value?.(r)}{/if}
{/snippet}

{#snippet defCell(d: DefinitionEntry, c: Column<DefinitionEntry>)}
  {#if c.id === 'name'}<span class="pl"><span>{d.name}</span>{#if d.overrides}<Chip label="overrides bundled" />{/if}{#each d.modes ?? [] as m (m)}<Chip label={m} />{/each}</span>
  {:else if c.id === 'valid'}<StatusBadge label={d.valid ? 'Valid' : 'Invalid'} icon={d.valid ? 'check-circle' : 'x-circle'} tone={d.valid ? 'ok' : 'error'} size="sm" appearance="plain" />
  {:else}{c.value?.(d)}{/if}
{/snippet}

{#snippet defDetail(d: DefinitionEntry)}
  {#if d.valid}<p class="help">{d.description ?? 'No description.'}</p>
  {:else}<ul class="problems">{#each problemLines(d) as line (line)}<li><code>{line}</code></li>{/each}</ul>{/if}
{/snippet}

<SettingsPage sections={settingsSections()} current="indexers" title="Indexers" description="Where corsarr searches: Torznab indexers, built-in tracker definitions and your own definition files.">
  {#snippet actions()}<Button variant="primary" icon="plus" onclick={() => { adding = true; addTest = undefined; }}>Add indexer</Button>{/snippet}

  <section class="block" aria-labelledby="ix-list">
    <h2 id="ix-list">Indexers</h2>
    {#if tested}
      <div class="tested">
        <CheckResults title="Test of {rows.find((r) => r.key === tested?.key)?.name ?? tested.key}" keepOrder
          checks={tested.result ? testChecks(tested.result) : [{ id: 'connect', name: 'Connection and sign-in', status: 'running' }]}
          onrerun={() => void runTest(tested!.key)} />
        <Button size="sm" variant="ghost" onclick={() => (tested = undefined)}>Close results</Button>
      </div>
    {/if}
    <DataTable {rows} {columns} rowKey={(r) => r.key} label="Indexers" noun="indexer" {cell} height="auto"
      {loading} error={error?.message} errorCode={error?.label} {onretry}
      onopen={(r) => (editing = r.key)} {rowActions} {onaction}>
      {#snippet empty()}
        <EmptyState title="No indexers yet" icon="search" level={3}>
          Add a public tracker from the bundled definitions, or a Torznab indexer you already run.
          {#snippet actions()}<Button variant="primary" icon="plus" onclick={() => (adding = true)}>Add indexer</Button>{/snippet}
        </EmptyState>
      {/snippet}
    </DataTable>
  </section>

  <section class="block" aria-labelledby="ix-torznab">
    <div class="row">
      <h2 id="ix-torznab">Share with other tools</h2>
      <Switch checked={torznab?.enabled ?? false} label="Torznab endpoint" disabled={!torznab} onchange={(on) => void ontorznab(on)} />
    </div>
    <p class="help">Other apps can search through corsarr's indexers, with corsarr's limits and health checks, as Torznab feeds. Add <code>apikey=</code> with corsarr's API key.</p>
    {#if torznab?.enabled}
      <div class="urls">
        <CopyableValue label="All indexers" value={torznabURL(torznab.all, origin)} />
        {#each rows.filter((r) => r.running) as r (r.key)}<CopyableValue label={r.name} value={torznabURL(r.torznab, origin)} />{/each}
      </div>
    {/if}
  </section>

  <section class="block" aria-labelledby="ix-defs">
    <div class="row">
      <h2 id="ix-defs">Definitions</h2>
      <Button size="sm" icon="refresh" loading={reloading} onclick={() => void reload()}>Reload definitions</Button>
    </div>
    <p class="help">Bundled with corsarr, plus YAML or JSON files in <code>{directory}</code>. A file with a bundled id replaces the bundled definition.</p>
    {#if invalidDefs}
      <Banner tone="warn" title={invalidDefs === 1 ? 'One definition file has problems' : `${invalidDefs} definition files have problems`}>
        Open a row to see each problem with its line. Fix the file and reload.
      </Banner>
    {/if}
    <DataTable rows={definitions} columns={defColumns} rowKey={(d) => `${d.origin}/${d.file}`} label="Definitions" noun="definition" cell={defCell} expand={defDetail} height="auto" />
  </section>
</SettingsPage>

<Dialog open={adding} title="Add indexer" description="Pick the kind of indexer, then fill in its settings. corsarr tests it before saving." size="lg"
  onclose={() => { adding = false; addPlugin = ''; }}>
  <div class="dialog">
    <EnumField label="Indexer" bind:value={addPlugin} options={choices} placeholder="Choose an indexer" required />
    {#if addManifest && addForm}
      {#key addPlugin}
        <SchemaForm schema={addForm} label="{addManifest.name} settings" saveLabel="Test and save" onsave={saveNew} oncancel={() => { adding = false; addPlugin = ''; }} />
      {/key}
    {/if}
    {#if addTest}<CheckResults title="Test" checks={testChecks(addTest)} keepOrder />{/if}
  </div>
</Dialog>

<Dialog open={!!editRow} title={editRow?.name ?? ''} description={editRow ? `${editRow.pluginName} · ${editRow.source === 'config' ? 'from the config file' : 'added here'}` : undefined} size="lg"
  onclose={() => (editing = undefined)}>
  <div class="dialog">
    {#if editRow && editForm}
      {#if editRow.source === 'config'}
        <Banner preset="managed" title="Managed by config">Change <code>indexers.{editRow.key}</code> in the config file; corsarr picks up the change.</Banner>
      {/if}
      {#key editRow.key}
        <SchemaForm schema={editForm.schema} values={editForm.values} meta={editForm.meta} label="{editRow.name} settings"
          readonly={editRow.source === 'config'} managedBanner={false} onsave={saveEdit} />
      {/key}
      {#if editRow.health?.lastErrorMessage}
        <Banner tone="error" title="Last error">{editRow.health.lastErrorMessage}{#if editRow.health.fixHint} {editRow.health.fixHint}{/if}</Banner>
      {/if}
    {:else if editRow}
      <EmptyState title="The plugin {editRow.plugin} is not in this build" icon="plug" level={3}>corsarr ignores this indexer until a build with that plugin runs.</EmptyState>
    {/if}
  </div>
</Dialog>

<ConfirmDialog open={!!deleting} title="Delete {deleting?.name ?? ''}?" confirmLabel="Delete indexer" loading={busy} undoable={false}
  consequences={['Searches stop using this indexer.', 'Its Torznab address stops answering.', 'Releases already grabbed are not affected.']}
  onconfirm={() => void confirmDelete()} oncancel={() => (deleting = undefined)} />

<style>
  .block { display: grid; gap: var(--space-3); min-width: 0; }
  .row { display: flex; align-items: center; justify-content: space-between; gap: var(--space-3); flex-wrap: wrap; }
  h2 { font-size: var(--text-lg); margin-top: var(--space-2); }
  .help { color: var(--ink-muted); font-size: var(--text-md); max-width: var(--measure); }
  .muted { color: var(--ink-muted); }
  code { font-family: var(--font-mono); font-size: var(--text-sm); color: var(--ink); overflow-wrap: anywhere; }
  .name { all: unset; cursor: pointer; color: var(--ink); font-weight: var(--weight-medium); }
  .name:hover { text-decoration: underline; }
  .name:focus-visible { outline: var(--focus-w) solid var(--focus-ring); outline-offset: var(--focus-offset); }
  .key { margin-inline-start: var(--space-2); color: var(--ink-muted); font-family: var(--font-mono); font-size: var(--text-sm); }
  .pl { display: flex; align-items: center; gap: var(--space-1-5); flex-wrap: wrap; min-width: 0; }
  .urls { display: grid; gap: var(--space-2); min-width: 0; }
  .tested { display: grid; gap: var(--space-2); justify-items: start; }
  .dialog { display: grid; gap: var(--space-4); min-width: 0; }
  .problems { display: grid; gap: var(--space-1); padding-inline-start: var(--space-4); }
</style>
