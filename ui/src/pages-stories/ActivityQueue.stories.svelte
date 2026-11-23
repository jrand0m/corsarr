<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import ActivityQueue from './ActivityQueue.svelte';
  import { fullscreen, desktopLight, desktopDark, phoneLight, phoneDark } from './story-params';

  const { Story } = defineMeta({ title: 'Pages/Activity Queue', component: ActivityQueue, parameters: fullscreen });
</script>

<Story name="Desktop light" globals={desktopLight} />
<Story name="Desktop dark" globals={desktopDark} />
<Story name="Phone light" globals={phoneLight} />
<Story name="Phone dark" globals={phoneDark} />
<Story name="Kill confirm open" args={{ killOpen: true }} globals={desktopLight} />
<Story name="Kill confirm open phone" args={{ killOpen: true }} globals={phoneDark} />
<Story name="After kill, toast with undo" args={{ toastShown: true }} globals={desktopLight} />
<Story name="After kill, toast with undo phone" args={{ toastShown: true }} globals={phoneLight} />
<Story name="Empty" args={{ empty: true }} globals={desktopLight} />
<Story name="Empty phone" args={{ empty: true }} globals={phoneDark} />
