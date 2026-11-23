<!--
  CommandPalette: Ctrl/Cmd+K reaches anything: items, screens, settings objects
  and actions in context. Keyboard first: type to fuzzy-match, ↑/↓ to move, Enter to open,
  Esc to close. With an empty query it shows recent picks first. Item search against the
  API belongs to the caller: listen to `onquery`, pass results in `items` with `loading`.
  Destructive actions still confirm in the caller after `onselect`.
-->
<script lang="ts" module>
  import type { IconName } from '../../atoms/Icon/icons';
  import type { MediaKind } from '../../../tokens';

  export interface PaletteItem {
    id: string;
    label: string;
    /** Section heading: "Actions", "Screens", "Items", "Settings". */
    group: string;
    icon?: IconName;
    /** Media items show their kind icon instead of `icon`. */
    kind?: MediaKind;
    /** Muted text after the label: "TV · 2024", "Settings › Indexers". */
    subtitle?: string;
    /** Extra words that should match: synonyms, old names. */
    keywords?: string[];
    /** Shortcut keys shown at the end (Kbd), e.g. ['g', 'q']. */
    keys?: string[];
    danger?: boolean;
  }
</script>

<script lang="ts">
  import Dialog from '../Dialog/Dialog.svelte';
  import Icon from '../../atoms/Icon/Icon.svelte';
  import Kbd from '../../atoms/Kbd/Kbd.svelte';
  import MediaKindIcon from '../../atoms/MediaKindIcon/MediaKindIcon.svelte';
  import Spinner from '../../atoms/Spinner/Spinner.svelte';
  import { fuzzyMatch, highlight } from '../../../search/fuzzy';

  interface Props {
    open?: boolean;
    items: PaletteItem[];
    /** Ids picked recently, newest first. */
    recent?: string[];
    query?: string;
    loading?: boolean;
    placeholder?: string;
    /** Results per group. */
    limit?: number;
    onselect?: (item: PaletteItem) => void;
    onquery?: (query: string) => void;
    modal?: boolean;
  }
  let {
    open = $bindable(false), items, recent = [], query = $bindable(''), loading = false,
    placeholder = 'Search titles, screens and actions', limit = 8, onselect, onquery, modal = true,
  }: Props = $props();

  const id = $props.id();
  type Row = { item: PaletteItem; group: string; key: string; indices: number[] };

  const groups = $derived.by(() => {
    const q = query.trim();
    const out = new Map<string, Row[]>();
    const add = (group: string, row: Row) => {
      const list = out.get(group) ?? [];
      if (list.length < limit) list.push(row);
      out.set(group, list);
    };
    if (!q) {
      const byId = new Map(items.map((i) => [i.id, i]));
      for (const rid of recent) {
        const item = byId.get(rid);
        if (item) add('Recent', { item, group: 'Recent', key: `r-${item.id}`, indices: [] });
      }
      for (const item of items) add(item.group, { item, group: item.group, key: item.id, indices: [] });
      return [...out];
    }
    const scored = items
      .map((item) => {
        const own = fuzzyMatch(q, item.label);
        const extra = [item.subtitle, ...(item.keywords ?? [])]
          .filter((s): s is string => !!s)
          .map((s) => fuzzyMatch(q, s))
          .reduce<number | null>((best, m) => (m && (best === null || m.score > best) ? m.score : best), null);
        const score = Math.max(own?.score ?? -Infinity, extra === null ? -Infinity : extra - 5);
        return { item, score, indices: own?.indices ?? [] };
      })
      .filter((r) => r.score > -Infinity)
      .sort((a, b) => b.score - a.score);
    for (const r of scored) add(r.item.group, { item: r.item, group: r.item.group, key: r.item.id, indices: r.indices });
    return [...out];
  });
  const flat = $derived(groups.flatMap(([, rows]) => rows));
  let active = $state(0);
  const current = $derived(flat[Math.min(active, flat.length - 1)]);

  function oninput() {
    active = 0;
    onquery?.(query);
  }
  function choose(row: Row) {
    open = false;
    onselect?.(row.item);
  }
  function onkeydown(e: KeyboardEvent) {
    if (!flat.length) return;
    const n = flat.length;
    const moves: Record<string, number> = { ArrowDown: active + 1, ArrowUp: active - 1, Home: 0, End: n - 1, PageDown: active + 5, PageUp: active - 5 };
    if (e.key in moves) {
      if ((e.key === 'Home' || e.key === 'End') && !e.ctrlKey) return; // keep caret movement in the field
      e.preventDefault();
      const next = moves[e.key];
      active = e.key.startsWith('Page') ? Math.max(0, Math.min(n - 1, next)) : (next + n) % n;
      document.getElementById(`${id}-o-${flat[active].key}`)?.scrollIntoView?.({ block: 'nearest' });
    } else if (e.key === 'Enter' && current) {
      e.preventDefault();
      choose(current);
    }
  }
  $effect(() => {
    if (open) active = 0;
  });
</script>

