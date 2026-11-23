<!--
  PlanDiff (lazy chunk): what applying a config plan would change, and what an
  apply did change in history and drift views. Grouped by object type: creates, updates with
  a field-level before → after, prunes, and skipped or deferred prunes with the reason and
  what still uses the object. Secret fields never show a value, before or after: only
  "differs". With `onconfirm` it is a pending plan: include toggles per operation and
  Apply / Cancel. Load it with `loadPlanDiff()`, never statically.
-->
<script lang="ts" module>
  import type { StatusValue } from '../../../status/vocabulary';

  export type PlanOpKind = StatusValue<'planOp'>;
  export interface PlanField {
    /** Dotted field path: "api_key", "categories.tv". */
    path: string;
    before?: string | null;
    after?: string | null;
    /** Secret: values are never rendered, only "differs". */
    secret?: boolean;
  }
  export interface PlanOp {
    id: string;
    op: PlanOpKind;
    name: string;
    fields?: PlanField[];
    /** Why a prune was skipped or deferred. */
    reason?: string;
    inUseBy?: { label: string; href?: string }[];
  }
  export interface PlanGroup {
    /** Object type: "Indexers", "Quality profiles". */
    type: string;
    ops: PlanOp[];
  }
</script>

<script lang="ts">
  import Button from '../../atoms/Button/Button.svelte';
  import Checkbox from '../../atoms/Checkbox/Checkbox.svelte';
  import Icon from '../../atoms/Icon/Icon.svelte';
  import StatusBadge from '../../atoms/StatusBadge/StatusBadge.svelte';
  import EmptyState from '../../molecules/EmptyState/EmptyState.svelte';
  import { formatCount } from '../../../format/quantity';

  interface Props {
    groups: PlanGroup[];
    /** Ids left out of the apply (pending plans). */
    excluded?: string[];
    /** Pending plan: show include toggles and Apply / Cancel. */
    onconfirm?: (included: string[]) => void;
    oncancel?: () => void;
    busy?: boolean;
    label?: string;
  }
  let { groups, excluded = $bindable([]), onconfirm, oncancel, busy = false, label = 'Plan' }: Props = $props();

  const uid = $props.id();
  const actionable = (o: PlanOp) => o.op === 'create' || o.op === 'update' || o.op === 'prune';
  const all = $derived(groups.flatMap((g) => g.ops));
  const counts = $derived(
    (['create', 'update', 'prune', 'skip', 'deferred'] as PlanOpKind[])
      .map((op) => ({ op, n: all.filter((o) => o.op === op).length }))
      .filter((c) => c.n),
  );
  const included = $derived(all.filter((o) => actionable(o) && !excluded.includes(o.id)).map((o) => o.id));
  const shown = (v: string | null | undefined) => (v === undefined || v === null || v === '' ? '—' : v);

  function toggle(id: string, on: boolean) {
    excluded = on ? excluded.filter((x) => x !== id) : [...excluded, id];
  }
</script>

