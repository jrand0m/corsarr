<!--
  PluginSettingsForm: a plugin module's settings form, built only from its typed schema. One pure
  converter maps the closed field types onto SchemaForm's field molecules; a schema with anything
  else shows "This plugin's settings cannot be shown" and renders nothing from it. Labels,
  descriptions, option labels, action labels and action results are bound as text. Up to three
  action buttons send the current values and show the plain-text answer. A configured instance is
  read-only with the managed lock; a new instance is a draft the server checks (`onsubmit`).

    <PluginSettingsForm schema={manifest.settings} values={config} meta={locks} readonly onaction={run} />
-->
<script lang="ts" module>
  import type { ActionResult, ModuleSchema, Problem } from '../../../forms/plugin-types';

  /** What an action call resolves with: the plugin's answer, per-field problems, or a refused text. */
  export type ActionReply = ActionResult | { problems: Problem[] } | { refused: true };
  export type { ModuleSchema, Problem };
</script>

<script lang="ts">
  import Button from '../../atoms/Button/Button.svelte';
  import Banner from '../../molecules/Banner/Banner.svelte';
  import PluginActionResult, { type ActionState } from '../../molecules/PluginActionResult/PluginActionResult.svelte';
  import SchemaForm from '../SchemaForm/SchemaForm.svelte';
  import type { FieldMeta, FormValues } from '../../../forms/schema';
  import { UnsupportedSchema, fromFormValues, problemErrors, toFormSchema, toFormValues } from '../../../forms/plugin-schema';

  interface Props {
    schema: ModuleSchema;
    /** Saved values in the module's types (config values may be strings). */
    values?: Record<string, unknown>;
    meta?: Record<string, FieldMeta>;
    /** Problems the server found in the values. */
    problems?: Problem[];
    /** Accessible name: "Upstairs box settings". */
    label: string;
    readonly?: boolean;
    /** Page already names the config source. */
    managedBanner?: boolean;
    /** Runs a plugin action with the current values. */
    onaction?: (id: string, values: Record<string, unknown>) => Promise<ActionReply>;
    /** Checks the draft; resolves with the problems the server found (none: accepted). */
    onsubmit?: (values: Record<string, unknown>) => Promise<Problem[] | void>;
    submitLabel?: string;
    /** Current values in the module's types, on every change. */
    onvalues?: (values: Record<string, unknown>) => void;
    /** A result already on screen (stories). */
    result?: { action: string; state: ActionState; text?: string };
  }
  let {
    schema, values = {}, meta = {}, problems = [], label, readonly = false, managedBanner = false,
    onaction, onsubmit, submitLabel = 'Check settings', onvalues, result: initialResult,
  }: Props = $props();

  const form = $derived.by(() => {
    try {
      return { ok: true as const, ...toFormSchema(schema) };
    } catch (e) {
      if (e instanceof UnsupportedSchema) return { ok: false as const };
      throw e;
    }
  });
  const formValues = $derived(form.ok ? toFormValues(schema.fields, values) : {});
  const keys = $derived(form.ok ? form.schema.fields.map((f) => f.key) : []);
  // svelte-ignore state_referenced_locally
  let fieldProblems = $state<Problem[]>(problems);
  $effect(() => { fieldProblems = problems; });
  const mapped = $derived(form.ok ? problemErrors(fieldProblems, keys, form.hints) : { fields: {}, other: [] });

  let current: Record<string, unknown> = {};
  function track(v: FormValues) {
    current = fromFormValues(schema.fields, v);
    onvalues?.(current);
  }

  // svelte-ignore state_referenced_locally
  let result = $state(initialResult);
  let running = $state<string>();

  async function run(id: string, actionLabel: string) {
    if (!onaction || running) return;
    running = id;
    result = { action: actionLabel, state: 'running' };
    try {
      const r = await onaction(id, { ...current });
      if ('refused' in r) result = { action: actionLabel, state: 'refused' };
      else if ('problems' in r) {
        fieldProblems = r.problems;
        result = { action: actionLabel, state: 'fail', text: 'Some settings need attention first.' };
      } else result = { action: actionLabel, state: r.outcome, text: String(r.text ?? '') };
    } catch (e) {
      result = { action: actionLabel, state: 'fail', text: e instanceof Error ? e.message : String(e) };
    } finally {
      running = undefined;
    }
  }

  async function submit(): Promise<Record<string, string> | void> {
    const found = (await onsubmit?.({ ...current })) ?? [];
    fieldProblems = found;
    const m = problemErrors(found, keys, form.ok ? form.hints : {});
    if (Object.keys(m.fields).length) return m.fields;
  }
</script>

{#if !form.ok}
  <Banner tone="warn" icon="alert-triangle" title="This plugin's settings cannot be shown">
    Its settings use something this version of corsarr does not support. Update corsarr or the plugin; the plugin's own text is not shown here.
  </Banner>
{:else}
  <div class="psf">
    <SchemaForm
      schema={form.schema} values={formValues} {meta} errors={mapped.fields} {label} {readonly} {managedBanner}
      onsave={!readonly && onsubmit ? submit : undefined} saveLabel={submitLabel} onvalues={track}
    />
    {#if mapped.other.length}
      <div class="other" role="alert">
        <p class="other-title">The server also reported</p>
        <ul>{#each mapped.other as line, i (i)}<li>{line}</li>{/each}</ul>
      </div>
    {/if}
    {#if form.actions.length && onaction}
      <div class="actions" role="group" aria-label="Plugin actions">
        {#each form.actions as a (a.id)}
          <Button size="sm" icon="bolt" loading={running === a.id} disabled={!!running && running !== a.id} onclick={() => run(a.id, a.label)}>{a.label}</Button>
        {/each}
      </div>
    {/if}
    {#if result}<PluginActionResult state={result.state} text={result.text} action={result.action} />{/if}
  </div>
{/if}

<style>
  .psf { display: grid; gap: var(--space-4); min-width: 0; max-width: 48rem; }
  .actions { display: flex; flex-wrap: wrap; gap: var(--space-2); }
  .other { display: grid; gap: var(--space-1); padding: var(--space-2) var(--space-3); border: var(--border-w) solid var(--tone-error-line); background: var(--tone-error-tint); border-radius: var(--radius-md); }
  .other-title { font-weight: var(--weight-semibold); color: var(--tone-error-ink); font-size: var(--text-md); }
  .other ul { padding-left: var(--space-5); font-size: var(--text-md); overflow-wrap: anywhere; }
</style>
