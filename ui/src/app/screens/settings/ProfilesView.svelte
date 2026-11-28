<!--
  Settings › Profiles: built-in and custom quality profiles with what they serve and how many
  series, movies and artists use them. The editor ranks the allowed qualities (groups count the
  same), picks the cutoff, scores custom formats (or rejects them) and sets size limits per
  quality. Built-in profiles and profiles from the config file open read-only; built-ins can be
  duplicated into an editable copy.
  Presentational: data and actions come in as props, so the routed screen and its story share it.
-->
<script lang="ts">
  import SettingsPage from '../../../lib/components/templates/SettingsPage/SettingsPage.svelte';
  import Button from '../../../lib/components/atoms/Button/Button.svelte';
  import Checkbox from '../../../lib/components/atoms/Checkbox/Checkbox.svelte';
  import Chip from '../../../lib/components/atoms/Chip/Chip.svelte';
  import IconButton from '../../../lib/components/atoms/IconButton/IconButton.svelte';
  import ManagedValue from '../../../lib/components/atoms/ManagedValue/ManagedValue.svelte';
  import Banner from '../../../lib/components/molecules/Banner/Banner.svelte';
  import BoolField from '../../../lib/components/molecules/BoolField/BoolField.svelte';
  import EnumField from '../../../lib/components/molecules/EnumField/EnumField.svelte';
  import MultiselectField from '../../../lib/components/molecules/MultiselectField/MultiselectField.svelte';
  import NumberField from '../../../lib/components/molecules/NumberField/NumberField.svelte';
  import TextField from '../../../lib/components/molecules/TextField/TextField.svelte';
  import ConfirmDialog from '../../../lib/components/organisms/ConfirmDialog/ConfirmDialog.svelte';
  import DataTable, { type Column } from '../../../lib/components/organisms/DataTable/DataTable.svelte';
  import Dialog from '../../../lib/components/organisms/Dialog/Dialog.svelte';
  import OrderedListEditor, { type OrderedItem } from '../../../lib/components/organisms/OrderedListEditor/OrderedListEditor.svelte';
  import type { ActionItem } from '../../../lib/components/molecules/ActionMenu/ActionMenu.svelte';
  import type { MediaKind } from '../../../lib/tokens';
  import type { FormatRow, ProfileDoc, ProfileRow, QualityChoices } from '../../api/types';
  import {
    cutoffOptions, groupsFromItems, kindsLabel, ladderItems, originLabel, profileDoc, repairCutoff, type Refused,
  } from './quality';
  import { settingsSections } from './sections';

  interface Props {
    rows: ProfileRow[];
    formats: FormatRow[];
    choices?: QualityChoices;
    loading?: boolean;
    error?: { message: string; label: string };
    onretry?: () => void;
    onsave: (doc: ProfileDoc, isNew: boolean) => Promise<Refused | void>;
    ondelete: (key: string) => Promise<string | void>;
    /** Resolves with the new row's key, or a refusal message. */
    onduplicate: (row: ProfileRow) => Promise<{ key?: string; message?: string }>;
    /** Story state: the editor open on a row ("kind:key"). */
    initialEdit?: string;
  }
  let { rows, formats, choices, loading = false, error, onretry, onsave, ondelete, onduplicate, initialEdit }: Props = $props();

  const rowId = (r: ProfileRow) => `${r.kinds[0]}:${r.key}`;
  // svelte-ignore state_referenced_locally
  let editingId = $state<string | undefined>(initialEdit);
  let draft = $state<ProfileDoc>();
  let ladder = $state<OrderedItem[]>([]);
  let refused = $state<Refused>();
  let saving = $state(false);
  let deleting = $state<ProfileRow>();
  let notice = $state<string>();
  let busy = $state(false);
  let sizeQuality = $state('');

  const editRow = $derived(editingId ? rows.find((r) => rowId(r) === editingId) : undefined);
  const readonly = $derived(!!editRow && (editRow.builtin || editRow.origin === 'config'));
  const music = $derived(draft?.kinds.includes('music') ?? false);
  const qualities = $derived(choices?.qualities[music ? 'music' : 'movie'] ?? []);
  const names = $derived(new Map(qualities.map((q) => [q.id, q.name])));
  const allowed = $derived(draft?.groups.flatMap((g) => g.qualities) ?? []);

  $effect(() => {
    // open the editor on the row once rows arrive (story state, or a reload)
    if (editRow && (!draft || draft.key !== editRow.key)) start(editRow);
  });

  function start(r: ProfileRow | undefined, doc?: ProfileDoc) {
    draft = doc ?? (r ? structuredClone(profileDoc(r)) : undefined);
    refused = undefined;
    if (draft) ladder = ladderItems(draft.groups, choices?.qualities[draft.kinds.includes('music') ? 'music' : 'movie'] ?? []);
  }
  function edit(r: ProfileRow) { editingId = rowId(r); start(r); }
  function add() {
    editingId = 'new';
    start(undefined, { key: '', name: '', kinds: ['tv', 'movie'], groups: [{ qualities: ['webdl-1080p'] }], cutoff: 'webdl-1080p',
      upgrades: true, minFormatScore: 0, cutoffFormatScore: 0, minUpgradeFormatScore: 1 });
  }
  function close() { editingId = undefined; draft = undefined; refused = undefined; }

  function onladder(items: OrderedItem[]) {
    if (!draft) return;
    ladder = items;
    draft.groups = groupsFromItems(items);
    draft.cutoff = repairCutoff(draft.cutoff, draft.groups);
    draft.sizes = draft.sizes?.filter((s) => allowed.includes(s.quality));
  }
  function setKinds(ks: string[]) {
    if (!draft) return;
    const wasMusic = music;
    const next = ks.includes('music') && !wasMusic ? ['music'] : wasMusic && ks.length > 1 ? ks.filter((k) => k !== 'music') : ks;
    draft.kinds = next as MediaKind[];
    if (draft.kinds.includes('music') !== wasMusic) {
      draft.groups = [];
      draft.cutoff = '';
      draft.sizes = [];
      ladder = ladderItems([], choices?.qualities[draft.kinds.includes('music') ? 'music' : 'movie'] ?? []);
    }
  }

  const scoreOf = (key: string) => draft?.formats?.find((f) => f.format === key);
  function setScore(key: string, patch: { score?: number; reject?: boolean }) {
    if (!draft) return;
    const list = (draft.formats ?? []).filter((f) => f.format !== key);
    const cur = { format: key, score: 0, reject: false, ...scoreOf(key), ...patch };
    if (cur.score !== 0 || cur.reject) list.push(cur);
    draft.formats = list;
  }
  function addSize() {
    if (!draft || !sizeQuality) return;
    draft.sizes = [...(draft.sizes ?? []), { quality: sizeQuality, min: 0, max: 100 }];
    sizeQuality = '';
  }

  async function save() {
    if (!draft) return;
    saving = true;
    try {
      const r = await onsave($state.snapshot(draft) as ProfileDoc, editingId === 'new');
      if (r) refused = r;
      else close();
    } finally {
      saving = false;
    }
  }

  async function duplicate(r: ProfileRow) {
    const res = await onduplicate(r);
    if (res.message) notice = res.message;
    else if (res.key) editingId = `${r.kinds[0]}:${res.key}`;
  }

  async function confirmDelete() {
    if (!deleting) return;
    busy = true;
    try {
      const msg = await ondelete(deleting.key);
      if (msg) notice = msg;
      deleting = undefined;
    } finally {
      busy = false;
    }
  }

  const columns: Column<ProfileRow>[] = [
    { id: 'name', label: 'Name', value: (r) => r.name, phone: 'title' },
    { id: 'kinds', label: 'For', width: '9rem', value: (r) => kindsLabel(r.kinds), phone: 'meta' },
    { id: 'cutoff', label: 'Upgrades until', width: '11rem', value: (r) => (r.upgrades ? r.cutoff : 'No upgrades'), phone: 'hidden' },
    { id: 'formats', label: 'Formats', width: '6rem', align: 'end', value: (r) => r.formats?.length ?? 0, phone: 'hidden' },
    { id: 'items', label: 'In use', width: '6rem', align: 'end', value: (r) => r.items, phone: 'trailing' },
    { id: 'source', label: 'Source', width: '9rem', value: (r) => originLabel(r), phone: 'hidden' },
  ];
  function rowActions(r: ProfileRow): ActionItem[] {
    const locked = r.builtin || r.origin === 'config';
    return [
      { id: 'edit', label: locked ? 'View' : 'Edit', icon: locked ? 'eye' : 'pencil' },
      { id: 'duplicate', label: 'Duplicate', icon: 'copy' },
      { id: 'delete', label: 'Delete…', icon: 'trash', danger: true, disabled: locked, group: 'danger' },
    ];
  }
  function onaction(id: string, r: ProfileRow) {
    if (id === 'edit') edit(r);
    else if (id === 'duplicate') void duplicate(r);
    else if (id === 'delete') { notice = undefined; deleting = r; }
  }
  const kindOptions = [{ value: 'tv', label: 'TV' }, { value: 'movie', label: 'Movies' }, { value: 'music', label: 'Music (on its own)' }];
