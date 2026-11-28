<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import SettingsPluginCatalogue from './SettingsPluginCatalogue.svelte';
  import { fullscreen, desktopLight, desktopDark, phoneLight, phoneDark } from './story-params';

  const { Story } = defineMeta({ title: 'Pages/Settings Plugin Catalogue', component: SettingsPluginCatalogue, parameters: fullscreen });
</script>

<Story name="Catalogue desktop light" globals={desktopLight} />
<Story name="Catalogue desktop dark" globals={desktopDark} />
<Story name="Catalogue phone light" globals={phoneLight} />
<Story name="Catalogue phone dark" globals={phoneDark} />
<Story name="Stale copy" args={{ view: 'stale' }} globals={desktopLight} />
<Story name="Fetch failed, no copy" args={{ view: 'failed' }} globals={desktopDark} />
<Story name="Locked by config" args={{ view: 'locked' }} globals={desktopLight} />
<Story name="Install refused" args={{ view: 'refused' }} globals={phoneLight} />
<Story name="Loading" args={{ view: 'loading' }} globals={desktopLight} />
