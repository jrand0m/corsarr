<!--
  DecisionPanel: one release as the decision engine and automation judged it, decision first.
  The leading line says what automatic search would do (the single pick, accepted but outranked
  and why, or rejected with the main reason in one line, "+N more"), how close the release is to
  the profile, and a score chip only when custom formats count. Then the title, the facts, the
  swarm and whether the whole payload is available. A rejection's full reason list, grouped by
  category, sits under a closed "Why it was rejected?" disclosure; what each category means is the
  category badge's tooltip. The override rule shows only through the button: preference and
  temporary reasons "Grab anyway"; permanent and blocklist reasons "Grab anyway…" with a
  confirmation; safety reasons are never overridden. `ongrab` receives the overridden reasons.
-->
<script lang="ts" module>
  import type { ReasonCategory } from '../EvidencePanel/EvidencePanel.svelte';
  import type { Desirability } from '../../molecules/DesirabilityGauge/DesirabilityGauge.svelte';
  import type { Health } from '../../molecules/SwarmHealth/SwarmHealth.svelte';
  import type { AvailabilityStatus } from '../../molecules/AvailabilityLine/AvailabilityLine.svelte';

  export interface DecisionReason {
    rule: string;
    category: ReasonCategory;
    /** The failing comparison in plain words: "58.2 GB is over the 40 GB cap". */
    message: string;
    /** A short line for the card: "Too large: 58.2 GB, max 40 GB". */
    summary?: string;
    /** The release is blocklisted (overridable only with confirmation). */
    blocklist?: boolean;
  }
  /** What automatic search would do with the release; pending until every indexer answered. */
  export type AutomationOutcome = 'grab' | 'shadow' | 'outranked' | 'declined' | 'pending' | 'unavailable';
  export interface Decision {
    release: {
      title: string;
      indexer: string;
      /** source: an album straight from a music source (no seeders). */
      protocol: 'torrent' | 'usenet' | 'source';
      size: number;
      publishedAt: Date | string | number;
      seeders?: number;
      quality: string;
      /** Item runtime for GB per hour. */
      runtimeSeconds?: number;
      /** Torrent swarm; without it only the seeder count shows. */
      swarm?: { seeders: number; leechers?: number; reported: boolean; health: Health };
      availability?: { status: AvailabilityStatus; grabs?: number; files?: number; minimumRatio?: number };
    };
    verdict: 'accept' | 'reject';
    rank?: number;
    candidates?: number;
    score: { total: number; formats: { name: string; score: number }[] };
    reasons: DecisionReason[];
    /** Automation's outcome; omitted where no search context exists. */
    automation?: { outcome: AutomationOutcome; lostTo?: string };
    /** Closeness to the profile; null = quality outside the profile, undefined = not known. */
    desirability?: Desirability | null;
  }
  export type OverrideLevel = 'allowed' | 'confirm' | 'never';

  /** Override rule: never for safety, confirm for permanent and blocklist, else allowed. */
  export function overrideLevel(reasons: DecisionReason[]): OverrideLevel {
    if (reasons.some((r) => r.category === 'safety')) return 'never';
    if (reasons.some((r) => r.category === 'permanent' || r.blocklist)) return 'confirm';
    return 'allowed';
  }

  const order: ReasonCategory[] = ['safety', 'permanent', 'temporary', 'preference'];

  /** The most severe reason first: safety, permanent, temporary, preference. */
  export function mainReason(reasons: DecisionReason[]): DecisionReason | undefined {
    return reasons.slice().sort((a, b) => order.indexOf(a.category) - order.indexOf(b.category))[0];
  }
</script>

