<!-- Page composition (story only): S02 sign-in, with wrong-password, wait and reverse-proxy states. -->
<script lang="ts">
  import AccessPage from '../lib/components/templates/AccessPage/AccessPage.svelte';
  import Button from '../lib/components/atoms/Button/Button.svelte';
  import Countdown from '../lib/components/atoms/Countdown/Countdown.svelte';
  import TextInput from '../lib/components/atoms/TextInput/TextInput.svelte';
  import Banner from '../lib/components/molecules/Banner/Banner.svelte';
  import Field from '../lib/components/molecules/Field/Field.svelte';
  import { NOW } from '../lib/storybook/fixtures';

  interface Props { state?: 'idle' | 'wrong' | 'wait' | 'busy'; proxy?: boolean; expired?: boolean }
  let { state = 'idle', proxy = false, expired = false }: Props = $props();
</script>

<AccessPage title="Sign in to corsarr">
  <form class="form" onsubmit={(e) => e.preventDefault()}>
    {#if expired}<Banner tone="info" icon="clock" title="Your session ended">Sign in again to pick up where you left off.</Banner>{/if}
    <Field label="Password" error={state === 'wrong' ? 'That password is not right. Check caps lock and try again.' : undefined} disabled={state === 'wait' || state === 'busy'}>
      {#snippet children(c)}
        <TextInput type="password" autocomplete="current-password" id={c.id} aria-describedby={c.describedby} invalid={c.invalid} disabled={c.disabled} required />
      {/snippet}
    </Field>
    {#if state === 'wait'}
      <p class="wait" role="status">Too many attempts. <Countdown to={NOW + 42_000} action="You can try again" now={NOW} expiredLabel="now" /></p>
    {/if}
    <Button type="submit" variant="primary" loading={state === 'busy'} disabled={state === 'wait' || state === 'busy'}>Sign in</Button>
  </form>
  {#snippet footer()}
    {#if proxy}
      <p>Your reverse proxy at <code>10.0.0.2</code> is trusted to sign people in. This form is for direct visits on the LAN.</p>
    {:else}
      <p>Forgot the password? Set a new one with <code>corsarr admin reset-password</code> on the server.</p>
    {/if}
  {/snippet}
</AccessPage>

<style>
  .form { display: grid; gap: var(--space-4); }
  .wait { font-size: var(--text-md); color: var(--ink-muted); }
  code { font-family: var(--font-mono); font-size: var(--text-sm); color: var(--ink); }
</style>
