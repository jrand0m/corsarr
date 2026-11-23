<!--
  SetupFlow: one generic panel for the plugin setup flows, driven only by plugin
  operations, so a plugin needs no UI code. SchemaForm renders one per `setup` entry.

    pinPairing   start() shows a code to enter on the other service; wait() settles when it is paired
    oauthDevice  start() shows a code and a URL to open; wait() settles when access is granted
    cookiePaste  submit({ cookie }) exchanges a pasted session cookie
    loginForm    submit({ username, password, otp? }) trades credentials for a session once;
                 the password is never stored

  Results land in the instance's secret fields; the panel reports which ones through `ondone`.
-->
<script lang="ts" module>
  import type { SetupFlowSpec } from '../../../forms/schema';

  export interface SetupCode { code: string; url?: string; expiresAt?: Date | string | number }
  export type SetupOutcome = { ok: true; message?: string } | { ok: false; error: string; expired?: boolean };

  /** Plugin operations behind the flows; the page wires them to /api/v1. */
  export interface SetupDriver {
    start?: (flow: SetupFlowSpec) => Promise<SetupCode>;
    /** Settles when the other side finishes (server pushes it; the UI does not poll). */
    wait?: (flow: SetupFlowSpec) => Promise<SetupOutcome>;
    submit?: (flow: SetupFlowSpec, input: Record<string, string>) => Promise<SetupOutcome>;
    cancel?: (flow: SetupFlowSpec) => void;
  }
</script>

<script lang="ts">
  import Button from '../../atoms/Button/Button.svelte';
  import CopyableValue from '../../atoms/CopyableValue/CopyableValue.svelte';
  import Countdown from '../../atoms/Countdown/Countdown.svelte';
  import Link from '../../atoms/Link/Link.svelte';
  import Spinner from '../../atoms/Spinner/Spinner.svelte';
  import StatusBadge from '../../atoms/StatusBadge/StatusBadge.svelte';
  import TextField from '../../molecules/TextField/TextField.svelte';
  import Field from '../../molecules/Field/Field.svelte';
  import Textarea from '../../atoms/Textarea/Textarea.svelte';
  import TextInput from '../../atoms/TextInput/TextInput.svelte';

  interface Props {
    flow: SetupFlowSpec;
    driver: SetupDriver;
    /** A previous run already filled the fields. */
    done?: boolean;
    disabled?: boolean;
    ondone?: (fills: string[], message?: string) => void;
    /** Fixed "now" (ms) for stories and tests. */
    now?: number;
  }
  let { flow, driver, done = false, disabled = false, ondone, now }: Props = $props();

  type Phase = 'idle' | 'starting' | 'waiting' | 'submitting' | 'done' | 'error';
  // svelte-ignore state_referenced_locally
  let phase = $state<Phase>(done ? 'done' : 'idle');
  let code = $state<SetupCode>();
  let error = $state<string>();
  let message = $state<string>();
  let cookie = $state('');
  let username = $state('');
  let password = $state('');
  let otp = $state('');
  let run = 0;
  // Another flow (or a save) filled the same fields.
  $effect(() => { if (done && phase === 'idle') phase = 'done'; });

  const codeFlow = $derived(flow.kind === 'pinPairing' || flow.kind === 'oauthDevice');
  const headingId = $props.id();

  function finish(out: SetupOutcome, token: number) {
    if (token !== run) return;
    if (out.ok) {
      phase = 'done';
      message = out.message;
      password = '';
      otp = '';
      cookie = '';
      ondone?.(flow.fills, out.message);
    } else {
      phase = 'error';
      error = out.error;
    }
  }
  function fail(e: unknown, token: number) {
    if (token !== run) return;
    phase = 'error';
    error = e instanceof Error ? e.message : String(e);
  }

  async function begin() {
    const token = ++run;
    error = undefined;
    phase = 'starting';
    try {
      code = await driver.start!(flow);
      if (token !== run) return;
      phase = 'waiting';
      finish(await driver.wait!(flow), token);
    } catch (e) { fail(e, token); }
  }
  async function submit(e: SubmitEvent) {
    e.preventDefault();
    const token = ++run;
    error = undefined;
    phase = 'submitting';
    const input: Record<string, string> = flow.kind === 'cookiePaste' ? { cookie } : { username, password, ...(flow.otp ? { otp } : {}) };
    try { finish(await driver.submit!(flow, input), token); } catch (err) { fail(err, token); }
  }
  function cancel() {
    run++;
    driver.cancel?.(flow);
    phase = 'idle';
    code = undefined;
  }
  function again() {
    phase = 'idle';
    message = undefined;
  }
  const canSubmit = $derived(flow.kind === 'cookiePaste' ? cookie.trim() !== '' : username.trim() !== '' && password !== '' && (!flow.otp || otp.trim() !== ''));
