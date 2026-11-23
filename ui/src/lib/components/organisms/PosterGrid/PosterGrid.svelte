<!--
  PosterGrid: the library as artwork. Virtualised by rows (only rows near the viewport are
  in the DOM, so only their art is requested), each tile with its title, year and
  status. Arrow keys move between tiles (one tab stop), Enter opens. Cursor paging like DataTable.
-->
<script lang="ts" module>
  import type { MediaKind } from '../../../tokens';
  import type { StatusFamily } from '../../../status/vocabulary';

  export interface PosterItem {
    id: string;
    title: string;
    kind: MediaKind;
    src?: string | null;
    year?: number | string;
    /** Second line after the year: "10 of 13 episodes". */
    detail?: string;
    status?: { family: StatusFamily; value: string };
  }
</script>

<script lang="ts">
  import { tick, type Snippet } from 'svelte';
  import Artwork from '../../atoms/Artwork/Artwork.svelte';
  import Skeleton from '../../atoms/Skeleton/Skeleton.svelte';
  import Spinner from '../../atoms/Spinner/Spinner.svelte';
  import StatusBadge from '../../atoms/StatusBadge/StatusBadge.svelte';
  import EmptyState from '../../molecules/EmptyState/EmptyState.svelte';

  interface Props {
    items: PosterItem[];
    /** Accessible name: "TV library". */
    label: string;
    /** Smallest tile width in px; tiles grow to fill the row. */
    minTile?: number;
    height?: string;
    loading?: boolean;
    hasMore?: boolean;
    loadingMore?: boolean;
    onloadmore?: () => void;
    onopen?: (item: PosterItem) => void;
    current?: string;
    empty?: Snippet;
  }
  let {
    items, label, minTile = 132, height = '36rem', loading = false, hasMore = false, loadingMore = false,
    onloadmore, onopen, current, empty,
  }: Props = $props();

  const GAP = 16, PAD = 16, CAPTION = 60, OVERSCAN = 2;
  let scroller = $state<HTMLElement>();
  let width = $state(0);
  let viewH = $state(0);
  let scrollTop = $state(0);
  let focusIndex = $state(0);

  const w = $derived(width || 960);
  const cols = $derived(Math.max(2, Math.floor((w - PAD * 2 + GAP) / (minTile + GAP))));
  const tileW = $derived(Math.floor((w - PAD * 2 - GAP * (cols - 1)) / cols));
  const rowH = $derived(Math.round(tileW * 1.5) + CAPTION + GAP);
  const rowCount = $derived(Math.ceil(items.length / cols));
  const firstRow = $derived(Math.max(0, Math.floor((scrollTop - PAD) / rowH) - OVERSCAN));
  const lastRow = $derived(Math.min(rowCount, Math.ceil((scrollTop + (viewH || 600)) / rowH) + OVERSCAN));
  const visible = $derived(items.slice(firstRow * cols, lastRow * cols).map((item, i) => ({ item, index: firstRow * cols + i })));

  let askedAt = -1;
  $effect(() => {
    if (!hasMore || loadingMore || loading || !onloadmore) return;
    if (lastRow >= rowCount - OVERSCAN && askedAt !== items.length) {
      askedAt = items.length;
      onloadmore();
    }
  });

  async function focusTile(i: number) {
    if (!items.length) return;
    focusIndex = Math.max(0, Math.min(items.length - 1, i));
    const top = PAD + Math.floor(focusIndex / cols) * rowH;
    if (scroller) {
      const room = viewH || 600;
      if (top < scrollTop) scroller.scrollTop = top - PAD;
      else if (top + rowH > scrollTop + room) scroller.scrollTop = top + rowH - room;
      scrollTop = scroller.scrollTop;
    }
    await tick();
    scroller?.querySelector<HTMLElement>(`[data-index="${focusIndex}"]`)?.focus();
  }
  function onkeydown(e: KeyboardEvent) {
    const el = (e.target as HTMLElement).closest<HTMLElement>('[data-index]');
    if (!el) return;
    const i = Number(el.dataset.index);
    const perPage = Math.max(1, Math.floor((viewH || 600) / rowH)) * cols;
    const to: Record<string, number> = {
      ArrowRight: i + 1, ArrowLeft: i - 1, ArrowDown: i + cols, ArrowUp: i - cols,
      Home: e.ctrlKey ? 0 : i - (i % cols), End: e.ctrlKey ? items.length - 1 : i - (i % cols) + cols - 1,
      PageDown: i + perPage, PageUp: i - perPage,
    };
    if (!(e.key in to)) return;
    e.preventDefault();
    focusTile(to[e.key]);
  }
