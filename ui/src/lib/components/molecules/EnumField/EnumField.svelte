<!--
  EnumField (schema field: enum/select): one value from a fixed or dynamic list. Short lists (≤ 4) can
  show as radios with `appearance="radio"`; long or searchable lists belong in AsyncPicker.
-->
<script lang="ts">
  import Field, { type FieldStateProps } from '../Field/Field.svelte';
  import Select, { type SelectOption } from '../../atoms/Select/Select.svelte';
  import Radio from '../../atoms/Radio/Radio.svelte';

  interface Props extends FieldStateProps {
    value?: string;
    options: (SelectOption & { description?: string })[];
    placeholder?: string;
    appearance?: 'select' | 'radio';
    onchange?: (value: string) => void;
  }
  let { value = $bindable(''), options, placeholder, appearance = 'select', onchange, ...field }: Props = $props();
</script>

{#if appearance === 'radio'}
  <Field {...field} group>
    {#snippet children(c)}
      <div class="radios" role="radiogroup" aria-required={c.required || undefined} aria-invalid={c.invalid || undefined}>
        {#each options as o (o.value)}
          <Radio
            bind:group={value} value={o.value} label={o.label} description={o.description} name={c.id}
            disabled={c.disabled || o.disabled || (c.readonly && value !== o.value)}
            onchange={() => onchange?.(value)}
          />
        {/each}
      </div>
    {/snippet}
  </Field>
{:else}
  <Field {...field}>
    {#snippet children(c)}
      <span class="w"><Select
        id={c.id} aria-describedby={c.describedby} invalid={c.invalid} required={c.required}
        disabled={c.disabled || c.readonly} bind:value {options} {placeholder}
        onchange={() => onchange?.(value)}
      /></span>
    {/snippet}
  </Field>
{/if}

<style>
  .radios { display: grid; gap: var(--space-2); }
  .w { display: grid; max-width: 24rem; }
</style>
