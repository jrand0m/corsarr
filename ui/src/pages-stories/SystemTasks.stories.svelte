<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import SystemTasks from './SystemTasks.svelte';
  import { fullscreen, desktopLight, desktopDark, phoneLight, phoneDark } from './story-params';

  const { Story } = defineMeta({ title: 'Pages/System Tasks', component: SystemTasks, parameters: fullscreen });
</script>

<Story name="Desktop light" globals={desktopLight} />
<Story name="Desktop dark" globals={desktopDark} />
<Story name="Phone light" globals={phoneLight} />
<Story name="Phone dark" globals={phoneDark} />
