<!--
  DetailPage (template): one item or object (S06 item detail, S14 download, S17 rule run). Entity
  header, tabs for the adapter sections, side panels beside them on wide screens and
  under them on narrow ones, the history timeline, and an outlet for drawers and dialogs
  (interactive search, confirm dialogs). Loading shows a layout-stable skeleton; a failed load
  shows the error with retry. With `syncUrl` the tab lives in the hash query (`?tab=files`).
-->
<script lang="ts">
  import { untrack, type ComponentProps, type Snippet } from 'svelte';
  import Skeleton from '../../atoms/Skeleton/Skeleton.svelte';
  import ErrorState from '../../molecules/ErrorState/ErrorState.svelte';
  import Tabs, { type TabItem } from '../../molecules/Tabs/Tabs.svelte';
  import EntityHeader from '../../organisms/EntityHeader/EntityHeader.svelte';
  import Timeline, { type TimelineEntry } from '../../organisms/Timeline/Timeline.svelte';
  import { currentRoute, replaceParams } from '../../../route/hash';

  interface Props {
    /** EntityHeader props: title, kind, art, facts, status, monitor, menu, back, actions. */
    entity: ComponentProps<typeof EntityHeader>;
    banner?: Snippet;
    tabs?: TabItem[];
    tab?: string;
    syncUrl?: boolean;
    ontab?: (id: string) => void;
    /** Body of the current tab (or the whole main column without tabs). */
    children?: Snippet<[string]>;
    /** Side panels: files, active download, properties. */
    aside?: Snippet;
    history?: TimelineEntry[];
    historyLoading?: boolean;
    historyLimit?: number;
    now?: Date;
    /** Drawers and dialogs that belong to this page. */
    drawer?: Snippet;
    loading?: boolean;
    error?: string;
    errorCode?: string;
    onretry?: () => void;
  }
  let {
    entity, banner, tabs = [], tab = $bindable(tabs[0]?.id ?? ''), syncUrl = false, ontab, children: body, aside, history,
    historyLoading = false, historyLimit = 8, now, drawer, loading = false, error, errorCode, onretry,
  }: Props = $props();

  untrack(() => {
    if (!syncUrl) return;
    const t = currentRoute().params.get('tab');
    if (t && tabs.some((x) => x.id === t && !x.disabled)) tab = t;
  });
  function change(id: string) {
    if (syncUrl) {
      const p = currentRoute().params;
      if (id === tabs[0]?.id) p.delete('tab'); else p.set('tab', id);
      replaceParams(p);
    }
    ontab?.(id);
  }
</script>

<div class="dp">
  {#if loading}
    <div class="skel-head" aria-busy="true" aria-label="Loading {entity.title}">
      <Skeleton shape="block" width="6rem" height="9rem" />
      <div class="skel-lines"><Skeleton width="40%" height="1.75rem" /><Skeleton lines={3} /></div>
    </div>
  {:else if error}
    <h1 class="visually-hidden">{entity.title}</h1>
    <div class="state"><ErrorState title="Couldn't load {entity.title}" code={errorCode} {onretry} level={2}>{error}</ErrorState></div>
  {:else}
    <EntityHeader {...entity} />
    {#if banner}{@render banner()}{/if}

    <div class="cols" class:with-aside={!!aside}>
      <div class="main">
        {#if tabs.length}
          <Tabs {tabs} bind:value={tab} label="{entity.title} sections" onchange={change}>
            {#snippet children(id)}{#if body}{@render body(id)}{/if}{/snippet}
          </Tabs>
        {:else if body}
          {@render body('')}
        {/if}

        {#if history}
          <section class="history" aria-labelledby="dp-history">
            <h2 id="dp-history">History</h2>
            <Timeline entries={history} label="History of {entity.title}" loading={historyLoading} limit={historyLimit} {now} />
          </section>
        {/if}
      </div>
      {#if aside}<aside class="aside" aria-label="Details">{@render aside()}</aside>{/if}
    </div>
  {/if}
  {#if drawer}{@render drawer()}{/if}
</div>

<style>
  .dp { display: grid; gap: var(--space-5); min-width: 0; container-type: inline-size; }
  .cols { display: grid; gap: var(--space-6); min-width: 0; }
  .main, .aside { display: grid; gap: var(--space-6); align-content: start; min-width: 0; }
  @container (width >= 52rem) {
    .cols.with-aside { grid-template-columns: minmax(0, 1fr) minmax(16rem, 20rem); }
  }
  .history { display: grid; gap: var(--space-3); }
  h2 { font-size: var(--text-lg); }
  .skel-head { display: flex; gap: var(--space-4); align-items: flex-start; }
  .skel-lines { display: grid; gap: var(--space-3); flex: 1; }
  .state { border: var(--border-w) solid var(--line); border-radius: var(--radius-lg); background: var(--surface-raised); padding: var(--space-8) var(--space-4); }
</style>
