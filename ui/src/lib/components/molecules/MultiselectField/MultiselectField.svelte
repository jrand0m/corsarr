<!--
  MultiselectField (schema field: multiselect): several values from a list, as checkboxes in a fieldset.
  Shows "n of m selected" so the choice is countable at a glance. Lists longer than a screen
  belong in AsyncPicker with `multiple`.
-->
<script lang="ts">
  import Field, { type FieldStateProps } from '../Field/Field.svelte';
  import Checkbox from '../../atoms/Checkbox/Checkbox.svelte';
  import type { SelectOption } from '../../atoms/Select/Select.svelte';
  import { formatCount } from '../../../format/quantity';

  interface Props extends FieldStateProps {
    value?: string[];
    options: (SelectOption & { description?: string })[];
    columns?: 1 | 2 | 3;
    onchange?: (value: string[]) => void;
  }
  let { value = $bindable([]), options, columns = 1, onchange, ...field }: Props = $props();

  function toggle(v: string, on: boolean) {
    value = on ? [...value.filter((x) => x !== v), v] : value.filter((x) => x !== v);
    onchange?.(value);
  }
</script>

<Field {...field} group>
  {#snippet trailing()}<span class="count num">{formatCount(value.length, options.length)} selected</span>{/snippet}
  {#snippet children(c)}
    <div class="opts" style:--cols={columns} aria-invalid={c.invalid || undefined}>
      {#each options as o (o.value)}
        <Checkbox
          label={o.label} description={o.description} checked={value.includes(o.value)}
          disabled={c.disabled || c.readonly || o.disabled}
          onchange={(e) => toggle(o.value, (e.currentTarget as HTMLInputElement).checked)}
        />
      {/each}
    </div>
  {/snippet}
</Field>

<style>
  .count { color: var(--ink-muted); font-size: var(--text-sm); }
  .opts { display: grid; grid-template-columns: repeat(var(--cols), minmax(0, 1fr)); gap: var(--space-2) var(--space-4); }
  @media (width < 48rem) {
    .opts { grid-template-columns: 1fr; }
  }
  @media (width < 48rem), (pointer: coarse) {
    .opts { gap: 0; }
    .opts :global(.cb) { min-height: var(--target-min); align-items: center; }
  }
</style>
