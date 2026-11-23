<!--
  BoolField (schema field: bool): a checkbox, because schema forms save later (Switch is for settings
  that apply at once). The checkbox carries the label; help, lock and drift sit below.
-->
<script lang="ts">
  import Field, { type FieldStateProps } from '../Field/Field.svelte';
  import Checkbox from '../../atoms/Checkbox/Checkbox.svelte';

  interface Props extends FieldStateProps {
    checked?: boolean;
    onchange?: (checked: boolean) => void;
  }
  let { checked = $bindable(false), onchange, ...field }: Props = $props();
</script>

<Field {...field} labelled="control">
  {#snippet children(c)}
    <Checkbox
      label={field.label} bind:checked aria-describedby={c.describedby} aria-invalid={c.invalid || undefined}
      disabled={c.disabled || c.readonly} aria-readonly={c.readonly || undefined}
      onchange={() => onchange?.(checked)}
    />
  {/snippet}
</Field>
