<!--
  Page composition (story only): S05 add series dialog over Library › TV. Provider matches ranked
  best first (one already in the library links to it), then the options form.
-->
<script lang="ts">
  import LibrarySeries from './LibrarySeries.svelte';
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
  import ListField from '../lib/components/molecules/ListField/ListField.svelte';
  import PathField from '../lib/components/molecules/PathField/PathField.svelte';
  import Dialog from '../lib/components/organisms/Dialog/Dialog.svelte';
  import type { PathLocation } from '../lib/components/molecules/PathPicker/PathPicker.svelte';
  import { monitorModes } from '../app/tv';

  interface Props { phase?: 'results' | 'searching' | 'offline'; query?: string }
  let { phase = 'results', query = 'long haul' }: Props = $props();

  const matches = [
    { id: 'tmdb:84012', title: 'The Long Haul', year: 2019, facts: 'Harbourline · Continuing · 3 seasons · 52m', overview: 'A night-shift crew keeps a container port running while the cargo, the weather and the owners all want something different.', inLibrary: false },
    { id: 'tmdb:40122', title: 'The Long Haul', year: 2004, facts: 'Quay One · Ended · 2 seasons · 45m', overview: 'Two drivers, one lorry and a route that never gets shorter.', inLibrary: true },
    { id: 'tmdb:91500', title: 'Long Haul Kitchens', year: 2023, facts: 'Pier Streaming · Continuing · 1 season · 24m', overview: 'Ship cooks compete across a season at sea.', inLibrary: false },
  ];
  const roots: PathLocation[] = [{ path: '/data/media/tv', label: 'TV', free: 1_240_000_000_000, access: 'writable' }];
  let open = $state(true);
  // svelte-ignore state_referenced_locally
  let q = $state(query);
  let picked = $state('tmdb:84012');
  let monitor = $state('all');
  let folders = $state(true);
  let tags = $state(['drama']);
</script>

<LibrarySeries />

<Dialog bind:open title="Add series" size="lg">
  <div class="add">
    <Field label="Search by name or provider ID" help="A title, or an id such as tmdb:84012, tvdb:371950 or imdb:tt9000001.">
      {#snippet children(c)}<TextInput type="search" icon="search" id={c.id} aria-describedby={c.describedby} bind:value={q} placeholder="Series title, or tvdb:371950" />{/snippet}
    </Field>

    {#if phase === 'searching'}
      <p class="searching" role="status"><Spinner size="sm" /><span>Looking up “{q}”…</span></p>
    {:else if phase === 'offline'}
      <ErrorState size="sm" title="The TV metadata source is not answering" code="upstream · 502" healthHref="#/system/health" onretry={() => {}} level={3}>
        Try again in a moment; nothing was added.
      </ErrorState>
    {:else}
      <fieldset class="matches">
        <legend>Matches for “{q}”, best first</legend>
        {#each matches as m, i (m.id)}
          <label class={['opt', { chosen: picked === m.id, dim: m.inLibrary }]}>
            <input type="radio" name="match" value={m.id} bind:group={picked} disabled={m.inLibrary} />
            <span class="rank num" aria-hidden="true">{i + 1}</span>
            <Artwork src={null} title={m.title} kind="tv" width={40} />
            <span class="text">
              <span class="top"><span class="name">{m.title} <span class="year num">({m.year})</span></span>
                {#if m.inLibrary}<StatusBadge family="item" value="cutoff-met" size="sm" detail="in library" />{/if}</span>
              <span class="facts">{m.facts}</span>
              <span class="reason">{#if m.inLibrary}Already in the library: <a href="#/library/tv/2">open it</a>{:else}{m.overview}{/if} · <span class="mono">{m.id}</span></span>
            </span>
          </label>
        {/each}
      </fieldset>

      <div class="options">
        <PathField label="Root folder" locations={roots} value="/data/media/tv" />
        <EnumField label="Quality profile" value="hd" options={[{ value: 'hd', label: 'HD' }, { value: 'uhd', label: 'UHD' }, { value: 'any', label: 'Any' }]} />
        <EnumField label="Monitor" bind:value={monitor} options={monitorModes} help={monitorModes.find((o) => o.value === monitor)?.description} />
        <EnumField label="New seasons" value="all" options={[{ value: 'all', label: 'Monitor them as they appear' }, { value: 'none', label: 'Leave them unmonitored' }]} />
        <EnumField label="Series type" value="standard" options={[
          { value: 'standard', label: 'Standard (season and episode)' }, { value: 'daily', label: 'Daily (by air date), later', disabled: true },
          { value: 'anime', label: 'Anime (absolute numbers), later', disabled: true },
        ]} />
        <div class="wide"><ListField label="Tags" itemLabel="tag" bind:value={tags} /></div>
        <div class="wide"><Switch label="Put each season in its own folder" bind:checked={folders} /></div>
      </div>
    {/if}
  </div>
  {#snippet actions()}
    <span class="keys" aria-hidden="true"><Kbd keys={['enter']} /> add · <Kbd keys={['mod', 'enter']} /> add and search</span>
    <Button onclick={() => (open = false)}>Cancel</Button>
    <Button disabled={phase !== 'results'}>Add</Button>
    <Button variant="primary" icon="search" disabled={phase !== 'results'}>Add and search missing</Button>
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
  .wide { grid-column: 1 / -1; }
  .keys { margin-right: auto; font-size: var(--text-xs); color: var(--ink-subtle); display: inline-flex; align-items: center; gap: var(--space-1); }
  @media (width < 48rem), (pointer: coarse) {
    .keys { display: none; }
    .opt { min-height: var(--target-min); }
  }
</style>
