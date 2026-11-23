<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import SettingsIndexers from './SettingsIndexers.svelte';
  import { fullscreen, desktopLight, desktopDark, phoneLight, phoneDark } from './story-params';

  const { Story } = defineMeta({ title: 'Pages/Settings Indexers', component: SettingsIndexers, parameters: fullscreen });
</script>

<Story name="Editor desktop light" globals={desktopLight} />
<Story name="Editor desktop dark" globals={desktopDark} />
<Story name="Editor phone light" globals={phoneLight} />
<Story name="Editor phone dark" globals={phoneDark} />
<Story name="Managed desktop light" args={{ view: 'managed' }} globals={desktopLight} />
<Story name="Managed desktop dark" args={{ view: 'managed' }} globals={desktopDark} />
<Story name="Managed phone light" args={{ view: 'managed' }} globals={phoneLight} />
<Story name="Managed phone dark" args={{ view: 'managed' }} globals={phoneDark} />
<Story name="List desktop light" args={{ view: 'list' }} globals={desktopLight} />
<Story name="List phone dark" args={{ view: 'list' }} globals={phoneDark} />