</script>

<section class="setup" aria-labelledby={headingId}>
  <div class="head">
    <h4 id={headingId}>{flow.label}</h4>
    {#if phase === 'done'}<StatusBadge label="Connected" icon="check-circle" tone="ok" size="sm" />{/if}
  </div>
  {#if flow.help && phase !== 'done'}<p class="help">{flow.help}</p>{/if}

  {#if phase === 'done'}
    <p class="outcome" role="status">{message ?? 'Done. The credentials are saved with this connection.'}</p>
    <div class="actions"><Button size="sm" icon="refresh" onclick={again} {disabled}>Set up again</Button></div>
  {:else if codeFlow}
    {#if phase === 'waiting' && code}
      <div class="code">
        {#if flow.kind === 'oauthDevice' && code.url}
          <p>Open <Link href={code.url} external>{code.url}</Link> and enter this code:</p>
        {:else if code.url}
          <p>Enter this code at <Link href={code.url} external>{code.url}</Link>:</p>
        {:else}
          <p>Enter this code on the other service:</p>
        {/if}
        <div class="code-value"><CopyableValue value={code.code} label="Pairing code" /></div>
        <p class="wait">
          <Spinner size="sm" /><span>Waiting for confirmation</span>
          {#if code.expiresAt}<Countdown to={code.expiresAt} action="Code expires" expiredLabel="Code expired" {now} />{/if}
        </p>
      </div>
      <div class="actions"><Button size="sm" onclick={cancel}>Cancel</Button></div>
    {:else}
      <div class="actions">
        <Button variant="primary" size="sm" icon="link" loading={phase === 'starting'} {disabled} onclick={begin}>
          {phase === 'error' ? 'Get a new code' : 'Get a code'}
        </Button>
      </div>
    {/if}
  {:else}
    <form class="form" onsubmit={submit} novalidate>
      {#if flow.kind === 'cookiePaste'}
        <Field label="Session cookie" help="Copy it from your browser's developer tools while signed in to the site. It is stored as a secret.">
          {#snippet children(c)}
            <Textarea id={c.id} aria-describedby={c.describedby} bind:value={cookie} mono rows={3} spellcheck={false} disabled={disabled || phase === 'submitting'} />
          {/snippet}
        </Field>
      {:else}
        <TextField label="Username" bind:value={username} autocomplete="username" disabled={disabled || phase === 'submitting'} />
        <Field label="Password" help="Used once to get a session. corsarr does not store it.">
          {#snippet children(c)}
            <TextInput id={c.id} aria-describedby={c.describedby} type="password" bind:value={password} autocomplete="current-password" disabled={disabled || phase === 'submitting'} />
          {/snippet}
        </Field>
        {#if flow.otp}<TextField label="One-time code" bind:value={otp} autocomplete="one-time-code" disabled={disabled || phase === 'submitting'} />{/if}
      {/if}
      <div class="actions">
        <Button type="submit" variant="primary" size="sm" loading={phase === 'submitting'} disabled={disabled || !canSubmit}>
          {flow.kind === 'cookiePaste' ? 'Save cookie' : 'Sign in'}
        </Button>
      </div>
    </form>
  {/if}

  {#if phase === 'error' && error}
    <p class="error" role="alert">{error}</p>
  {/if}
</section>

<style>
  .setup {
    display: grid; gap: var(--space-3); padding: var(--space-4);
    border: var(--border-w) solid var(--line); border-radius: var(--radius-lg); background: var(--surface-raised);
    min-width: 0;
  }
  .head { display: flex; align-items: center; gap: var(--space-2); flex-wrap: wrap; }
  h4 { font-size: var(--text-md); font-weight: var(--weight-semibold); }
  p { font-size: var(--text-md); max-width: var(--measure); }
  .help { color: var(--ink-muted); }
  .code { display: grid; gap: var(--space-2); }
  .code-value { font-size: var(--text-xl); letter-spacing: 0.08em; }
  .wait { display: flex; align-items: center; gap: var(--space-1-5); color: var(--ink-muted); font-size: var(--text-sm); flex-wrap: wrap; }
  .form { display: grid; gap: var(--space-3); max-width: 28rem; }
  .actions { display: flex; gap: var(--space-2); flex-wrap: wrap; }
  .error { color: var(--tone-error-ink); font-weight: var(--weight-medium); font-size: var(--text-sm); }
  .outcome { color: var(--ink-muted); }
</style>
