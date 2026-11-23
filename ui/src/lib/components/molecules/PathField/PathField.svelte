<!-- PathField (schema field: path): a folder path in a schema form, with known locations via PathPicker. -->
<script lang="ts">
  import Field, { type FieldStateProps } from '../Field/Field.svelte';
  import PathPicker, { type PathLocation } from '../PathPicker/PathPicker.svelte';

  interface Props extends FieldStateProps {
    value?: string;
    locations?: PathLocation[];
    allowOther?: boolean;
    placeholder?: string;
    onchange?: (value: string) => void;
  }
  let { value = $bindable(''), locations = [], allowOther = true, placeholder, onchange, ...field }: Props = $props();
</script>

<Field {...field} group={locations.length > 0}>
  {#snippet children(c)}
    <PathPicker
      bind:value {locations} {allowOther} {placeholder} {onchange}
      id={locations.length ? undefined : c.id} describedby={locations.length ? undefined : c.describedby}
      invalid={c.invalid} readonly={c.readonly} disabled={c.disabled} required={c.required}
    />
  {/snippet}
</Field>
