<!--
  Add artist (Dialog): look up by name or mbid:, pick a match (disambiguation, type, country and
  start year tell same-named artists apart), choose the root folder, quality and metadata
  profiles, monitoring and new-album handling and tags, then add (Enter) or add and search
  wanted albums (Ctrl/Cmd+Enter).
-->
<script lang="ts">
  import { onMount } from 'svelte';
  import Button from '../../../lib/components/atoms/Button/Button.svelte';
  import Kbd from '../../../lib/components/atoms/Kbd/Kbd.svelte';
  import Spinner from '../../../lib/components/atoms/Spinner/Spinner.svelte';
  import StatusBadge from '../../../lib/components/atoms/StatusBadge/StatusBadge.svelte';
  import TextInput from '../../../lib/components/atoms/TextInput/TextInput.svelte';
  import EnumField from '../../../lib/components/molecules/EnumField/EnumField.svelte';
  import ErrorState from '../../../lib/components/molecules/ErrorState/ErrorState.svelte';
  import EmptyState from '../../../lib/components/molecules/EmptyState/EmptyState.svelte';
  import Field from '../../../lib/components/molecules/Field/Field.svelte';
  import ListField from '../../../lib/components/molecules/ListField/ListField.svelte';
  import PathField from '../../../lib/components/molecules/PathField/PathField.svelte';
  import Dialog from '../../../lib/components/organisms/Dialog/Dialog.svelte';
  import { app } from '../../context.svelte';
  import { Resource } from '../../resource.svelte';
  import { ApiError, toApiError } from '../../api/client';
  import type { Artist, ArtistRecord, MusicProfiles, RootFolder } from '../../api/types';
  import { albumMonitorModes, monitorNewOptions } from '../../music';

  let { profiles, onclose, onadded }: { profiles?: MusicProfiles; onclose: () => void; onadded?: (a: Artist) => void } = $props();
  const { api, toasts, navigate } = app();

  let open = $state(true);
  let term = $state('');
  let results = $state<ArtistRecord[]>([]);
  let searching = $state(false);
  let lookupError = $state<ApiError | undefined>();
  let picked = $state<string | undefined>();
  let rootFolder = $state('');
  let profileKey = $state('');
  let metadataProfile = $state('standard');
  let monitor = $state<string>('all');
  let monitorNew = $state<string>('all');
  let tags = $state<string[]>([]);
  let saving = $state<'add' | 'search' | undefined>();
  let fieldErrors = $state<Record<string, string>>({});
  let saveError = $state<string | undefined>();

  const roots = new Resource((s) => api.list<RootFolder>('rootfolders', undefined, s));
  onMount(() => void roots.load());
  $effect(() => {
    if (!rootFolder && roots.data) rootFolder = roots.data.find((r) => r.kind === 'music' && r.accessible)?.path ?? roots.data.find((r) => r.kind === 'music')?.path ?? '';
  });
  $effect(() => {
    if (!profileKey && profiles?.quality.length) profileKey = (profiles.quality.find((p) => p.isDefault) ?? profiles.quality[0]).key;
  });

  // Look up 350 ms after typing stops; a newer term cancels the older request.
  let ctl: AbortController | undefined;
  $effect(() => {
    const q = term.trim();
    ctl?.abort();
    if (q.length < 2) { results = []; searching = false; lookupError = undefined; return; }
    const c = new AbortController();
    ctl = c;
    searching = true;
    const t = setTimeout(async () => {
      try {
        const r = await api.list<ArtistRecord>('artists/lookup', { term: q }, c.signal);
        if (c.signal.aborted) return;
        results = r;
        lookupError = undefined;
        if (!r.some((x) => x.mbid === picked)) picked = r.find((x) => !x.libraryId)?.mbid;
      } catch (e) {
        if (!c.signal.aborted) { lookupError = toApiError(e); results = []; }
      } finally {
        if (!c.signal.aborted) searching = false;
      }
    }, 350);
    return () => { clearTimeout(t); c.abort(); };
  });

  const choice = $derived(results.find((r) => r.mbid === picked && !r.libraryId));
  const locations = $derived((roots.data ?? []).filter((r) => r.kind === 'music').map((r) => ({
    path: r.path, label: 'Music', free: r.freeSpace || undefined, access: r.accessible ? 'writable' as const : 'unreachable' as const,
    problem: r.accessible ? undefined : 'corsarr cannot reach this folder',
  })));
  const facts = (r: ArtistRecord) => [r.type ?? '', r.country ?? '', r.beginYear ? `since ${r.beginYear}` : ''].filter(Boolean).join(' · ');

  async function add(searchNow: boolean) {
    if (!choice || saving) return;
    saving = searchNow ? 'search' : 'add';
    fieldErrors = {};
    saveError = undefined;
    try {
      const a = await api.post<Artist>('artists', { mbid: choice.mbid, rootFolder, profileKey, metadataProfile, monitor, monitorNew, tags, searchNow });
      onadded?.(a);
      toasts.push({ message: `Added ${a.name}`, detail: searchNow ? 'Searching for wanted albums now.' : 'Monitored albums show up in Wanted once they are out.', tone: 'ok',
        actionLabel: 'Open', onaction: () => navigate(`/library/music/${a.id}`) });
      close();
    } catch (e) {
      const x = toApiError(e);
      if (x.field) fieldErrors = { [x.field]: x.message };
      else saveError = x.code === 'kind_mode' ? 'Music is turned off in the config, so artists cannot be added. Enable the music media kind first.' : x.message;
    } finally {
      saving = undefined;
    }
  }

  function close() {
    open = false;
    onclose();
  }

  function onkeydown(e: KeyboardEvent) {
    if (e.key !== 'Enter' || !choice) return;
    const t = e.target as HTMLElement;
    if (t.tagName === 'BUTTON' || t.tagName === 'TEXTAREA' || t.tagName === 'SELECT' || t.closest('.tags')) return;
    e.preventDefault();
    void add(e.ctrlKey || e.metaKey);
  }
