<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import WantedMissing from './WantedMissing.svelte';
  import { fullscreen, desktopLight, desktopDark, phoneLight, phoneDark } from './story-params';

  const { Story } = defineMeta({ title: 'Pages/Wanted Missing', component: WantedMissing, parameters: fullscreen });
</script>

<Story name="Desktop light" globals={desktopLight} />
<Story name="Desktop dark" globals={desktopDark} />
<Story name="Phone light" globals={phoneLight} />
<Story name="Phone dark" globals={phoneDark} />
<Story name="Selection with bulk bar" args={{ selected: ['w0', 'w2', 'w5'] }} globals={desktopLight} />
<Story name="Search all with budget" args={{ searchAllOpen: true }} globals={desktopDark} />
<Story name="Empty" args={{ empty: true }} globals={desktopLight} />
<Story name="Empty phone" args={{ empty: true }} globals={phoneDark} />
