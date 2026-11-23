<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import SystemHealth from './SystemHealth.svelte';
  import { fullscreen, desktopLight, desktopDark, phoneLight, phoneDark } from './story-params';

  const { Story } = defineMeta({ title: 'Pages/System Health', component: SystemHealth, parameters: fullscreen });
</script>

<Story name="Desktop light" globals={desktopLight} />
<Story name="Desktop dark" globals={desktopDark} />
<Story name="Phone light" globals={phoneLight} />
<Story name="Phone dark" globals={phoneDark} />
<Story name="All OK" args={{ allOk: true }} globals={desktopLight} />
<Story name="All OK phone" args={{ allOk: true }} globals={phoneDark} />