</script>

{#if !loading && !items.length}
  <div class="state">{#if empty}{@render empty()}{:else}<EmptyState title="Nothing to show" icon="library" />{/if}</div>
{:else}
  <!-- Arrow keys from the tiles are handled here (delegated roving focus). -->
  <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
  <div
    class="viewport"
    style:height
    bind:this={scroller}
    bind:clientWidth={width}
    bind:clientHeight={viewH}
    onscroll={() => (scrollTop = scroller?.scrollTop ?? 0)}
    {onkeydown}
    role="region"
    aria-label="{label}, scrollable"
    aria-busy={loading || loadingMore}
  >
    <ul
      class="grid"
      aria-label={label}
      style:--cols={cols}
      style:padding-top="{PAD + firstRow * rowH}px"
      style:padding-bottom="{PAD + Math.max(0, rowCount - lastRow) * rowH}px"
    >
      {#if loading && !items.length}
        {#each Array(cols * 2) as _, i (i)}
          <li class="tile-skel" aria-hidden="true"><Skeleton shape="block" height="{Math.round(tileW * 1.5)}px" /><Skeleton lines={2} /></li>
        {/each}
      {/if}
      {#each visible as { item, index } (item.id)}
        <li style:height="{rowH - GAP}px" aria-setsize={items.length} aria-posinset={index + 1}>
          <button
            type="button"
            class="tile"
            class:current={current === item.id}
            data-index={index}
            tabindex={index === focusIndex ? 0 : -1}
            aria-current={current === item.id ? 'true' : undefined}
            onclick={() => { focusIndex = index; onopen?.(item); }}
          >
            <Artwork src={item.src} title={item.title} kind={item.kind} width={tileW} />
            <span class="cap">
              <span class="title" title={item.title}>{item.title}</span>
              <span class="meta">
                {#if item.status}<StatusBadge family={item.status.family} value={item.status.value} size="sm" appearance="plain" />{/if}
                {#if item.year || item.detail}<span class="sub">{[item.year, item.detail].filter(Boolean).join(' · ')}</span>{/if}
              </span>
            </span>
          </button>
        </li>
      {/each}
    </ul>
    {#if loadingMore}<p class="more" role="status"><Spinner size="sm" /><span>Loading more…</span></p>{/if}
  </div>
{/if}

<style>
  .state { border: var(--border-w) solid var(--line); border-radius: var(--radius-lg); background: var(--surface-raised); padding: var(--space-8) var(--space-4); }
  .viewport {
    overflow-y: auto; overflow-x: hidden; overscroll-behavior: contain; min-width: 0;
    border: var(--border-w) solid var(--line); border-radius: var(--radius-lg); background: var(--surface-ground);
    scrollbar-color: var(--line-strong) transparent;
  }
  .grid {
    list-style: none; margin: 0; padding-left: var(--space-4); padding-right: var(--space-4);
    display: grid; grid-template-columns: repeat(var(--cols), minmax(0, 1fr)); gap: var(--space-4); align-items: start;
  }
  li { min-width: 0; }
  .tile {
    display: grid; gap: var(--space-2); width: 100%; padding: 0; border: 0; background: none; color: inherit;
    font: inherit; text-align: start; cursor: pointer; border-radius: var(--radius-md);
  }
  .tile :global(img), .tile :global(.art) { transition: filter var(--dur-fast) var(--ease-out); }
  .tile:hover :global(img) { filter: brightness(1.06); }
  .tile.current { outline: var(--focus-w) solid var(--accent); outline-offset: var(--focus-offset); }
  .cap { display: grid; gap: var(--space-0-5); min-width: 0; }
  .title { font-size: var(--text-md); font-weight: var(--weight-medium); color: var(--ink); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .meta { display: flex; align-items: center; gap: var(--space-1-5); min-width: 0; overflow: hidden; white-space: nowrap; }
  .sub { font-size: var(--text-xs); color: var(--ink-muted); overflow: hidden; text-overflow: ellipsis; }
  .tile-skel { display: grid; gap: var(--space-2); }
  .more { display: flex; align-items: center; justify-content: center; gap: var(--space-2); padding: var(--space-3); font-size: var(--text-sm); color: var(--ink-muted); }
</style>
