<!--
  Page composition (story only): S05 add movie dialog over Library › Movies. Provider matches are
  ranked like the variant picker (rank, best first, key reason), then the options form.
-->
<script lang="ts">
  import LibraryMovies from './LibraryMovies.svelte';
  import Artwork from '../lib/components/atoms/Artwork/Artwork.svelte';
  import Button from '../lib/components/atoms/Button/Button.svelte';
  import Kbd from '../lib/components/atoms/Kbd/Kbd.svelte';
  import Spinner from '../lib/components/atoms/Spinner/Spinner.svelte';
  import StatusBadge from '../lib/components/atoms/StatusBadge/StatusBadge.svelte';
  import Switch from '../lib/components/atoms/Switch/Switch.svelte';
  import TextInput from '../lib/components/atoms/TextInput/TextInput.svelte';
  import EnumField from '../lib/components/molecules/EnumField/EnumField.svelte';
  import ErrorState from '../lib/components/molecules/ErrorState/ErrorState.svelte';
  import Field from '../lib/components/molecules/Field/Field.svelte';
  import PathField from '../lib/components/molecules/PathField/PathField.svelte';
  import Dialog from '../lib/components/organisms/Dialog/Dialog.svelte';
  import { rootFolders } from '../lib/storybook/fixtures';

  interface Props { phase?: 'results' | 'searching' | 'offline'; query?: string }
  let { phase = 'results', query = 'dockside' }: Props = $props();

  const matches = [
    { id: 'tmdb:10101', title: 'Dockside', year: 2026, facts: 'Movie · 1h 52m · Harbourline Pictures', reason: 'Exact title and year', inLibrary: false },
    { id: 'tmdb:9302', title: 'Dockside', year: 1987, facts: 'Movie · 1h 38m · Pier Films', reason: 'Same title, earlier film', inLibrary: true },
    { id: 'tmdb:55120', title: 'Dockside Stories', year: 2019, facts: 'Documentary · 54m', reason: 'Partial title match', inLibrary: false },
  ];
  let open = $state(true);
  // svelte-ignore state_referenced_locally
  let q = $state(query);
  let picked = $state('tmdb:10101');
  let monitor = $state(true);
</script>

<LibraryMovies />

<Dialog bind:open title="Add movie" size="lg">
  <div class="add">
    <Field label="Search by name or provider ID">
      {#snippet children(c)}<TextInput type="search" icon="search" id={c.id} aria-describedby={c.describedby} bind:value={q} placeholder="Dockside, or tmdb:10101" />{/snippet}
    </Field>

    {#if phase === 'searching'}
      <p class="searching" role="status"><Spinner size="sm" /><span>Searching TMDB…</span></p>
    {:else if phase === 'offline'}
      <ErrorState size="sm" title="TMDB is not answering" code="metadata.unavailable · 503" healthHref="#/system/health" onretry={() => {}} level={3}>
        You can still add by provider ID once it is back; nothing was added.
      </ErrorState>
    {:else}
      <fieldset class="matches">
        <legend>Matches for “{q}”, best first</legend>
        {#each matches as m, i (m.id)}
          <label class={['opt', { chosen: picked === m.id, dim: m.inLibrary }]}>
            <input type="radio" name="match" value={m.id} bind:group={picked} disabled={m.inLibrary} />
            <span class="rank num" aria-hidden="true">{i + 1}</span>
            <Artwork src={null} title={m.title} kind="movie" width={40} />
            <span class="text">
              <span class="top"><span class="name">{m.title} <span class="year num">({m.year})</span></span>
                {#if m.inLibrary}<StatusBadge family="item" value="cutoff-met" size="sm" detail="in library" />{/if}</span>
              <span class="facts">{m.facts}</span>
              <span class="reason">{m.inLibrary ? 'Already in the library: open it instead.' : m.reason} · <span class="mono">{m.id}</span></span>
            </span>
          </label>
        {/each}
      </fieldset>

      <div class="options">
        <PathField label="Root folder" locations={rootFolders} value="/data/media/movies" />
        <EnumField label="Quality profile" value="hd" options={[{ value: 'hd', label: 'HD' }, { value: 'uhd', label: 'UHD' }, { value: 'any', label: 'Any' }]} />
        <EnumField label="Minimum availability" value="digital" options={[
          { value: 'announced', label: 'Announced' }, { value: 'cinemas', label: 'In cinemas (3 Oct 2026)' }, { value: 'digital', label: 'Digital release (14 Nov 2026)' },
        ]} />
        <div class="switch"><Switch label="Monitor for new releases and upgrades" bind:checked={monitor} /></div>
      </div>
    {/if}
  </div>
  {#snippet actions()}
    <span class="keys" aria-hidden="true"><Kbd keys={['enter']} /> add · <Kbd keys={['mod', 'enter']} /> add and search</span>
    <Button onclick={() => (open = false)}>Cancel</Button>
    <Button disabled={phase !== 'results'}>Add</Button>
    <Button variant="primary" icon="search" disabled={phase !== 'results'}>Add and search</Button>
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
  .mono { font-family: var(--font-mono); font-size: var(--text-xs); }
  .options { display: grid; gap: var(--space-4); grid-template-columns: repeat(auto-fit, minmax(min(100%, 15rem), 1fr)); }
  .switch { grid-column: 1 / -1; }
  .keys { margin-right: auto; font-size: var(--text-xs); color: var(--ink-subtle); display: inline-flex; align-items: center; gap: var(--space-1); }
  @media (width < 48rem), (pointer: coarse) {
    .keys { display: none; }
    .opt { min-height: var(--target-min); }
  }
</style>
