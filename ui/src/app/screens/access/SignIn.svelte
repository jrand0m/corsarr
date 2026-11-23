<!-- Sign-in: user name and password; wrong-password and session-ended states. -->
<script lang="ts">
  import AccessPage from '../../../lib/components/templates/AccessPage/AccessPage.svelte';
  import Button from '../../../lib/components/atoms/Button/Button.svelte';
  import TextInput from '../../../lib/components/atoms/TextInput/TextInput.svelte';
  import Banner from '../../../lib/components/molecules/Banner/Banner.svelte';
  import Field from '../../../lib/components/molecules/Field/Field.svelte';
  import { app } from '../../context.svelte';
  import { toApiError } from '../../api/client';
  import type { ScreenProps } from '../../screen';

  let { expired = false, onsignedin }: ScreenProps = $props();
  const { api } = app();

  let username = $state('admin');
  let password = $state('');
  let busy = $state(false);
  let wrong = $state(false);
  let error = $state<string | undefined>();

  async function submit(e: SubmitEvent) {
    e.preventDefault();
    busy = true;
    wrong = false;
    error = undefined;
    try {
      await api.post('auth/login', { username, password });
      onsignedin?.();
    } catch (err) {
      const x = toApiError(err);
      if (x.code === 'invalid_credentials') wrong = true;
      else error = x.status === 429 ? 'Too many attempts. Wait a minute, then try again.' : x.message;
    } finally {
      busy = false;
    }
  }
</script>

<AccessPage title="Sign in to corsarr">
  <form class="form" onsubmit={submit}>
    {#if expired}<Banner tone="info" icon="clock" title="Your session ended">Sign in again to pick up where you left off.</Banner>{/if}
    {#if error}<Banner tone="error" icon="alert-octagon" title="Couldn't sign in">{error}</Banner>{/if}
    <Field label="User name" disabled={busy}>
      {#snippet children(c)}
        <TextInput autocomplete="username" id={c.id} aria-describedby={c.describedby} disabled={c.disabled} bind:value={username} required />
      {/snippet}
    </Field>
    <Field label="Password" error={wrong ? 'That user name or password is not right. Check caps lock and try again.' : undefined} disabled={busy}>
      {#snippet children(c)}
        <TextInput type="password" autocomplete="current-password" id={c.id} aria-describedby={c.describedby} invalid={c.invalid} disabled={c.disabled} bind:value={password} required />
      {/snippet}
    </Field>
    <Button type="submit" variant="primary" loading={busy} disabled={busy || !password}>Sign in</Button>
  </form>
  {#snippet footer()}
    <p>The user name set on first run is <code>admin</code>. Scripts and bots use the API key instead of a password.</p>
  {/snippet}
</AccessPage>

<style>
  .form { display: grid; gap: var(--space-4); }
  code { font-family: var(--font-mono); font-size: var(--text-sm); color: var(--ink); }
</style>
