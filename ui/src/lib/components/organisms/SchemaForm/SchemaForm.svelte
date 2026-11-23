<!--
  SchemaForm: renders a plugin settings schema with no per-plugin
  UI code. Each field type maps to its field molecule; groups become fieldsets; conditional
  fields follow their parent; dynamic options load through `loadoptions` when the form opens.
  Managed-by-config fields are locked with their source key, drift is marked per field, secrets
  are write-only. Dirty tracking drives Save/Cancel; client checks run on save and the server's
  per-field messages (from `errors` or returned by `onsave`) win. Setup flows render
  as generic SetupFlow panels that fill the secret fields.

    <SchemaForm {schema} values={saved} meta={fieldMeta} onsave={(changes) => api.patch(changes)} />
-->
<script lang="ts">
  import { untrack } from 'svelte';
  import Button from '../../atoms/Button/Button.svelte';
  import Icon from '../../atoms/Icon/Icon.svelte';
  import Banner from '../../molecules/Banner/Banner.svelte';
  import TextField from '../../molecules/TextField/TextField.svelte';
  import SecretField from '../../molecules/SecretField/SecretField.svelte';
  import NumberField from '../../molecules/NumberField/NumberField.svelte';
  import BoolField from '../../molecules/BoolField/BoolField.svelte';
  import EnumField from '../../molecules/EnumField/EnumField.svelte';
  import MultiselectField from '../../molecules/MultiselectField/MultiselectField.svelte';
  import DurationField from '../../molecules/DurationField/DurationField.svelte';
  import SizeField from '../../molecules/SizeField/SizeField.svelte';
  import UrlField from '../../molecules/UrlField/UrlField.svelte';
  import PathField from '../../molecules/PathField/PathField.svelte';
  import RegexField from '../../molecules/RegexField/RegexField.svelte';
  import ListField from '../../molecules/ListField/ListField.svelte';
  import MapField, { type MapEntry } from '../../molecules/MapField/MapField.svelte';
  import type { PathLocation } from '../../molecules/PathPicker/PathPicker.svelte';
  import SetupFlow, { type SetupDriver } from '../SetupFlow/SetupFlow.svelte';
  import {
    changes, dirtyKeys, initialValues, isVisible, validate,
    type FieldMeta, type FormValues, type SchemaField, type SchemaOption, type SettingsSchema,
  } from '../../../forms/schema';
  import { formatCount } from '../../../format/quantity';

  interface Props {
    schema: SettingsSchema;
    /** Saved values from the server (secrets never included). */
    values?: FormValues;
    meta?: Record<string, FieldMeta>;
    /** Server-side messages by field key. */
    errors?: Record<string, string>;
    /** Accessible name of the form: "Connection settings for nzbgeek". */
    label: string;
    /** Loads dynamic options for a field. */
    loadoptions?: (field: SchemaField, values: FormValues) => Promise<SchemaOption[]>;
    /** Saves the changed values; resolves with per-field errors when the server refuses them. */
    onsave?: (changes: FormValues) => Promise<Record<string, string> | void> | Record<string, string> | void;
    oncancel?: () => void;
    setup?: SetupDriver;
    pathLocations?: PathLocation[];
    saveLabel?: string;
    /** Whole form read-only (no Save). */
    readonly?: boolean;
    now?: number;
    /** Show the "managed by config" banner; off when the page already shows one with the source. */
    managedBanner?: boolean;
  }
  let {
    schema, values = {}, meta = {}, errors = {}, label, loadoptions, onsave, oncancel, setup,
    pathLocations = [], saveLabel = 'Save', readonly = false, now, managedBanner = true,
  }: Props = $props();

  const id = $props.id();
  // svelte-ignore state_referenced_locally
  let baseline = $state<FormValues>(initialValues(schema, values));
  // svelte-ignore state_referenced_locally
  let draft = $state<FormValues>(initialValues(schema, values));
  let localMeta = $state<Record<string, FieldMeta>>({});
  // svelte-ignore state_referenced_locally
  let serverErrors = $state<Record<string, string>>({ ...errors });
  let attempted = $state(false);
  let saving = $state(false);
  let formError = $state<string>();
  let saved = $state(false);
  let summary = $state<HTMLElement>();
  let root = $state<HTMLFormElement>();

  // A new saved value set (another object, or a refetch) resets the form.
  $effect(() => {
    const next = initialValues(schema, values);
    untrack(() => {
      baseline = next;
      draft = structuredClone(next);
      localMeta = {};
      attempted = false;
    });
  });
  $effect(() => { serverErrors = { ...errors }; });

  const metaOf = (key: string): FieldMeta => ({ ...meta[key], ...localMeta[key] });
  const allMeta = $derived(Object.fromEntries(schema.fields.map((f) => [f.key, metaOf(f.key)])));
  const visible = $derived(schema.fields.filter((f) => isVisible(f, draft, schema)));
  const clientErrors = $derived(validate(schema, draft, allMeta));
  const shownErrors = $derived({ ...(attempted ? clientErrors : {}), ...serverErrors });
  const errorList = $derived(visible.filter((f) => shownErrors[f.key]));
  const dirty = $derived(dirtyKeys(schema, baseline, draft, allMeta));
  const managedCount = $derived(schema.fields.filter((f) => allMeta[f.key]?.managed).length);
  const allManaged = $derived(managedCount > 0 && managedCount === schema.fields.length);

  const sections = $derived.by(() => {
    const groups = schema.groups ?? [];
    const out: { id: string; label?: string; help?: string; fields: SchemaField[] }[] = [];
    const loose = visible.filter((f) => !f.group || !groups.some((g) => g.id === f.group));
    if (loose.length) out.push({ id: '', fields: loose });
    for (const g of groups) {
      const fields = visible.filter((f) => f.group === g.id);
      if (fields.length) out.push({ ...g, fields });
    }
    return out;
  });

  // Dynamic options: fetched once when the form opens, retried on demand.
  type OptState = { status: 'loading' | 'ready' | 'error'; options: SchemaOption[]; error?: string };
  let dyn = $state<Record<string, OptState>>({});
  async function load(field: SchemaField) {
    if (!loadoptions) return;
    dyn[field.key] = { status: 'loading', options: dyn[field.key]?.options ?? [] };
    try {
      dyn[field.key] = { status: 'ready', options: await loadoptions(field, $state.snapshot(draft)) };
    } catch (e) {
      dyn[field.key] = { status: 'error', options: [], error: e instanceof Error ? e.message : String(e) };
    }
  }
  $effect(() => {
    untrack(() => schema.fields.filter((f) => f.dynamicOptions).forEach(load));
  });
  const optionsOf = (f: SchemaField) => (f.dynamicOptions ? (dyn[f.key]?.options ?? []) : (f.options ?? []));

  function set(key: string, v: unknown) {
    draft[key] = v;
    saved = false;
    if (serverErrors[key]) {
      const { [key]: _, ...rest } = serverErrors;
      serverErrors = rest;
    }
  }
  const get = <T,>(key: string) => draft[key] as T;

  function focusField(key: string) {
    const el = root?.querySelector<HTMLElement>(`[data-key="${CSS.escape(key)}"]`);
    el?.querySelector<HTMLElement>('input:not([type=hidden]), select, textarea, button')?.focus();
  }

  async function save(e: SubmitEvent) {
    e.preventDefault();
    if (readonly || saving) return;
    attempted = true;
    formError = undefined;
    if (Object.keys(clientErrors).length) {
      queueMicrotask(() => summary?.focus());
      return;
    }
    const payload = changes(schema, baseline, $state.snapshot(draft), allMeta);
    saving = true;
    try {
      const refused = await onsave?.(payload);
      if (refused && Object.keys(refused).length) {
        serverErrors = refused;
        queueMicrotask(() => summary?.focus());
        return;
      }
      for (const f of schema.fields) {
        if (f.secret && payload[f.key] !== undefined) {
          localMeta[f.key] = { ...localMeta[f.key], isSet: true, ref: undefined };
          draft[f.key] = '';
        }
      }
      baseline = structuredClone($state.snapshot(draft));
      attempted = false;
      saved = true;
    } catch (err) {
      formError = err instanceof Error ? err.message : String(err);
    } finally {
      saving = false;
    }
  }
  function cancel() {
    draft = structuredClone($state.snapshot(baseline));
    serverErrors = {};
    attempted = false;
    formError = undefined;
    oncancel?.();
  }
  function setupDone(fills: string[]) {
    for (const k of fills) {
      localMeta[k] = { ...localMeta[k], isSet: true, ref: undefined };
      draft[k] = '';
    }
  }
