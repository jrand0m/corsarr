<!-- Page composition (story only): S01 first run, set the admin password. -->
<script lang="ts">
  import AccessPage from '../lib/components/templates/AccessPage/AccessPage.svelte';
  import Button from '../lib/components/atoms/Button/Button.svelte';
  import CopyableValue from '../lib/components/atoms/CopyableValue/CopyableValue.svelte';
  import Icon from '../lib/components/atoms/Icon/Icon.svelte';
  import NewPasswordField from '../lib/components/molecules/NewPasswordField/NewPasswordField.svelte';

  interface Props { value?: string; confirm?: string; showErrors?: boolean; saving?: boolean; error?: string }
  let { value = '', confirm = '', showErrors = false, saving = false, error }: Props = $props();
  // svelte-ignore state_referenced_locally
  let tried = $state(showErrors);
  let valid = $state(false);
</script>

<AccessPage title="Set up corsarr" description="Choose the admin password. Everyone in the household signs in with it; you can change it later in Settings." width="wide">
  <form class="form" novalidate onsubmit={(e) => { e.preventDefault(); tried = true; }}>
    <NewPasswordField {value} {confirm} showErrors={tried} disabled={saving} {error} onchange={(s) => (valid = s.valid)} />
    <Button type="submit" variant="primary" loading={saving} disabled={saving}>{saving ? 'Saving' : 'Set password and continue'}</Button>
  </form>
  {#snippet footer()}
    <div class="key">
      <span class="label">API key for scripts and bots</span>
      <CopyableValue label="API key" value="c0rs-4f9e1b7a2d6c4e08b3a5" secret />
      <span class="hint">Shown again under Settings › General. It is not needed to use the web interface.</span>
    </div>
    <p class="source"><Icon name="file" size="sm" /><span>Configuration comes from <code>/config/corsarr.yaml</code>.</span></p>
  {/snippet}
</AccessPage>

<style>
  .form { display: grid; gap: var(--space-5); }
  .key { display: grid; gap: var(--space-1-5); }
  .label { font-size: var(--text-sm); font-weight: var(--weight-medium); color: var(--ink); }
  .hint { font-size: var(--text-xs); }
  .source { display: flex; align-items: center; gap: var(--space-1-5); }
  code { font-family: var(--font-mono); font-size: var(--text-sm); color: var(--ink); }
</style>
