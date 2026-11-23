<!--
  DashboardPage (template): S03 home, "see in seconds whether anything needs attention". The
  attention list comes first (import blocked, ambiguous for a week, action-needed health); when
  it is empty a single calm line says so. Then stat tiles, active downloads as queue cards
  with the total speed, the health summary and recent history. Each region has its
  own loading and error state, so one failed source never blanks the page.
-->
<script lang="ts" module>
  import type { StatusFamily } from '../../../status/vocabulary';
  import type { MediaKind } from '../../../tokens';

  export interface AttentionItem {
    id: string;
    title: string;
    detail?: string;
    status: { family: StatusFamily; value: string; detail?: string };
    kind?: MediaKind;
    href: string;
  }
  export interface HealthProblem {
    id: string;
    name: string;
    status: string;
    message?: string;
    href?: string;
  }
  export type DashboardRegion = 'attention' | 'tiles' | 'downloads' | 'health' | 'history';
</script>

<script lang="ts">
  import type { ComponentProps } from 'svelte';
  import Icon from '../../atoms/Icon/Icon.svelte';
  import Link from '../../atoms/Link/Link.svelte';
  import MediaKindIcon from '../../atoms/MediaKindIcon/MediaKindIcon.svelte';
  import Skeleton from '../../atoms/Skeleton/Skeleton.svelte';
  import StatusBadge from '../../atoms/StatusBadge/StatusBadge.svelte';
  import EmptyState from '../../molecules/EmptyState/EmptyState.svelte';
  import ErrorState from '../../molecules/ErrorState/ErrorState.svelte';
  import QueueCard, { type QueueAction, type QueueCardData } from '../../molecules/QueueCard/QueueCard.svelte';
  import StatTile from '../../molecules/StatTile/StatTile.svelte';
  import Timeline, { type TimelineEntry } from '../../organisms/Timeline/Timeline.svelte';
  import { formatCount, formatSpeed } from '../../../format/quantity';

  interface Props {
    title?: string;
    attention?: AttentionItem[];
    /** "since 09:12": shown in the calm line when nothing needs attention. */
    healthySince?: string;
    tiles?: ComponentProps<typeof StatTile>[];
    downloads?: QueueCardData[];
    /** Downloads beyond the cards shown. */
    downloadsTotal?: number;
    /** Bytes per second across all downloads. */
    totalSpeed?: number;
    queueHref?: string;
    onqueueaction?: (action: QueueAction, id: string) => void;
    health?: { ok: number; total: number; problems: HealthProblem[] };
    healthHref?: string;
    history?: TimelineEntry[];
    historyHref?: string;
    loading?: DashboardRegion[] | boolean;
    errors?: Partial<Record<DashboardRegion, string>>;
    onretry?: (region: DashboardRegion) => void;
    now?: number;
  }
  let {
    title = 'Dashboard', attention = [], healthySince, tiles = [], downloads = [], downloadsTotal, totalSpeed, queueHref = '#/activity/queue',
    onqueueaction, health, healthHref = '#/system/health', history = [], historyHref = '#/activity/history', loading = false, errors = {},
    onretry, now,
  }: Props = $props();

  const busy = (r: DashboardRegion) => loading === true || (Array.isArray(loading) && loading.includes(r));
  const retry = (r: DashboardRegion) => (onretry ? () => onretry(r) : undefined);
</script>

