<!--
  Settings › Custom formats: every format from the web UI and the config file with what it tests
  and which profiles score it. The editor lists the conditions and tests a pasted title against
  the unsaved format; formats from the config file open read-only.
  Presentational: data and actions come in as props, so the routed screen and its story share it.
-->
<script lang="ts">
  import SettingsPage from '../../../lib/components/templates/SettingsPage/SettingsPage.svelte';
  import Button from '../../../lib/components/atoms/Button/Button.svelte';
  import ManagedValue from '../../../lib/components/atoms/ManagedValue/ManagedValue.svelte';
  import Banner from '../../../lib/components/molecules/Banner/Banner.svelte';
  import EmptyState from '../../../lib/components/molecules/EmptyState/EmptyState.svelte';
  import TextField from '../../../lib/components/molecules/TextField/TextField.svelte';
  import ConditionRow from '../../../lib/components/organisms/ConditionRow/ConditionRow.svelte';
  import ConfirmDialog from '../../../lib/components/organisms/ConfirmDialog/ConfirmDialog.svelte';
  import DataTable, { type Column } from '../../../lib/components/organisms/DataTable/DataTable.svelte';
  import Dialog from '../../../lib/components/organisms/Dialog/Dialog.svelte';
  import LiveTestBox, { type TestResult } from '../../../lib/components/organisms/LiveTestBox/LiveTestBox.svelte';
  import type { ActionItem } from '../../../lib/components/molecules/ActionMenu/ActionMenu.svelte';
  import type { ConditionDoc, ConditionKindInfo, FormatDoc, FormatRow, FormatTestResult } from '../../api/types';
  import { conditionSummary, emptyFormat, formatDoc, formatSummary, type Refused } from './quality';
  import { settingsSections } from './sections';

  interface Props {
    rows: FormatRow[];
    kinds: ConditionKindInfo[];
    loading?: boolean;
    error?: { message: string; label: string };
    onretry?: () => void;
    /** isNew: create; else update the row with doc.key. */
    onsave: (doc: FormatDoc, isNew: boolean) => Promise<Refused | void>;
    /** Resolves with a refusal message ("used by the profiles …"). */
    ondelete: (key: string) => Promise<string | void>;
    ontest: (title: string, draft: FormatDoc) => Promise<FormatTestResult | undefined>;
    /** Story state: an editor open from the start ('' = a new format). */
    initialEdit?: string;
  }
  let { rows, kinds, loading = false, error, onretry, onsave, ondelete, ontest, initialEdit }: Props = $props();

  const open = (key: string | undefined): FormatDoc | undefined => {
    if (key === undefined) return undefined;
    const row = rows.find((r) => r.key === key);
    return row ? formatDoc($state.snapshot(row) as FormatRow) : emptyFormat();
  };
  // svelte-ignore state_referenced_locally
  let draft = $state<FormatDoc | undefined>(open(initialEdit));
  // svelte-ignore state_referenced_locally
  let isNew = $state(initialEdit === '');
  let refused = $state<Refused>();
  let saving = $state(false);
  let results = $state<boolean[]>();
  let deleting = $state<FormatRow>();
  let deleteError = $state<string>();
  let busy = $state(false);

  const editRow = $derived(draft && !isNew ? rows.find((r) => r.key === draft!.key) : undefined);
  const readonly = $derived(editRow?.origin === 'config');

  function edit(key: string) { draft = open(key); isNew = false; refused = undefined; results = undefined; }
  function add() { draft = emptyFormat(); isNew = true; refused = undefined; results = undefined; }
  function duplicate(r: FormatRow) {
    draft = { ...formatDoc($state.snapshot(r) as FormatRow), key: '', name: `${r.name} (copy)` };
    isNew = true; refused = undefined; results = undefined;
  }
  function close() { draft = undefined; refused = undefined; }

  function setCondition(i: number, c: ConditionDoc) {
    if (!draft) return;
    draft.conditions[i] = c;
    results = undefined;
  }
  function addCondition() { draft?.conditions.push({ kind: 'title', pattern: '' }); }
  function removeCondition(i: number) { draft?.conditions.splice(i, 1); results = undefined; }

  async function save() {
    if (!draft) return;
    saving = true;
    try {
      const r = await onsave($state.snapshot(draft) as FormatDoc, isNew);
      if (r) refused = r;
      else close();
    } finally {
      saving = false;
    }
  }

  async function test(title: string): Promise<TestResult> {
    if (!draft) return {};
    const r = await ontest(title, $state.snapshot(draft) as FormatDoc);
    const f = r?.formats[0];
    results = f?.conditions;
    if (!f) return { verdict: { matched: false, text: 'Fix the conditions to test them' } };
    return {
      verdict: { matched: f.matched, text: f.matched ? 'The format matches this title' : 'The format does not match this title' },
      conditions: draft.conditions.map((c, i) => ({ label: conditionSummary(c, kinds), matched: f.conditions[i] ?? false, required: c.required })),
    };
  }

  async function confirmDelete() {
    if (!deleting) return;
    busy = true;
    try {
      const msg = await ondelete(deleting.key);
      if (msg) deleteError = msg;
      deleting = undefined;
    } finally {
      busy = false;
    }
  }

  const columns: Column<FormatRow>[] = [
    { id: 'name', label: 'Name', value: (r) => r.name, phone: 'title' },
    { id: 'tests', label: 'Tests', value: (r) => formatSummary(r, kinds), phone: 'meta' },
    { id: 'used', label: 'Used by', width: '8rem', align: 'end', value: (r) => r.usedBy.length, phone: 'trailing' },
    { id: 'source', label: 'Source', width: '9rem', value: (r) => r.origin, phone: 'hidden' },
  ];
  function rowActions(r: FormatRow): ActionItem[] {
    const managed = r.origin === 'config';
    return [
      { id: 'edit', label: managed ? 'View' : 'Edit', icon: managed ? 'eye' : 'pencil' },
      { id: 'duplicate', label: 'Duplicate', icon: 'copy' },
      { id: 'delete', label: 'Delete…', icon: 'trash', danger: true, disabled: managed, group: 'danger' },
    ];
  }
  function onaction(id: string, r: FormatRow) {
    if (id === 'edit') edit(r.key);
    else if (id === 'duplicate') duplicate(r);
    else if (id === 'delete') { deleteError = undefined; deleting = r; }
  }