<script lang="ts">
  import Button from '../../atoms/Button/Button.svelte';
  import Chip from '../../atoms/Chip/Chip.svelte';
  import Icon from '../../atoms/Icon/Icon.svelte';
  import Quantity from '../../atoms/Quantity/Quantity.svelte';
  import StatusBadge from '../../atoms/StatusBadge/StatusBadge.svelte';
  import Timestamp from '../../atoms/Timestamp/Timestamp.svelte';
  import ConfirmDialog from '../ConfirmDialog/ConfirmDialog.svelte';
  import DesirabilityGauge, { desirabilityParts } from '../../molecules/DesirabilityGauge/DesirabilityGauge.svelte';
  import SwarmHealth from '../../molecules/SwarmHealth/SwarmHealth.svelte';
  import AvailabilityLine from '../../molecules/AvailabilityLine/AvailabilityLine.svelte';
  import { formatCount } from '../../../format/quantity';

  interface Props {
    decision: Decision;
    ongrab?: (overridden: DecisionReason[]) => void;
    /** Fixed "now" (ms) for stories and tests. */
    now?: number;
    /** Heading level of the release title inside the page outline. */
    level?: 2 | 3;
    /** Opens the rejection details (stories). */
    expanded?: boolean;
  }
  let { decision: d, ongrab, now, level = 3, expanded = false }: Props = $props();

  const id = $props.id();
  const grouped = $derived(order.map((c) => ({ c, items: d.reasons.filter((r) => r.category === c) })).filter((g) => g.items.length));
  const level$ = $derived(overrideLevel(d.reasons));
  const main = $derived(mainReason(d.reasons));
  const more = $derived(d.reasons.length - 1);
  const outcome = $derived(d.automation?.outcome ?? (d.verdict === 'accept' ? undefined : 'declined'));
  const pick = $derived(outcome === 'grab' || outcome === 'shadow');
  const showScore = $derived(d.score.total !== 0 || d.score.formats.length > 0);
  const total = $derived(d.score.total > 0 ? `+${formatCount(d.score.total)}` : d.score.total < 0 ? `−${formatCount(-d.score.total)}` : '0');
  let confirming = $state(false);
</script>

