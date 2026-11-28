<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import SettingsQualityScreen from './SettingsQualityScreen.svelte';
  import { fullscreen, desktopLight, desktopDark, phoneLight, phoneDark } from './story-params';

  const { Story } = defineMeta({ title: 'Pages/Settings Profiles and Formats', component: SettingsQualityScreen, parameters: fullscreen });
</script>

<Story name="Profiles desktop light" globals={desktopLight} />
<Story name="Profiles desktop dark" globals={desktopDark} />
<Story name="Profiles phone light" globals={phoneLight} />
<Story name="Profile editor" args={{ view: 'edit' }} globals={desktopLight} />
<Story name="Profile editor phone" args={{ view: 'edit' }} globals={phoneDark} />
<Story name="Built-in profile" args={{ view: 'builtin' }} globals={desktopDark} />
<Story name="Formats desktop light" args={{ screen: 'formats' }} globals={desktopLight} />
<Story name="Formats phone dark" args={{ screen: 'formats' }} globals={phoneDark} />
<Story name="Format editor" args={{ screen: 'formats', view: 'edit' }} globals={desktopLight} />
<Story name="New format" args={{ screen: 'formats', view: 'new' }} globals={desktopDark} />
<Story name="No formats" args={{ screen: 'formats', view: 'empty' }} globals={desktopLight} />
