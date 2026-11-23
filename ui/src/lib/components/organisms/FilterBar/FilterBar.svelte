<!--
  FilterBar: text filter plus facets for every list screen, active filters as removable
  chips, and Clear all. The state is plain data that round-trips through the URL with
  `filterToParams` / `filterFromParams` (lib/filters/url), so the page owns the query string.
  `/` focuses the text filter from anywhere on the page.
-->
<script lang="ts">
  import type { Snippet } from 'svelte';
  import TextInput from '../../atoms/TextInput/TextInput.svelte';
  import Button from '../../atoms/Button/Button.svelte';
  import Checkbox from '../../atoms/Checkbox/Checkbox.svelte';
  import Radio from '../../atoms/Radio/Radio.svelte';
  import Chip from '../../atoms/Chip/Chip.svelte';
  import Icon from '../../atoms/Icon/Icon.svelte';
  import Kbd from '../../atoms/Kbd/Kbd.svelte';
  import { activeCount, type FilterFacet, type FilterState } from '../../../filters/url';
  import { formatCount } from '../../../format/quantity';

  interface Props {
    facets?: FilterFacet[];
    value?: FilterState;
    /** Accessible name of the text filter: "Filter series". */
    searchLabel?: string;
    placeholder?: string;
    /** Result summary: "412 series", "12 of 412 series". */
    summary?: string;
    /** Bind `/` to the text filter. Turn off when two bars share a page. */
    shortcut?: boolean;
    onchange?: (value: FilterState) => void;
    /** Controls after the facets (view switch, sort). */
    trailing?: Snippet;
  }
  let {
    facets = [], value = $bindable({ q: '', facets: {} }), searchLabel = 'Filter', placeholder = 'Filter by title',
    summary, shortcut = true, onchange, trailing,
  }: Props = $props();

  const id = $props.id();
  let input = $state<HTMLElement>();
  let openFacet = $state<string>();
  const panels: Record<string, HTMLElement> = {};
  const triggers: Record<string, HTMLElement> = {};
  let pos = $state({ top: 0, left: 0 });

  function emit(next: FilterState) {
    value = next;
    onchange?.(next);
  }
  const selected = (f: FilterFacet) => value.facets[f.id] ?? [];
  function setFacet(f: FilterFacet, values: string[]) {
    const facetsNext = { ...value.facets };
    if (values.length) facetsNext[f.id] = values; else delete facetsNext[f.id];
    emit({ ...value, facets: facetsNext });
  }
  function toggleOption(f: FilterFacet, v: string, on: boolean) {
    const cur = selected(f);
    setFacet(f, f.multiple === false ? (on ? [v] : []) : on ? [...cur.filter((x) => x !== v), v] : cur.filter((x) => x !== v));
  }
  const labelOf = (f: FilterFacet, v: string) => f.options.find((o) => o.value === v)?.label ?? v;
  const chips = $derived(facets.flatMap((f) => selected(f).map((v) => ({ f, v }))));
  const anyActive = $derived(activeCount(value) > 0);

  function place(fid: string) {
    const t = triggers[fid], p = panels[fid];
    if (!t || !p) return;
    const r = t.getBoundingClientRect();
    const w = p.offsetWidth, h = p.offsetHeight;
    const below = r.bottom + 4 + h <= innerHeight || r.top < h + 4;
    pos = { top: below ? r.bottom + 4 : r.top - 4 - h, left: Math.max(8, Math.min(r.left, innerWidth - w - 8)) };
  }
  function ontoggle(e: Event, fid: string) {
    const open = (e as ToggleEvent).newState === 'open';
    openFacet = open ? fid : openFacet === fid ? undefined : openFacet;
    if (open) {
      place(fid);
      panels[fid]?.querySelector<HTMLElement>('input')?.focus();
    }
  }
  function onpanelkey(e: KeyboardEvent, fid: string) {
    if (e.key === 'Escape') { panels[fid]?.hidePopover?.(); triggers[fid]?.focus(); }
  }

  function ondocumentkey(e: KeyboardEvent) {
    if (!shortcut || e.key !== '/' || e.ctrlKey || e.metaKey || e.altKey) return;
    const t = e.target as HTMLElement | null;
    if (t && (t.isContentEditable || /^(INPUT|TEXTAREA|SELECT)$/.test(t.tagName))) return;
    e.preventDefault();
    input?.querySelector('input')?.focus();
  }
</script>

<svelte:document onkeydown={ondocumentkey} />

