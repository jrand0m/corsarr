<!--
  DecisionPanel: one release as the decision engine judged it. Header with the
  release and its facts, the verdict, rank among candidates, the score with every custom
  format that made it, and ALL failing reasons grouped by category with a plain line on what
  each category means. The override rule: preference and temporary reasons
  can be grabbed anyway; permanent and blocklist reasons need a confirmation; safety
  reasons are never overridden, so no button is offered. `ongrab` receives the overridden
  reasons for `grab.sent`.
-->
<script lang="ts" module>
  import type { ReasonCategory } from '../EvidencePanel/EvidencePanel.svelte';

  export interface DecisionReason {
    rule: string;
    category: ReasonCategory;
    /** The failing comparison in plain words: "58.2 GB is over the 40 GB cap". */
    message: string;
    /** The release is blocklisted (overridable only with confirmation). */
    blocklist?: boolean;
  }
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
    };
    verdict: 'accept' | 'reject';
    rank?: number;
    candidates?: number;
    score: { total: number; formats: { name: string; score: number }[] };
    reasons: DecisionReason[];
  }
  export type OverrideLevel = 'allowed' | 'confirm' | 'never';

  /** Override rule: never for safety, confirm for permanent and blocklist, else allowed. */
  export function overrideLevel(reasons: DecisionReason[]): OverrideLevel {
    if (reasons.some((r) => r.category === 'safety')) return 'never';
    if (reasons.some((r) => r.category === 'permanent' || r.blocklist)) return 'confirm';
    return 'allowed';
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
  import { formatCount } from '../../../format/quantity';

  interface Props {
    decision: Decision;
    ongrab?: (overridden: DecisionReason[]) => void;
    /** Fixed "now" (ms) for stories and tests. */
    now?: number;
    /** Heading level of the release title inside the page outline. */
    level?: 2 | 3;
  }
  let { decision: d, ongrab, now, level = 3 }: Props = $props();

  const id = $props.id();
  const order: ReasonCategory[] = ['safety', 'permanent', 'temporary', 'preference'];
  const explain: Record<ReasonCategory, string> = {
    safety: 'Protects your machine and library. Never overridden.',
    permanent: 'Will not change by waiting: blocklisted, wrong item or below the profile floor. Overriding needs a confirmation.',
    temporary: 'May pass later: seeders, a delay profile or an indexer limit. You can grab anyway.',
    preference: 'Your profile’s choices: size, formats, resolution cap. You can grab anyway.',
  };
  const grouped = $derived(order.map((c) => ({ c, items: d.reasons.filter((r) => r.category === c) })).filter((g) => g.items.length));
  const level$ = $derived(overrideLevel(d.reasons));
  const total = $derived(d.score.total > 0 ? `+${formatCount(d.score.total)}` : d.score.total < 0 ? `−${formatCount(-d.score.total)}` : '0');
  let confirming = $state(false);
</script>

<section class="dp" aria-labelledby="{id}-title">
  <header>
    <svelte:element this={`h${level}`} id="{id}-title" class="title">{d.release.title}</svelte:element>
    <div class="facts">
      <span>{d.release.quality}</span>
      <Quantity kind="bytes" value={d.release.size} />
      {#if d.release.runtimeSeconds}<Quantity kind="gbph" value={d.release.size} runtime={d.release.runtimeSeconds} />{/if}
      {#if d.release.protocol === 'torrent'}
        <span>{d.release.seeders === undefined ? '—' : `${formatCount(d.release.seeders)} seeders`}</span>
      {:else if d.release.protocol === 'source'}<span>Direct source</span>
      {:else}<span>Usenet</span>{/if}
      <span>{d.release.indexer}</span>
      <Timestamp value={d.release.publishedAt} now={now === undefined ? undefined : new Date(now)} />
    </div>
  </header>

  <div class="verdict">
    <StatusBadge family="verdict" value={d.verdict} />
    {#if d.rank !== undefined}
      <span class="rank num">Ranked {d.rank}{d.candidates ? ` of ${formatCount(d.candidates)}` : ""}</span>
    {/if}
    {#if d.verdict === 'reject'}<StatusBadge family="override" value={level$} size="sm" appearance="plain" />{/if}
  </div>

  <div class="score">
    <span class="total">Score <strong class="num">{total}</strong></span>
    {#if d.score.formats.length}
      <ul class="formats" aria-label="Custom formats">
        {#each d.score.formats as f (f.name)}<li><Chip variant="score" label={f.name} score={f.score} /></li>{/each}
      </ul>
    {:else}<span class="none">No custom formats matched</span>{/if}
  </div>

  {#if grouped.length}
    <div class="reasons">
      <h4 class="sub">Why it was rejected <span class="count">({d.reasons.length})</span></h4>
      {#each grouped as g (g.c)}
        <div class={['group', g.c]}>
          <div class="ghead">
            <StatusBadge family="reason" value={g.c} size="sm" />
            <span class="explain">{explain[g.c]}</span>
          </div>
          <ul>
            {#each g.items as r, i (i)}
              <li><span class="msg">{r.message}</span><code class="rule">{r.rule}</code></li>
            {/each}
          </ul>
        </div>
      {/each}
    </div>
  {/if}

  {#if ongrab}
    <footer>
      {#if d.verdict === 'accept'}
        <Button variant="primary" icon="download" onclick={() => ongrab([])}>Grab</Button>
      {:else if level$ === 'allowed'}
        <Button icon="download" onclick={() => ongrab(d.reasons)}>Grab anyway</Button>
      {:else if level$ === 'confirm'}
        <Button icon="download" onclick={() => (confirming = true)}>Grab anyway…</Button>
      {:else}
        <p class="never"><Icon name="lock" size="sm" />A safety rule failed, so this release can’t be grabbed.</p>
      {/if}
    </footer>
  {/if}
</section>

{#if ongrab && level$ === 'confirm'}
  <ConfirmDialog
    bind:open={confirming} title="Grab {d.release.title} anyway?" variant="primary" confirmLabel="Grab anyway"
    consequences={[...d.reasons.map((r) => `Overrides: ${r.message}`), 'Records the override in History']}
    undoable="You can remove the download from the queue at any time."
    onconfirm={() => ongrab(d.reasons)}
  />
{/if}

<style>
  .dp {
    display: grid;
    gap: var(--space-3);
    min-width: 0;
    padding: var(--space-4);
    color: var(--ink);
    background: var(--surface-raised);
    border: var(--border-w) solid var(--line);
    border-radius: var(--radius-lg);
  }
  header { display: grid; gap: var(--space-1); min-width: 0; }
  .title { font-family: var(--font-mono); font-size: var(--text-md); font-weight: var(--weight-medium); overflow-wrap: anywhere; line-height: var(--leading-snug); }
  .facts { display: flex; flex-wrap: wrap; gap: var(--space-0-5) var(--space-3); font-size: var(--text-sm); color: var(--ink-muted); }
  .verdict { display: flex; flex-wrap: wrap; align-items: center; gap: var(--space-2) var(--space-3); }
  .rank { font-size: var(--text-sm); font-weight: var(--weight-medium); }
  .score { display: flex; flex-wrap: wrap; align-items: center; gap: var(--space-2); }
  .total { font-size: var(--text-sm); color: var(--ink-muted); }
  .total strong { color: var(--ink); }
  .formats { display: flex; flex-wrap: wrap; gap: var(--space-1); padding: 0; list-style: none; }
  .none { font-size: var(--text-sm); color: var(--ink-subtle); }

  .reasons { display: grid; gap: var(--space-2); }
  .sub { font-size: var(--text-md); }
  .count { color: var(--ink-muted); font-weight: var(--weight-regular); }
  .group { display: grid; gap: var(--space-1-5); padding: var(--space-2) var(--space-3); background: var(--surface-sunken); border: var(--border-w) solid var(--line); border-radius: var(--radius-md); }
  .group.safety { background: var(--tone-error-tint); border-color: var(--tone-error-line); }
  .safety .explain, .safety .rule { color: var(--tone-error-ink); }
  .ghead { display: flex; flex-wrap: wrap; align-items: center; gap: var(--space-1) var(--space-2); }
  .explain { font-size: var(--text-xs); color: var(--ink-muted); }
  .group ul { display: grid; gap: var(--space-1); margin: 0; padding-left: var(--space-5); font-size: var(--text-sm); }
  .group li { line-height: var(--leading-snug); }
  .rule { margin-left: var(--space-2); font-size: var(--text-xs); color: var(--ink-subtle); }

  footer { display: flex; justify-content: flex-end; }
  .never { display: flex; align-items: center; gap: var(--space-1-5); font-size: var(--text-sm); color: var(--tone-error-ink); }
</style>
