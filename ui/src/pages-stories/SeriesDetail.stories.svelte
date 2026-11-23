<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import SeriesDetail from './SeriesDetail.svelte';
  import { fullscreen, desktopLight, desktopDark, phoneLight, phoneDark } from './story-params';

  const { Story } = defineMeta({ title: 'Pages/Series detail', component: SeriesDetail, parameters: fullscreen });
</script>

<Story name="Seasons desktop light" globals={desktopLight} />
<Story name="Seasons desktop dark" globals={desktopDark} />
<Story name="Seasons phone light" globals={phoneLight} />
<Story name="Seasons phone dark" globals={phoneDark} />
<Story name="Season search desktop light" args={{ search: true }} globals={desktopLight} />
<Story name="Season search phone dark" args={{ search: true }} globals={phoneDark} />
