<!--
  SecretField (schema field: secret): write-only. The server never sends the stored value, only whether
  one is set (`isSet`) or which reference supplies it (`ref`, "${env:NZBGEEK_API_KEY}" or
  "${file:/run/secrets/qbit}"). Typing sets a new value (`value`); the input is masked with a
  reveal toggle for what was just typed. A reference shows instead of the input until the
  owner chooses to replace it.
-->
<script lang="ts" module>
  /** Parse "${env:NAME}" / "${file:/path}" into its kind and target. */
  export function parseSecretRef(ref: string): { kind: 'env' | 'file'; target: string } | undefined {
    const m = /^\$\{(env|file):([^}]+)\}$/.exec(ref.trim());
    return m ? { kind: m[1] as 'env' | 'file', target: m[2] } : undefined;
  }
</script>

<script lang="ts">
  import Field, { type FieldStateProps } from '../Field/Field.svelte';
  import TextInput from '../../atoms/TextInput/TextInput.svelte';
  import IconButton from '../../atoms/IconButton/IconButton.svelte';
  import Button from '../../atoms/Button/Button.svelte';
  import StatusBadge from '../../atoms/StatusBadge/StatusBadge.svelte';

  interface Props extends FieldStateProps {
    /** The new value being typed; empty means "keep what is stored". */
    value?: string;
    isSet?: boolean;
    ref?: string;
    onchange?: (value: string) => void;
  }
  let { value = $bindable(''), isSet = false, ref, onchange, ...field }: Props = $props();

  let revealed = $state(false);
  let replacing = $state(false);
  const parsed = $derived(ref ? parseSecretRef(ref) : undefined);
  const refText = $derived(parsed ? `${parsed.kind === 'env' ? 'env' : 'file'}: ${parsed.target}` : (ref ?? ''));
  const showRef = $derived(!!ref && !replacing);
  const status = $derived(ref ? 'ref' : isSet || value ? 'set' : 'unset');
</script>

<Field {...field}>
  {#snippet trailing()}<StatusBadge family="secret" value={status} size="sm" />{/snippet}
  {#snippet children(c)}
    {#if showRef}
      <div class="ref">
        <TextInput
          readonly mono id={c.id} aria-describedby={c.describedby} disabled={c.disabled}
          icon={parsed?.kind === 'file' ? 'file' : 'key'} value={refText}
        />
        {#if !c.readonly && !c.disabled}
          <Button size="sm" variant="ghost" onclick={() => (replacing = true)}>Use a value instead</Button>
        {/if}
      </div>
    {:else}
      <div class="row">
        <TextInput
          type={revealed ? 'text' : 'password'} mono autocomplete="new-password" spellcheck="false"
          id={c.id} aria-describedby={c.describedby} invalid={c.invalid} readonly={c.readonly}
          required={c.required && !isSet} disabled={c.disabled} bind:value
          placeholder={isSet ? 'Stored; type to replace' : 'Not set'}
          onchange={() => onchange?.(value)}
        />
        <IconButton
          icon={revealed ? 'eye-off' : 'eye'} label={revealed ? 'Hide what you typed' : 'Show what you typed'}
          pressed={revealed} disabled={!value || c.disabled} onclick={() => (revealed = !revealed)}
        />
        {#if ref && replacing}
          <Button size="sm" variant="ghost" onclick={() => { replacing = false; value = ''; }}>Keep reference</Button>
        {/if}
      </div>
    {/if}
  {/snippet}
</Field>

<style>
  .row, .ref { display: flex; align-items: center; gap: var(--space-1); min-width: 0; }
  @media (width < 48rem), (pointer: coarse) {
    .ref { flex-wrap: wrap; }
  }
</style>