</script>

<Dialog bind:open title="Add artist" size="lg" onclose={() => onclose()}>
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div class="add" {onkeydown}>
    <Field label="Search by name or MusicBrainz ID" help="A name, or an id such as mbid:5b11f4ce-a62d-471e-81fc-a69a8278c7da.">
      {#snippet children(c)}
        <!-- svelte-ignore a11y_autofocus -->
        <TextInput type="search" icon="search" id={c.id} aria-describedby={c.describedby} bind:value={term} placeholder="Artist name, or mbid:…" autofocus autocomplete="off" />
      {/snippet}
    </Field>

    {#if searching && !results.length}
      <p class="searching" role="status"><Spinner size="sm" /><span>Looking up “{term.trim()}”…</span></p>
    {:else if lookupError}
      <ErrorState size="sm" title={lookupError.code === 'no_metadata' ? 'No music metadata source is ready' : 'The lookup failed'} code={lookupError.label}
        healthHref="#/system/health" onretry={() => { const t = term; term = ''; queueMicrotask(() => (term = t)); }} level={3}>
        {lookupError.code === 'kind_mode' ? 'Music is turned off in the config. Nothing was added.' : `${lookupError.message} Nothing was added.`}
      </ErrorState>
    {:else if term.trim().length >= 2 && !results.length && !searching}
      <EmptyState size="sm" title="No matches for “{term.trim()}”" icon="search" level={3}>Check the spelling, or paste a MusicBrainz id as mbid:….</EmptyState>
    {:else if results.length}
      <fieldset class="matches" aria-busy={searching}>
        <legend>Matches for “{term.trim()}”, best first</legend>
        {#each results.slice(0, 8) as m, i (m.mbid)}
          {@const own = m.libraryId > 0}
          <label class={['opt', { chosen: picked === m.mbid, dim: own }]}>
            <input type="radio" name="match" value={m.mbid} bind:group={picked} disabled={own} />
            <span class="rank num" aria-hidden="true">{i + 1}</span>
            <span class="text">
              <span class="top"><span class="name">{m.name}</span>{#if m.disambiguation}<span class="dis">{m.disambiguation}</span>{/if}
                {#if own}<StatusBadge family="item" value="cutoff-met" size="sm" detail="in library" />{/if}</span>
              <span class="facts">{facts(m) || 'No details on MusicBrainz'}</span>
              <span class="reason">{#if own}Already in the library: <a href="#/library/music/{m.libraryId}" onclick={close}>open it</a> · {/if}<span class="mono">mbid:{m.mbid}</span></span>
            </span>
          </label>
        {/each}
      </fieldset>
    {/if}

    {#if choice}
      <div class="options">
        <PathField label="Root folder" {locations} bind:value={rootFolder} placeholder="/data/media/music" error={fieldErrors.rootFolder} required
          help={locations.length ? undefined : 'No music root folder is configured; add one with kind music to rootFolders in the config.'} />
        <EnumField label="Quality profile" bind:value={profileKey} error={fieldErrors.profileKey} options={(profiles?.quality ?? []).map((p) => ({ value: p.key, label: p.name }))} />
        <EnumField label="Metadata profile" bind:value={metadataProfile} error={fieldErrors.metadataProfile}
          help="Decides which release types are monitored and searched on their own; every album stays visible and can be monitored by hand."
          options={(profiles?.metadata ?? []).map((p) => ({ value: p.key, label: p.name }))} />
        <EnumField label="Monitor" bind:value={monitor} error={fieldErrors.monitor} options={albumMonitorModes}
          help={albumMonitorModes.find((o) => o.value === monitor)?.description} />
        <EnumField label="New albums" bind:value={monitorNew} error={fieldErrors.monitorNew} options={monitorNewOptions}
          help={monitorNewOptions.find((o) => o.value === monitorNew)?.description} />
        <div class="tags"><ListField label="Tags" itemLabel="tag" bind:value={tags} error={fieldErrors.tags} placeholder="vinyl" /></div>
      </div>
      {#if saveError}<p class="err" role="alert">{saveError}</p>{/if}
    {/if}
  </div>
  {#snippet actions()}
    <span class="keys" aria-hidden="true"><Kbd keys={['enter']} /> add · <Kbd keys={['mod', 'enter']} /> add and search</span>
    <Button onclick={close}>Cancel</Button>
    <Button disabled={!choice || !!saving} loading={saving === 'add'} onclick={() => add(false)}>Add</Button>
    <Button variant="primary" icon="search" disabled={!choice || !!saving} loading={saving === 'search'} onclick={() => add(true)}>Add and search wanted</Button>
  {/snippet}
</Dialog>

<style>
  .add { display: grid; gap: var(--space-5); }
  .searching { display: flex; align-items: center; gap: var(--space-2); color: var(--ink-muted); }
  fieldset { border: 0; margin: 0; padding: 0; display: grid; gap: var(--space-2); min-width: 0; }
  legend { font-size: var(--text-sm); font-weight: var(--weight-medium); color: var(--ink-muted); margin-bottom: var(--space-2); }
  .opt {
    display: grid; grid-template-columns: auto auto minmax(0, 1fr); gap: var(--space-2) var(--space-3); align-items: start;
    padding: var(--space-2) var(--space-3); border: var(--border-w) solid var(--line); border-radius: var(--radius-md); cursor: pointer;
    transition: background-color var(--dur-fast) var(--ease-out), border-color var(--dur-fast) var(--ease-out);
  }
  .opt:hover { background: var(--surface-hover); }
  .opt.chosen { background: var(--surface-selected); border-color: var(--accent); }
  .opt.dim { cursor: default; }
  .opt:has(input:focus-visible) { outline: var(--focus-w) solid var(--focus-ring); outline-offset: var(--focus-offset); }
  input { margin: 0.1875rem 0 0; accent-color: var(--accent); }
  input:focus-visible { outline: none; }
  .rank { font-size: var(--text-sm); color: var(--ink-subtle); line-height: 1.5; }
  .text { display: grid; gap: var(--space-0-5); min-width: 0; }
  .top { display: flex; align-items: center; gap: var(--space-2); flex-wrap: wrap; }
  .name { font-weight: var(--weight-semibold); }
  .dis { color: var(--ink-muted); }
  .facts, .reason { font-size: var(--text-sm); color: var(--ink-muted); }
  .reason a { color: var(--accent-ink); }
  .mono { font-family: var(--font-mono); font-size: var(--text-xs); overflow-wrap: anywhere; }
  /* Two equal columns, fields top-aligned; the root folder picker spans the row. */
  .options { display: grid; gap: var(--space-4); grid-template-columns: repeat(2, minmax(0, 1fr)); align-items: start; }
  .options > :global(:first-child) { grid-column: 1 / -1; }
  @media (max-width: 40rem) { .options { grid-template-columns: minmax(0, 1fr); } }
  .tags { grid-column: 1 / -1; }
  .err { color: var(--tone-error-ink); font-size: var(--text-md); }
  .keys { margin-right: auto; font-size: var(--text-xs); color: var(--ink-subtle); display: inline-flex; align-items: center; gap: var(--space-1); }
  @media (width < 48rem), (pointer: coarse) {
    .keys { display: none; }
    .opt { min-height: var(--target-min); }
  }
</style>
