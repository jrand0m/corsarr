<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import SystemLogs from './SystemLogs.svelte';
  import { fullscreen, desktopLight, desktopDark, phoneLight, phoneDark } from './story-params';

  const { Story } = defineMeta({ title: 'Pages/System Logs', component: SystemLogs, parameters: fullscreen });
</script>

<Story name="Desktop light" globals={desktopLight} />
<Story name="Desktop dark" globals={desktopDark} />
<Story name="Phone light" globals={phoneLight} />
<Story name="Phone dark" globals={phoneDark} />
<Story name="Loading the viewer" args={{ loading: true }} globals={desktopLight} />
<Story name="Live tail" args={{ live: true }} globals={desktopDark} />
<Story name="Disconnected" args={{ connection: 'offline' }} globals={phoneLight} />
