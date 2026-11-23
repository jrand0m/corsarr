<!--
  EvidencePanel: why an automatic action happened, or will. Names the rule and its
  mode, gives the verdict in one plain sentence, compares each threshold with what was
  observed, lists the timestamped observations behind it, counts down to the action, marks
  dry runs ("would"), and offers Undo while the action can still be reversed. Every
  automatic accept, reject, removal, retry or skip links here. `variant="popover"`
  puts it behind a "Why?" button for table rows and queue cards; `panel` sits in a drawer.
-->
<script lang="ts" module>
  import type { StatusValue } from '../../../status/vocabulary';

  export type ReasonCategory = 'safety' | 'permanent' | 'preference' | 'temporary';
  export interface Threshold {
    label: string;
    /** The rule's limit, formatted: "2h", "1 peer". */
    limit: string;
    /** What was seen, formatted: "2h 14m", "0 peers". */
    observed: string;
    /** The observation crosses the limit. */
    exceeded: boolean;
  }
  export interface Evidence {
    rule: { id: string; name: string; href?: string };
    mode: StatusValue<'ruleMode'>;
    classification?: StatusValue<'classification'>;
    /** The conclusion in one plain sentence. */
    verdict: string;
    thresholds?: Threshold[];
    observations?: { at: Date | string | number; text: string }[];
    /** What the rule does (or did) and when. `verb` leads the countdown: "Removes". */
    action?: { summary: string; verb: string; at?: Date | string | number };
    reasons?: { category: ReasonCategory; message: string }[];
    links?: { label: string; href: string }[];
    /** The action was taken and can be undone until this time. */
    undoUntil?: Date | string | number;
  }
</script>

<script lang="ts">
  import Button from '../../atoms/Button/Button.svelte';
  import Icon from '../../atoms/Icon/Icon.svelte';
  import StatusBadge from '../../atoms/StatusBadge/StatusBadge.svelte';
  import Countdown from '../../atoms/Countdown/Countdown.svelte';
  import Timestamp from '../../atoms/Timestamp/Timestamp.svelte';
  import { placeNear } from '../../../overlay/anchor';
  import { toDate } from '../../../format/time';

  interface Props {
    evidence: Evidence;
    variant?: 'panel' | 'popover';
    /** Popover trigger text. */
    triggerLabel?: string;
    onundo?: () => void;
    /** Fixed "now" (ms) for stories and tests. */
    now?: number;
  }
  let { evidence: ev, variant = 'panel', triggerLabel = 'Why?', onundo, now }: Props = $props();

  const id = $props.id();
  const dry = $derived(ev.mode === 'dry-run');
  const nowDate = $derived(now === undefined ? undefined : new Date(now));
  const canUndo = $derived(!!onundo && ev.undoUntil !== undefined && toDate(ev.undoUntil).getTime() > (now ?? Date.now()));
  const order: ReasonCategory[] = ['safety', 'permanent', 'temporary', 'preference'];
  const grouped = $derived(
    order.map((c) => ({ c, items: (ev.reasons ?? []).filter((r) => r.category === c) })).filter((g) => g.items.length),
  );

  let panel = $state<HTMLElement>();
  let trigger = $state<HTMLElement>();
  let pos = $state({ top: 0, left: 0 });
  let open = $state(false);
  function ontoggle(e: Event) {
    open = (e as ToggleEvent).newState === 'open';
    if (open && trigger && panel) {
      pos = placeNear(trigger.getBoundingClientRect(), { width: panel.offsetWidth, height: panel.offsetHeight }, { width: innerWidth, height: innerHeight }, 'start');
    }
  }
</script>

