<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import SettingsNotifications from './SettingsNotifications.svelte';
  import { fullscreen, desktopLight, desktopDark, phoneLight, phoneDark } from './story-params';

  const { Story } = defineMeta({ title: 'Pages/Settings Notifications', component: SettingsNotifications, parameters: fullscreen });
</script>

<Story name="Desktop light" globals={desktopLight} />
<Story name="Desktop dark" globals={desktopDark} />
<Story name="Phone light" globals={phoneLight} />
<Story name="Phone dark" globals={phoneDark} />
<Story name="Raw URL tab desktop light" args={{ tab: 'raw' }} globals={desktopLight} />
<Story name="Raw URL tab phone dark" args={{ tab: 'raw' }} globals={phoneDark} />
<Story name="ntfy tab desktop dark" args={{ tab: 'ntfy' }} globals={desktopDark} />
