<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import MovieDetail from './MovieDetail.svelte';
  import { fullscreen, desktopLight, desktopDark, phoneLight, phoneDark } from './story-params';

  const { Story } = defineMeta({ title: 'Pages/Movie detail', component: MovieDetail, parameters: fullscreen });
</script>

<Story name="Search streaming desktop light" globals={desktopLight} />
<Story name="Search streaming desktop dark" globals={desktopDark} />
<Story name="Search streaming phone light" globals={phoneLight} />
<Story name="Search streaming phone dark" globals={phoneDark} />
<Story name="Detail desktop light" args={{ search: false }} globals={desktopLight} />
<Story name="Detail desktop dark" args={{ search: false }} globals={desktopDark} />
<Story name="Detail phone light" args={{ search: false }} globals={phoneLight} />
<Story name="Detail phone dark" args={{ search: false }} globals={phoneDark} />
<Story name="Search finished" args={{ stream: false }} globals={desktopLight} />
