<!--
  NewPasswordField (S01 first run, password change): a new password with a show/hide toggle, a
  strength hint that says what would make it stronger, and a confirm input that reports a
  mismatch once the owner has typed enough to tell. Never prefilled, never sent anywhere but the
  form; `autocomplete="new-password"` lets password managers offer to generate and save one.
-->
<script lang="ts" module>
  export type PasswordStrength = { score: 0 | 1 | 2 | 3 | 4; label: string; hint?: string };

  /** Local, dependency-free estimate: length carries most weight, variety the rest. */
  export function passwordStrength(pw: string, minLength = 12): PasswordStrength {
    const len = Array.from(pw).length;
    if (!len) return { score: 0, label: 'Not set', hint: `Use at least ${minLength} characters.` };
    if (len < minLength) return { score: 1, label: 'Too short', hint: `${minLength - len} more ${minLength - len === 1 ? 'character' : 'characters'} needed.` };
    const classes = [/[a-z]/, /[A-Z]/, /\d/, /[^A-Za-z0-9]/].filter((r) => r.test(pw)).length;
    const repeats = /(.)\1{3,}/.test(pw) || new Set(pw).size < len / 3;
    let score = 2 + (len >= minLength + 4 ? 1 : 0) + (classes >= 3 ? 1 : 0) - (repeats ? 1 : 0);
    score = Math.max(2, Math.min(4, score));
    if (score === 2) return { score: 2, label: 'Fair', hint: repeats ? 'Avoid repeated characters.' : 'Longer is stronger: a few unrelated words work well.' };
    if (score === 3) return { score: 3, label: 'Good' };
    return { score: 4, label: 'Strong' };
  }
</script>

<script lang="ts">
  import Field from '../Field/Field.svelte';
  import TextInput from '../../atoms/TextInput/TextInput.svelte';
  import IconButton from '../../atoms/IconButton/IconButton.svelte';
  import Meter from '../../atoms/Meter/Meter.svelte';

  interface Props {
    label?: string;
    confirmLabel?: string;
    value?: string;
    confirm?: string;
    minLength?: number;
    /** Error from the server for the password itself ("This password was found in a breach list"). */
    error?: string;
    disabled?: boolean;
    /** Show the mismatch and length errors now (after a submit attempt). */
    showErrors?: boolean;
    onchange?: (state: { value: string; valid: boolean }) => void;
  }
  let {
    label = 'Password', confirmLabel = 'Confirm password', value = $bindable(''), confirm = $bindable(''), minLength = 12,
    error, disabled = false, showErrors = false, onchange,
  }: Props = $props();

  let revealed = $state(false);
  let confirmTouched = $state(false);
  const strength = $derived(passwordStrength(value, minLength));
  const mismatch = $derived(confirm !== '' && confirm !== value && (confirmTouched || confirm.length >= value.length));
  const lengthError = $derived(showErrors && strength.score < 2 ? (value ? `Use at least ${minLength} characters` : `${label} is required`) : undefined);
  const confirmError = $derived(mismatch ? 'The passwords do not match' : showErrors && !confirm && value ? `Type the ${label.toLowerCase()} again` : undefined);
  const valid = $derived(strength.score >= 2 && confirm === value);
  const emit = () => onchange?.({ value, valid });
  const hintId = $props.id();
</script>

<div class="npf">
  <Field {label} error={error ?? lengthError} required {disabled} describedby="{hintId}-s">
    {#snippet children(c)}
      <div class="row">
        <TextInput
          type={revealed ? 'text' : 'password'} autocomplete="new-password" spellcheck="false" minlength={minLength}
          id={c.id} aria-describedby={c.describedby} invalid={c.invalid} required disabled={c.disabled}
          bind:value oninput={emit}
        />
        <IconButton
          icon={revealed ? 'eye-off' : 'eye'} label={revealed ? 'Hide password' : 'Show password'}
          pressed={revealed} disabled={c.disabled} onclick={() => (revealed = !revealed)}
        />
      </div>
      <div class="strength" id="{hintId}-s">
        <Meter label="Password strength" value={strength.score} max={4} better="high" errorAt={0.25} warnAt={0.5} valueText={strength.label} />
        <span class="words"><span class="level">{strength.label}</span>{#if strength.hint}<span class="hint">{strength.hint}</span>{/if}</span>
      </div>
    {/snippet}
  </Field>
  <Field label={confirmLabel} error={confirmError} required {disabled}>
    {#snippet children(c)}
      <TextInput
        type={revealed ? 'text' : 'password'} autocomplete="new-password" spellcheck="false"
        id={c.id} aria-describedby={c.describedby} invalid={c.invalid} required disabled={c.disabled}
        bind:value={confirm} oninput={emit} onblur={() => (confirmTouched = true)}
      />
    {/snippet}
  </Field>
</div>

<style>
  .npf { display: grid; gap: var(--space-4); min-width: 0; }
  .row { display: flex; align-items: center; gap: var(--space-1); min-width: 0; }
  .row :global(input) { flex: 1; min-width: 0; }
  .strength { display: grid; gap: var(--space-1); margin-top: var(--space-1-5); }
  .words { display: flex; flex-wrap: wrap; gap: var(--space-1) var(--space-2); font-size: var(--text-xs); color: var(--ink-muted); }
  .level { font-weight: var(--weight-medium); color: var(--ink); }
</style>