<Dialog bind:open title="Command palette" hideHeader size="lg" {modal} class="cp-dialog">
  <div class="cp">
    <div class="field">
      <Icon name="search" size="md" />
      <input
        type="text" role="combobox" aria-label="Search titles, screens and actions" aria-expanded="true"
        aria-controls="{id}-list" aria-autocomplete="list" aria-activedescendant={current ? `${id}-o-${current.key}` : undefined}
        autocomplete="off" spellcheck="false" {placeholder} bind:value={query} {oninput} {onkeydown} data-autofocus
      />
      {#if loading}<Spinner size="md" label="Searching items" />{/if}
    </div>

    <div class="results" id="{id}-list" role="listbox" aria-label="Results">
      {#each groups as [group, rows], gi (group)}
        <div role="group" aria-labelledby="{id}-g{gi}" class="group">
          <div class="gh" id="{id}-g{gi}" role="presentation">{group}</div>
          {#each rows as row (row.key)}
            {@const isActive = current?.key === row.key}
            <!-- Options are chosen with the field's Enter key or a click; focus stays in the field. -->
            <!-- svelte-ignore a11y_click_events_have_key_events, a11y_interactive_supports_focus -->
            <div
              id="{id}-o-{row.key}" role="option" aria-selected={isActive} class="opt" class:active={isActive} class:danger={row.item.danger}
              onclick={() => choose(row)} onpointermove={() => (active = flat.indexOf(row))}
            >
              <span class="glyph">
                {#if row.item.kind}<MediaKindIcon kind={row.item.kind} />{:else}<Icon name={row.group === 'Recent' ? 'clock' : (row.item.icon ?? 'arrow-right')} size="md" />{/if}
              </span>
              <span class="label">
                {#each highlight(row.item.label, row.indices) as run, ri (ri)}{#if run.hit}<mark>{run.text}</mark>{:else}{run.text}{/if}{/each}
              </span>
              {#if row.item.subtitle}<span class="subtitle">{row.item.subtitle}</span>{/if}
              {#if row.item.keys}<span class="keys"><Kbd keys={row.item.keys.slice(0, 1)} then={row.item.keys.length > 1 ? row.item.keys.slice(1) : undefined} /></span>{/if}
            </div>
          {/each}
        </div>
      {/each}
      {#if !flat.length && !loading}
        <p class="empty">Nothing matches “{query.trim()}”. Try part of a title, a screen such as Queue, or an action such as Pause.</p>
      {/if}
    </div>

    <p class="hints" aria-hidden="true">
      <span><Kbd keys={['up']} /><Kbd keys={['down']} /> move</span>
      <span><Kbd keys={['enter']} /> open</span>
      <span><Kbd keys={['esc']} /> close</span>
    </p>
  </div>
</Dialog>

<style>
  :global(.cp-dialog[open]:modal) { margin-top: min(12vh, 6rem); }
  .cp { display: grid; gap: var(--space-2); }
  .field {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    padding: 0 var(--space-3);
    min-height: var(--control-h-lg);
    color: var(--ink-muted);
    background: var(--surface-raised);
    border: var(--border-w) solid var(--line-strong);
    border-radius: var(--radius-md);
  }
  .field:focus-within { border-color: var(--accent); outline: var(--focus-w) solid var(--focus-ring); outline-offset: var(--focus-offset); }
  input { flex: 1; min-width: 0; height: var(--control-h-lg); font-size: var(--text-base); color: var(--ink); background: none; border: 0; }
  input:focus-visible { outline: none; }

  .results { display: grid; gap: var(--space-2); max-height: min(24rem, 55dvh); overflow: auto; overscroll-behavior: contain; }
  .group { display: grid; }
  .gh { padding: var(--space-2) var(--space-2) var(--space-1); font-size: var(--text-xs); font-weight: var(--weight-medium); color: var(--ink-muted); }
  .opt {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    min-height: var(--control-h-md);
    padding: var(--space-1) var(--space-2);
    font-size: var(--text-md);
    border-radius: var(--radius-sm);
    cursor: pointer;
  }
  .opt.active { background: var(--surface-selected); }
  .glyph { display: grid; place-items: center; width: var(--icon-lg); color: var(--ink-muted); flex: none; }
  .label { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  mark { color: inherit; background: none; font-weight: var(--weight-semibold); text-decoration: underline; text-decoration-color: var(--accent); text-underline-offset: 0.2em; }
  .subtitle { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: var(--text-sm); color: var(--ink-muted); }
  .label + .keys { margin-left: auto; }
  .keys { flex: none; }
  .danger .label, .danger .glyph { color: var(--tone-error-ink); }
  .empty { padding: var(--space-4) var(--space-2); font-size: var(--text-md); color: var(--ink-muted); }
  .hints { display: flex; gap: var(--space-4); padding: var(--space-2) var(--space-1) 0; font-size: var(--text-xs); color: var(--ink-muted); border-top: var(--border-w) solid var(--line); }
  .hints span { display: inline-flex; align-items: center; gap: var(--space-1); }

  @media (width < 48rem), (pointer: coarse) {
    .opt { min-height: var(--target-min); }
    .hints, .keys { display: none; }
  }
</style>
