<!--
  UrlField (schema field: url): an http(s) address. Checks the shape on blur and explains the fix; the
  server validator stays the authority (its `error` wins over the local hint).
-->
<script lang="ts" module>
  /** A local shape check: absolute http(s) URL with a host. Returns a message or undefined. */
  export function urlProblem(value: string, schemes: readonly string[] = ['http', 'https']): string | undefined {
    if (!value.trim()) return undefined;
    let u: URL;
    try { u = new URL(value.trim()); } catch { return `Enter a full address with its scheme, like ${schemes[0]}://10.0.0.20:8080`; }
    if (!schemes.includes(u.protocol.replace(/:$/, ''))) return `Use ${schemes.map((s) => `${s}://`).join(' or ')}; ${u.protocol}// is not supported here`;
    if (!u.hostname) return 'The address has no host name';
    return undefined;
  }
</script>

<script lang="ts">
  import Field, { type FieldStateProps } from '../Field/Field.svelte';
  import TextInput from '../../atoms/TextInput/TextInput.svelte';

  interface Props extends FieldStateProps {
    value?: string;
    placeholder?: string;
    schemes?: string[];
    onchange?: (value: string) => void;
  }
  let { value = $bindable(''), placeholder = 'http://10.0.0.20:8080', schemes = ['http', 'https'], onchange, error, ...field }: Props = $props();
  let local = $state<string>();
</script>

<Field {...field} error={error ?? local}>
  {#snippet children(c)}
    <TextInput
      type="url" inputmode="url" mono autocomplete="off" spellcheck="false"
      id={c.id} aria-describedby={c.describedby} invalid={c.invalid} readonly={c.readonly}
      required={c.required} disabled={c.disabled} bind:value {placeholder}
      oninput={() => { if (local) local = urlProblem(value, schemes); }}
      onblur={() => (local = urlProblem(value, schemes))}
      onchange={() => onchange?.(value)}
    />
  {/snippet}
</Field>
