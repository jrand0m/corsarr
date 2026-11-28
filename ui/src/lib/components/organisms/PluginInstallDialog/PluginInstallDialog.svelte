<!--
  PluginInstallDialog: the stop before a plugin module changes. Install and update show what the
  module may touch (PermissionList); an update that asks for more marks the additions and asks for
  approval. Rollback names the version it returns to. Uninstall lists the configured instances that
  stop until the module is back. When the config locks module changes the dialog says where the
  lock is set and the change button stays off. Busy keeps the dialog open; a refusal from the
  server shows as an error with its reason. Focus starts on Cancel.
-->
<script lang="ts" module>
  import type { InstanceRef, ModulePermissions, PermissionDiff } from '../../../forms/plugin-types';
  import type { ManagedSource } from '../../atoms/ManagedValue/ManagedValue.svelte';

  export type ModuleChange = 'install' | 'update' | 'rollback' | 'uninstall';
  export type { InstanceRef, ManagedSource, ModulePermissions, PermissionDiff };

  /** True when an update reaches a host or a secret the approved version could not. */
  export const grows = (d?: PermissionDiff) => !!d && (d.addedHosts.length > 0 || d.addedSecrets.length > 0);
</script>

<script lang="ts">
  import Dialog from '../Dialog/Dialog.svelte';
  import Button from '../../atoms/Button/Button.svelte';
  import Icon from '../../atoms/Icon/Icon.svelte';
  import ManagedValue from '../../atoms/ManagedValue/ManagedValue.svelte';
  import Banner from '../../molecules/Banner/Banner.svelte';
  import PermissionList from '../../molecules/PermissionList/PermissionList.svelte';

  interface Props {
    open?: boolean;
    change: ModuleChange;
    /** Module name ("Harbour"). */
    name: string;
    /** The version the change leads to (install, update, rollback). */
    version?: string;
    /** The version installed now. */
    installed?: string;
    permissions?: ModulePermissions;
    diff?: PermissionDiff;
    /** Setting labels by key for permission wording. */
    labels?: Record<string, string>;
    /** Uninstall: configured instances that use the module. */
    usedBy?: InstanceRef[];
    /** Set when the config locks module changes: where the lock is set. */
    lock?: ManagedSource;
    busy?: boolean;
    /** Why the server refused the change. */
    error?: string;
    onconfirm?: () => void;
    oncancel?: () => void;
    modal?: boolean;
  }
  let {
    open = $bindable(false), change, name, version, installed, permissions, diff, labels = {}, usedBy = [], lock,
    busy = false, error, onconfirm, oncancel, modal = true,
  }: Props = $props();

  const needsApproval = $derived(change === 'update' && grows(diff));
  const title = $derived(
    change === 'install' ? `Install ${name} ${version ?? ''}?`.replace(' ?', '?')
      : change === 'update' ? `Update ${name} to ${version}?`
        : change === 'rollback' ? `Roll back ${name} to ${version}?`
          : `Uninstall ${name}?`,
  );
  const confirmLabel = $derived(
    change === 'install' ? 'Install' : change === 'update' ? (needsApproval ? 'Approve and update' : 'Update') : change === 'rollback' ? 'Roll back' : 'Uninstall',
  );
  let confirmed = false;
  function closed() {
    if (!confirmed) oncancel?.();
    confirmed = false;
  }
  function confirm() {
    confirmed = true;
    onconfirm?.();
  }
</script>

<Dialog bind:open {title} role={change === 'uninstall' ? 'alertdialog' : 'dialog'} size="md" {modal} onclose={closed} dismissible={!busy}
  description={installed && change !== 'install' ? `Installed now: ${installed}` : undefined}>
  <div class="content">
    {#if lock}
      <Banner preset="managed" title="Module changes are off">
        The config sets <ManagedValue value="plugins.uiInstall: false" managed={lock} />. Set it to true to install, update or remove modules here.
      </Banner>
    {/if}
    {#if error}
      <Banner tone="error" icon="alert-octagon" title="corsarr did not make the change" live>{error}</Banner>
    {/if}

    {#if change === 'uninstall'}
      {#if usedBy.length}
        <p>These instances stop and report "module not installed". They stay in the config and run again once {name} is installed:</p>
        <ul class="instances" aria-label="Instances that stop">
          {#each usedBy as i (`${i.section}.${i.key}`)}
            <li><Icon name="plug" size="sm" /><span>{i.name || i.key}</span><span class="ref">{i.section}.{i.key}</span></li>
          {/each}
        </ul>
      {:else}
        <p>No configured instance uses {name}.</p>
      {/if}
      <p class="note"><Icon name="undo" size="sm" /><span>Install it again from the catalogue at any time; its settings schema is kept.</span></p>
    {:else if change === 'rollback'}
      <p>corsarr loads the previous file again. Instances keep their settings.</p>
    {:else}
      {#if needsApproval}
        <Banner tone="warn" icon="shield" title="This version asks for more">Check the entries marked New before you approve.</Banner>
      {/if}
      {#if permissions}
        <section class="perms" aria-label="Permissions">
          <PermissionList {permissions} {diff} {labels} level={3} />
        </section>
      {/if}
      <p class="note"><Icon name="shield" size="sm" /><span>corsarr checks the file's size, checksum and signature against the catalogue before loading it.</span></p>
    {/if}
  </div>

  {#snippet actions()}
    <Button variant="ghost" data-autofocus disabled={busy} onclick={() => (open = false)}>Cancel</Button>
    <Button variant={change === 'uninstall' ? 'danger' : 'primary'} loading={busy} disabled={!!lock} onclick={confirm}>{confirmLabel}</Button>
  {/snippet}
</Dialog>

<style>
  .content { display: grid; gap: var(--space-4); }
  .instances { list-style: none; padding: 0; display: grid; gap: var(--space-1-5); }
  .instances li { display: flex; align-items: center; gap: var(--space-2); flex-wrap: wrap; }
  .instances li :global(svg) { color: var(--ink-muted); }
  .ref { font-family: var(--font-mono); font-size: var(--text-sm); color: var(--ink-muted); }
  .perms { padding: var(--space-3); border: var(--border-w) solid var(--line); border-radius: var(--radius-md); background: var(--surface-sunken); }
  .note { display: flex; gap: var(--space-2); align-items: flex-start; font-size: var(--text-sm); color: var(--ink-muted); }
  .note :global(svg) { margin-top: 0.125rem; flex: none; }
</style>
