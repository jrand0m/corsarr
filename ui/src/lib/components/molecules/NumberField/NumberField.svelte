<!-- NumberField (schema field: number): an integer or decimal with optional bounds and unit. -->
<script lang="ts">
  import Field, { type FieldStateProps } from '../Field/Field.svelte';
  import NumberInput from '../../atoms/NumberInput/NumberInput.svelte';

  interface Props extends FieldStateProps {
    value?: number | null;
    min?: number;
    max?: number;
    step?: number;
    unit?: string;
    onchange?: (value: number | null) => void;
  }
  let { value = $bindable(null), min, max, step = 1, unit, onchange, ...field }: Props = $props();
</script>

<Field {...field}>
  {#snippet children(c)}
    <span class="w">
      <NumberInput
        id={c.id} aria-describedby={c.describedby} invalid={c.invalid} readonly={c.readonly}
        required={c.required} disabled={c.disabled} bind:value {min} {max} {step} {unit}
        onvaluechange={(v) => onchange?.(v)}
      />
    </span>
  {/snippet}
</Field>

<style>
  .w { display: block; max-width: 14rem; }
</style>
