<!--
  Add movie (Dialog): look up by name or tmdb:/imdb: id, pick a match, choose the root
  folder, quality profile, minimum availability and monitoring, then add (Enter) or add and search
  (Ctrl/Cmd+Enter).
-->
<script lang="ts">
  import { artSrc } from '../../art';
  import { onMount } from 'svelte';
  import Artwork from '../../../lib/components/atoms/Artwork/Artwork.svelte';
  import Button from '../../../lib/components/atoms/Button/Button.svelte';
  import Kbd from '../../../lib/components/atoms/Kbd/Kbd.svelte';
  import Spinner from '../../../lib/components/atoms/Spinner/Spinner.svelte';
  import StatusBadge from '../../../lib/components/atoms/StatusBadge/StatusBadge.svelte';
  import Switch from '../../../lib/components/atoms/Switch/Switch.svelte';
  import TextInput from '../../../lib/components/atoms/TextInput/TextInput.svelte';
  import EnumField from '../../../lib/components/molecules/EnumField/EnumField.svelte';
  import ErrorState from '../../../lib/components/molecules/ErrorState/ErrorState.svelte';
  import EmptyState from '../../../lib/components/molecules/EmptyState/EmptyState.svelte';
  import Field from '../../../lib/components/molecules/Field/Field.svelte';
  import PathField from '../../../lib/components/molecules/PathField/PathField.svelte';
  import Dialog from '../../../lib/components/organisms/Dialog/Dialog.svelte';
  import { formatAbsolute } from '../../../lib/format/time';
  import { app } from '../../context.svelte';
  import { Resource } from '../../resource.svelte';
  import { ApiError, toApiError } from '../../api/client';
  import type { Movie, MovieRecord, RootFolder } from '../../api/types';

  let { onclose }: { onclose: () => void } = $props();
  const { api, movies, profiles, toasts, navigate } = app();

  let open = $state(true);
  let term = $state('');
  let results = $state<MovieRecord[]>([]);
  let searching = $state(false);
  let lookupError = $state<ApiError | undefined>();
  let picked = $state<string | undefined>();
  let rootFolder = $state('');
  let profileKey = $state('');
  let minAvailability = $state('released');
  let monitored = $state(true);
  let saving = $state<'add' | 'search' | undefined>();
  let fieldErrors = $state<Record<string, string>>({});
  let saveError = $state<string | undefined>();

  const roots = new Resource((s) => api.list<RootFolder>('rootfolders', undefined, s));
  onMount(() => {
    void roots.load();
    movies.ensure();
    if (!profiles.data) void profiles.load();
  });
  $effect(() => {
    if (!rootFolder && roots.data) rootFolder = roots.data.find((r) => r.kind === 'movie' && r.accessible)?.path ?? roots.data.find((r) => r.kind === 'movie')?.path ?? '';
  });
  $effect(() => {
    if (!profileKey && profiles.data?.length) profileKey = (profiles.data.find((p) => p.isDefault) ?? profiles.data[0]).key;
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
        const r = await api.list<MovieRecord>('movies/lookup', { term: q }, c.signal);
        if (c.signal.aborted) return;
        results = r;
        lookupError = undefined;
        if (!r.some((x) => idOf(x) === picked)) { const first = r.find((x) => !inLibrary(x)); picked = first ? idOf(first) : undefined; }
      } catch (e) {
        if (!c.signal.aborted) { lookupError = toApiError(e); results = []; }
      } finally {
        if (!c.signal.aborted) searching = false;
      }
    }, 350);
    return () => { clearTimeout(t); c.abort(); };
  });

  const idOf = (r: MovieRecord) => `tmdb:${r.ids.tmdb}`;
  const owned = (r: MovieRecord): Movie | undefined => movies.rows.find((m) => String(m.tmdbId) === r.ids.tmdb);
  const inLibrary = (r: MovieRecord) => !!owned(r);
  const choice = $derived(results.find((r) => idOf(r) === picked));

  const day = (s?: string) => (s ? formatAbsolute(new Date(s)).replace(/,.*$/, '') : undefined);
  const availOptions = $derived([
    { value: 'announced', label: 'Announced', description: 'Search as soon as it is listed.' },
    { value: 'inCinemas', label: choice?.inCinemas ? `In cinemas (${day(choice.inCinemas)})` : 'In cinemas' },
    { value: 'released', label: choice?.digital || choice?.physical ? `Released (${day(choice.digital ?? choice.physical)})` : 'Released (digital or disc)' },
  ]);
  const locations = $derived((roots.data ?? []).filter((r) => r.kind === 'movie').map((r) => ({
    path: r.path, label: 'Movies', free: r.freeSpace || undefined, access: r.accessible ? 'writable' as const : 'unreachable' as const,
    problem: r.accessible ? undefined : 'corsarr cannot reach this folder',
  })));
  const facts = (r: MovieRecord) => ['Movie', r.runtimeMin ? `${Math.floor(r.runtimeMin / 60)}h ${r.runtimeMin % 60}m` : '', r.genres?.slice(0, 2).join(', ') ?? '', r.status ?? '']
    .filter(Boolean).join(' · ');

  async function add(searchNow: boolean) {
    if (!choice || saving) return;
    saving = searchNow ? 'search' : 'add';
    fieldErrors = {};
    saveError = undefined;
    try {
      const m = await api.post<Movie>('movies', { tmdbId: Number(choice.ids.tmdb), rootFolder, profileKey, monitored, minAvailability, searchNow });
      movies.upsert(m);
      toasts.push({ message: `Added ${m.title}`, detail: searchNow ? 'Searching for a release now.' : 'It shows up in Wanted once it is available.', tone: 'ok',
        actionLabel: 'Open', onaction: () => navigate(`/library/movies/${m.id}`) });
      close();
    } catch (e) {
      const x = toApiError(e);
      if (x.field) fieldErrors = { [x.field]: x.message };
      else saveError = x.message;
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
    if (t.tagName === 'BUTTON' || t.tagName === 'TEXTAREA' || t.tagName === 'SELECT') return;
    e.preventDefault();
    void add(e.ctrlKey || e.metaKey);
  }
</script>

<Dialog bind:open title="Add movie" size="lg" onclose={() => onclose()}>
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div class="add" {onkeydown}>
    <Field label="Search by name or provider ID" help="A title, or an id such as tmdb:603 or imdb:tt0133093.">
      {#snippet children(c)}
        <!-- svelte-ignore a11y_autofocus -->
        <TextInput type="search" icon="search" id={c.id} aria-describedby={c.describedby} bind:value={term} placeholder="The Matrix, or tmdb:603" autofocus autocomplete="off" />
      {/snippet}
    </Field>

    {#if searching && !results.length}
      <p class="searching" role="status"><Spinner size="sm" /><span>Looking up “{term.trim()}”…</span></p>
    {:else if lookupError}
      <ErrorState size="sm" title={lookupError.code === 'no_metadata' ? 'No movie metadata source is ready' : 'The lookup failed'} code={lookupError.label}
        healthHref="#/system/health" onretry={() => { const t = term; term = ''; queueMicrotask(() => (term = t)); }} level={3}>
        {lookupError.code === 'no_metadata' ? 'Configure TMDB with a valid API key, then try again. Nothing was added.' : `${lookupError.message} Nothing was added.`}
      </ErrorState>
    {:else if term.trim().length >= 2 && !results.length && !searching}
      <EmptyState size="sm" title="No matches for “{term.trim()}”" icon="search" level={3}>Check the spelling, or paste the TMDB id as tmdb:603.</EmptyState>
    {:else if results.length}
      <fieldset class="matches" aria-busy={searching}>
        <legend>Matches for “{term.trim()}”, best first</legend>
        {#each results.slice(0, 8) as m, i (idOf(m))}
          {@const own = owned(m)}
          <label class={['opt', { chosen: picked === idOf(m), dim: !!own }]}>
            <input type="radio" name="match" value={idOf(m)} bind:group={picked} disabled={!!own} />
            <span class="rank num" aria-hidden="true">{i + 1}</span>
            <Artwork src={artSrc(m.posterUrl, 40)} title={m.title} kind="movie" width={40} />
            <span class="text">
              <span class="top"><span class="name">{m.title}{#if m.year}{' '}<span class="year num">({m.year})</span>{/if}</span>
                {#if own}<StatusBadge family="item" value="cutoff-met" size="sm" detail="in library" />{/if}</span>
              <span class="facts">{facts(m)}</span>
              <span class="reason">{#if own}Already in the library: <a href="#/library/movies/{own.id}" onclick={close}>open it</a>{:else}{m.overview ? m.overview.slice(0, 120) + (m.overview.length > 120 ? '…' : '') : 'No overview'}{/if} · <span class="mono">{idOf(m)}</span></span>
            </span>
          </label>
        {/each}
      </fieldset>
    {/if}

    {#if choice}
      <div class="options">
        <PathField label="Root folder" locations={locations} bind:value={rootFolder} placeholder="/data/media/movies" error={fieldErrors.rootFolder} required
          help={locations.length ? undefined : 'No movie root folder is configured; add rootFolders to the config.'} />
        <EnumField label="Quality profile" bind:value={profileKey} error={fieldErrors.profileKey} options={(profiles.data ?? []).map((p) => ({ value: p.key, label: p.name }))} />
        <EnumField label="Minimum availability" bind:value={minAvailability} error={fieldErrors.minAvailability} options={availOptions} />
        <div class="switch"><Switch label="Monitor for new releases and upgrades" bind:checked={monitored} /></div>
      </div>
      {#if saveError}<p class="err" role="alert">{saveError}</p>{/if}
    {/if}
  </div>
  {#snippet actions()}
    <span class="keys" aria-hidden="true"><Kbd keys={['enter']} /> add · <Kbd keys={['mod', 'enter']} /> add and search</span>
    <Button onclick={close}>Cancel</Button>
    <Button disabled={!choice || !!saving} loading={saving === 'add'} onclick={() => add(false)}>Add</Button>
    <Button variant="primary" icon="search" disabled={!choice || !!saving} loading={saving === 'search'} onclick={() => add(true)}>Add and search</Button>
  {/snippet}
</Dialog>

<style>
  .add { display: grid; gap: var(--space-5); }
  .searching { display: flex; align-items: center; gap: var(--space-2); color: var(--ink-muted); }
  fieldset { border: 0; margin: 0; padding: 0; display: grid; gap: var(--space-2); min-width: 0; }
  legend { font-size: var(--text-sm); font-weight: var(--weight-medium); color: var(--ink-muted); margin-bottom: var(--space-2); }
  .opt {
    display: grid; grid-template-columns: auto auto auto minmax(0, 1fr); gap: var(--space-2) var(--space-3); align-items: start;
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
  .year { font-weight: var(--weight-regular); color: var(--ink-muted); }
  .facts, .reason { font-size: var(--text-sm); color: var(--ink-muted); }
  .reason a { color: var(--accent-ink); }
  .mono { font-family: var(--font-mono); font-size: var(--text-xs); }
  /* Two equal columns, fields top-aligned; the root folder picker spans the row. */
  .options { display: grid; gap: var(--space-4); grid-template-columns: repeat(2, minmax(0, 1fr)); align-items: start; }
  .options > :global(:first-child) { grid-column: 1 / -1; }
  @media (max-width: 40rem) { .options { grid-template-columns: minmax(0, 1fr); } }
  .switch { grid-column: 1 / -1; }
  .err { color: var(--tone-error-ink); font-size: var(--text-md); }
  .keys { margin-right: auto; font-size: var(--text-xs); color: var(--ink-subtle); display: inline-flex; align-items: center; gap: var(--space-1); }
  @media (width < 48rem), (pointer: coarse) {
    .keys { display: none; }
    .opt { min-height: var(--target-min); }
  }
</style>
