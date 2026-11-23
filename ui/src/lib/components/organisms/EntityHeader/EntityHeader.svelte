<!--
  EntityHeader: the top of an item page. Artwork, the item's title as the page
  heading, a meta line (kind, year, network or label), its states, a few key facts, the
  monitor toggle and the actions (one primary in the `actions` snippet, the rest in a menu).
  Missing art falls back to initials and the kind icon; long, CJK and RTL titles wrap.
-->
<script lang="ts" module>
  import type { StatusFamily } from '../../../status/vocabulary';
  export interface EntityStatus { family: StatusFamily; value: string; detail?: string }
  export interface EntityFact { label: string; value: string }
</script>

<script lang="ts">
  import type { Snippet } from 'svelte';
  import Artwork from '../../atoms/Artwork/Artwork.svelte';
  import MediaKindIcon from '../../atoms/MediaKindIcon/MediaKindIcon.svelte';
  import StatusBadge from '../../atoms/StatusBadge/StatusBadge.svelte';
  import Link from '../../atoms/Link/Link.svelte';
  import ActionMenu, { type ActionItem } from '../../molecules/ActionMenu/ActionMenu.svelte';
  import MonitorToggle, { type MonitorState } from '../../molecules/MonitorToggle/MonitorToggle.svelte';
  import type { MediaKind } from '../../../tokens';

  interface Props {
    title: string;
    kind: MediaKind;
    /** Cached poster or cover URL; null shows the fallback. */
    art?: string | null;
    /** Meta line after the kind: "2024 · Harbourline · 52m". */
    meta?: string;
    status?: EntityStatus[];
    facts?: EntityFact[];
    monitor?: MonitorState;
    monitorReason?: string;
    onmonitor?: (state: MonitorState) => void;
    /** Primary and secondary buttons. */
    actions?: Snippet;
    menu?: ActionItem[];
    onmenu?: (id: string) => void;
    /** Parent link above the title: Library › TV. */
    back?: { href: string; label: string };
  }
  let { title, kind, art, meta, status = [], facts = [], monitor, monitorReason, onmonitor, actions, menu, onmenu, back }: Props = $props();

  const kindName: Record<MediaKind, string> = { tv: 'TV series', movie: 'Movie', music: 'Album', book: 'Book' };
  const shape = $derived(kind === 'music' ? 'cover' : 'poster');
</script>

<header class="eh">
  <div class="art" style:--ratio={shape === 'cover' ? '1' : '2 / 3'}><Artwork src={art} {title} {kind} {shape} width={128} eager /></div>
  <div class="main">
    {#if back}<Link href={back.href}>{back.label}</Link>{/if}
    <h1 dir="auto">{title}</h1>
    <p class="meta">
      <MediaKindIcon {kind} />
      <span>{kindName[kind]}{meta ? ` · ${meta}` : ''}</span>
    </p>
    {#if status.length || monitor}
      <div class="states">
        {#if monitor}
          <MonitorToggle state={monitor} item={title} reason={monitorReason} appearance="labelled" size="sm" onchange={onmonitor} />
        {/if}
        {#each status as s, i (i)}<StatusBadge family={s.family} value={s.value} detail={s.detail} />{/each}
      </div>
    {/if}
    {#if facts.length}
      <dl class="facts">
        {#each facts as f (f.label)}<div><dt>{f.label}</dt><dd class="num">{f.value}</dd></div>{/each}
      </dl>
    {/if}
    {#if actions || menu?.length}
      <div class="actions">
        {@render actions?.()}
        {#if menu?.length}<ActionMenu items={menu} label="More actions for {title}" variant="secondary" onselect={onmenu} />{/if}
      </div>
    {/if}
  </div>
</header>

<style>
  .eh { display: grid; grid-template-columns: auto minmax(0, 1fr); gap: var(--space-5); align-items: start; color: var(--ink); }
  .main { display: grid; gap: var(--space-2); min-width: 0; justify-items: start; }
  h1 { font-size: var(--text-2xl); line-height: var(--leading-tight); overflow-wrap: anywhere; max-width: 40ch; }
  .meta { display: flex; align-items: center; gap: var(--space-1-5); font-size: var(--text-md); color: var(--ink-muted); }
  .states { display: flex; flex-wrap: wrap; align-items: center; gap: var(--space-1-5); margin-top: var(--space-1); }
  .facts { display: flex; flex-wrap: wrap; gap: var(--space-2) var(--space-6); margin: var(--space-2) 0 0; max-width: 100%; }
  .facts div { display: grid; gap: var(--space-0-5); min-width: 0; }
  dt { font-size: var(--text-xs); color: var(--ink-muted); }
  /* Long values (library paths) wrap instead of widening the page on phones. */
  dd { margin: 0; font-size: var(--text-md); font-weight: var(--weight-medium); overflow-wrap: anywhere; }
  .actions { display: flex; flex-wrap: wrap; gap: var(--space-2); margin-top: var(--space-2); max-width: 100%; }

  @media (width < 48rem) {
    .eh { gap: var(--space-3); }
    /* Artwork sizes itself inline for layout stability; the phone header shows it smaller. */
    .art { width: 4.5rem; }
    .art > :global(.art) { width: 100% !important; height: auto !important; aspect-ratio: var(--ratio, 2 / 3); }
    .art :global(img) { width: 100%; height: 100%; }
    h1 { font-size: var(--text-xl); }
    .actions { grid-column: 1 / -1; }
  }
</style>
