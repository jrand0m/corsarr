<!-- First run: set the admin password, then show the API key and where config comes from. -->
<script lang="ts">
  import AccessPage from '../../../lib/components/templates/AccessPage/AccessPage.svelte';
  import Button from '../../../lib/components/atoms/Button/Button.svelte';
  import CopyableValue from '../../../lib/components/atoms/CopyableValue/CopyableValue.svelte';
  import Icon from '../../../lib/components/atoms/Icon/Icon.svelte';
  import Banner from '../../../lib/components/molecules/Banner/Banner.svelte';
  import NewPasswordField from '../../../lib/components/molecules/NewPasswordField/NewPasswordField.svelte';
  import { app } from '../../context.svelte';
  import { toApiError } from '../../api/client';
  import type { SystemStatus } from '../../api/types';
  import type { ScreenProps } from '../../screen';

  let { onsignedin }: ScreenProps = $props();
  const { api } = app();

  let password = $state('');
  let valid = $state(false);
  let tried = $state(false);
  let saving = $state(false);
  let error = $state<string | undefined>();
  let done = $state<{ apiKey: string; source?: SystemStatus['configSource'] } | undefined>();

  async function submit(e: SubmitEvent) {
    e.preventDefault();
    tried = true;
    if (!valid) return;
    saving = true;
    error = undefined;
    try {
      await api.post('auth/setup', { username: 'admin', password });
      const [key, st] = await Promise.all([
        api.get<{ apiKey: string }>('auth/apikey'),
        api.get<SystemStatus>('system/status').catch(() => undefined),
      ]);
      done = { apiKey: key.apiKey, source: st?.configSource };
    } catch (err) {
      const e2 = toApiError(err);
      if (e2.code === 'already_set_up') onsignedin?.();
      else error = e2.message;
    } finally {
      saving = false;
    }
  }

  const sourceText = (s?: SystemStatus['configSource']) =>
    !s ? undefined : s.kind === 'file' ? `the file ${s.path}` : s.kind === 'args' ? 'command-line arguments' : s.kind === 'env' ? 'environment variables' : 'built-in defaults';
</script>

{#if done}
  <AccessPage title="corsarr is ready" description="The admin password is set and you are signed in." width="wide">
    <div class="form">
      <div class="key">
        <span class="label">API key for scripts and bots</span>
        <CopyableValue label="API key" value={done.apiKey} secret />
        <span class="hint">Shown again under Settings › General. It is not needed to use the web interface.</span>
      </div>
      <Button variant="primary" icon="arrow-right" onclick={() => onsignedin?.()}>Continue to the dashboard</Button>
    </div>
    {#snippet footer()}
      {#if done?.source}<p class="source"><Icon name="file" size="sm" /><span>Configuration comes from {sourceText(done?.source)}.</span></p>{/if}
    {/snippet}
  </AccessPage>
{:else}
  <AccessPage title="Set up corsarr" description="Choose the admin password. Everyone in the household signs in with it; the user name is admin." width="wide">
    <form class="form" novalidate onsubmit={submit}>
      {#if error}<Banner tone="error" icon="alert-octagon" title="The password was not saved">{error}</Banner>{/if}
      <NewPasswordField value={password} showErrors={tried} disabled={saving} onchange={(s) => { password = s.value; valid = s.valid; }} />
      <Button type="submit" variant="primary" loading={saving} disabled={saving}>{saving ? 'Saving' : 'Set password and continue'}</Button>
    </form>
    {#snippet footer()}
      <p>The API key for scripts and bots appears once the password is set.</p>
    {/snippet}
  </AccessPage>
{/if}

<style>
  .form { display: grid; gap: var(--space-5); }
  .key { display: grid; gap: var(--space-1-5); }
  .label { font-size: var(--text-sm); font-weight: var(--weight-medium); color: var(--ink); }
  .hint { font-size: var(--text-xs); }
  .source { display: flex; align-items: center; gap: var(--space-1-5); }
</style>
