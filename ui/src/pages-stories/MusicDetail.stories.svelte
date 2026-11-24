<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import MusicDetail from './MusicDetail.svelte';
  import { fullscreen, desktopLight, desktopDark, phoneLight, phoneDark } from './story-params';

  const { Story } = defineMeta({ title: 'Pages/Music detail', component: MusicDetail, parameters: fullscreen });
</script>

<Story name="Artist desktop light" globals={desktopLight} />
<Story name="Artist desktop dark" globals={desktopDark} />
<Story name="Artist phone light" globals={phoneLight} />
<Story name="Artist with releases outside the profile" args={{ all: true }} globals={desktopLight} />
<Story name="Album desktop light" args={{ page: 'album' }} globals={desktopLight} />
<Story name="Album phone dark" args={{ page: 'album' }} globals={phoneDark} />
<Story name="Album interactive search desktop light" args={{ page: 'album', search: true }} globals={desktopLight} />
<Story name="Album interactive search phone dark" args={{ page: 'album', search: true }} globals={phoneDark} />
