<!--
  DurationField (schema field: duration): a span of time stored in seconds, edited as an amount and a unit (s, m, h, d), with the compact reading ("1h 30m") beside it.
-->
<script lang="ts">
  import { untrack } from 'svelte';
  import Field, { type FieldStateProps } from '../Field/Field.svelte';
  import NumberInput from '../../atoms/NumberInput/NumberInput.svelte';
  import Select from '../../atoms/Select/Select.svelte';
  import { durationUnits, splitUnit, joinUnit, type DurationUnit } from '../../../format/units';
  import { formatDuration } from '../../../format/quantity';

  interface Props extends FieldStateProps {
    /** Stored value in seconds; null when empty. */
    value?: number | null;
    /** Units offered; defaults to all. */
    units?: DurationUnit[];
    /** Unit used for an empty or zero value. */
    unit?: DurationUnit;
    min?: number;
    max?: number;
    onchange?: (value: number | null) => void;
  }
  let { value = $bindable(null), units, unit = 'm', min, max, onchange, ...field }: Props = $props();

  const pick = (only?: DurationUnit[]) => durationUnits.filter((u) => !only || only.includes(u.unit));
  const defs = $derived(pick(units));
  // Initial split only; the effect below keeps amount/unit in step with outside changes.
  // svelte-ignore state_referenced_locally
  const initial = splitUnit(value, pick(units), unit);
  let amount = $state<number | null>(initial.amount);
  let chosen = $state<DurationUnit>(initial.unit);

  // Follow outside changes to `value` only; typing changes `amount` until it is committed.
  // svelte-ignore state_referenced_locally
  let synced = value;
  $effect(() => {
    const v = value;
    if (v === synced) return;
    synced = v;
    const s = splitUnit(v, untrack(() => defs), untrack(() => chosen));
    amount = s.amount;
    chosen = s.unit;
  });

  function commit() {
    value = joinUnit(amount, chosen, defs);
    synced = value;
    onchange?.(value);
  }
  const bounds = $derived.by(() => {
    const f = defs.find((u) => u.unit === chosen)?.factor ?? 1;
    return { min: min === undefined ? 0 : min / f, max: max === undefined ? undefined : max / f };
  });
</script>

<Field {...field}>
  {#snippet children(c)}
    <span class="row">
      <span class="amt">
        <NumberInput
          id={c.id} aria-describedby={c.describedby} invalid={c.invalid} readonly={c.readonly}
          required={c.required} disabled={c.disabled} bind:value={amount} min={bounds.min} max={bounds.max}
          step={0.1} steppers={false} onvaluechange={commit}
        />
      </span>
      <Select
        aria-label="{field.label}: unit" bind:value={chosen} disabled={c.disabled || c.readonly}
        options={defs.map((u) => ({ value: u.unit, label: u.label }))} onchange={commit}
      />
      {#if value !== null && value > 0}<output class="read num" for={c.id}>{formatDuration(value)}</output>{/if}
    </span>
  {/snippet}
</Field>

<style>
  .row { display: flex; flex-wrap: wrap; align-items: center; gap: var(--space-2); }
  .amt { width: 8rem; }
  .read { font-size: var(--text-sm); color: var(--ink-muted); }
</style>
