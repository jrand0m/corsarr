<!--
  TextField (schema field: text): a single-line string. `mono` for keys, hashes and identifiers.
  `action` puts a control beside the input (a Test button); `result` shows a live check result under it.
-->
<script lang="ts">
  import type { Snippet } from 'svelte';
  import Field, { type FieldStateProps } from '../Field/Field.svelte';
  import TextInput from '../../atoms/TextInput/TextInput.svelte';

  interface Props extends FieldStateProps {
    value?: string;
    placeholder?: string;
    maxlength?: number;
    mono?: boolean;
    autocomplete?: HTMLInputElement['autocomplete'];
    onchange?: (value: string) => void;
    /** Every keystroke or paste, after `value` updates. */
    oninput?: (value: string) => void;
    onpaste?: () => void;
    onblur?: (value: string) => void;
    /** A control beside the input, such as a Test button. */
    action?: Snippet;
    /** A live result under the input (a check outcome); announced politely. */
    result?: Snippet;
  }
  let {
    value = $bindable(''), placeholder, maxlength, mono = false, autocomplete = 'off', onchange, oninput, onpaste, onblur, action, result, ...field
  }: Props = $props();
  const rid = $props.id();
</script>

<Field {...field} describedby={result ? `${rid}-result` : undefined}>
  {#snippet children(c)}
    <div class="row">
      <TextInput
        id={c.id} aria-describedby={c.describedby} invalid={c.invalid} readonly={c.readonly}
        required={c.required} disabled={c.disabled} bind:value {placeholder} {maxlength} {mono} {autocomplete}
        onchange={() => onchange?.(value)}
        oninput={() => oninput?.(value)} onpaste={() => onpaste?.()} onblur={() => onblur?.(value)}
      />
      {#if action}{@render action()}{/if}
    </div>
    {#if result}<div class="result" id="{rid}-result" aria-live="polite">{@render result()}</div>{/if}
  {/snippet}
</Field>

<style>
  .row { display: flex; align-items: center; gap: var(--space-1); min-width: 0; }
  .result { margin-top: var(--space-1-5); min-width: 0; }
</style>
