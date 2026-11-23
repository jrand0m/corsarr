<!--
  DataTable: the dense list of every list screen. Sortable columns (the page sorts, usually
  on the server), virtualised rows (DOM stays near one viewport for 10 000+ rows),
  cursor paging (`onloadmore` fires as the end comes into view), row selection with the
  BulkActionBar, a density switch, a sticky header, phone list mode below 768 px, and
  keyboard row navigation: j/k or arrows move, Enter opens, x selects, Shift+x selects a range,
  Ctrl/Cmd+A selects all loaded, "." opens the row's actions.

    <DataTable rows={page} {columns} rowKey={(r) => r.id} label="Series" onopen={openDrawer}>
      {#snippet cell(row, col)}{#if col.id === 'status'}<StatusBadge … />{:else}{col.value?.(row)}{/if}{/snippet}
    </DataTable>
-->
<script lang="ts" module>
  export interface Column<T> {
    id: string;
    label: string;
    sortable?: boolean;
    align?: 'start' | 'end';
    /** CSS width of the column ("8rem", "20%"); the rest share what is left. */
    width?: string;
    /** Plain value: the default cell text and the phone list text. */
    value?: (row: T) => string | number | null | undefined;
    /** Phone list role: first line, second line (default), right side, or left out. */
    phone?: 'title' | 'meta' | 'trailing' | 'hidden';
    /** Keep the header visually empty (actions, artwork). */
    hideLabel?: boolean;
  }
  export interface SortState { column: string; direction: 'asc' | 'desc' }
</script>

<script lang="ts" generics="T">
  import { tick, type Snippet } from 'svelte';
  import Checkbox from '../../atoms/Checkbox/Checkbox.svelte';
  import Icon from '../../atoms/Icon/Icon.svelte';
  import Button from '../../atoms/Button/Button.svelte';
  import Skeleton from '../../atoms/Skeleton/Skeleton.svelte';
  import Spinner from '../../atoms/Spinner/Spinner.svelte';
  import ActionMenu, { type ActionItem } from '../../molecules/ActionMenu/ActionMenu.svelte';
  import BulkActionBar from '../../molecules/BulkActionBar/BulkActionBar.svelte';
  import EmptyState from '../../molecules/EmptyState/EmptyState.svelte';
  import ErrorState from '../../molecules/ErrorState/ErrorState.svelte';
  import SegmentedControl from '../../molecules/SegmentedControl/SegmentedControl.svelte';
  import { formatCount } from '../../../format/quantity';
  import type { Density } from '../../../tokens';

  interface Props {
    rows: T[];
    columns: Column<T>[];
    rowKey: (row: T) => string;
    /** Accessible name: "Series". */
    label: string;
    sort?: SortState;
    onsort?: (sort: SortState) => void;
    selectable?: boolean;
    selected?: string[];
    onselectionchange?: (keys: string[]) => void;
    density?: Density;
    densitySwitch?: boolean;
    /** "auto" switches to the list below 768 px. */
    mode?: 'auto' | 'table' | 'list';
    /** Height of the scrolling viewport. */
    height?: string;
    loading?: boolean;
    error?: string;
    errorCode?: string;
    onretry?: () => void;
    hasMore?: boolean;
    loadingMore?: boolean;
    onloadmore?: () => void;
    /** Rows on the server, when known. */
    total?: number;
    noun?: string;
    nounPlural?: string;
    onopen?: (row: T) => void;
    /** Key of the row open in a drawer. */
    current?: string;
    rowActions?: (row: T) => ActionItem[];
    onaction?: (action: string, row: T) => void;
    cell?: Snippet<[T, Column<T>]>;
    bulkActions?: Snippet<[string[]]>;
    empty?: Snippet;
    toolbar?: Snippet;
    /** Row detail shown under an expanded row (reasons, event detail, diffs). Adds a disclosure button per row;
        Right arrow expands and Left arrow collapses the focused row. */
    expand?: Snippet<[T]>;
    /** Keys of expanded rows. */
    expanded?: string[];
    onexpandchange?: (keys: string[]) => void;
  }
  let {
    rows, columns, rowKey, label, sort = $bindable(), onsort, selectable = false, selected = $bindable([]),
    onselectionchange, density = $bindable('comfortable'), densitySwitch = false, mode = 'auto', height = '36rem',
    loading = false, error, errorCode, onretry, hasMore = false, loadingMore = false, onloadmore, total,
    noun = 'row', nounPlural, onopen, current, rowActions, onaction, cell, bulkActions, empty, toolbar,
    expand, expanded = $bindable([]), onexpandchange,
  }: Props = $props();

  const OVERSCAN = 6;
  const id = $props.id();
  let scroller = $state<HTMLElement>();
  let scrollTop = $state(0);
  let viewH = $state(0);
  let headH = $state(0);
  let measured = $state<Record<string, number>>({});
  let focusIndex = $state(0);
  let anchor = $state<number>();

  // Phone list mode follows the viewport unless the caller pins a mode.
  let narrow = $state(typeof matchMedia === 'function' && matchMedia('(width < 48rem)').matches);
  $effect(() => {
    if (typeof matchMedia !== 'function') return;
    const mq = matchMedia('(width < 48rem)');
    const on = () => (narrow = mq.matches);
    mq.addEventListener?.('change', on);
    return () => mq.removeEventListener?.('change', on);
  });
  const list = $derived(mode === 'list' || (mode === 'auto' && narrow));
  const effDensity = $derived<Density>(list ? 'comfortable' : density);

  // Row height in px: tokens (44 / 32 / phone 72) until a real row is measured.
  const layoutKey = $derived(list ? 'list' : effDensity);
  const rowH = $derived(measured[layoutKey] ?? (list ? 72 : effDensity === 'compact' ? 32 : 44));
  const viewport = $derived(viewH || 600);

  // Expanded rows add their detail height below the row; offsets stay exact with a sorted prefix sum
  // over the (few) expanded rows, so virtualisation keeps working.
  const DETAIL_GUESS = 160;
  let detailH = $state<Record<string, number>>({});
  const expandedSet = $derived(new Set(expand ? expanded : []));
  const extras = $derived.by(() => {
    if (!expandedSet.size) return [] as { index: number; h: number }[];
    const out: { index: number; h: number }[] = [];
    rows.forEach((r, index) => { const k = rowKey(r); if (expandedSet.has(k)) out.push({ index, h: detailH[k] ?? DETAIL_GUESS }); });
    return out;
  });
  const extraBefore = (i: number) => { let n = 0; for (const e of extras) { if (e.index >= i) break; n += e.h; } return n; };
  const extraTotal = $derived(extras.reduce((n, e) => n + e.h, 0));
  const offsetOf = (i: number) => i * rowH + extraBefore(i);
  function indexAt(y: number) {
    let lo = 0, hi = rows.length;
    while (lo < hi) { const mid = (lo + hi + 1) >> 1; if (offsetOf(mid) <= y) lo = mid; else hi = mid - 1; }
    return lo;
  }
  const first = $derived(Math.max(0, indexAt(Math.max(0, scrollTop - (list ? 0 : headH))) - OVERSCAN));
  const last = $derived(Math.min(rows.length, indexAt(scrollTop + viewport) + 1 + OVERSCAN));
  const padTop = $derived(offsetOf(first));
  const padBottom = $derived(Math.max(0, (rows.length - last) * rowH + extraTotal - extraBefore(last)));
  function setExpanded(keys: string[]) {
    expanded = keys;
    onexpandchange?.(keys);
  }
  const toggleExpand = (key: string) => setExpanded(expandedSet.has(key) ? expanded.filter((k) => k !== key) : [...expanded, key]);
  function measureDetail(node: HTMLElement, key: string) {
    const write = () => { const h = node.offsetHeight; if (h && Math.abs((detailH[key] ?? 0) - h) > 0.5) detailH = { ...detailH, [key]: h }; };
    write();
    const ro = typeof ResizeObserver === 'function' ? new ResizeObserver(write) : undefined;
    ro?.observe(node);
    return { destroy: () => ro?.disconnect() };
  }
  const visible = $derived(rows.slice(first, last).map((row, i) => ({ row, index: first + i, key: rowKey(row) })));
  const perPage = $derived(Math.max(1, Math.floor((viewport - (list ? 0 : headH)) / rowH) - 1));

  $effect(() => {
    void layoutKey; void visible.length;
    const el = scroller?.querySelector<HTMLElement>('[data-index]');
    const h = el?.offsetHeight;
    if (h && Math.abs(h - rowH) > 0.5) measured = { ...measured, [layoutKey]: h };
  });

  // Cursor paging: ask for the next page once per page as the end comes into view.
  let askedAt = -1;
  $effect(() => {
    if (!hasMore || loadingMore || loading || error || !onloadmore) return;
    if (last >= rows.length - OVERSCAN && askedAt !== rows.length) {
      askedAt = rows.length;
      onloadmore();
    }
  });

  const selectedSet = $derived(new Set(selected));
  const allSelected = $derived(rows.length > 0 && rows.every((r) => selectedSet.has(rowKey(r))));
  const someSelected = $derived(selected.length > 0 && !allSelected);
  function setSelection(keys: string[]) {
    selected = keys;
    onselectionchange?.(keys);
  }
  function toggleRow(index: number, range = false) {
    const key = rowKey(rows[index]);
    if (range && anchor !== undefined) {
      const [a, b] = anchor < index ? [anchor, index] : [index, anchor];
      const keys = rows.slice(a, b + 1).map(rowKey);
      setSelection([...new Set([...selected, ...keys])]);
    } else {
      setSelection(selectedSet.has(key) ? selected.filter((k) => k !== key) : [...selected, key]);
      anchor = index;
    }
  }
  const selectAllLoaded = () => setSelection(rows.map(rowKey));

  function sortBy(col: Column<T>) {
    const next: SortState = { column: col.id, direction: sort?.column === col.id && sort.direction === 'asc' ? 'desc' : 'asc' };
    sort = next;
    onsort?.(next);
  }
  const ariaSort = (col: Column<T>) => (sort?.column === col.id ? (sort.direction === 'asc' ? 'ascending' : 'descending') : col.sortable ? 'none' : undefined);

  async function focusRow(i: number) {
    if (!rows.length) return;
    focusIndex = Math.max(0, Math.min(rows.length - 1, i));
    const top = offsetOf(focusIndex);
    const room = viewport - (list ? 0 : headH);
    if (scroller) {
      if (top < scrollTop) scroller.scrollTop = top;
      else if (top + rowH > scrollTop + room) scroller.scrollTop = top + rowH - room;
      scrollTop = scroller.scrollTop;
    }
    await tick();
    scroller?.querySelector<HTMLElement>(`[data-index="${focusIndex}"]`)?.focus();
  }

  function onkeydown(e: KeyboardEvent) {
    const rowEl = (e.target as HTMLElement).closest<HTMLElement>('[data-index]');
    const onRow = rowEl !== null && rowEl === e.target;
    const mod = e.ctrlKey || e.metaKey;
    if (mod && e.key.toLowerCase() === 'a' && selectable && (onRow || e.target === scroller)) {
      e.preventDefault();
      selectAllLoaded();
      return;
    }
    if (!onRow) return;
    const i = Number(rowEl!.dataset.index);
    const moves: Record<string, number> = {
      ArrowDown: i + 1, j: i + 1, ArrowUp: i - 1, k: i - 1, Home: 0, End: rows.length - 1,
      PageDown: i + perPage, PageUp: i - perPage,
    };
    if (e.key in moves && !mod && !e.altKey) {
      e.preventDefault();
      focusRow(moves[e.key]);
    } else if (expand && (e.key === 'ArrowRight' || e.key === 'ArrowLeft') && !mod) {
      e.preventDefault();
      const key = rowKey(rows[i]);
      if ((e.key === 'ArrowRight') !== expandedSet.has(key)) toggleExpand(key);
    } else if (e.key === 'Enter' && onopen) {
      e.preventDefault();
      onopen(rows[i]);
    } else if (selectable && (e.key === 'x' || e.key === 'X' || e.key === ' ')) {
      e.preventDefault();
      toggleRow(i, e.shiftKey);
    } else if (e.key === '.' && rowActions) {
      e.preventDefault();
      rowEl!.querySelector<HTMLElement>('[data-actions] button')?.click();
    } else if (e.key === 'Escape' && selected.length) {
      setSelection([]);
    }
  }
  function onrowclick(e: MouseEvent, index: number) {
    focusIndex = index;
    if ((e.target as HTMLElement).closest('button, a, input, select, label, [popover]')) return;
    if (e.shiftKey && selectable) { toggleRow(index, true); return; }
    onopen?.(rows[index]);
  }

  const text = (row: T, col: Column<T>) => {
    const v = col.value?.(row);
    return v === undefined || v === null || v === '' ? '—' : typeof v === 'number' ? formatCount(v) : v;
  };
  const titleCol = $derived(columns.find((c) => c.phone === 'title') ?? columns[0]);
  const metaCols = $derived(columns.filter((c) => c !== titleCol && (c.phone ?? 'meta') === 'meta'));
  const trailCols = $derived(columns.filter((c) => c.phone === 'trailing'));
  const word = (n: number) => (n === 1 ? noun : (nounPlural ?? `${noun}s`));
  const countText = $derived(
    total !== undefined && total !== rows.length ? `${formatCount(rows.length)} of ${formatCount(total)} ${word(total)} loaded` : `${formatCount(rows.length)} ${word(rows.length)}`,
  );
  const colCount = $derived(columns.length + (selectable ? 1 : 0) + (rowActions ? 1 : 0) + (expand ? 1 : 0));
</script>

{#snippet content(row: T, col: Column<T>)}
  {#if cell}{@render cell(row, col)}{:else}{text(row, col)}{/if}
{/snippet}

{#snippet disclosure(row: T, key: string)}
  <button type="button" class="disc" aria-expanded={expandedSet.has(key)} aria-controls="{id}-d-{key}" tabindex={-1}
    aria-label="{expandedSet.has(key) ? 'Hide' : 'Show'} details for {text(row, titleCol)}" onclick={() => toggleExpand(key)}>
    <Icon name={expandedSet.has(key) ? 'chevron-down' : 'chevron-right'} size="sm" />
  </button>
{/snippet}

{#snippet actionsFor(row: T)}
  {#if rowActions}
    <span data-actions>
      <ActionMenu size="sm" label="Actions for {text(row, titleCol)}" items={rowActions(row)} onselect={(a) => onaction?.(a, row)} />
    </span>
  {/if}
{/snippet}

<div class="dt" class:compact={effDensity === 'compact'}>
  {#if selectable && selected.length}
    <BulkActionBar selected={selected.length} loaded={rows.length} {total} {noun} {nounPlural} onselectall={selectAllLoaded} onclear={() => setSelection([])}>
      {#snippet actions()}{#if bulkActions}{@render bulkActions(selected)}{/if}{/snippet}
    </BulkActionBar>
  {:else if toolbar || densitySwitch || rows.length}
    <div class="toolbar">
      {#if toolbar}{@render toolbar()}{/if}
      {#if rows.length && !loading}<span class="count num">{countText}</span>{/if}
      {#if densitySwitch && !list}
        <span class="density">
          <SegmentedControl label="Row density" size="sm" bind:value={density}
            options={[{ value: 'comfortable', label: 'Comfortable' }, { value: 'compact', label: 'Compact' }]} />
        </span>
      {/if}
    </div>
  {/if}

  {#if error && !rows.length}
    <div class="state"><ErrorState title="Couldn't load the {nounPlural ?? `${noun}s`}" code={errorCode} {onretry}>{error}</ErrorState></div>
  {:else if !loading && !rows.length}
    <div class="state">
      {#if empty}{@render empty()}{:else}<EmptyState title="No {nounPlural ?? `${noun}s`} to show" />{/if}
    </div>
  {:else}
    <!-- Keys move between rows (delegated from the focused row); the viewport scrolls. -->
    <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
    <div
      class="viewport"
      class:scrolled={scrollTop > 0}
      style:height
      bind:this={scroller}
      bind:clientHeight={viewH}
      onscroll={() => (scrollTop = scroller?.scrollTop ?? 0)}
      {onkeydown}
      role="region"
      aria-label="{label}, scrollable"
      aria-busy={loading || loadingMore}
    >
      {#if list}
        <ul class="list" aria-label={label} aria-describedby="{id}-keys" style:padding-top="{padTop}px" style:padding-bottom="{padBottom}px">
          {#if loading && !rows.length}
            {#each Array(8) as _, i (i)}<li class="item skel"><Skeleton lines={2} /></li>{/each}
          {/if}
          {#each visible as { row, index, key } (key)}
            <!-- Rows take focus for keyboard navigation; their keys are handled on the viewport (delegated). -->
            <!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_noninteractive_tabindex -->
            <li
              class="item"
              class:sel={selectedSet.has(key)}
              class:current={current === key}
              data-index={index}
              tabindex={index === focusIndex ? 0 : -1}
              aria-setsize={total ?? rows.length}
              aria-posinset={index + 1}
              aria-current={current === key ? 'true' : undefined}
              onclick={(e) => onrowclick(e, index)}
            >
              {#if selectable}
                <span class="lsel"><Checkbox checked={selectedSet.has(key)} aria-label="Select {text(row, titleCol)}" tabindex={-1} onchange={() => toggleRow(index)} /></span>
              {/if}
              <span class="lmain">
                <span class="ltitle">{@render content(row, titleCol)}</span>
                {#if metaCols.length}
                  <span class="lmeta">{#each metaCols as c, ci (c.id)}{#if ci > 0}<span class="dot" aria-hidden="true">·</span>{/if}<span>{@render content(row, c)}</span>{/each}</span>
                {/if}
              </span>
              {#each trailCols as c (c.id)}<span class="ltrail">{@render content(row, c)}</span>{/each}
              {@render actionsFor(row)}
              {#if expand}{@render disclosure(row, key)}{/if}
            </li>
            {#if expand && expandedSet.has(key)}
              <li class="ldetail" id="{id}-d-{key}" use:measureDetail={key} aria-label="Details for {text(row, titleCol)}">{@render expand(row)}</li>
            {/if}
          {/each}
        </ul>
      {:else}
        <table aria-label={label} aria-rowcount={(total ?? rows.length) + 1} aria-describedby="{id}-keys">
          <colgroup>
            {#if expand}<col class="c-exp" />{/if}
            {#if selectable}<col class="c-sel" />{/if}
            {#each columns as c (c.id)}<col style:width={c.width} />{/each}
            {#if rowActions}<col class="c-act" />{/if}
          </colgroup>
          <thead bind:clientHeight={headH}>
            <tr aria-rowindex={1}>
              {#if expand}<th scope="col" class="exp"><span class="visually-hidden">Details</span></th>{/if}
              {#if selectable}
                <th scope="col" class="sel">
                  <Checkbox checked={allSelected} indeterminate={someSelected} aria-label="Select all {formatCount(rows.length)} loaded"
                    onchange={() => (allSelected ? setSelection([]) : selectAllLoaded())} />
                </th>
              {/if}
              {#each columns as c (c.id)}
                <th scope="col" class:end={c.align === 'end'} aria-sort={ariaSort(c)}>
                  {#if c.hideLabel}<span class="visually-hidden">{c.label}</span>
                  {:else if c.sortable}
                    <button type="button" class="sort" onclick={() => sortBy(c)}>
                      <span>{c.label}</span>
                      <Icon name={sort?.column === c.id ? (sort.direction === 'asc' ? 'arrow-up' : 'arrow-down') : 'sort'} size="sm" />
                    </button>
                  {:else}{c.label}{/if}
                </th>
              {/each}
              {#if rowActions}<th scope="col" class="act"><span class="visually-hidden">Actions</span></th>{/if}
            </tr>
          </thead>
          <tbody>
            {#if loading && !rows.length}
              {#each Array(8) as _, i (i)}
                <tr class="skel">{#each Array(colCount) as _c, j (j)}<td><Skeleton /></td>{/each}</tr>
              {/each}
            {/if}
            {#if first > 0}<tr class="spacer" aria-hidden="true"><td colspan={colCount} style:height="{padTop}px"></td></tr>{/if}
            {#each visible as { row, index, key } (key)}
              <tr
                data-index={index}
                tabindex={index === focusIndex ? 0 : -1}
                aria-rowindex={index + 2}
                aria-selected={selectable ? selectedSet.has(key) : undefined}
                aria-current={current === key ? 'true' : undefined}
                class:sel={selectedSet.has(key)}
                class:current={current === key}
                class:openable={!!onopen}
                onclick={(e) => onrowclick(e, index)}
              >
                {#if expand}<td class="exp">{@render disclosure(row, key)}</td>{/if}
                {#if selectable}
                  <td class="sel"><Checkbox checked={selectedSet.has(key)} aria-label="Select {text(row, titleCol)}" tabindex={-1} onchange={() => toggleRow(index)} /></td>
                {/if}
                {#each columns as c (c.id)}
                  <td class:end={c.align === 'end'} class:num={c.align === 'end'}>{@render content(row, c)}</td>
                {/each}
                {#if rowActions}<td class="act">{@render actionsFor(row)}</td>{/if}
              </tr>
              {#if expand && expandedSet.has(key)}
                <tr class="detail" id="{id}-d-{key}" use:measureDetail={key}><td colspan={colCount}><div class="dbody">{@render expand(row)}</div></td></tr>
              {/if}
            {/each}
            {#if padBottom > 0}<tr class="spacer" aria-hidden="true"><td colspan={colCount} style:height="{padBottom}px"></td></tr>{/if}
          </tbody>
        </table>
      {/if}
      {#if loadingMore}
        <p class="more" role="status"><Spinner size="sm" /><span>Loading more {nounPlural ?? `${noun}s`}…</span></p>
      {:else if error && rows.length}
        <div class="more error"><Icon name="alert-octagon" size="sm" /><span>Couldn't load more: {error}</span>{#if onretry}<Button size="sm" onclick={onretry}>Retry</Button>{/if}</div>
      {:else if hasMore && onloadmore}
        <div class="more"><Button size="sm" variant="ghost" onclick={onloadmore}>Load more</Button></div>
      {/if}
    </div>
    <p class="visually-hidden" id="{id}-keys">
      Arrow keys or J and K move between rows{expand ? ', Right and Left arrows show and hide details' : ''}{onopen ? ', Enter opens' : ''}{selectable ? ', X selects, Shift X selects a range' : ''}{rowActions ? ', period opens the row actions' : ''}.
    </p>
  {/if}
</div>

<style>
  .dt { display: grid; gap: var(--space-2); min-width: 0; }
  .toolbar { display: flex; align-items: center; gap: var(--space-3); flex-wrap: wrap; min-height: var(--control-h-md); }
  .count { font-size: var(--text-sm); color: var(--ink-muted); }
  .density { margin-left: auto; }
  .state { border: var(--border-w) solid var(--line); border-radius: var(--radius-lg); background: var(--surface-raised); padding: var(--space-8) var(--space-4); }
  .viewport {
    overflow: auto; overscroll-behavior: contain; position: relative; min-width: 0;
    border: var(--border-w) solid var(--line); border-radius: var(--radius-lg); background: var(--surface-raised);
    scrollbar-color: var(--line-strong) transparent;
  }
  table { width: 100%; border-collapse: separate; border-spacing: 0; table-layout: fixed; font-size: var(--text-md); }
  .compact table { font-size: var(--text-sm); }
  col.c-sel { width: 2.75rem; }
  col.c-act { width: 3rem; }
  col.c-exp { width: 2.5rem; }
  td.exp, th.exp { padding: 0 0 0 var(--space-1); }
  .disc {
    display: inline-grid; place-items: center; width: var(--control-h-sm); height: var(--control-h-sm); position: relative;
    color: var(--ink-muted); background: none; border: 0; border-radius: var(--radius-md); cursor: pointer;
    transition: background-color var(--dur-fast) var(--ease-out), color var(--dur-fast) var(--ease-out);
  }
  .disc:hover { background: var(--surface-hover); color: var(--ink); }
  .disc[aria-expanded='true'] { color: var(--ink); }
  tr.detail td { height: auto; white-space: normal; overflow: visible; padding: 0; background: var(--surface-sunken); }
  .dbody { padding: var(--space-3) var(--space-4) var(--space-4) calc(2.5rem + var(--space-1)); }
  .ldetail { padding: var(--space-3) var(--space-4) var(--space-4); background: var(--surface-sunken); border-bottom: var(--border-w) solid var(--line); }
  @media (width < 48rem), (pointer: coarse) {
    .disc::before { content: ''; position: absolute; inset: 50% auto auto 50%; width: 2.75rem; height: 2.75rem; transform: translate(-50%, -50%); }
  }
  th {
    position: sticky; top: 0; z-index: var(--z-sticky); background: var(--surface-sunken); color: var(--ink-muted);
    font-size: var(--text-sm); font-weight: var(--weight-medium); text-align: start; white-space: nowrap;
    height: var(--row-h-compact); padding: 0 var(--space-3); border-bottom: var(--border-w) solid var(--line);
    transition: box-shadow var(--dur-fast) var(--ease-out);
  }
  .scrolled th { box-shadow: var(--shadow-sm); }
  th.end, td.end { text-align: end; }
  .sort {
    display: inline-flex; align-items: center; gap: var(--space-1); font: inherit; color: inherit; background: none;
    border: 0; padding: 0; cursor: pointer; max-width: 100%;
  }
  th.end .sort { flex-direction: row-reverse; }
  .sort:hover, th[aria-sort='ascending'] .sort, th[aria-sort='descending'] .sort { color: var(--ink); }
  td {
    height: var(--row-h-comfortable); padding: 0 var(--space-3); border-bottom: var(--border-w) solid var(--line);
    overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--ink);
  }
  .compact td { height: var(--row-h-compact); }
  td.sel, th.sel { padding: 0 0 0 var(--space-3); }
  td.act, th.act { padding: 0 var(--space-1); text-align: end; }
  tr.spacer td { padding: 0; border: 0; }
  tbody tr[data-index] { outline-offset: -2px; transition: background-color var(--dur-fast) var(--ease-out); }
  tbody tr.openable { cursor: pointer; }
  tbody tr[data-index]:hover { background: var(--surface-hover); }
  tbody tr.sel { background: var(--surface-selected); }
  tbody tr.current { background: var(--accent-tint); }
  .list { list-style: none; margin: 0; padding: 0; }
  .item {
    display: flex; align-items: center; gap: var(--space-3); height: 4.5rem; padding: 0 var(--space-3) 0 var(--space-4);
    border-bottom: var(--border-w) solid var(--line); outline-offset: -2px; min-width: 0; box-sizing: border-box;
  }
  .item.sel { background: var(--surface-selected); }
  .item.current { background: var(--accent-tint); }
  .item.skel { display: block; padding-top: var(--space-4); }
  .lmain { display: grid; gap: var(--space-0-5); flex: 1; min-width: 0; }
  .ltitle { font-size: var(--text-base); color: var(--ink); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .lmeta { display: flex; gap: var(--space-1-5); align-items: center; font-size: var(--text-sm); color: var(--ink-muted); overflow: hidden; white-space: nowrap; min-width: 0; }
  .ltrail { flex: none; }
  .lsel { flex: none; }
  .more { display: flex; align-items: center; justify-content: center; gap: var(--space-2); padding: var(--space-3); font-size: var(--text-sm); color: var(--ink-muted); }
  .more.error { color: var(--tone-error-ink); }
</style>
