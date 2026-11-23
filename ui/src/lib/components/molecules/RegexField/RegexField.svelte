<!--
  RegexField (schema field: regex): a pattern with a live test. The owner pastes a sample release title
  and sees at once whether it matches and which part. Matching runs in the browser with
  JavaScript regular expressions, which accept a superset of the server's RE2 syntax for
  common patterns; the server validator stays the authority (pass its message as `error`).
-->
<script lang="ts" module>
  export type RegexTest =
    | { kind: 'empty' }
    | { kind: 'invalid'; message: string }
    | { kind: 'match'; before: string; match: string; after: string }
    | { kind: 'no-match' };

  export function testRegex(pattern: string, sample: string, ignoreCase: boolean): RegexTest {
    if (!pattern) return { kind: 'empty' };
    let re: RegExp;
    try { re = new RegExp(pattern, ignoreCase ? 'i' : ''); } catch (e) {
      return { kind: 'invalid', message: (e as Error).message.replace(/^Invalid regular expression: /, '') };
    }
    if (!sample) return { kind: 'empty' };
    const m = re.exec(sample);
    if (!m) return { kind: 'no-match' };
    return { kind: 'match', before: sample.slice(0, m.index), match: m[0], after: sample.slice(m.index + m[0].length) };
  }
</script>

<script lang="ts">
  import Field, { type FieldStateProps } from '../Field/Field.svelte';
  import TextInput from '../../atoms/TextInput/TextInput.svelte';
  import Checkbox from '../../atoms/Checkbox/Checkbox.svelte';
  import Icon from '../../atoms/Icon/Icon.svelte';

  interface Props extends FieldStateProps {
    value?: string;
    /** Sample text to test against; bindable so a form can remember it. */
    sample?: string;
    ignoreCase?: boolean;
    placeholder?: string;
    onchange?: (value: string) => void;
  }
  let {
    value = $bindable(''), sample = $bindable(''), ignoreCase = $bindable(true),
    placeholder = '\\b(720p|1080p)\\b', onchange, error, ...field
  }: Props = $props();

  const tid = $props.id();
  const result = $derived(testRegex(value, sample, ignoreCase));
  const syntax = $derived(result.kind === 'invalid' ? `This pattern does not compile: ${result.message}` : undefined);
</script>

<Field {...field} error={error ?? syntax} describedby="{tid}-res">
  {#snippet children(c)}
    <div class="rx">
      <TextInput
        id={c.id} aria-describedby={c.describedby} invalid={c.invalid} readonly={c.readonly}
        required={c.required} disabled={c.disabled} bind:value mono spellcheck="false" autocomplete="off"
        {placeholder} onchange={() => onchange?.(value)}
      />
      <div class="test">
        <div class="sample">
          <TextInput aria-label="Test text for {field.label}" bind:value={sample} size="sm" placeholder="Paste a release title to test" spellcheck="false" />
          <Checkbox label="Ignore case" bind:checked={ignoreCase} disabled={c.disabled} />
        </div>
        <p class={['res', result.kind]} id="{tid}-res">
          {#if result.kind === 'match'}
            <Icon name="check-circle" size="sm" /><span><span class="lead">Matches:</span> <code>{result.before}<mark>{result.match}</mark>{result.after}</code></span>
          {:else if result.kind === 'no-match'}
            <Icon name="minus-circle" size="sm" /><span>No match in this text</span>
          {:else if result.kind === 'invalid'}
            <Icon name="x-circle" size="sm" /><span>Fix the pattern to test it</span>
          {:else}
            <Icon name="info" size="sm" /><span>{value ? 'Paste a title above to see what matches' : 'Enter a pattern, then paste a title to test it'}</span>
          {/if}
        </p>
      </div>
    </div>
  {/snippet}
</Field>

<style>
  .rx { display: grid; gap: var(--space-2); }
  .test { display: grid; gap: var(--space-1-5); padding: var(--space-2) var(--space-3); background: var(--surface-sunken); border-radius: var(--radius-md); }
  .sample { display: flex; flex-wrap: wrap; align-items: center; gap: var(--space-2) var(--space-3); }
  .sample > :global(.ti) { flex: 1 1 14rem; }
  .res { display: flex; gap: var(--space-1-5); align-items: flex-start; font-size: var(--text-sm); color: var(--ink-muted); max-width: none; }
  .res :global(svg) { margin-top: 0.125rem; }
  .match :global(svg) { color: var(--tone-ok-ink); }
  .no-match :global(svg) { color: var(--ink-subtle); }
  .invalid :global(svg) { color: var(--tone-error-ink); }
  .lead { color: var(--ink); font-weight: var(--weight-medium); }
  code { font-size: var(--text-sm); overflow-wrap: anywhere; color: var(--ink); }
  mark { background: var(--tone-ok-tint); color: var(--tone-ok-ink); outline: var(--border-w) solid var(--tone-ok-line); border-radius: var(--radius-xs); }
</style>
