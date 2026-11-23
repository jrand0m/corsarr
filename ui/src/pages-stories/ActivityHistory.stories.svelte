<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import ActivityHistory from './ActivityHistory.svelte';
  import { fullscreen, desktopLight, desktopDark, phoneLight, phoneDark } from './story-params';

  const { Story } = defineMeta({ title: 'Pages/Activity History', component: ActivityHistory, parameters: fullscreen, args: { expanded: ['e2'] } });
</script>

<Story name="Desktop light" globals={desktopLight} />
<Story name="Desktop dark" globals={desktopDark} />
<Story name="Phone light" globals={phoneLight} />
<Story name="Phone dark" globals={phoneDark} />
<Story name="Collapsed" args={{ expanded: [] }} globals={desktopLight} />
<Story name="Deliveries stream empty" args={{ stream: 'deliveries', expanded: [] }} globals={desktopDark} />
