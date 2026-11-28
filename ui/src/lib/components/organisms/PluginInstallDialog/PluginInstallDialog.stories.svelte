<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import PluginInstallDialog from './PluginInstallDialog.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';
  import { catalogue } from '../../../storybook/plugin-module-fixtures';

  const dock = catalogue.offers![0];
  const quay = catalogue.offers![2];
  const labels = { url: 'Address', token: 'Access token' };
  const lock = { key: 'plugins.uiInstall', kind: 'file', file: '/config/corsarr.yml' };
  const usedBy = [
    { section: 'downloadClients', key: 'upstairs', name: 'Upstairs box', enabled: true },
    { section: 'downloadClients', key: 'cellar', name: 'Cellar', enabled: false },
  ];

  const { Story } = defineMeta({
    title: 'Organisms/PluginInstallDialog', component: PluginInstallDialog, tags: ['autodocs'],
    args: { change: 'install', name: dock.entry.name, version: dock.entry.version, permissions: dock.entry.permissions, labels, modal: false, open: true },
  });
</script>

<script lang="ts">
  let open = $state(false);
  let busy = $state(false);
  function confirm() {
    busy = true;
    setTimeout(() => { busy = false; open = false; }, 900);
  }
</script>

<Story name="Playground" />

<Story name="Install flow" asChild>
  <button type="button" onclick={() => (open = true)}>Install Dock</button>
  <PluginInstallDialog bind:open change="install" name="Dock" version="1.0.2" permissions={dock.entry.permissions} {labels} {busy} onconfirm={confirm} />
</Story>

<Story name="All states" asChild>
  <ThemePair>
    <Row label="Install"><PluginInstallDialog modal={false} open change="install" name="Dock" version="1.0.2" permissions={dock.entry.permissions} {labels} /></Row>
    <Row label="Update with grown permissions">
      <PluginInstallDialog modal={false} open change="update" name="Quay" version="0.5.0" installed="0.4.1" permissions={quay.entry.permissions} diff={quay.diff} {labels} />
    </Row>
    <Row label="Rollback"><PluginInstallDialog modal={false} open change="rollback" name="Harbour" version="1.2.0" installed="1.3.0" /></Row>
    <Row label="Uninstall listing instances"><PluginInstallDialog modal={false} open change="uninstall" name="Harbour" installed="1.3.0" {usedBy} /></Row>
    <Row label="Uninstall, unused"><PluginInstallDialog modal={false} open change="uninstall" name="Quay" installed="0.4.1" /></Row>
    <Row label="Locked by config"><PluginInstallDialog modal={false} open change="install" name="Dock" version="1.0.2" permissions={dock.entry.permissions} {lock} /></Row>
    <Row label="Busy"><PluginInstallDialog modal={false} open busy change="install" name="Dock" version="1.0.2" permissions={dock.entry.permissions} /></Row>
    <Row label="Refused by the server">
      <PluginInstallDialog modal={false} open change="install" name="Dock" version="1.0.2" permissions={dock.entry.permissions}
        error="The catalogue now offers another version of Dock. Check the new entry and confirm again." />
    </Row>
  </ThemePair>
</Story>