{#snippet body()}
  <header>
    <h3 id="{id}-title">
      {#if ev.rule.href}<a href={ev.rule.href}>{ev.rule.name}</a>{:else}{ev.rule.name}{/if}
    </h3>
    <div class="badges">
      <StatusBadge family="ruleMode" value={ev.mode} size="sm" />
      {#if ev.classification}<StatusBadge family="classification" value={ev.classification} size="sm" />{/if}
    </div>
  </header>

  <p class="verdict">{ev.verdict}</p>

  {#if dry}
    <p class="dry"><Icon name="eye" size="sm" />Dry run: nothing changes. This shows what the rule would do.</p>
  {/if}

  {#if ev.action}
    <div class="action">
      <span>{dry ? 'Would: ' : ''}{ev.action.summary}</span>
      {#if ev.action.at !== undefined && !canUndo}
        <Countdown to={ev.action.at} action={dry ? `Would act` : ev.action.verb} tone="warn" {now} expiredLabel={dry ? 'would act now' : 'acting now'} />
      {/if}
    </div>
  {/if}

  {#if ev.thresholds?.length}
    <table>
      <caption class="visually-hidden">Thresholds compared with observations</caption>
      <thead><tr><th scope="col">Check</th><th scope="col" class="num">Limit</th><th scope="col" class="num">Observed</th><th scope="col">Result</th></tr></thead>
      <tbody>
        {#each ev.thresholds as t (t.label)}
          <tr>
            <th scope="row">{t.label}</th>
            <td class="num">{t.limit}</td>
            <td class="num" class:over={t.exceeded}>{t.observed}</td>
            <td>
              <span class={['result', t.exceeded ? 'over' : 'within']}>
                <Icon name={t.exceeded ? 'x-circle' : 'check-circle'} size="sm" />{t.exceeded ? 'Over limit' : 'Within limit'}
              </span>
            </td>
          </tr>
        {/each}
      </tbody>
    </table>
  {/if}

  {#if grouped.length}
    <div class="reasons">
      {#each grouped as g (g.c)}
        <div class="group">
          <StatusBadge family="reason" value={g.c} size="sm" appearance="plain" />
          <ul>{#each g.items as r, i (i)}<li>{r.message}</li>{/each}</ul>
        </div>
      {/each}
    </div>
  {/if}

  {#if ev.observations?.length}
    <section class="obs" aria-labelledby="{id}-obs">
      <h4 id="{id}-obs">Observations</h4>
      <ol>
        {#each ev.observations as o, i (i)}
          <li><Timestamp value={o.at} now={nowDate} /><span>{o.text}</span></li>
        {/each}
      </ol>
    </section>
  {/if}

  {#if canUndo || ev.links?.length}
    <footer>
      {#if canUndo}
        <Button size="sm" icon="undo" onclick={onundo}>Undo</Button>
        <span class="until">until <Timestamp value={ev.undoUntil!} mode="absolute" /></span>
      {/if}
      {#each ev.links ?? [] as l (l.href)}<a href={l.href}>{l.label}</a>{/each}
    </footer>
  {/if}
{/snippet}

{#if variant === 'popover'}
  <span class="trigger" bind:this={trigger}>
    <Button size="sm" variant="ghost" icon="help-circle" popovertarget="{id}-pop" aria-expanded={open} aria-controls="{id}-pop" aria-label="{triggerLabel} {ev.rule.name}">{triggerLabel}</Button>
  </span>
  <div class="ev pop" id="{id}-pop" popover="auto" bind:this={panel} {ontoggle} style:top="{pos.top}px" style:left="{pos.left}px" role="dialog" aria-labelledby="{id}-title">
    {@render body()}
  </div>
{:else}
  <section class="ev panel" aria-labelledby="{id}-title">{@render body()}</section>
{/if}

<style>
  .ev { display: grid; gap: var(--space-3); color: var(--ink); font-size: var(--text-md); }
  .panel { padding: var(--space-4); background: var(--surface-raised); border: var(--border-w) solid var(--line); border-radius: var(--radius-lg); }
  .pop {
    position: fixed;
    inset: auto;
    margin: 0;
    width: min(26rem, calc(100vw - var(--space-4)));
    max-height: min(32rem, calc(100dvh - var(--space-8)));
    overflow: auto;
    padding: var(--space-4);
    background: var(--surface-overlay);
    border: var(--border-w) solid var(--line);
    border-radius: var(--radius-md);
    box-shadow: var(--shadow-sm);
  }
  .trigger { display: inline-flex; }
  header { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: var(--space-2); }
  h3 { font-size: var(--text-base); }
  h3 a { color: inherit; }
  .badges { display: flex; gap: var(--space-1); }
  .verdict { font-weight: var(--weight-medium); line-height: var(--leading-snug); }
  .dry {
    display: flex;
    align-items: center;
    gap: var(--space-1-5);
    padding: var(--space-1-5) var(--space-2);
    font-size: var(--text-sm);
    color: var(--tone-info-ink);
    background: var(--tone-info-tint);
    border: var(--border-w) solid var(--tone-info-line);
    border-radius: var(--radius-sm);
  }
  .action { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: var(--space-1) var(--space-3); color: var(--ink-muted); }

  table { width: 100%; font-size: var(--text-sm); }
  th, td { padding: var(--space-1-5) var(--space-2); text-align: left; border-bottom: var(--border-w) solid var(--line); }
  thead th { font-weight: var(--weight-medium); color: var(--ink-muted); background: var(--surface-sunken); }
  tbody th { font-weight: var(--weight-regular); }
  .num { text-align: right; }
  td.over { color: var(--tone-error-ink); font-weight: var(--weight-medium); }
  .result { display: inline-flex; align-items: center; gap: var(--space-1); white-space: nowrap; }
  .result.over { color: var(--tone-error-ink); }
  .result.within { color: var(--tone-ok-ink); }

  .reasons { display: grid; gap: var(--space-2); }
  .group { display: grid; gap: var(--space-1); }
  .group ul { margin: 0; padding-left: var(--space-5); font-size: var(--text-sm); }

  .obs { display: grid; gap: var(--space-1-5); }
  h4 { font-size: var(--text-sm); color: var(--ink-muted); font-weight: var(--weight-medium); }
  .obs ol { display: grid; gap: var(--space-1); padding: 0; list-style: none; font-size: var(--text-sm); }
  .obs li { display: grid; grid-template-columns: 5.5rem minmax(0, 1fr); gap: var(--space-2); }
  .obs li :global(time) { color: var(--ink-muted); }

  footer { display: flex; flex-wrap: wrap; align-items: center; gap: var(--space-2) var(--space-3); padding-top: var(--space-1); font-size: var(--text-sm); }
  .until { color: var(--ink-muted); }

  @media (width < 48rem), (pointer: coarse) {
    footer a { display: inline-flex; align-items: center; min-height: var(--target-min); }
  }
</style>
