<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import AddSeries from './AddSeries.svelte';
  import { fullscreen, desktopLight, desktopDark, phoneLight, phoneDark } from './story-params';

  const { Story } = defineMeta({ title: 'Pages/Add series', component: AddSeries, parameters: fullscreen });
</script>

<Story name="Desktop light" globals={desktopLight} />
<Story name="Desktop dark" globals={desktopDark} />
<Story name="Phone light" globals={phoneLight} />
<Story name="Phone dark" globals={phoneDark} />
<Story name="Searching" args={{ phase: 'searching' }} globals={desktopLight} />
<Story name="Provider offline, phone" args={{ phase: 'offline' }} globals={phoneDark} />