<section class="plan" aria-label={label}>
  {#if !all.length}
    <EmptyState title="Nothing to change" icon="check-circle" size="sm">The running config already matches the source.</EmptyState>
  {:else}
    <ul class="summary" aria-label="Summary">
      {#each counts as c (c.op)}
        <li><StatusBadge family="planOp" value={c.op} detail={formatCount(c.n)} size="sm" /></li>
      {/each}
    </ul>

    {#each groups as g, gi (g.type)}
      {#if g.ops.length}
        <section class="group" aria-labelledby="{uid}-g{gi}">
          <h3 id="{uid}-g{gi}">{g.type} <span class="n num">{formatCount(g.ops.length)}</span></h3>
          <ul class="ops">
            {#each g.ops as o (o.id)}
              {@const off = excluded.includes(o.id)}
              <li class={['op', o.op]} class:off>
                <div class="head">
                  {#if onconfirm && actionable(o)}
                    <Checkbox checked={!off} aria-label="Include {o.op} {o.name}" onchange={(e) => toggle(o.id, (e.currentTarget as HTMLInputElement).checked)} />
                  {/if}
                  <StatusBadge family="planOp" value={o.op} size="sm" />
                  <span class="name">{o.name}</span>
                  {#if off}<span class="left-out">Left out</span>{/if}
                </div>

                {#if o.fields?.length}
                  <table>
                    <caption class="visually-hidden">Field changes for {o.name}</caption>
                    <thead><tr><th scope="col">Field</th><th scope="col">Before</th><th scope="col"><span class="visually-hidden">changes to</span></th><th scope="col">After</th></tr></thead>
                    <tbody>
                      {#each o.fields as f (f.path)}
                        <tr>
                          <th scope="row"><code>{f.path}</code></th>
                          {#if f.secret}
                            <td colspan="3" class="secret"><Icon name="key" size="sm" /><span aria-hidden="true">••••</span> differs</td>
                          {:else}
                            <td class="before"><code>{shown(f.before)}</code></td>
                            <td class="arrow" aria-hidden="true"><Icon name="arrow-right" size="sm" /></td>
                            <td class="after"><code>{shown(f.after)}</code></td>
                          {/if}
                        </tr>
                      {/each}
                    </tbody>
                  </table>
                {/if}

                {#if o.reason || o.inUseBy?.length}
                  <p class="why">
                    {#if o.reason}<span>{o.reason}</span>{/if}
                    {#if o.inUseBy?.length}
                      <span class="use">In use by
                        {#each o.inUseBy as u, i (u.label)}{#if i > 0}, {/if}{#if u.href}<a href={u.href}>{u.label}</a>{:else}{u.label}{/if}{/each}
                      </span>
                    {/if}
                  </p>
                {/if}
              </li>
            {/each}
          </ul>
        </section>
      {/if}
    {/each}

    {#if onconfirm}
      <footer>
        {#if oncancel}<Button variant="ghost" onclick={oncancel} disabled={busy}>Cancel</Button>{/if}
        <Button variant="primary" loading={busy} disabled={!included.length} onclick={() => onconfirm(included)}>
          {included.length ? `Apply ${formatCount(included.length)} ${included.length === 1 ? 'change' : 'changes'}` : 'Nothing included'}
        </Button>
      </footer>
    {/if}
  {/if}
</section>

<style>
  .plan { display: grid; gap: var(--space-5); color: var(--ink); font-size: var(--text-md); min-width: 0; }
  .summary { display: flex; flex-wrap: wrap; gap: var(--space-1-5); padding: 0; list-style: none; }
  .group { display: grid; gap: var(--space-2); }
  h3 { display: flex; align-items: baseline; gap: var(--space-2); font-size: var(--text-base); }
  .n { font-size: var(--text-sm); font-weight: var(--weight-regular); color: var(--ink-muted); }
  .ops { display: grid; padding: 0; list-style: none; background: var(--surface-raised); border: var(--border-w) solid var(--line); border-radius: var(--radius-lg); }
  .op { display: grid; gap: var(--space-2); padding: var(--space-3); min-width: 0; }
  .op + .op { border-top: var(--border-w) solid var(--line); }
  .head { display: flex; flex-wrap: wrap; align-items: center; gap: var(--space-2); min-width: 0; }
  .name { font-weight: var(--weight-medium); overflow-wrap: anywhere; }
  .off .name { color: var(--ink-muted); text-decoration: line-through; }
  .left-out { font-size: var(--text-xs); color: var(--ink-muted); }

  table { width: 100%; font-size: var(--text-sm); table-layout: auto; }
  th, td { padding: var(--space-1) var(--space-2); text-align: left; vertical-align: top; border-top: var(--border-w) solid var(--line); }
  thead th { font-weight: var(--weight-medium); color: var(--ink-muted); background: var(--surface-sunken); border-top: 0; }
  tbody th { font-weight: var(--weight-regular); white-space: nowrap; }
  code { font-size: var(--text-sm); overflow-wrap: anywhere; }
  .before code { color: var(--tone-error-ink); }
  .after code { color: var(--tone-ok-ink); }
  .arrow { width: var(--icon-sm); color: var(--ink-subtle); }
  .secret { display: flex; align-items: center; gap: var(--space-1-5); color: var(--tone-warn-ink); }
  .why { display: flex; flex-wrap: wrap; gap: var(--space-1) var(--space-3); font-size: var(--text-sm); color: var(--ink-muted); }

  footer { display: flex; justify-content: flex-end; gap: var(--space-2); }

  @media (width < 48rem) {
    thead { display: none; }
    tr { display: grid; grid-template-columns: minmax(0, 1fr) auto minmax(0, 1fr); border-top: var(--border-w) solid var(--line); }
    tbody th { grid-column: 1 / -1; border: 0; padding-bottom: 0; }
    td { border: 0; }
    .secret { grid-column: 1 / -1; }
  }
</style>
