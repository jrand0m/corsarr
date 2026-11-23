<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import LibraryMovies from './LibraryMovies.svelte';
  import { fullscreen, desktopLight, desktopDark, phoneLight, phoneDark } from './story-params';

  const { Story } = defineMeta({ title: 'Pages/Library Movies', component: LibraryMovies, parameters: fullscreen });
</script>

<Story name="Table desktop light" globals={desktopLight} />
<Story name="Table desktop dark" globals={desktopDark} />
<Story name="Table phone light" globals={phoneLight} />
<Story name="Table phone dark" globals={phoneDark} />
<Story name="Posters desktop light" args={{ view: 'posters' }} globals={desktopLight} />
<Story name="Posters desktop dark" args={{ view: 'posters' }} globals={desktopDark} />
<Story name="Posters phone light" args={{ view: 'posters' }} globals={phoneLight} />
<Story name="Posters phone dark" args={{ view: 'posters' }} globals={phoneDark} />
<Story name="Selection with bulk bar" args={{ selected: ['m1', 'm3', 'm8'] }} globals={desktopLight} />
