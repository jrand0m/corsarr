<!--
  SettingsPage (template): settings section lists and object editors in one frame. A section list
  (side column on wide screens, a disclosure on narrow ones), the page header, the "Managed by
  config" banner with the source key and file line, an optional SchemaForm that
  locks read-only when managed, and a slot after the form for tests (LiveTestBox, CheckResults)
  shown on save. Lists and per-service tabs go in `children`.
-->
<script lang="ts" module>
  export interface SettingsSection {
    id: string;
    label: string;
    href: string;
    /** Objects in the section ("4 indexers"). */
    count?: number;
    /** Something in the section needs attention. */
    attention?: boolean;
  }
</script>

<script lang="ts">
  import type { Snippet } from 'svelte';
  import Icon from '../../atoms/Icon/Icon.svelte';
  import StatusDot from '../../atoms/StatusDot/StatusDot.svelte';
  import Banner from '../../molecules/Banner/Banner.svelte';
  import type { PathLocation } from '../../molecules/PathPicker/PathPicker.svelte';
  import SchemaForm from '../../organisms/SchemaForm/SchemaForm.svelte';
  import type { FieldMeta, FormValues, SchemaField, SchemaOption, SettingsSchema } from '../../../forms/schema';
  import { formatCount } from '../../../format/quantity';

  interface Props {
    sections: SettingsSection[];
    current: string;
    title: string;
    description?: string;
    actions?: Snippet;
    banner?: Snippet;
    /** Managed by config: `true` or the source key ("indexers.dock_feeds"). */
    managed?: boolean | string;
    /** Where in the config file: "corsarr.yaml, line 42". */
    managedSource?: string;
    /** Buttons on the managed banner ("Copy as YAML"). */
    managedActions?: Snippet;
    children?: Snippet;
    schema?: SettingsSchema;
    values?: FormValues;
    meta?: Record<string, FieldMeta>;
    errors?: Record<string, string>;
    formLabel?: string;
    saveLabel?: string;
    loadoptions?: (field: SchemaField, values: FormValues) => Promise<SchemaOption[]>;
    onsave?: (changes: FormValues) => Promise<Record<string, string> | void> | Record<string, string> | void;
    oncancel?: () => void;
    pathLocations?: PathLocation[];
    /** After the form: the test run on save (LiveTestBox, CheckResults). */
    test?: Snippet;
    now?: number;
  }
  let {
    sections, current, title, description, actions, banner, managed, managedSource, managedActions, children, schema, values, meta,
    errors, formLabel, saveLabel, loadoptions, onsave, oncancel, pathLocations, test, now,
  }: Props = $props();

  const here = $derived(sections.find((s) => s.id === current));
</script>

{#snippet sectionLinks()}
  <ul>
    {#each sections as s (s.id)}
      <li>
        <a href={s.href} aria-current={s.id === current ? 'page' : undefined}>
          <span class="name">{s.label}</span>
          {#if s.attention}<StatusDot tone="error" label="Needs attention" />{/if}
          {#if s.count !== undefined}<span class="count num">{formatCount(s.count)}</span>{/if}
        </a>
      </li>
    {/each}
  </ul>
{/snippet}

<div class="frame">
<div class="sp">
  <nav class="sections wide" aria-label="Settings sections">{@render sectionLinks()}</nav>
  <details class="sections narrow">
    <summary><span>Section: <strong>{here?.label ?? title}</strong></span><Icon name="chevron-down" size="sm" /></summary>
    <nav aria-label="Settings sections, compact">{@render sectionLinks()}</nav>
  </details>

  <div class="content">
    <header class="head">
      <div class="titles">
        <h1>{title}</h1>
        {#if description}<p class="desc">{description}</p>{/if}
      </div>
      {#if actions}<div class="actions">{@render actions()}</div>{/if}
    </header>

    {#if managed}
      <Banner preset="managed">
        Set in the config file{typeof managed === 'string' ? ' under ' : ''}{#if typeof managed === 'string'}<code>{managed}</code>{/if}{managedSource ? ` (${managedSource})` : ''}.
        Change it there; corsarr applies the file on save.
        {#snippet actions()}{#if managedActions}{@render managedActions()}{/if}{/snippet}
      </Banner>
    {/if}
    {#if banner}{@render banner()}{/if}

    {#if children}{@render children()}{/if}

    {#if schema}
      <div class="editor">
        <SchemaForm {schema} {values} {meta} {errors} label={formLabel ?? title} {loadoptions} {onsave} {oncancel} {pathLocations} {saveLabel} readonly={!!managed} managedBanner={!managed} {now} />
      </div>
    {/if}

    {#if test}<section class="test" aria-label="Test">{@render test()}</section>{/if}
  </div>
</div>
</div>

<style>
  .frame { container-type: inline-size; min-width: 0; }
  .sp {
    display: grid; gap: var(--space-4) var(--space-8); min-width: 0;
    grid-template-columns: minmax(0, 1fr);
  }
  .content { display: grid; gap: var(--space-5); align-content: start; min-width: 0; max-width: 52rem; }
  .head { display: flex; align-items: flex-end; justify-content: space-between; gap: var(--space-3) var(--space-4); flex-wrap: wrap; }
  .titles { display: grid; gap: var(--space-1); min-width: 0; }
  h1 { font-size: var(--text-xl); line-height: var(--leading-tight); }
  .desc { color: var(--ink-muted); font-size: var(--text-md); max-width: var(--measure); }
  .actions { display: flex; gap: var(--space-2); flex-wrap: wrap; }
  code { font-family: var(--font-mono); font-size: var(--text-sm); }
  .editor { padding: var(--space-5); background: var(--surface-raised); border: var(--border-w) solid var(--line); border-radius: var(--radius-lg); min-width: 0; }

  ul { list-style: none; margin: 0; padding: 0; display: grid; gap: var(--space-0-5); }
  .sections a {
    display: flex; align-items: center; gap: var(--space-2); min-height: var(--control-h-md); padding: 0 var(--space-3);
    border-radius: var(--radius-md); color: var(--ink); text-decoration: none; font-size: var(--text-md);
    transition: background-color var(--dur-fast) var(--ease-out);
  }
  .sections a:hover { background: var(--surface-hover); }
  .sections a[aria-current='page'] { background: var(--surface-selected); font-weight: var(--weight-semibold); }
  .name { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .count { color: var(--ink-muted); font-size: var(--text-sm); }
  .wide { display: none; }
  .narrow { border: var(--border-w) solid var(--line); border-radius: var(--radius-lg); background: var(--surface-raised); }
  summary {
    display: flex; align-items: center; justify-content: space-between; gap: var(--space-2); min-height: var(--target-min);
    padding: 0 var(--space-3); cursor: pointer; list-style: none; font-size: var(--text-md); color: var(--ink-muted);
  }
  summary::-webkit-details-marker { display: none; }
  summary strong { color: var(--ink); font-weight: var(--weight-semibold); }
  details[open] summary :global(svg) { transform: rotate(180deg); }
  .narrow nav { padding: 0 var(--space-1) var(--space-2); }
  .narrow a { min-height: var(--target-min); }

  @container (width >= 52rem) {
    .sp { grid-template-columns: 13rem minmax(0, 1fr); }
    .wide { display: block; position: sticky; top: calc(3.5rem + var(--space-6)); align-self: start; }
    .narrow { display: none; }
  }
  @media (width < 48rem) {
    .editor { padding: var(--space-4) var(--space-3); }
  }
</style>
