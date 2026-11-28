<!--
  Field: the shared wrapper of every SchemaForm field. It owns the label, the required
  mark, help, the inline error, the "Managed by config" lock (violet seal, read-only control,
  source key) and the drift marker, and hands the control what it needs to wire itself up:

    <Field label="Host" help="…" error={err}>
      {#snippet children(c)}<TextInput id={c.id} aria-describedby={c.describedby} invalid={c.invalid} readonly={c.readonly} required={c.required} />{/snippet}
    </Field>

  `group` renders a fieldset + legend for controls made of several inputs (multiselect, list,
  map). `labelled="control"` is for a checkbox that carries its own label.
-->
<script lang="ts" module>
  import type { ManagedSource } from '../../atoms/ManagedValue/ManagedValue.svelte';

  export interface FieldControl {
    id: string;
    describedby: string | undefined;
    invalid: boolean;
    readonly: boolean;
    required: boolean;
    disabled: boolean;
  }

  /** Props every field type shares; each field type extends this. */
  export interface FieldStateProps {
    label: string;
    help?: string;
    error?: string;
    required?: boolean;
    disabled?: boolean;
    /**
     * Managed by config: `true`, the key in the config ("indexers.nzbgeek.api_key"), or the key with
     * the source that sets it (file path, argument or environment variable).
     */
    managed?: boolean | string | ManagedSource;
    /** Drift marker: what differs between the config file and the running value. */
    drift?: string;
  }
</script>

<script lang="ts">
  import type { Snippet } from 'svelte';
  import Icon from '../../atoms/Icon/Icon.svelte';
  import StatusBadge from '../../atoms/StatusBadge/StatusBadge.svelte';
  import ManagedValue, { managedText } from '../../atoms/ManagedValue/ManagedValue.svelte';

  interface Props extends FieldStateProps {
    group?: boolean;
    labelled?: 'field' | 'control';
    /** Extra id to add to aria-describedby (a live example, a test result). */
    describedby?: string;
    /** Content after the label (a "Test" link, a unit hint). */
    trailing?: Snippet;
    children: Snippet<[FieldControl]>;
    class?: string;
  }
  let {
    label, help, error, required = false, disabled = false, managed, drift, group = false,
    labelled = 'field', describedby: extra, trailing, children, class: cls,
  }: Props = $props();

  const id = $props.id();
  const source = $derived(typeof managed === 'string' ? managed : undefined);
  const by = $derived<ManagedSource | undefined>(
    typeof managed === 'object' ? managed : managed ? { key: source } : undefined,
  );
  const describedby = $derived(
    [error && `${id}-err`, managed && `${id}-lock`, drift && `${id}-drift`, help && `${id}-help`, extra]
      .filter(Boolean).join(' ') || undefined,
  );
  const control = $derived<FieldControl>({
    id: `${id}-c`, describedby, invalid: !!error, readonly: !!managed, required, disabled,
  });
</script>

{#snippet badges()}
  {#if required && !managed}<span class="req">Required</span>{/if}
  {#if by}<ManagedValue managed={by} />{/if}
  {#if drift}<StatusBadge family="config" value="drifted" size="sm" />{/if}
{/snippet}

{#snippet notes()}
  {#if error}<p class="error" id="{id}-err"><Icon name="alert-octagon" size="sm" /><span>{error}</span></p>{/if}
  {#if managed}
    <p class="note" id="{id}-lock">
      {#if typeof managed === 'object'}{managedText(managed)}.{:else}Set in the config file{#if source}{' at '}<code>{source}</code>{/if}.{/if} Change it there; the form shows what is running.
    </p>
  {/if}
  {#if drift}<p class="note drift" id="{id}-drift"><Icon name="diff" size="sm" /><span>{drift}</span></p>{/if}
  {#if help}<p class="help" id="{id}-help">{help}</p>{/if}
{/snippet}

{#if group}
  <fieldset class={['field', cls]} aria-describedby={describedby} {disabled}>
    <legend class="head"><span class="label">{label}</span>{@render badges()}</legend>
    {#if trailing}<div class="trailing">{@render trailing()}</div>{/if}
    <div class="control">{@render children(control)}</div>
    {@render notes()}
  </fieldset>
{:else}
  <div class={['field', cls]} class:check={labelled === 'control'}>
    {#if labelled === 'field'}
      <div class="head">
        <label class="label" for="{id}-c">{label}</label>
        {@render badges()}
        {#if trailing}<span class="trailing">{@render trailing()}</span>{/if}
      </div>
      <div class="control">{@render children(control)}</div>
    {:else}
      <div class="head">
        <div class="control">{@render children(control)}</div>
        {@render badges()}
      </div>
    {/if}
    {@render notes()}
  </div>
{/if}

<style>
  .field { display: grid; gap: var(--space-1-5); width: 100%; min-width: 0; padding: 0; border: 0; margin: 0; }
  .head { display: flex; flex-wrap: wrap; align-items: center; gap: var(--space-1-5) var(--space-2); padding: 0; min-width: 0; }
  .label { font-size: var(--text-md); font-weight: var(--weight-medium); color: var(--ink); line-height: var(--leading-snug); }
  .req { font-size: var(--text-xs); color: var(--ink-subtle); }
  .trailing { margin-left: auto; display: inline-flex; gap: var(--space-2); font-size: var(--text-sm); }
  .control { min-width: 0; }
  .check .control { flex: 0 1 auto; }
  p { max-width: var(--measure); font-size: var(--text-sm); line-height: var(--leading-snug); }
  .help { color: var(--ink-muted); }
  .error, .drift { display: flex; gap: var(--space-1); align-items: flex-start; }
  .error { color: var(--tone-error-ink); font-weight: var(--weight-medium); }
  .error :global(svg), .drift :global(svg) { margin-top: 0.125rem; }
  .note { color: var(--tone-managed-ink); }
  .drift { color: var(--tone-warn-ink); }
  code { font-size: var(--text-xs); overflow-wrap: anywhere; }
</style>
