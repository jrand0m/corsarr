<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import SettingsPluginModules from './SettingsPluginModules.svelte';
  import { fullscreen, desktopLight, desktopDark, phoneLight, phoneDark } from './story-params';

  const { Story } = defineMeta({ title: 'Pages/Settings Plugin Modules', component: SettingsPluginModules, parameters: fullscreen });
</script>

<Story name="List desktop light" globals={desktopLight} />
<Story name="List desktop dark" globals={desktopDark} />
<Story name="List phone light" globals={phoneLight} />
<Story name="List phone dark" globals={phoneDark} />
<Story name="Locked by config" args={{ view: 'locked' }} globals={desktopLight} />
<Story name="Locked by config phone" args={{ view: 'locked' }} globals={phoneDark} />
<Story name="Empty" args={{ view: 'empty' }} globals={desktopLight} />
<Story name="Loading" args={{ view: 'loading' }} globals={desktopDark} />
<Story name="Error" args={{ view: 'error' }} globals={desktopLight} />