<section class={['dp', { pick }]} aria-labelledby="{id}-title" aria-describedby="{id}-lead">
  <div class="lead" id="{id}-lead">
    {#if outcome}<StatusBadge family="automation" value={outcome} size="sm" />
    {:else}<StatusBadge family="verdict" value={d.verdict} size="sm" />{/if}
    {#if d.desirability !== undefined}<DesirabilityGauge desirability={d.desirability ?? undefined} />{/if}
    {#if d.verdict === 'reject' && main}
      <span class="why"><span class="main">{main.summary || main.message}</span>{#if more > 0}<span class="more">{` +${more} more`}</span>{/if}</span>
    {:else if outcome === 'outranked' && d.automation?.lostTo}
      <span class="why">Not the pick: {d.automation.lostTo}</span>
    {/if}
    {#if showScore}
      <span class="fscore" title={d.score.formats.map((f) => `${f.name} ${f.score > 0 ? '+' : ''}${f.score}`).join(', ') || undefined}>Formats <strong class="num">{total}</strong></span>
    {/if}
  </div>

  <header>
    <svelte:element this={`h${level}`} id="{id}-title" class="title">{d.release.title}</svelte:element>
    <div class="facts">
      <span>{d.release.quality}</span>
      <Quantity kind="bytes" value={d.release.size} />
      {#if d.release.runtimeSeconds}<Quantity kind="gbph" value={d.release.size} runtime={d.release.runtimeSeconds} />{/if}
      {#if d.release.protocol === 'source'}<span>Direct source</span>
      {:else if d.release.protocol === 'usenet'}<span>Usenet</span>
      {:else if !d.release.swarm}<span>{d.release.seeders === undefined ? '—' : `${formatCount(d.release.seeders)} seeders`}</span>{/if}
      <span>{d.release.indexer}</span>
      <Timestamp value={d.release.publishedAt} now={now === undefined ? undefined : new Date(now)} />
      {#if d.rank !== undefined}<span class="rank num">rank {d.rank}{d.candidates ? ` of ${formatCount(d.candidates)}` : ''}</span>{/if}
    </div>
  </header>

  {#if d.release.swarm || d.release.availability}
    <div class="payload">
      {#if d.release.swarm}<SwarmHealth {...d.release.swarm} />{/if}
      {#if d.release.availability}<AvailabilityLine {...d.release.availability} protocol={d.release.protocol} />{/if}
    </div>
  {/if}

  {#if d.desirability}
    <p class="parts">{desirabilityParts(d.desirability).join(' · ')}</p>
  {/if}

  {#if d.score.formats.length}
    <ul class="formats" aria-label="Custom formats">
      {#each d.score.formats as f (f.name)}<li><Chip variant="score" label={f.name} score={f.score} /></li>{/each}
    </ul>
  {/if}

  {#if grouped.length}
    <details class="reasons" open={expanded}>
      <summary>Why it was rejected? <span class="count">({d.reasons.length})</span></summary>
      {#each grouped as g (g.c)}
        <div class={['group', g.c]}>
          <StatusBadge family="reason" value={g.c} size="sm" />
          <ul>
            {#each g.items as r, i (i)}
              <li><span class="msg">{r.message}</span><code class="rule">{r.rule}</code></li>
            {/each}
          </ul>
        </div>
      {/each}
    </details>
  {/if}

  {#if ongrab}
    <footer>
      {#if d.verdict === 'accept'}
        <Button variant={pick ? 'primary' : 'secondary'} icon="download" onclick={() => ongrab([])}>Grab</Button>
      {:else if level$ === 'never'}
        <p class="never"><Icon name="lock" size="sm" />A safety rule failed, so this release can’t be grabbed.</p>
      {:else}
        <span id="{id}-override" class="visually-hidden">
          Overrides the rejection{level$ === 'confirm' ? '; asks for confirmation first' : ''}.
        </span>
        {#if level$ === 'allowed'}
          <Button icon="download" aria-describedby="{id}-override" onclick={() => ongrab(d.reasons)}>Grab anyway</Button>
        {:else}
          <Button icon="download" aria-describedby="{id}-override" onclick={() => (confirming = true)}>Grab anyway…</Button>
        {/if}
      {/if}
    </footer>
  {/if}
</section>

{#if ongrab && level$ === 'confirm'}
  <ConfirmDialog
    bind:open={confirming} title="Grab {d.release.title} anyway?" variant="primary" confirmLabel="Grab anyway"
    consequences={['Manually overrides the rejection; automation would skip this release', ...d.reasons.map((r) => `Overrides: ${r.message}`), 'Records the override in History']}
    undoable="You can remove the download from the queue at any time."
    onconfirm={() => ongrab(d.reasons)}
  />
{/if}

<style>
  .dp {
    display: grid;
    gap: var(--space-2);
    min-width: 0;
    padding: var(--space-4);
    color: var(--ink);
    background: var(--surface-raised);
    border: var(--border-w) solid var(--line);
    border-radius: var(--radius-lg);
  }
  .dp.pick { border: 3px solid var(--tone-ok-solid); box-shadow: var(--shadow-md); }
  .lead { display: flex; flex-wrap: wrap; align-items: center; gap: var(--space-1-5) var(--space-3); }
  .why { font-size: var(--text-sm); font-weight: var(--weight-medium); color: var(--ink); }
  .more { color: var(--ink-muted); font-weight: var(--weight-regular); }
  .fscore { font-size: var(--text-sm); color: var(--ink-muted); }
  .fscore strong { color: var(--ink); }
  header { display: grid; gap: var(--space-1); min-width: 0; }
  .title { font-family: var(--font-mono); font-size: var(--text-md); font-weight: var(--weight-medium); overflow-wrap: anywhere; line-height: var(--leading-snug); }
  .facts { display: flex; flex-wrap: wrap; gap: var(--space-0-5) var(--space-3); font-size: var(--text-sm); color: var(--ink-muted); }
  .rank { color: var(--ink-subtle); }
  .payload { display: flex; flex-wrap: wrap; gap: var(--space-1) var(--space-4); }
  .parts { margin: 0; font-size: var(--text-xs); color: var(--ink-muted); }
  .formats { display: flex; flex-wrap: wrap; gap: var(--space-1); margin: 0; padding: 0; list-style: none; }

  .reasons { display: grid; gap: var(--space-2); }
  .reasons summary { cursor: pointer; font-size: var(--text-sm); font-weight: var(--weight-medium); color: var(--ink-muted); }
  .reasons[open] summary { margin-bottom: var(--space-2); }
  .count { font-weight: var(--weight-regular); }
  .group { display: grid; gap: var(--space-1-5); margin-top: var(--space-1-5); padding: var(--space-2) var(--space-3); background: var(--surface-sunken); border: var(--border-w) solid var(--line); border-radius: var(--radius-md); justify-items: start; }
  .group.safety { background: var(--tone-error-tint); border-color: var(--tone-error-line); }
  .safety .rule { color: var(--tone-error-ink); }
  .group ul { display: grid; gap: var(--space-1); margin: 0; padding-left: var(--space-5); font-size: var(--text-sm); }
  .group li { line-height: var(--leading-snug); }
  .rule { margin-left: var(--space-2); font-size: var(--text-xs); color: var(--ink-subtle); }

  footer { display: flex; justify-content: flex-end; }
  .never { display: flex; align-items: center; gap: var(--space-1-5); font-size: var(--text-sm); color: var(--tone-error-ink); }
</style>
