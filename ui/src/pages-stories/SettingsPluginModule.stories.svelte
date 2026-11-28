<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import SettingsPluginModule from './SettingsPluginModule.svelte';
  import { fullscreen, desktopLight, desktopDark, phoneLight, phoneDark } from './story-params';

  const { Story } = defineMeta({ title: 'Pages/Settings Plugin Module', component: SettingsPluginModule, parameters: fullscreen });
</script>

<Story name="Detail desktop light" globals={desktopLight} />
<Story name="Detail desktop dark" globals={desktopDark} />
<Story name="Detail phone light" globals={phoneLight} />
<Story name="Detail phone dark" globals={phoneDark} />
<Story name="Locked by config" args={{ view: 'locked' }} globals={desktopLight} />
<Story name="Failed to load" args={{ view: 'failed' }} globals={desktopDark} />
<Story name="Not installed" args={{ view: 'missing' }} globals={desktopLight} />
<Story name="Loading" args={{ view: 'loading' }} globals={phoneLight} />
