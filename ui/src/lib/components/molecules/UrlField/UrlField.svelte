<!--
  UrlField (schema field: url): an http(s) address. Checks the shape on blur and explains the fix; the
  server validator stays the authority (its `error` wins over the local hint). `action` puts a
  control beside the input (a Test button); `result` shows a live check result under it, linked
  to the input for assistive technology.
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
  import type { Snippet } from 'svelte';
  import Field, { type FieldStateProps } from '../Field/Field.svelte';
  import TextInput from '../../atoms/TextInput/TextInput.svelte';

  interface Props extends FieldStateProps {
    value?: string;
    placeholder?: string;
    schemes?: string[];
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
    value = $bindable(''), placeholder = 'http://10.0.0.20:8080', schemes = ['http', 'https'], onchange, oninput, onpaste, onblur,
    action, result, error, ...field
  }: Props = $props();
  let local = $state<string>();
  const rid = $props.id();
</script>

<Field {...field} error={error ?? local} describedby={result ? `${rid}-result` : undefined}>
  {#snippet children(c)}
    <div class="row">
      <TextInput
        type="url" inputmode="url" mono autocomplete="off" spellcheck="false"
        id={c.id} aria-describedby={c.describedby} invalid={c.invalid} readonly={c.readonly}
        required={c.required} disabled={c.disabled} bind:value {placeholder}
        oninput={() => { if (local) local = urlProblem(value, schemes); oninput?.(value); }}
        onpaste={() => onpaste?.()}
        onblur={() => { local = urlProblem(value, schemes); onblur?.(value); }}
        onchange={() => onchange?.(value)}
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
