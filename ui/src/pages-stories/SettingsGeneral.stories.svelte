<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import SettingsGeneral from './SettingsGeneral.svelte';
  import { fullscreen, desktopLight, desktopDark, phoneLight, phoneDark } from './story-params';

  const { Story } = defineMeta({ title: 'Pages/Settings General', component: SettingsGeneral, parameters: fullscreen });
</script>

<Story name="Desktop light" globals={desktopLight} />
<Story name="Desktop dark" globals={desktopDark} />
<Story name="Phone light" globals={phoneLight} />
<Story name="Phone dark" globals={phoneDark} />
<Story name="Regenerate API key confirm" args={{ regenerate: true }} globals={desktopLight} />
<Story name="Regenerate API key confirm phone dark" args={{ regenerate: true }} globals={phoneDark} />