{#snippet failed(r: DashboardRegion, what: string)}
  <ErrorState size="sm" title="Couldn't load {what}" onretry={retry(r)} level={3}>{errors[r]}</ErrorState>
{/snippet}

<div class="frame">
  <div class="db">
    <h1>{title}</h1>

    <section class="attention" aria-labelledby="db-att">
      <h2 id="db-att" class:visually-hidden={!busy('attention') && !errors.attention && !attention.length}>Needs attention</h2>
      {#if busy('attention')}
        <Skeleton lines={2} />
      {:else if errors.attention}
        {@render failed('attention', 'the attention list')}
      {:else if attention.length}
        <ul class="att-list">
          {#each attention as a (a.id)}
            <li>
              <a href={a.href} class="att">
                <StatusBadge family={a.status.family} value={a.status.value} detail={a.status.detail} size="sm" />
                <span class="att-text">
                  <span class="att-title">{#if a.kind}<MediaKindIcon kind={a.kind} />{/if}<span>{a.title}</span></span>
                  {#if a.detail}<span class="att-detail">{a.detail}</span>{/if}
                </span>
                <Icon name="chevron-right" size="sm" />
              </a>
            </li>
          {/each}
        </ul>
      {:else}
        <p class="calm"><Icon name="check-circle" /><span>Nothing needs you right now{healthySince ? `; all OK ${healthySince}` : ''}.</span></p>
      {/if}
    </section>

    {#if tiles.length || busy('tiles')}
      <section class="tiles" aria-label="Summary">
        {#if errors.tiles}{@render failed('tiles', 'the summary')}
        {:else}{#each tiles as t (t.label)}<StatTile {...t} loading={busy('tiles') || t.loading} />{/each}{/if}
      </section>
    {/if}

    <div class="cols">
      <section class="panel downloads" aria-labelledby="db-dl">
        <div class="ph">
          <h2 id="db-dl">Downloading</h2>
          {#if totalSpeed}<span class="speed num"><Icon name="arrow-down" size="sm" />{formatSpeed(totalSpeed)}</span>{/if}
          <Link href={queueHref} variant="quiet">Open queue</Link>
        </div>
        {#if busy('downloads')}
          <div class="cards">{#each Array(2) as _, i (i)}<Skeleton shape="block" height="7rem" />{/each}</div>
        {:else if errors.downloads}
          {@render failed('downloads', 'the queue')}
        {:else if downloads.length}
          <div class="cards">{#each downloads as d (d.id)}<QueueCard download={d} onaction={onqueueaction} {now} />{/each}</div>
          {#if downloadsTotal && downloadsTotal > downloads.length}
            <p class="more"><Link href={queueHref}>{formatCount(downloadsTotal - downloads.length)} more in the queue</Link></p>
          {/if}
        {:else}
          <EmptyState size="sm" title="Nothing downloading" icon="arrow-down" level={3}>New grabs show up here as soon as they are sent.</EmptyState>
        {/if}
      </section>

      <div class="side">
        <section class="panel" aria-labelledby="db-health">
          <div class="ph"><h2 id="db-health">Health</h2><Link href={healthHref} variant="quiet">All integrations</Link></div>
          {#if busy('health')}
            <Skeleton lines={3} />
          {:else if errors.health}
            {@render failed('health', 'health')}
          {:else if health}
            {#if health.problems.length}
              <ul class="health">
                {#each health.problems as p (p.id)}
                  <li>
                    <StatusBadge family="health" value={p.status} size="sm" />
                    <span class="hp">{#if p.href}<a href={p.href}>{p.name}</a>{:else}{p.name}{/if}{#if p.message}<span class="hm">{p.message}</span>{/if}</span>
                  </li>
                {/each}
              </ul>
            {/if}
            <p class="hsum num">{formatCount(health.ok)} of {formatCount(health.total)} integrations OK</p>
          {/if}
        </section>

        <section class="panel" aria-labelledby="db-hist">
          <div class="ph"><h2 id="db-hist">Recent</h2><Link href={historyHref} variant="quiet">History</Link></div>
          {#if errors.history}
            {@render failed('history', 'history')}
          {:else}
            <Timeline entries={history} label="Recent history" limit={5} groupByDay={false} loading={busy('history')} emptyTitle="Nothing has happened yet" now={now !== undefined ? new Date(now) : undefined} />
          {/if}
        </section>
      </div>
    </div>
  </div>
</div>

<style>
  .frame { container-type: inline-size; min-width: 0; }
  .db { display: grid; gap: var(--space-6); min-width: 0; }
  h1 { font-size: var(--text-xl); line-height: var(--leading-tight); }
  h2 { font-size: var(--text-lg); }
  .attention { display: grid; gap: var(--space-3); }
  .att-list { list-style: none; margin: 0; padding: 0; border: var(--border-w) solid var(--tone-error-line); border-radius: var(--radius-lg); background: var(--surface-raised); overflow: hidden; }
  .att-list li + li { border-top: var(--border-w) solid var(--line); }
  .att {
    display: flex; align-items: center; gap: var(--space-3); min-height: var(--row-h-comfortable); padding: var(--space-2) var(--space-4);
    color: var(--ink); text-decoration: none; transition: background-color var(--dur-fast) var(--ease-out);
  }
  .att:hover { background: var(--surface-hover); }
  .att > :global(svg) { color: var(--ink-subtle); flex: none; }
  .att-text { display: grid; gap: var(--space-0-5); flex: 1; min-width: 0; }
  .att-title { display: flex; align-items: center; gap: var(--space-1-5); font-weight: var(--weight-medium); min-width: 0; }
  .att-title span { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .att-detail { font-size: var(--text-sm); color: var(--ink-muted); }
  .calm { display: flex; align-items: center; gap: var(--space-2); padding: var(--space-3) var(--space-4); color: var(--ink-muted);
    background: var(--surface-raised); border: var(--border-w) solid var(--line); border-radius: var(--radius-lg); }
  .calm :global(svg) { color: var(--tone-ok-solid); flex: none; }
  .tiles { display: grid; gap: var(--space-3); grid-template-columns: repeat(auto-fit, minmax(min(100%, 11rem), 1fr)); }
  .cols { display: grid; gap: var(--space-6); min-width: 0; }
  .side { display: grid; gap: var(--space-6); align-content: start; min-width: 0; }
  .panel { display: grid; gap: var(--space-3); align-content: start; min-width: 0; }
  .ph { display: flex; align-items: baseline; gap: var(--space-3); flex-wrap: wrap; }
  .ph :global(a) { margin-left: auto; font-size: var(--text-md); }
  .speed { display: inline-flex; align-items: center; gap: var(--space-1); font-size: var(--text-md); color: var(--ink-muted); }
  .cards { display: grid; gap: var(--space-3); grid-template-columns: repeat(auto-fill, minmax(min(100%, 18rem), 1fr)); }
  .more { font-size: var(--text-md); }
  .health { list-style: none; margin: 0; padding: 0; display: grid; gap: var(--space-3); }
  .health li { display: flex; align-items: flex-start; gap: var(--space-2); }
  .hp { display: grid; gap: var(--space-0-5); min-width: 0; font-size: var(--text-md); }
  .hm { color: var(--ink-muted); font-size: var(--text-sm); }
  .hsum { font-size: var(--text-md); color: var(--ink-muted); }
  @container (width >= 56rem) {
    .cols { grid-template-columns: minmax(0, 1.6fr) minmax(18rem, 1fr); }
  }
</style>
