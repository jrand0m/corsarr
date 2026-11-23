<!-- TextField (schema field: text): a single-line string. `mono` for keys, hashes and identifiers. -->
<script lang="ts">
  import Field, { type FieldStateProps } from '../Field/Field.svelte';
  import TextInput from '../../atoms/TextInput/TextInput.svelte';

  interface Props extends FieldStateProps {
    value?: string;
    placeholder?: string;
    maxlength?: number;
    mono?: boolean;
    autocomplete?: HTMLInputElement['autocomplete'];
    onchange?: (value: string) => void;
  }
  let { value = $bindable(''), placeholder, maxlength, mono = false, autocomplete = 'off', onchange, ...field }: Props = $props();
</script>

<Field {...field}>
  {#snippet children(c)}
    <TextInput
      id={c.id} aria-describedby={c.describedby} invalid={c.invalid} readonly={c.readonly}
      required={c.required} disabled={c.disabled} bind:value {placeholder} {maxlength} {mono} {autocomplete}
      onchange={() => onchange?.(value)}
    />
  {/snippet}
</Field>
