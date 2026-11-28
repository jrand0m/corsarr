<!--
  ConditionRow: one condition of a custom format. The kind decides the input: a pattern (release
  title, group, edition), values from a fixed list (source, resolution, codec, audio, HDR, …) or a
  range (size in GB, year). Every condition can be turned around (negate) or made required;
  language conditions can also mean "any language but these". An optional `matched` shows the
  result for the title in the test box.
-->
<script lang="ts" module>
  export interface ConditionValue {
    kind: string; pattern?: string; values?: string[]; min?: number; max?: number;
    negate?: boolean; required?: boolean; except?: boolean;
  }
  export interface ConditionKindOption {
    kind: string; label: string; input: 'pattern' | 'values' | 'range'; unit?: string;
    values?: { value: string; label: string }[];
  }

  /** A condition switched to another kind keeps only the flags. */
  export function switchKind(c: ConditionValue, kind: string): ConditionValue {
    return { kind, negate: c.negate, required: c.required };
  }
</script>

<script lang="ts">
  import Checkbox from '../../atoms/Checkbox/Checkbox.svelte';
  import IconButton from '../../atoms/IconButton/IconButton.svelte';
  import StatusBadge from '../../atoms/StatusBadge/StatusBadge.svelte';
  import EnumField from '../../molecules/EnumField/EnumField.svelte';
  import MultiselectField from '../../molecules/MultiselectField/MultiselectField.svelte';
  import NumberField from '../../molecules/NumberField/NumberField.svelte';
  import TextField from '../../molecules/TextField/TextField.svelte';

  interface Props {
    value: ConditionValue;
    kinds: ConditionKindOption[];
    /** 1-based position, for labels: "Condition 2". */
    position: number;
    /** Field messages from the server, keyed by "kind", "pattern", "values", "min", "max", "except". */
    errors?: Record<string, string>;
    readonly?: boolean;
    /** The result for the test title; undefined while nothing is tested. */
    matched?: boolean;
    onchange?: (value: ConditionValue) => void;
    onremove?: () => void;
  }
  let { value, kinds, position, errors = {}, readonly = false, matched, onchange, onremove }: Props = $props();

  const info = $derived(kinds.find((k) => k.kind === value.kind));
  const label = $derived(`Condition ${position}`);
  const set = (patch: Partial<ConditionValue>) => onchange?.({ ...value, ...patch });
</script>

<fieldset class="row" aria-label={label}>
  <div class="head">
    <div class="kind">
      <EnumField label="{label}: looks at" value={value.kind} options={kinds.map((k) => ({ value: k.kind, label: k.label }))}
        error={errors.kind} disabled={readonly} onchange={(k) => onchange?.(switchKind(value, k))} />
    </div>
    {#if matched !== undefined}
      <StatusBadge label={matched ? 'Matches' : 'No match'} icon={matched ? 'check-circle' : 'x-circle'} tone={matched ? 'ok' : 'neutral'} size="sm" appearance="plain" />
    {/if}
    {#if onremove && !readonly}<IconButton icon="trash" label="Remove {label.toLowerCase()}" onclick={onremove} />{/if}
  </div>
  {#if info?.input === 'pattern'}
    <TextField label="Pattern" mono value={value.pattern ?? ''} placeholder={'\\bDTS\\b'} error={errors.pattern} disabled={readonly}
      help="A regular expression; letter case does not matter." onchange={(v) => set({ pattern: v })} oninput={(v) => set({ pattern: v })} />
  {:else if info?.input === 'values'}
    <MultiselectField label="Any of" value={value.values ?? []} options={info.values ?? []} columns={3} error={errors.values} disabled={readonly}
      onchange={(v) => set({ values: v })} />
  {:else if info?.input === 'range'}
    <div class="range">
      <NumberField label="From" value={value.min ?? null} min={0} step={info.unit ? 0.1 : 1} unit={info.unit} error={errors.min} disabled={readonly}
        onchange={(v) => set({ min: v ?? 0 })} />
      <NumberField label="To" value={value.max ?? null} min={0} step={info.unit ? 0.1 : 1} unit={info.unit} error={errors.max} disabled={readonly}
        help="Empty or 0: no upper limit." onchange={(v) => set({ max: v ?? 0 })} />
    </div>
  {/if}
  <div class="flags">
    <Checkbox label="Negate" description="Holds when the test fails." checked={value.negate ?? false} disabled={readonly}
      onchange={(e) => set({ negate: (e.currentTarget as HTMLInputElement).checked })} />
    <Checkbox label="Required" description="The format matches only when this holds." checked={value.required ?? false} disabled={readonly}
      onchange={(e) => set({ required: (e.currentTarget as HTMLInputElement).checked })} />
    {#if value.kind === 'language'}
      <Checkbox label="Any language but these" checked={value.except ?? false} disabled={readonly}
        onchange={(e) => set({ except: (e.currentTarget as HTMLInputElement).checked })} />
    {/if}
  </div>
</fieldset>

<style>
  .row { display: grid; gap: var(--space-3); min-width: 0; border: var(--border-w, 1px) solid var(--line); border-radius: var(--radius-md);
    padding: var(--space-3); margin: 0; }
  .head { display: flex; align-items: flex-end; gap: var(--space-2); min-width: 0; }
  .kind { flex: 1; min-width: 0; }
  .range { display: grid; grid-template-columns: repeat(auto-fit, minmax(10rem, 1fr)); gap: var(--space-3); }
  .flags { display: flex; flex-wrap: wrap; gap: var(--space-3) var(--space-5); }
</style>
