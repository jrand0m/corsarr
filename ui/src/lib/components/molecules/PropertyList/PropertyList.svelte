<!--
  PropertyList: label/value pairs for detail views (a download's properties, an
  indexer's caps, diagnostics). Missing values show an em dash. A value can be a link, a
  copyable value (CopyableValue) or mono data; the `value` snippet renders anything richer
  (a badge, a timestamp) for rows marked `custom`. Two columns on wide screens, stacked on phones.
-->
<script lang="ts" module>
  import type { ManagedSource } from '../../atoms/ManagedValue/ManagedValue.svelte';

  export interface Property {
    key: string;
    label: string;
    value?: string | number | null;
    href?: string;
    copy?: boolean;
    secret?: boolean;
    mono?: boolean;
    /** Set by the config: shows the lock after the value; with its key and source, the lock says where. */
    managed?: boolean | ManagedSource;
    /** Render this row's value with the `value` snippet. */
    custom?: boolean;
  }
</script>

<script lang="ts">
  import type { Snippet } from 'svelte';
  import CopyableValue from '../../atoms/CopyableValue/CopyableValue.svelte';
  import Link from '../../atoms/Link/Link.svelte';
  import Icon from '../../atoms/Icon/Icon.svelte';
  import ManagedValue from '../../atoms/ManagedValue/ManagedValue.svelte';
  import type { Density } from '../../../tokens';

  interface Props {
    items: Property[];
    density?: Density;
    /** Value renderer for rows marked `custom`. */
    value?: Snippet<[Property]>;
    /** Layout: side by side (default) or label above value. */
    layout?: 'columns' | 'stacked';
    label?: string;
  }
  let { items, density = 'comfortable', value: custom, layout = 'columns', label }: Props = $props();
  const empty = (v: Property['value']) => v === undefined || v === null || v === '';
</script>

<dl class={['pl', density, layout]} aria-label={label}>
  {#each items as p (p.key)}
    <div class="row">
      <dt>{p.label}</dt>
      <dd>
        {#if p.custom && custom}
          {@render custom(p)}
        {:else if empty(p.value)}
          <span class="none" aria-label="Not set">—</span>
        {:else if p.copy}
          <CopyableValue value={String(p.value)} label={p.label} secret={p.secret} mono={p.mono ?? true} />
        {:else if p.href}
          <Link href={p.href}>{p.value}</Link>
        {:else}
          <span class:mono={p.mono} class="v">{p.value}</span>
        {/if}
        {#if typeof p.managed === 'object'}<ManagedValue managed={p.managed} />
        {:else if p.managed}<span class="seal"><Icon name="lock" size="sm" label="Managed by config" /></span>{/if}
      </dd>
    </div>
  {/each}
</dl>

<style>
  .pl { display: grid; margin: 0; font-size: var(--text-md); }
  .row {
    display: grid;
    grid-template-columns: minmax(8rem, 14rem) minmax(0, 1fr);
    gap: var(--space-1) var(--space-4);
    align-items: baseline;
    padding-block: var(--space-2);
    border-bottom: var(--border-w) solid var(--line);
  }
  .row:last-child { border-bottom: 0; }
  .compact .row { padding-block: var(--space-1); font-size: var(--text-sm); }
  .stacked .row { grid-template-columns: minmax(0, 1fr); gap: var(--space-0-5); }
  dt { color: var(--ink-muted); }
  dd { display: flex; flex-wrap: wrap; align-items: center; gap: var(--space-1-5); min-width: 0; margin: 0; color: var(--ink); }
  .v { overflow-wrap: anywhere; }
  .mono { font-family: var(--font-mono); font-size: var(--text-sm); }
  .none { color: var(--ink-subtle); }
  .seal { display: inline-flex; color: var(--tone-managed-ink); }
  @media (width < 48rem) {
    .row { grid-template-columns: minmax(0, 1fr); gap: var(--space-0-5); }
    dt { font-size: var(--text-sm); }
  }
</style>
