<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import SettingsIndexersScreen from './SettingsIndexersScreen.svelte';
  import { fullscreen, desktopLight, desktopDark, phoneLight, phoneDark } from './story-params';

  const { Story } = defineMeta({ title: 'Pages/Settings Indexers Screen', component: SettingsIndexersScreen, parameters: fullscreen });
</script>

<Story name="List desktop light" globals={desktopLight} />
<Story name="List desktop dark" globals={desktopDark} />
<Story name="List phone light" globals={phoneLight} />
<Story name="List phone dark" globals={phoneDark} />
<Story name="Add indexer" args={{ view: 'add' }} globals={desktopLight} />
<Story name="Add indexer phone" args={{ view: 'add' }} globals={phoneDark} />
<Story name="Config indexer locked" args={{ view: 'managed' }} globals={desktopLight} />
<Story name="Test failed" args={{ view: 'tested' }} globals={desktopDark} />
<Story name="Shared over Torznab" args={{ view: 'shared' }} globals={desktopLight} />
<Story name="Shared over Torznab phone" args={{ view: 'shared' }} globals={phoneLight} />
<Story name="Empty" args={{ view: 'empty' }} globals={desktopLight} />