</script>

{#snippet cell(r: ProfileRow, c: Column<ProfileRow>)}
  {#if c.id === 'name'}<button type="button" class="name" onclick={() => edit(r)}>{r.name}</button>{#each r.default ?? [] as k (k)}<Chip label="default for {k}" />{/each}
  {:else if c.id === 'items'}{r.builtin ? '—' : r.items}
  {:else if c.id === 'source'}{#if r.origin === 'config'}<ManagedValue value="Config" managed={r.managed ?? { key: `qualityProfiles.${r.key}` }} />{:else}<span class="muted">{originLabel(r)}</span>{/if}
  {:else}{c.value?.(r)}{/if}
{/snippet}

<SettingsPage sections={settingsSections()} current="profiles" title="Profiles"
  description="Which qualities series, movies and artists may get, how long upgrades go on, and how custom formats count.">
  {#snippet actions()}<Button variant="primary" icon="plus" onclick={add}>Add profile</Button>{/snippet}
  {#if notice}<Banner tone="error" title="Not done" ondismiss={() => (notice = undefined)}>{notice}</Banner>{/if}
  <DataTable {rows} {columns} rowKey={rowId} label="Quality profiles" noun="profile" {cell} height="auto"
    {loading} error={error?.message} errorCode={error?.label} {onretry} onopen={edit} {rowActions} {onaction} />
</SettingsPage>

<Dialog open={!!draft} title={editingId === 'new' ? 'Add profile' : (draft?.name ?? '')} size="lg"
  description={editRow?.builtin ? 'Built-in profile' : editRow?.origin === 'config' ? 'From the config file' : 'Custom profile'} onclose={close}>
  {#if draft}
    <div class="dialog">
      {#if editRow?.builtin}
        <Banner tone="info" title="Built-in profiles stay as shipped">Duplicate it to change a copy.
          {#snippet actions()}<Button size="sm" icon="copy" onclick={() => editRow && void duplicate(editRow)}>Duplicate</Button>{/snippet}
        </Banner>
      {:else if editRow?.origin === 'config'}
        <Banner preset="managed" title="Managed by config">Change <code>qualityProfiles.{draft.key}</code> in the config file.</Banner>
      {/if}
      {#if refused?.message && !Object.keys(refused.fields).length}<Banner tone="error" title="Not saved">{refused.message}</Banner>{/if}
      <TextField label="Name" bind:value={draft.name} required disabled={readonly} error={refused?.fields.name ?? refused?.fields.key} />
      <MultiselectField label="For" value={draft.kinds} options={kindOptions} columns={3} disabled={readonly} error={refused?.fields.kinds}
        help="One profile can serve TV and movies; music profiles stand alone." onchange={setKinds} />

      <section class="part" aria-labelledby="pf-q">
        <h3 id="pf-q">Qualities</h3>
        <OrderedListEditor items={ladder} label="Allowed qualities" hint="Top is most wanted. Switch a quality on to allow it; group qualities that count the same."
          toggleable groupable {readonly} onchange={onladder} />
        {#if refused?.fields.groups}<p class="err">{refused.fields.groups}</p>{/if}
        <div class="two">
          <EnumField label="Upgrade until" value={draft.cutoff} options={cutoffOptions(draft.groups, qualities)} disabled={readonly || !draft.upgrades}
            error={refused?.fields.cutoff} onchange={(v) => draft && (draft.cutoff = v)} />
          <BoolField label="Upgrade files" bind:checked={draft.upgrades} disabled={readonly}
            help="Replace files with better releases until the quality and score targets are met." />
        </div>
      </section>

      <section class="part" aria-labelledby="pf-f">
        <h3 id="pf-f">Custom formats</h3>
        <div class="three">
          <NumberField label="Minimum score" value={draft.minFormatScore} disabled={readonly} help="Releases scoring less are rejected."
            onchange={(v) => draft && (draft.minFormatScore = v ?? 0)} />
          <NumberField label="Upgrade until score" value={draft.cutoffFormatScore} disabled={readonly}
            onchange={(v) => draft && (draft.cutoffFormatScore = v ?? 0)} />
          <NumberField label="Smallest upgrade step" value={draft.minUpgradeFormatScore ?? 1} min={1} disabled={readonly}
            error={refused?.fields.minUpgradeFormatScore} onchange={(v) => draft && (draft.minUpgradeFormatScore = v ?? 1)} />
        </div>
        {#if formats.length}
          <ul class="scores" aria-label="Format scores">
            {#each formats as f (f.key)}
              <li>
                <span class="fname">{f.name}</span>
                <NumberField label="Score for {f.name}" value={scoreOf(f.key)?.score ?? 0} disabled={readonly || !!scoreOf(f.key)?.reject}
                  onchange={(v) => setScore(f.key, { score: v ?? 0 })} />
                <Checkbox label="Reject" checked={scoreOf(f.key)?.reject ?? false} disabled={readonly}
                  onchange={(e) => setScore(f.key, { reject: (e.currentTarget as HTMLInputElement).checked })} />
              </li>
            {/each}
          </ul>
        {:else}<p class="muted">No custom formats yet. Add some under Custom formats to score releases.</p>{/if}
        {#if refused?.fields.formats}<p class="err">{refused.fields.formats}</p>{/if}
      </section>

      <section class="part" aria-labelledby="pf-s">
        <h3 id="pf-s">Size limits</h3>
        <p class="muted">In MB per minute of runtime. Qualities without a limit here use the global limits.</p>
        {#each draft.sizes ?? [] as s, i (s.quality)}
          <div class="size">
            <span class="fname">{names.get(s.quality) ?? s.quality}</span>
            <NumberField label="Smallest" value={s.min} min={0} step={0.5} unit="MB/min" disabled={readonly} onchange={(v) => draft?.sizes && (draft.sizes[i].min = v ?? 0)} />
            <NumberField label="Aim for" value={s.preferred ?? null} min={0} step={0.5} unit="MB/min" disabled={readonly} onchange={(v) => draft?.sizes && (draft.sizes[i].preferred = v ?? 0)} />
            <NumberField label="Largest" value={s.max} min={0} step={0.5} unit="MB/min" disabled={readonly} onchange={(v) => draft?.sizes && (draft.sizes[i].max = v ?? 0)} />
            {#if !readonly}<IconButton icon="trash" label="Remove the limit for {names.get(s.quality) ?? s.quality}" onclick={() => draft && (draft.sizes = draft.sizes?.filter((_, j) => j !== i))} />{/if}
          </div>
        {/each}
        {#if refused?.fields.sizes}<p class="err">{refused.fields.sizes}</p>{/if}
        {#if !readonly}
          <div class="add">
            <EnumField label="Add a limit for" bind:value={sizeQuality} placeholder="Choose a quality"
              options={allowed.filter((q) => !draft?.sizes?.some((s) => s.quality === q)).map((q) => ({ value: q, label: names.get(q) ?? q }))} />
            <Button size="sm" icon="plus" disabled={!sizeQuality} onclick={addSize}>Add limit</Button>
          </div>
        {/if}
      </section>
    </div>
  {/if}
  {#snippet actions()}
    <Button variant="ghost" onclick={close}>{readonly ? 'Close' : 'Cancel'}</Button>
    {#if !readonly}<Button variant="primary" loading={saving} onclick={() => void save()}>Save profile</Button>{/if}
  {/snippet}
</Dialog>

<ConfirmDialog open={!!deleting} title="Delete {deleting?.name ?? ''}?" confirmLabel="Delete profile" loading={busy} undoable={false}
  consequences={['The profile is removed.', 'A profile still used by series, movies or artists cannot be deleted.']}
  onconfirm={() => void confirmDelete()} oncancel={() => (deleting = undefined)} />

<style>
  .dialog { display: grid; gap: var(--space-4); min-width: 0; }
  .part { display: grid; gap: var(--space-3); min-width: 0; }
  h3 { font-size: var(--text-md); font-weight: var(--weight-medium); }
  .two { display: grid; grid-template-columns: repeat(auto-fit, minmax(14rem, 1fr)); gap: var(--space-3); align-items: start; }
  .three { display: grid; grid-template-columns: repeat(auto-fit, minmax(11rem, 1fr)); gap: var(--space-3); align-items: start; }
  .scores { display: grid; gap: var(--space-2); list-style: none; padding: 0; margin: 0; }
  .scores li, .size { display: grid; grid-template-columns: minmax(8rem, 1fr) minmax(7rem, 9rem) auto; gap: var(--space-3); align-items: end; }
  .size { grid-template-columns: minmax(7rem, 1fr) repeat(3, minmax(6rem, 8rem)) auto; }
  @media (max-width: 40rem) { .scores li, .size { grid-template-columns: 1fr; } }
  .add { display: flex; gap: var(--space-2); align-items: flex-end; flex-wrap: wrap; }
  .fname { font-weight: var(--weight-medium); overflow-wrap: anywhere; align-self: center; }
  .muted { color: var(--ink-muted); }
  .err { color: var(--tone-error-ink); font-size: var(--text-sm); }
  code { font-family: var(--font-mono); font-size: var(--text-sm); overflow-wrap: anywhere; }
  .name { all: unset; cursor: pointer; color: var(--ink); font-weight: var(--weight-medium); margin-inline-end: var(--space-2); }
  .name:hover { text-decoration: underline; }
  .name:focus-visible { outline: var(--focus-w) solid var(--focus-ring); outline-offset: var(--focus-offset); }
</style>