</script>

{#snippet field(f: SchemaField)}
  {@const m = allMeta[f.key] ?? {}}
  {@const common = { label: f.label, help: f.help, error: shownErrors[f.key], required: f.required, managed: m.managed, drift: m.drift, disabled: readonly && !m.managed }}
  {@const v = f.validation ?? {}}
  <div class="slot" data-key={f.key} class:wide={f.type === 'map' || f.type === 'list' || f.type === 'regex' || f.type === 'multiselect'}>
    {#if f.secret || f.type === 'password'}
      <SecretField {...common} isSet={m.isSet} ref={m.ref} bind:value={() => get<string>(f.key), (x) => set(f.key, x)} />
    {:else if f.type === 'number'}
      <NumberField {...common} min={v.min} max={v.max} unit={f.unit} bind:value={() => get<number | null>(f.key), (x) => set(f.key, x)} />
    {:else if f.type === 'bool'}
      <BoolField {...common} bind:checked={() => get<boolean>(f.key), (x) => set(f.key, x)} />
    {:else if f.type === 'select'}
      {@const st = dyn[f.key]}
      <EnumField
        {...common}
        options={optionsOf(f)}
        appearance={f.appearance}
        placeholder={st?.status === 'loading' ? 'Loading options…' : (f.placeholder ?? `Choose a ${f.label.toLowerCase()}`)}
        error={common.error ?? (st?.status === 'error' ? `Couldn't load the options: ${st.error}` : undefined)}
        bind:value={() => get<string>(f.key), (x) => set(f.key, x)}
      />
      {#if f.dynamicOptions && st?.status !== 'loading' && !m.managed}
        <div class="reload"><Button size="sm" variant="ghost" icon="refresh" onclick={() => load(f)}>Reload options</Button></div>
      {/if}
    {:else if f.type === 'multiselect'}
      <MultiselectField {...common} options={optionsOf(f)} columns={optionsOf(f).length > 6 ? 2 : 1} bind:value={() => get<string[]>(f.key), (x) => set(f.key, x)} />
    {:else if f.type === 'duration'}
      <DurationField {...common} units={f.durationUnits} min={v.min} max={v.max} bind:value={() => get<number | null>(f.key), (x) => set(f.key, x)} />
    {:else if f.type === 'size'}
      <SizeField {...common} units={f.sizeUnits} min={v.min} max={v.max} bind:value={() => get<number | null>(f.key), (x) => set(f.key, x)} />
    {:else if f.type === 'url'}
      <UrlField {...common} schemes={v.schemes} placeholder={f.placeholder} bind:value={() => get<string>(f.key), (x) => set(f.key, x)} />
    {:else if f.type === 'path'}
      <PathField {...common} locations={pathLocations} placeholder={f.placeholder} bind:value={() => get<string>(f.key), (x) => set(f.key, x)} />
    {:else if f.type === 'regex'}
      <RegexField {...common} placeholder={f.placeholder} bind:value={() => get<string>(f.key), (x) => set(f.key, x)} />
    {:else if f.type === 'list'}
      <ListField {...common} itemLabel={f.itemLabel} mono={f.mono} max={v.maxItems} placeholder={f.placeholder} bind:value={() => get<string[]>(f.key), (x) => set(f.key, x)} />
    {:else if f.type === 'map'}
      <MapField {...common} mono={f.mono} bind:value={() => get<MapEntry[]>(f.key), (x) => set(f.key, x)} />
    {:else}
      <TextField {...common} mono={f.mono} placeholder={f.placeholder} bind:value={() => get<string>(f.key), (x) => set(f.key, x)} />
    {/if}
  </div>
{/snippet}

<form class="form" aria-label={label} onsubmit={save} novalidate bind:this={root}>
  {#if !managedBanner}
    <!-- The page names the config source. -->
  {:else if allManaged}
    <Banner preset="managed">Every setting here comes from the config file. Change them there; the form shows what is running.</Banner>
  {:else if managedCount}
    <Banner preset="managed" title="Partly managed by config">{formatCount(managedCount)} of {formatCount(schema.fields.length)} settings come from the config file and are read-only here.</Banner>
  {/if}

  {#if errorList.length || formError}
    <div class="summary" role="alert" tabindex="-1" bind:this={summary} aria-labelledby="{id}-sum">
      <p class="sum-title" id="{id}-sum">
        <Icon name="alert-octagon" size="sm" />
        {#if formError}Couldn't save: {formError}{:else}{errorList.length === 1 ? 'One setting needs attention' : `${errorList.length} settings need attention`}{/if}
      </p>
      {#if errorList.length}
        <ul>
          {#each errorList as f (f.key)}
            <li><button type="button" class="jump" onclick={() => focusField(f.key)}>{f.label}: {shownErrors[f.key]}</button></li>
          {/each}
        </ul>
      {/if}
    </div>
  {/if}

  {#if schema.setup?.length && setup}
    <div class="setups">
      {#each schema.setup as flow (flow.kind + flow.label)}
        <SetupFlow {flow} driver={setup} done={flow.fills.every((k) => allMeta[k]?.isSet || allMeta[k]?.ref)} disabled={readonly} ondone={setupDone} {now} />
      {/each}
    </div>
  {/if}

  {#each sections as s (s.id)}
    {#if s.label}
      <fieldset class="group">
        <legend><h3>{s.label}</h3></legend>
        {#if s.help}<p class="group-help">{s.help}</p>{/if}
        <div class="fields">{#each s.fields as f (f.key)}{@render field(f)}{/each}</div>
      </fieldset>
    {:else}
      <div class="fields">{#each s.fields as f (f.key)}{@render field(f)}{/each}</div>
    {/if}
  {/each}

  {#if !readonly && !allManaged}
    <div class="footer">
      <p class="state" role="status">
        {#if dirty.length}{dirty.length === 1 ? '1 unsaved change' : `${dirty.length} unsaved changes`}{:else if saved}Saved{:else}No changes{/if}
      </p>
      <div class="buttons">
        <Button onclick={cancel} disabled={saving || (!dirty.length && !oncancel)}>Cancel</Button>
        <Button type="submit" variant="primary" loading={saving} disabled={!dirty.length}>{saveLabel}</Button>
      </div>
    </div>
  {/if}
</form>

<style>
  .form { display: grid; gap: var(--space-6); min-width: 0; max-width: 48rem; }
  .fields { display: grid; grid-template-columns: repeat(auto-fill, minmax(min(100%, 18rem), 1fr)); gap: var(--space-5) var(--space-6); }
  .slot { display: grid; gap: var(--space-1); min-width: 0; align-content: start; }
  .slot.wide { grid-column: 1 / -1; }
  .reload { justify-self: start; }
  .group { border: 0; padding: 0; margin: 0; display: grid; gap: var(--space-3); min-width: 0; }
  .group legend { padding: 0; margin-bottom: var(--space-3); }
  h3 { font-size: var(--text-lg); font-weight: var(--weight-semibold); line-height: var(--leading-tight); }
  .group-help { color: var(--ink-muted); font-size: var(--text-md); max-width: var(--measure); margin-top: calc(-1 * var(--space-2)); }
  .setups { display: grid; gap: var(--space-3); }
  .summary {
    display: grid; gap: var(--space-2); padding: var(--space-3) var(--space-4);
    border: var(--border-w) solid var(--tone-error-line); background: var(--tone-error-tint); border-radius: var(--radius-lg);
  }
  .sum-title { display: flex; align-items: center; gap: var(--space-1-5); color: var(--tone-error-ink); font-weight: var(--weight-semibold); font-size: var(--text-md); }
  .summary ul { display: grid; gap: var(--space-1); padding-left: var(--space-6); }
  .jump {
    font: inherit; font-size: var(--text-md); color: var(--tone-error-ink); background: none; border: 0; padding: 0;
    text-align: start; text-decoration: underline; text-underline-offset: 0.2em; cursor: pointer;
  }
  .footer {
    display: flex; align-items: center; justify-content: space-between; gap: var(--space-3); flex-wrap: wrap;
    padding-top: var(--space-4); border-top: var(--border-w) solid var(--line);
  }
  .state { font-size: var(--text-md); color: var(--ink-muted); }
  .buttons { display: flex; gap: var(--space-2); margin-left: auto; }
  @media (width < 48rem), (pointer: coarse) {
    .jump { min-height: var(--target-min); }
    .buttons { width: 100%; }
    .buttons :global(> *) { flex: 1; }
  }
</style>