<div class="fb" role="search" aria-label={searchLabel}>
  <div class="bar">
    <div class="search" bind:this={input}>
      <TextInput
        type="search"
        icon="search"
        aria-label={searchLabel}
        aria-keyshortcuts={shortcut ? '/' : undefined}
        {placeholder}
        value={value.q}
        oninput={(e) => emit({ ...value, q: (e.currentTarget as HTMLInputElement).value })}
      />
      {#if shortcut && !value.q}<span class="kbd" aria-hidden="true"><Kbd keys={['/']} /></span>{/if}
    </div>
    {#each facets as f (f.id)}
      {@const n = selected(f).length}
      <button
        type="button"
        class="facet"
        class:on={n > 0}
        popovertarget="{id}-{f.id}"
        aria-expanded={openFacet === f.id}
        aria-haspopup="true"
        bind:this={triggers[f.id]}
      >
        <span>{f.label}</span>
        {#if n}<span class="count num" aria-label="{n} selected">{n}</span>{/if}
        <Icon name="chevron-down" size="sm" />
      </button>
      <!-- svelte-ignore a11y_no_noninteractive_element_interactions (Escape from the options inside returns focus to the trigger) -->
      <div
        class="panel"
        id="{id}-{f.id}"
        popover="auto"
        role="group"
        aria-label={f.label}
        bind:this={panels[f.id]}
        style:top="{pos.top}px"
        style:left="{pos.left}px"
        ontoggle={(e) => ontoggle(e, f.id)}
        onkeydown={(e) => onpanelkey(e, f.id)}
      >
        <div class="opts">
          {#each f.options as o (o.value)}
            <div class="opt">
              {#if f.multiple === false}
                <Radio name="{id}-{f.id}" value={o.value} label={o.label} group={selected(f)[0] ?? ''}
                  onchange={() => toggleOption(f, o.value, true)} />
              {:else}
                <Checkbox label={o.label} checked={selected(f).includes(o.value)}
                  onchange={(e) => toggleOption(f, o.value, (e.currentTarget as HTMLInputElement).checked)} />
              {/if}
              {#if o.count !== undefined}<span class="oc num">{formatCount(o.count)}</span>{/if}
            </div>
          {/each}
        </div>
        {#if n}<div class="pfoot"><Button size="sm" variant="ghost" onclick={() => setFacet(f, [])}>Clear {f.label.toLowerCase()}</Button></div>{/if}
      </div>
    {/each}
    {#if trailing}<div class="trailing">{@render trailing()}</div>{/if}
  </div>

  {#if chips.length || summary || anyActive}
    <div class="active">
      {#if summary}<span class="summary" role="status">{summary}</span>{/if}
      {#if chips.length}
        <ul class="chips" aria-label="Active filters">
          {#each chips as { f, v } (f.id + v)}
            <li><Chip variant="filter" facet={f.label} label={labelOf(f, v)} onremove={() => toggleOption(f, v, false)} /></li>
          {/each}
        </ul>
      {/if}
      {#if anyActive}<Button size="sm" variant="ghost" icon="x" onclick={() => emit({ q: '', facets: {} })}>Clear all</Button>{/if}
    </div>
  {/if}
</div>

<style>
  .fb { display: grid; gap: var(--space-2); min-width: 0; }
  .bar { display: flex; flex-wrap: wrap; align-items: center; gap: var(--space-2); }
  .search { position: relative; flex: 1 1 16rem; min-width: 0; max-width: 28rem; }
  .kbd { position: absolute; right: var(--space-2); top: 50%; transform: translateY(-50%); pointer-events: none; }
  .facet {
    display: inline-flex; align-items: center; gap: var(--space-1-5); height: var(--control-h-md); padding: 0 var(--space-3);
    font: inherit; font-size: var(--text-md); color: var(--ink); background: var(--surface-raised);
    border: var(--border-w) solid var(--line-strong); border-radius: var(--radius-md); cursor: pointer;
    transition: background-color var(--dur-fast) var(--ease-out);
  }
  .facet:hover { background: var(--surface-hover); }
  .facet.on { border-color: var(--accent); background: var(--accent-tint); color: var(--accent-ink); }
  .count { min-width: 1.25rem; padding: 0 var(--space-1); border-radius: var(--radius-sm); background: var(--accent); color: var(--ink-on-accent); font-size: var(--text-xs); font-weight: var(--weight-semibold); text-align: center; line-height: 1.25rem; }
  .panel {
    position: fixed; inset: auto; margin: 0; min-width: 14rem; max-width: min(20rem, calc(100vw - 16px)); max-height: 60vh; overflow: auto;
    padding: var(--space-2); background: var(--surface-overlay); color: var(--ink);
    border: var(--border-w) solid var(--line); border-radius: var(--radius-md); box-shadow: var(--shadow-sm);
  }
  .opts { display: grid; gap: var(--space-0-5); }
  .opt { display: flex; align-items: center; justify-content: space-between; gap: var(--space-3); padding: var(--space-1) var(--space-1-5); border-radius: var(--radius-sm); }
  .opt:hover { background: var(--surface-hover); }
  .oc { font-size: var(--text-xs); color: var(--ink-muted); }
  .pfoot { border-top: var(--border-w) solid var(--line); margin-top: var(--space-2); padding-top: var(--space-1); }
  .trailing { margin-left: auto; display: flex; align-items: center; gap: var(--space-2); }
  .active { display: flex; flex-wrap: wrap; align-items: center; gap: var(--space-2); min-height: var(--control-h-sm); }
  .summary { font-size: var(--text-sm); color: var(--ink-muted); margin-right: var(--space-1); }
  .chips { display: contents; list-style: none; }
  @media (width < 48rem), (pointer: coarse) {
    .search { flex-basis: 100%; max-width: none; }
    .kbd { display: none; }
    .facet { height: var(--target-min); }
    .opt { min-height: var(--target-min); }
  }
</style>