</script>

{#snippet cell(r: FormatRow, c: Column<FormatRow>)}
  {#if c.id === 'name'}<button type="button" class="name" onclick={() => edit(r.key)}>{r.name}</button>
  {:else if c.id === 'used'}{r.usedBy.length ? `${r.usedBy.length} profile${r.usedBy.length === 1 ? '' : 's'}` : '—'}
  {:else if c.id === 'source'}{#if r.origin === 'config'}<ManagedValue value="Config" managed={r.managed ?? { key: `customFormats.${r.key}` }} />{:else}<span class="muted">Added here</span>{/if}
  {:else}{c.value?.(r)}{/if}
{/snippet}

<SettingsPage sections={settingsSections()} current="custom-formats" title="Custom formats"
  description="Tests a release can pass, such as an audio codec your player handles or a group you avoid. Profiles score them.">
  {#snippet actions()}<Button variant="primary" icon="plus" onclick={add}>Add format</Button>{/snippet}
  {#if deleteError}<Banner tone="error" title="Not deleted" ondismiss={() => (deleteError = undefined)}>{deleteError}</Banner>{/if}
  <DataTable {rows} {columns} rowKey={(r) => r.key} label="Custom formats" noun="format" {cell} height="auto"
    {loading} error={error?.message} errorCode={error?.label} {onretry} onopen={(r) => edit(r.key)} {rowActions} {onaction}>
    {#snippet empty()}
      <EmptyState title="No custom formats yet" icon="filter" level={3}>
        Add one to prefer or avoid releases by codec, HDR, audio, group or anything in the title. A migration brings yours over.
        {#snippet actions()}<Button variant="primary" icon="plus" onclick={add}>Add format</Button>{/snippet}
      </EmptyState>
    {/snippet}
  </DataTable>
</SettingsPage>

<Dialog open={!!draft} title={isNew ? 'Add custom format' : (draft?.name ?? '')} size="lg"
  description={readonly ? 'From the config file' : 'A release matches when each kind of condition has a hit and every required condition holds.'}
  onclose={close}>
  {#if draft}
    <div class="dialog">
      {#if readonly}<Banner preset="managed" title="Managed by config">Change <code>customFormats.{draft.key}</code> in the config file; corsarr picks up the change.</Banner>{/if}
      {#if refused?.message && !Object.keys(refused.fields).length && !refused.conditions.length}<Banner tone="error" title="Not saved">{refused.message}</Banner>{/if}
      <TextField label="Name" bind:value={draft.name} required disabled={readonly} error={refused?.fields.name ?? refused?.fields.key}
        help="Shown in profiles and in decision reasons." />
      <div class="conds">
        {#each draft.conditions as c, i (i)}
          <ConditionRow value={c} {kinds} position={i + 1} {readonly} errors={refused?.conditions[i]} matched={results?.[i]}
            onchange={(v) => setCondition(i, v as ConditionDoc)} onremove={draft.conditions.length > 1 ? () => removeCondition(i) : undefined} />
        {/each}
        {#if refused?.fields.conditions}<p class="err">{refused.fields.conditions}</p>{/if}
        {#if !readonly}<div><Button icon="plus" size="sm" onclick={addCondition}>Add condition</Button></div>{/if}
      </div>
      <LiveTestBox label="Test with a release title" placeholder="Paste a release title" test={test}
        samples={['Show.S01E01.1080p.WEB-DL.DDP5.1.H.264-GRP', 'Movie.2024.2160p.UHD.BluRay.TrueHD.7.1.DV.HEVC-GRP']} />
    </div>
  {/if}
  {#snippet actions()}
    <Button variant="ghost" onclick={close}>{readonly ? 'Close' : 'Cancel'}</Button>
    {#if !readonly}<Button variant="primary" loading={saving} onclick={() => void save()}>Save format</Button>{/if}
  {/snippet}
</Dialog>

<ConfirmDialog open={!!deleting} title="Delete {deleting?.name ?? ''}?" confirmLabel="Delete format" loading={busy} undoable={false}
  consequences={['Releases are no longer tested against it.', 'A format some profile still scores cannot be deleted.']}
  onconfirm={() => void confirmDelete()} oncancel={() => (deleting = undefined)} />

<style>
  .dialog { display: grid; gap: var(--space-4); min-width: 0; }
  .conds { display: grid; gap: var(--space-3); min-width: 0; }
  .muted { color: var(--ink-muted); }
  .err { color: var(--tone-error-ink, var(--ink)); font-size: var(--text-sm); }
  code { font-family: var(--font-mono); font-size: var(--text-sm); overflow-wrap: anywhere; }
  .name { all: unset; cursor: pointer; color: var(--ink); font-weight: var(--weight-medium); }
  .name:hover { text-decoration: underline; }
  .name:focus-visible { outline: var(--focus-w) solid var(--focus-ring); outline-offset: var(--focus-offset); }
</style>
